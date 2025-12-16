# kcp Workspace Hierarchy

This document captures the initial workspace and API export layout that mirrors the
maintainerd domain model inside kcp.

## Prerequisites

- kcp v0.28.3+ running locally (`make kcp-install` followed by `./bin/kcp start ...`). The install
  target downloads the `kcp` CLI and the `apigen` helper and verifies release checksums.
- kubectl krew plugins for kcp. Install them as described in the upstream guide
  <https://docs.kcp.io/kcp/latest/setup/kubectl-plugin/>:

  ```bash
  kubectl krew index add kcp-dev https://github.com/kcp-dev/krew-index.git
  kubectl krew install kcp-dev/kcp
  kubectl krew install kcp-dev/ws
  kubectl krew install kcp-dev/create-workspace

  # verify the plugins are available
  kubectl krew list
  kubectl kcp version
  kubectl ws --help
  ```

## Workspace Topology

```
root
└── cncf                                   # Foundation-scoped configuration
    ├── people                            # Global directory for maintainers/collaborators/officers
    └── projects
        ├── <project-a>                   # Per-project workspaces
        └── <project-b>
```

- `root:cncf` owns the CRD definitions and publishes them through an API export.
- `root:cncf:people` holds globally visible identity resources (maintainers, collaborators, officers).
- `root:cncf:projects:*` workspaces host day-to-day project resources and are the primary targets
  for the bootstrap job.

## Bootstrapping Commands

```bash
# target the root workspace (absolute path syntax uses :root)
kubectl ws use :root

# create the cncf configuration workspace tree
kubectl ws create cncf
kubectl ws use cncf
kubectl ws create people
kubectl ws create projects

# projects are created on demand
kubectl ws create projects/kubernetes
kubectl ws create projects/etcd
```

## API Export Layout

1. Regenerate CRDs and APIResourceSchemas after any Go type change:

   ```bash
   make kcp-generate
   ```

   This runs `controller-gen` to refresh the CRDs in `config/crd/bases/` and then uses the kcp
   `apigen` tool to build matching APIResourceSchemas plus an APIExport manifest under
   `config/kcp/`. Run `make kcp-install` once to download the required `kcp` and `apigen`
   binaries into `./bin/`.

2. Apply the CRDs within the `root:cncf` workspace:

   ```bash
   kubectl ws use :root:cncf
   kubectl apply -f config/crd/bases/
   ```

3. Publish the API export from the same workspace:

   ```bash
   kubectl ws use :root:cncf
   kubectl apply -f config/kcp/api-export.yaml

   # the schema files live alongside the export for convenience
   kubectl apply -f config/kcp/schema-*.yaml
   ```

4. Grant access to the people workspace and project workspaces by creating API bindings that point
   to the `maintainer-d.cncf.io` export. The export owner must allow bindings first:

```bash
kubectl ws use cncf
kubectl apply -f config/kcp/api-export-bind-rbac.yaml
```

   Then bind from each consumer workspace. Using the plugin helper command handles the workspace
   path syntax and waits for the binding to become ready:

```bash
kubectl ws use cncf/people
kubectl kcp bind apiexport root:cncf:maintainer-d.cncf.io --name maintainer-d-foundation

# confirm the binding is ready
kubectl get apibinding maintainer-d-foundation -o wide
```

Repeat the binding step for each project workspace to make the API group available.

- Integrate the bootstrap process so it writes CRDs into the `cncf/people` workspace and the
  relevant `cncf/projects/*` workspaces.
- Define RBAC policies that map foundation operators, project maintainers, and observers to the
  appropriate workspaces.
- Keep a read-only SQLite mirror for ad-hoc SQL if desired, but treat kcp as the source of truth
  so all services consume the same API surface.

## Why kcp instead of direct SQL/GORM?

- **Unified API contract**: CRDs expose maintainer data through Kubernetes-native REST endpoints
  backed by OpenAPI schemas, so front-end, automations, and external clients share the same
  contract and generated tooling.
- **Workspace isolation**: kcp workspaces give each foundation/project a scoped control plane with
  fine-grained RBAC, avoiding bespoke tenancy filters in application code.
- **Event-driven controllers**: any change to a CRD emits watch events, enabling sidecar
  controllers (GitHub sync, onboarding tasks, service reconciliation) without overloading the core
  API server.
- **OIDC + RBAC out of the box**: integrate identity once and use standard Kubernetes roles to
  delegate management to project maintainers.
- **Scalable architecture**: when you need to attach compute clusters (via konnector) or expose the
  API to partner organizations, kcp handles namespace isolation, permission claims, and federation
  semantics.
- **Explicit schema evolution**: versioned CRDs and APIResourceSchemas provide a clear upgrade
  path versus implicit GORM migrations hidden in application code.
