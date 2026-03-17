package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadAuditResults(t *testing.T) {
	// Use temp directory for cache
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create test files
	testFile1 := filepath.Join(tmpDir, "test1.md")
	testFile2 := filepath.Join(tmpDir, "test2.md")
	os.WriteFile(testFile1, []byte("content1"), 0o644)
	os.WriteFile(testFile2, []byte("content2"), 0o644)

	files := []string{testFile1, testFile2}
	findings := []Finding{
		{
			FilePath:    testFile1,
			LineNumber:  10,
			IssueType:   "secret",
			Severity:    "high",
			Description: "API key detected",
			Snippet:     "API_KEY=secret123",
		},
	}

	// Save results
	err := SaveAuditResults("test-target", files, findings, PermissionScope{})
	if err != nil {
		t.Fatalf("SaveAuditResults failed: %v", err)
	}

	// Load results
	cache, err := LoadAuditResults("test-target", files)
	if err != nil {
		t.Fatalf("LoadAuditResults failed: %v", err)
	}

	if cache == nil {
		t.Fatal("expected cache to be loaded")
	}

	// Verify cache contents
	if cache.Target != "test-target" {
		t.Errorf("expected target 'test-target', got %s", cache.Target)
	}

	if len(cache.Files) != 2 {
		t.Errorf("expected 2 files in cache, got %d", len(cache.Files))
	}

	if len(cache.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(cache.Findings))
	}

	if cache.Findings[0].Description != "API key detected" {
		t.Errorf("unexpected finding description: %s", cache.Findings[0].Description)
	}
}

func TestValidateCache_Valid(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.md")
	os.WriteFile(testFile, []byte("content"), 0o644)

	files := []string{testFile}

	// Build file info
	fileInfo, err := buildFileInfo(testFile)
	if err != nil {
		t.Fatalf("buildFileInfo failed: %v", err)
	}

	cache := &AuditCache{
		Target:    "test",
		Timestamp: time.Now(),
		Files:     []FileInfo{fileInfo},
		Findings:  []Finding{},
	}

	// Validate - should be valid
	if !ValidateCache(cache, files) {
		t.Error("expected cache to be valid")
	}
}

func TestValidateCache_FileChanged(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.md")
	os.WriteFile(testFile, []byte("original"), 0o644)

	files := []string{testFile}

	// Build file info with original content
	fileInfo, _ := buildFileInfo(testFile)

	cache := &AuditCache{
		Target:    "test",
		Timestamp: time.Now(),
		Files:     []FileInfo{fileInfo},
		Findings:  []Finding{},
	}

	// Modify file
	time.Sleep(10 * time.Millisecond) // Ensure mtime changes
	os.WriteFile(testFile, []byte("modified"), 0o644)

	// Validate - should be invalid
	if ValidateCache(cache, files) {
		t.Error("expected cache to be invalid after file change")
	}
}

func TestValidateCache_FileAdded(t *testing.T) {
	tmpDir := t.TempDir()

	testFile1 := filepath.Join(tmpDir, "test1.md")
	testFile2 := filepath.Join(tmpDir, "test2.md")
	os.WriteFile(testFile1, []byte("content1"), 0o644)

	// Cache only has file1
	fileInfo1, _ := buildFileInfo(testFile1)
	cache := &AuditCache{
		Target:    "test",
		Timestamp: time.Now(),
		Files:     []FileInfo{fileInfo1},
		Findings:  []Finding{},
	}

	// Now we have file2 as well
	os.WriteFile(testFile2, []byte("content2"), 0o644)
	files := []string{testFile1, testFile2}

	// Validate - should be invalid (new file added)
	if ValidateCache(cache, files) {
		t.Error("expected cache to be invalid after file added")
	}
}

func TestValidateCache_Nil(t *testing.T) {
	if ValidateCache(nil, []string{}) {
		t.Error("expected nil cache to be invalid")
	}
}

func TestGenerateCacheKey(t *testing.T) {
	key1 := generateCacheKey("target1", []string{"file1.md", "file2.md"})
	key2 := generateCacheKey("target1", []string{"file1.md", "file2.md"})
	key3 := generateCacheKey("target2", []string{"file1.md", "file2.md"})

	// Same inputs should produce same key
	if key1 != key2 {
		t.Error("expected same cache key for same inputs")
	}

	// Different target should produce different key
	if key1 == key3 {
		t.Error("expected different cache key for different target")
	}
}

func TestBuildFileInfo(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	content := []byte("test content")
	os.WriteFile(testFile, content, 0o644)

	info, err := buildFileInfo(testFile)
	if err != nil {
		t.Fatalf("buildFileInfo failed: %v", err)
	}

	if info.Path != testFile {
		t.Errorf("expected path %s, got %s", testFile, info.Path)
	}

	if info.MTime == 0 {
		t.Error("expected non-zero mtime")
	}

	if info.Hash == "" {
		t.Error("expected non-empty hash")
	}
}

func TestSaveLoadAuditResults_WithPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	testFile := filepath.Join(tmpDir, "test.md")
	os.WriteFile(testFile, []byte("content"), 0o644)
	files := []string{testFile}

	findings := []Finding{}
	perms := PermissionScope{
		FileReads: []string{"~/.ssh/config"},
		Network:   []string{"http://1.2.3.4"},
		Commands:  []string{"curl"},
	}

	if err := SaveAuditResults("perm-test", files, findings, perms); err != nil {
		t.Fatalf("SaveAuditResults failed: %v", err)
	}

	cache, err := LoadAuditResults("perm-test", files)
	if err != nil {
		t.Fatalf("LoadAuditResults failed: %v", err)
	}
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	if len(cache.Permissions.FileReads) != 1 || cache.Permissions.FileReads[0] != "~/.ssh/config" {
		t.Errorf("unexpected Permissions.FileReads: %v", cache.Permissions.FileReads)
	}
	if len(cache.Permissions.Commands) != 1 || cache.Permissions.Commands[0] != "curl" {
		t.Errorf("unexpected Permissions.Commands: %v", cache.Permissions.Commands)
	}
}

func TestLoadAuditResults_OldCacheCompat(t *testing.T) {
	// Write a cache JSON that lacks the "permissions" key (old cache format).
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	testFile := filepath.Join(tmpDir, "test.md")
	os.WriteFile(testFile, []byte("content"), 0o644)
	files := []string{testFile}

	// Build a raw old-format cache file at the expected path
	cacheDir := filepath.Join(tmpDir, ".axon", "audit-results")
	os.MkdirAll(cacheDir, 0o755)
	cacheKey := generateCacheKey("", files)
	cachePath := filepath.Join(cacheDir, cacheKey+".json")

	oldJSON := `{"target":"","timestamp":"2024-01-01T00:00:00Z","llm_provider":"openai","llm_model":"gpt-4","files":[],"findings":[]}`
	os.WriteFile(cachePath, []byte(oldJSON), 0o600)

	cache, err := LoadAuditResults("", files)
	if err != nil {
		t.Fatalf("LoadAuditResults failed on old cache: %v", err)
	}
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	// Permissions should be zero-value (Go unmarshals missing keys as zero value)
	if len(cache.Permissions.FileReads) != 0 {
		t.Errorf("expected empty FileReads from old cache, got %v", cache.Permissions.FileReads)
	}
}
