package vendor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCachePath_DerivesOwnerRepo(t *testing.T) {
	tests := []struct {
		repoURL     string
		wantOwner   string
		wantRepo    string
	}{
		{"https://github.com/anthropics/claude-code.git", "anthropics", "claude-code"},
		{"https://github.com/anthropics/claude-code", "anthropics", "claude-code"},
		{"git@github.com:anthropics/claude-code.git", "anthropics", "claude-code"},
	}
	for _, tc := range tests {
		got, err := CachePath(tc.repoURL)
		if err != nil {
			t.Fatalf("CachePath(%q): %v", tc.repoURL, err)
		}
		// Path should end with …/vendors/<owner>/<repo>
		if filepath.Base(got) != tc.wantRepo {
			t.Errorf("CachePath(%q): want last segment %q, got %q", tc.repoURL, tc.wantRepo, filepath.Base(got))
		}
		if filepath.Base(filepath.Dir(got)) != tc.wantOwner {
			t.Errorf("CachePath(%q): want owner segment %q, got %q", tc.repoURL, tc.wantOwner, filepath.Base(filepath.Dir(got)))
		}
		if filepath.Base(filepath.Dir(filepath.Dir(got))) != "vendors" {
			t.Errorf("CachePath(%q): expected grandparent dir 'vendors', got %q", tc.repoURL, filepath.Base(filepath.Dir(filepath.Dir(got))))
		}
	}
}

func TestIsCloned_False_WhenMissing(t *testing.T) {
	dir := t.TempDir()
	if IsCloned(filepath.Join(dir, "nonexistent")) {
		t.Error("expected IsCloned to return false for nonexistent path")
	}
}

func TestIsCloned_False_WhenNoGitDir(t *testing.T) {
	dir := t.TempDir()
	if IsCloned(dir) {
		t.Error("expected IsCloned to return false when .git is absent")
	}
}

func TestIsCloned_True_WhenGitDirExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsCloned(dir) {
		t.Error("expected IsCloned to return true when .git dir exists")
	}
}

func TestSourcePath_ReturnsError_WhenMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := SourcePath(dir, "nonexistent/path")
	if err == nil {
		t.Error("expected error when subdir does not exist")
	}
}

func TestSourcePath_ReturnsError_WhenFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "subdir")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := SourcePath(dir, "subdir")
	if err == nil {
		t.Error("expected error when subdir path points to a file")
	}
}

func TestReadWriteVendorSHA_RoundTrip(t *testing.T) {
	orig := CacheRootOverride
	CacheRootOverride = t.TempDir()
	defer func() { CacheRootOverride = orig }()

	// No file yet — should return empty string without error.
	got, err := ReadVendorSHA("my-vendor")
	if err != nil {
		t.Fatalf("ReadVendorSHA before write: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty SHA before first write, got %q", got)
	}

	const sha = "abc123def456abc123def456abc123def456abc1"
	if err := WriteVendorSHA("my-vendor", sha); err != nil {
		t.Fatalf("WriteVendorSHA: %v", err)
	}

	got, err = ReadVendorSHA("my-vendor")
	if err != nil {
		t.Fatalf("ReadVendorSHA after write: %v", err)
	}
	if got != sha {
		t.Errorf("got %q, want %q", got, sha)
	}
}

func TestWriteVendorSHA_IndependentPerName(t *testing.T) {
	orig := CacheRootOverride
	CacheRootOverride = t.TempDir()
	defer func() { CacheRootOverride = orig }()

	if err := WriteVendorSHA("alpha", "sha-for-alpha"); err != nil {
		t.Fatal(err)
	}
	if err := WriteVendorSHA("beta", "sha-for-beta"); err != nil {
		t.Fatal(err)
	}

	gotA, _ := ReadVendorSHA("alpha")
	gotB, _ := ReadVendorSHA("beta")
	if gotA != "sha-for-alpha" {
		t.Errorf("alpha: got %q", gotA)
	}
	if gotB != "sha-for-beta" {
		t.Errorf("beta: got %q", gotB)
	}
}

func TestSourcePath_ReturnsAbsPath_WhenValid(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "skills", "foo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := SourcePath(dir, "skills/foo")
	if err != nil {
		t.Fatalf("SourcePath: %v", err)
	}
	if got != sub {
		t.Errorf("got %q, want %q", got, sub)
	}
}
