# Axon CLI

**Hub-and-Spoke skill manager for AI editors/CLIs/agents.**

Axon keeps your AI-editor/CLI/agent skills, workflows, and commands in sync across all your machines using a single Git-backed Hub at `~/.axon/repo/`. One command to rule them all — `axon sync`.

![axon-how-it-works](https://files.seeusercontent.com/2026/02/28/rnQ5/vscode_picgo_1772260135208.jpeg)

---

## How It Works

```
~/.axon/repo/          ← The Hub (local Git repo, optionally synced to remote)
  skills/
  workflows/
  commands/

~/.codeium/windsurf/skills     → symlink → Hub/skills/
~/.gemini/antigravity/...      → symlink → Hub/skills/
~/.cursor/skills               → symlink → Hub/skills/
... (20 targets out of the box)
```

## Quick Start

> **Prerequisite:** `git` must be installed and on your `PATH` before running any axon command.
>
> | OS            | Install                                                                                  |
> | ------------- | ---------------------------------------------------------------------------------------- |
> | macOS         | `brew install git` or Xcode Command Line Tools: `xcode-select --install`                 |
> | Ubuntu/Debian | `sudo apt install git`                                                                   |
> | Fedora/RHEL   | `sudo dnf install git`                                                                   |
> | Windows       | [git-scm.com/download/win](https://git-scm.com/download/win) or `winget install Git.Git` |

```bash
# 1. Download axon for your platform (see Releases)
# 2. Bootstrap a local Hub
axon init

# 3. Link all your AI tools to the Hub
axon link

# 4. Sync to a remote repo (optional — add a remote first)
axon sync
```

## Commands

| Command                   | Description                                           |
| ------------------------- | ----------------------------------------------------- |
| `axon init [repo-url]`    | Bootstrap the Hub; import existing skills             |
| `axon link [name\|all]`   | Create symlinks from tool dirs to the Hub             |
| `axon unlink [name\|all]` | Remove symlinks; restore backups if available         |
| `axon sync`               | Commit → pull → push (or pull-only in read-only mode) |
| `axon remote set <url>`   | Set or update the Hub's git remote origin URL         |
| `axon status [--fetch]`   | Validate symlinks + show Hub git status               |
| `axon doctor`             | Pre-flight environment check                          |
| `axon search <query>`     | Search skills/workflows/commands (keyword + semantic) |
| `axon inspect <skill>`    | Show metadata and structure of a skill                |
| `axon update`             | Self-update axon to the latest GitHub release         |
| `axon version`            | Show detailed version/build/runtime info              |

Global flags:

| Flag            | Description                         |
| --------------- | ----------------------------------- |
| `-v, --version` | Print axon version and exit         |

### `axon init` — Three Modes

| Mode                  | Command                                   | Effect                              |
| --------------------- | ----------------------------------------- | ----------------------------------- |
| **A** Local only      | `axon init`                               | `git init` in Hub; add remote later |
| **B** Personal remote | `axon init git@github.com:you/skills.git` | Clone or init + set origin          |
| **C** Public upstream | `axon init --upstream`                    | Clone public hub, read-only         |

Axon ships with a default public upstream Hub at:

`https://github.com/kamusis/axon-hub`

This upstream repo is intended as an optional starting point (a curated baseline of skills/workflows/commands). It is used by `axon init --upstream` (Mode C).

If you want to sync your Hub to **your own repo**, use `axon remote set <url>` to change the Hub's git `origin` remote (then run `axon sync`). Editing `upstream:` in `~/.axon/axon.yaml` only affects which repo `axon init --upstream` clones.

During init, Axon **safely imports** your existing skills:

- Files with the **same MD5** → one copy kept, duplicate skipped
- Files with the **same name but different content** → both preserved:
  `oracle_expert.md` + `oracle_expert.conflict-antigravity.md`

### `axon link` / `axon unlink`

`axon link` creates symlinks from each configured tool directory (the "spokes") to the Hub (`~/.axon/repo/`). This makes all supported AI tools read the same canonical `skills/`, `workflows/`, and `commands/` content.

If the destination path already exists as a **non-empty real directory**, Axon moves it aside first (backup) under `~/.axon/backups/<target>_<timestamp>/` and then creates the symlink. (Empty directories are removed and replaced with a symlink.)

`axon unlink` only removes destinations that are **symlinks** (it refuses to delete real directories/files). If a backup exists (created by `axon link`), Axon restores the **most recent** backup back to the original destination.

Common usage:

```bash
# Link everything configured in ~/.axon/axon.yaml
axon link

# Link a single target by name
axon link windsurf-skills

# Remove links (restores backups if available)
axon unlink

# Unlink a single target by name
axon unlink windsurf-skills
```

### `axon remote set <url>`

`axon remote set <url>` sets (or updates) the Hub repo's Git remote `origin` URL. If `origin` does not exist, it is added; otherwise, its URL is updated.

After setting the remote, Axon also does a best-effort `git fetch --prune origin` and attempts to auto-detect the remote default branch by running `git remote set-head origin -a` (used by commands like `axon status --fetch`).

Common usage:

```bash
# Set origin for the Hub (HTTPS)
axon remote set https://github.com/you/axon-hub.git

# Or SSH
axon remote set git@github.com:you/axon-hub.git

# Then sync to push your local Hub content
axon sync
```

### `axon sync` — Two Modes

Configured via `sync_mode` in `~/.axon/axon.yaml`:

- **`read-write`** (default): `git add` → `git commit` → `git pull --rebase` → `git push`
- **`read-only`**: `git pull --ff-only` only; warns if local edits exist

Note: when you run `axon init --upstream`, Axon writes `sync_mode: read-only` automatically (only when generating a fresh `axon.yaml`). If you later change `sync_mode` to `read-write` without switching `origin` to a repo you control, `axon sync` will typically fail at `git push` due to missing write permission. Use `axon remote set <url>` to point `origin` to your own repo before syncing in read-write mode.

Axon-layer exclude patterns (from `excludes:` in `axon.yaml`) are written to `.git/info/exclude` before every sync — junk files can never reach a commit even without a `.gitignore`.

To prevent cross-platform CRLF/LF churn, `axon init` also writes a default `.gitattributes` into the Hub repo (if missing): `* text=auto eol=lf`.

**Embedded `.git` auto-strip:** Skills downloaded via `git clone` often contain their own `.git` directory. Axon automatically detects and removes nested `.git` dirs before each `git add` so skills are committed as regular content, not as unresolvable submodules. Original skill files are never touched — only the `.git` metadata folder is stripped.

### `axon status`

`axon status` shows symlink health and the Hub repo's local git status.

Add `--fetch` to also fetch `origin` and show whether your local Hub branch is ahead/behind the remote default branch. If the remote is newer, run `axon sync` to pull updates.

If `origin/HEAD` is missing, re-run `axon remote set <url>` to initialize the remote default branch reference.

### `axon inspect` — Skill Metadata

Quickly inspect a skill without navigating the file system:

```bash
axon inspect humanizer        # exact name
axon inspect git              # fuzzy match → shows git-pr-creator, git-release, github-issues
axon inspect windsurf-skills  # by target name
```

Parses `SKILL.md` frontmatter and shows: name, version, description, triggers, allowed tools, scripts, and declared dependencies (`requires.bins` / `requires.envs` with live availability check).

### `axon search` — Keyword + Semantic

`axon search` searches documents in your Hub repo (by default: `skills/`, `workflows/`, `commands/`). It supports:

- Keyword search (offline)
- Semantic search (requires a local index + embeddings provider)

Default behavior:

- Axon tries semantic search first.
- If semantic search is unavailable, it falls back to keyword search.

Common usage:

```bash
# Search (semantic when available; otherwise keyword fallback)
axon search "database performance"

# Force keyword-only
axon search --keyword "git release"

# Force semantic-only (errors if semantic search cannot be performed)
axon search --semantic "postgres index"
```

#### Build / update the local semantic index

Semantic search needs a local index under `~/.axon/search/`.

```bash
# Build or update the local semantic index
axon search --index

# Rebuild even if no changes detected
axon search --index --force
```

 Indexing requires embeddings configuration. Axon resolves embeddings config from environment variables first, then `~/.axon/.env`:
 
 - `AXON_EMBEDDINGS_PROVIDER` (currently: `openai`)
 - `AXON_EMBEDDINGS_MODEL` (recommended: `text-embedding-3-small`)
 - `AXON_EMBEDDINGS_API_KEY`
 - `AXON_EMBEDDINGS_BASE_URL` (optional, default: `https://api.openai.com/v1`)
 
 Notes:

- The embeddings model must match the model used to build the index. If you change `AXON_EMBEDDINGS_MODEL`, rebuild the index.
- Use `--debug` to see which index directory was used (and semantic fallback reasons).

#### Flags

- `--index`: build/update `~/.axon/search/`
- `--keyword`: keyword search only
- `--semantic`: semantic search only (no fallback)
- `--k <int>`: number of results to show (default: `5`)
- `--min-score <float>`: minimum cosine similarity (semantic only). If not specified, Axon applies a default threshold unless `--k` is explicitly set.
- `--force`: force re-indexing (with `--index`)
- `--debug`: print debug information

### `axon update` — Self Update

`axon update` downloads the latest GitHub release for your platform, verifies its checksum (`checksums.txt`), and replaces the currently running binary (with rollback on failure).

Common usage:

```bash
axon update --check
axon update
```

Useful flags:

- `--check`: check if an update is available (no download/install)
- `--dry-run`: resolve release + asset without downloading
- `--timeout`: overall timeout budget (default 30s)
- `--force`: reinstall even if already on the latest version
- `--repo owner/name`: override the default repo (default: `kamusis/axon-cli`)

Optional environment variables (helpful for GitHub API rate limits in shared networks):

- `AXON_GITHUB_TOKEN` (preferred)
- `GITHUB_TOKEN` (fallback)

Bootstrap note:

- Self-update requires that the target release supports `-v/--version` for post-install verification.
- Self-update is supported starting from `v0.2.0`. If you are on an older release, upgrade manually once to `v0.2.0` (or newer) before using `axon update`.

## Configuration

`~/.axon/axon.yaml` is generated automatically by `axon init`. Edit it to add or remove targets.

```yaml
repo_path: ~/.axon/repo
sync_mode: read-write
upstream: https://github.com/kamusis/axon-hub.git

excludes:
  - .DS_Store
  - Thumbs.db
  - "*.tmp"
  - "*.log"
  # ... more patterns

targets:
  - name: windsurf-skills
    source: skills
    destination: ~/.codeium/windsurf/skills
    type: directory

  - name: antigravity-workflows
    source: workflows
    destination: ~/.gemini/antigravity/global_workflows
    type: directory

  # ... other targets pre-configured out of the box
```

## Prerequisites

| Dependency | Required | Notes                                                                                                                                    |
| ---------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| **git**    | ✅ Hard  | Must be installed and on `PATH`. Axon uses `git` for all Hub operations (`init`, `link`, `sync`, `remote`). Run `axon doctor` to verify. |

If `git` is not found, affected commands will exit immediately with a clear error message.

## Installation

### Homebrew (macOS/Linux) — coming soon

```bash
brew install kamusis/tap/axon
```

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/kamusis/axon-cli/releases) and place `axon` in your `$PATH`.

### Build from Source

```bash
git clone https://github.com/kamusis/axon-cli.git
cd axon-cli/src
go build -o axon .
sudo mv axon /usr/local/bin/
```

## Windows Notes

Creating symbolic links on Windows requires an Administrator terminal.

Run `axon doctor` to check your environment before running `axon link`.
WSL is fully supported without these restrictions.

## Supported AI Editors (Out of the Box)

| Tool        | Skills | Workflows | Commands |
| ----------- | ------ | --------- | -------- |
| Antigravity | ✓      | ✓         |          |
| Claude Code | ✓      |           | ✓        |
| Codex       | ✓      |           |          |
| Cursor      | ✓      |           |          |
| Gemini      | ✓      |           | ✓        |
| Neovate     | ✓      |           |          |
| OpenClaw    | ✓      |           |          |
| OpenCode    | ✓      |           |          |
| Qoder       | ✓      |           | ✓        |
| Trae        | ✓      |           |          |
| VSCode      | ✓      |           |          |
| Windsurf    | ✓      | ✓         |          |

---

## License

MIT
