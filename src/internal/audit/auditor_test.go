package audit

import (
	"context"
	"testing"

	"github.com/kamusis/axon-cli/internal/llm"
)

// mockProvider is a mock LLM provider for testing.
type mockProvider struct {
	response string
	err      error
}

func (m *mockProvider) Chat(ctx context.Context, messages []llm.Message) (*llm.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.Response{
		Content: m.response,
		Model:   "mock-model",
	}, nil
}

func (m *mockProvider) Name() string {
	return "mock"
}

// ── AuditFile tests ──────────────────────────────────────────────────────────

func TestAuditFile_Success(t *testing.T) {
	provider := &mockProvider{
		response: `{
			"findings": [
				{
					"line_number": 10,
					"issue_type": "secret",
					"severity": "high",
					"description": "Hardcoded API key detected",
					"snippet": "API_KEY=sk-1234567890"
				},
				{
					"line_number": 25,
					"issue_type": "injection",
					"severity": "medium",
					"description": "Potential shell injection",
					"snippet": "os.system(user_input)"
				}
			],
			"permissions": {
				"file_reads": [],
				"file_writes": [],
				"network": [],
				"commands": ["eval"]
			}
		}`,
	}

	result, err := AuditFile(context.Background(), provider, "test.py", "test content")
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}

	if len(result.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(result.Findings))
	}

	// Check first finding
	if result.Findings[0].LineNumber != 10 {
		t.Errorf("expected line 10, got %d", result.Findings[0].LineNumber)
	}
	if result.Findings[0].IssueType != "secret" {
		t.Errorf("expected type 'secret', got %s", result.Findings[0].IssueType)
	}
	if result.Findings[0].Severity != "high" {
		t.Errorf("expected severity 'high', got %s", result.Findings[0].Severity)
	}
	if result.Findings[0].FilePath != "test.py" {
		t.Errorf("expected file 'test.py', got %s", result.Findings[0].FilePath)
	}

	// Check second finding
	if result.Findings[1].IssueType != "injection" {
		t.Errorf("expected type 'injection', got %s", result.Findings[1].IssueType)
	}

	// Check permissions
	if len(result.Permissions.Commands) != 1 || result.Permissions.Commands[0] != "eval" {
		t.Errorf("expected commands [eval], got %v", result.Permissions.Commands)
	}
}

func TestAuditFile_NoFindings(t *testing.T) {
	provider := &mockProvider{
		response: `{"findings": [], "permissions": {"file_reads": [], "file_writes": [], "network": [], "commands": []}}`,
	}

	result, err := AuditFile(context.Background(), provider, "test.py", "clean content")
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}

	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestAuditFile_WithMarkdownCodeBlock(t *testing.T) {
	provider := &mockProvider{
		response: "```json\n{\"findings\": [], \"permissions\": {\"file_reads\": [], \"file_writes\": [], \"network\": [], \"commands\": []}}\n```",
	}

	result, err := AuditFile(context.Background(), provider, "test.py", "content")
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}

	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestAuditFile_InvalidJSON(t *testing.T) {
	provider := &mockProvider{
		response: "This is not JSON",
	}

	result, err := AuditFile(context.Background(), provider, "test.py", "content")
	if err != nil {
		t.Fatalf("AuditFile should not fail on parse error: %v", err)
	}

	// Should return a parse_error finding instead of failing
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 parse_error finding, got %d", len(result.Findings))
	}

	if result.Findings[0].IssueType != "parse_error" {
		t.Errorf("expected issue_type 'parse_error', got %s", result.Findings[0].IssueType)
	}
}

func TestAuditFile_NilProvider(t *testing.T) {
	_, err := AuditFile(context.Background(), nil, "test.py", "content")
	if err == nil {
		t.Error("expected error for nil provider")
	}
}

// ── parseAuditResponse tests ─────────────────────────────────────────────────

func TestParseAuditResponse_Valid(t *testing.T) {
	response := `{
		"findings": [
			{
				"line_number": 5,
				"issue_type": "pii",
				"severity": "medium",
				"description": "Email address detected",
				"snippet": "user@example.com"
			}
		],
		"permissions": {
			"file_reads": [],
			"file_writes": [],
			"network": [],
			"commands": []
		}
	}`

	result, err := parseAuditResponse(response, "test.md")
	if err != nil {
		t.Fatalf("parseAuditResponse failed: %v", err)
	}

	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}

	if result.Findings[0].IssueType != "pii" {
		t.Errorf("expected type 'pii', got %s", result.Findings[0].IssueType)
	}
}

func TestParseAuditResponse_SkipInvalid(t *testing.T) {
	response := `{
		"findings": [
			{
				"line_number": 5,
				"issue_type": "secret",
				"severity": "high",
				"description": "Valid finding",
				"snippet": "test"
			},
			{
				"line_number": 10,
				"issue_type": "",
				"severity": "high",
				"description": "Missing issue_type",
				"snippet": "test"
			},
			{
				"line_number": 15,
				"issue_type": "secret",
				"severity": "",
				"description": "Missing severity",
				"snippet": "test"
			}
		],
		"permissions": {"file_reads": [], "file_writes": [], "network": [], "commands": []}
	}`

	result, err := parseAuditResponse(response, "test.md")
	if err != nil {
		t.Fatalf("parseAuditResponse failed: %v", err)
	}

	// Should only include the valid finding
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 valid finding, got %d", len(result.Findings))
	}

	if result.Findings[0].Description != "Valid finding" {
		t.Errorf("unexpected finding: %s", result.Findings[0].Description)
	}
}

func TestParseAuditResponse_NewStructure(t *testing.T) {
	response := `{
		"findings": [
			{
				"line_number": 42,
				"issue_type": "privilege",
				"severity": "extreme",
				"description": "sudo access detected",
				"snippet": "sudo rm -rf /"
			}
		],
		"permissions": {
			"file_reads": ["~/.ssh/id_rsa"],
			"file_writes": ["/etc/hosts"],
			"network": ["http://1.2.3.4"],
			"commands": ["curl", "sudo"]
		}
	}`

	result, err := parseAuditResponse(response, "evil.sh")
	if err != nil {
		t.Fatalf("parseAuditResponse failed: %v", err)
	}

	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Severity != "extreme" {
		t.Errorf("expected severity 'extreme', got %s", result.Findings[0].Severity)
	}
	if result.Findings[0].IssueType != "privilege" {
		t.Errorf("expected issue_type 'privilege', got %s", result.Findings[0].IssueType)
	}
	if len(result.Permissions.FileReads) != 1 || result.Permissions.FileReads[0] != "~/.ssh/id_rsa" {
		t.Errorf("unexpected file_reads: %v", result.Permissions.FileReads)
	}
	if len(result.Permissions.Commands) != 2 {
		t.Errorf("expected 2 commands, got %v", result.Permissions.Commands)
	}
}

func TestParseAuditResponse_BarArrayFallback(t *testing.T) {
	// Old bare array format — should still be accepted with empty permissions
	response := `[
		{
			"line_number": 5,
			"issue_type": "secret",
			"severity": "high",
			"description": "Legacy finding",
			"snippet": "TOKEN=abc123"
		}
	]`

	result, err := parseAuditResponse(response, "legacy.sh")
	if err != nil {
		t.Fatalf("parseLegacyArrayResponse failed: %v", err)
	}

	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].IssueType != "secret" {
		t.Errorf("expected type 'secret', got %s", result.Findings[0].IssueType)
	}
	// Permissions should be empty/zero for legacy format
	if len(result.Permissions.FileReads) != 0 {
		t.Errorf("expected no file_reads in legacy format, got %v", result.Permissions.FileReads)
	}
}

// ── buildAuditPrompt tests ───────────────────────────────────────────────────

func TestBuildAuditPrompt(t *testing.T) {
	prompt := buildAuditPrompt("test.py", "print('hello')")

	// Verify prompt contains key elements
	if !containsString(prompt, "test.py") {
		t.Error("prompt should contain file path")
	}

	if !containsString(prompt, "print('hello')") {
		t.Error("prompt should contain file content")
	}

	if !containsString(prompt, "Hardcoded") {
		t.Error("prompt should mention secrets/hardcoded detection")
	}

	if !containsString(prompt, "JSON") {
		t.Error("prompt should specify JSON output format")
	}
}

func TestBuildAuditPrompt_RedFlags(t *testing.T) {
	prompt := buildAuditPrompt("evil.sh", "sudo rm -rf /")

	redFlagTerms := []string{"~/.ssh", "base64", "sudo", "extreme", "permissions"}
	for _, term := range redFlagTerms {
		if !containsString(prompt, term) {
			t.Errorf("prompt should contain red flag term %q", term)
		}
	}
}

// ── ComputeVerdict tests ─────────────────────────────────────────────────────

func TestComputeVerdict(t *testing.T) {
	tests := []struct {
		name     string
		findings []Finding
		want     string
	}{
		{
			name:     "no findings",
			findings: []Finding{},
			want:     "✓ SAFE TO RUN",
		},
		{
			name: "only low severity",
			findings: []Finding{
				{Severity: "low"},
			},
			want: "✓ SAFE TO RUN",
		},
		{
			name: "only medium severity",
			findings: []Finding{
				{Severity: "medium"},
			},
			want: "✓ SAFE TO RUN",
		},
		{
			name: "at least one high",
			findings: []Finding{
				{Severity: "medium"},
				{Severity: "high"},
			},
			want: "⚠ INSTALL WITH CAUTION",
		},
		{
			name: "at least one extreme",
			findings: []Finding{
				{Severity: "high"},
				{Severity: "extreme"},
			},
			want: "⛔ DO NOT RUN",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeVerdict(tc.findings)
			if got != tc.want {
				t.Errorf("ComputeVerdict() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ── MergePermissions tests ───────────────────────────────────────────────────

func TestMergePermissions(t *testing.T) {
	agg := PermissionScope{}
	MergePermissions(&agg, PermissionScope{
		FileReads: []string{"~/.ssh/config", "/etc/passwd"},
		Commands:  []string{"curl"},
	})
	MergePermissions(&agg, PermissionScope{
		FileReads: []string{"~/.ssh/config"}, // duplicate — should be deduped
		Network:   []string{"http://1.2.3.4"},
		Commands:  []string{"curl", "eval"}, // curl duplicate
	})

	if len(agg.FileReads) != 2 {
		t.Errorf("expected 2 unique file_reads, got %d: %v", len(agg.FileReads), agg.FileReads)
	}
	if len(agg.Commands) != 2 {
		t.Errorf("expected 2 unique commands, got %d: %v", len(agg.Commands), agg.Commands)
	}
	if len(agg.Network) != 1 {
		t.Errorf("expected 1 network entry, got %d: %v", len(agg.Network), agg.Network)
	}
}

// ── Helper ───────────────────────────────────────────────────────────────────

// containsString checks whether s contains substr.
func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) >= len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
