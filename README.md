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

| Command                        | Description                                               |
| ------------------------------ | --------------------------------------------------------- |
| `axon init [repo-url]`         | Bootstrap the Hub; import existing skills                 |
| `axon link [name\|all]`        | Create symlinks from tool dirs to the Hub                 |
| `axon unlink [name\|all]`      | Remove symlinks; restore backups if available             |
| `axon sync`                    | Commit → pull → push (or pull-only in read-only mode)     |
| `axon remote set <url>`        | Set or update the Hub's git remote origin URL             |
| `axon status [skill-name]`     | Validate symlinks + Hub git status; or show skill history |
| `axon rollback <skill\|--all>` | Revert a skill or the entire Hub to a previous commit     |
| `axon audit [target]`          | Run AI-powered security audit on Hub content              |
| `axon doctor`                  | Pre-flight environment check                              |
| `axon list`                    | List local items grouped by category from axon.yaml       |
| `axon search <query>`          | Search skills/workflows/commands (keyword + semantic)     |
| `axon inspect <skill>`         | Show metadata and structure of a skill                    |
| `axon update`                  | Self-update axon to the latest GitHub release             |
| `axon vendor sync`             | Mirror external GitHub subdirs into the Hub               |
| `axon version`                 | Show detailed version/build/runtime info                  |

Global flags:

| Flag            | Description                 |
| --------------- | --------------------------- |
| `-v, --version` | Print axon version and exit |

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

Pass an optional `skill-name` to switch to **skill-level inspection mode** — shows the skill's resolved path, whether it is currently linked, and its recent commit history:

```bash
axon status humanizer
axon status humanizer --fetch   # also show remote ahead/behind count for this skill
```

```
=== Skill: humanizer ===
  Path:    /home/user/.axon/repo/skills/humanizer
  Linked:  ✓

● Recent commits:
  #1   abc1234  2026-03-05 14:20   axon: sync from mac-mini
  #2   def5678  2026-03-04 10:15   axon: sync from vps-1
```

### `axon rollback`

`axon rollback` reverts a skill directory or the entire Hub to a previous commit **without requiring any Git knowledge**. It always creates a new forward commit (never rewrites history), so `axon sync` can safely propagate the rollback to all your machines.

```bash
# Revert one skill to the previous commit
axon rollback humanizer

# Revert one skill to a specific commit
axon rollback humanizer --revision abc1234

# Revert the entire Hub one commit back
axon rollback --all

# Revert the entire Hub to a specific commit
axon rollback --all --revision abc1234
```

The command prints a summary before acting:

```
[ Rollback ]
  Skill:   humanizer
  Current: axon: sync from mac-mini (2026-03-05 14:20)
  Target:  axon: sync from vps-1   (2026-03-04 10:15)

  Reverting skills/humanizer... DONE
✓ Skill "humanizer" rolled back to def5678. Run 'axon sync' to propagate.
```

**Key behaviours:**

- Refuses to run if the Hub has uncommitted changes — commit or stash first.
- Use `axon status <skill-name>` to browse recent commits and find a target SHA before rolling back.
- Running `axon rollback` twice on the same target cancels out (the second run reverts the first). To go back multiple steps, use `--revision <sha>` to target a specific commit directly.
- After rolling back, run `axon sync` to push the revert commit to your remote and pull it on other machines.

Flags:

| Flag               | Description                                                              |
| ------------------ | ------------------------------------------------------------------------ |
| `--all`            | Revert the entire Hub (mutually exclusive with a skill name)             |
| `--revision <sha>` | Target a specific Git SHA, tag, or branch instead of the previous commit |

### `axon audit` — Security Audit

Scan your Hub content for security issues before sharing skills publicly. Uses AI-powered analysis to detect:

- **Hardcoded secrets** — API keys, passwords, tokens, private keys
- **Suspicious execution patterns** — shell injection, eval/exec, command substitution
- **Data exfiltration** — unexpected curl/wget, outbound network calls
- **PII** — emails, phone numbers, addresses in shared content

```bash
axon audit                  # scan entire Hub
axon audit humanizer        # scan a single skill
axon audit workflow.md      # scan a single file
axon audit --fix            # interactive redaction mode
axon audit --force          # force re-scan, ignore cache
```

**Configuration** (in `~/.axon/.env`):

```bash
AXON_AUDIT_PROVIDER=openai
AXON_AUDIT_MODEL=gpt-4o-mini
AXON_AUDIT_API_KEY=sk-...
AXON_AUDIT_BASE_URL=                                    # optional: for Ollama or custom endpoints
AXON_AUDIT_ALLOWED_EXTENSIONS=.md,.sh,.py,.js,.ts,.yaml,.yml  # comma-separated list
```

**Example output:**

```text
=== Security Audit: audit-fixture-risky-skill ===

  ⚠  AI-powered analysis may produce false positives or miss issues.
      All findings should be manually reviewed before taking action.

  Scanning 4 file(s)...


  SECURITY AUDIT REPORT
  ═══════════════════════════════════════════════
  Target: audit-fixture-risky-skill    Files: 4
  ───────────────────────────────────────────────
  RED FLAGS FOUND: 2

  • [EXTREME]  (L13) scripts/publish.sh
    Use of eval with potentially untrusted input can lead to code injection.
    "eval "$CMD" >/dev/null || true"

  • [HIGH]     (L12) scripts/report.py
    Hardcoded API key detected
    "api_key="sk-test-1234567890abcdef1234567890abcdef""

  ───────────────────────────────────────────────
  PERMISSIONS REQUIRED (estimated):
  • File Reads  : ~/.ssh/config
  • File Writes : output.txt
  • Network     : https://example.com/upload
  • Commands    : curl, eval, subprocess.run
  ───────────────────────────────────────────────
  RISK LEVEL: EXTREME
  VERDICT   : ⛔ DO NOT RUN
  ═══════════════════════════════════════════════

  2 potential issue(s) found. Review manually or run 'axon audit --fix'.
```

**Interactive fix mode** (`--fix`):

For each finding, choose an action:
- `r` — Redact (replace with `[REDACTED]`)
- `d` — Delete the entire line
- `s` — Skip this finding
- `q` — Quit (stop processing)

**Caching:**

Audit results are cached in `~/.axon/audit-results/` to avoid duplicate LLM API calls. When using `--fix`, cached results are used if files haven't changed. Use `--force` to bypass cache and re-scan.

**Supported LLM providers:**

- OpenAI (including compatible APIs)
- Custom endpoints via `AXON_AUDIT_BASE_URL` (e.g., Ollama)

Flags:

| Flag      | Description                                |
| --------- | ------------------------------------------ |
| `--fix`   | Interactive redaction mode                 |
| `--force` | Force re-scan, ignore cache                |

### `axon inspect` — Skill Metadata

Quickly inspect a skill without navigating the file system:

```bash
axon inspect humanizer        # exact name
axon inspect git              # fuzzy match → shows git-pr-creator, git-release, github-issues
axon inspect windsurf-skills  # by target name
```

Parses `SKILL.md` frontmatter and shows: name, version, description, triggers, allowed tools, scripts, and declared dependencies (`requires.bins` / `requires.envs` with live availability check).

### `axon list` — Local Inventory

`axon list` provides a lightweight overview of all items currently in your local Hub repository, grouped by category (e.g., `skills`, `workflows`, `commands`, `rules`).

```bash
axon list
```

Categories are derived from the unique `source:` paths in your `axon.yaml`. For each category, Axon scans the immediate children and displays them with a minimal icon:

- `+` for directories (e.g., typical skill folders)
- `·` for files (e.g., flat markdown workflows or rules)

Example output:

```text
=== Local Inventory ===

● Skills
  +  algorithmic-art
  +  brainstorming

● Workflows
  ·  access-database.md
  ·  codebase-review.md

● Commands
  -  (empty)
```

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

### `axon vendor sync` — External Imports

`axon vendor sync` allows you to populate your Hub with selected content from external GitHub repositories without the complexity of Git submodules. It clones external repos to a persistent cache (`~/.axon/cache/vendors`), performs an efficient **sparse-checkout** of the requested subdirectory, and mirrors the plain files into your Hub.

This is perfect for importing curated skills or workflows from community repositories.

```bash
# Sync all vendors configured in axon.yaml
axon vendor sync
```

**How it works:**

- It uses a **force-overwrite** strategy: local changes in the Hub destination will be overwritten by the upstream source.
- Content in the Hub is just **plain files**; no `.git` metadata from the source is imported, keeping your Hub's own Git history clean.
- It calculates a manifest of the last-synced Git SHA for each vendor. If the SHA hasn't changed, it skips the mirror step.

#### Configuration Example

Add a `vendors:` section to your `~/.axon/axon.yaml`:

```yaml
vendors:
  - name: extra-skills
    repo: https://github.com/someuser/cool-skills.git
    subdir: skills/coding
    dest: skills/coding
    ref: main # optional, defaults to main
```

- `name`: Unique identifier for the vendor entry.
- `repo`: The Git URL of the external repository.
- `subdir`: The directory inside the external repo you want to import.
- `dest`: The destination path relative to your Hub root (`~/.axon/repo/`).
- `ref`: (Optional) The Git branch, tag, or SHA to pin to.

## Configuration

`~/.axon/axon.yaml` is generated automatically by `axon init`. It contains your Hub path and pre-configured targets for various AI tools. You can manually edit it to add or remove targets, or to configure **external sources** (vendors).

```yaml
repo_path: ~/.axon/repo
sync_mode: read-write
upstream: https://github.com/kamusis/axon-hub.git

# ... (excludes section)

targets:
  # ... (pre-configured tool targets)
  - name: windsurf-skills
    source: skills
    destination: ~/.codeium/windsurf/skills
    type: directory
  # ... other targets

# === USER ADDED: External Sources (Optional) ===
# These are synced via `axon vendor sync`
vendors:
  - name: community-skill
    repo: https://github.com/kamusis/axon-hub.git
    subdir: skills/community-skill
    dest: skills/community-skill
    ref: main
```

## Prerequisites

| Dependency | Required | Notes                                                                                                                                    |
| ---------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| **git**    | ✅ Hard  | Must be installed and on `PATH`. Axon uses `git` for all Hub operations (`init`, `link`, `sync`, `remote`). Run `axon doctor` to verify. |
| **rsync**  | 💡 Soft  | Highly recommended for `axon vendor sync`. If missing, Axon falls back to `rm` + `cp`.                                                   |

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
