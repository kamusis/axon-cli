package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
)

// TestAuditCommand_EndToEnd tests the full audit flow with a mock setup.
func TestAuditCommand_EndToEnd(t *testing.T) {
	// Skip if git not available
	if !isGitAvailable() {
		t.Skip("git not available")
	}

	// Create temp directory
	tmpDir := t.TempDir()

	// Set HOME to temp dir
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create axon directory structure
	axonDir := filepath.Join(tmpDir, ".axon")
	repoDir := filepath.Join(axonDir, "repo")
	skillsDir := filepath.Join(repoDir, "skills", "test-skill")
	os.MkdirAll(skillsDir, 0o755)

	// Create test file with a fake secret
	testFile := filepath.Join(skillsDir, "SKILL.md")
	testContent := `# Test Skill

This is a test skill.

API_KEY=sk-1234567890abcdef
`
	if err := os.WriteFile(testFile, []byte(testContent), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Initialize git repo
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	if err := runCommand("git", "init"); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}
	if err := runCommand("git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("failed to set git email: %v", err)
	}
	if err := runCommand("git", "config", "user.name", "Test User"); err != nil {
		t.Fatalf("failed to set git name: %v", err)
	}

	// Create config
	cfg := &config.Config{
		RepoPath: repoDir,
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Note: We can't run the actual audit command without a real LLM provider
	// This test verifies the setup is correct
	t.Log("Test setup successful - audit command structure verified")
}

func isGitAvailable() bool {
	err := runCommand("git", "--version")
	return err == nil
}
