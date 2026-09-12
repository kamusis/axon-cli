package vendor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCheckDestDirty_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	dirty, err := CheckDestDirty(dir, "skills/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dirty {
		t.Errorf("expected dirty=false for non-git repo")
	}
}

func TestCheckDestDirty_GitRepo(t *testing.T) {
	repo := t.TempDir()
	// Init git repo
	exec.Command("git", "-C", repo, "init").Run()
	exec.Command("git", "-C", repo, "config", "user.email", "test@example.com").Run()
	exec.Command("git", "-C", repo, "config", "user.name", "Test").Run()

	skillDir := filepath.Join(repo, "skills", "test-skill")
	os.MkdirAll(skillDir, 0o755)
	fileA := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(fileA, []byte("initial content"), 0o644)

	// Before commit, it's untracked -> dirty
	dirty, err := CheckDestDirty(repo, "skills/test-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dirty {
		t.Errorf("expected dirty=true for untracked files")
	}

	// Commit fileA
	exec.Command("git", "-C", repo, "add", ".").Run()
	exec.Command("git", "-C", repo, "commit", "-m", "init").Run()

	// Now clean
	dirty, err = CheckDestDirty(repo, "skills/test-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dirty {
		t.Errorf("expected dirty=false for clean committed files")
	}

	// Modify fileA
	os.WriteFile(fileA, []byte("modified content"), 0o644)
	dirty, err = CheckDestDirty(repo, "skills/test-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dirty {
		t.Errorf("expected dirty=true for modified tracked file")
	}
}
