# Axon Audit Design

## Overview

`axon audit` is an AI-powered security scanner that helps users detect sensitive information and security issues in their Hub content before sharing skills publicly.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      axon audit                              │
│                    (cmd/audit.go)                            │
└──────────────┬──────────────────────────────────────────────┘
               │
               ├─► Scanner (internal/audit/scanner.go)
               │   • File discovery and filtering
               │   • Extension-based filtering
               │   • Exclude pattern matching
               │   • Target resolution (skill/workflow/file)
               │
               ├─► Cache (internal/audit/cache.go)
               │   • Result persistence (~/.axon/audit-results/)
               │   • File change detection (SHA256 + mtime)
               │   • Cache validation
               │
               ├─► Auditor (internal/audit/auditor.go)
               │   • LLM prompt construction
               │   • Response parsing (JSON)
               │   • Finding extraction
               │
               ├─► LLM Provider (internal/llm/)
               │   • OpenAI-compatible API
               │   • Custom endpoints (Ollama, etc.)
               │   • Configuration loading
               │
               └─► Fixer (internal/audit/fixer.go)
                   • Interactive fix mode
                   • Redaction (replace with [REDACTED])
                   • Line deletion
                   • Statistics tracking
```

## Data Flow

### 1. Scan Flow

```
User runs: axon audit [target]
    ↓
Load config from ~/.axon/axon.yaml
    ↓
Load LLM config from ~/.axon/.env
    ↓
Scan files (entire Hub or target)
    ↓
Check cache (if --fix and not --force)
    ↓
For each file:
    Read content → Call LLM → Parse findings
    ↓
Save results to cache
    ↓
Display findings (grouped by severity)
```

### 2. Fix Flow

```
User runs: axon audit --fix
    ↓
Load cached results (if valid)
    ↓
For each finding:
    Display: file:line, description, snippet
    Prompt: [r/d/s/q]
    ↓
    r → Redact snippet with [REDACTED]
    d → Delete entire line
    s → Skip
    q → Quit (remaining = skipped)
    ↓
Display summary (total, redacted, deleted, skipped)
```

## Configuration

All configuration is in `~/.axon/.env` (no changes to `axon.yaml`):

```bash
AXON_AUDIT_PROVIDER=openai          # LLM provider
AXON_AUDIT_MODEL=gpt-4o-mini        # Model name
AXON_AUDIT_API_KEY=sk-...           # API key
AXON_AUDIT_BASE_URL=                # Optional: custom endpoint
AXON_AUDIT_ALLOWED_EXTENSIONS=...   # File extensions to scan
```

## Cache Strategy

### Cache Key

MD5 hash of: `target + sorted file list`

### Cache Structure

```json
{
  "target": "skills/humanizer",
  "timestamp": "2026-03-15T14:30:00Z",
  "llm_provider": "openai",
  "llm_model": "gpt-4o-mini",
  "files": [
    {
      "path": "/path/to/file.md",
      "mtime": 1234567890,
      "hash": "sha256..."
    }
  ],
  "findings": [...]
}
```

### Cache Validation

Cache is valid if:
1. All files in cache still exist
2. No new files added
3. All file mtimes match
4. All file hashes match

### Cache Location

`~/.axon/audit-results/<cache-key>.json`

## LLM Prompt Design

### System Prompt

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
- line_number (approximate line number, integer)
- issue_type (one of: "secret", "injection", "exfiltration", "pii")
- severity (one of: "high", "medium", "low")
- description (one sentence describing the issue)
- snippet (the problematic code/text, keep it short)

Output format: JSON array of findings. If no issues found, return empty array [].

IMPORTANT: Return ONLY the JSON array, no other text.
```

### Response Parsing

- Supports standard JSON arrays
- Handles markdown code blocks (```json ... ```)
- Validates required fields
- Skips invalid findings
- Graceful fallback on parse errors

## Finding Structure

```go
type Finding struct {
    FilePath    string `json:"file_path"`
    LineNumber  int    `json:"line_number"`
    IssueType   string `json:"issue_type"`   // "secret", "injection", "exfiltration", "pii"
    Severity    string `json:"severity"`     // "high", "medium", "low"
    Description string `json:"description"`
    Snippet     string `json:"snippet"`
}
```

## Interactive Fix Mode

### User Actions

- `r` — Redact: Replace snippet with `[REDACTED]`
- `d` — Delete: Remove entire line
- `s` — Skip: Leave unchanged
- `q` — Quit: Stop processing (remaining = skipped)

### Redaction Logic

1. Try exact match of snippet in file
2. If not found, try trimmed match
3. Replace first occurrence with `[REDACTED]`
4. Write back to file

### Deletion Logic

1. Validate line number
2. Split file into lines
3. Remove line at index (line_number - 1)
4. Join and write back

## Testing Strategy

### Unit Tests

- Scanner: file discovery, filtering, target resolution
- Cache: save/load, validation, file change detection
- Auditor: prompt construction, response parsing
- LLM: mock provider, API calls
- Fixer: redact, delete, skip, quit actions

### Integration Tests

- End-to-end audit flow
- Cache hit/miss scenarios
- Fix mode with mock input

### Manual Testing

- Real LLM providers (OpenAI, Ollama)
- Various file types and content
- Edge cases (large files, binary files, etc.)

## Future Enhancements

1. **Anthropic provider** — Add Claude support
2. **Ollama provider** — Document local model setup
3. **Batch optimization** — Send multiple small files in one LLM call
4. **Cache expiration** — Add TTL or manual cache clearing
5. **Custom rules** — User-defined security patterns
6. **CI integration** — Exit with non-zero code for high-severity findings

## Performance Considerations

- **Caching** reduces duplicate LLM calls (saves cost and time)
- **Progress indicator** shows scan progress every 10 files
- **File filtering** by extension reduces unnecessary scans
- **Exclude patterns** skip irrelevant files
- **Timeout** 60s per LLM call prevents hanging

## Security Considerations

- API keys stored in `~/.axon/.env` (mode 0600)
- Cache files stored in `~/.axon/audit-results/` (mode 0600)
- No secrets logged or printed to stdout
- User must explicitly approve fixes (interactive mode)
- Disclaimer about AI false positives/negatives

## Error Handling

- Graceful fallback if LLM not configured
- Continue scanning if individual file fails
- Parse errors → warning finding (not fatal)
- File read errors → skip with warning
- Invalid JSON → parse_error finding

## Output Format

Uses unified output helpers from `cmd/output.go`:

- `printSection()` — Section headers
- `printWarn()` — Findings
- `printInfo()` — Status messages
- `printOK()` — Success messages

Consistent with other axon commands.
