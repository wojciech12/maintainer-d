# maintainerd

maintainerd is a database-backed application that tracks Maintainers that work on CNCF Projects using multiple datasources.

At present, a Project Maintainer registers with the CNCF by
 - sending an email triggering a manual add to an internal Google Worksheet
 - requesting access to services via a Service Desk Request

## Proposed Maintainer and Project registration methods
 - maintainer.yaml formally lists out the maintainers for a project
 - openprofile.dev
 - admin front end for CNCF Project Team

# Components
## Database
The maintainerd backend is a database implemented using GORM, a golang object relational mapping tool.

Building and running the resultant db executable loads data from the internal worksheet.

```
cd db
go build
./db
2025/06/10 05:06:34 maintainerd: backend: database successfully seeded demo.db (SQLite)
```
Data is stored in a file called demo.db
MD_WORKSHEET needs to contain the ID of the Goolge Worksheet being read
credentials.json needs to contain the Google Service Account that is allowed to read the
worksheet.

## Service Plugins

A plugin will reconcile the list of maintainers for a project and ensure that they are registered
with their chosen services.

## Make Targets (deploy)

Common workflow targets for local or cluster deployments. All paths now reference the Kustomize base under `deploy/kustomize/base`.

- env: Generate `bootstrap.env` from `.envrc` (KEY=VALUE lines)
- apply-env: Create/update Secret `maintainerd-bootstrap-env` from `bootstrap.env`
- apply-creds: Create/update Secret `workspace-credentials` from `cmd/bootstrap/credentials.json`
- secrets: Run `env`, `apply-env`, and `apply-creds`
- apply-pvc: Apply `deploy/kustomize/base/pvc.yaml`
- bootstrap-apply: Apply `deploy/kustomize/base/bootstrap.yaml` (DB bootstrap Job)
- bootstrap-wait: Wait for bootstrap Job completion
- maintainerd-apply: Apply `deploy/kustomize/base/maintainerd.yaml` (Deployment/Service/Ingress)
- kustomize-base-apply: Apply the entire base with `kubectl apply -k deploy/kustomize/base`
- maintainerd-port-forward: Forward `localhost:2525 -> svc/maintainerd:2525`
 - kustomize-diff-prod: Show differences for the prod overlay (`kubectl diff -k deploy/kustomize/overlays/prod`)
 - kustomize-diff-dev: Show differences for the dev overlay (`kubectl diff -k deploy/kustomize/overlays/dev`)
 - kustomize-dryrun-prod: Server-side dry run for prod overlay (schema/admission checks)
 - kustomize-dryrun-dev: Server-side dry run for dev overlay (schema/admission checks)

Notes
- Root-level YAMLs (`bootstrap.yaml`, `pvc.yaml`, `maintainerd.yaml`) were removed. Use the Kustomize base copies under `deploy/kustomize/base` or the `kustomize-base-apply` target.
- Argo CD uses the overlays in `deploy/kustomize/overlays/{dev,prod}`. Paths are constrained to be within the kustomization; avoid `..` in kustomize `resources`.

## Argo CD

Tips for syncing and troubleshooting the Argo CD Applications:

- Hard refresh: On the app tile, open the app → click the caret next to Refresh → Hard refresh to clear repo cache.
- Sync resources: Click Sync; in the left Resources tree, tick All (or specific items). The message “Select at least one resource” means nothing is checked yet.
- Enable Auto‑Sync: In the app page, three‑dot menu → Enable Auto‑Sync (recommended with prune + selfHeal). Or set in the Application spec under `spec.syncPolicy.automated`.
- Webhooks vs polling: Configure a GitHub webhook to your Argo CD server at `/api/webhook` for immediate refreshes; otherwise Argo CD refreshes on its polling interval.
- Kustomize paths: Argo CD enforces load restrictions—resources must be in or below the kustomization directory. Use the base at `deploy/kustomize/base` and overlays under `deploy/kustomize/overlays/*`.
- Inspect errors: Check App Details → Manifest/Events/Conditions. To reproduce locally, run `kustomize build deploy/kustomize/overlays/prod` (or `kubectl kustomize ...`).
