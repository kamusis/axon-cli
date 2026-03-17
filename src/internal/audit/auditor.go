package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kamusis/axon-cli/internal/llm"
)

// AuditFile analyzes a single file for security issues and estimated permission scope
// using an LLM provider. Returns a FileAuditResult containing findings and permissions.
func AuditFile(ctx context.Context, provider llm.Provider, filePath string, content string) (FileAuditResult, error) {
	if provider == nil {
		return FileAuditResult{}, fmt.Errorf("LLM provider is nil")
	}

	// Construct prompt
	prompt := buildAuditPrompt(filePath, content)

	// Call LLM
	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := provider.Chat(ctx, messages)
	if err != nil {
		return FileAuditResult{}, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	result, err := parseAuditResponse(resp.Content, filePath)
	if err != nil {
		// If parsing fails, return a warning finding instead of failing completely
		return FileAuditResult{
			Findings: []Finding{
				{
					FilePath:    filePath,
					LineNumber:  0,
					IssueType:   "parse_error",
					Severity:    "low",
					Description: fmt.Sprintf("Failed to parse LLM response: %v", err),
					Snippet:     resp.Content[:min(100, len(resp.Content))],
				},
			},
		}, nil
	}

	return result, nil
}

// buildAuditPrompt constructs the security audit prompt with Agent-specific red flag rules.
func buildAuditPrompt(filePath, content string) string {
	return fmt.Sprintf(`You are a security auditor reviewing Agent skills and automation scripts. Analyze the following file for security issues.

File: %s
---
%s
---

## What to Detect

### Category A — Secrets & Credentials
- Hardcoded API keys, passwords, tokens, private keys, bearer tokens

### Category B — Dangerous Execution
- Shell injection, eval/exec with external or dynamic input
- base64 decode followed by code execution (obfuscation attempt)
- Dynamic code generation and execution patterns

### Category C — Data Exfiltration / Unauthorized Network
- curl/wget calls, especially to hardcoded IP addresses (not domains) — treat as RED FLAG
- Unexpected outbound HTTP/HTTPS calls to third-party servers
- Data being sent out without clear justification

### Category D — Unauthorized File System Access (RED FLAG)
- Reading sensitive paths: ~/.ssh, ~/.aws, ~/.config, /etc/passwd, /etc/shadow, ~/.gnupg
- Reading Agent memory or identity files: MEMORY.md, IDENTITY.md, SOUL.md, USER.md
- Modifying or deleting files outside the expected working directory

### Category E — Privilege Escalation (RED FLAG)
- Use of sudo, su, or doas
- Modifying /etc/sudoers, system cron jobs, or init scripts
- Requesting elevated OS permissions

### Category F — PII
- Plain-text emails, phone numbers, physical addresses in shared content

## Severity Levels
- "extreme"  — Zero-tolerance: unauthorized credential file access, identity file tampering, privilege escalation, IP-based exfiltration, obfuscated execution
- "high"     — Hardcoded secrets, shell injection, unexpected network requests to domains
- "medium"   — Potentially dangerous patterns needing context to confirm
- "low"      — Minor issues, informational warnings

## Output Format

Return a single JSON object with two keys: "findings" and "permissions".

"findings" is an array of issues, each with:
- line_number (integer, approximate)
- issue_type: one of "secret", "injection", "exfiltration", "pii", "privilege"
- severity: one of "extreme", "high", "medium", "low"
- description (one sentence)
- snippet (the problematic code/text, keep it short)

"permissions" summarizes what capabilities this code claims or exercises:
- file_reads: list of file paths the code reads (e.g., ["~/.ssh/config"])
- file_writes: list of file paths the code writes or deletes
- network: list of URLs, domains, or IPs contacted
- commands: list of external OS commands executed (e.g., ["curl", "eval"])

If a list is empty, use [].

Example output:
{
  "findings": [
    {
      "line_number": 23,
      "issue_type": "secret",
      "severity": "high",
      "description": "Hardcoded API key detected",
      "snippet": "API_KEY=sk-1234567890abcdef"
    }
  ],
  "permissions": {
    "file_reads": ["~/.aws/credentials"],
    "file_writes": [],
    "network": ["https://api.example.com"],
    "commands": ["curl"]
  }
}

IMPORTANT: Return ONLY the JSON object, no other text.`, filePath, content)
}

// parseAuditResponse parses the LLM's JSON response into a FileAuditResult.
// Accepts both the new {"findings":[], "permissions":{}} structure and the legacy bare array [].
func parseAuditResponse(response, filePath string) (FileAuditResult, error) {
	// Clean response — remove markdown code blocks if present
	cleaned := strings.TrimSpace(response)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// Try new structured format first: {"findings": [...], "permissions": {...}}
	if strings.HasPrefix(cleaned, "{") {
		return parseStructuredResponse(cleaned, filePath)
	}

	// Fall back to legacy bare array format: [...]
	return parseLegacyArrayResponse(cleaned, filePath)
}

// parseStructuredResponse parses the new {"findings", "permissions"} JSON structure.
func parseStructuredResponse(cleaned, filePath string) (FileAuditResult, error) {
	var raw struct {
		Findings []struct {
			LineNumber  int    `json:"line_number"`
			IssueType   string `json:"issue_type"`
			Severity    string `json:"severity"`
			Description string `json:"description"`
			Snippet     string `json:"snippet"`
		} `json:"findings"`
		Permissions struct {
			FileReads  []string `json:"file_reads"`
			FileWrites []string `json:"file_writes"`
			Network    []string `json:"network"`
			Commands   []string `json:"commands"`
		} `json:"permissions"`
	}

	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		return FileAuditResult{}, fmt.Errorf("failed to parse structured JSON: %w", err)
	}

	findings := make([]Finding, 0, len(raw.Findings))
	for _, f := range raw.Findings {
		if f.IssueType == "" || f.Severity == "" || f.Description == "" {
			continue // Skip malformed entries
		}
		findings = append(findings, Finding{
			FilePath:    filePath,
			LineNumber:  f.LineNumber,
			IssueType:   f.IssueType,
			Severity:    f.Severity,
			Description: f.Description,
			Snippet:     f.Snippet,
		})
	}

	return FileAuditResult{
		Findings: findings,
		Permissions: PermissionScope{
			FileReads:  normalizeList(raw.Permissions.FileReads),
			FileWrites: normalizeList(raw.Permissions.FileWrites),
			Network:    normalizeList(raw.Permissions.Network),
			Commands:   normalizeList(raw.Permissions.Commands),
		},
	}, nil
}

// parseLegacyArrayResponse handles the old bare JSON array format for backward compatibility.
func parseLegacyArrayResponse(cleaned, filePath string) (FileAuditResult, error) {
	var rawFindings []struct {
		LineNumber  int    `json:"line_number"`
		IssueType   string `json:"issue_type"`
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Snippet     string `json:"snippet"`
	}

	if err := json.Unmarshal([]byte(cleaned), &rawFindings); err != nil {
		return FileAuditResult{}, fmt.Errorf("failed to parse JSON: %w", err)
	}

	findings := make([]Finding, 0, len(rawFindings))
	for _, f := range rawFindings {
		if f.IssueType == "" || f.Severity == "" || f.Description == "" {
			continue
		}
		findings = append(findings, Finding{
			FilePath:    filePath,
			LineNumber:  f.LineNumber,
			IssueType:   f.IssueType,
			Severity:    f.Severity,
			Description: f.Description,
			Snippet:     f.Snippet,
		})
	}

	return FileAuditResult{Findings: findings}, nil
}

// normalizeList returns nil instead of nil for empty slices, keeping JSON output clean.
func normalizeList(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

// MergePermissions merges a PermissionScope into an existing aggregate scope.
func MergePermissions(agg *PermissionScope, p PermissionScope) {
	agg.FileReads = appendUnique(agg.FileReads, p.FileReads...)
	agg.FileWrites = appendUnique(agg.FileWrites, p.FileWrites...)
	agg.Network = appendUnique(agg.Network, p.Network...)
	agg.Commands = appendUnique(agg.Commands, p.Commands...)
}

// appendUnique appends items that are not already in the slice.
func appendUnique(existing []string, items ...string) []string {
	seen := make(map[string]struct{}, len(existing))
	for _, v := range existing {
		seen[v] = struct{}{}
	}
	for _, item := range items {
		if _, ok := seen[item]; !ok {
			existing = append(existing, item)
			seen[item] = struct{}{}
		}
	}
	return existing
}

// ComputeVerdict returns a verdict string based on the worst severity found.
func ComputeVerdict(findings []Finding) string {
	worst := "none"
	for _, f := range findings {
		switch f.Severity {
		case "extreme":
			return "⛔ DO NOT RUN"
		case "high":
			worst = "high"
		case "medium":
			if worst != "high" {
				worst = "medium"
			}
		case "low":
			if worst == "none" {
				worst = "low"
			}
		}
	}
	if worst == "high" {
		return "⚠ INSTALL WITH CAUTION"
	}
	return "✓ SAFE TO RUN"
}

// ComputeRiskLevel returns the overall risk level string based on findings.
func ComputeRiskLevel(findings []Finding) string {
	if len(findings) == 0 {
		return "NONE"
	}

	level := "LOW"
	for _, f := range findings {
		switch f.Severity {
		case "extreme":
			return "EXTREME"
		case "high":
			level = "HIGH"
		case "medium":
			if level != "HIGH" {
				level = "MEDIUM"
			}
		}
	}
	return level
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
