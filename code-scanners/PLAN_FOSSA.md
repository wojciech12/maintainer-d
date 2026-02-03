# Fossa Integration Implementation Plan

**Last Updated**: 2026-02-04
**Status**: Revised after API research

## Overview

Implement actual Fossa integration for the `CodeScannerFossa` controller. The controller will create FOSSA **Teams** (not Projects) via the FOSSA API using credentials from a Kubernetes secret.

**Important**: See `FOSSA_RESEARCH.md` for detailed API research findings that informed this plan.

## Key Clarification: Teams vs Projects

After researching the FOSSA API (see `FOSSA_RESEARCH.md`):

- **FOSSA Projects**: Code repositories being scanned, created via `fossa analyze` CLI or Quick Import (NOT via direct API)
- **FOSSA Teams**: Organizational units for RBAC, created via `POST /api/teams` ✅

**This operator creates FOSSA Teams**, following the existing pattern in `/plugins/fossa/client.go` and `/onboarding/server.go`.

## Requirements (Clarified)

1. Read the CodeScannerFossa CRD
2. Extract `projectName` from spec (represents CNCF project name)
3. Create a **FOSSA Team** named after the project
4. Retrieve FOSSA token and organization ID from secret `code-scanners`
5. Store team details in ConfigMap and CR status

## Architecture

```
CodeScannerFossa CR (projectName: "argo")
        ↓
    Controller
        ↓
    ┌───────────────────┐
    │ Get Secret        │
    │ code-scanners     │
    │ - fossa-api-token │
    │ - fossa-org-id    │
    └───────────────────┘
        ↓
    ┌───────────────────┐
    │ Existing Client   │ ──→ POST /api/teams
    │ plugins/fossa/    │      {"name": "argo"}
    └───────────────────┘
        ↓
    ┌───────────────────┐
    │ Create ConfigMap  │
    │ - TeamID: 123     │
    │ - TeamName: argo  │
    └───────────────────┘
        ↓
    ┌───────────────────┐
    │ Update CR Status  │
    │ - fossaTeam.id    │
    │ - conditions      │
    └───────────────────┘
```

## Implementation Phases

### Phase 1: Import Existing Fossa Client ✅ COMPLETE

**Goal:** Reuse the existing, battle-tested FOSSA client

**Decision**: Use existing `/plugins/fossa/client.go` instead of creating new client

**Rationale**:
- Already implements `CreateTeam()` and `FetchTeam()`
- ~700 lines of tested code
- Consistent with webhook server
- Avoids code duplication

**Tasks:**

1. ✅ Import client in controller:
   ```go
   import "github.com/cncf/maintainer-d/plugins/fossa"
   ```

2. ✅ Review existing client capabilities:
   - ✅ `CreateTeam(name string) (*Team, error)` - Line 432
   - ✅ `FetchTeam(name string) (*Team, error)` - Line 206
   - ✅ `GetTeam(teamID int) (*Team, error)` - Line 406
   - ✅ Error handling with idempotency (code 2003)
   - ✅ HTTP client with proper auth headers

3. ✅ No new files to create - just import

**Files modified:**
- ✅ `code-scanners/internal/controller/codescannerfossa_controller.go` - Added import

**Testing approach:**
- Use existing mock patterns from `/onboarding/fossa_mock.go`
- Mock `FossaClient` interface in controller tests

**Dependencies:**
- None new - already in parent project

---

### Phase 2: Secret Management and Credentials ✅ COMPLETE

**Goal:** Add secret reading capability to controller

**Tasks:**

1. Define secret structure (document in comments):
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: code-scanners
     namespace: code-scanners  # Same namespace as CR
   type: Opaque
   stringData:
     fossa-api-token: "<your-fossa-api-token>"
     fossa-organization-id: "162"  # Numeric, currently hardcoded in client.go:166
   ```

2. Add constants in `internal/controller/constants.go`:
   ```go
   const (
       // ... existing constants ...

       // SecretName is the name of the secret containing scanner credentials
       SecretName = "code-scanners"

       // SecretKeyFossaToken is the key for FOSSA API token
       SecretKeyFossaToken = "fossa-api-token"

       // SecretKeyFossaOrgID is the key for FOSSA organization ID
       SecretKeyFossaOrgID = "fossa-organization-id"

       // Condition types
       ConditionTypeFossaTeamReady = "FossaTeamReady"
       ConditionTypeConfigMapReady = "ConfigMapReady"

       // Condition reasons
       ReasonTeamCreated         = "TeamCreated"
       ReasonTeamExists          = "TeamExists"
       ReasonFossaAPIError       = "APIError"
       ReasonCredentialsNotFound = "CredentialsNotFound"
       ReasonConfigMapCreated    = "ConfigMapCreated"
   )
   ```

3. Add credentials helper to controller:
   ```go
   // getFossaCredentials retrieves FOSSA credentials from the secret
   func (r *CodeScannerFossaReconciler) getFossaCredentials(ctx context.Context, namespace string) (token, orgID string, err error) {
       log := logf.FromContext(ctx)

       secret := &corev1.Secret{}
       key := client.ObjectKey{
           Name:      SecretName,
           Namespace: namespace,
       }

       if err := r.Get(ctx, key, secret); err != nil {
           if errors.IsNotFound(err) {
               return "", "", fmt.Errorf("secret %s not found in namespace %s", SecretName, namespace)
           }
           return "", "", fmt.Errorf("failed to get secret %s: %w", SecretName, err)
       }

       token = string(secret.Data[SecretKeyFossaToken])
       orgID = string(secret.Data[SecretKeyFossaOrgID])

       if token == "" {
           return "", "", fmt.Errorf("missing %s in secret", SecretKeyFossaToken)
       }
       if orgID == "" {
           return "", "", fmt.Errorf("missing %s in secret", SecretKeyFossaOrgID)
       }

       log.V(1).Info("Retrieved FOSSA credentials", "orgID", orgID)
       // NEVER log token value
       return token, orgID, nil
   }
   ```

4. Update RBAC markers in `codescannerfossa_controller.go`:
   ```go
   // +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
   // +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
   ```

5. Run `make manifests` to update RBAC

**Files to modify:**
- `internal/controller/constants.go`
- `internal/controller/codescannerfossa_controller.go`

**Files to regenerate:**
- `config/rbac/role.yaml` (via make manifests)

**Testing approach:**
- Unit test with envtest: create secret and verify retrieval
- Test missing secret error
- Test missing keys error
- Test empty values error

---

### Phase 3: Update CRD Status

**Goal:** Extend status to track FOSSA Team details (not Project)

**Tasks:**

1. Update `CodeScannerFossaStatus` in `api/v1alpha1/codescannerfossa_types.go`:
   ```go
   type CodeScannerFossaStatus struct {
       // ObservedGeneration is the generation observed by the controller
       // +optional
       ObservedGeneration int64 `json:"observedGeneration,omitempty"`

       // ConfigMapRef is the namespace/name reference to the created ConfigMap
       // +optional
       ConfigMapRef string `json:"configMapRef,omitempty"`

       // FossaTeam contains details about the created FOSSA Team
       // +optional
       FossaTeam *FossaTeamReference `json:"fossaTeam,omitempty"`

       // Conditions represent the latest available observations of the resource's state
       // +listType=map
       // +listMapKey=type
       // +optional
       Conditions []metav1.Condition `json:"conditions,omitempty"`
   }

   // FossaTeamReference contains details about the FOSSA Team
   type FossaTeamReference struct {
       // ID is the FOSSA team ID
       ID int `json:"id"`

       // Name is the team name (matches projectName)
       Name string `json:"name"`

       // OrganizationID is the FOSSA organization ID
       OrganizationID int `json:"organizationId"`

       // URL is a link to the team in FOSSA UI
       // +optional
       URL string `json:"url,omitempty"`

       // CreatedAt is when the team was created in FOSSA
       // +optional
       CreatedAt *metav1.Time `json:"createdAt,omitempty"`
   }
   ```

2. Add print columns to show FOSSA team status:
   ```go
   // +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.projectName`
   // +kubebuilder:printcolumn:name="FossaTeamID",type=integer,JSONPath=`.status.fossaTeam.id`
   // +kubebuilder:printcolumn:name="ConfigMap",type=string,JSONPath=`.status.configMapRef`
   // +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="FossaTeamReady")].status`
   // +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
   ```

3. Run `make manifests generate` to update CRD

**Files to modify:**
- `api/v1alpha1/codescannerfossa_types.go`

**Files to regenerate:**
- `config/crd/bases/maintainer-d.cncf.io_codescannerfossas.yaml`

**Testing approach:**
- Verify CRD generation
- Test status updates in controller tests
- Verify print columns display correctly

---

### Phase 4: Controller Reconciliation Logic

**Goal:** Implement FOSSA Team creation in controller

**Tasks:**

1. Add FOSSA client interface to reconciler (for testability):
   ```go
   // FossaClient defines the interface for FOSSA operations needed by the controller
   type FossaClient interface {
       CreateTeam(name string) (*fossa.Team, error)
       FetchTeam(name string) (*fossa.Team, error)
   }

   // Ensure the real client implements the interface
   var _ FossaClient = (*fossa.Client)(nil)

   type CodeScannerFossaReconciler struct {
       client.Client
       Scheme   *runtime.Scheme
       Recorder record.EventRecorder

       // FossaClientFactory creates FOSSA clients (injectable for testing)
       FossaClientFactory func(token string) FossaClient
   }
   ```

2. Update `SetupWithManager` to initialize factory:
   ```go
   func (r *CodeScannerFossaReconciler) SetupWithManager(mgr ctrl.Manager) error {
       // Initialize event recorder
       if r.Recorder == nil {
           r.Recorder = mgr.GetEventRecorderFor("codescannerfossa-controller")
       }

       // Initialize FOSSA client factory
       if r.FossaClientFactory == nil {
           r.FossaClientFactory = func(token string) FossaClient {
               return fossa.NewClient(token)
           }
       }

       return ctrl.NewControllerManagedBy(mgr).
           For(&maintainerdcncfiov1alpha1.CodeScannerFossa{}).
           Owns(&corev1.ConfigMap{}).
           Named("codescannerfossa").
           Complete(r)
   }
   ```

3. Rewrite `Reconcile` method with FOSSA Team creation:
   ```go
   func (r *CodeScannerFossaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
       log := logf.FromContext(ctx)

       // 1. Fetch CR
       fossaCR := &maintainerdcncfiov1alpha1.CodeScannerFossa{}
       if err := r.Get(ctx, req.NamespacedName, fossaCR); err != nil {
           if errors.IsNotFound(err) {
               log.Info("CodeScannerFossa resource not found, ignoring")
               return ctrl.Result{}, nil
           }
           log.Error(err, "Failed to get CodeScannerFossa")
           return ctrl.Result{}, err
       }

       // 2. Handle deletion (finalizer logic will be added later)
       // For now, ConfigMap is deleted via owner reference

       // 3. Get FOSSA credentials
       token, orgID, err := r.getFossaCredentials(ctx, fossaCR.Namespace)
       if err != nil {
           log.Error(err, "Failed to get FOSSA credentials")
           r.setCondition(fossaCR, ConditionTypeFossaTeamReady, metav1.ConditionFalse,
                          ReasonCredentialsNotFound, err.Error())
           if updateErr := r.Status().Update(ctx, fossaCR); updateErr != nil {
               log.Error(updateErr, "Failed to update status")
               return ctrl.Result{}, updateErr
           }
           r.Recorder.Event(fossaCR, corev1.EventTypeWarning, ReasonCredentialsNotFound, err.Error())
           // Don't requeue - requires manual intervention
           return ctrl.Result{}, nil
       }

       // 4. Create FOSSA client
       fossaClient := r.FossaClientFactory(token)

       // 5. Ensure FOSSA Team exists
       team, err := r.ensureFossaTeam(ctx, fossaClient, fossaCR.Spec.ProjectName)
       if err != nil {
           log.Error(err, "Failed to ensure FOSSA team")
           r.setCondition(fossaCR, ConditionTypeFossaTeamReady, metav1.ConditionFalse,
                          ReasonFossaAPIError, err.Error())
           if updateErr := r.Status().Update(ctx, fossaCR); updateErr != nil {
               log.Error(updateErr, "Failed to update status")
           }
           r.Recorder.Event(fossaCR, corev1.EventTypeWarning, ReasonFossaAPIError, err.Error())
           // Requeue for transient errors
           return ctrl.Result{RequeueAfter: time.Minute}, err
       }

       // 6. Update status with FOSSA team details
       orgIDInt, _ := strconv.Atoi(orgID)
       fossaCR.Status.ObservedGeneration = fossaCR.Generation
       fossaCR.Status.FossaTeam = &maintainerdcncfiov1alpha1.FossaTeamReference{
           ID:             team.ID,
           Name:           team.Name,
           OrganizationID: orgIDInt,
           URL:            fmt.Sprintf("https://app.fossa.com/account/settings/organization/teams/%d", team.ID),
           CreatedAt:      &metav1.Time{Time: team.CreatedAt},
       }
       r.setCondition(fossaCR, ConditionTypeFossaTeamReady, metav1.ConditionTrue,
                      ReasonTeamCreated, fmt.Sprintf("FOSSA team %q ready (ID: %d)", team.Name, team.ID))

       // 7. Create/update ConfigMap
       configMap := r.configMapForFossa(fossaCR, team)
       if err := ctrl.SetControllerReference(fossaCR, configMap, r.Scheme); err != nil {
           log.Error(err, "Failed to set owner reference on ConfigMap")
           return ctrl.Result{}, err
       }

       existingCM := &corev1.ConfigMap{}
       err = r.Get(ctx, client.ObjectKeyFromObject(configMap), existingCM)
       if err != nil && errors.IsNotFound(err) {
           log.Info("Creating ConfigMap", "name", configMap.Name, "namespace", configMap.Namespace)
           if err := r.Create(ctx, configMap); err != nil {
               log.Error(err, "Failed to create ConfigMap")
               return ctrl.Result{}, err
           }
           r.Recorder.Event(fossaCR, corev1.EventTypeNormal, ReasonConfigMapCreated,
                           fmt.Sprintf("ConfigMap %s created", configMap.Name))
       } else if err == nil {
           // Only update if data changed
           if !reflect.DeepEqual(existingCM.Data, configMap.Data) {
               existingCM.Data = configMap.Data
               if err := r.Update(ctx, existingCM); err != nil {
                   log.Error(err, "Failed to update ConfigMap")
                   return ctrl.Result{}, err
               }
               log.Info("Updated ConfigMap", "name", configMap.Name)
           }
       } else {
           log.Error(err, "Failed to get ConfigMap")
           return ctrl.Result{}, err
       }

       r.setCondition(fossaCR, ConditionTypeConfigMapReady, metav1.ConditionTrue,
                      ReasonConfigMapCreated, "ConfigMap ready")

       // 8. Update annotations
       configMapRef := fmt.Sprintf("%s/%s", configMap.Namespace, configMap.Name)
       if fossaCR.Annotations == nil {
           fossaCR.Annotations = make(map[string]string)
       }
       if fossaCR.Annotations[AnnotationConfigMapRef] != configMapRef {
           patch := client.MergeFrom(fossaCR.DeepCopy())
           fossaCR.Annotations[AnnotationConfigMapRef] = configMapRef
           if err := r.Patch(ctx, fossaCR, patch); err != nil {
               log.Error(err, "Failed to update annotation")
               return ctrl.Result{}, err
           }
       }

       // 9. Update status
       fossaCR.Status.ConfigMapRef = configMapRef
       if err := r.Status().Update(ctx, fossaCR); err != nil {
           log.Error(err, "Failed to update status")
           return ctrl.Result{}, err
       }

       log.Info("Reconciliation complete",
                "fossaTeamID", team.ID,
                "fossaTeamName", team.Name,
                "configMap", configMapRef)
       r.Recorder.Event(fossaCR, corev1.EventTypeNormal, "Reconciled",
                       fmt.Sprintf("FOSSA team %q (ID: %d) ready", team.Name, team.ID))
       return ctrl.Result{}, nil
   }
   ```

4. Add helper method for team creation:
   ```go
   func (r *CodeScannerFossaReconciler) ensureFossaTeam(ctx context.Context, client FossaClient, teamName string) (*fossa.Team, error) {
       log := logf.FromContext(ctx)

       // Try to get existing team
       log.V(1).Info("Checking if FOSSA team exists", "teamName", teamName)
       team, err := client.FetchTeam(teamName)
       if err == nil {
           log.Info("FOSSA team already exists", "teamName", teamName, "teamID", team.ID)
           return team, nil
       }

       // Create new team
       log.Info("Creating FOSSA team", "teamName", teamName)
       team, err = client.CreateTeam(teamName)
       if err != nil {
           return nil, fmt.Errorf("failed to create FOSSA team: %w", err)
       }

       log.Info("FOSSA team created", "teamName", team.Name, "teamID", team.ID)
       return team, nil
   }
   ```

5. Add helper for condition management:
   ```go
   func (r *CodeScannerFossaReconciler) setCondition(cr *maintainerdcncfiov1alpha1.CodeScannerFossa, condType string, status metav1.ConditionStatus, reason, message string) {
       condition := metav1.Condition{
           Type:               condType,
           Status:             status,
           ObservedGeneration: cr.Generation,
           LastTransitionTime: metav1.Now(),
           Reason:             reason,
           Message:            message,
       }

       // Find and update existing condition or append new one
       found := false
       for i, c := range cr.Status.Conditions {
           if c.Type == condType {
               // Only update if status or reason changed
               if c.Status != status || c.Reason != reason {
                   cr.Status.Conditions[i] = condition
               }
               found = true
               break
           }
       }
       if !found {
           cr.Status.Conditions = append(cr.Status.Conditions, condition)
       }
   }
   ```

6. Update ConfigMap builder to include FOSSA team details:
   ```go
   func (r *CodeScannerFossaReconciler) configMapForFossa(cr *maintainerdcncfiov1alpha1.CodeScannerFossa, team *fossa.Team) *corev1.ConfigMap {
       data := map[string]string{
           ConfigMapKeyCodeScanner: ScannerTypeFossa,
           ConfigMapKeyProjectName: cr.Spec.ProjectName,
       }

       if team != nil {
           data["FossaTeamID"] = strconv.Itoa(team.ID)
           data["FossaTeamName"] = team.Name
           data["FossaTeamURL"] = fmt.Sprintf("https://app.fossa.com/account/settings/organization/teams/%d", team.ID)
       }

       return &corev1.ConfigMap{
           ObjectMeta: metav1.ObjectMeta{
               Name:      cr.Name,
               Namespace: cr.Namespace,
           },
           Data: data,
       }
   }
   ```

**Files to modify:**
- `internal/controller/codescannerfossa_controller.go`

**Testing approach:**
- Unit tests with mocked FOSSA client
- Test scenarios:
  - Successful team creation
  - Team already exists (idempotent)
  - Missing credentials
  - API errors
  - ConfigMap creation/update
  - Condition updates

---

### Phase 5: Testing

**Goal:** Comprehensive testing coverage

**Tasks:**

1. Update controller tests (`internal/controller/codescannerfossa_controller_test.go`):
   ```go
   // Mock FOSSA client for testing
   type mockFossaClient struct {
       teams      map[string]*fossa.Team
       createErr  error
       fetchErr   error
       nextTeamID int
   }

   func newMockFossaClient() *mockFossaClient {
       return &mockFossaClient{
           teams:      make(map[string]*fossa.Team),
           nextTeamID: 1,
       }
   }

   func (m *mockFossaClient) CreateTeam(name string) (*fossa.Team, error) {
       if m.createErr != nil {
           return nil, m.createErr
       }
       if team, exists := m.teams[name]; exists {
           return team, nil
       }
       team := &fossa.Team{
           ID:             m.nextTeamID,
           Name:           name,
           OrganizationID: 162,
           CreatedAt:      time.Now(),
       }
       m.teams[name] = team
       m.nextTeamID++
       return team, nil
   }

   func (m *mockFossaClient) FetchTeam(name string) (*fossa.Team, error) {
       if m.fetchErr != nil {
           return nil, m.fetchErr
       }
       if team, exists := m.teams[name]; exists {
           return team, nil
       }
       return nil, fmt.Errorf("team not found: %s", name)
   }

   func TestReconcile_CreatesFossaTeam(t *testing.T) {
       // Setup
       ctx := context.Background()
       fossaCR := &maintainerdcncfiov1alpha1.CodeScannerFossa{
           ObjectMeta: metav1.ObjectMeta{
               Name:      "test-project",
               Namespace: "code-scanners",
           },
           Spec: maintainerdcncfiov1alpha1.CodeScannerFossaSpec{
               ProjectName: "test-project",
           },
       }

       // Create secret
       secret := &corev1.Secret{
           ObjectMeta: metav1.ObjectMeta{
               Name:      "code-scanners",
               Namespace: "code-scanners",
           },
           Data: map[string][]byte{
               "fossa-api-token":      []byte("test-token"),
               "fossa-organization-id": []byte("162"),
           },
       }

       // Setup k8s client with envtest
       k8sClient := setupTestClient(t, fossaCR, secret)

       // Setup reconciler with mock FOSSA client
       mockClient := newMockFossaClient()
       reconciler := &CodeScannerFossaReconciler{
           Client: k8sClient,
           Scheme: scheme.Scheme,
           FossaClientFactory: func(token string) FossaClient {
               return mockClient
           },
       }

       // Reconcile
       req := ctrl.Request{
           NamespacedName: client.ObjectKeyFromObject(fossaCR),
       }
       result, err := reconciler.Reconcile(ctx, req)

       // Assertions
       if err != nil {
           t.Errorf("Reconcile failed: %v", err)
       }
       if result.Requeue || result.RequeueAfter > 0 {
           t.Error("Unexpected requeue")
       }

       // Verify team created
       if len(mockClient.teams) != 1 {
           t.Errorf("Expected 1 team, got %d", len(mockClient.teams))
       }
       if _, exists := mockClient.teams["test-project"]; !exists {
           t.Error("Team 'test-project' not created")
       }

       // Verify status updated
       updated := &maintainerdcncfiov1alpha1.CodeScannerFossa{}
       if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(fossaCR), updated); err != nil {
           t.Fatalf("Failed to get updated CR: %v", err)
       }
       if updated.Status.FossaTeam == nil {
           t.Error("Status.FossaTeam not set")
       }
       if updated.Status.FossaTeam.Name != "test-project" {
           t.Errorf("Expected team name 'test-project', got %q", updated.Status.FossaTeam.Name)
       }

       // Verify ConfigMap created
       cm := &corev1.ConfigMap{}
       cmKey := client.ObjectKey{Name: "test-project", Namespace: "code-scanners"}
       if err := k8sClient.Get(ctx, cmKey, cm); err != nil {
           t.Fatalf("ConfigMap not created: %v", err)
       }
       if cm.Data["FossaTeamName"] != "test-project" {
           t.Errorf("ConfigMap FossaTeamName incorrect: %q", cm.Data["FossaTeamName"])
       }
   }

   func TestReconcile_MissingSecret(t *testing.T) {
       // Test that reconcile handles missing secret gracefully
       // Should set condition and not requeue
   }

   func TestReconcile_Idempotency(t *testing.T) {
       // Test that multiple reconciles don't create duplicate teams
   }

   func TestReconcile_APIError(t *testing.T) {
       // Test handling of FOSSA API errors
       // Should set condition and requeue
   }
   ```

2. Integration test checklist:
   - [ ] Deploy operator to test cluster
   - [ ] Create secret with real FOSSA credentials
   - [ ] Create CodeScannerFossa CR
   - [ ] Verify FOSSA team created in UI: `https://app.fossa.com/account/settings/organization/teams`
   - [ ] Verify ConfigMap has correct data
   - [ ] Verify CR status shows team ID and URL
   - [ ] Delete CR, verify ConfigMap deleted (owner reference)
   - [ ] Create CR with same name (idempotency test)
   - [ ] Test with invalid token
   - [ ] Test with missing secret

**Files to modify:**
- `internal/controller/codescannerfossa_controller_test.go`

---

### Phase 6: Documentation and Samples

**Goal:** Provide complete setup documentation

**Tasks:**

1. Create secret template in `config/samples/`:
   ```yaml
   # config/samples/secret-code-scanners.yaml
   ---
   apiVersion: v1
   kind: Secret
   metadata:
     name: code-scanners
     namespace: code-scanners
   type: Opaque
   stringData:
     # Get your FOSSA API token from: https://app.fossa.com/account/settings/integrations/api_tokens
     # Use a "Full API Token" (not Push-Only) with team creation permissions
     fossa-api-token: "your-fossa-api-token-here"

     # Your FOSSA organization ID (numeric string)
     # Find this in your FOSSA organization settings or contact FOSSA support
     fossa-organization-id: "162"
   ```

2. Update CR sample with detailed comments:
   ```yaml
   # config/samples/maintainer-d.cncf.io_v1alpha1_codescannerfossa.yaml
   ---
   apiVersion: maintainer-d.cncf.io/v1alpha1
   kind: CodeScannerFossa
   metadata:
     name: argo-fossa
     namespace: code-scanners
   spec:
     # ProjectName is the name of the CNCF project
     # A FOSSA Team will be created with this name for RBAC purposes
     projectName: argo

   # Status will be updated automatically by the controller:
   # status:
   #   observedGeneration: 1
   #   configMapRef: code-scanners/argo-fossa
   #   fossaTeam:
   #     id: 123
   #     name: argo
   #     organizationId: 162
   #     url: https://app.fossa.com/account/settings/organization/teams/123
   #     createdAt: "2026-02-04T10:00:00Z"
   #   conditions:
   #   - type: FossaTeamReady
   #     status: "True"
   #     reason: TeamCreated
   #     message: "FOSSA team 'argo' ready (ID: 123)"
   #   - type: ConfigMapReady
   #     status: "True"
   #     reason: ConfigMapCreated
   ```

3. Create setup guide:
   ```markdown
   # code-scanners/FOSSA_SETUP.md

   ## Prerequisites
   - FOSSA account with Enterprise access (API available to Enterprise customers)
   - FOSSA Full API Token (not Push-Only token)
   - FOSSA organization ID
   - Kubernetes cluster with code-scanners operator deployed

   ## Understanding FOSSA Teams vs Projects

   **Important**: This operator creates FOSSA **Teams**, not Projects.

   - **FOSSA Teams**: Organizational units for RBAC (created by this operator)
   - **FOSSA Projects**: Code repositories being scanned (created via `fossa analyze` CLI)

   See `FOSSA_RESEARCH.md` for detailed explanation.

   ## Setup Steps

   ### 1. Get FOSSA Credentials

   **API Token**:
   1. Log in to FOSSA
   2. Go to: https://app.fossa.com/account/settings/integrations/api_tokens
   3. Click "Generate Token"
   4. Select "Full API Token" (not Push-Only)
   5. Copy the token

   **Organization ID**:
   - Contact FOSSA support or check your organization settings
   - It's a numeric value (e.g., "162")

   ### 2. Create Secret

   ```bash
   kubectl create namespace code-scanners

   kubectl create secret generic code-scanners \
     --from-literal=fossa-api-token='your-token-here' \
     --from-literal=fossa-organization-id='162' \
     --namespace=code-scanners
   ```

   Or apply the secret YAML:
   ```bash
   kubectl apply -f config/samples/secret-code-scanners.yaml
   ```

   ### 3. Create CodeScannerFossa CR

   ```bash
   kubectl apply -f config/samples/maintainer-d.cncf.io_v1alpha1_codescannerfossa.yaml
   ```

   ### 4. Verify Team Created in FOSSA

   Check the CR status:
   ```bash
   kubectl get codescannerfossa argo-fossa -n code-scanners -o yaml
   ```

   Look for:
   ```yaml
   status:
     fossaTeam:
       id: 123
       url: https://app.fossa.com/account/settings/organization/teams/123
   ```

   Visit the URL to verify the team exists in FOSSA.

   ### 5. Check ConfigMap

   ```bash
   kubectl get configmap argo-fossa -n code-scanners -o yaml
   ```

   Should contain:
   ```yaml
   data:
     CodeScanner: Fossa
     ProjectName: argo
     FossaTeamID: "123"
     FossaTeamName: argo
     FossaTeamURL: https://app.fossa.com/account/settings/organization/teams/123
   ```

   ## Troubleshooting

   ### Team Not Created

   Check CR conditions:
   ```bash
   kubectl describe codescannerfossa argo-fossa -n code-scanners
   ```

   Look for condition messages.

   ### Secret Not Found

   Error: `secret code-scanners not found in namespace code-scanners`

   Solution: Create the secret in the same namespace as the CR.

   ### Invalid Token

   Error: `Failed to ensure FOSSA team: request failed: ...`

   Solution: Verify token is a "Full API Token" with correct permissions.

   ### Check Controller Logs

   ```bash
   kubectl logs -n code-scanners-system deployment/code-scanners-controller-manager -f
   ```

   ## Next Steps

   After the FOSSA Team is created:
   1. Import code repositories into FOSSA (via `fossa analyze` or Quick Import)
   2. Assign imported projects to the team
   3. Add users to the team with appropriate roles
   ```

4. Update code-scanners/CLAUDE.md with FOSSA integration details:
   ```markdown
   ## FOSSA Integration

   The CodeScannerFossa controller creates FOSSA Teams (not Projects) for RBAC management.

   - **Creates**: FOSSA Teams via `/plugins/fossa/client.go`
   - **API**: `POST /api/teams` with Bearer token auth
   - **Credentials**: Read from secret `code-scanners` in CR namespace
   - **ConfigMap**: Contains team ID, name, and URL
   - **Status**: Tracks team details and conditions

   See `FOSSA_SETUP.md` and `FOSSA_RESEARCH.md` for details.
   ```

**Files to create:**
- `config/samples/secret-code-scanners.yaml`
- `code-scanners/FOSSA_SETUP.md`

**Files to modify:**
- `config/samples/maintainer-d.cncf.io_v1alpha1_codescannerfossa.yaml`
- `code-scanners/CLAUDE.md`

---

## File Change Summary

### Files to Create
- `config/samples/secret-code-scanners.yaml` - Secret template
- `code-scanners/FOSSA_SETUP.md` - Setup guide

### Files to Modify
- `internal/controller/codescannerfossa_controller.go` - Main implementation
- `internal/controller/constants.go` - Add constants
- `api/v1alpha1/codescannerfossa_types.go` - Update status with FossaTeamReference
- `internal/controller/codescannerfossa_controller_test.go` - Add tests
- `config/samples/maintainer-d.cncf.io_v1alpha1_codescannerfossa.yaml` - Update example
- `code-scanners/CLAUDE.md` - Documentation

### Files to Regenerate (via make manifests)
- `config/crd/bases/maintainer-d.cncf.io_codescannerfossas.yaml`
- `config/rbac/role.yaml`

### Files NOT Created (Reusing Existing)
- ❌ `pkg/fossa/client.go` - Use `/plugins/fossa/client.go` instead
- ❌ `pkg/fossa/types.go` - Types already in `/plugins/fossa/client.go`
- ❌ `pkg/fossa/errors.go` - Errors already in `/plugins/fossa/client.go`

## Dependencies

### Go Dependencies
- ✅ `github.com/cncf/maintainer-d/plugins/fossa` - Already in parent project
- ✅ `sigs.k8s.io/controller-runtime` - Already present
- ✅ Standard library - No new dependencies

### External Dependencies
- FOSSA Enterprise account
- FOSSA Full API Token
- FOSSA organization ID

## Success Criteria

- [x] Research complete: Teams vs Projects clarified
- [ ] Controller can read FOSSA credentials from secret
- [ ] Controller creates FOSSA teams via existing client
- [ ] ConfigMap includes FOSSA team details (ID, name, URL)
- [ ] CR status reflects FOSSA team state
- [ ] Idempotent: running multiple times doesn't create duplicates
- [ ] Clear error messages for common failures (missing secret, invalid token, API errors)
- [ ] Unit tests pass with >80% coverage
- [ ] Integration tests pass
- [ ] Documentation complete

## Risk Assessment

### Technical Risks

1. **API Token Permissions** - Token might not have team creation permissions
   - Mitigation: Document required permissions, provide clear error messages

2. **Rate Limiting** - FOSSA API might have rate limits
   - Mitigation: Use exponential backoff (controller-runtime default)

3. **Hardcoded Org ID** - Existing client has hardcoded "162" in one place (line 166)
   - Mitigation: Use org ID from secret, document this inconsistency

### Security Risks

1. **Secret Exposure** - FOSSA token in logs
   - Mitigation: Never log token value, only log "token found/not found"

2. **RBAC** - Controller needs secret read access
   - Mitigation: Limit to specific namespace, document in security notes

## Timeline Estimate

- Phase 1: Import existing client (1-2 hours)
- Phase 2: Secret management (2-4 hours)
- Phase 3: Update CRD (1-2 hours)
- Phase 4: Controller logic (8-12 hours)
- Phase 5: Testing (4-8 hours)
- Phase 6: Documentation (2-4 hours)

**Total**: 18-32 hours

## Open Questions

1. ✅ **Teams vs Projects** - RESOLVED: Create Teams (not Projects)
2. ✅ **Client Reuse** - RESOLVED: Reuse existing `/plugins/fossa/client.go`
3. ⏭️ **Cleanup Strategy** - Should we delete FOSSA teams when CR is deleted?
   - Recommendation: No, preserve teams (use finalizer but don't delete)
4. ⏭️ **Multi-Org Support** - Support multiple FOSSA organizations?
   - Recommendation: Phase 1 = single org, Phase 2 = per-CR override

## Related Documents

- `FOSSA_RESEARCH.md` - Detailed API research findings
- `PROMPT_FOSSA.md` - Original requirements (contains inaccuracy about "projects")
- `/plugins/fossa/client.go` - Existing FOSSA client implementation
- `/onboarding/server.go` - Example usage of FOSSA client

## Notes

- The original PROMPT_FOSSA.md asks to "create a project in Fossa" but this is technically incorrect
- FOSSA Projects are created by scanning code, not via direct API
- The correct approach (validated by existing codebase) is to create FOSSA Teams
- This plan aligns with the existing maintainer-d webhook server implementation
