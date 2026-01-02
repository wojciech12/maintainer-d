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

package kcp

import (
	"context"
	"fmt"
	"time"

	corev1alpha1 "github.com/kcp-dev/kcp/sdk/apis/core/v1alpha1"
	tenancyv1alpha1 "github.com/kcp-dev/kcp/sdk/apis/tenancy/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// DefaultWorkspaceType is the workspace type to create
	DefaultWorkspaceType = "kdp-organization"

	// WorkspaceReadyTimeout is the maximum time to wait for workspace to be ready
	WorkspaceReadyTimeout = 2 * time.Minute

	// WorkspaceCheckInterval is the interval to check workspace readiness
	WorkspaceCheckInterval = 5 * time.Second
)

// WorkspaceInfo contains information about a workspace
type WorkspaceInfo struct {
	Name  string
	URL   string
	Phase string
	Ready bool
}

// CreateWorkspace creates a new workspace in kcp with the specified name
func (c *Client) CreateWorkspace(ctx context.Context, projectName, projectNamespace, projectResourceName, workspaceType string) (*WorkspaceInfo, error) {
	if projectName == "" {
		return nil, fmt.Errorf("project name cannot be empty")
	}

	if workspaceType == "" {
		workspaceType = DefaultWorkspaceType
	}

	workspace := &tenancyv1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: projectName,
			Annotations: map[string]string{
				"managed-by":        "kdp-workspace-operator",
				"project-namespace": projectNamespace,
				"project-name":      projectResourceName,
			},
		},
		Spec: tenancyv1alpha1.WorkspaceSpec{
			Type: &tenancyv1alpha1.WorkspaceTypeReference{
				Name: tenancyv1alpha1.WorkspaceTypeName(workspaceType),
				Path: "root",
			},
		},
	}

	clusterClient := c.GetClusterClient()
	createdWs, err := clusterClient.TenancyV1alpha1().Workspaces().Create(ctx, workspace, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace %s: %w", projectName, err)
	}

	return &WorkspaceInfo{
		Name:  createdWs.Name,
		URL:   createdWs.Spec.URL,
		Phase: string(createdWs.Status.Phase),
		Ready: createdWs.Status.Phase == corev1alpha1.LogicalClusterPhaseReady,
	}, nil
}

// GetWorkspace retrieves workspace information by name
func (c *Client) GetWorkspace(ctx context.Context, workspaceName string) (*WorkspaceInfo, error) {
	if workspaceName == "" {
		return nil, fmt.Errorf("workspace name cannot be empty")
	}

	clusterClient := c.GetClusterClient()
	ws, err := clusterClient.TenancyV1alpha1().Workspaces().Get(ctx, workspaceName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get workspace %s: %w", workspaceName, err)
	}

	return &WorkspaceInfo{
		Name:  ws.Name,
		URL:   ws.Spec.URL,
		Phase: string(ws.Status.Phase),
		Ready: ws.Status.Phase == corev1alpha1.LogicalClusterPhaseReady,
	}, nil
}

// DeleteWorkspace deletes a workspace by name
func (c *Client) DeleteWorkspace(ctx context.Context, workspaceName string) error {
	if workspaceName == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}

	clusterClient := c.GetClusterClient()
	err := clusterClient.TenancyV1alpha1().Workspaces().Delete(ctx, workspaceName, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Workspace already deleted, not an error
			return nil
		}
		return fmt.Errorf("failed to delete workspace %s: %w", workspaceName, err)
	}

	return nil
}

// WaitForWorkspaceReady waits for the workspace to reach Ready phase
func (c *Client) WaitForWorkspaceReady(ctx context.Context, workspaceName string) (*WorkspaceInfo, error) {
	if workspaceName == "" {
		return nil, fmt.Errorf("workspace name cannot be empty")
	}

	var lastInfo *WorkspaceInfo

	err := wait.PollUntilContextTimeout(ctx, WorkspaceCheckInterval, WorkspaceReadyTimeout, true, func(ctx context.Context) (bool, error) {
		info, err := c.GetWorkspace(ctx, workspaceName)
		if err != nil {
			return false, err
		}

		if info == nil {
			return false, fmt.Errorf("workspace %s not found", workspaceName)
		}

		lastInfo = info
		return info.Ready, nil
	})

	if err != nil {
		if lastInfo != nil {
			return lastInfo, fmt.Errorf("workspace %s not ready after timeout, current phase: %s: %w", workspaceName, lastInfo.Phase, err)
		}
		return nil, fmt.Errorf("failed to wait for workspace %s to be ready: %w", workspaceName, err)
	}

	return lastInfo, nil
}

// WorkspaceExists checks if a workspace exists
func (c *Client) WorkspaceExists(ctx context.Context, workspaceName string) (bool, error) {
	info, err := c.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return false, err
	}
	return info != nil, nil
}
