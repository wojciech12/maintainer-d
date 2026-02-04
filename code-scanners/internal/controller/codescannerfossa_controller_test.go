/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	maintainerdcncfiov1alpha1 "github.com/cncf/maintainer-d/code-scanners/api/v1alpha1"
	"github.com/cncf/maintainer-d/plugins/fossa"
)

// mockFossaClient implements FossaClient for testing
type mockFossaClient struct {
	teams      map[string]*fossa.Team
	createErr  error
	fetchErr   error
	nextTeamID int

	// User invitation fields
	users                 []fossa.User
	pendingInvitations    map[string]bool
	sendInvitationErr     error
	fetchUsersErr         error
	pendingInvitationErr  error
}

func newMockFossaClient() *mockFossaClient {
	return &mockFossaClient{
		teams:              make(map[string]*fossa.Team),
		nextTeamID:         1,
		pendingInvitations: make(map[string]bool),
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

func (m *mockFossaClient) SendUserInvitation(email string) error {
	if m.sendInvitationErr != nil {
		return m.sendInvitationErr
	}
	m.pendingInvitations[email] = true
	return nil
}

func (m *mockFossaClient) HasPendingInvitation(email string) (bool, error) {
	if m.pendingInvitationErr != nil {
		return false, m.pendingInvitationErr
	}
	return m.pendingInvitations[email], nil
}

func (m *mockFossaClient) FetchUsers() ([]fossa.User, error) {
	if m.fetchUsersErr != nil {
		return nil, m.fetchUsersErr
	}
	return m.users, nil
}

// TestReconcile_CreatesFossaTeam tests successful team creation and ConfigMap generation
func TestReconcile_CreatesFossaTeam(t *testing.T) {
	ctx := context.Background()
	const resourceName = "test-project"
	const namespace = "code-scanners"

	// Create CR
	fossaCR := &maintainerdcncfiov1alpha1.CodeScannerFossa{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
		Spec: maintainerdcncfiov1alpha1.CodeScannerFossaSpec{
			ProjectName: resourceName,
		},
	}

	// Create secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			SecretKeyFossaToken: []byte("test-token"),
			SecretKeyFossaOrgID: []byte("162"),
		},
	}

	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}
	defer func() {
		if err := k8sClient.Delete(ctx, secret); err != nil {
			t.Logf("Failed to delete secret: %v", err)
		}
	}()

	if err := k8sClient.Create(ctx, fossaCR); err != nil {
		t.Fatalf("Failed to create CodeScannerFossa: %v", err)
	}
	defer func() {
		if err := k8sClient.Delete(ctx, fossaCR); err != nil {
			t.Logf("Failed to delete CodeScannerFossa: %v", err)
		}
	}()

	// Setup mock FOSSA client
	mockClient := newMockFossaClient()
	reconciler := &CodeScannerFossaReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
		FossaClientFactory: func(token string) FossaClient {
			if token != "test-token" {
				t.Errorf("Expected token 'test-token', got %q", token)
			}
			return mockClient
		},
	}

	// Reconcile
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		},
	}
	result, err := reconciler.Reconcile(ctx, req)

	// Assertions
	if err != nil {
		t.Errorf("Reconcile failed: %v", err)
	}
	if result.Requeue || result.RequeueAfter > 0 {
		t.Error("Unexpected requeue")
	}

	// Verify team created in mock
	if len(mockClient.teams) != 1 {
		t.Errorf("Expected 1 team, got %d", len(mockClient.teams))
	}
	if _, exists := mockClient.teams[resourceName]; !exists {
		t.Error("Team 'test-project' not created")
	}

	// Verify status updated
	updated := &maintainerdcncfiov1alpha1.CodeScannerFossa{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: namespace}, updated); err != nil {
		t.Fatalf("Failed to get updated CR: %v", err)
	}
	if updated.Status.FossaTeam == nil {
		t.Error("Status.FossaTeam not set")
	} else {
		if updated.Status.FossaTeam.Name != resourceName {
			t.Errorf("Expected team name %q, got %q", resourceName, updated.Status.FossaTeam.Name)
		}
		if updated.Status.FossaTeam.ID != 1 {
			t.Errorf("Expected team ID 1, got %d", updated.Status.FossaTeam.ID)
		}
		if updated.Status.FossaTeam.OrganizationID != 162 {
			t.Errorf("Expected organization ID 162, got %d", updated.Status.FossaTeam.OrganizationID)
		}
	}

	// Verify ConfigMap created
	cm := &corev1.ConfigMap{}
	cmKey := types.NamespacedName{Name: resourceName, Namespace: namespace}
	if err := k8sClient.Get(ctx, cmKey, cm); err != nil {
		t.Fatalf("ConfigMap not created: %v", err)
	}
	if cm.Data[ConfigMapKeyCodeScanner] != ScannerTypeFossa {
		t.Errorf("ConfigMap CodeScanner incorrect: %q", cm.Data[ConfigMapKeyCodeScanner])
	}
	if cm.Data[ConfigMapKeyProjectName] != resourceName {
		t.Errorf("ConfigMap ProjectName incorrect: %q", cm.Data[ConfigMapKeyProjectName])
	}
	if cm.Data["FossaTeamName"] != resourceName {
		t.Errorf("ConfigMap FossaTeamName incorrect: %q", cm.Data["FossaTeamName"])
	}
	if cm.Data["FossaTeamID"] != "1" {
		t.Errorf("ConfigMap FossaTeamID incorrect: %q", cm.Data["FossaTeamID"])
	}

	// Verify annotation set
	if updated.Annotations == nil {
		t.Error("Annotations not set")
	} else {
		expectedRef := fmt.Sprintf("%s/%s", namespace, resourceName)
		if updated.Annotations[AnnotationConfigMapRef] != expectedRef {
			t.Errorf("Expected annotation %q, got %q", expectedRef, updated.Annotations[AnnotationConfigMapRef])
		}
	}

	// Verify condition
	if len(updated.Status.Conditions) == 0 {
		t.Error("No conditions set")
	} else {
		foundReady := false
		foundConfigMap := false
		for _, cond := range updated.Status.Conditions {
			if cond.Type == ConditionTypeFossaTeamReady {
				foundReady = true
				if cond.Status != metav1.ConditionTrue {
					t.Errorf("Expected FossaTeamReady=True, got %s", cond.Status)
				}
				if cond.Reason != ReasonTeamCreated {
					t.Errorf("Expected reason %q, got %q", ReasonTeamCreated, cond.Reason)
				}
			}
			if cond.Type == ConditionTypeConfigMapReady {
				foundConfigMap = true
				if cond.Status != metav1.ConditionTrue {
					t.Errorf("Expected ConfigMapReady=True, got %s", cond.Status)
				}
			}
		}
		if !foundReady {
			t.Error("FossaTeamReady condition not set")
		}
		if !foundConfigMap {
			t.Error("ConfigMapReady condition not set")
		}
	}
}

// TestReconcile_MissingSecret tests that reconcile handles missing secret gracefully
func TestReconcile_MissingSecret(t *testing.T) {
	ctx := context.Background()
	const resourceName = "test-missing-secret"
	const namespace = "code-scanners"

	// Create CR without creating secret
	fossaCR := &maintainerdcncfiov1alpha1.CodeScannerFossa{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
		Spec: maintainerdcncfiov1alpha1.CodeScannerFossaSpec{
			ProjectName: resourceName,
		},
	}

	if err := k8sClient.Create(ctx, fossaCR); err != nil {
		t.Fatalf("Failed to create CodeScannerFossa: %v", err)
	}
	defer func() {
		if err := k8sClient.Delete(ctx, fossaCR); err != nil {
			t.Logf("Failed to delete CodeScannerFossa: %v", err)
		}
	}()

	// Setup reconciler
	mockClient := newMockFossaClient()
	reconciler := &CodeScannerFossaReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
		FossaClientFactory: func(token string) FossaClient {
			return mockClient
		},
	}

	// Reconcile
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		},
	}
	result, err := reconciler.Reconcile(ctx, req)

	// Assertions - should not error but should not requeue
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result.Requeue || result.RequeueAfter > 0 {
		t.Error("Should not requeue when secret is missing (requires manual intervention)")
	}

	// Verify team NOT created
	if len(mockClient.teams) != 0 {
		t.Errorf("Expected no teams created, got %d", len(mockClient.teams))
	}

	// Verify status updated with error condition
	updated := &maintainerdcncfiov1alpha1.CodeScannerFossa{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: namespace}, updated); err != nil {
		t.Fatalf("Failed to get updated CR: %v", err)
	}

	foundCondition := false
	for _, cond := range updated.Status.Conditions {
		if cond.Type == ConditionTypeFossaTeamReady {
			foundCondition = true
			if cond.Status != metav1.ConditionFalse {
				t.Errorf("Expected FossaTeamReady=False, got %s", cond.Status)
			}
			if cond.Reason != ReasonCredentialsNotFound {
				t.Errorf("Expected reason %q, got %q", ReasonCredentialsNotFound, cond.Reason)
			}
		}
	}
	if !foundCondition {
		t.Error("FossaTeamReady condition not set")
	}
}

// TestReconcile_MissingSecretKeys tests handling of secret with missing keys
func TestReconcile_MissingSecretKeys(t *testing.T) {
	ctx := context.Background()
	const resourceName = "test-missing-keys"
	const namespace = "code-scanners"

	// Create CR
	fossaCR := &maintainerdcncfiov1alpha1.CodeScannerFossa{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
		Spec: maintainerdcncfiov1alpha1.CodeScannerFossaSpec{
			ProjectName: resourceName,
		},
	}

	// Create secret with missing token
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			SecretKeyFossaOrgID: []byte("162"),
			// Missing SecretKeyFossaToken
		},
	}

	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}
	defer func() {
		if err := k8sClient.Delete(ctx, secret); err != nil {
			t.Logf("Failed to delete secret: %v", err)
		}
	}()

	if err := k8sClient.Create(ctx, fossaCR); err != nil {
		t.Fatalf("Failed to create CodeScannerFossa: %v", err)
	}
	defer func() {
		if err := k8sClient.Delete(ctx, fossaCR); err != nil {
			t.Logf("Failed to delete CodeScannerFossa: %v", err)
		}
	}()

	// Setup reconciler
	mockClient := newMockFossaClient()
	reconciler := &CodeScannerFossaReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
		FossaClientFactory: func(token string) FossaClient {
			return mockClient
		},
	}

	// Reconcile
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		},
	}
	result, err := reconciler.Reconcile(ctx, req)

	// Assertions
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result.Requeue || result.RequeueAfter > 0 {
		t.Error("Should not requeue when secret keys are missing")
	}

	// Verify condition set
	updated := &maintainerdcncfiov1alpha1.CodeScannerFossa{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: namespace}, updated); err != nil {
		t.Fatalf("Failed to get updated CR: %v", err)
	}

	foundCondition := false
	for _, cond := range updated.Status.Conditions {
		if cond.Type == ConditionTypeFossaTeamReady {
			foundCondition = true
			if cond.Status != metav1.ConditionFalse {
				t.Errorf("Expected FossaTeamReady=False, got %s", cond.Status)
			}
			if cond.Reason != ReasonCredentialsNotFound {
				t.Errorf("Expected reason %q, got %q", ReasonCredentialsNotFound, cond.Reason)
			}
		}
	}
	if !foundCondition {
		t.Error("FossaTeamReady condition not set")
	}
}

// TestReconcile_Idempotency tests that multiple reconciles don't create duplicate teams
func TestReconcile_Idempotency(t *testing.T) {
	ctx := context.Background()
	const resourceName = "test-idempotent"
	const namespace = "code-scanners"

	// Create CR and secret
	fossaCR := &maintainerdcncfiov1alpha1.CodeScannerFossa{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
		Spec: maintainerdcncfiov1alpha1.CodeScannerFossaSpec{
			ProjectName: resourceName,
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			SecretKeyFossaToken: []byte("test-token"),
			SecretKeyFossaOrgID: []byte("162"),
		},
	}

	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}
	defer func() {
		if err := k8sClient.Delete(ctx, secret); err != nil {
			t.Logf("Failed to delete secret: %v", err)
		}
	}()

	if err := k8sClient.Create(ctx, fossaCR); err != nil {
		t.Fatalf("Failed to create CodeScannerFossa: %v", err)
	}
	defer func() {
		if err := k8sClient.Delete(ctx, fossaCR); err != nil {
			t.Logf("Failed to delete CodeScannerFossa: %v", err)
		}
	}()

	// Setup mock FOSSA client
	mockClient := newMockFossaClient()
	reconciler := &CodeScannerFossaReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
		FossaClientFactory: func(token string) FossaClient {
			return mockClient
		},
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		},
	}

	// First reconcile - creates team
	result1, err1 := reconciler.Reconcile(ctx, req)
	if err1 != nil {
		t.Fatalf("First reconcile failed: %v", err1)
	}
	if result1.Requeue || result1.RequeueAfter > 0 {
		t.Error("First reconcile should not requeue")
	}

	if len(mockClient.teams) != 1 {
		t.Fatalf("Expected 1 team after first reconcile, got %d", len(mockClient.teams))
	}
	firstTeam := mockClient.teams[resourceName]
	if firstTeam == nil {
		t.Fatal("Team not created after first reconcile")
	}
	firstTeamID := firstTeam.ID

	// Second reconcile - should be idempotent
	result2, err2 := reconciler.Reconcile(ctx, req)
	if err2 != nil {
		t.Fatalf("Second reconcile failed: %v", err2)
	}
	if result2.Requeue || result2.RequeueAfter > 0 {
		t.Error("Second reconcile should not requeue")
	}

	// Verify still only one team with same ID
	if len(mockClient.teams) != 1 {
		t.Errorf("Expected 1 team after second reconcile, got %d", len(mockClient.teams))
	}
	secondTeam := mockClient.teams[resourceName]
	if secondTeam == nil {
		t.Fatal("Team disappeared after second reconcile")
	}
	if secondTeam.ID != firstTeamID {
		t.Errorf("Team ID changed from %d to %d", firstTeamID, secondTeam.ID)
	}

	// Third reconcile - ensure it's still idempotent
	result3, err3 := reconciler.Reconcile(ctx, req)
	if err3 != nil {
		t.Fatalf("Third reconcile failed: %v", err3)
	}
	if result3.Requeue || result3.RequeueAfter > 0 {
		t.Error("Third reconcile should not requeue")
	}

	if len(mockClient.teams) != 1 {
		t.Errorf("Expected 1 team after third reconcile, got %d", len(mockClient.teams))
	}
}

// TestReconcile_APIError tests handling of FOSSA API errors
func TestReconcile_APIError(t *testing.T) {
	ctx := context.Background()
	const resourceName = "test-api-error"
	const namespace = "code-scanners"

	// Create CR and secret
	fossaCR := &maintainerdcncfiov1alpha1.CodeScannerFossa{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
		Spec: maintainerdcncfiov1alpha1.CodeScannerFossaSpec{
			ProjectName: resourceName,
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			SecretKeyFossaToken: []byte("test-token"),
			SecretKeyFossaOrgID: []byte("162"),
		},
	}

	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}
	defer func() {
		if err := k8sClient.Delete(ctx, secret); err != nil {
			t.Logf("Failed to delete secret: %v", err)
		}
	}()

	if err := k8sClient.Create(ctx, fossaCR); err != nil {
		t.Fatalf("Failed to create CodeScannerFossa: %v", err)
	}
	defer func() {
		if err := k8sClient.Delete(ctx, fossaCR); err != nil {
			t.Logf("Failed to delete CodeScannerFossa: %v", err)
		}
	}()

	// Setup mock FOSSA client with error
	mockClient := newMockFossaClient()
	mockClient.fetchErr = fmt.Errorf("team not found")
	mockClient.createErr = fmt.Errorf("API rate limit exceeded")

	reconciler := &CodeScannerFossaReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
		FossaClientFactory: func(token string) FossaClient {
			return mockClient
		},
	}

	// Reconcile
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		},
	}
	result, err := reconciler.Reconcile(ctx, req)

	// Assertions - should error and requeue for transient errors
	if err == nil {
		t.Error("Expected error for API failure")
	}
	if result.RequeueAfter != time.Minute {
		t.Errorf("Expected requeue after 1 minute, got %v", result.RequeueAfter)
	}

	// Verify condition set
	updated := &maintainerdcncfiov1alpha1.CodeScannerFossa{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: namespace}, updated); err != nil {
		t.Fatalf("Failed to get updated CR: %v", err)
	}

	foundCondition := false
	for _, cond := range updated.Status.Conditions {
		if cond.Type == ConditionTypeFossaTeamReady {
			foundCondition = true
			if cond.Status != metav1.ConditionFalse {
				t.Errorf("Expected FossaTeamReady=False, got %s", cond.Status)
			}
			if cond.Reason != ReasonFossaAPIError {
				t.Errorf("Expected reason %q, got %q", ReasonFossaAPIError, cond.Reason)
			}
		}
	}
	if !foundCondition {
		t.Error("FossaTeamReady condition not set")
	}
}

// TestReconcile_ConfigMapUpdate tests that ConfigMap is updated when team details change
func TestReconcile_ConfigMapUpdate(t *testing.T) {
	ctx := context.Background()
	const resourceName = "test-configmap-update"
	const namespace = "code-scanners"

	// Create CR and secret
	fossaCR := &maintainerdcncfiov1alpha1.CodeScannerFossa{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
		Spec: maintainerdcncfiov1alpha1.CodeScannerFossaSpec{
			ProjectName: resourceName,
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			SecretKeyFossaToken: []byte("test-token"),
			SecretKeyFossaOrgID: []byte("162"),
		},
	}

	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}
	defer func() {
		if err := k8sClient.Delete(ctx, secret); err != nil {
			t.Logf("Failed to delete secret: %v", err)
		}
	}()

	if err := k8sClient.Create(ctx, fossaCR); err != nil {
		t.Fatalf("Failed to create CodeScannerFossa: %v", err)
	}
	defer func() {
		if err := k8sClient.Delete(ctx, fossaCR); err != nil {
			t.Logf("Failed to delete CodeScannerFossa: %v", err)
		}
	}()

	// Create initial ConfigMap with old data
	oldConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
		Data: map[string]string{
			ConfigMapKeyCodeScanner: ScannerTypeFossa,
			ConfigMapKeyProjectName: resourceName,
			"FossaTeamID":           "999",
			"FossaTeamName":         "old-name",
		},
	}
	if err := k8sClient.Create(ctx, oldConfigMap); err != nil {
		t.Fatalf("Failed to create old ConfigMap: %v", err)
	}

	// Setup mock FOSSA client
	mockClient := newMockFossaClient()
	reconciler := &CodeScannerFossaReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
		FossaClientFactory: func(token string) FossaClient {
			return mockClient
		},
	}

	// Reconcile
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		},
	}
	result, err := reconciler.Reconcile(ctx, req)

	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if result.Requeue || result.RequeueAfter > 0 {
		t.Error("Unexpected requeue")
	}

	// Verify ConfigMap updated with new team data
	cm := &corev1.ConfigMap{}
	cmKey := types.NamespacedName{Name: resourceName, Namespace: namespace}
	if err := k8sClient.Get(ctx, cmKey, cm); err != nil {
		t.Fatalf("Failed to get ConfigMap: %v", err)
	}

	if cm.Data["FossaTeamID"] != "1" {
		t.Errorf("ConfigMap FossaTeamID not updated: expected %q, got %q", "1", cm.Data["FossaTeamID"])
	}
	if cm.Data["FossaTeamName"] != resourceName {
		t.Errorf("ConfigMap FossaTeamName not updated: expected %q, got %q", resourceName, cm.Data["FossaTeamName"])
	}
}

// TestReconcile_ResourceNotFound tests handling of deleted resources
func TestReconcile_ResourceNotFound(t *testing.T) {
	ctx := context.Background()
	const resourceName = "non-existent"
	const namespace = "code-scanners"

	reconciler := &CodeScannerFossaReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
	}

	// Reconcile non-existent resource
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		},
	}
	result, err := reconciler.Reconcile(ctx, req)

	// Should return no error and no requeue
	if err != nil {
		t.Errorf("Expected no error for not found resource, got: %v", err)
	}
	if result.Requeue || result.RequeueAfter > 0 {
		t.Error("Should not requeue for not found resource")
	}
}
