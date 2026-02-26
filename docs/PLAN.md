# Development Plan: Axon CLI

## Phase 1: Foundation (The Skeleton)

- [ ] Set up Go project structure with Cobra.
- [ ] Implement `~/.axon` directory creation and `axon.yaml` config management:
  - Read/Write YAML config (including `repo_path`, `sync_mode`, `upstream`, `excludes`, `targets`).
  - `~` expansion via `os.UserHomeDir()` at runtime.
  - Generate a default `.gitignore` in the Hub repo on first init.
- [ ] Implement `axon status`:
  - Validate all symlinks (each `destination` points to the correct Hub path).
  - Show Git status of the Hub (`git status`).
- **Milestone**: CLI exists, can read/write its own config, and report environment state.

## Phase 2: Init Engine (The Bootstrap)

- [ ] Implement `axon init [repo-url]` — three modes:
  - **Mode A** (local only): `git init` in `repo_path`, set `sync_mode: read-write`.
  - **Mode B** (personal remote): clone if non-empty remote, else init locally + set origin.
  - **Mode C** (`--upstream`): clone public upstream, set `sync_mode: read-only`.
- [ ] Implement existing-skill import (Modes A & B only):
  - Scan each `destination` path; copy real (non-symlinked) directories into Hub.
  - Skip if Hub target path already has content (remote-clone guard).
  - **Exclude filtering**: skip files matching global `excludes` patterns during copy.
- [ ] Implement MD5 conflict resolution during import:
  - Identical MD5 → keep one copy, skip duplicate silently.
  - Differing MD5 → conflict-safe write: first file as `name.ext`, subsequent as `name.conflict-<tool>.ext`.
  - Print post-import conflict report listing all preserved conflict files.
- **Milestone**: `axon init` bootstraps Hub, imports existing skills safely, and reports conflicts.

## Phase 3: Link Engine (The Hands)

- [ ] Implement robust path expansion (`~` and env vars) across platforms.
- [ ] Implement `axon link [target-name | all]`:
  - Already correct symlink → no-op (idempotent).
  - Wrong symlink → remove and re-create.
  - Non-empty real directory → backup to `~/.axon/backups/<name>_<YYYYMMDDHHMMSS>/`, delete, create symlink.
  - Empty real directory → delete and create symlink.
  - Does not exist → create symlink directly.
  - Cross-platform: `os.Symlink()` on macOS/Linux; detect WSL vs. native Windows at runtime.
- [ ] Implement `axon unlink` to safely restore original state from backup.
- **Milestone**: Successfully link Windsurf and Antigravity skills to the Hub on all target platforms.

## Phase 4: Git Integration (The Heart)

- [ ] Implement Git wrapper logic in Go (thin shell around `git` system commands).
- [ ] Implement `axon sync`:
  - **Exclude filtering** (both modes): walk Hub tree before `git add`, filter files matching `excludes` globs from staging (not disk).
  - **`read-write` mode**: filter → `git add .` → `git commit` → `git pull --rebase` → `git push`.
  - **`read-only` mode**: `git pull --ff-only`; warn user if local edits are present.
  - Basic merge conflict detection: stop and prompt human on conflict.
- **Milestone**: Changes on VPS automatically sync to MacBook through the CLI; junk files never reach a commit.

## Phase 5: Doctor & Distribution (The Polish)

- [ ] Implement `axon doctor` — pre-flight self-check:
  - `git` installed (`git --version` exits 0).
  - Hub directory (`~/.axon/repo/`) exists.
  - `axon.yaml` is valid and parseable.
  - All symlinks healthy.
  - **Windows only**: attempt throwaway symlink; on failure print remediation path (`Settings → System → For developers → Developer Mode → ON`) and admin-terminal alternative.
- [ ] Configure `GoReleaser` or custom build scripts:
  - Linux (amd64 / arm64)
  - macOS (darwin/amd64 / darwin/arm64)
  - Windows (amd64 .exe)
- [ ] Write `README.md` and installation one-liners.
- **Milestone**: Single binaries available for download across all platforms; `axon doctor` catches environment issues before they become user bugs.

## Future Considerations (V2)

- S3 / Cloud Storage backend for users who don't want Git.
- Per-target `excludes` overrides (in addition to the global list).
- `axon remote set <url>` subcommand to add/change Git remote post-init.
- Plugin system for Axon itself.
- UI Dashboard for visual management.
