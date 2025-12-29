/*
Copyright 2025.

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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cncf/maintainer-d/kdp-workspaces/internal/kcp"
)

const (
	// WorkspaceReadyCondition is the condition type for workspace readiness
	WorkspaceReadyCondition = "WorkspaceReady"

	// AnnotationWorkspaceName stores the workspace name in annotations
	AnnotationWorkspaceName = "kdp-workspaces.cncf.io/workspace-name"
	// AnnotationWorkspaceURL stores the workspace URL in annotations
	AnnotationWorkspaceURL = "kdp-workspaces.cncf.io/workspace-url"
	// AnnotationWorkspacePhase stores the workspace phase in annotations
	AnnotationWorkspacePhase = "kdp-workspaces.cncf.io/workspace-phase"
)

// ProjectReconciler reconciles maintainer-d Project objects and creates kcp workspaces
type ProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// KCP configuration
	KCPConfigMapName      string
	KCPConfigMapNamespace string
	KCPSecretName         string
	KCPSecretNamespace    string
	WorkspaceType         string
}

// +kubebuilder:rbac:groups=maintainer-d.cncf.io,resources=projects,verbs=get;list;watch
// +kubebuilder:rbac:groups=maintainer-d.cncf.io,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile handles the reconciliation of Project resources
func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Define a minimal Project type for unmarshaling
	project := &metav1.PartialObjectMetadata{}
	project.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "maintainer-d.cncf.io",
		Version: "v1alpha1",
		Kind:    "Project",
	})

	// Fetch the Project resource using unstructured client
	if err := r.Get(ctx, req.NamespacedName, project); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Project resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Project resource")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling Project", "project", project.Name, "namespace", project.Namespace)

	// Load kcp configuration from ConfigMap and Secret
	kcpConfig, err := kcp.LoadConfigFromCluster(
		ctx,
		r.Client,
		r.KCPConfigMapName,
		r.KCPSecretName,
		r.KCPConfigMapNamespace,
	)
	if err != nil {
		logger.Error(err, "Failed to load kcp configuration")
		if updateErr := r.updateCondition(ctx, project, metav1.Condition{
			Type:    WorkspaceReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  "ConfigurationError",
			Message: fmt.Sprintf("Failed to load kcp configuration: %v", err),
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to update condition")
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}

	// Create kcp client
	kcpClient, err := kcp.NewClient(kcpConfig)
	if err != nil {
		logger.Error(err, "Failed to create kcp client")
		if updateErr := r.updateCondition(ctx, project, metav1.Condition{
			Type:    WorkspaceReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  "KCPConnectionError",
			Message: fmt.Sprintf("Failed to create kcp client: %v", err),
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to update condition")
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}

	// Derive workspace name from Project name (must be DNS-1123 compliant)
	workspaceName := strings.ToLower(project.Name)

	// Check if workspace already exists
	workspaceInfo, err := kcpClient.GetWorkspace(ctx, workspaceName)
	if err != nil {
		logger.Error(err, "Failed to get workspace from kcp", "workspace", workspaceName)
		if updateErr := r.updateCondition(ctx, project, metav1.Condition{
			Type:    WorkspaceReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  "WorkspaceCheckError",
			Message: fmt.Sprintf("Failed to check workspace: %v", err),
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to update condition")
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
	}

	if workspaceInfo != nil {
		// Workspace already exists
		logger.Info("Workspace already exists", "workspace", workspaceName, "phase", workspaceInfo.Phase, "ready", workspaceInfo.Ready)

		// Update status with workspace details
		if err := r.updateWorkspaceStatus(ctx, project, workspaceInfo); err != nil {
			logger.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}

		// Requeue if workspace is not ready yet
		if !workspaceInfo.Ready {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		return ctrl.Result{}, nil
	}

	// Workspace doesn't exist, create it
	logger.Info("Creating workspace in kcp", "workspace", workspaceName)

	// Update condition to indicate creation in progress
	if err := r.updateCondition(ctx, project, metav1.Condition{
		Type:    WorkspaceReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  "Creating",
		Message: "Creating workspace in kcp",
	}); err != nil {
		logger.Error(err, "Failed to update condition to Creating")
	}

	workspaceType := r.WorkspaceType
	if workspaceType == "" {
		workspaceType = kcp.DefaultWorkspaceType
	}

	createdInfo, err := kcpClient.CreateWorkspace(ctx, workspaceName, project.Namespace, project.Name, workspaceType)
	if err != nil {
		logger.Error(err, "Failed to create workspace", "workspace", workspaceName)
		if updateErr := r.updateCondition(ctx, project, metav1.Condition{
			Type:    WorkspaceReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  "CreationFailed",
			Message: fmt.Sprintf("Workspace creation failed: %v", err),
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to update condition")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	logger.Info("Workspace created successfully", "workspace", workspaceName, "phase", createdInfo.Phase)

	// Wait for workspace to be ready (with timeout)
	logger.Info("Waiting for workspace to be ready", "workspace", workspaceName)
	readyInfo, err := kcpClient.WaitForWorkspaceReady(ctx, workspaceName)
	if err != nil {
		logger.Error(err, "Workspace not ready", "workspace", workspaceName)
		// Update status with current info
		if updateErr := r.updateWorkspaceStatus(ctx, project, createdInfo); updateErr != nil {
			logger.Error(updateErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	logger.Info("Workspace is ready", "workspace", workspaceName)

	// Update status with workspace details
	if err := r.updateWorkspaceStatus(ctx, project, readyInfo); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateWorkspaceStatus updates the Project with workspace information
func (r *ProjectReconciler) updateWorkspaceStatus(ctx context.Context, project *metav1.PartialObjectMetadata, info *kcp.WorkspaceInfo) error {
	// Update annotations with workspace details
	if project.Annotations == nil {
		project.Annotations = make(map[string]string)
	}
	project.Annotations[AnnotationWorkspaceName] = info.Name
	project.Annotations[AnnotationWorkspaceURL] = info.URL
	project.Annotations[AnnotationWorkspacePhase] = info.Phase

	if err := r.Update(ctx, project); err != nil {
		return fmt.Errorf("failed to update annotations: %w", err)
	}

	// Update condition based on workspace readiness
	condition := metav1.Condition{
		Type:               WorkspaceReadyCondition,
		ObservedGeneration: project.Generation,
		LastTransitionTime: metav1.Now(),
	}

	if info.Ready {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "WorkspaceReady"
		condition.Message = fmt.Sprintf("Workspace %s is ready at %s", info.Name, info.URL)
	} else {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "WorkspaceNotReady"
		condition.Message = fmt.Sprintf("Workspace %s is in phase %s", info.Name, info.Phase)
	}

	return r.updateCondition(ctx, project, condition)
}

// updateCondition updates a condition in the Project status
func (r *ProjectReconciler) updateCondition(ctx context.Context, project *metav1.PartialObjectMetadata, condition metav1.Condition) error {
	// Get the current project using unstructured to access status
	currentProject := &unstructured.Unstructured{}
	currentProject.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "maintainer-d.cncf.io",
		Version: "v1alpha1",
		Kind:    "Project",
	})

	if err := r.Get(ctx, types.NamespacedName{Name: project.Name, Namespace: project.Namespace}, currentProject); err != nil {
		return fmt.Errorf("failed to get current project: %w", err)
	}

	// Extract conditions from status if they exist
	var conditions []metav1.Condition
	if statusMap, ok := currentProject.Object["status"].(map[string]interface{}); ok {
		if conditionsRaw, ok := statusMap["conditions"].([]interface{}); ok {
			for _, condRaw := range conditionsRaw {
				if condMap, ok := condRaw.(map[string]interface{}); ok {
					cond := metav1.Condition{}
					if t, ok := condMap["type"].(string); ok {
						cond.Type = t
					}
					if s, ok := condMap["status"].(string); ok {
						cond.Status = metav1.ConditionStatus(s)
					}
					if r, ok := condMap["reason"].(string); ok {
						cond.Reason = r
					}
					if m, ok := condMap["message"].(string); ok {
						cond.Message = m
					}
					conditions = append(conditions, cond)
				}
			}
		}
	}

	// Set or update the condition
	meta.SetStatusCondition(&conditions, condition)

	// Update the status
	statusMap := make(map[string]interface{})
	if status, ok := currentProject.Object["status"].(map[string]interface{}); ok {
		statusMap = status
	}
	statusMap["conditions"] = conditions
	if err := unstructured.SetNestedField(currentProject.Object, statusMap, "status"); err != nil {
		return fmt.Errorf("failed to set status: %w", err)
	}

	if err := r.Status().Update(ctx, currentProject); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create a metadata-only object for watching
	project := &metav1.PartialObjectMetadata{}
	project.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "maintainer-d.cncf.io",
		Version: "v1alpha1",
		Kind:    "Project",
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(project).
		Complete(r)
}
