package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
)

// setupFileLinkTest creates a tmp HOME so backups land alongside test data
// (avoiding cross-device rename issues in CI/sandboxed runs) and returns a
// minimal Config with one file-type target.
func setupFileLinkTest(t *testing.T) (*config.Config, string) {
	t.Helper()
	tmp := t.TempDir()
	// Redirect HOME so config.AxonDir() / backupPath() write under tmp.
	t.Setenv("HOME", tmp)

	hubRoot := filepath.Join(tmp, "hub")
	if err := os.MkdirAll(hubRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		RepoPath: hubRoot,
		Targets: []config.Target{
			{
				Name:        "codex-rules",
				Source:      "global_rules.md",
				Destination: filepath.Join(tmp, "dest", "AGENTS.md"),
				Type:        "file",
			},
		},
	}
	return cfg, tmp
}

func TestLinkFileTarget_DoesNotExist_CreatesHubFileAndSymlink(t *testing.T) {
	cfg, tmp := setupFileLinkTest(t)
	tgt := cfg.Targets[0]
	dest := tgt.Destination
	hubFile := filepath.Join(cfg.RepoPath, "global_rules.md")

	// Parent of dest must exist (tool installed).
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}

	state, _, notInstalled := linkTarget(cfg, tgt)
	if notInstalled != "" {
		t.Fatalf("unexpected notInstalled=%q", notInstalled)
	}
	if state != "linked" {
		t.Fatalf("state = %q, want linked", state)
	}

	// Hub file must exist as a real (empty) file.
	hi, err := os.Lstat(hubFile)
	if err != nil {
		t.Fatalf("hub file lstat: %v", err)
	}
	if hi.Mode()&os.ModeSymlink != 0 || hi.IsDir() {
		t.Errorf("hub file should be a real file, got mode %v", hi.Mode())
	}

	// dest must be a symlink to hubFile.
	di, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("dest lstat: %v", err)
	}
	if di.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("dest is not a symlink")
	}
	resolved, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if resolved != hubFile {
		t.Errorf("symlink target = %q, want %q", resolved, hubFile)
	}
	_ = tmp
}

func TestLinkFileTarget_HubFileAlreadyHasContent_NotOverwritten(t *testing.T) {
	cfg, _ := setupFileLinkTest(t)
	tgt := cfg.Targets[0]
	dest := tgt.Destination
	hubFile := filepath.Join(cfg.RepoPath, "global_rules.md")

	// Pre-populate hub file.
	if err := os.WriteFile(hubFile, []byte("hub-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}

	if state, _, _ := linkTarget(cfg, tgt); state != "linked" {
		t.Fatalf("state = %q, want linked", state)
	}

	got, err := os.ReadFile(hubFile)
	if err != nil {
		t.Fatalf("read hub: %v", err)
	}
	if string(got) != "hub-content" {
		t.Errorf("hub file content was modified: %q", string(got))
	}
}

func TestLinkFileTarget_AlreadyCorrect_NoOp(t *testing.T) {
	cfg, _ := setupFileLinkTest(t)
	tgt := cfg.Targets[0]
	dest := tgt.Destination
	hubFile := filepath.Join(cfg.RepoPath, "global_rules.md")

	if err := os.WriteFile(hubFile, []byte("hub"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(hubFile, dest); err != nil {
		t.Fatal(err)
	}

	state, _, _ := linkTarget(cfg, tgt)
	if state != "already" {
		t.Errorf("state = %q, want already", state)
	}
	resolved, _ := os.Readlink(dest)
	if resolved != hubFile {
		t.Errorf("symlink target changed to %q", resolved)
	}
}

func TestLinkFileTarget_WrongSymlink_ReLinked(t *testing.T) {
	cfg, tmp := setupFileLinkTest(t)
	tgt := cfg.Targets[0]
	dest := tgt.Destination
	hubFile := filepath.Join(cfg.RepoPath, "global_rules.md")
	wrong := filepath.Join(tmp, "somewhere-else.md")
	if err := os.WriteFile(wrong, []byte("wrong"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(wrong, dest); err != nil {
		t.Fatal(err)
	}

	state, _, _ := linkTarget(cfg, tgt)
	if state != "relinked" {
		t.Fatalf("state = %q, want relinked", state)
	}
	resolved, _ := os.Readlink(dest)
	if resolved != hubFile {
		t.Errorf("symlink target = %q, want %q", resolved, hubFile)
	}
}

func TestLinkFileTarget_RealFileBackedUp(t *testing.T) {
	cfg, _ := setupFileLinkTest(t)
	tgt := cfg.Targets[0]
	dest := tgt.Destination
	hubFile := filepath.Join(cfg.RepoPath, "global_rules.md")

	// Pre-existing real file at dest with valuable content.
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	const original = "user-existing-rules"
	if err := os.WriteFile(dest, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	state, detail, _ := linkTarget(cfg, tgt)
	if state != "backed_up" {
		t.Fatalf("state = %q, want backed_up", state)
	}

	// dest is now a symlink to hubFile.
	di, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("lstat dest: %v", err)
	}
	if di.Mode()&os.ModeSymlink == 0 {
		t.Errorf("dest should be a symlink after backup")
	}
	resolved, _ := os.Readlink(dest)
	if resolved != hubFile {
		t.Errorf("symlink target = %q, want %q", resolved, hubFile)
	}

	// Backup should be a real regular file with the original content.
	// detail looks like "backed up → /home/.../...".
	idx := strings.Index(detail, "→ ")
	if idx == -1 {
		t.Fatalf("unexpected detail %q (no arrow)", detail)
	}
	bkp := strings.TrimSpace(detail[idx+len("→ "):])
	bi, err := os.Lstat(bkp)
	if err != nil {
		t.Fatalf("lstat backup: %v", err)
	}
	if bi.IsDir() {
		t.Errorf("backup should be a file, got directory")
	}
	if bi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("backup should be a real file, got symlink")
	}
	got, err := os.ReadFile(bkp)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(got) != original {
		t.Errorf("backup content = %q, want %q", string(got), original)
	}
}

func TestLinkFileTarget_RealDirAtDest_Errors(t *testing.T) {
	cfg, _ := setupFileLinkTest(t)
	tgt := cfg.Targets[0]
	dest := tgt.Destination

	// Place a real directory where a file is expected.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	state, detail, _ := linkTarget(cfg, tgt)
	if state != "error" {
		t.Fatalf("state = %q, want error", state)
	}
	if !strings.Contains(detail, "directory") {
		t.Errorf("detail %q should mention 'directory'", detail)
	}
}

func TestLinkFileTarget_ParentMissing_NotInstalled(t *testing.T) {
	cfg, _ := setupFileLinkTest(t)
	tgt := cfg.Targets[0]

	// dest parent does not exist — tool not installed.
	state, _, notInstalled := linkTarget(cfg, tgt)
	if state == "error" {
		t.Fatalf("unexpected error state")
	}
	if notInstalled == "" {
		t.Error("expected notInstalled to be set when dest parent is missing")
	}
	if _, err := os.Lstat(tgt.Destination); !os.IsNotExist(err) {
		t.Error("dest should not have been created when parent is missing")
	}
}

func TestLinkFileTarget_MultipleTargetsSameSource(t *testing.T) {
	cfg, tmp := setupFileLinkTest(t)
	hubFile := filepath.Join(cfg.RepoPath, "global_rules.md")

	// Add three more targets sharing the same source — different destinations.
	for _, name := range []string{"claude-rules", "windsurf-rules", "gemini-rules"} {
		dest := filepath.Join(tmp, "dest", name+".md")
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		cfg.Targets = append(cfg.Targets, config.Target{
			Name:        name,
			Source:      "global_rules.md",
			Destination: dest,
			Type:        "file",
		})
	}
	// Ensure first target's parent exists too.
	if err := os.MkdirAll(filepath.Dir(cfg.Targets[0].Destination), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tgt := range cfg.Targets {
		if state, detail, ni := linkTarget(cfg, tgt); state != "linked" {
			t.Fatalf("target %q: state=%q detail=%q notInstalled=%q (want linked)",
				tgt.Name, state, detail, ni)
		}
		resolved, err := os.Readlink(tgt.Destination)
		if err != nil {
			t.Fatalf("readlink %s: %v", tgt.Destination, err)
		}
		if resolved != hubFile {
			t.Errorf("target %q symlink → %q, want %q", tgt.Name, resolved, hubFile)
		}
	}
}

func TestLatestBackup_MatchesFiles(t *testing.T) {
	// File-type backups are plain files. latestBackup must return them too.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := &config.Config{}

	bkp, err := backupPath(cfg, "codex-rules")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bkp, []byte("backed-up"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := latestBackup(cfg, "codex-rules")
	if err != nil {
		t.Fatal(err)
	}
	if got != bkp {
		t.Errorf("latestBackup = %q, want %q", got, bkp)
	}
}

func TestUnlink_FileTarget_RestoresFromBackup(t *testing.T) {
	// End-to-end: link a file target with an existing real file → backup created;
	// then runUnlink-style restore (we exercise the same primitives latestBackup +
	// rename) and verify content lands at dest.
	cfg, _ := setupFileLinkTest(t)
	tgt := cfg.Targets[0]
	dest := tgt.Destination

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	const original = "precious user content"
	if err := os.WriteFile(dest, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if state, _, _ := linkTarget(cfg, tgt); state != "backed_up" {
		t.Fatalf("setup: link should have backed up the original")
	}

	// Simulate the unlink restore step.
	if err := os.Remove(dest); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}
	bkp, err := latestBackup(cfg, tgt.Name)
	if err != nil {
		t.Fatalf("latestBackup: %v", err)
	}
	if bkp == "" {
		t.Fatal("latestBackup returned empty path; backup not found")
	}
	if err := os.Rename(bkp, dest); err != nil {
		t.Fatalf("restore rename: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if string(got) != original {
		t.Errorf("restored content = %q, want %q", string(got), original)
	}
}
