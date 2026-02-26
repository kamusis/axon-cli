---
trigger: always_on
---

# Workspace Rules

When generating or modifying any code in this Go project (github.com/kamusis/axon-cli), you MUST strictly follow this workflow:

1. **Test After Modifying**: After ANY code modification is completed, you must run `go test ./...` in the `src` directory to verify that no existing tests were broken.
2. **Build After Modifying**: In addition to testing, you must run `go build ./...` in the `src` directory to ensure there are no compilation errors or subtle typing/syntax errors (such as non-constant format strings in fmt.Errorf) that `go test` alone or your AI models might miss.
3. **Never Skip**: Do not assume your code changes are trivial enough to skip testing and building. Always compile and test locally before concluding a task.

Project context:

- The main Go code is located in the `src/` directory. Run the above commands inside the `src/` directory (e.g., `cd src && go test ./...`).
