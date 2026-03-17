package audit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixInteractive_Redact(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	content := "Line 1\nAPI_KEY=secret123\nLine 3\n"
	os.WriteFile(testFile, []byte(content), 0o644)

	findings := []Finding{
		{
			FilePath:    testFile,
			LineNumber:  2,
			IssueType:   "secret",
			Severity:    "high",
			Description: "API key detected",
			Snippet:     "API_KEY=secret123",
		},
	}

	// Mock input: choose "r" (redact)
	input := strings.NewReader("r\n")
	output := &bytes.Buffer{}

	stats, err := FixInteractive(findings, input, output)
	if err != nil {
		t.Fatalf("FixInteractive failed: %v", err)
	}

	if stats.Redacted != 1 {
		t.Errorf("expected 1 redacted, got %d", stats.Redacted)
	}

	// Verify file was modified
	modified, _ := os.ReadFile(testFile)
	if !strings.Contains(string(modified), "[REDACTED]") {
		t.Errorf("expected [REDACTED] in file, got: %s", string(modified))
	}
	if strings.Contains(string(modified), "secret123") {
		t.Errorf("secret should be redacted, got: %s", string(modified))
	}
}

func TestFixInteractive_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	content := "Line 1\nLine 2 with secret\nLine 3\n"
	os.WriteFile(testFile, []byte(content), 0o644)

	findings := []Finding{
		{
			FilePath:    testFile,
			LineNumber:  2,
			IssueType:   "secret",
			Severity:    "high",
			Description: "Secret detected",
			Snippet:     "secret",
		},
	}

	// Mock input: choose "d" (delete)
	input := strings.NewReader("d\n")
	output := &bytes.Buffer{}

	stats, err := FixInteractive(findings, input, output)
	if err != nil {
		t.Fatalf("FixInteractive failed: %v", err)
	}

	if stats.Deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", stats.Deleted)
	}

	// Verify line was deleted
	modified, _ := os.ReadFile(testFile)
	lines := strings.Split(string(modified), "\n")
	if len(lines) < 3 || strings.Contains(lines[1], "Line 2") {
		t.Errorf("line 2 should be deleted, got: %v", lines)
	}
}

func TestFixInteractive_Skip(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	content := "Line 1\nLine 2\n"
	os.WriteFile(testFile, []byte(content), 0o644)

	findings := []Finding{
		{
			FilePath:    testFile,
			LineNumber:  2,
			IssueType:   "secret",
			Severity:    "low",
			Description: "Potential issue",
			Snippet:     "Line 2",
		},
	}

	// Mock input: choose "s" (skip)
	input := strings.NewReader("s\n")
	output := &bytes.Buffer{}

	stats, err := FixInteractive(findings, input, output)
	if err != nil {
		t.Fatalf("FixInteractive failed: %v", err)
	}

	if stats.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", stats.Skipped)
	}

	// Verify file was not modified
	modified, _ := os.ReadFile(testFile)
	if string(modified) != content {
		t.Errorf("file should not be modified")
	}
}

func TestFixInteractive_Quit(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	os.WriteFile(testFile, []byte("content"), 0o644)

	findings := []Finding{
		{FilePath: testFile, LineNumber: 1, Description: "Issue 1", Snippet: "a"},
		{FilePath: testFile, LineNumber: 2, Description: "Issue 2", Snippet: "b"},
		{FilePath: testFile, LineNumber: 3, Description: "Issue 3", Snippet: "c"},
	}

	// Mock input: skip first, then quit
	input := strings.NewReader("s\nq\n")
	output := &bytes.Buffer{}

	stats, err := FixInteractive(findings, input, output)
	if err != nil {
		t.Fatalf("FixInteractive failed: %v", err)
	}

	if stats.Skipped != 3 {
		t.Errorf("expected 3 skipped (1 manual + 2 from quit), got %d", stats.Skipped)
	}
}

func TestFixInteractive_InvalidAction(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	os.WriteFile(testFile, []byte("content"), 0o644)

	findings := []Finding{
		{FilePath: testFile, LineNumber: 1, Description: "Issue", Snippet: "x"},
	}

	// Mock input: invalid action, then skip
	input := strings.NewReader("invalid\n")
	output := &bytes.Buffer{}

	stats, err := FixInteractive(findings, input, output)
	if err != nil {
		t.Fatalf("FixInteractive failed: %v", err)
	}

	// Invalid action should be treated as skip
	if stats.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", stats.Skipped)
	}
}

func TestFixInteractive_EmptyFindings(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}

	stats, err := FixInteractive([]Finding{}, input, output)
	if err != nil {
		t.Fatalf("FixInteractive failed: %v", err)
	}

	if stats.Total != 0 {
		t.Errorf("expected 0 total, got %d", stats.Total)
	}
}

func TestRedactFinding(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "Before\nAPI_KEY=secret123\nAfter\n"
	os.WriteFile(testFile, []byte(content), 0o644)

	finding := Finding{
		FilePath: testFile,
		Snippet:  "API_KEY=secret123",
	}

	err := redactFinding(finding)
	if err != nil {
		t.Fatalf("redactFinding failed: %v", err)
	}

	modified, _ := os.ReadFile(testFile)
	if !strings.Contains(string(modified), "[REDACTED]") {
		t.Error("expected [REDACTED] in file")
	}
	if strings.Contains(string(modified), "secret123") {
		t.Error("secret should be removed")
	}
}

func TestRedactFinding_SnippetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("content"), 0o644)

	finding := Finding{
		FilePath: testFile,
		Snippet:  "nonexistent",
	}

	err := redactFinding(finding)
	if err == nil {
		t.Error("expected error when snippet not found")
	}
}

func TestDeleteLine(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "Line 1\nLine 2\nLine 3\n"
	os.WriteFile(testFile, []byte(content), 0o644)

	finding := Finding{
		FilePath:   testFile,
		LineNumber: 2,
	}

	err := deleteLine(finding)
	if err != nil {
		t.Fatalf("deleteLine failed: %v", err)
	}

	modified, _ := os.ReadFile(testFile)
	lines := strings.Split(string(modified), "\n")

	// Should have Line 1, Line 3, and empty string from final newline
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines after deletion, got %d", len(lines))
	}
	if strings.Contains(string(modified), "Line 2") {
		t.Error("Line 2 should be deleted")
	}
}

func TestDeleteLine_InvalidLineNumber(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("Line 1\n"), 0o644)

	finding := Finding{
		FilePath:   testFile,
		LineNumber: 100,
	}

	err := deleteLine(finding)
	if err == nil {
		t.Error("expected error for invalid line number")
	}
}
