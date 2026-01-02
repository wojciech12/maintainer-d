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

	kcpclientset "github.com/kcp-dev/kcp/sdk/client/clientset/versioned"
	kcpcluster "github.com/kcp-dev/kcp/sdk/client/clientset/versioned/cluster"
	"github.com/kcp-dev/logicalcluster/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client wraps the kcp tenancy client and provides workspace operations
type Client struct {
	clusterClient *kcpcluster.ClusterClientset
	workspacePath string
}

// Config holds the configuration needed to connect to kcp
type Config struct {
	KCPURL        string
	WorkspacePath string
	Kubeconfig    []byte
}

// NewClient creates a new kcp client from the provided configuration
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if len(cfg.Kubeconfig) == 0 {
		return nil, fmt.Errorf("kubeconfig cannot be empty")
	}

	// Create rest.Config from kubeconfig
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(cfg.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create rest config from kubeconfig: %w", err)
	}

	// Override the host if KCPURL is provided
	if cfg.KCPURL != "" {
		restConfig.Host = cfg.KCPURL
	}

	// Create kcp cluster client
	clusterClient, err := kcpcluster.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kcp cluster client: %w", err)
	}

	workspacePath := cfg.WorkspacePath
	if workspacePath == "" {
		workspacePath = "root"
	}

	return &Client{
		clusterClient: clusterClient,
		workspacePath: workspacePath,
	}, nil
}

// LoadConfigFromCluster loads kcp configuration from ConfigMap and Secret
func LoadConfigFromCluster(ctx context.Context, k8sClient client.Client, configMapName, secretName, namespace string) (*Config, error) {
	// Load ConfigMap
	configMap := &corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      configMapName,
		Namespace: namespace,
	}, configMap); err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w", namespace, configMapName, err)
	}

	// Load Secret
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get Secret %s/%s: %w", namespace, secretName, err)
	}

	// Extract configuration
	kcpURL := configMap.Data["kcp-url"]
	workspacePath := configMap.Data["kcp-workspace-path"]
	kubeconfig := secret.Data["kubeconfig"]

	if len(kubeconfig) == 0 {
		return nil, fmt.Errorf("kubeconfig not found in secret %s/%s", namespace, secretName)
	}

	return &Config{
		KCPURL:        kcpURL,
		WorkspacePath: workspacePath,
		Kubeconfig:    kubeconfig,
	}, nil
}

// GetClusterClient returns a cluster-scoped client for the configured workspace path
func (c *Client) GetClusterClient() kcpclientset.Interface {
	return c.clusterClient.Cluster(logicalcluster.NewPath(c.workspacePath))
}
