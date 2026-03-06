package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addSkillCommit writes content to a file inside skillPath in the repo and
// commits it with the given message.
func addSkillCommit(t *testing.T, repoPath, skillPath, content, msg string) {
	t.Helper()
	full := filepath.Join(repoPath, skillPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", repoPath, "add", skillPath},
		{"-C", repoPath, "commit", "-m", msg},
	} {
		if err := gitRun(args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
}

// commitCount returns the number of commits in the repo HEAD log.
func commitCount(t *testing.T, repoPath string) int {
	t.Helper()
	out, err := gitOutput(repoPath, "rev-list", "--count", "HEAD")
	if err != nil {
		t.Fatalf("rev-list --count: %v", err)
	}
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d", &n); err != nil {
		t.Fatalf("parse commit count %q: %v", out, err)
	}
	return n
}

// ── rollbackSkill tests ───────────────────────────────────────────────────────

func TestRollbackSkill_PreviousVersion(t *testing.T) {
	cfg, _ := initTestRepo(t)
	repo := cfg.RepoPath

	// Two commits touching the same skill file.
	addSkillCommit(t, repo, "skills/foo/SKILL.md", "version 1\n", "axon: sync from host1")
	addSkillCommit(t, repo, "skills/foo/SKILL.md", "version 2\n", "axon: sync from host2")

	countBefore := commitCount(t, repo)

	if err := rollbackSkill(repo, "skills/foo/SKILL.md", ""); err != nil {
		t.Fatalf("rollbackSkill: %v", err)
	}

	// A new commit should have been created.
	countAfter := commitCount(t, repo)
	if countAfter != countBefore+1 {
		t.Errorf("expected %d commits after rollback, got %d", countBefore+1, countAfter)
	}

	// The file content should be restored to version 1.
	data, err := os.ReadFile(filepath.Join(repo, "skills/foo/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "version 1" {
		t.Errorf("expected 'version 1' after rollback, got %q", string(data))
	}

	// The new commit message should contain "rollback".
	logOut, _ := gitOutput(repo, "log", "--oneline", "-1")
	if !strings.Contains(logOut, "rollback") {
		t.Errorf("expected rollback commit message, got: %s", logOut)
	}
}

func TestRollbackSkill_NoHistory(t *testing.T) {
	cfg, _ := initTestRepo(t)

	// Skill was never committed — only one (initial) commit exists.
	err := rollbackSkill(cfg.RepoPath, "skills/nonexistent/SKILL.md", "")
	if err == nil {
		t.Fatal("expected error for skill with no history, got nil")
	}
}

func TestRollbackSkill_WithRevision(t *testing.T) {
	cfg, _ := initTestRepo(t)
	repo := cfg.RepoPath

	// Three commits: v1, v2, v3.
	addSkillCommit(t, repo, "skills/bar/SKILL.md", "v1\n", "axon: sync v1")
	sha1, _ := gitOutput(repo, "rev-parse", "HEAD")
	sha1 = strings.TrimSpace(sha1)
	addSkillCommit(t, repo, "skills/bar/SKILL.md", "v2\n", "axon: sync v2")
	addSkillCommit(t, repo, "skills/bar/SKILL.md", "v3\n", "axon: sync v3")

	// Roll back explicitly to the sha1 commit.
	if err := rollbackSkill(repo, "skills/bar/SKILL.md", sha1); err != nil {
		t.Fatalf("rollbackSkill --revision: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(repo, "skills/bar/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "v1" {
		t.Errorf("expected 'v1' after rollback to sha1, got %q", string(data))
	}
}

func TestRollbackSkill_InvalidRevision(t *testing.T) {
	cfg, _ := initTestRepo(t)
	repo := cfg.RepoPath

	addSkillCommit(t, repo, "skills/inv/SKILL.md", "v1\n", "axon: sync v1")

	err := rollbackSkill(repo, "skills/inv/SKILL.md", "deadbeefdeadbeef")
	if err == nil {
		t.Fatal("expected error for invalid revision, got nil")
	}
	if !strings.Contains(err.Error(), "unknown revision") {
		t.Errorf("expected 'unknown revision' in error, got: %v", err)
	}
}

func TestRollbackSkill_Shorthand(t *testing.T) {
	cfg, _ := initTestRepo(t)
	repo := cfg.RepoPath

	// Two commits using a fully-qualified path; rollback via shorthand name.
	addSkillCommit(t, repo, "skills/shorthand/SKILL.md", "v1\n", "axon: sync v1")
	addSkillCommit(t, repo, "skills/shorthand/SKILL.md", "v2\n", "axon: sync v2")

	// "shorthand" should resolve to "skills/shorthand" via resolveSkillPath.
	if err := rollbackSkill(repo, "shorthand", ""); err != nil {
		t.Fatalf("rollbackSkill with shorthand name: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(repo, "skills/shorthand/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "v1" {
		t.Errorf("expected 'v1' after shorthand rollback, got %q", string(data))
	}
}

// ── rollbackHubAll tests ──────────────────────────────────────────────────────

func TestRollbackAll(t *testing.T) {
	cfg, _ := initTestRepo(t)
	repo := cfg.RepoPath

	// Two commits: v1 then v2.
	addSkillCommit(t, repo, "skills/baz/SKILL.md", "v1\n", "axon: sync baz v1")
	addSkillCommit(t, repo, "skills/baz/SKILL.md", "v2\n", "axon: sync baz v2")
	countBefore := commitCount(t, repo)

	if err := rollbackHubAll(repo, ""); err != nil {
		t.Fatalf("rollbackHubAll: %v", err)
	}

	// A new revert commit should have been created (HEAD moves forward by 1).
	countAfter := commitCount(t, repo)
	if countAfter != countBefore+1 {
		t.Errorf("expected %d commits after rollback, got %d", countBefore+1, countAfter)
	}

	// File content should be reverted to v1.
	data, err := os.ReadFile(filepath.Join(repo, "skills/baz/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "v1" {
		t.Errorf("expected 'v1' after hub rollback, got %q", string(data))
	}

	// The new commit message should contain "rollback".
	logOut, _ := gitOutput(repo, "log", "--oneline", "-1")
	if !strings.Contains(logOut, "rollback") {
		t.Errorf("expected rollback commit message, got: %s", logOut)
	}
}

func TestRollbackAll_SingleCommit(t *testing.T) {
	cfg, _ := initTestRepo(t)
	// initTestRepo creates exactly one commit; HEAD~1 does not exist.
	err := rollbackHubAll(cfg.RepoPath, "")
	if err == nil {
		t.Fatal("expected error when Hub has only one commit, got nil")
	}
}

func TestRollbackAll_WithRevision(t *testing.T) {
	cfg, _ := initTestRepo(t)
	repo := cfg.RepoPath

	// Three commits: initial + v1 + v2.
	addSkillCommit(t, repo, "skills/rev/SKILL.md", "v1\n", "axon: sync v1")
	sha1, _ := gitOutput(repo, "rev-parse", "HEAD")
	sha1 = strings.TrimSpace(sha1)
	addSkillCommit(t, repo, "skills/rev/SKILL.md", "v2\n", "axon: sync v2")
	countBefore := commitCount(t, repo)

	// Roll back hub to sha1 (reverting v2).
	if err := rollbackHubAll(repo, sha1); err != nil {
		t.Fatalf("rollbackHubAll --revision: %v", err)
	}

	// A single squashed revert commit should have been created.
	countAfter := commitCount(t, repo)
	if countAfter != countBefore+1 {
		t.Errorf("expected %d commits after hub rollback --revision, got %d", countBefore+1, countAfter)
	}

	// File content should be back to v1.
	data, err := os.ReadFile(filepath.Join(repo, "skills/rev/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "v1" {
		t.Errorf("expected 'v1' after hub rollback to sha1, got %q", string(data))
	}

	// Commit message should reference "rollback".
	logOut, _ := gitOutput(repo, "log", "--oneline", "-1")
	if !strings.Contains(logOut, "rollback") {
		t.Errorf("expected rollback in commit message, got: %s", logOut)
	}
}

func TestRollbackAll_InvalidRevision(t *testing.T) {
	cfg, _ := initTestRepo(t)
	err := rollbackHubAll(cfg.RepoPath, "deadbeefdeadbeef")
	if err == nil {
		t.Fatal("expected error for invalid revision, got nil")
	}
	if !strings.Contains(err.Error(), "unknown revision") {
		t.Errorf("expected 'unknown revision' in error, got: %v", err)
	}
}

// ── showSkillStatus tests ─────────────────────────────────────────────────────

func TestShowSkillStatus(t *testing.T) {
	cfg, _ := initTestRepo(t)
	repo := cfg.RepoPath

	addSkillCommit(t, repo, "skills/qux/SKILL.md", "hello\n", "axon: sync qux")
	addSkillCommit(t, repo, "skills/qux/SKILL.md", "world\n", "axon: sync qux again")

	// Capture stdout to assert output content.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := showSkillStatus(cfg, "skills/qux/SKILL.md", false)

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if err != nil {
		t.Fatalf("showSkillStatus: %v", err)
	}
	if !strings.Contains(out, "skills/qux/SKILL.md") {
		t.Errorf("expected output to contain skill path, got:\n%s", out)
	}
	if !strings.Contains(out, "axon: sync qux") {
		t.Errorf("expected output to contain at least one commit entry, got:\n%s", out)
	}
}

// ── gitLogEntries tests ───────────────────────────────────────────────────────

func TestGitLogEntries(t *testing.T) {
	cfg, _ := initTestRepo(t)
	repo := cfg.RepoPath

	addSkillCommit(t, repo, "skills/log/SKILL.md", "a\n", "axon: commit 1")
	addSkillCommit(t, repo, "skills/log/SKILL.md", "b\n", "axon: commit 2")

	entries, err := gitLogEntries(repo, "skills/log/SKILL.md", 0, 10)
	if err != nil {
		t.Fatalf("gitLogEntries: %v", err)
	}
	if len(entries) < 2 {
		t.Errorf("expected at least 2 log entries, got %d", len(entries))
	}
	// Most recent first.
	if !strings.Contains(entries[0].subject, "commit 2") {
		t.Errorf("expected most recent commit first, got subject %q", entries[0].subject)
	}
	// fullSHA must be populated and longer than the abbreviated sha.
	if entries[0].fullSHA == "" {
		t.Error("expected non-empty fullSHA in log entry")
	}
	if len(entries[0].fullSHA) <= len(entries[0].sha) {
		t.Errorf("fullSHA (%q) should be longer than abbreviated sha (%q)", entries[0].fullSHA, entries[0].sha)
	}
}

func TestGitLogEntries_Skip(t *testing.T) {
	cfg, _ := initTestRepo(t)
	repo := cfg.RepoPath

	addSkillCommit(t, repo, "skills/skip/SKILL.md", "a\n", "axon: commit 1")
	addSkillCommit(t, repo, "skills/skip/SKILL.md", "b\n", "axon: commit 2")
	addSkillCommit(t, repo, "skills/skip/SKILL.md", "c\n", "axon: commit 3")

	// skip=1 should exclude the most recent commit and return commits 2 and 1.
	entries, err := gitLogEntries(repo, "skills/skip/SKILL.md", 1, 10)
	if err != nil {
		t.Fatalf("gitLogEntries skip=1: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries with skip=1, got %d", len(entries))
	}
	if !strings.Contains(entries[0].subject, "commit 2") {
		t.Errorf("expected 'commit 2' as first entry after skip=1, got %q", entries[0].subject)
	}
	if !strings.Contains(entries[1].subject, "commit 1") {
		t.Errorf("expected 'commit 1' as second entry after skip=1, got %q", entries[1].subject)
	}
}

// ── gitCurrentSHA tests ───────────────────────────────────────────────────────

func TestGitCurrentSHA(t *testing.T) {
	cfg, _ := initTestRepo(t)
	sha, err := gitCurrentSHA(cfg.RepoPath)
	if err != nil {
		t.Fatalf("gitCurrentSHA: %v", err)
	}
	if len(sha) == 0 || len(sha) > 10 {
		t.Errorf("expected abbreviated SHA (1-10 chars), got %q", sha)
	}
	for _, c := range sha {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("SHA contains non-hex character %q in %q", c, sha)
			break
		}
	}
}

// ── resolveSkillPath tests ───────────────────────────────────────────────────

func TestResolveSkillPath(t *testing.T) {
	cfg, _ := initTestRepo(t)
	repo := cfg.RepoPath

	// 1. Setup Hub structure
	dirs := []string{
		"skills/foo",
		"workflows/bar",
		"commands/baz",
		"skills/collision",
		"workflows/collision",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(repo, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		input    string
		want     string
		wantErr  bool
		errMatch string
	}{
		{"shorthand skill", "foo", "skills/foo", false, ""},
		{"shorthand workflow", "bar", "workflows/bar", false, ""},
		{"shorthand command", "baz", "commands/baz", false, ""},
		{"direct match", "skills/foo", "skills/foo", false, ""},
		{"collision", "collision", "", true, "ambiguous"},
		{"not found", "nonexistent", "", true, "cannot find"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSkillPath(repo, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveSkillPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMatch != "" && !strings.Contains(err.Error(), tt.errMatch) {
				t.Errorf("resolveSkillPath() error = %v, want matching %q", err, tt.errMatch)
			}
			if got != tt.want {
				t.Errorf("resolveSkillPath() = %v, want %v", got, tt.want)
			}
		})
	}
}
