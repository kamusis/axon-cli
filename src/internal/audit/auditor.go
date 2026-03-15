package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kamusis/axon-cli/internal/llm"
)

// AuditFile analyzes a file for security issues using an LLM provider.
func AuditFile(ctx context.Context, provider llm.Provider, filePath string, content string) ([]Finding, error) {
	if provider == nil {
		return nil, fmt.Errorf("LLM provider is nil")
	}

	// Construct prompt
	prompt := buildAuditPrompt(filePath, content)

	// Call LLM
	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := provider.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	findings, err := parseAuditResponse(resp.Content, filePath)
	if err != nil {
		// If parsing fails, return a warning finding instead of failing completely
		return []Finding{
			{
				FilePath:    filePath,
				LineNumber:  0,
				IssueType:   "parse_error",
				Severity:    "low",
				Description: fmt.Sprintf("Failed to parse LLM response: %v", err),
				Snippet:     resp.Content[:min(100, len(resp.Content))],
			},
		}, nil
	}

	return findings, nil
}

// buildAuditPrompt constructs the security audit prompt.
func buildAuditPrompt(filePath, content string) string {
	return fmt.Sprintf(`You are a security auditor. Analyze the following file for security issues:

File: %s
---
%s
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
Example:
[
  {
    "line_number": 23,
    "issue_type": "secret",
    "severity": "high",
    "description": "Hardcoded API key detected",
    "snippet": "API_KEY=sk-1234567890abcdef"
  }
]

IMPORTANT: Return ONLY the JSON array, no other text.`, filePath, content)
}

// parseAuditResponse parses the LLM's JSON response into findings.
func parseAuditResponse(response, filePath string) ([]Finding, error) {
	// Clean response - remove markdown code blocks if present
	cleaned := strings.TrimSpace(response)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// Parse JSON
	var rawFindings []struct {
		LineNumber  int    `json:"line_number"`
		IssueType   string `json:"issue_type"`
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Snippet     string `json:"snippet"`
	}

	if err := json.Unmarshal([]byte(cleaned), &rawFindings); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Convert to Finding structs
	findings := make([]Finding, 0, len(rawFindings))
	for _, raw := range rawFindings {
		// Validate required fields
		if raw.IssueType == "" || raw.Severity == "" || raw.Description == "" {
			continue // Skip invalid findings
		}

		findings = append(findings, Finding{
			FilePath:    filePath,
			LineNumber:  raw.LineNumber,
			IssueType:   raw.IssueType,
			Severity:    raw.Severity,
			Description: raw.Description,
			Snippet:     raw.Snippet,
		})
	}

	return findings, nil
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

