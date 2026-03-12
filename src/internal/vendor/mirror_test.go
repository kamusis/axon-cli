package vendor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── ValidateDest ─────────────────────────────────────────────────────────────

func TestValidateDest_Absolute_Rejected(t *testing.T) {
	_, err := ValidateDest("/absolute/path")
	if err == nil {
		t.Error("expected error for absolute dest path")
	}
}

func TestValidateDest_DotDot_Rejected(t *testing.T) {
	cases := []string{"../escape", "../../escape", "skills/../../../etc"}
	for _, c := range cases {
		_, err := ValidateDest(c)
		if err == nil {
			t.Errorf("expected error for traversal path %q", c)
		}
	}
}

func TestValidateDest_Valid_Returned(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"skills/foo", "skills/foo"},
		{"skills/foo/", "skills/foo"},
		{"./skills/foo", "skills/foo"},
	}
	for _, c := range cases {
		got, err := ValidateDest(c.input)
		if err != nil {
			t.Errorf("ValidateDest(%q): unexpected error: %v", c.input, err)
			continue
		}
		if got != c.want {
			t.Errorf("ValidateDest(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── Mirror ────────────────────────────────────────────────────────────────────

func setupMirrorDirs(t *testing.T) (hubRoot, src string) {
	t.Helper()
	hub := t.TempDir()

	// Create Hub structure with a parent "skills" dir (simulating axon init).
	skillsDir := filepath.Join(hub, "skills")
	if err := os.Mkdir(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create source directory with some files.
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("# skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "extra.md"), []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return hub, srcDir
}

func TestMirror_Rsync_CopiesFiles(t *testing.T) {
	hub, src := setupMirrorDirs(t)

	// Force rsync path.
	orig := RsyncAvailable
	RsyncAvailable = func() bool { return true }
	defer func() { RsyncAvailable = orig }()

	if err := Mirror(hub, "skills/foo", src); err != nil {
		// If rsync truly isn't available in the test environment, skip.
		if strings.Contains(err.Error(), "rsync") {
			t.Skip("rsync not available in test environment")
		}
		t.Fatalf("Mirror: %v", err)
	}

	assertFileExists(t, filepath.Join(hub, "skills", "foo", "SKILL.md"))
	assertFileExists(t, filepath.Join(hub, "skills", "foo", "extra.md"))
}

func TestMirror_Fallback_CopiesFiles(t *testing.T) {
	hub, src := setupMirrorDirs(t)

	// Force fallback path.
	orig := RsyncAvailable
	RsyncAvailable = func() bool { return false }
	defer func() { RsyncAvailable = orig }()

	if err := Mirror(hub, "skills/foo", src); err != nil {
		t.Fatalf("Mirror fallback: %v", err)
	}

	assertFileExists(t, filepath.Join(hub, "skills", "foo", "SKILL.md"))
	assertFileExists(t, filepath.Join(hub, "skills", "foo", "extra.md"))
}

func TestMirror_Fallback_OverwritesExisting(t *testing.T) {
	hub, src := setupMirrorDirs(t)

	// Pre-populate destination with a stale file.
	destDir := filepath.Join(hub, "skills", "foo")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "stale.md"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := RsyncAvailable
	RsyncAvailable = func() bool { return false }
	defer func() { RsyncAvailable = orig }()

	if err := Mirror(hub, "skills/foo", src); err != nil {
		t.Fatalf("Mirror fallback: %v", err)
	}

	// Stale file must be gone after overwrite.
	if _, err := os.Stat(filepath.Join(destDir, "stale.md")); !os.IsNotExist(err) {
		t.Error("stale.md should have been removed by force overwrite")
	}
	assertFileExists(t, filepath.Join(destDir, "SKILL.md"))
}

func TestMirror_MissingParent_ReturnsError(t *testing.T) {
	hub := t.TempDir()
	// Do NOT create "skills/" inside hub — parent is missing.
	src := t.TempDir()

	orig := RsyncAvailable
	RsyncAvailable = func() bool { return false }
	defer func() { RsyncAvailable = orig }()

	err := Mirror(hub, "skills/foo", src)
	if err == nil {
		t.Error("expected error when parent directory does not exist")
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file %q to exist", path)
	}
}
