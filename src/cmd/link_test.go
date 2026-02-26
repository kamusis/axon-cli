package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
)

// setupLinkTest creates a minimal config and Hub directory for testing.
func setupLinkTest(t *testing.T) (*config.Config, string) {
	t.Helper()
	tmp := t.TempDir()
	hub := filepath.Join(tmp, "hub", "skills")
	if err := os.MkdirAll(hub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hub, "sentinel.md"), []byte("hub content"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		RepoPath: filepath.Join(tmp, "hub"),
		Targets: []config.Target{
			{
				Name:        "test-skills",
				Source:      "skills",
				Destination: filepath.Join(tmp, "dest", "skills"),
				Type:        "directory",
			},
		},
	}
	return cfg, tmp
}

// callLinkTarget wraps the new linkTarget signature into an error return
// for test readability.
func callLinkTarget(cfg *config.Config, t config.Target) error {
	state, detail, _ := linkTarget(cfg, t)
	if state == "error" {
		return &linkErr{detail}
	}
	return nil
}

type linkErr struct{ msg string }

func (e *linkErr) Error() string { return e.msg }

func TestLinkTarget_DoesNotExist(t *testing.T) {
	cfg, _ := setupLinkTest(t)
	dest := cfg.Targets[0].Destination
	hubPath := filepath.Join(cfg.RepoPath, "skills")

	// Pre-create the parent dir — simulates tool installed, skill subdir not yet created.
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := callLinkTarget(cfg, cfg.Targets[0]); err != nil {
		t.Fatalf("link failed: %v", err)
	}

	info, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("lstat %s: %v", dest, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("dest should be a symlink")
	}
	target, _ := os.Readlink(dest)
	if target != hubPath {
		t.Errorf("symlink → %s, want %s", target, hubPath)
	}
}

func TestLinkTarget_ParentMissing(t *testing.T) {
	cfg, _ := setupLinkTest(t)
	// dest parent (~/.cursor/) does not exist — tool not installed.
	// linkTarget should skip gracefully without creating any directories.
	state, _, notInstalled := linkTarget(cfg, cfg.Targets[0])
	if state == "error" {
		t.Fatalf("unexpected error state")
	}
	if notInstalled == "" {
		t.Error("expected notInstalled to be set when parent dir is missing")
	}
	// Neither the parent nor dest should have been created.
	if _, err := os.Lstat(cfg.Targets[0].Destination); !os.IsNotExist(err) {
		t.Error("dest should not exist when parent is missing (tool not installed)")
	}
}

func TestLinkTarget_AlreadyCorrect(t *testing.T) {
	cfg, _ := setupLinkTest(t)
	hubPath := filepath.Join(cfg.RepoPath, "skills")
	dest := cfg.Targets[0].Destination

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(hubPath, dest); err != nil {
		t.Fatal(err)
	}

	// Should be a no-op; symlink must remain unchanged.
	state, _, _ := linkTarget(cfg, cfg.Targets[0])
	if state != "already" {
		t.Errorf("expected state 'already', got %q", state)
	}
	target, _ := os.Readlink(dest)
	if target != hubPath {
		t.Errorf("symlink changed to %s", target)
	}
}

func TestLinkTarget_WrongSymlink(t *testing.T) {
	cfg, tmp := setupLinkTest(t)
	dest := cfg.Targets[0].Destination
	wrongHub := filepath.Join(tmp, "wrong")

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(wrongHub, dest); err != nil {
		t.Fatal(err)
	}

	if err := callLinkTarget(cfg, cfg.Targets[0]); err != nil {
		t.Fatalf("link failed: %v", err)
	}
	target, _ := os.Readlink(dest)
	expected := filepath.Join(cfg.RepoPath, "skills")
	if target != expected {
		t.Errorf("symlink → %s, want %s", target, expected)
	}
}

func TestLinkTarget_NonEmptyDir(t *testing.T) {
	cfg, tmp := setupLinkTest(t)
	dest := cfg.Targets[0].Destination

	// Place a non-empty real directory at dest.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "old.md"), []byte("precious data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := callLinkTarget(cfg, cfg.Targets[0]); err != nil {
		t.Fatalf("link failed: %v", err)
	}
	info, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("lstat %s: %v", dest, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("dest should be a symlink after backup+link")
	}
	// old.md must NOT be accessible at dest (it's in backup).
	if _, err := os.Stat(filepath.Join(dest, "old.md")); err == nil {
		t.Error("old.md should be in backup, not at dest")
	}
	_ = tmp
}

func TestLinkTarget_EmptyDir(t *testing.T) {
	cfg, _ := setupLinkTest(t)
	dest := cfg.Targets[0].Destination

	// Empty real directory at dest.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := callLinkTarget(cfg, cfg.Targets[0]); err != nil {
		t.Fatalf("link failed: %v", err)
	}
	info, _ := os.Lstat(dest)
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("dest should be a symlink after empty-dir removal")
	}
}
