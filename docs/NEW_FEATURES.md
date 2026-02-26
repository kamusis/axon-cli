# New Feature Design: Axon CLI Enhancements

This document outlines the design for upcoming high-value features inspired by industry best practices (ClawHub) and advanced AI agent management needs.

---

## 1. `axon inspect <target|skill>`
Provides a quick metadata summary and structure overview of a specific skill or target without requiring the user to navigate the file system manually.

### Logic
- **Scope**: Can inspect either a defined `target` (e.g., `windsurf`) or a specific `skill` folder inside the Hub.
- **Parsing**:
    - Locate the `SKILL.md` within the specified directory.
    - Extract YAML frontmatter (name, description, version, triggers).
    - Scan for a `scripts/` directory and list available executables.
- **Output**: A clean, formatted CLI table or summary block.

### Example Usage
```bash
axon inspect oracle-health-check
```

### Mock Output
```text
📦 Skill: oracle-health-check
Summary: Perform a health check on an Oracle 23ai database using SQLcl MCP.
Triggers: (数据库|oracle)(巡检|健康检查|状态)

Files:
  - SKILL.md (Instructions)
  - scripts/oracle_health_report.py (Executable)

Dependencies:
  - bin: sql (Found)
  - env: TNS_ADMIN (Set)
```

---

## 2. Enhanced `axon doctor`
Transforms the basic environment check into a comprehensive diagnostic and self-healing suite.

### New Diagnostic Modules
- **Binary Dependency Check**: Scans the `metadata.requires.bins` (if present in `SKILL.md`) of all linked skills and verifies if those binaries exist in the system `$PATH`.
- **Symlink Integrity Audit**: Verifies that every link at `destination` actually points to its corresponding `source` in the Hub. Identifies "broken" links or "hijacked" directories.
- **Git Health**: Checks for uncommitted changes, detached HEAD states, or divergent branches in the Hub repo.
- **Permission Sentinel**: Detailed check for write permissions at `destination` paths and symlink creation rights (critical for Windows).

### Actionable Remediation
If a check fails, `doctor` should provide a specific copy-pasteable command or a direct settings link to fix it (e.g., "Run `axon link <target> --force` to repair this broken link").

---

## 3. `axon search <query>` (Local Semantic Search)
Allows the user to find the right skill for a task using natural language, essential as the personal Hub grows to dozens or hundreds of skills.

### Implementation Phases
- **Phase 1: Simple Keywords (Regex)**: Search through `name` and `description` fields in all `SKILL.md` files in the Hub.
- **Phase 2: Local Vector Search**:
    - Use a lightweight, embedded vector store (e.g., **LanceDB** or **Bleve**).
    - Generate embeddings locally (using a small model or a one-time API call during indexing).
    - **Trigger**: Run `axon search --index` to update the local vector index after a `sync`.

### Example Usage
```bash
axon search "how do I analyze my database performance?"
```

---

## 4. `axon resolve` (Conflict Management)
A dedicated interactive tool to handle the `.conflict-<tool>` files generated during `axon init`.

### Logic
- List all files with the `.conflict-` suffix in the Hub.
- For each conflict, offer:
    1. **Keep Original**: Delete the conflict file.
    2. **Keep New**: Overwrite the original with the conflict file.
    3. **Diff**: Show a side-by-side diff of the two files.
    4. **Merge**: Open the user's default `$EDITOR` to merge manually.

---

## 5. Technical Considerations for Go Implementation
- **YAML Handling**: Use `gopkg.in/yaml.v3` for robust frontmatter extraction.
- **Glob Matching**: Use `path/filepath.Match` or a specialized globbing library for the `excludes` logic.
- **Local DB**: If implementing semantic search, look for Go-native embedded DBs to maintain the "Single Binary" requirement.
