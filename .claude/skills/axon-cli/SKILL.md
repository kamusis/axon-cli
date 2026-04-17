```markdown
# axon-cli Development Patterns

> Auto-generated skill from repository analysis

## Overview
This skill teaches the core development patterns and workflows for contributing to the `axon-cli` project, a Go-based command-line tool. You'll learn about file organization, code style, commit conventions, and how to write and run tests in this repository.

## Coding Conventions

### File Naming
- Use **snake_case** for all file names.
  - Example: `main_cli.go`, `utils_test.go`

### Imports
- Use **relative imports** for internal packages.
  - Example:
    ```go
    import "./utils"
    ```

### Exports
- Use **named exports** for functions and variables.
  - Example:
    ```go
    // In utils.go
    package utils

    func ParseArgs(args []string) []string {
        // implementation
    }
    ```

### Commit Messages
- Follow the **Conventional Commits** style.
- Use the `fix` prefix for bug fixes.
  - Example:
    ```
    fix: correct argument parsing for multi-word commands
    ```

## Workflows

### Creating a Feature or Bugfix
**Trigger:** When starting new development work or fixing a bug  
**Command:** `/start-feature`

1. Create a new branch with a descriptive name (e.g., `feature/add-logging` or `fix/arg-parsing`).
2. Implement your changes following coding conventions.
3. Write or update tests as needed.
4. Commit using the conventional format.
5. Push your branch and open a pull request.

### Running Tests
**Trigger:** Before submitting a pull request or after making changes  
**Command:** `/run-tests`

1. Locate test files matching the `*.test.*` pattern.
2. Run tests using Go's testing tool:
    ```sh
    go test ./...
    ```
3. Ensure all tests pass before merging.

### Reviewing Code
**Trigger:** When reviewing a pull request  
**Command:** `/review-pr`

1. Check for adherence to file naming and import conventions.
2. Verify commit messages follow the conventional pattern.
3. Ensure tests are present and passing.
4. Leave feedback or approve the PR.

## Testing Patterns

- Test files follow the `*.test.*` naming pattern (e.g., `utils.test.go`).
- Testing framework is not explicitly defined; use Go's built-in `testing` package.
- Example test:
    ```go
    // In utils.test.go
    package utils

    import "testing"

    func TestParseArgs(t *testing.T) {
        args := []string{"--help"}
        result := ParseArgs(args)
        if len(result) != 1 || result[0] != "--help" {
            t.Errorf("Expected [--help], got %v", result)
        }
    }
    ```

## Commands
| Command         | Purpose                                           |
|-----------------|---------------------------------------------------|
| /start-feature  | Start a new feature or bugfix branch              |
| /run-tests      | Run all tests in the repository                   |
| /review-pr      | Review a pull request for code and style quality  |
```