# code-scanners

Kubernetes operator for managing code scanner integrations (FOSSA, Snyk) for CNCF projects.

## Description

The code-scanners operator automates the lifecycle of code scanning tools by managing team creation, user invitations, and access control via Custom Resource Definitions (CRDs). It creates ConfigMaps with scanner metadata for consumption by other systems and maintains automatic team membership synchronization

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
export IMG=wbkubermatic/code-scanners:0.0.1

make docker-build docker-push IMG=$IMG
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
make deploy IMG=<some-registry>/code-scanners:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

### Configure FOSSA Credentials

Before creating `CodeScannerFossa` resources, you need to create a secret with your FOSSA credentials in the same namespace:

```sh
kubectl create secret generic code-scanners \
  -n code-scanners \
  --from-literal=fossa-api-token='YOUR_FOSSA_API_TOKEN' \
  --from-literal=fossa-organization-id='YOUR_FOSSA_ORG_ID'
```

Or apply a YAML manifest:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: code-scanners
  namespace: code-scanners
type: Opaque
stringData:
  fossa-api-token: "YOUR_FOSSA_API_TOKEN"
  fossa-organization-id: "YOUR_FOSSA_ORG_ID"
```

**Required keys:**
- `fossa-api-token` - Your FOSSA Full API Token
- `fossa-organization-id` - Your FOSSA organization ID

## FOSSA Workflow

The `CodeScannerFossa` controller automates FOSSA team creation and user management.

### Reconciliation Flow

```yaml
apiVersion: maintainer-d.cncf.io/v1alpha1
kind: CodeScannerFossa
metadata:
  name: my-project
  namespace: code-scanners
spec:
  projectName: my-project           # Creates/fetches FOSSA team
  fossaUserEmails:                  # Optional: invite users
    - alice@example.com
    - bob@example.com
```

**Automatic behavior:**
1. **Team creation**: Creates FOSSA team matching `projectName` (idempotent)
2. **User invitations**: Sends email invitations to FOSSA organization
3. **Team membership**: Adds users to team once they accept (automatic)
4. **ConfigMap**: Creates ConfigMap with team metadata for consumption

### Status Tracking

```yaml
status:
  fossaTeam:
    id: 456
    name: my-project
    url: https://app.fossa.com/account/settings/organization/teams/456
  userInvitations:
    - email: alice@example.com
      status: AddedToTeam              # Pending → Accepted → AddedToTeam
      invitedAt: "2026-02-01T10:00:00Z"
      acceptedAt: "2026-02-02T14:30:00Z"
      addedToTeamAt: "2026-02-02T14:35:00Z"
    - email: bob@example.com
      status: Pending                  # Awaiting user acceptance
      invitedAt: "2026-02-04T09:00:00Z"
```

**Invitation states:**
- `Pending` → User invited, awaiting acceptance (48h TTL)
- `Accepted` → User accepted, pending team addition
- `AddedToTeam` → User is team member (stable state)
- `AlreadyMember` → User was already org member
- `Failed` → Invitation/team addition failed
- `Expired` → Invitation expired, will be resent

**Requeue behavior:**
- Pending invitations: Reconciles every 1 hour
- Stable state (all users on team): No requeue

### Edge Cases Handled

- **Idempotency**: Safe to apply CR multiple times
- **Race conditions**: Handles concurrent user additions via `ErrUserAlreadyMember`
- **Manual changes**: Re-adds users if removed from team outside controller
- **Expired invitations**: Automatically resends after 48h
- **Case-insensitive emails**: Handles email case variations

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

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/code-scanners:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/code-scanners/<tag or branch>/dist/install.yaml
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

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

