# Axon CLI

**Hub-and-Spoke skill manager for AI editors/CLIs/agents.**

Axon keeps your AI-editor/CLI/agent skills, workflows, and commands in sync across all your machines using a single Git-backed Hub at `~/.axon/repo/`. One command to rule them all — `axon sync`.

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
| `axon inspect <skill>`    | Show metadata and structure of a skill                |
| `axon update`             | Self-update axon to the latest GitHub release         |
| `axon version`            | Show detailed version/build/runtime info              |

Global flags:

| Flag            | Description                         |
| --------------- | ----------------------------------- |
| `-v, --version` | Print axon version and exit         |

### `axon status --fetch`

`axon status` shows symlink health and the Hub repo's local git status.

Add `--fetch` to also fetch `origin` and show whether your local Hub branch is ahead/behind the remote default branch. If the remote is newer, run `axon sync` to pull updates.

If `origin/HEAD` is missing, re-run `axon remote set <url>` to initialize the remote default branch reference.

### `axon init` — Three Modes

| Mode                  | Command                                   | Effect                              |
| --------------------- | ----------------------------------------- | ----------------------------------- |
| **A** Local only      | `axon init`                               | `git init` in Hub; add remote later |
| **B** Personal remote | `axon init git@github.com:you/skills.git` | Clone or init + set origin          |
| **C** Public upstream | `axon init --upstream`                    | Clone public hub, read-only         |

During init, Axon **safely imports** your existing skills:

- Files with the **same MD5** → one copy kept, duplicate skipped
- Files with the **same name but different content** → both preserved:
  `oracle_expert.md` + `oracle_expert.conflict-antigravity.md`

### `axon sync` — Two Modes

Configured via `sync_mode` in `~/.axon/axon.yaml`:

- **`read-write`** (default): `git add` → `git commit` → `git pull --rebase` → `git push`
- **`read-only`**: `git pull --ff-only` only; warns if local edits exist

Axon-layer exclude patterns (from `excludes:` in `axon.yaml`) are written to `.git/info/exclude` before every sync — junk files can never reach a commit even without a `.gitignore`.

To prevent cross-platform CRLF/LF churn, `axon init` also writes a default `.gitattributes` into the Hub repo (if missing): `* text=auto eol=lf`.

**Embedded `.git` auto-strip:** Skills downloaded via `git clone` often contain their own `.git` directory. Axon automatically detects and removes nested `.git` dirs before each `git add` so skills are committed as regular content, not as unresolvable submodules. Original skill files are never touched — only the `.git` metadata folder is stripped.

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

  # ... 20 targets pre-configured out of the box
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
| Windsurf    | ✓      | ✓         | ✓        |
| Antigravity | ✓      | ✓         |          |
| Cursor      | ✓      |           |          |
| OpenClaw    | ✓      | ✓         | ✓        |
| OpenCode    | ✓      |           |          |
| Neovate     | ✓      |           |          |
| Claude Code | ✓      |           | ✓        |
| Codex       | ✓      | ✓         | ✓        |
| Gemini      | ✓      | ✓         | ✓        |
| PearAI      | ✓      |           |          |

## `axon inspect` — Skill Metadata

Quickly inspect a skill without navigating the file system:

```bash
axon inspect humanizer        # exact name
axon inspect git              # fuzzy match → shows git-pr-creator, git-release, github-issues
axon inspect windsurf-skills  # by target name
```

Parses `SKILL.md` frontmatter and shows: name, version, description, triggers, allowed tools, scripts, and declared dependencies (`requires.bins` / `requires.envs` with live availability check).

---

## License

MIT
