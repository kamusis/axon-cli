# Design Doc: `axon update` (CLI Self-Update)

## Context / Problem
Axon is intended to be “the central nervous system” for AI agent skills, so users need an easy way to stay on the latest CLI version. Today, updating requires manual downloads and installation.

Issue #6 requests a native `axon update` that:

- Fetches latest GitHub release metadata
- Detects platform (`runtime.GOOS`) and arch (`runtime.GOARCH`) and selects the correct release asset
- Downloads safely to a temp location
- Verifies checksum (if provided)
- Atomically replaces the current binary
- Verifies permissions + version

## Goals

- **One-command update**: `axon update` updates the installed `axon` binary to the latest GitHub release.
- **Safety-first**:
  - Download to temp
  - Verify checksum when available
  - Replace via atomic rename (with backup and rollback on failure)
- **Cross-platform**: Support Linux/macOS/Windows with correct asset selection.
- **Good UX**:
  - Clear progress messages
  - Detect “already up to date”
  - `--check` mode to only report availability

## Non-Goals

- Updating via package managers (`brew`, `apt`, `scoop`) (we can detect and warn later, but not required).
- Updating “skills” or the Hub repo (that’s already `axon sync`).
- Supporting arbitrary GitHub Enterprise instances (unless already in scope later).

## Current Codebase Notes (relevant constraints)

- CLI uses **Cobra** and subcommands live in `src/cmd/*.go`.
- There is **no existing version/build info surfaced** to users yet (no `axon -v/--version` flag).
- Commands tend to be self-contained in their file and register via `init()` with `rootCmd.AddCommand(...)`.

This impacts update verification: we should introduce a canonical “version string” injected at build time.

## Prerequisite: version awareness (`axon -v/--version`, `axon version`)

Reliable self-update requires the CLI to know and report its own version.

### Current status

The release pipeline already uses **GoReleaser** (triggered by `git tag` + push) and performs build-time injection via linker flags.

In `src/.goreleaser.yaml`:

```yaml
ldflags:
  - -s -w \
    -X github.com/kamusis/axon-cli/cmd.version={{.Version}} \
    -X github.com/kamusis/axon-cli/cmd.commit={{.Commit}} \
    -X github.com/kamusis/axon-cli/cmd.buildDate={{.Date}}
```

This means the version value (typically the git tag, e.g. `v0.3.1`) is embedded into the binary **at build time**, when GoReleaser runs `go build` during the GitHub Actions release workflow.

### Goal

Add version awareness via:

- A **global** `-v/--version` flag for a fast, script-friendly version check.
- An `axon version` subcommand that can print richer build/runtime details.

This capability is the primary foundation for `axon update` because it enables:

- Comparing the installed version vs the latest release tag before downloading.
- Post-update verification by running the new binary and checking its reported version.
- Clear bug reports and diagnostics (users can paste version output).

Example output for `axon version`:

```bash
$ axon version
Version:    v0.3.1
Commit:     abc123
Build Date: 2024-01-15
Go Version: go1.21.0
OS/Arch:    linux/amd64
```

## Proposed UX / CLI Interface

### Command

- `axon update`
  - Default behavior: update to **latest** GitHub release for current platform.

### Flags

- `--check`: only check if an update is available; don’t download/install.
- `--dry-run`: resolve release + asset and print what would happen (no download).
- `--repo owner/name`: override default GitHub repo (default: `kamusis/axon-cli`).
- `--prerelease`: allow prereleases (default false).
- `--force`: reinstall even if versions match.
- `--timeout`: overall timeout budget (default: 30s). Used to bound GitHub API calls, downloads, lock acquisition waiting, and Windows helper waiting to avoid indefinite hangs.
- `--verbose`: print additional debug details (URLs, paths).

## Release Asset Convention (Contract)

Axon release artifacts are produced by GoReleaser. The current archive naming convention is:

- Archive asset:
  - `axon_<version>_<os>_<arch>.tar.gz` (Linux/macOS)
  - `axon_<version>_<os>_<arch>.zip` (Windows)
- Contained binary:
  - `axon` (or `axon.exe` on Windows)
- Checksum asset:
  - `checksums.txt` (GoReleaser manifest)

Note on tags vs filenames:

- GitHub release tags are typically `vX.Y.Z`.
- GoReleaser archives typically use `X.Y.Z` in filenames.
- `axon update` should normalise `vX.Y.Z` → `X.Y.Z` when matching assets and verifying versions.

## Technical Design

### 1. Version/Build Metadata

Introduce build-time variables in the `cmd` package so that GoReleaser can inject values using linker flags (current config injects `cmd.version`, `cmd.commit`, and `cmd.buildDate`).

- `version` (semantic version like `v0.3.1`, default `dev`)
- `commit` (git SHA, default empty; optional)
- `buildDate` (ISO8601, default empty; optional)

Additional runtime-reported fields (not injected at build time):

- `GoVersion` (from `runtime.Version()`)
- `OS/Arch` (from `runtime.GOOS` / `runtime.GOARCH`)

This enables:

- `axon update` to compare current vs latest
- Post-update verification by calling the new binary with `axon -v/--version` (or parsing `axon version` output)

Also add:

- Global `axon -v/--version` flag that prints a minimal version string.
- `axon version` subcommand that prints rich build/runtime info.

### 2. GitHub Release Fetch

Use GitHub REST API:

- `GET https://api.github.com/repos/{owner}/{repo}/releases/latest`
- If `--prerelease`, call:
  - `GET https://api.github.com/repos/{owner}/{repo}/releases` and choose first non-draft (optionally include prerelease).

Implementation approach:

- Use Go `net/http` + `encoding/json`.
- Use `--timeout` to bound API calls (e.g., via `context.WithTimeout` per request or an `http.Client{Timeout: ...}`).
- Respect rate limiting:
  - Add `User-Agent: axon-cli`.
  - If a GitHub API token is provided via environment variable (recommend `AXON_GITHUB_TOKEN`, with `GITHUB_TOKEN` as a fallback), use it to authenticate and reduce rate-limit risk.
    - Authenticated requests increase GitHub API rate limits even when accessing a public repo.
    - The token must be provided by the user/environment at runtime and must never be embedded in source code or release artifacts.

Data model (minimal):

- `tag_name`
- `assets[] { name, browser_download_url, size }`

### 3. Platform/Arch Asset Selection

Compute expected asset prefix:

- `goos := runtime.GOOS`
- `goarch := runtime.GOARCH`

Expected asset filename patterns:

- `axon_<version>_<goos>_<goarch>.tar.gz`
- `axon_<version>_<goos>_<goarch>.zip` (windows)

Selection rules:

- Prefer exact match (case-sensitive).
- If multiple matches, prefer archive formats in a configured preference list:
  - Windows: `.zip` then `.tar.gz`
  - Others: `.tar.gz` then `.zip`

### 4. Download to Temporary Location

- Choose a temp base directory that is extremely likely to be writable:
  - Prefer `os.TempDir()` (respects `TMPDIR`/platform defaults).
  - If creating a temp dir under `os.TempDir()` fails (permission/readonly/no space), fall back to a per-user writable directory:
    - Use `os.UserCacheDir()` and create `filepath.Join(cacheDir, "axon", "tmp")`.
    - If `os.UserCacheDir()` is unavailable, fall back to `~/.axon/tmp`.
  - In all cases, validate by attempting `os.MkdirAll(base, 0o755)` and creating a small probe file, then remove the probe.
- Create the working temp dir: `os.MkdirTemp(base, "axon-update-*")`.
- Download the release asset with strict HTTP status checking and enforce `--timeout` (avoid hanging indefinitely on slow/broken networks).
- Stream to disk (avoid loading into memory).
- **v1 requirement: progress display**
  - Track `bytesDownloaded` while copying response body to the file.
  - If `Content-Length` is available, compute percentage and show a single-line updating progress indicator.
  - If `Content-Length` is missing, still show downloaded bytes (and optionally an approximate rate).

### 5. Checksum Verification (if provided)

Axon releases built by GoReleaser already publish a checksum manifest asset:

- `checksums.txt` (example: `https://github.com/kamusis/axon-cli/releases/download/v0.1.9/checksums.txt`)

`axon update` should use this manifest as the primary checksum source.

Support two common release patterns:

1. **Sidecar checksum file**:
   - `${asset}.sha256` (contains hash possibly with filename)
2. **Checksums manifest**:
   - `checksums.txt` or `sha256sums.txt`

Implementation note (v1): `axon update` uses the `checksums.txt` manifest as the primary checksum mechanism.
Sidecar checksum files are optional and not required for v1.

Algorithm:

- Look for known checksum assets in the same release (prefer `checksums.txt`).
- If found, download the checksum file and parse for the target asset’s SHA256.
  - GoReleaser `checksums.txt` format is typically: `<sha256> <filename>` (whitespace-separated).
  - Parse line-by-line, ignore empty lines, and match by exact filename (the release asset name).
- Compute SHA256 of the downloaded archive and compare.
- If checksum asset not present:
  - Warn and continue (unless `--require-checksum` is added later; not required by issue).

### 6. Extract the Binary

Based on archive type:

- `.tar.gz`: use `archive/tar` + `compress/gzip`
- `.zip`: use `archive/zip`

Extraction rules:

- Only extract the binary file (`axon` / `axon.exe`).
- Do not write arbitrary paths from archive (prevent ZipSlip / Tar path traversal):
  - Reject entries with `..` or absolute paths.
- Write extracted binary to temp path like `${tmp}/axon.new`.

### 7. Atomic Replace with Backup + Rollback

We need to replace the currently-running binary safely.

Determine current executable path:

- `currentPath, err := os.Executable()`
- Resolve symlinks: `filepath.EvalSymlinks(currentPath)` (platform nuance)

Steps:

1. Preconditions
   - The downloaded archive has been verified (checksum if available).
   - The extracted candidate binary exists at `newPath` (e.g. `${tmp}/axon.new`).

2. Ensure extracted binary has executable permissions
   - On unix: `chmod 0755`
   - On windows: skip chmod

3. Backup current binary
   - `backupPath := currentPath + ".bak"`
   - Best-effort remove any existing stale backup (or overwrite it) to avoid rename failures.

4. Swap (two renames)
   - Rename `currentPath` -> `backupPath`
     - If this fails, abort (nothing has been replaced yet).
   - Rename `newPath` -> `currentPath`
     - If this fails, attempt immediate rollback:
       - Rename `backupPath` -> `currentPath`
       - If rollback fails, return an error indicating manual recovery is required.

5. Post-update verification
   - Execute `currentPath -v` (or `currentPath version`) and ensure it reports the expected release version.
   - If verification fails:
     - Attempt rollback:
       - Rename `currentPath` aside (best-effort) and restore `backupPath` -> `currentPath`.
     - Return an error.

6. Cleanup
   - Only after successful verification, delete `backupPath`.
   - If cleanup fails, warn but do not treat as a fatal error (the update itself succeeded).

### 7b. Concurrency control (file lock)

To prevent conflicting replacements/rollbacks when multiple `axon update` processes run concurrently, use a per-user OS-level file lock.

- Lock location (per-user, writable):
  - Prefer `os.UserCacheDir()/axon/update.lock`.
  - Fallback: `~/.axon/update.lock`.
- Lock scope:
  - Hold the lock for the entire update critical section (from “decide to update” through staging, swap, verification, and cleanup).
  - The Windows helper swap command must also acquire the same lock before touching `axon.exe`, `axon.new.exe`, or backups.
- Behavior if lock cannot be acquired:
  - Exit with a clear message (another update is in progress) and suggest retrying.
  - Optional: support a bounded wait/timeout before failing.

**Windows note**: Renaming the currently-running `.exe` often fails due to file locks.

- Windows strategy (v1): internal helper subcommand/process
  - Stage the new binary as `axon.new.exe` next to the current `axon.exe`.
  - Spawn a detached helper process implemented inside Axon (e.g. a hidden `axon __selfupdate-swap` command).
    - The helper receives: parent PID, current path, new path, backup path.
    - It waits for the parent process to exit (with a timeout/backoff loop), then performs the swap.
  - Swap procedure (best-effort, rollback-capable):
    - Rename `axon.exe` -> `axon.exe.bak`.
    - Rename `axon.new.exe` -> `axon.exe`.
    - Ensure the new binary is executable.
    - Optionally run `axon.exe -v` (or `axon version`) to verify the expected version; if verification fails, restore the backup.
  - UX: `axon update` should print a clear message that the update has been staged and will complete after the process exits.

### 8. Error Handling / Messaging

Errors should include:

- Release fetch failure (HTTP status and body snippet)
- Asset not found (print expected name and list available assets)
- Checksum mismatch (expected vs actual)
- Extraction issues (unsupported archive, missing binary)
- Swap/permission errors

## Security Considerations

- **Checksum verification** when present reduces risk of corrupted downloads.
- Prevent path traversal on extract.
- Use HTTPS only (GitHub URLs are HTTPS).
- Avoid executing downloaded content except the final installed binary path.

## Alternatives Considered

- **Use `go install`**: not suitable for users installing a binary release; also requires Go toolchain.
- **Self-update library** (3rd party): increases dependencies; simple enough to implement directly.
- **Always download from “latest” URL** without API: loses checksum/metadata and is less robust.

## Implementation Plan (phased)

1. **Add build info + `axon -v/--version` + `axon version`**
   - Build-time variables (`version`, optional `commit`, optional `buildDate`)
   - Global `-v/--version` flag (minimal output)
   - `axon version` subcommand (rich output: commit/date + Go version + OS/Arch)
2. **Implement GitHub release client**
   - Fetch latest (and prerelease option)
   - Asset selection by platform/arch
3. **Implement download + checksum verify**
4. **Implement extract + atomic swap**
5. **Post-update verification + rollback**
6. **Tests**
   - Unit tests for:
     - asset selection
     - checksum parsing
     - safe extraction path handling
   - Integration-ish tests can be limited due to binary replacement complexity.

## Testing Strategy

- **Unit tests** (pure Go):
  - Asset selection given fake release JSON
  - Checksum parsing from `.sha256` and `checksums.txt`
  - Archive extraction security checks (reject `../evil`)
- **Manual tests**:
  - Build two local versions, publish test release (or mock server) and verify update works on Linux/macOS
  - Ensure rollback works when verification fails

## Rollout / Compatibility

- Backward compatible: new command only.
- Requires that release artifacts follow naming convention going forward.
- If users installed via package manager, self-update may diverge from package manager expectations; we can detect common cases later.

## Open Questions

1. **Target repo owner/name**: should it be hard-coded to `kamusis/axon-cli` or configurable only via flag?
2. **Windows behavior**:
   - Do you want v1 to fully support Windows swapping (helper process), or is “download + instructions” acceptable for now?
3. **Checksum format** in releases:
   - Are you planning to publish `.sha256` sidecars, a `checksums.txt`, or neither?
