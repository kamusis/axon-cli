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

func TestAuditFile_Success(t *testing.T) {
	provider := &mockProvider{
		response: `[
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
		]`,
	}

	findings, err := AuditFile(context.Background(), provider, "test.py", "test content")
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}

	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}

	// Check first finding
	if findings[0].LineNumber != 10 {
		t.Errorf("expected line 10, got %d", findings[0].LineNumber)
	}
	if findings[0].IssueType != "secret" {
		t.Errorf("expected type 'secret', got %s", findings[0].IssueType)
	}
	if findings[0].Severity != "high" {
		t.Errorf("expected severity 'high', got %s", findings[0].Severity)
	}
	if findings[0].FilePath != "test.py" {
		t.Errorf("expected file 'test.py', got %s", findings[0].FilePath)
	}

	// Check second finding
	if findings[1].IssueType != "injection" {
		t.Errorf("expected type 'injection', got %s", findings[1].IssueType)
	}
}

func TestAuditFile_NoFindings(t *testing.T) {
	provider := &mockProvider{
		response: `[]`,
	}

	findings, err := AuditFile(context.Background(), provider, "test.py", "clean content")
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}

	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestAuditFile_WithMarkdownCodeBlock(t *testing.T) {
	provider := &mockProvider{
		response: "```json\n[]\n```",
	}

	findings, err := AuditFile(context.Background(), provider, "test.py", "content")
	if err != nil {
		t.Fatalf("AuditFile failed: %v", err)
	}

	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestAuditFile_InvalidJSON(t *testing.T) {
	provider := &mockProvider{
		response: "This is not JSON",
	}

	findings, err := AuditFile(context.Background(), provider, "test.py", "content")
	if err != nil {
		t.Fatalf("AuditFile should not fail on parse error: %v", err)
	}

	// Should return a parse_error finding instead of failing
	if len(findings) != 1 {
		t.Fatalf("expected 1 parse_error finding, got %d", len(findings))
	}

	if findings[0].IssueType != "parse_error" {
		t.Errorf("expected issue_type 'parse_error', got %s", findings[0].IssueType)
	}
}

func TestAuditFile_NilProvider(t *testing.T) {
	_, err := AuditFile(context.Background(), nil, "test.py", "content")
	if err == nil {
		t.Error("expected error for nil provider")
	}
}

func TestParseAuditResponse_Valid(t *testing.T) {
	response := `[
		{
			"line_number": 5,
			"issue_type": "pii",
			"severity": "medium",
			"description": "Email address detected",
			"snippet": "user@example.com"
		}
	]`

	findings, err := parseAuditResponse(response, "test.md")
	if err != nil {
		t.Fatalf("parseAuditResponse failed: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	if findings[0].IssueType != "pii" {
		t.Errorf("expected type 'pii', got %s", findings[0].IssueType)
	}
}

func TestParseAuditResponse_SkipInvalid(t *testing.T) {
	response := `[
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
	]`

	findings, err := parseAuditResponse(response, "test.md")
	if err != nil {
		t.Fatalf("parseAuditResponse failed: %v", err)
	}

	// Should only include the valid finding
	if len(findings) != 1 {
		t.Errorf("expected 1 valid finding, got %d", len(findings))
	}

	if findings[0].Description != "Valid finding" {
		t.Errorf("unexpected finding: %s", findings[0].Description)
	}
}

func TestBuildAuditPrompt(t *testing.T) {
	prompt := buildAuditPrompt("test.py", "print('hello')")

	// Verify prompt contains key elements
	if !containsString(prompt, "test.py") {
		t.Error("prompt should contain file path")
	}

	if !containsString(prompt, "print('hello')") {
		t.Error("prompt should contain file content")
	}

	if !containsString(prompt, "Hardcoded secrets") {
		t.Error("prompt should mention secrets")
	}

	if !containsString(prompt, "JSON array") {
		t.Error("prompt should specify JSON output format")
	}
}

// Helper function
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
