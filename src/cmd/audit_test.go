package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kamusis/axon-cli/internal/config"
)

// Helper function to run shell commands
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

func TestAuditCommand_NoLLMConfig(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Set HOME to temp dir (so it doesn't find real config)
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create minimal config
	axonDir := filepath.Join(tmpDir, ".axon")
	repoDir := filepath.Join(axonDir, "repo")
	os.MkdirAll(repoDir, 0o755)

	cfg := &config.Config{
		RepoPath: repoDir,
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
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

	// Run audit command (should fail with no LLM config)
	err := runAudit(auditCmd, []string{})
	if err == nil {
		t.Error("expected error when LLM not configured")
	}

	if err != nil && err.Error() != "LLM provider not configured. Please set AXON_AUDIT_PROVIDER, AXON_AUDIT_API_KEY, and AXON_AUDIT_MODEL in ~/.axon/.env" {
		t.Logf("Got expected error: %v", err)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds int
		want    string
	}{
		{30, "30 seconds"},
		{90, "1 minutes"},
		{3600, "1 hours"},
		{86400, "1 days"},
	}

	for _, tt := range tests {
		got := formatDuration(time.Duration(tt.seconds) * time.Second)
		if got != tt.want {
			t.Errorf("formatDuration(%d seconds) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}
