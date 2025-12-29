# kdp-workspaces
// TODO(user): Add simple overview of use/purpose

## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.
- Admin access to a KDP (kcp) instance for service account setup

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/kdp-workspaces:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/kdp-workspaces:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## KDP Service Account Setup

The kdp-workspaces operator requires a service account in your KDP instance to manage workspaces. This section describes how to create and configure the service account for production deployments.

### Why Use a Service Account?

Service accounts provide:
- **Token-based authentication** compatible with containerized environments
- **Least privilege access** with scoped RBAC permissions
- **Long-lived credentials** suitable for automated systems
- **No dependency on user authentication flows** (OIDC, exec plugins, etc.)

### Step 1: Create ServiceAccount in KDP

Connect to your KDP instance with admin privileges:

```bash
# Set your KDP kubeconfig
export KUBECONFIG=/path/to/your/kdp-admin-kubeconfig

# Verify connection
kubectl get workspaces

# Set context to root workspace (or your target workspace)
kubectl kcp workspace use root
```

Create a namespace and service account for the operator:

```bash
# Create namespace for operator service account
kubectl create namespace kdp-workspaces-sa

# Create the ServiceAccount
kubectl create serviceaccount kdp-workspaces-operator -n kdp-workspaces-sa
```

### Step 2: Configure RBAC Permissions

Create a ClusterRole with workspace management permissions:

```bash
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kdp-workspaces-manager
rules:
  # Workspace management permissions
  - apiGroups: ["tenancy.kcp.io"]
    resources: ["workspaces"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
  - apiGroups: ["tenancy.kcp.io"]
    resources: ["workspaces/status"]
    verbs: ["get", "list", "watch"]
  # Workspace type permissions
  - apiGroups: ["tenancy.kcp.io"]
    resources: ["workspacetypes"]
    verbs: ["get", "list"]
EOF
```

Bind the ClusterRole to the ServiceAccount:

```bash
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kdp-workspaces-operator-workspaces-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kdp-workspaces-manager
subjects:
  - kind: ServiceAccount
    name: kdp-workspaces-operator
    namespace: kdp-workspaces-sa
EOF
```

### Step 3: Create Service Account Token

For Kubernetes 1.24+, create a Secret to obtain a long-lived token:

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: kdp-workspaces-operator-token
  namespace: kdp-workspaces-sa
  annotations:
    kubernetes.io/service-account.name: kdp-workspaces-operator
type: kubernetes.io/service-account-token
EOF
```

Wait a few seconds, then extract the token and CA certificate:

```bash
# Extract token
TOKEN=$(kubectl get secret kdp-workspaces-operator-token -n kdp-workspace-sa \
  -o jsonpath='{.data.token}' | base64 -d)

# Extract CA certificate
CA_CERT=$(kubectl get secret kdp-workspaces-operator-token -n kdp-workspace-sa \
  -o jsonpath='{.data.ca\.crt}')

# Verify token exists
echo "Token length: ${#TOKEN}"
```

### Step 4: Create Kubeconfig for the Operator

Create a kubeconfig file using the service account credentials:

```bash
# Replace with your actual KDP API server URL
KDP_SERVER="https://your-kdp-api-server:8443/clusters/root"

cat > kdp-workspace-operator-kubeconfig.yaml <<EOF
apiVersion: v1
kind: Config
clusters:
  - name: kdp
    cluster:
      certificate-authority-data: ${CA_CERT}
      server: ${KDP_SERVER}
contexts:
  - name: kdp
    context:
      cluster: kdp
      user: kdp-workspace-operator
current-context: kdp
users:
  - name: kdp-workspace-operator
    user:
      token: ${TOKEN}
EOF
```

### Step 5: Verify Service Account Access

Test the kubeconfig to ensure proper permissions:

```bash
# Use the new kubeconfig
export KUBECONFIG=./kdp-workspace-operator-kubeconfig.yaml

# Test workspace listing
kubectl get workspaces

# Test workspace creation
kubectl apply -f - <<EOF
apiVersion: tenancy.kcp.io/v1alpha1
kind: Workspace
metadata:
  name: test-sa-verification
spec:
  type:
    name: kdp-organization
EOF

# Verify creation succeeded
kubectl get workspace test-sa-verification

# Clean up test workspace
kubectl delete workspace test-sa-verification
```

### Step 6: Configure the Operator

Create the Secret and ConfigMap in your Kubernetes cluster:

```bash
# Switch to your local Kubernetes cluster
export KUBECONFIG=~/.kube/config

# Create namespace
kubectl create namespace kdp-workspaces-system

# Create Secret with the operator kubeconfig
kubectl create secret generic kdp-workspace-kubeconfig \
  --from-file=kubeconfig=./kdp-workspace-operator-kubeconfig.yaml \
  -n kdp-workspaces-system

# Create ConfigMap with KDP connection details
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: kdp-workspace-config
  namespace: kdp-workspaces-system
data:
  kcp-url: "https://your-kdp-api-server:8443"
  kcp-workspace-path: "root"
  workspace-type: "kdp-organization"
EOF
```

**Important:**
- Replace `https://your-kdp-api-server:8443` with your actual KDP API server URL
- Store the `kdp-workspace-operator-kubeconfig.yaml` file securely
- Do not commit kubeconfig files to version control

### Security Best Practices

**Token Management:**
- Service account tokens are long-lived; implement rotation policies as needed
- Consider using secret management systems (Vault, Sealed Secrets, etc.)
- Regularly audit token usage and permissions

**Least Privilege:**
- The ClusterRole grants only workspace management permissions
- Add delete permissions only when implementing workspace cleanup features
- Review and adjust permissions based on your security requirements

**Secret Protection:**
- Ensure Kubernetes RBAC restricts access to the Secret in `kdp-workspaces-system` namespace
- Use encryption at rest for Secrets in your Kubernetes cluster
- Monitor access to the Secret using audit logs

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/kdp-workspaces:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/kdp-workspaces/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

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

