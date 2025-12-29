## Testing Operator

### Setup

```bash
# Create kcp configuration (requires kcp kubeconfig with workspace permissions)
kubectl apply -f config/samples/kcp_config.yaml
kubectl create secret generic kdp-workspace-kubeconfig \
  --from-file=kubeconfig=/path/to/kcp-kubeconfig.yaml \
  -n kdp-workspaces-system

# Run locally
make run

# Or deploy
make deploy IMG=<registry>/kdp-workspaces:latest
```

### Verify on KDP Cluster (https://services.cncf.io/)

```bash
export KUBECONFIG=$(git rev-parse --show-toplevel)/tmp/KDP_KUBECONFIG
kubectl get ws                    # List workspaces (type: kdp-organization)
kubectl describe ws <project-name>
```

### Verify on Service Cluster

```bash
kubectx context-cdv2c4jfn5q
# Check Project status and annotations
kubectl get projects.maintainer-d.cncf.io -n maintainerd
kubectl get projects.maintainer-d.cncf.io <project-name> -n maintainerd -o yaml

# Debug: check operator logs
kubectl logs -n kdp-workspaces-system deployment/kdp-workspaces-controller-manager -f
```

## Validation Results

See [VALIDATION.md](./VALIDATION.md) for detailed validation results from testing against real Project CRDs.

**Key Findings**:

- ✅ All 249 project names are DNS-1123 compliant
- ✅ No existing status.conditions on any project
- ✅ No existing annotations on any project
- ✅ All projects are in `maintainerd` namespace
- ✅ Controller implementation is validated and ready
