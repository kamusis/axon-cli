package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
)

// initTestRepo creates a real git repo in a temp dir and returns the config.
func initTestRepo(t *testing.T) (*config.Config, string) {
	t.Helper()
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	// Initialise git repo with a baseline commit.
	for _, args := range [][]string{
		{"-C", repo, "init"},
		{"-C", repo, "config", "user.email", "test@axon.local"},
		{"-C", repo, "config", "user.name", "Axon Test"},
	} {
		if err := gitRun(args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	// Add an initial file so HEAD exists.
	readme := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readme, []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", repo, "add", "."},
		{"-C", repo, "commit", "-m", "initial"},
	} {
		if err := gitRun(args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	cfg := &config.Config{
		RepoPath: repo,
		SyncMode: "read-write",
		Excludes: []string{".DS_Store", "*.tmp"},
		Targets:  []config.Target{},
	}
	return cfg, tmp
}

func TestWriteGitExcludes(t *testing.T) {
	cfg, _ := initTestRepo(t)
	if err := writeGitExcludes(cfg); err != nil {
		t.Fatalf("writeGitExcludes: %v", err)
	}

	excludeFile := filepath.Join(cfg.RepoPath, ".git", "info", "exclude")
	data, err := os.ReadFile(excludeFile)
	if err != nil {
		t.Fatalf("read exclude file: %v", err)
	}
	content := string(data)
	for _, pattern := range cfg.Excludes {
		if !strings.Contains(content, pattern) {
			t.Errorf("pattern %q not found in exclude file:\n%s", pattern, content)
		}
	}
}

func TestGitIsDirty(t *testing.T) {
	cfg, _ := initTestRepo(t)

	// Clean repo should not be dirty.
	dirty, err := gitIsDirty(cfg.RepoPath)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Error("fresh repo should be clean")
	}

	// Add an untracked file — repo becomes dirty.
	if err := os.WriteFile(filepath.Join(cfg.RepoPath, "new.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err = gitIsDirty(cfg.RepoPath)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Error("repo with untracked file should be dirty")
	}
}

func TestSyncReadWrite_NoRemote(t *testing.T) {
	cfg, _ := initTestRepo(t)

	// Add a new file to commit.
	if err := os.WriteFile(filepath.Join(cfg.RepoPath, "skill.md"), []byte("skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also add an excluded file — it should NOT be committed.
	if err := os.WriteFile(filepath.Join(cfg.RepoPath, "junk.tmp"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeGitExcludes(cfg); err != nil {
		t.Fatal(err)
	}
	if err := syncReadWrite(cfg); err != nil {
		t.Fatalf("syncReadWrite: %v", err)
	}

	// skill.md must be committed.
	out, _ := gitOutput(cfg.RepoPath, "log", "--oneline")
	if !strings.Contains(out, "axon: sync from") {
		t.Errorf("expected sync commit in log:\n%s", out)
	}

	// junk.tmp must NOT be tracked by git.
	tracked, _ := gitOutput(cfg.RepoPath, "ls-files", "junk.tmp")
	if strings.TrimSpace(tracked) != "" {
		t.Error("junk.tmp should be excluded from git tracking")
	}
}

func TestGitHasRemote(t *testing.T) {
	cfg, _ := initTestRepo(t)
	// Fresh local repo should have no remote.
	if gitHasRemote(cfg.RepoPath) {
		t.Error("fresh local repo should have no remote")
	}
}
