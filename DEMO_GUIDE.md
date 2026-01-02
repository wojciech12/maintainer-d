# Maintainerd Demo Guide

## Overview

This repository includes comprehensive demo tests that showcase the maintainerd onboarding workflow. The demos use in-memory databases and mocked clients to demonstrate the system without requiring external dependencies.

## Running the Demo

### Quick Demo Run

```bash
# Run all demo tests
go test ./... -v

# Run only onboarding demos
go test ./onboarding -v

# Run specific demo test
go test ./onboarding -v -run TestFossaChosen_Basic/successful_onboarding_with_new_team
```

## Demo Scenarios

The demo tests showcase the complete CNCF project onboarding workflow:

### 1. **FOSSA Onboarding Workflow** (`TestFossaChosen_Basic`)

#### Scenario: Successful Onboarding with New Team

**What it demonstrates:**

- Creates a new FOSSA team for a project
- Sends invitations to all project maintainers
- Posts a detailed onboarding report to GitHub

**Demo Output:**

```
2025/12/31 11:11:35 fossaChosen: DBG by test-project
2025/12/31 11:11:35 team created: test-project
2025/12/31 11:11:35 handleWebhook: INF, ### maintainer-d CNCF FOSSA onboarding - Report

#### :spiral_notepad: Actions taken during onboarding...

- ✅  test-project has 2 maintainers registered in maintainer-d
- 👥  [test-project team](https://app.fossa.com/account/settings/organization/teams/1000) has been created in FOSSA
- ✅ Invitation(s) to join CNCF FOSSA sent to @alice @bob
- The test-project project has not yet imported repos
---

When you have accepted your invitation to join CNCF FOSSA :

- Add a comment _/fossa-invite accepted_ to this issue, maintainer-d onboarding process will add you to you team as a **Team Admin** ([FOSSA RBAC](https://docs.fossa.com/docs/role-based-access-control#team-roles)).

- then, _and only then_, can you start importing your code and documentation repositories into FOSSA: [Getting Started Guide](https://docs.fossa.com/docs/getting-started#importing-a-project).
```

#### Scenario: Maintainer Already Has Pending Invitation

**What it demonstrates:**

- Handles cases where a maintainer already has a pending FOSSA invitation
- Still creates the team and sends invitations to other maintainers
- Provides clear status in the onboarding report

#### Scenario: Maintainer Already Exists in FOSSA

**What it demonstrates:**

- Handles existing FOSSA users gracefully
- Adds existing users directly to the team as Team Admins
- Only sends invitations to non-members

### 2. **Label Command System** (`TestLabelCommand`)

The demo showcases the `/label` command system that allows maintainers to choose their preferred license scanning tool.

#### Scenario: Successful Label Addition - FOSSA

**What it demonstrates:**

- Registered maintainer adds "fossa" label to onboarding issue
- System validates authorization
- Label is added to GitHub issue
- Confirmation comment is posted

**Demo Output:**

```
2025/12/31 11:11:35 handleLabelCommand: INF, @alice added label "fossa" to issue #100 for project "test-project"
```

#### Scenario: Successful Label Addition - Snyk

**What it demonstrates:**

- Same workflow for "snyk" label
- System supports multiple license scanning tools

**Demo Output:**

```
2025/12/31 11:11:35 handleLabelCommand: INF, @bob added label "snyk" to issue #101 for project "test-project"
```

#### Scenario: Unauthorized User

**What it demonstrates:**

- Security: only registered maintainers can execute commands
- Clear error message for unauthorized attempts

**Demo Output:**

```
2025/12/31 11:11:35 handleLabelCommand: WRN, @unauthorized-user is not authorized for project "test-project"
```

#### Scenario: Invalid Label Name

**What it demonstrates:**

- Input validation for label commands
- Only "fossa" and "snyk" are valid options
- Helpful error message with valid options

#### Scenario: Invalid Command Format

**What it demonstrates:**

- Validates command syntax
- Provides usage instructions
- Guides users to correct format

#### Scenario: Project Not Found

**What it demonstrates:**

- Error handling for non-existent projects
- Clear messaging about missing projects

**Demo Output:**

```
2025/12/31 11:11:35 handleLabelCommand: WRN, project "non-existent-project" not found in cache
```

#### Scenario: Case Insensitive Label Names

**What it demonstrates:**

- User-friendly: accepts "FOSSA", "Fossa", "fossa"
- Normalizes to lowercase internally

### 3. **Mock Infrastructure** (`TestMockInfrastructure`)

The demo showcases the testing infrastructure:

#### GitHub Transport Mock

**What it demonstrates:**

- Captures all GitHub API calls
- Records comments created
- Tracks labels added
- Enables verification without hitting real API

#### FOSSA Client Mock

**What it demonstrates:**

- Simulates FOSSA API in memory
- Tracks invitations sent
- Records teams created
- Manages user existence state
- Fast, reliable, no external dependencies

#### In-Memory Database

**What it demonstrates:**

- Fast database operations
- No file cleanup needed
- Isolated test environments
- Same schema as production

## Demo Test Results

All demo tests pass successfully:

```
=== RUN   TestMockInfrastructure
--- PASS: TestMockInfrastructure (0.01s)
    --- PASS: TestMockInfrastructure/mock_GitHub_transport_captures_comments
    --- PASS: TestMockInfrastructure/mock_FOSSA_client_tracks_invitations
    --- PASS: TestMockInfrastructure/mock_FOSSA_client_creates_teams
    --- PASS: TestMockInfrastructure/in-memory_database_works
    --- PASS: TestMockInfrastructure/helper_functions_create_proper_events

=== RUN   TestFossaChosen_Basic
--- PASS: TestFossaChosen_Basic (0.03s)
    --- PASS: TestFossaChosen_Basic/successful_onboarding_with_new_team
    --- PASS: TestFossaChosen_Basic/maintainer_already_has_pending_invitation
    --- PASS: TestFossaChosen_Basic/maintainer_already_exists_in_FOSSA

=== RUN   TestLabelCommand
--- PASS: TestLabelCommand (0.06s)
    --- PASS: TestLabelCommand/successful_label_addition_-_fossa
    --- PASS: TestLabelCommand/successful_label_addition_-_snyk
    --- PASS: TestLabelCommand/unauthorized_user
    --- PASS: TestLabelCommand/invalid_label_name
    --- PASS: TestLabelCommand/invalid_command_format
    --- PASS: TestLabelCommand/project_not_found
    --- PASS: TestLabelCommand/case_insensitive_label_names
```

## What the Demo Shows

### Complete Onboarding Workflow

1. **Project Registration** - Maintainers are registered in the database
2. **Tool Selection** - Maintainers choose FOSSA or Snyk via `/label` command
3. **Team Creation** - FOSSA team is created for the project
4. **Invitations** - Maintainers receive FOSSA invitations
5. **Acceptance** - Maintainers accept invitations
6. **Team Assignment** - Maintainers are added to their project team as Team Admins
7. **Repository Import** - Maintainers import their repositories into FOSSA

### Security Features

- Authorization checks for all commands
- Only registered maintainers can execute actions
- Clear error messages for unauthorized attempts

### User Experience

- Clear, informative GitHub comments
- Step-by-step guidance
- Links to documentation
- Status updates throughout the process

### Error Handling

- Graceful handling of edge cases
- Helpful error messages
- Recovery from partial failures
- Clear next steps

## Running Individual Demos

### FOSSA Onboarding Demo

```bash
# Run all FOSSA onboarding scenarios
go test ./onboarding -v -run TestFossaChosen_Basic

# Run specific scenario
go test ./onboarding -v -run TestFossaChosen_Basic/successful_onboarding_with_new_team
```

### Label Command Demo

```bash
# Run all label command scenarios
go test ./onboarding -v -run TestLabelCommand

# Run specific scenario
go test ./onboarding -v -run TestLabelCommand/successful_label_addition_-_fossa
```

### Infrastructure Demo

```bash
# Run mock infrastructure tests
go test ./onboarding -v -run TestMockInfrastructure
```

## Demo vs Production

| Aspect      | Demo                    | Production                   |
| ----------- | ----------------------- | ---------------------------- |
| Database    | In-memory SQLite        | SQLite file                  |
| GitHub API  | Mocked HTTP transport   | Real GitHub API              |
| FOSSA API   | Mocked in-memory client | Real FOSSA API               |
| Speed       | Fast (milliseconds)     | Slower (network calls)       |
| Reliability | 100% (no network)       | Depends on external services |
| Setup       | Zero configuration      | Requires credentials         |

## Next Steps

1. **Explore the code** - See [`onboarding/server.go`](onboarding/server.go:1) for implementation
2. **Read documentation** - See [`onboarding/README.MD`](onboarding/README.MD:1) for workflow details
3. **Run the server** - See [`QUICKSTART.md`](QUICKSTART.md:1) to run locally
4. **Test with real data** - Set up credentials for live testing

## Demo Files

- [`onboarding/server_test.go`](onboarding/server_test.go:1) - Demo test suite
- [`onboarding/github_mock.go`](onboarding/github_mock.go:1) - GitHub API mock
- [`onboarding/fossa_mock.go`](onboarding/fossa_mock.go:1) - FOSSA API mock
- [`onboarding/test_helpers.go`](onboarding/test_helpers.go:1) - Test utilities
- [`onboarding/TESTING_QUICKSTART.md`](onboarding/TESTING_QUICKSTART.md:1) - Testing guide
- [`onboarding/TESTING_STRATEGY.md`](onboarding/TESTING_STRATEGY.md:1) - Full testing strategy

## Summary

The maintainerd demo provides a complete, working demonstration of the CNCF project onboarding system. It showcases:

✅ FOSSA team creation and management
✅ Maintainer invitation workflow
✅ GitHub webhook handling
✅ Label-based command system
✅ Authorization and security
✅ Error handling and edge cases
✅ User-friendly messaging
✅ Complete onboarding lifecycle

Run `go test ./... -v` to see all demos in action!
