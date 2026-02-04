# FOSSA User Email Invitation Implementation Plan

**Last Updated**: 2026-02-04
**Status**: Draft

## Overview

Implement FOSSA user invitation functionality for the `CodeScannerFossa` controller. The controller will invite users to FOSSA via email and track the invitation status in the CR.

**Reference**: See [user-invites.md](../plugins/fossa/user-invites.md) for FOSSA API documentation on user invitations.

## Requirements

Based on [PROMPT_FOSSA.md](PROMPT_FOSSA.md):

1. Add a `fossaUserEmails` field (array of emails) to the FOSSA CRD
2. Invite users to FOSSA through the FOSSA API using existing plugin
3. Report on user invitation status (pending, accepted, etc.) in CR status

## Key FOSSA API Endpoints

From `plugins/fossa/user-invites.md`:

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/user-invitations` | List pending invitations (48h lifetime) |
| POST | `/api/organizations/:id/invite` | Create user invitations |
| DELETE | `/api/user-invitations/:email` | Cancel pending invitation |

## Existing FOSSA Client Capabilities

The `/plugins/fossa/client.go` already provides:

- ✅ `SendUserInvitation(email string) error` - Line 155
- ✅ `FetchUserInvitations() (string, error)` - Line 119
- ✅ `HasPendingInvitation(email string) (bool, error)` - Line 143
- ✅ `FetchUsers() ([]User, error)` - Line 64 (to check if user accepted)
- ✅ Error codes: `ErrCodeInviteAlreadyExists (2011)`, `ErrCodeUserAlreadyMember (2001)`

## Architecture

```
CodeScannerFossa CR
  spec:
    projectName: "argo"
    fossaUserEmails:        <── NEW FIELD
      - "alice@example.com"
      - "bob@example.com"
        ↓
    Controller
        ↓
    ┌────────────────────────────────┐
    │ For each email in userEmails:  │
    │   1. Check if user exists      │
    │   2. Check pending invitation  │
    │   3. Send invitation if needed │
    └────────────────────────────────┘
        ↓
    ┌────────────────────────────────┐
    │ Update CR Status               │
    │ - userInvitations[]            │
    │   - email                      │
    │   - status (pending/accepted)  │
    │   - invitedAt                  │
    │   - acceptedAt                 │
    └────────────────────────────────┘
```

## Implementation Phases

### Phase 1: Add CRD Fields

**Goal:** Extend `CodeScannerFossaSpec` and `CodeScannerFossaStatus` with user email fields

**Tasks:**

1. Add `fossaUserEmails` field to `CodeScannerFossaSpec`:
   ```go
   // FossaUserEmails is a list of email addresses to invite to FOSSA
   // +optional
   FossaUserEmails []string `json:"fossaUserEmails,omitempty"`
   ```

2. Add `FossaUserInvitation` status struct:
   ```go
   // FossaUserInvitation tracks the invitation status for a user
   type FossaUserInvitation struct {
       // Email is the user's email address
       Email string `json:"email"`
       
       // Status is the current invitation status (Pending, Accepted, Failed)
       Status string `json:"status"`
       
       // Message provides additional context about the status
       // +optional
       Message string `json:"message,omitempty"`
       
       // InvitedAt is when the invitation was sent
       // +optional
       InvitedAt *metav1.Time `json:"invitedAt,omitempty"`
       
       // AcceptedAt is when the user accepted the invitation
       // +optional
       AcceptedAt *metav1.Time `json:"acceptedAt,omitempty"`
   }
   ```

3. Add `UserInvitations` field to `CodeScannerFossaStatus`:
   ```go
   // UserInvitations tracks the status of user invitations
   // +optional
   UserInvitations []FossaUserInvitation `json:"userInvitations,omitempty"`
   ```

4. Regenerate CRD manifests:
   ```bash
   make generate manifests
   ```

**Files to modify:**
- `code-scanners/api/v1alpha1/codescannerfossa_types.go`

**Files generated:**
- `code-scanners/config/crd/bases/maintainer-d.cncf.io_codescannerfossas.yaml`

---

### Phase 2: Extend FossaClient Interface

**Goal:** Add invitation methods to the controller's `FossaClient` interface

**Tasks:**

1. Extend `FossaClient` interface in controller:
   ```go
   type FossaClient interface {
       CreateTeam(name string) (*fossa.Team, error)
       FetchTeam(name string) (*fossa.Team, error)
       // New methods for user invitations
       SendUserInvitation(email string) error
       HasPendingInvitation(email string) (bool, error)
       FetchUsers() ([]fossa.User, error)
   }
   ```

2. Verify interface implementation matches real client

**Files to modify:**
- `code-scanners/internal/controller/codescannerfossa_controller.go`

---

### Phase 3: Implement Invitation Logic

**Goal:** Add reconciliation logic to process user invitations

**Tasks:**

1. Create helper function `ensureUserInvitations()`:
   ```go
   func (r *CodeScannerFossaReconciler) ensureUserInvitations(
       ctx context.Context,
       fossaClient FossaClient,
       emails []string,
   ) ([]maintainerdcncfiov1alpha1.FossaUserInvitation, error)
   ```

2. Implement invitation flow for each email:
   - Check if user is already a FOSSA member (`FetchUsers`)
   - Check if invitation is pending (`HasPendingInvitation`)
   - Send invitation if not member and no pending invite (`SendUserInvitation`)
   - Handle idempotency errors gracefully

3. Add invitation reconciliation to main `Reconcile()`:
   - Process after team creation is confirmed
   - Update status with invitation results
   - Add appropriate condition for invitation status

4. Add new condition type:
   ```go
   const (
       ConditionTypeUserInvitations = "UserInvitationsProcessed"
       ReasonInvitationsSent        = "InvitationsSent"
       ReasonInvitationsPartial     = "InvitationsPartiallyProcessed"
       ReasonInvitationsFailed      = "InvitationsFailed"
   )
   ```

**Files to modify:**
- `code-scanners/internal/controller/codescannerfossa_controller.go`
- `code-scanners/internal/controller/constants.go`

---

### Phase 4: Add Status Reconciliation

**Goal:** Periodically check and update invitation statuses

**Tasks:**

1. Implement status polling logic:
   - For pending invitations: check if still pending or expired (48h TTL)
   - For accepted: check if user exists in FOSSA users list
   - Update status timestamps accordingly

2. Add requeue logic for pending invitations:
   - Requeue after 1 hour if any invitations are pending
   - Stop requeuing once all are accepted or failed

**Files to modify:**
- `code-scanners/internal/controller/codescannerfossa_controller.go`

---

### Phase 5: Add Unit Tests

**Goal:** Comprehensive test coverage for invitation functionality

**Tasks:**

1. Extend mock FossaClient:
   ```go
   type MockFossaClient struct {
       // Existing fields
       CreateTeamFunc func(name string) (*fossa.Team, error)
       FetchTeamFunc  func(name string) (*fossa.Team, error)
       // New fields
       SendUserInvitationFunc  func(email string) error
       HasPendingInvitationFunc func(email string) (bool, error)
       FetchUsersFunc          func() ([]fossa.User, error)
   }
   ```

2. Add test cases:
   - Invite new user successfully
   - User already a member (idempotent)
   - Invitation already pending (idempotent)
   - Invitation API failure
   - Mixed success/failure scenarios
   - Status transition from Pending to Accepted

**Files to modify:**
- `code-scanners/internal/controller/codescannerfossa_controller_test.go`

---

### Phase 6: Add Integration/E2E Tests

**Goal:** Validate end-to-end flow with real or simulated FOSSA API

**Tasks:**

1. Add E2E test for invitation flow
2. Update existing E2E tests to include user emails

**Files to modify:**
- `code-scanners/test/e2e/` (existing E2E test structure)

---

## Status Values

| Status | Description |
|--------|-------------|
| `Pending` | Invitation sent, awaiting user acceptance |
| `Accepted` | User accepted invitation and is a FOSSA member |
| `AlreadyMember` | User was already a FOSSA organization member |
| `Failed` | Invitation could not be sent (see message) |
| `Expired` | Invitation expired (48h TTL) without acceptance |

---

## Error Handling

| FOSSA Error Code | Error | Handling |
|------------------|-------|----------|
| 2011 | `ErrInviteAlreadyExists` | Set status to `Pending`, no action needed |
| 2001 | `ErrUserAlreadyMember` | Set status to `AlreadyMember`, no action needed |
| Other | API error | Set status to `Failed` with message, emit warning event |

---

## Dependencies

- Existing `/plugins/fossa/client.go` (no changes required)
- `github.com/cncf/maintainer-d/plugins/fossa` import already present

---

## Validation Checklist

- [ ] CRD schema accepts `fossaUserEmails` array
- [ ] Controller sends invitations for new emails
- [ ] Controller is idempotent (no duplicate invitations)
- [ ] Status reflects accurate invitation state
- [ ] Events are emitted for success/failure
- [ ] Unit tests pass
- [ ] E2E tests pass
- [ ] `make generate manifests` produces valid CRD

---

## Example CR

```yaml
apiVersion: maintainer-d.cncf.io/v1alpha1
kind: CodeScannerFossa
metadata:
  name: argo-fossa
  namespace: code-scanners
spec:
  projectName: argo
  fossaUserEmails:
    - "maintainer1@argoproj.io"
    - "maintainer2@argoproj.io"
```

## Example Status After Reconciliation

```yaml
status:
  observedGeneration: 1
  fossaTeam:
    id: 456
    name: argo
    organizationId: 162
    url: https://app.fossa.com/account/settings/organization/teams/456
  userInvitations:
    - email: "maintainer1@argoproj.io"
      status: Accepted
      invitedAt: "2026-02-01T10:00:00Z"
      acceptedAt: "2026-02-02T14:30:00Z"
    - email: "maintainer2@argoproj.io"
      status: Pending
      invitedAt: "2026-02-04T09:00:00Z"
      message: "Invitation sent, awaiting acceptance"
  conditions:
    - type: FossaTeamReady
      status: "True"
      reason: TeamCreated
      message: "FOSSA team 'argo' ready (ID: 456)"
    - type: UserInvitationsProcessed
      status: "True"
      reason: InvitationsSent
      message: "2 invitations processed (1 accepted, 1 pending)"
```
