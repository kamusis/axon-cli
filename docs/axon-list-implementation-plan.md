# feat: `axon list` — list local items grouped by category

## Background

Issue #13 requests a lightweight `axon list` command that scans the local Hub repo and prints items grouped by the categories defined in `axon.yaml` (e.g. `skills`, `workflows`, `commands`). The command intentionally shows **only names** — no detail; that is the job of `axon inspect`.

Currently, `axon.yaml` has a `targets` slice where each target carries a `Source` field (e.g. `"skills"`, `"workflows"`, `"commands"`). The unique parent directories of all sources are the **categories**. Items within each category are the subdirectories of that source directory inside the Hub at `cfg.RepoPath`.

---

## Proposed Changes

### `cmd` package

#### [NEW] [list.go](file:///home/kamus/CascadeProjects/axon-cli/src/cmd/list.go)

New file implementing the `listCmd` cobra command.

**Logic (`runList`)**:

1. Load config with `config.Load()` — fail gracefully if axon is not initialised.
2. Derive the set of **unique source directories** from `cfg.Targets` (same algorithm as `uniqueSourceRoots` already used by `inspect.go`, but we'll write a local inline version or reuse the existing helper).
3. For each unique source (= category):
   - Category label = last path segment of the source (e.g. `"skills"`, `"workflows"`, `"commands"`).
   - Scan `filepath.Join(cfg.RepoPath, source)` for **all immediate children** — both subdirectories and files (skip hidden entries like `.git`). **Do not recurse.**
   - Each category may have a different structure: `skills` typically contains subdirectories, while `workflows`, `commands`, or `rules` may contain flat markdown files. The command lists whatever is there, one level only.
   - Collect child names (dirs and files alike) as the items for that category.
4. **Output**:
   - Print category label (plain, no prefix).
   - Print each item name indented two spaces (one per line).
   - If a category has no items, still print the category header with `  (empty)` — consistent, predictable behaviour.
   - Categories are printed in the order they first appear in `cfg.Targets`.

**Example output** (matching issue spec):

```text
skill
  foo
  bar
workflow
  release
  access-database
commands
  doctor
```

**Cobra registration**:

```go
var listCmd = &cobra.Command{
    Use:   "list",
    Short: "List local items grouped by category from axon.yaml",
    Long:  `...`,
    Args:  cobra.NoArgs,
    RunE:  runList,
}

func init() { rootCmd.AddCommand(listCmd) }
```

#### [NEW] [list_test.go](file:///home/kamus/CascadeProjects/axon-cli/src/cmd/list_test.go)

Unit tests for `runList` / the core logic. We follow the pattern established in `link_test.go` and `rollback_test.go`:

- Use `t.TempDir()` for an isolated Hub directory.
- Create a `*config.Config` directly (no file I/O, no `config.Load()`).
- Call the internal `listItems(cfg)` helper (extracted from `runList`) and assert on the returned `[]categoryItems` slice.

**Test cases:**

| Test                                   | Description                                                                                                    |
| -------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `TestListItems_DirsAndFiles`           | Category with mixed subdirs and files; assert both dirs and files appear by name.                              |
| `TestListItems_FlatCategory`           | Category containing only markdown files (no subdirs, simulating `workflows`); assert files are listed by name. |
| `TestListItems_EmptyCategory`          | Category source dir exists but has no children; assert zero items returned for that group.                     |
| `TestListItems_MissingSourceDir`       | Source dir does not exist on disk; assert the category still appears with zero items (graceful degradation).   |
| `TestListItems_DeduplicatesCategories` | Multiple targets share the same `Source`; category should appear only once.                                    |
| `TestListItems_SkipsHidden`            | Hidden entries (`.git`, `.DS_Store`, names starting with `.`) must not be listed.                              |

**Design decision**: Extract a pure `listItems(cfg *config.Config) []categoryItems` function (where `categoryItems` is a small struct `{Label string; Items []string}`) so it is directly testable without stdout capture.

---

### `docs/`

#### [NEW] docs/axon-list-implementation-plan.md

A copy of this plan exported to the project `docs/` directory for project-level traceability (same pattern as `docs/axon-rollback-implementation-plan.md`).

---

## Verification Plan

### Automated Tests

Run inside `src/`:

```bash
cd /home/kamus/CascadeProjects/axon-cli/src
go test ./...
go build ./...
```

Expected: all tests pass and the binary builds with no errors.

### Manual Smoke Test

After building (`go build -o /tmp/axon ./` inside `src/`):

```bash
/tmp/axon list
```

Expected: output lists categories from `~/.axon/axon.yaml` (e.g. `skills`, `workflows`, `commands`), each followed by subdirectory names found in `~/.axon/repo/`.
