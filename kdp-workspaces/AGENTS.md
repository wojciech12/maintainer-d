# CLAUDE.md

## Overview

The KDP Workspaces Operator is responsible for creating and managing KDP (Kubermatic Development Platform) organizations for CNCF projects and foundation services. This operator bridges the maintainer-d CRDs with KCP workspace provisioning.

### Purpose

The operator serves as the foundation for a self-service portal for CNCF project maintainers by:

1. **Organization Creation** - Automatically creates KDP organizations for:
   - Every CNCF project (service consumers, 240+ projects)
   - Foundation services (service providers and management)

2. **Access Management** - Sets up access for:
   - Project maintainers (as KDP organization admins)
   - Collaborators (as read-only users)
   - CNCF Staff and support team members (as admins)

3. **CRD Integration** - Consumes CRDs from the maintainer-d operator:
   - `projects.maintainer-d.cncf.io` - source for project organizations
   - `maintainers.maintainer-d.cncf.io` - for admin membership
   - `collaborators.maintainer-d.cncf.io` - for read-only access

4. **Workspace Hierarchy** - Manages KDP workspace structure using kcp:
   - v1 uses a flat hierarchy under root workspace
   - KDP organization names derived from GitHub org slugs
   - Delegates cross-workspace service setup to separate operators

### Design Reference

See @CLAUDE_20251223_kdp_organiztion_op_design_doc.md for detailed architecture and design decisions.

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
