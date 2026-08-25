package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
)

// ── validateVendors ───────────────────────────────────────────────────────────

func TestValidateVendors_EmptyName(t *testing.T) {
	vendors := []config.Vendor{{Name: "", Repo: "https://github.com/x/y.git", Subdir: "a", Dest: "a"}}
	if err := validateVendors(vendors); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestValidateVendors_EmptyRepo(t *testing.T) {
	vendors := []config.Vendor{{Name: "v1", Repo: "", Subdir: "a", Dest: "a"}}
	if err := validateVendors(vendors); err == nil {
		t.Error("expected error for empty repo")
	}
}

func TestValidateVendors_EmptySubdir(t *testing.T) {
	vendors := []config.Vendor{{Name: "v1", Repo: "https://github.com/x/y.git", Subdir: "", Dest: "a"}}
	if err := validateVendors(vendors); err == nil {
		t.Error("expected error for empty subdir")
	}
}

func TestValidateVendors_EmptyDest(t *testing.T) {
	vendors := []config.Vendor{{Name: "v1", Repo: "https://github.com/x/y.git", Subdir: "a", Dest: ""}}
	if err := validateVendors(vendors); err == nil {
		t.Error("expected error for empty dest")
	}
}

func TestValidateVendors_DuplicateName(t *testing.T) {
	v := config.Vendor{Name: "dup", Repo: "https://github.com/x/y.git", Subdir: "a", Dest: "a"}
	if err := validateVendors([]config.Vendor{v, v}); err == nil {
		t.Error("expected error for duplicate vendor name")
	}
}

func TestValidateVendors_Valid(t *testing.T) {
	vendors := []config.Vendor{
		{Name: "v1", Repo: "https://github.com/x/y.git", Subdir: "a", Dest: "skills/a"},
		{Name: "v2", Repo: "https://github.com/x/z.git", Subdir: "b", Dest: "skills/b"},
	}
	if err := validateVendors(vendors); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── selectVendorByName ────────────────────────────────────────────────────────

func TestSelectVendorByName_Found(t *testing.T) {
	vendors := []config.Vendor{
		{Name: "v1", Repo: "https://github.com/x/y.git", Subdir: "a", Dest: "skills/a"},
		{Name: "v2", Repo: "https://github.com/x/z.git", Subdir: "b", Dest: "skills/b"},
	}
	got, err := selectVendorByName(vendors, "v2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "v2" {
		t.Errorf("selectVendorByName(v2) = %+v, want single entry named v2", got)
	}
}

func TestSelectVendorByName_NotFound(t *testing.T) {
	vendors := []config.Vendor{
		{Name: "v1", Repo: "https://github.com/x/y.git", Subdir: "a", Dest: "skills/a"},
		{Name: "v2", Repo: "https://github.com/x/z.git", Subdir: "b", Dest: "skills/b"},
	}
	_, err := selectVendorByName(vendors, "missing")
	if err == nil {
		t.Fatal("expected error for unknown vendor name")
	}
	if !strings.Contains(err.Error(), "v1") || !strings.Contains(err.Error(), "v2") {
		t.Errorf("error should list configured vendor names, got: %v", err)
	}
}

// ── syncVendorEntry (integration-style with a local git repo as source) ───────

// makeLocalVendorRepo creates a minimal git repo with a subdir containing a file,
// suitable for use as a vendor source in integration tests.
func makeLocalVendorRepo(t *testing.T, subdir, filename, content string) string {
	t.Helper()
	repoDir := t.TempDir()

	for _, args := range [][]string{
		{"-C", repoDir, "init"},
		{"-C", repoDir, "config", "user.email", "test@axon.local"},
		{"-C", repoDir, "config", "user.name", "Axon Test"},
		{"-C", repoDir, "config", "core.autocrlf", "false"},
	} {
		if err := gitRun(args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	fullPath := filepath.Join(repoDir, filepath.FromSlash(subdir), filename)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"-C", repoDir, "add", "."},
		{"-C", repoDir, "commit", "-m", "initial"},
	} {
		if err := gitRun(args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	return repoDir
}

// resetVendorCache sets up an isolated cache root for the test and restores it on cleanup.
func resetVendorCache(t *testing.T) {
	t.Helper()
	orig := vendor.CacheRootOverride
	vendor.CacheRootOverride = t.TempDir()
	t.Cleanup(func() { vendor.CacheRootOverride = orig })
}

func TestSyncVendorEntry_MirrorsContent(t *testing.T) {
	resetVendorCache(t)

	// Build a local git repo to use as vendor source.
	srcRepo := makeLocalVendorRepo(t, "skills/foo", "SKILL.md", "# Foo Skill\n")

	// Hub: a temp dir with the "skills" parent already present.
	hubRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(hubRoot, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Force fallback mirror to avoid rsync dependency in CI.
	orig := vendor.RsyncAvailable
	vendor.RsyncAvailable = func() bool { return false }
	defer func() { vendor.RsyncAvailable = orig }()

	v := config.Vendor{
		Name:   "foo-skill",
		Repo:   srcRepo,
		Subdir: "skills/foo",
		Dest:   "skills/foo",
		Ref:    "master",
	}

	if _, err := syncVendorEntry(hubRoot, v); err != nil {
		t.Fatalf("syncVendorEntry: %v", err)
	}

	dest := filepath.Join(hubRoot, "skills", "foo", "SKILL.md")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("expected SKILL.md in Hub destination: %v", err)
	}
	if string(data) != "# Foo Skill\n" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

// TestSyncVendorEntry_SameRepoTwoSubdirs ensures that two vendor entries that
// share the same upstream repo both get mirrored correctly.  This is the
// regression test for the bug where the second entry was skipped because the
// HEAD-based up-to-date check saw HEAD == origin/<ref> after the first entry's
// Checkout call.
func TestSyncVendorEntry_SameRepoTwoSubdirs(t *testing.T) {
	resetVendorCache(t)

	// Build a single local git repo with two independent subdirs.
	repoDir := t.TempDir()
	for _, args := range [][]string{
		{"-C", repoDir, "init"},
		{"-C", repoDir, "config", "user.email", "test@axon.local"},
		{"-C", repoDir, "config", "user.name", "Axon Test"},
	} {
		if err := gitRun(args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	for _, p := range []struct{ path, content string }{
		{"skills/alpha/SKILL.md", "# Alpha\n"},
		{"skills/beta/SKILL.md", "# Beta\n"},
	} {
		full := filepath.Join(repoDir, filepath.FromSlash(p.path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(p.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"-C", repoDir, "add", "."},
		{"-C", repoDir, "commit", "-m", "initial"},
	} {
		if err := gitRun(args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	hubRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(hubRoot, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	orig := vendor.RsyncAvailable
	vendor.RsyncAvailable = func() bool { return false }
	defer func() { vendor.RsyncAvailable = orig }()

	vAlpha := config.Vendor{Name: "alpha", Repo: repoDir, Subdir: "skills/alpha", Dest: "skills/alpha", Ref: "master"}
	vBeta := config.Vendor{Name: "beta", Repo: repoDir, Subdir: "skills/beta", Dest: "skills/beta", Ref: "master"}

	if _, err := syncVendorEntry(hubRoot, vAlpha); err != nil {
		t.Fatalf("syncVendorEntry(alpha): %v", err)
	}
	if _, err := syncVendorEntry(hubRoot, vBeta); err != nil {
		t.Fatalf("syncVendorEntry(beta): %v", err)
	}

	for _, tc := range []struct{ path, want string }{
		{"skills/alpha/SKILL.md", "# Alpha\n"},
		{"skills/beta/SKILL.md", "# Beta\n"},
	} {
		data, err := os.ReadFile(filepath.Join(hubRoot, filepath.FromSlash(tc.path)))
		if err != nil {
			t.Fatalf("expected %s mirrored to Hub: %v", tc.path, err)
		}
		if string(data) != tc.want {
			t.Errorf("%s: got %q, want %q", tc.path, string(data), tc.want)
		}
	}
}

// TestSyncVendorEntry_SubdirDeletedUpstream verifies that when the vendor subdir
// no longer exists in the upstream repo, syncVendorEntry returns an actionable error
// that guides the user to update axon.yaml — and does NOT modify the local destination.
func TestSyncVendorEntry_SubdirDeletedUpstream(t *testing.T) {
	resetVendorCache(t)

	// Build a repo that contains "skills/present" but NOT "skills/deleted".
	srcRepo := makeLocalVendorRepo(t, "skills/present", "SKILL.md", "# Present\n")

	hubRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(hubRoot, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Pre-populate a local copy to verify it is untouched after the failed sync.
	localDest := filepath.Join(hubRoot, "skills", "deleted")
	if err := os.Mkdir(localDest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDest, "SKILL.md"), []byte("# Deleted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := vendor.RsyncAvailable
	vendor.RsyncAvailable = func() bool { return false }
	defer func() { vendor.RsyncAvailable = orig }()

	v := config.Vendor{
		Name:   "deleted-skill",
		Repo:   srcRepo,
		Subdir: "skills/deleted",
		Dest:   "skills/deleted",
		Ref:    "master",
	}

	_, err := syncVendorEntry(hubRoot, v)
	if err == nil {
		t.Fatal("expected error when subdir is missing upstream")
	}
	if !strings.Contains(err.Error(), "axon.yaml") {
		t.Errorf("error should mention axon.yaml to guide the user, got: %v", err)
	}

	// Local destination must be completely untouched.
	data, readErr := os.ReadFile(filepath.Join(localDest, "SKILL.md"))
	if readErr != nil {
		t.Fatalf("local destination was unexpectedly removed: %v", readErr)
	}
	if string(data) != "# Deleted\n" {
		t.Errorf("local destination content was modified, got: %q", string(data))
	}
}

func TestSyncVendorEntry_IdempotentOnRerun(t *testing.T) {
	resetVendorCache(t)

	srcRepo := makeLocalVendorRepo(t, "skills/bar", "README.md", "bar\n")
	hubRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(hubRoot, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	orig := vendor.RsyncAvailable
	vendor.RsyncAvailable = func() bool { return false }
	defer func() { vendor.RsyncAvailable = orig }()

	v := config.Vendor{
		Name:   "bar-skill",
		Repo:   srcRepo,
		Subdir: "skills/bar",
		Dest:   "skills/bar",
		Ref:    "master",
	}

	// Run twice — should succeed both times.
	for i := 0; i < 2; i++ {
		if _, err := syncVendorEntry(hubRoot, v); err != nil {
			t.Fatalf("run %d: syncVendorEntry: %v", i+1, err)
		}
	}

	dest := filepath.Join(hubRoot, "skills", "bar", "README.md")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("file missing after second run: %v", err)
	}
}
