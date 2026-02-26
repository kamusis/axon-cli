# Design Document: Axon CLI

## 1. Architecture Overview

Axon CLI follows a **Hub-and-Spoke** model:

- **The Core**: Located at `~/.axon/`. This is the hidden directory where Axon maintains its own state.
- **The Hub**: A local Git repository managed at `~/.axon/repo/`. This directory contains the master copies of all skills, workflows, and commands.
- **The Spokes**: The original skill directories used by various AI editors (Windsurf, Antigravity, etc.), which are transformed into symbolic links pointing back to specific folders inside the Hub.

## 2. Technology Stack

- **Language**: Go (Golang) 1.22+
- **CLI Framework**: Cobra (industry standard for Go CLIs)
- **Config Format**: YAML (for readability and comments)
- **Version Control**: Executing `git` system commands (requires `git` to be installed on host).

## 3. Data Structures

### Configuration (`~/.axon/axon.yaml`)

```yaml
# Global configuration
repo_path: ~/.axon/repo
# sync_mode: read-write  # Default. Requires a personal remote repo with push access.
# sync_mode: read-only   # Pull-only from upstream. No personal repo needed.
upstream: https://github.com/kamusis/axon-hub.git # Public upstream (used in read-only mode)

# Global exclude patterns (applied during both `axon init` import and `axon sync`).
# Supports glob syntax. These filters are Axon-layer guards that work even if
# the user has not configured a .gitignore in the Hub repo.
excludes:
  - .DS_Store # macOS metadata
  - Thumbs.db # Windows thumbnail cache
  - "*.tmp"
  - "*.bak"
  - "*~" # Editor backup files (Emacs, Vim, etc.)
  - .idea/ # JetBrains IDE directories
  - .vscode/ # VS Code workspace settings

targets:
  - name: windsurf
    source: skills # Relative to repo_path, shared across all tools
    destination: ~/.codeium/windsurf/skills
    type: directory
  - name: antigravity
    source: skills
    destination: ~/.gemini/antigravity/global_skills
    type: directory
```

## 4. Core Commands

### `axon init [repo-url]`

`repo-url` is optional. Three initialization modes:

**Mode A — Local only (no `repo-url`):**

- Creates `~/.axon/` and the `repo_path` directory.
- Initializes a local-only Git repo (`git init`). A remote can be added later via `axon remote set <url>`.
- Sets `sync_mode: read-write` in `axon.yaml`.

**Mode B — Personal remote (e.g., `axon init git@github.com:user/skills.git`):**

- If the remote repo **exists and is non-empty**: clone it into `repo_path`.
- If the remote repo **is empty or does not exist yet**: init locally and set the remote origin; push on the next `axon sync`.
- Sets `sync_mode: read-write` in `axon.yaml`.

**Mode C — Public upstream, read-only (no personal repo, e.g., `axon init --upstream`):**

- Clones the default public upstream repo (compiled-in default URL, e.g., `https://github.com/owner/skills.git`) into `repo_path`.
- Sets `sync_mode: read-only` in `axon.yaml`. No personal remote is configured; `axon sync` will only pull, never push.
- Suitable for general users who want to consume the published skills without managing their own repo.

**Importing existing skills (Modes A & B only):**
After the Hub is set up, `axon init` scans all `destination` paths defined in `axon.yaml`. For each destination that is a real (non-symlinked) directory with content, it **copies** that content into the Hub (`repo_path/source/`). Skipped if the Hub path already has content (e.g., cloned from remote) to avoid overwriting remote data.

#### Conflict Resolution during `axon init`

When multiple sources (e.g., Windsurf and Antigravity) contain a file with the **same name**, `axon init` applies MD5-based content fingerprinting before writing to the Hub:

| Scenario                                       | Action                                                               |
| ---------------------------------------------- | -------------------------------------------------------------------- |
| MD5 **matches** — identical content            | Keep one copy; skip the duplicate silently.                          |
| MD5 **differs** — same name, different content | **Conflict-safe write**: preserve both versions without overwriting. |

**Conflict-safe write rules:**

- The **first** file encountered is written as-is: `oracle_expert.md`
- Each subsequent conflicting version is stored with a tagged suffix that encodes its source tool name:
  ```
  oracle_expert.conflict-<tool-name>.md
  ```
  Example: `oracle_expert.conflict-antigravity.md`
- The suffix uses a single hyphen (`-`) between `conflict` and the tool name to avoid ambiguity with multi-segment extensions (e.g., `oracle_expert.prompt.md`).
- No file is ever silently overwritten during the import phase.

**Post-import conflict report:**

After all sources are scanned, if any conflicts were detected, `axon init` prints a summary before exiting:

```
⚠  1 conflict(s) detected during import.
   All versions have been preserved in ~/.axon/repo/.
   Please review and resolve the following files manually:
     - oracle_expert.conflict-antigravity.md  ← conflicts with oracle_expert.md
```

If no conflicts exist, no conflict report is printed.

### `axon link [target-name | all]`

**Supported argument forms:**

- `axon link <name>` — link a single target by its `name` field in `axon.yaml` (e.g., `axon link windsurf`).
- `axon link all` — link all targets defined in `axon.yaml`.
- `axon link` (no argument) — equivalent to `axon link all`.

**Linking logic** — for each target being linked, inspect the destination path:

- **Already a symlink pointing to the correct Hub path**: no-op, print "already linked" and exit. (`axon link` is idempotent.)
- **Already a symlink pointing elsewhere**: remove the existing link and re-create it pointing to the Hub (no backup needed, as the original data is not here).
- **Real directory, non-empty**: back up to `~/.axon/backups/<target-name>_<YYYYMMDDHHMMSS>/`, then delete the original and create the symlink.
- **Real directory, empty**: delete it and create the symlink (no backup needed).
- **Does not exist**: create the symlink directly.
- Creates a symbolic link from `<repo_path>/<source>` to the destination.

### `axon sync`

Behavior depends on `sync_mode` in `axon.yaml`:

**Exclude filtering (both modes):**

Before any Git operation, Axon walks the Hub working tree and removes (from the staging area, not disk) any files matching the `excludes` glob patterns defined in `axon.yaml`. This is an Axon-layer guard that operates independently of `.gitignore`, ensuring junk files never reach a commit even if `.gitignore` is absent or incomplete.

**`read-write` (default):**

- Apply exclude filtering (see above)
- `git add .`
- `git commit -m "axon: sync from [hostname]"` (skipped if nothing to commit)
- `git pull --rebase origin master`
- `git push origin master`

**`read-only`:**

- `git pull origin master` (fast-forward only)
- No commit, no push. Local edits are allowed but will be overwritten on the next sync (user is warned).

### `axon status`

- Validates all symlinks.
- Shows Git status of the Hub.

### `axon doctor`

Runs a pre-flight environment check and reports any issues that would prevent Axon from working correctly. Intended to be the first command a user runs when something seems wrong, or before filing a bug report.

**Checks performed:**

| Check                                            | Pass condition                                                   |
| ------------------------------------------------ | ---------------------------------------------------------------- |
| `git` installed                                  | `git --version` exits 0                                          |
| Hub directory exists                             | `~/.axon/repo/` is present                                       |
| `axon.yaml` is valid                             | Parseable YAML with required fields                              |
| All symlinks healthy                             | Each `destination` is a symlink pointing to the correct Hub path |
| **Symlink creation permission** _(Windows only)_ | A temporary symlink can be created without error                 |

**Windows symlink permission check (detail):**

On native Windows (non-WSL), `axon doctor` attempts to create a throwaway symlink in a temp directory. If the call fails with an access-denied error, it prints an actionable remediation message:

```
✗  Symlink creation failed — Developer Mode or Administrator rights required.
   To enable Developer Mode on Windows:
     Settings → System → For developers → Developer Mode → ON
   Alternatively, run Axon in an Administrator terminal.
   WSL users are not affected by this restriction.
```

If the check passes, the temp symlink is deleted immediately and a success line is printed.

## 5. Platform-Specific Strategies

- **Config Format**: All paths in `axon.yaml` are written in Unix style using `~` for the home directory (e.g., `~/.codeium/windsurf/skills`). The CLI expands `~` at runtime using `os.UserHomeDir()`, which works correctly on all platforms. Users write one config that works everywhere.
- **Symlink Creation**: On macOS/Linux, use `os.Symlink()`. On native Windows, use `mklink /D` (requires Developer Mode or Admin rights); detect WSL vs. native at runtime. Run `axon doctor` to verify symlink permission before using `axon link` on Windows.
- **Path Resolution**: Convert all paths to absolute form internally to avoid ambiguity across platforms.

## 6. Security

- Axon will not store Git credentials; it relies on the host's existing `git-credential-helper` (SSH keys or GCM).
- **Two-layer junk-file defense:**
  - **Layer 1 — Axon `excludes`** (runtime): glob patterns in `axon.yaml` filter files before `git add`, regardless of `.gitignore` presence. This is the user-configurable, always-on guard.
  - **Layer 2 — `.gitignore`** (Git layer): a default `.gitignore` template is generated in the Hub repo on `axon init` to cover common patterns (`.DS_Store`, `__pycache__`, etc.). Complements Layer 1 for users who interact with the Hub repo directly via `git`.
