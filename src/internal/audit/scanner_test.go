package audit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
)

func TestScanFiles_EntireHub(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create test files
	createTestFile(t, tmpDir, "skills/test/SKILL.md", "# Test skill")
	createTestFile(t, tmpDir, "workflows/test.md", "# Test workflow")
	createTestFile(t, tmpDir, "commands/test.sh", "#!/bin/bash")
	createTestFile(t, tmpDir, "README.md", "# README")
	createTestFile(t, tmpDir, ".git/config", "ignored")
	createTestFile(t, tmpDir, "test.txt", "should be ignored")

	// Scan entire Hub
	files, err := ScanFiles(tmpDir, "", nil)
	if err != nil {
		t.Fatalf("ScanFiles failed: %v", err)
	}

	// Should find .md and .sh files, but not .txt or .git/
	expectedCount := 4 // SKILL.md, test.md, test.sh, README.md
	if len(files) != expectedCount {
		t.Errorf("expected %d files, got %d: %v", expectedCount, len(files), files)
	}

	// Verify .git was skipped
	for _, f := range files {
		if filepath.Base(filepath.Dir(f)) == ".git" {
			t.Errorf("should not include .git files: %s", f)
		}
	}

	// Verify .txt was skipped
	for _, f := range files {
		if filepath.Ext(f) == ".txt" {
			t.Errorf("should not include .txt files: %s", f)
		}
	}
}

func TestScanFiles_SingleSkill(t *testing.T) {
	tmpDir := t.TempDir()

	createTestFile(t, tmpDir, "skills/humanizer/SKILL.md", "# Humanizer")
	createTestFile(t, tmpDir, "skills/humanizer/test.py", "print('test')")
	createTestFile(t, tmpDir, "skills/other/SKILL.md", "# Other")

	// Scan single skill by name
	files, err := ScanFiles(tmpDir, "humanizer", nil)
	if err != nil {
		t.Fatalf("ScanFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestScanFiles_SingleFile(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.md")
	createTestFile(t, tmpDir, "test.md", "# Test")

	// Scan single file
	files, err := ScanFiles(tmpDir, testFile, nil)
	if err != nil {
		t.Fatalf("ScanFiles failed: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
	if files[0] != testFile {
		t.Errorf("expected %s, got %s", testFile, files[0])
	}
}

func TestScanFiles_WithExcludes(t *testing.T) {
	tmpDir := t.TempDir()

	createTestFile(t, tmpDir, "test.md", "# Test")
	createTestFile(t, tmpDir, "excluded.md", "# Excluded")

	cfg := &config.Config{
		Excludes: []string{"excluded.md"},
	}

	files, err := ScanFiles(tmpDir, "", cfg)
	if err != nil {
		t.Fatalf("ScanFiles failed: %v", err)
	}

	// Should only find test.md
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
	if filepath.Base(files[0]) != "test.md" {
		t.Errorf("expected test.md, got %s", files[0])
	}
}

func TestResolveTarget(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	skillDir := filepath.Join(tmpDir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0o755)

	// Test resolving skill name
	resolved, err := resolveTarget(tmpDir, "test-skill")
	if err != nil {
		t.Fatalf("resolveTarget failed: %v", err)
	}
	if resolved != skillDir {
		t.Errorf("expected %s, got %s", skillDir, resolved)
	}

	// Test non-existent target
	_, err = resolveTarget(tmpDir, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent target")
	}
}

func TestParseAllowedExtensions(t *testing.T) {
	// Test default
	exts := parseAllowedExtensions()
	if len(exts) != 7 {
		t.Errorf("expected 7 default extensions, got %d", len(exts))
	}

	// Verify defaults include common extensions
	hasMarkdown := false
	for _, ext := range exts {
		if ext == ".md" {
			hasMarkdown = true
		}
	}
	if !hasMarkdown {
		t.Error("expected .md in default extensions")
	}
}

// Helper function to create test files
func createTestFile(t *testing.T, baseDir, relPath, content string) {
	fullPath := filepath.Join(baseDir, relPath)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
}
