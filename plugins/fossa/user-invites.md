# FOSSA User Invitation Endpoints

## List Pending Invitations
- **Method**: GET
- **Path**: /api/user-invitations
- **Description**: Returns all active invitations that have not yet expired (48-hour lifetime). Includes invitee email, creator information, and relevant timestamps.

## Create Invitations
- **Method**: POST
- **Path**: /api/organizations/:id/invite
- **Description**: Creates new user invitations, supporting both single and bulk operations for the specified organization.

## Delete Invitation
- **Method**: DELETE
- **Path**: /api/user-invitations/:email
- **Description**: Cancels a pending invitation identified by the invitee email address.

## Pending SSO Domain Invitations
- **Method**: GET
- **Path**: /api/organizations/:id/pending-sso-domains
- **Description**: Lists outstanding SSO domain verification invitations for the specified organization.

## Authorization
- Proper user invitation permissions are required to call these endpoints successfully.
