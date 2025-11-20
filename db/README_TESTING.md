# Database Testing Guide

## Overview

Unit tests for the database layer use an **in-memory SQLite database** for fast, isolated testing without requiring external dependencies.

## Approach

### In-Memory SQLite Database
- Tests create a fresh `:memory:` SQLite database for each test
- Schema is migrated using GORM's AutoMigrate
- No cleanup required - database is garbage collected after test completion

### Test Structure

1. **`setupTestDB(t)`** - Creates and migrates a clean in-memory database
2. **`seedTestData(t, db)`** - Populates the database with test fixtures
3. **Individual test functions** - Test specific functionality

## Example: TestGetMaintainersByProject

```go
func TestGetMaintainersByProject(t *testing.T) {
    db := setupTestDB(t)
    company, project1, project2, m1, m2, m3 := seedTestData(t, db)
    store := NewSQLStore(db)
    
    // Test cases...
}
```

### Test Fixtures

The `seedTestData` function creates:
- 1 company ("Test Company")
- 2 projects (kubernetes, prometheus)
- 3 maintainers (Alice, Bob, Charlie)
- Associations: 
  - kubernetes → Alice, Bob
  - prometheus → Bob, Charlie

### What Gets Tested

1. ✅ Returns correct maintainers for a project
2. ✅ Company relationships are preloaded
3. ✅ Empty results for projects with no maintainers
4. ✅ Empty results for non-existent projects
5. ✅ All maintainer fields are populated correctly
6. ✅ Projects field is NOT preloaded (as expected)

## Running Tests

```bash
# Run all db tests
go test ./db

# Run specific test
go test ./db -run TestGetMaintainersByProject

# Run with verbose output
go test -v ./db

# Run with coverage
go test -cover ./db
```

## Benefits of This Approach

1. **Fast** - In-memory database is extremely fast
2. **Isolated** - Each test gets a fresh database
3. **No dependencies** - No need for Docker, external DB, or test infrastructure
4. **Deterministic** - Tests always start with the same state
5. **Parallel-safe** - Each test has its own database instance

## Adding New Tests

To add a new test:

1. Use `setupTestDB(t)` to get a fresh database
2. Create your own fixtures or use `seedTestData(t, db)` if appropriate
3. Test your function
4. Assert expected behavior

Example:
```go
func TestYourFunction(t *testing.T) {
    db := setupTestDB(t)
    // Create custom test data if needed
    project := model.Project{Name: "test"}
    require.NoError(t, db.Create(&project).Error)
    
    store := NewSQLStore(db)
    result, err := store.YourFunction(project.ID)
    
    require.NoError(t, err)
    assert.Equal(t, expectedValue, result)
}
```
