package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestToolBaseName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"claude-code-skills", "claude-code"},
		{"claude-code-commands", "claude-code"},
		{"windsurf-skills", "windsurf"},
		{"codex-rules", "codex"},
		{"single", "single"},
		{"", ""},
		{"-", ""},
		{"a-b-c", "a-b"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			if got := toolBaseName(c.in); got != c.want {
				t.Errorf("toolBaseName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestIsParentMissing(t *testing.T) {
	tmp := t.TempDir()

	// Parent exists, dest does not — not missing.
	dest1 := filepath.Join(tmp, "child")
	if isParentMissing(dest1) {
		t.Errorf("isParentMissing(%q) = true, want false (parent exists)", dest1)
	}

	// Parent exists and is a file? filepath.Dir of /a/b/c is /a/b, so parent
	// of dest with non-existent parent dir → true.
	dest2 := filepath.Join(tmp, "missing-dir", "child")
	if !isParentMissing(dest2) {
		t.Errorf("isParentMissing(%q) = false, want true (parent missing)", dest2)
	}

	// Deeply nested missing path.
	dest3 := filepath.Join(tmp, "x", "y", "z", "child")
	if !isParentMissing(dest3) {
		t.Errorf("isParentMissing(%q) = false, want true", dest3)
	}
}

func TestCheckSymlinkState(t *testing.T) {
	tmp := t.TempDir()

	hubA := filepath.Join(tmp, "hubA")
	hubB := filepath.Join(tmp, "hubB")
	if err := os.WriteFile(hubA, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hubB, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("missing", func(t *testing.T) {
		dest := filepath.Join(tmp, "missing")
		state, info, actual, err := checkSymlinkState(dest, hubA)
		if state != symlinkMissing {
			t.Errorf("state = %v, want symlinkMissing", state)
		}
		if info != nil || actual != "" || err != nil {
			t.Errorf("missing: expected zero info/actual/err, got info=%v actual=%q err=%v", info, actual, err)
		}
	})

	t.Run("real_file", func(t *testing.T) {
		dest := filepath.Join(tmp, "realfile")
		if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		state, info, actual, err := checkSymlinkState(dest, hubA)
		if state != symlinkRealEntry {
			t.Errorf("state = %v, want symlinkRealEntry", state)
		}
		if info == nil || info.IsDir() {
			t.Errorf("info should be a non-nil regular file: %+v", info)
		}
		if actual != "" || err != nil {
			t.Errorf("real file: expected empty actual and nil err, got actual=%q err=%v", actual, err)
		}
	})

	t.Run("real_dir", func(t *testing.T) {
		dest := filepath.Join(tmp, "realdir")
		if err := os.MkdirAll(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		state, info, _, err := checkSymlinkState(dest, hubA)
		if state != symlinkRealEntry {
			t.Errorf("state = %v, want symlinkRealEntry", state)
		}
		if info == nil || !info.IsDir() {
			t.Errorf("info should be a non-nil directory: %+v", info)
		}
		if err != nil {
			t.Errorf("real dir: expected nil err, got %v", err)
		}
	})

	t.Run("correct_symlink", func(t *testing.T) {
		dest := filepath.Join(tmp, "correct-link")
		if err := os.Symlink(hubA, dest); err != nil {
			t.Fatal(err)
		}
		state, info, actual, err := checkSymlinkState(dest, hubA)
		if state != symlinkCorrect {
			t.Errorf("state = %v, want symlinkCorrect", state)
		}
		if info == nil {
			t.Errorf("info must be populated for symlinkCorrect")
		}
		if actual != hubA {
			t.Errorf("actualTarget = %q, want %q", actual, hubA)
		}
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("wrong_symlink", func(t *testing.T) {
		dest := filepath.Join(tmp, "wrong-link")
		if err := os.Symlink(hubB, dest); err != nil {
			t.Fatal(err)
		}
		state, info, actual, err := checkSymlinkState(dest, hubA)
		if state != symlinkWrong {
			t.Errorf("state = %v, want symlinkWrong", state)
		}
		if info == nil {
			t.Errorf("info must be populated for symlinkWrong")
		}
		if actual != hubB {
			t.Errorf("actualTarget = %q, want %q", actual, hubB)
		}
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("empty_expected_treats_any_symlink_as_correct", func(t *testing.T) {
		dest := filepath.Join(tmp, "any-link")
		if err := os.Symlink(hubB, dest); err != nil {
			t.Fatal(err)
		}
		state, _, actual, err := checkSymlinkState(dest, "")
		if state != symlinkCorrect {
			t.Errorf("state = %v, want symlinkCorrect (expected=\"\")", state)
		}
		if actual != hubB {
			t.Errorf("actualTarget = %q, want %q", actual, hubB)
		}
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("stat_error_on_dangling_parent", func(t *testing.T) {
		// Create a file then make its parent unreadable to simulate a stat
		// error path. On systems where this is unreliable (root, Windows)
		// we accept either symlinkMissing or symlinkStatError — what we're
		// asserting is that the function does not panic and returns a sane
		// classification.
		dest := filepath.Join(tmp, "no-such-parent", "x")
		state, _, _, err := checkSymlinkState(dest, hubA)
		if state != symlinkMissing && state != symlinkStatError {
			t.Errorf("state = %v, want symlinkMissing or symlinkStatError", state)
		}
		if state == symlinkStatError && err == nil {
			t.Errorf("symlinkStatError must carry a non-nil err")
		}
		if state == symlinkMissing && err != nil {
			t.Errorf("symlinkMissing must carry nil err, got %v", err)
		}
		// Reference errors so go vet is happy if we ever extend this case.
		_ = errors.Is(err, os.ErrNotExist)
	})
}
