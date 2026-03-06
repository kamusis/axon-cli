# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Test

All commands run from the `src/` directory:

```bash
cd src

# Build
go build -o axon .

# Run all tests
go test ./...

# Run tests for a specific package
go test ./cmd/...
go test ./internal/config/...
go test ./internal/search/...

# Run a single test
go test ./cmd/ -run TestRollback

# Install locally
go build -o axon . && sudo mv axon /usr/local/bin/
```

Releases are built via GoReleaser (triggered on `v*` tags via GitHub Actions).

## Architecture

**Axon** is a hub-and-spoke skill manager: a single Git-backed Hub at `~/.axon/repo/` stores `skills/`, `workflows/`, and `commands/` directories, and symlinks connect each AI editor's config dirs to the Hub.

### Code layout

```
src/
  main.go                    # entry point, calls cmd.Execute()
  cmd/                       # cobra subcommands (one file per command)
    root.go                  # rootCmd, -v/--version flag, Execute()
    git_utils.go             # shared git helpers (gitRun, gitOutput, gitIsDirty, resolveSkillPath, gitCommitInfo, gitLogEntries, gitCurrentSHA, etc.)
    output.go                # printOK / printWarn / printInfo / printSkip helpers
    init.go                  # axon init (bootstrap Hub, import existing skills)
    link.go / unlink.go      # symlink management + backup/restore
    sync.go                  # read-write and read-only sync modes
    status.go                # symlink health check + git status + per-skill status
    rollback.go              # skill-level and hub-wide rollback via git
    remote.go                # axon remote set
    doctor.go                # pre-flight env checks
    search.go                # keyword + semantic search frontend
    inspect.go               # skill metadata inspection
    update.go                # self-update from GitHub releases
    version.go               # version string
  internal/
    config/
      config.go              # Config struct, Load/Save, DefaultConfig, ExpandPath
      dotenv.go              # .env file loader for embeddings config
    importer/
      importer.go            # skill import logic (MD5 dedup, conflict naming)
    embeddings/
      provider.go            # embeddings provider interface
      openai.go              # OpenAI embeddings implementation
    search/
      keyword.go             # keyword (BM25-like) search
      skills.go              # skill document walker
      frontmatter.go         # YAML frontmatter parser for skill files
      index/                 # semantic index (build, load, vector storage)
```

### Key design patterns

**Config** (`internal/config/Config`): loaded from `~/.axon/axon.yaml` at the start of most commands. `RepoPath` is always `~`-expanded at load time. `Targets` drive both symlink management and `EffectiveSearchRoots()`.

**Git operations**: all git calls go through helpers in `cmd/git_utils.go`. `gitRun()` streams output; `gitOutput()` captures it. Commands call `checkGitAvailable()` before touching git.

**Sync flow** (`cmd/sync.go`):
- Writes excludes to `.git/info/exclude` (not `.gitignore`)
- Strips nested `.git` dirs from skills before `git add`
- `read-write`: add → commit → pull `--rebase -X theirs` → push (fallback to merge)
- `read-only`: pull `--ff-only` only

**Rollback** (`cmd/rollback.go`): refuses to run on a dirty repo. Both modes create a **forward commit** (never rewrite history) so `axon sync` can propagate the rollback safely. Skill rollback uses `git checkout <sha> -- <path>` + `git commit`; hub-wide uses `git revert --no-commit <range>` + `git commit` (squashed into one commit). `resolveSkillPath` in `git_utils.go` resolves shorthand skill names (e.g. `"humanizer"` → `"skills/humanizer"`).

**Search**: two modes — keyword (offline) and semantic (requires embeddings config via env or `~/.axon/.env`). Semantic index stored in `~/.axon/search/`. `axon search` tries semantic first and falls back to keyword.

**Output conventions**: use `printOK`, `printWarn`, `printInfo`, `printSkip` from `cmd/output.go` for consistent terminal output. Errors are returned (not printed) and surfaced by `Execute()`.

## Module

The Go module is `github.com/kamusis/axon-cli` (in `src/go.mod`). Key dependencies: `cobra` (CLI), `yaml.v3` (config), `flock` (file locking for self-update).
