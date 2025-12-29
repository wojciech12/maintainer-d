# KDP Workspace Operator - Implementation Plan

## Executive Summary

A Kubernetes operator that watches maintainer-d CRDs (Projects, Maintainers, Collaborators) on a service cluster and automatically provisions KDP workspaces with proper RBAC on a remote KDP/kcp instance. This operator enables self-service portal access for CNCF project maintainers and foundation staff.

**Key Objective**: After the operator executes, all maintainers and CNCF staff should be able to log in to the KDP service portal and access their respective organizations with appropriate permissions.

## Architecture Overview

### System Components

```
┌──────────────────────────────────────────────────────────────────┐
│                      Service Cluster                              │
│                                                                    │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  maintainer-d CRDs (Input)                                  │  │
│  │  • projects.maintainer-d.cncf.io                            │  │
│  │  • maintainers.maintainer-d.cncf.io                         │  │
│  │  • collaborators.maintainer-d.cncf.io                       │  │
│  └────────────────────────────────────────────────────────────┘  │
│                            ↓ watches                              │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  kdp-workspace-operator                                     │  │
│  │  • Project Controller                                       │  │
│  │  • RBAC Controller (TODO: clarify user management)          │  │
│  │  • kcp Client (github.com/kcp-dev/client-go)                │  │
│  └────────────────────────────────────────────────────────────┘  │
│                            ↓ creates                              │
└──────────────────────────────────────────────────────────────────┘
                             ↓
┌──────────────────────────────────────────────────────────────────┐
│                    KDP Control Plane (kcp)                        │
│                                                                    │
│  root/                                                            │
│  ├── kcp          (project workspace for kcp project)            │
│  ├── kubernetes   (project workspace for k8s project)            │
│  ├── etcd         (project workspace for etcd project)           │
│  ├── ...          (240+ CNCF project workspaces)                 │
│  └── foundation   (service provider workspace)                   │
│                                                                    │
│  Each workspace contains:                                         │
│  • ClusterRoleBindings for maintainers (admin access)            │
│  • ClusterRoleBindings for collaborators (read-only)             │
│  • ClusterRoleBindings for support team (admin access)           │
└──────────────────────────────────────────────────────────────────┘
```

### Data Flow

1. **Input**: maintainer-d sync job populates Project/Maintainer/Collaborator CRDs in service cluster
2. **Watch**: kdp-workspace-operator watches these CRDs
3. **Reconcile**: Operator creates/updates kcp Workspaces and ClusterRoleBindings
4. **Output**: Users can access their KDP organizations via the service portal

### Key Design Decisions

- **Flat Hierarchy**: All workspaces are direct children of `root` (v1 simplification)
- **Workspace Type**: Use `kdp-organization` for all workspaces (projects and foundation)
- **Workspace Naming**: Derived from `Project.metadata.name` for projects, hardcoded for foundation
- **RBAC Strategy**: ClusterRoleBindings within each workspace assign users to KDP roles
- **Framework**: Kubebuilder for operator scaffolding
- **kcp Client**: github.com/kcp-dev/client-go for workspace management

## Requirements

### Functional Requirements

1. **Workspace Creation**
   - Create one KDP workspace per CNCF project
   - Create foundation workspace for service providers
   - Workspace name derived from `Project.metadata.name` (must be DNS-1123 compliant)
   - Set workspace type to `kdp-organization` (path: `root`)

2. **Access Control**
   - Maintainers get admin/developer role in project workspace
   - Collaborators get member/viewer role in project workspace
   - Support team gets admin role in all project workspaces
   - Support team gets admin role in foundation workspace

3. **Lifecycle Management**
   - Create workspace when Project CRD is created
   - Update RBAC when maintainer/collaborator list changes
   - Delete workspace when Project CRD is deleted (with finalizer)
   - Handle workspace state transitions (Scheduling → Initializing → Ready)

4. **Status Reporting**
   - Track workspace creation status in Project.status
   - Report workspace URL for portal access
   - Expose conditions for observability
   - Emit Kubernetes events for major state changes

### Non-Functional Requirements

1. **Reliability**
   - Idempotent reconciliation (safe to retry)
   - Graceful handling of kcp unavailability
   - Finalizer-based cleanup to prevent orphaned resources

2. **Security**
   - Minimal kcp credentials (workspace creation only)
   - Audit trail via workspace annotations
   - Input validation to prevent injection attacks

3. **Observability**
   - Structured logging with context
   - Conditions reflect current state
   - Events for user-visible actions

## Custom Resource Definitions

### Option A: Reuse maintainer-d CRDs (Recommended for v1)

**Pros**:

- No new CRDs to manage
- Direct 1:1 mapping from Project → Workspace
- Simpler architecture

**Cons**:

- Status updates pollute maintainer-d CRDs
- Tight coupling with maintainer-d schema

**Implementation**:

- Watch `projects.maintainer-d.cncf.io`
- Update `Project.status.kdpWorkspace` with workspace details
- Watch `maintainers.maintainer-d.cncf.io` and `collaborators.maintainer-d.cncf.io` for RBAC

### Option B: New CRDs for workspace binding (Future Enhancement)

Create new CRDs to decouple from maintainer-d:

```go
// WorkspaceBinding represents the relationship between a Project and a kcp Workspace
type WorkspaceBinding struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   WorkspaceBindingSpec   `json:"spec,omitempty"`
    Status WorkspaceBindingStatus `json:"status,omitempty"`
}

type WorkspaceBindingSpec struct {
    // ProjectRef references the maintainer-d Project
    ProjectRef corev1.LocalObjectReference `json:"projectRef"`

    // WorkspaceName is the desired workspace name in kcp
    WorkspaceName string `json:"workspaceName"`

    // WorkspaceType is the kcp workspace type (default: kdp-organization)
    WorkspaceType string `json:"workspaceType,omitempty"`
}

type WorkspaceBindingStatus struct {
    // WorkspaceURL is the API endpoint for the workspace
    WorkspaceURL string `json:"workspaceURL,omitempty"`

    // Phase represents workspace state (Creating, Ready, Failed, Deleting)
    Phase string `json:"phase,omitempty"`

    // Conditions track detailed status
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // LastSyncTime is when we last synced with kcp
    LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}
```

**For v1, we'll use Option A to minimize complexity.**

## Controller Design

### Controller 1: Project Controller

**Responsibility**: Manage workspace lifecycle based on Project CRDs

#### Watches

- `projects.maintainer-d.cncf.io` (primary)
- `maintainers.maintainer-d.cncf.io` (secondary - trigger RBAC reconciliation)
- `collaborators.maintainer-d.cncf.io` (secondary - trigger RBAC reconciliation)

#### Reconciliation Logic

```go
func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 1. Fetch Project
    var project maintainersv1alpha1.Project
    if err := r.Get(ctx, req.NamespacedName, &project); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Handle deletion with finalizer
    if !project.DeletionTimestamp.IsZero() {
        return r.handleDeletion(ctx, &project)
    }

    // 3. Add finalizer if missing
    if !controllerutil.ContainsFinalizer(&project, WorkspaceFinalizerName) {
        controllerutil.AddFinalizer(&project, WorkspaceFinalizerName)
        if err := r.Update(ctx, &project); err != nil {
            return ctrl.Result{}, err
        }
    }

    // 4. Derive workspace name from Project.metadata.name
    workspaceName, err := r.deriveWorkspaceName(&project)
    if err != nil {
        r.Recorder.Event(&project, corev1.EventTypeWarning, "InvalidProject", err.Error())
        return ctrl.Result{}, err
    }

    // 5. Connect to kcp
    kcpClient, err := r.getKCPClient(ctx)
    if err != nil {
        meta.SetStatusCondition(&project.Status.Conditions, metav1.Condition{
            Type:    "Ready",
            Status:  metav1.ConditionFalse,
            Reason:  "KCPConnectionError",
            Message: err.Error(),
        })
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // 6. Get or create workspace
    workspace, err := r.reconcileWorkspace(ctx, kcpClient, &project, workspaceName)
    if err != nil {
        r.Recorder.Event(&project, corev1.EventTypeWarning, "WorkspaceReconcileError", err.Error())
        return ctrl.Result{RequeueAfter: 10 * time.Second}, err
    }

    // 7. Wait for workspace to be ready
    if !r.isWorkspaceReady(workspace) {
        log.Info("Workspace not ready yet", "workspace", workspaceName, "phase", workspace.Status.Phase)
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }

    // 8. Reconcile RBAC (ClusterRoleBindings)
    if err := r.reconcileRBAC(ctx, kcpClient, &project, workspace); err != nil {
        r.Recorder.Event(&project, corev1.EventTypeWarning, "RBACReconcileError", err.Error())
        return ctrl.Result{RequeueAfter: 10 * time.Second}, err
    }

    // 9. Update Project status
    project.Status.KDPWorkspace = &maintainersv1alpha1.KDPWorkspaceStatus{
        Name:  workspaceName,
        URL:   workspace.Spec.URL,
        Phase: string(workspace.Status.Phase),
    }

    meta.SetStatusCondition(&project.Status.Conditions, metav1.Condition{
        Type:    "Ready",
        Status:  metav1.ConditionTrue,
        Reason:  "WorkspaceReady",
        Message: fmt.Sprintf("Workspace %s is ready", workspaceName),
    })

    if err := r.Status().Update(ctx, &project); err != nil {
        return ctrl.Result{}, err
    }

    r.Recorder.Event(&project, corev1.EventTypeNormal, "WorkspaceReady",
        fmt.Sprintf("Workspace %s is ready at %s", workspaceName, workspace.Spec.URL))

    return ctrl.Result{}, nil
}
```

#### Workspace Naming Strategy

Workspace names are derived directly from `Project.metadata.name`, which must be DNS-1123 compliant (lowercase alphanumeric with dashes).

```go
func (r *ProjectReconciler) deriveWorkspaceName(project *maintainersv1alpha1.Project) (string, error) {
    // Use Project.metadata.name as workspace name
    // Validate it's DNS-1123 compliant (kcp requirement)
    if errs := validation.IsDNS1123Subdomain(project.Name); len(errs) > 0 {
        return "", fmt.Errorf("project name %s is not DNS-1123 compliant: %v", project.Name, errs)
    }

    // kcp workspace names must be lowercase
    workspaceName := strings.ToLower(project.Name)

    return workspaceName, nil
}
```

#### Finalizer Logic

```go
const WorkspaceFinalizerName = "workspace.kdp-workspace-operator.cncf.io/cleanup"

func (r *ProjectReconciler) handleDeletion(ctx context.Context, project *maintainersv1alpha1.Project) (ctrl.Result, error) {
    if !controllerutil.ContainsFinalizer(project, WorkspaceFinalizerName) {
        return ctrl.Result{}, nil
    }

    // Update phase to Deleting
    project.Status.KDPWorkspace.Phase = "Deleting"
    r.Status().Update(ctx, project)

    // Get workspace name
    workspaceName, _ := r.deriveWorkspaceName(project)

    // Connect to kcp
    kcpClient, err := r.getKCPClient(ctx)
    if err != nil {
        log.Error(err, "Failed to connect to kcp during deletion, will retry")
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // Delete workspace
    workspaces := kcpClient.Cluster(logicalcluster.NewPath("root")).TenancyV1alpha1().Workspaces()
    err = workspaces.Delete(ctx, workspaceName, metav1.DeleteOptions{})
    if err != nil && !apierrors.IsNotFound(err) {
        log.Error(err, "Failed to delete workspace", "workspace", workspaceName)
        return ctrl.Result{RequeueAfter: 10 * time.Second}, err
    }

    // Remove finalizer
    controllerutil.RemoveFinalizer(project, WorkspaceFinalizerName)
    if err := r.Update(ctx, project); err != nil {
        return ctrl.Result{}, err
    }

    r.Recorder.Event(project, corev1.EventTypeNormal, "WorkspaceDeleted",
        fmt.Sprintf("Deleted workspace %s from kcp", workspaceName))

    return ctrl.Result{}, nil
}
```

### RBAC Reconciliation

### User Identity Mapping

KDP uses **GitHub OIDC authentication**. Users are identified by their email with the `oidc:` prefix in ClusterRoleBinding subjects.

**Example**: `oidc:alice@kubernetes.io`

```go
func (r *ProjectReconciler) reconcileRBAC(ctx context.Context, kcpClient *kcptenancy.ClusterInterface, project *maintainersv1alpha1.Project, workspace *tenancyv1alpha1.Workspace) error {
    log := log.FromContext(ctx)

    // Get workspace-scoped client
    workspaceClient := kcpClient.Cluster(logicalcluster.NewPath("root:" + workspace.Name))

    // 1. Reconcile maintainers (admin/developer role)
    for _, maintainerRef := range project.Spec.MaintainerRefs {
        var maintainer maintainersv1alpha1.Maintainer
        if err := r.Get(ctx, types.NamespacedName{
            Name:      maintainerRef.Name,
            Namespace: project.Namespace,
        }, &maintainer); err != nil {
            log.Error(err, "Failed to get maintainer", "ref", maintainerRef.Name)
            continue
        }

        // Skip if no email provided
        if maintainer.Spec.PrimaryEmail == "" {
            log.Info("Skipping maintainer without email", "maintainer", maintainer.Name)
            continue
        }

        // Create ClusterRoleBinding for maintainer
        // TODO: Determine the correct role name ("Developer", "kdp-developer", etc.)
        crb := &rbacv1.ClusterRoleBinding{
            ObjectMeta: metav1.ObjectMeta{
                Name: fmt.Sprintf("maintainer-%s", sanitizeName(maintainer.Name)),
                Annotations: map[string]string{
                    "managed-by": "kdp-workspace-operator",
                    "project":    project.Name,
                    "email":      maintainer.Spec.PrimaryEmail,
                },
            },
            RoleRef: rbacv1.RoleRef{
                APIGroup: "rbac.authorization.k8s.io",
                Kind:     "ClusterRole",
                Name:     "Developer", // KDP meta-role (TODO: confirm exact name)
            },
            Subjects: []rbacv1.Subject{
                {
                    Kind: "User",
                    Name: fmt.Sprintf("oidc:%s", maintainer.Spec.PrimaryEmail),
                },
            },
        }

        // Create or update
        if err := r.createOrUpdateClusterRoleBinding(ctx, workspaceClient, crb); err != nil {
            return err
        }
    }

    // 2. Reconcile collaborators (member/viewer role)
    for _, collaboratorRef := range project.Spec.CollaboratorRefs {
        var collaborator maintainersv1alpha1.Collaborator
        if err := r.Get(ctx, types.NamespacedName{
            Name:      collaboratorRef.Name,
            Namespace: project.Namespace,
        }, &collaborator); err != nil {
            log.Error(err, "Failed to get collaborator", "ref", collaboratorRef.Name)
            continue
        }

        // Skip if no email provided
        if collaborator.Spec.PrimaryEmail == "" {
            log.Info("Skipping collaborator without email", "collaborator", collaborator.Name)
            continue
        }

        crb := &rbacv1.ClusterRoleBinding{
            ObjectMeta: metav1.ObjectMeta{
                Name: fmt.Sprintf("collaborator-%s", sanitizeName(collaborator.Name)),
                Annotations: map[string]string{
                    "managed-by": "kdp-workspace-operator",
                    "project":    project.Name,
                    "email":      collaborator.Spec.PrimaryEmail,
                },
            },
            RoleRef: rbacv1.RoleRef{
                APIGroup: "rbac.authorization.k8s.io",
                Kind:     "ClusterRole",
                Name:     "Member", // KDP meta-role (TODO: confirm exact name)
            },
            Subjects: []rbacv1.Subject{
                {
                    Kind: "User",
                    Name: fmt.Sprintf("oidc:%s", collaborator.Spec.PrimaryEmail),
                },
            },
        }

        if err := r.createOrUpdateClusterRoleBinding(ctx, workspaceClient, crb); err != nil {
            return err
        }
    }

    // 3. Reconcile support team members (admin role)
    // TODO: CLARIFY - Where is support team list stored?
    supportTeamMembers, err := r.getSupportTeamMembers(ctx)
    if err != nil {
        return err
    }

    for _, member := range supportTeamMembers {
        crb := &rbacv1.ClusterRoleBinding{
            ObjectMeta: metav1.ObjectMeta{
                Name: fmt.Sprintf("support-%s", sanitizeName(member.Name)),
                Annotations: map[string]string{
                    "managed-by": "kdp-workspace-operator",
                    "role":       "support",
                },
            },
            RoleRef: rbacv1.RoleRef{
                APIGroup: "rbac.authorization.k8s.io",
                Kind:     "ClusterRole",
                Name:     "Developer", // Admin access (TODO: confirm role)
            },
            Subjects: []rbacv1.Subject{
                {
                    Kind: "User",
                    Name: fmt.Sprintf("oidc:%s", member.Email),
                },
            },
        }

        if err := r.createOrUpdateClusterRoleBinding(ctx, workspaceClient, crb); err != nil {
            return err
        }
    }

    return nil
}

// sanitizeName converts a name to be suitable for Kubernetes resource naming
func sanitizeName(name string) string {
    // Convert to lowercase and replace invalid characters with hyphens
    result := strings.ToLower(name)
    result = strings.ReplaceAll(result, "@", "-at-")
    result = strings.ReplaceAll(result, ".", "-")
    result = strings.ReplaceAll(result, "_", "-")
    return result
}
```

### Controller 2: Foundation Workspace Controller (Optional)

**Responsibility**: Create and manage the `foundation` workspace for service providers

**TODO: CLARIFY** - Should we create foundation workspace, or is it pre-created?

If we create it:

```go
type FoundationWorkspaceReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func (r *FoundationWorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // This runs once on startup or can be triggered by a ConfigMap change

    // 1. Connect to kcp
    kcpClient, err := r.getKCPClient(ctx)
    if err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    // 2. Create foundation workspace if not exists
    workspace := &tenancyv1alpha1.Workspace{
        ObjectMeta: metav1.ObjectMeta{
            Name: "foundation",
            Annotations: map[string]string{
                "managed-by": "kdp-workspace-operator",
                "purpose":    "service-provider",
            },
        },
        Spec: tenancyv1alpha1.WorkspaceSpec{
            Type: &tenancyv1alpha1.WorkspaceTypeReference{
                Name: "kdp-organization",
                Path: "root",
            },
        },
    }

    workspaces := kcpClient.Cluster(logicalcluster.NewPath("root")).TenancyV1alpha1().Workspaces()
    _, err = workspaces.Create(ctx, workspace, metav1.CreateOptions{})
    if err != nil && !apierrors.IsAlreadyExists(err) {
        return ctrl.Result{RequeueAfter: 10 * time.Second}, err
    }

    // 3. Assign foundation team members as admins
    // TODO: Get foundation team member list

    return ctrl.Result{}, nil
}
```

## kcp Client Integration

### Dependencies

```go
// go.mod
require (
    github.com/kcp-dev/client-go v0.28.3 // or latest
    github.com/kcp-dev/sdk v0.28.3
    github.com/kcp-dev/logicalcluster/v3 v3.0.8
    k8s.io/client-go v0.28.0
    sigs.k8s.io/controller-runtime v0.16.0
)
```

### kcp Client Wrapper

```go
// internal/kcp/client.go
package kcp

import (
    "context"
    "fmt"

    kcptenancy "github.com/kcp-dev/sdk/client/clientset/versioned"
    "github.com/kcp-dev/logicalcluster/v3"
    "k8s.io/client-go/rest"
    "k8s.io/client-go/tools/clientcmd"
    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
    tenancyClient *kcptenancy.Clientset
    config        *rest.Config
}

func NewClient(ctx context.Context, k8sClient client.Client, configMapName, secretName, namespace string) (*Client, error) {
    // 1. Load ConfigMap
    var configMap corev1.ConfigMap
    if err := k8sClient.Get(ctx, client.ObjectKey{
        Name:      configMapName,
        Namespace: namespace,
    }, &configMap); err != nil {
        return nil, fmt.Errorf("failed to get ConfigMap: %w", err)
    }

    // 2. Load Secret
    var secret corev1.Secret
    if err := k8sClient.Get(ctx, client.ObjectKey{
        Name:      secretName,
        Namespace: namespace,
    }, &secret); err != nil {
        return nil, fmt.Errorf("failed to get Secret: %w", err)
    }

    // 3. Parse kubeconfig
    kubeconfigData, ok := secret.Data["kubeconfig"]
    if !ok {
        return nil, fmt.Errorf("kubeconfig not found in secret")
    }

    restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
    if err != nil {
        return nil, fmt.Errorf("failed to parse kubeconfig: %w", err)
    }

    // 4. Create kcp tenancy client
    tenancyClient, err := kcptenancy.NewForConfig(restConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create kcp client: %w", err)
    }

    return &Client{
        tenancyClient: tenancyClient,
        config:        restConfig,
    }, nil
}

// Cluster returns a cluster-scoped client for the given workspace path
func (c *Client) Cluster(path logicalcluster.Path) *kcptenancy.Cluster {
    return c.tenancyClient.Cluster(path)
}
```

### Workspace Operations

```go
// internal/kcp/workspace.go
package kcp

import (
    "context"
    "fmt"
    "time"

    tenancyv1alpha1 "github.com/kcp-dev/sdk/apis/tenancy/v1alpha1"
    "github.com/kcp-dev/logicalcluster/v3"
    corev1alpha1 "github.com/kcp-dev/sdk/apis/core/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/api/errors"
)

type WorkspaceManager struct {
    client *Client
}

func NewWorkspaceManager(client *Client) *WorkspaceManager {
    return &WorkspaceManager{client: client}
}

func (m *WorkspaceManager) CreateWorkspace(ctx context.Context, name string, annotations map[string]string) (*tenancyv1alpha1.Workspace, error) {
    workspace := &tenancyv1alpha1.Workspace{
        ObjectMeta: metav1.ObjectMeta{
            Name:        name,
            Annotations: annotations,
        },
        Spec: tenancyv1alpha1.WorkspaceSpec{
            Type: &tenancyv1alpha1.WorkspaceTypeReference{
                Name: "kdp-organization",
                Path: "root",
            },
        },
    }

    workspaces := m.client.Cluster(logicalcluster.NewPath("root")).TenancyV1alpha1().Workspaces()
    created, err := workspaces.Create(ctx, workspace, metav1.CreateOptions{})
    if err != nil {
        if errors.IsAlreadyExists(err) {
            // Workspace already exists, get it
            return workspaces.Get(ctx, name, metav1.GetOptions{})
        }
        return nil, fmt.Errorf("failed to create workspace: %w", err)
    }

    return created, nil
}

func (m *WorkspaceManager) GetWorkspace(ctx context.Context, name string) (*tenancyv1alpha1.Workspace, error) {
    workspaces := m.client.Cluster(logicalcluster.NewPath("root")).TenancyV1alpha1().Workspaces()
    return workspaces.Get(ctx, name, metav1.GetOptions{})
}

func (m *WorkspaceManager) DeleteWorkspace(ctx context.Context, name string) error {
    workspaces := m.client.Cluster(logicalcluster.NewPath("root")).TenancyV1alpha1().Workspaces()
    err := workspaces.Delete(ctx, name, metav1.DeleteOptions{})
    if errors.IsNotFound(err) {
        return nil // Already deleted
    }
    return err
}

func (m *WorkspaceManager) IsWorkspaceReady(workspace *tenancyv1alpha1.Workspace) bool {
    return workspace.Status.Phase == corev1alpha1.LogicalClusterPhaseReady
}

func (m *WorkspaceManager) WaitForWorkspaceReady(ctx context.Context, name string, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)

    for time.Now().Before(deadline) {
        workspace, err := m.GetWorkspace(ctx, name)
        if err != nil {
            return err
        }

        if m.IsWorkspaceReady(workspace) {
            return nil
        }

        time.Sleep(2 * time.Second)
    }

    return fmt.Errorf("timeout waiting for workspace %s to be ready", name)
}
```

## Configuration Management

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kdp-workspace-config
  namespace: kdp-workspace-system
data:
  kcp-url: "https://services.cncf.io:8443"
  kcp-workspace-path: "root"
  workspace-type: "kdp-organization"
  support-team-members: |
    - name: "Alice Support"
      email: "alice@cncf.io"
    - name: "Bob Support"
      email: "bob@cncf.io"
```

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: kdp-workspace-kubeconfig
  namespace: kdp-workspace-system
type: Opaque
data:
  kubeconfig: <base64-encoded-kubeconfig>
```

The kubeconfig should point to the KDP/kcp instance with credentials that have permission to:

- Create workspaces in root
- Create ClusterRoleBindings within workspaces
- Get/List/Watch workspaces
- Delete workspaces

## Directory Structure

```
kdp-workspaces/
├── api/
│   └── v1alpha1/                    # (Optional) Future CRDs
│       ├── workspacebinding_types.go
│       └── groupversion_info.go
├── internal/
│   ├── controller/
│   │   ├── project_controller.go    # Main workspace reconciliation
│   │   ├── project_controller_test.go
│   │   ├── foundation_controller.go # Foundation workspace (optional)
│   │   └── suite_test.go
│   ├── kcp/
│   │   ├── client.go                # kcp client wrapper
│   │   ├── workspace.go             # Workspace operations
│   │   ├── rbac.go                  # RBAC management
│   │   ├── config.go                # Configuration loading
│   │   └── *_test.go
│   └── util/
│       ├── naming.go                # Workspace naming logic
│       └── naming_test.go
├── cmd/
│   └── main.go                      # Operator entrypoint
├── config/
│   ├── crd/                         # (If creating new CRDs)
│   ├── rbac/                        # RBAC for operator
│   ├── manager/                     # Deployment manifests
│   ├── samples/                     # Example configs
│   │   ├── kcp_config.yaml
│   │   └── kcp_secret_template.yaml
│   └── default/                     # Kustomization
├── test/
│   ├── e2e/
│   └── integration/
├── go.mod
├── go.sum
├── Makefile
├── PROJECT                          # Kubebuilder metadata
├── Dockerfile
├── CLAUDE.md                        # Operator usage guide
├── CLAUDE_PLAN.md                   # This file
├── CLAUDE_KDP_WORKSPACE_PROMPT.md   # Original requirements
└── README.md
```

## Implementation Stages

### Stage 1: Minimal Workspace Creation (Week 1)

**Goal**: Create workspaces in kcp for each Project CRD

**Deliverables**:

- ✅ Kubebuilder project scaffolding
- ✅ Project controller that watches `projects.maintainer-d.cncf.io`
- ✅ kcp client wrapper with connection management
- ✅ Workspace creation logic
- ✅ Basic status reporting (workspace name, URL, phase)
- ✅ ConfigMap + Secret based kcp connection
- ✅ Manual testing with sample Project

**What Works**:

- Create Project → workspace appears in kcp
- Workspace named from `Project.metadata.name` (validated as DNS-1123 compliant)
- Status shows workspace details
- Idempotent: creating same Project twice doesn't error

**What Doesn't Work Yet**:

- ❌ No RBAC (ClusterRoleBindings)
- ❌ No deletion support
- ❌ No finalizers
- ❌ Maintainer/Collaborator changes don't trigger updates

**Success Criteria**:

```bash
# Apply a Project CRD
kubectl apply -f - <<EOF
apiVersion: maintainer-d.cncf.io/v1alpha1
kind: Project
metadata:
  name: kubernetes
  namespace: maintainerd
spec:
  displayName: "Kubernetes"
  maturity: "Graduated"
EOF

# Workspace should be created in kcp
export KUBECONFIG=temp/KDP_KUBECONFIG
kubectl get workspaces
# NAME         TYPE              PHASE   URL
# kubernetes   kdp-organization  Ready   https://...

# Project status should be updated
kubectl get projects kubernetes -o jsonpath='{.status.kdpWorkspace}'
# {"name":"kubernetes","url":"https://...","phase":"Ready"}
```

### Stage 2: RBAC Integration (Week 2)

**Goal**: Add ClusterRoleBinding creation for maintainers and collaborators

**Deliverables**:

- ✅ Watch Maintainer and Collaborator CRDs (for owned-by)
- ✅ RBAC reconciliation logic
- ✅ ClusterRoleBinding creation in workspace context
- ✅ Support team member assignment
- ✅ RBAC status tracking

**What Works**:

- Everything from Stage 1
- Maintainers get Developer role in their project workspace
- Collaborators get Member role
- Support team gets Developer role in all workspaces
- Updating maintainer list triggers RBAC reconciliation

**Authentication** ✅ **RESOLVED**:

- Users authenticate via GitHub OIDC
- ClusterRoleBinding subjects use format: `oidc:email@domain.com`
- Uses `Maintainer.spec.primaryEmail` and `Collaborator.spec.primaryEmail`

**Open Questions** (TODO: CLARIFY):

- What are the exact KDP role names? (Developer, Member, or custom?)

**Success Criteria**:

```bash
# After workspace creation, check RBAC
kubectl --context kdp-workspace-kubernetes get clusterrolebindings
# Should show bindings for all maintainers and collaborators
```

### Stage 3: Full Lifecycle Management (Week 3)

**Goal**: Add deletion support with finalizers

**Deliverables**:

- ✅ Finalizer implementation
- ✅ Workspace deletion logic
- ✅ ClusterRoleBinding cleanup
- ✅ Status updates during deletion
- ✅ Graceful handling of kcp unavailability

**What Works**:

- Everything from Stage 2
- Delete Project → workspace deleted from kcp
- Finalizer prevents premature deletion
- RBAC cleaned up before workspace deletion

**Success Criteria**:

```bash
# Delete a project
kubectl delete project kubernetes

# Workspace should be deleted from kcp
kubectl get workspaces kubernetes
# Error: workspaces.tenancy.kcp.io "kubernetes" not found

# No orphaned resources
```

### Stage 4: Enhanced Observability (Week 4)

**Goal**: Rich status conditions, events, and logging

**Deliverables**:

- ✅ Condition support (WorkspaceReady, RBACReady, etc.)
- ✅ Kubernetes events for major state changes
- ✅ Improved error messages
- ✅ Structured logging with context

**What Works**:

- Everything from Stage 3
- Detailed conditions show workspace state
- Events visible in `kubectl describe project`
- Clear error messages for troubleshooting

**Success Criteria**:

```bash
kubectl describe project kubernetes
# Should show detailed conditions and events
```

### Stage 5: Foundation Workspace (Week 5 - Optional)

**Goal**: Create and manage foundation workspace for service providers

**TODO: CLARIFY** - Should we create it or is it pre-created?

**Deliverables**:

- ✅ Foundation workspace controller
- ✅ Foundation team member RBAC
- ✅ Configuration for foundation team

### Stage 6: Production Readiness (Week 6)

**Goal**: Testing, documentation, deployment readiness

**Deliverables**:

- ✅ Comprehensive unit tests (>80% coverage)
- ✅ Integration tests with mock kcp
- ✅ E2E tests with real kcp instance
- ✅ Complete documentation
- ✅ Dockerfile and deployment manifests
- ✅ CI/CD pipeline
- ✅ Metrics and monitoring (optional)

## Testing Strategy

### Unit Testing

```go
// internal/controller/project_controller_test.go
func TestProjectReconciler_Reconcile(t *testing.T) {
    tests := []struct {
        name           string
        project        *maintainersv1alpha1.Project
        existingWS     *tenancyv1alpha1.Workspace
        expectedPhase  string
        expectedError  bool
    }{
        {
            name: "create new workspace",
            project: &maintainersv1alpha1.Project{
                ObjectMeta: metav1.ObjectMeta{Name: "test-project"},
                Spec: maintainersv1alpha1.ProjectSpec{
                    DisplayName: "Test Project",
                },
            },
            existingWS:    nil,
            expectedPhase: "Ready",
            expectedError: false,
        },
        {
            name: "workspace already exists",
            project: &maintainersv1alpha1.Project{
                ObjectMeta: metav1.ObjectMeta{Name: "test-project"},
                Spec: maintainersv1alpha1.ProjectSpec{
                    DisplayName: "Test Project",
                },
            },
            existingWS: &tenancyv1alpha1.Workspace{
                ObjectMeta: metav1.ObjectMeta{Name: "test"},
                Status: tenancyv1alpha1.WorkspaceStatus{
                    Phase: corev1alpha1.LogicalClusterPhaseReady,
                },
            },
            expectedPhase: "Ready",
            expectedError: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup fake clients
            // Run reconciliation
            // Assert results
        })
    }
}
```

### Integration Testing

Test against a mock kcp server or use kcp's test framework.

### E2E Testing

```bash
# test/e2e/e2e_test.go
# Deploy operator to test cluster
# Create Project CRDs
# Verify workspace creation in real kcp
# Verify RBAC
# Test deletion
# Cleanup
```

## RBAC Requirements

### Local Cluster RBAC (Operator)

```go
// +kubebuilder:rbac:groups=maintainer-d.cncf.io,resources=projects,verbs=get;list;watch
// +kubebuilder:rbac:groups=maintainer-d.cncf.io,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=maintainer-d.cncf.io,resources=projects/finalizers,verbs=update
// +kubebuilder:rbac:groups=maintainer-d.cncf.io,resources=maintainers,verbs=get;list;watch
// +kubebuilder:rbac:groups=maintainer-d.cncf.io,resources=collaborators,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
```

### Remote kcp RBAC (via kubeconfig)

The kubeconfig in the Secret must have permissions to:

- Create/Get/List/Watch/Delete `workspaces.tenancy.kcp.io` in root workspace
- Create/Get/List/Watch/Delete `clusterrolebindings.rbac.authorization.k8s.io` in all workspaces
- Get workspace status

## Open Questions & TODOs

### Critical (Must Resolve Before Stage 2)

1. **~~User Identity Mapping~~** ✅ **RESOLVED**
   - [x] KDP uses GitHub OIDC authentication
   - [x] Users are identified by email with `oidc:` prefix in ClusterRoleBinding subjects
   - [x] Example: `oidc:alice@kubernetes.io`
   - [x] No User CRDs needed, just ClusterRoleBindings

2. **KDP Role Names**
   - [ ] What are the exact ClusterRole names in KDP? (Developer, Member, or custom?)
   - [ ] Are these roles pre-created in KDP or do we create them?
   - [ ] What permissions does each role have?

3. **Support Team Configuration**
   - [ ] Where is the support team member list stored? (ConfigMap, CRD, hardcoded?)
   - [ ] How is it updated? (Manual edit, sync from another system?)

4. **~~GitHub Org Slug~~** ✅ **RESOLVED**
   - [x] Workspace name is derived from `Project.metadata.name`
   - [x] Must be DNS-1123 compliant (validated by operator)

### Important (Can Defer to Stage 5)

5. **Foundation Workspace**
   - [ ] Should the operator create the foundation workspace?
   - [ ] Or is it manually pre-created by admins?
   - [ ] Where is the foundation team member list?

6. **Workspace Labels/Annotations**
   - [ ] Should we add any custom labels to workspaces?
   - [ ] What metadata should we track? (project maturity, etc.)

### Nice-to-Have (Post-v1)

7. **Updates and Reconciliation**
   - [ ] How often should we reconcile RBAC?
   - [ ] Do we need to detect and remove stale ClusterRoleBindings?

8. **Multi-tenancy**
   - [ ] Do we support multiple KDP instances?
   - [ ] Do we need namespace isolation for Projects?

## Error Scenarios and Handling

| Scenario                             | Detection                       | Action                                           | Status          | Requeue                  |
| ------------------------------------ | ------------------------------- | ------------------------------------------------ | --------------- | ------------------------ |
| kcp unreachable                      | Client creation fails           | Set Ready=False, reason: KCPConnectionError      | Ready=False     | Yes, 30s backoff         |
| Invalid kubeconfig                   | Config parsing fails            | Set Ready=False, reason: InvalidConfiguration    | Ready=False     | Yes, when Secret changes |
| Workspace already exists (owned)     | Get succeeds, annotations match | Update status, reconcile RBAC                    | Ready=True      | No                       |
| Workspace already exists (not owned) | Get succeeds, no annotations    | Set Ready=False, reason: WorkspaceConflict       | Ready=False     | No (manual fix)          |
| Workspace creation fails             | Create returns error            | Set Ready=False, reason: WorkspaceCreationFailed | Ready=False     | Yes, 10s backoff         |
| Workspace stuck in Initializing      | Phase != Ready after timeout    | Set Ready=False, reason: WorkspaceNotReady       | Ready=False     | Yes, 5s                  |
| RBAC creation fails                  | ClusterRoleBinding create fails | Set RBACReady=False                              | RBACReady=False | Yes, 10s                 |
| Maintainer CRD not found             | Get Maintainer fails            | Log warning, skip binding                        | Ready=True      | No                       |
| ConfigMap not found                  | Get ConfigMap fails             | Set Ready=False, reason: ConfigurationMissing    | Ready=False     | Yes, 30s                 |
| Secret not found                     | Get Secret fails                | Set Ready=False, reason: ConfigurationMissing    | Ready=False     | Yes, 30s                 |
| Project validation fails             | Invalid workspace name          | Set Ready=False, reason: InvalidSpec             | Ready=False     | No (requires spec fix)   |
| Workspace deletion fails             | Delete returns error            | Keep finalizer, log error                        | Phase=Deleting  | Yes, 10s backoff         |

## Security Considerations

1. **Minimal Credentials**
   - kcp kubeconfig should only have workspace creation permissions
   - Use service account with RBAC, not admin credentials

2. **Audit Trail**
   - Add annotations to all created workspaces: `managed-by`, `project-name`, `created-at`
   - Add annotations to ClusterRoleBindings: `managed-by`, `project`, `role-type`

3. **Input Validation**
   - Validate workspace names against DNS-1123 subdomain rules
   - Sanitize all user input before passing to kcp API
   - Prevent injection attacks via kubebuilder validation markers

4. **Secret Management**
   - Rotate kcp credentials regularly
   - Consider using external secret management (Vault, ESO)
   - Limit Secret access to operator service account only

## Metrics and Observability (Future)

### Prometheus Metrics

```go
// Metrics to add in future:
- workspace_reconcile_duration_seconds (histogram)
- workspace_reconcile_errors_total (counter)
- workspace_creation_duration_seconds (histogram)
- workspace_creation_failures_total (counter)
- rbac_reconcile_duration_seconds (histogram)
- rbac_reconcile_errors_total (counter)
- kcp_connection_errors_total (counter)
- workspaces_total (gauge, by phase)
- clusterrolebindings_total (gauge, by role)
```

### Structured Logging

```go
log.Info("Reconciling project",
    "project", project.Name,
    "workspace", workspaceName,
    "maintainerCount", len(project.Spec.MaintainerRefs),
    "collaboratorCount", len(project.Spec.CollaboratorRefs))

log.Error(err, "Failed to create workspace",
    "project", project.Name,
    "workspace", workspaceName,
    "error", err)
```

### Kubernetes Events

```go
r.Recorder.Event(project, corev1.EventTypeNormal, "WorkspaceCreating",
    fmt.Sprintf("Creating workspace %s in kcp", workspaceName))

r.Recorder.Event(project, corev1.EventTypeNormal, "WorkspaceReady",
    fmt.Sprintf("Workspace %s is ready at %s", workspaceName, workspace.Spec.URL))

r.Recorder.Event(project, corev1.EventTypeWarning, "RBACFailed",
    fmt.Sprintf("Failed to create ClusterRoleBinding for maintainer %s: %v", maintainer.Name, err))
```

## Development Commands

### Setup

```bash
# Initialize kubebuilder project (don't do this if already exists)
mkdir -p kdp-workspaces
cd kdp-workspaces
kubebuilder init --domain cncf.io --repo github.com/cncf/maintainer-d/kdp-workspaces

# Add kcp dependencies
go get github.com/kcp-dev/client-go@v0.28.3
go get github.com/kcp-dev/sdk@v0.28.3
go get github.com/kcp-dev/logicalcluster/v3@v3.0.8
go mod tidy
```

### Local Development

```bash
# Install CRDs (maintainer-d CRDs should already exist)
make install

# Create configuration
kubectl create namespace kdp-workspace-system
kubectl apply -f config/samples/kcp_config.yaml
kubectl apply -f config/samples/kcp_secret.yaml

# Run operator locally
make run

# In another terminal, verify
kubectl get projects -w
```

### Testing

```bash
# Run unit tests
make test

# Run with coverage
make test-coverage

# Run E2E tests (requires kcp instance)
make test-e2e
```

### Deployment

```bash
# Build and push image
make docker-build docker-push IMG=ghcr.io/cncf/kdp-workspace-operator:latest

# Deploy to cluster
make deploy IMG=ghcr.io/cncf/kdp-workspace-operator:latest

# Verify deployment
kubectl get pods -n kdp-workspace-system
kubectl logs -n kdp-workspace-system deployment/kdp-workspace-controller-manager -f

# Undeploy
make undeploy
```

## Next Steps

### Immediate (Before Starting Implementation)

1. **Answer Remaining Open Questions**
   - ✅ ~~User identity mapping~~ - RESOLVED: GitHub OIDC with `oidc:email` format
   - ✅ ~~Workspace naming~~ - RESOLVED: `Project.metadata.name`
   - ⏳ Get exact KDP role names (Developer, Member, or custom?)
   - ⏳ Determine support team configuration source

2. **Setup Development Environment**
   - Access to KDP/kcp instance for testing
   - Service account credentials for operator
   - Sample maintainer-d CRDs for testing

3. **Create Design Doc Review**
   - Share plan with stakeholders
   - Get approval on architecture
   - Confirm implementation stages

### Week 1: Start Implementation

1. Follow Stage 1 implementation plan
2. Get basic workspace creation working
3. Manual testing with sample Projects

## References

### Documentation

- [kcp Documentation](https://docs.kcp.io/) - Main kcp concepts
- [kcp Workspaces](https://docs.kcp.io/kcp/main/concepts/workspaces/)
- [kcp client-go](https://github.com/kcp-dev/client-go) - Multi-cluster-aware clients
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [KDP Documentation](https://docs.kubermatic.com/developer-platform/)
- [KDP RBAC](https://docs.kubermatic.com/developer-platform/platform-users/rbac/)

### Code References

- [kdp-admin operator](../kdp-simple-service/kdp-admin/) - Reference implementation
- [greeter-operator](../kdp-simple-service/greeter-operator/) - Kubebuilder example
- [maintainer-d CRDs](apis/maintainers/v1alpha1/types.go) - Input CRDs

### Related Files

- [CLAUDE.md](CLAUDE.md) - General context about maintainer-d
- [CLAUDE_KDP_WORKSPACE_PROMPT.md](CLAUDE_KDP_WORKSPACE_PROMPT.md) - Original requirements
- [CLAUDE_20251223_kdp_organiztion_op_design_doc.md](CLAUDE_20251223_kdp_organiztion_op_design_doc.md) - Design document
- [kcp workspace hierarchy](../docs/kcp-workspace-hierarchy.md) - kcp workspace concepts

## Appendix A: Example Project CRD

```yaml
apiVersion: maintainer-d.cncf.io/v1alpha1
kind: Project
metadata:
  name: kubernetes
  namespace: maintainerd
spec:
  displayName: "Kubernetes"
  maturity: "Graduated"
  maintainerRefs:
    - name: maintainer-alice
    - name: maintainer-bob
  collaboratorRefs:
    - name: collaborator-charlie
status:
  kdpWorkspace:
    name: "kubernetes"
    url: "https://services.cncf.io/clusters/root:kubernetes"
    phase: "Ready"
  conditions:
    - type: Ready
      status: "True"
      reason: WorkspaceReady
      message: "Workspace kubernetes is ready"
      lastTransitionTime: "2025-12-29T12:00:00Z"
    - type: RBACReady
      status: "True"
      reason: RBACReconciled
      message: "ClusterRoleBindings created for 2 maintainers and 1 collaborator"
      lastTransitionTime: "2025-12-29T12:00:05Z"
```

## Appendix B: Example Workspace in kcp

```yaml
apiVersion: tenancy.kcp.io/v1alpha1
kind: Workspace
metadata:
  name: kubernetes
  annotations:
    managed-by: kdp-workspace-operator
    project-name: kubernetes
    project-namespace: maintainerd
    created-at: "2025-12-29T12:00:00Z"
  labels:
    project-maturity: graduated
spec:
  type:
    name: kdp-organization
    path: root
  cluster: <generated-by-kcp>
  URL: https://services.cncf.io/clusters/root:kubernetes
status:
  phase: Ready
  conditions:
    - type: WorkspaceScheduled
      status: "True"
    - type: WorkspaceInitialized
      status: "True"
    - type: APIBindingsInitialized
      status: "True"
```

## Appendix C: Example ClusterRoleBinding

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: maintainer-alice
  annotations:
    managed-by: kdp-workspace-operator
    project: kubernetes
    role-type: maintainer
    email: alice@kubernetes.io
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: Developer # KDP meta-role (TODO: Confirm exact name)
subjects:
  - kind: User
    name: oidc:alice@kubernetes.io # GitHub OIDC format
```

---

**End of Plan**

This plan provides a comprehensive roadmap for implementing the kdp-workspace operator. The implementation should proceed in stages, with each stage building on the previous one. Key open questions (marked as TODO) must be resolved before proceeding with Stage 2 (RBAC integration).
