# Issue #15 Implementation Plan

This plan adds vendor-style external subdirectory syncing to Axon by introducing config-driven repo/subdir mappings, syncing them through a cache outside the Hub, and mirroring plain files into `~/.axon/repo` without embedding nested Git repositories.

## Goal

Enable Axon to populate selected Hub paths from arbitrary GitHub repositories and subdirectories, while preserving the existing Hub model:

- `~/.axon/repo` remains the single canonical Git-backed Hub
- vendored content inside the Hub is plain files/directories only
- external Git metadata lives outside the Hub in an Axon-managed cache
- repeated syncs are deterministic and overwrite local Hub copies by design

## Confirmed Product Decisions

- **Overwrite strategy**: force overwrite, not merge
- **Version tracking**: configurable in `axon.yaml`, default ref is `main`
- **Mirror strategy**: prefer `rsync`; fall back to `rm -rf` then `cp -a` if `rsync` is unavailable

## Current Code Constraints

### Existing configuration model

`src/internal/config/config.go` currently models:

- `repo_path`
- `sync_mode`
- `upstream`
- `excludes`
- `targets`

There is no existing concept for "content sources" that feed the Hub from external repos.

### Existing Hub lifecycle

`axon init` creates or clones the Hub and optionally copies existing local skill directories into it.

### Existing sync behavior

`axon sync` treats the Hub as a normal Git repo and strips nested `.git` directories before `git add`. This makes submodules or nested clones inside `~/.axon/repo` incompatible with the current model.

## Proposed User-Facing Design

### New config block

Add a new top-level config block in `~/.axon/axon.yaml`:

```yaml
vendors:
  - name: skill-creator
    repo: https://github.com/anthropics/skills.git
    subdir: skills/skill-creator
    dest: skills/skill-creator
    ref: main
```

### Field semantics

- `name`: stable logical identifier used for logging and cache directory naming
- `repo`: remote Git repository URL
- `subdir`: source path inside the remote repo
- `dest`: destination path inside the Hub repo, relative to `repo_path`
- `ref`: branch, tag, or commit-ish to sync; defaults to `main` when omitted

### Recommended command shape

Add a new command family rather than overloading `axon sync` directly:

- `axon vendor sync`

Rationale:

- keeps Hub Git sync concerns separate from external content ingestion
- makes overwrite semantics explicit
- gives room for future commands like `axon vendor list` or `axon vendor status`

Optional follow-up aliases can be considered later, but the first implementation should keep the new behavior isolated.

## Proposed Internal Design

### Cache layout

Store external repos outside the Hub, for example:

```text
~/.axon/cache/vendors/<name>/
```

This path should not be committed to the Hub and should be owned entirely by Axon.

### Vendor sync flow

For each configured vendor entry:

1. Validate config fields (`name`, `repo`, `subdir`, `dest`)
2. Resolve cache path for the vendor entry
3. Clone the repo into cache if missing
4. Configure sparse checkout for `subdir`
5. Fetch latest refs from remote
6. Checkout the configured `ref` (default `main`)
7. Ensure the requested `subdir` exists in the checked out tree
8. Mirror `<cache>/<subdir>` into `<repo_path>/<dest>`
9. Print a concise result summary

### Mirror behavior

Preferred path:

```bash
rsync -a --delete <src>/ <dest>/
```

Fallback path:

1. remove existing destination
2. copy the source directory into place with `cp -a`

This should be implemented in a dedicated helper so command logic stays readable.

### Why sparse-checkout

Sparse checkout is the best fit because it:

- avoids checking out unrelated repo content
- still allows normal Git update flows in cache
- keeps the Hub free of nested Git state

## Implementation Breakdown

> **Legend**: `[NEW]` = new file &nbsp;|&nbsp; `[MODIFY]` = existing file to be modified
>
> **Note**: `src/internal/importer/` (files `importer.go` / `importer_test.go`) already exists, but it handles copying local skill directories into the Hub during `axon init`. It is **entirely unrelated** to this issue's vendor sync feature — do not reuse or modify it.

## Phase 1: Config model and parsing

### Files

| Tag        | File                            |
| ---------- | ------------------------------- |
| `[MODIFY]` | `src/internal/config/config.go` |

### Changes

- Add a `Vendor` struct in `src/internal/config/config.go`
- Extend `Config` with `Vendors []Vendor `yaml:"vendors,omitempty"``
- Ensure `Load()` handles missing `vendors` gracefully
- Keep `DefaultConfig()` backward-compatible by defaulting `vendors` to empty

### Validation expectations

Validation can initially live in the command layer instead of `Load()` to avoid making unrelated commands fail on partial config mistakes.

## Phase 2: Vendor command surface

### Files

| Tag        | File                                                                    |
| ---------- | ----------------------------------------------------------------------- |
| `[NEW]`    | `src/cmd/vendor.go` — Cobra parent command (`axon vendor`)              |
| `[NEW]`    | `src/cmd/vendor_sync.go` — `axon vendor sync` subcommand implementation |
| `[MODIFY]` | `src/cmd/root.go` — register the new `vendor` parent command            |

### Behavior

- load config
- fail clearly if no vendors are configured
- check required external tools (`git` always; `rsync` optionally)
- run each configured vendor entry in sequence
- stop on first hard failure in MVP

### Output style

Follow existing Axon command style:

- concise progress lines
- clear success/warn/error summaries
- enough detail to identify which vendor entry failed

## Phase 3: Cache management helpers

### Files

| Tag     | File                                                                                             |
| ------- | ------------------------------------------------------------------------------------------------ |
| `[NEW]` | `src/internal/vendor/cache.go` — cache path calculation, clone, sparse-checkout, fetch, checkout |

### Changes

Add vendor-specific helpers in a dedicated `src/internal/vendor/` package:

- compute cache root/path
- detect whether cache repo exists
- clone repo when missing
- set sparse-checkout cone mode
- fetch remote
- checkout requested ref
- resolve source path inside cache

### Git considerations

The implementation should avoid assuming only branches exist. `ref` should support:

- branch names
- tags
- commit SHAs

MVP can keep the checkout logic simple, but error messages should say which ref failed to resolve.

## Phase 4: Hub mirroring

### Files

| Tag     | File                                                                                                                  |
| ------- | --------------------------------------------------------------------------------------------------------------------- |
| `[NEW]` | `src/internal/vendor/mirror.go` — rsync-first mirror with rm+cp fallback and Hub-relative dest path safety validation |

### Changes

Add a mirror helper that:

- resolves absolute source and destination paths
- applies a **two-level directory creation rule** before mirroring:
  1. **Verify the immediate parent directory exists** (e.g. `skills/` for a `dest` of `skills/skill-creator`). If it does not exist, abort with a clear error — this likely means `axon init` was never run or the Hub directory was accidentally deleted. Do **not** auto-create it silently.
  2. **Auto-create only the leaf destination directory** (e.g. `skills/skill-creator/`). This is safe because the parent's existence confirms the Hub is properly initialized.
- prefers `rsync`
- falls back to remove-and-copy when `rsync` is missing
- guarantees the destination is an exact mirror of the vendored source

### Safety guardrails

The destination path must be validated as Hub-relative to prevent accidental writes outside `repo_path`.

At minimum:

- reject absolute `dest`
- clean and normalize `dest`
- reject `..` path traversal escaping the Hub root

## Phase 5: Documentation and examples

### README updates

Document:

- what `vendors` is for
- why nested repos/submodules are not used inside the Hub
- the new command and examples
- overwrite semantics
- `rsync` preference and fallback behavior

### Config example

Add one short example showing:

- one vendor entry from `anthropics-skills`
- one vendor entry from a different repo

## Error Handling Expectations

The first version should explicitly handle and message:

- invalid `vendors` entry
- Git clone/fetch/checkout failure
- sparse-checkout failure
- missing `subdir` in checked-out ref
- invalid destination path
- `rsync` unavailable (warn + fallback)
- fallback copy failure

## Testing Plan

## Unit-level coverage

### Files

| Tag        | File                                                                                                       |
| ---------- | ---------------------------------------------------------------------------------------------------------- |
| `[MODIFY]` | `src/internal/config/config_test.go` (if it exists) or `[NEW]` — config parsing with and without `vendors` |
| `[NEW]`    | `src/internal/vendor/cache_test.go` — cache path derivation and dest path validation                       |
| `[NEW]`    | `src/internal/vendor/mirror_test.go` — rsync/fallback strategy selection and mirror behavior               |

Add tests for:

- config parsing with and without `vendors`
- destination path validation
- cache path derivation
- mirror strategy selection logic

## Integration-style coverage

### Files

| Tag     | File                                                            |
| ------- | --------------------------------------------------------------- |
| `[NEW]` | `src/cmd/vendor_sync_test.go` — command-level integration tests |

Prefer command-level tests or focused helper tests for:

- syncing a local test repo subdirectory into a temp Hub path
- overwrite behavior when destination already exists
- fallback path when `rsync` is not available

If full Git integration tests are too heavy for the first pass, at minimum isolate file mirroring and path validation behind testable helpers.

## Open Design Choices to Resolve During Implementation

- **Command nesting**: `axon vendor sync` vs a flat `axon vendorsync`-style command. Recommendation: `axon vendor sync`.
- **Cache naming collisions**: if two entries share the same `name` but different repos, either reject duplicates or derive cache key from repo URL. Recommendation: reject duplicate names in MVP.
- **Ref defaulting location**: apply default `main` during execution rather than mutating config objects globally.
- **Execution mode**: sequential only in MVP. Parallel sync can be deferred.

## Out of Scope for MVP

- lock file / recorded commit SHA
- partial sync by vendor name
- dry-run mode
- merge-aware behavior
- automatic invocation from `axon sync`
- background refresh or scheduled vendor syncs

## Recommended Delivery Order

1. Add `vendors` config support
2. Add vendor command and validation
3. Add cache + sparse-checkout helpers
4. Add mirror helper with `rsync` fallback
5. Add tests
6. Update README and issue references

## Acceptance Checklist

- `axon.yaml` can define multiple external vendor entries
- `axon vendor sync` populates the expected Hub destinations
- vendored content contains no nested `.git` inside the Hub
- reruns are idempotent
- default `ref` is `main`
- `rsync` is used when available
- fallback copy path works when `rsync` is unavailable
- resulting Hub content remains compatible with existing `axon sync`, `link`, and `status`
