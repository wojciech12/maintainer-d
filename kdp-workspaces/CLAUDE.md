## Testing Operator

KDP offers a central control plane for IDPs by providing an API backbone that allows registration (as a service provider) and consumption (as a platform user) of services. KDP itself does not host the actual workloads providing such services (for example, if a database service is offered, the underlying PostgreSQL pods are not hosted in KDP) and instead delegates this to so-called service clusters. A component called api-syncagent is installed on service clusters, which allows service providers (who own the service clusters) to publish APIs from their service cluster onto KDP's central platform.

### KDP cluster https://services.cncf.io/

You can test whether the operator correctly creates the workspaces:

```bash
export KUBECONFIG=$(git rev-parse --show-toplevel)/tmp/KDP_KUBECONFIG
```

```bash
kubectl get ws
kubectl describe ws cncf
```

We will operate Workspaces (`ws`) of type `kdp-organization`.

### Service cluster

1. Ensure you are on the right cluster:

   ```bash
   unset KUBECONFIG
   kubectx context-cdv2c4jfn5q
   ```

2. Read the CRDs:

   ```bash
   kubectl get crd
   kubectl get crd | grep maintainer
   ```

3. Get the values for CRs:

   ```bash
   kubectl get projects.maintainer-d.cncf.io -n maintainerd
   ```

## Validation Results

See [VALIDATION.md](./VALIDATION.md) for detailed validation results from testing against real Project CRDs.

**Key Findings**:
- ✅ All 249 project names are DNS-1123 compliant
- ✅ No existing status.conditions on any project
- ✅ No existing annotations on any project
- ✅ All projects are in `maintainerd` namespace
- ✅ Controller implementation is validated and ready
