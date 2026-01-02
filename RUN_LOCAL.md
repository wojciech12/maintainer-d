# Running Maintainerd Locally

## Quick Start

### 1. Configure Environment Variables

Edit the `.env` file with your actual credentials:

```bash
# Edit .env and replace the placeholder values
nano .env
```

Required variables:

- `GITHUB_WEBHOOK_SECRET`: A random secret string for webhook validation
- `GITHUB_API_TOKEN`: Your GitHub personal access token
- `FOSSA_API_TOKEN`: Your FOSSA API token

Optional variables (for full bootstrap with seed data):

- `MD_WORKSHEET`: Google Spreadsheet ID for loading maintainer data
- `WORKSPACE_CREDENTIALS_FILE`: Path to Google Workspace credentials JSON file

### 2. Initialize the Database

**Option A: Quick Start (Schema Only)**

Run the demo database initializer to create the schema without external dependencies:

```bash
go build -o init_demo_db cmd/init_demo_db/main.go
./init_demo_db
```

This creates `maintainers.db` with the database schema but no seed data.

**Option B: Full Bootstrap (With Seed Data)**

If you have Google Workspace credentials and spreadsheet access:

```bash
export MD_WORKSHEET="your-spreadsheet-id"
export WORKSPACE_CREDENTIALS_FILE="path/to/credentials.json"
./bootstrap --db=maintainers.db --seed=true
```

### 3. Run the Main Application

From the project root, run the main application:

```bash
# Load environment variables and run
export $(cat .env | xargs)
./maintainerd --db-path=maintainers.db
```

The server will start listening on port 2525 by default.

### 4. Run Everything with One Command

Use the provided `run.sh` script:

```bash
./run.sh
```

This script will:

- Load environment variables from `.env`
- Initialize the database if it doesn't exist
- Start the maintainerd server

## Running with Custom Parameters

You can override defaults with command-line flags:

```bash
./maintainerd \
  --addr=:8080 \
  --db-path=/path/to/database.db \
  --org=your-github-org \
  --repo=your-repo \
  --webhook-secret=your-secret \
  --gh-api=your-github-token
```

## Testing the Application

### Run Unit Tests

```bash
# Run all tests
make test

# Run tests with verbose output
make test-verbose

# Run tests with coverage
make test-coverage

# Run specific package tests
make test-package PKG=db
```

### Run Database Tests

```bash
# Database tests use in-memory SQLite, no setup required
go test ./db -v
```

## Development Workflow

### Build Executables

```bash
# Build main application
go build -o maintainerd main.go

# Build database initializer
go build -o init_demo_db cmd/init_demo_db/main.go

# Build bootstrap tool (requires Google credentials)
go build -o bootstrap cmd/bootstrap/main.go
```

### Run with Make

```bash
# Run tests
make test

# Format code
make fmt

# Run linters
make lint

# Run full CI checks locally
make ci-local
```

## Troubleshooting

### Database Issues

If you encounter database errors:

1. Delete the existing database file:

   ```bash
   rm maintainers.db
   ```

2. Re-run the database initialization:
   ```bash
   go build -o init_demo_db cmd/init_demo_db/main.go
   ./init_demo_db
   ```

### Permission Issues

If you get permission denied errors:

```bash
# Make executables executable
chmod +x maintainerd
chmod +x init_demo_db
chmod +x bootstrap
```

### Port Already in Use

If port 2525 is already in use, specify a different port:

```bash
./maintainerd --addr=:8080 --db-path=maintainers.db
```

## Project Structure

- [`main.go`](main.go:1) - Main application entry point
- [`db/`](db/) - Database layer and initialization
  - [`bootstrap.go`](db/bootstrap.go:1) - Database seeding logic
  - [`store.go`](db/store.go:1) - Database interface
  - [`store_impl.go`](db/store_impl.go:1) - SQLite implementation
- [`onboarding/`](onboarding/) - Onboarding server logic
- [`apis/`](apis/) - Kubernetes API definitions
- [`cmd/`](cmd/) - Command-line tools
  - [`bootstrap/main.go`](cmd/bootstrap/main.go:1) - Bootstrap CLI tool

## Environment Variables Reference

| Variable                     | Description                               | Default | Required |
| ---------------------------- | ----------------------------------------- | ------- | -------- |
| `GITHUB_WEBHOOK_SECRET`      | Secret for GitHub webhook validation      | -       | Yes      |
| `GITHUB_API_TOKEN`           | GitHub personal access token              | -       | Yes      |
| `FOSSA_API_TOKEN`            | FOSSA API token                           | -       | Yes      |
| `MD_WORKSHEET`               | Google Spreadsheet ID for maintainer data | -       | Optional |
| `WORKSPACE_CREDENTIALS_FILE` | Path to Google Workspace credentials JSON | -       | Optional |
| `ORG`                        | GitHub organization name                  | cncf    | Optional |
| `REPO`                       | GitHub repository name                    | sandbox | Optional |

## Additional Resources

- See [`README.MD`](README.MD:1) for deployment instructions
- See [`db/README_TESTING.md`](db/README_TESTING.md:1) for database testing guide
- See [`onboarding/README.MD`](onboarding/README.MD:1) for onboarding workflow details
