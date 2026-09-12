package cmd

import (
	"os"
	"os/exec"
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
		{"-C", repoDir, "init", "-b", "master"},
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

	if _, err := syncVendorEntry(hubRoot, v, false); err != nil {
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

	// Verify provenance file was written
	prov, err := vendor.ReadProvenance(filepath.Join(hubRoot, "skills", "foo"))
	if err != nil {
		t.Fatalf("ReadProvenance failed: %v", err)
	}
	if prov == nil || prov.Vendor != "foo-skill" {
		t.Errorf("expected provenance for foo-skill, got %+v", prov)
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
		{"-C", repoDir, "init", "-b", "master"},
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

	if _, err := syncVendorEntry(hubRoot, vAlpha, false); err != nil {
		t.Fatalf("syncVendorEntry(alpha): %v", err)
	}
	if _, err := syncVendorEntry(hubRoot, vBeta, false); err != nil {
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

	_, err := syncVendorEntry(hubRoot, v, false)
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
		if _, err := syncVendorEntry(hubRoot, v, false); err != nil {
			t.Fatalf("run %d: syncVendorEntry: %v", i+1, err)
		}
	}

	dest := filepath.Join(hubRoot, "skills", "bar", "README.md")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("file missing after second run: %v", err)
	}
}

// TestSyncVendorEntry_RemirrorsWhenDestDeleted ensures that deleting the Hub
// destination after a successful sync forces a re-mirror even when the upstream
// SHA is unchanged. Regression for the SHA-only skip that ignored missing dests.
func TestSyncVendorEntry_RemirrorsWhenDestDeleted(t *testing.T) {
	resetVendorCache(t)

	srcRepo := makeLocalVendorRepo(t, "skills/eli5", "SKILL.md", "# ELI5\n")
	hubRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(hubRoot, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	orig := vendor.RsyncAvailable
	vendor.RsyncAvailable = func() bool { return false }
	defer func() { vendor.RsyncAvailable = orig }()

	v := config.Vendor{
		Name:   "eli5",
		Repo:   srcRepo,
		Subdir: "skills/eli5",
		Dest:   "skills/eli5",
		Ref:    "master",
	}

	mirrored, err := syncVendorEntry(hubRoot, v, false)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if !mirrored {
		t.Fatal("first sync should mirror content")
	}

	destDir := filepath.Join(hubRoot, "skills", "eli5")
	if err := os.RemoveAll(destDir); err != nil {
		t.Fatalf("remove dest: %v", err)
	}
	if _, err := os.Stat(destDir); !os.IsNotExist(err) {
		t.Fatal("expected dest to be gone after RemoveAll")
	}

	mirrored, err = syncVendorEntry(hubRoot, v, false)
	if err != nil {
		t.Fatalf("second sync after dest delete: %v", err)
	}
	if !mirrored {
		t.Fatal("second sync should re-mirror when Hub destination is missing")
	}

	data, err := os.ReadFile(filepath.Join(destDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("expected SKILL.md restored: %v", err)
	}
	if string(data) != "# ELI5\n" {
		t.Errorf("unexpected content after re-mirror: %q", string(data))
	}
}

func TestSyncVendorEntry_DirtyGuard(t *testing.T) {
	resetVendorCache(t)

	srcRepo := makeLocalVendorRepo(t, "skills/guard", "SKILL.md", "# Guard\n")
	hubRoot := t.TempDir()

	// Initialize hubRoot as a git repository with an initial commit
	for _, args := range [][]string{
		{"-C", hubRoot, "init", "-b", "master"},
		{"-C", hubRoot, "config", "user.email", "test@axon.local"},
		{"-C", hubRoot, "config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v failed: %v", args, err)
		}
	}

	if err := os.Mkdir(filepath.Join(hubRoot, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	orig := vendor.RsyncAvailable
	vendor.RsyncAvailable = func() bool { return false }
	defer func() { vendor.RsyncAvailable = orig }()

	v := config.Vendor{
		Name:   "guard-skill",
		Repo:   srcRepo,
		Subdir: "skills/guard",
		Dest:   "skills/guard",
		Ref:    "master",
	}

	// 1. First sync cleanly
	if _, err := syncVendorEntry(hubRoot, v, false); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	// Commit the mirrored files into Hub
	exec.Command("git", "-C", hubRoot, "add", ".").Run()
	exec.Command("git", "-C", hubRoot, "commit", "-m", "mirrored guard-skill").Run()

	// 2. Introduce an uncommitted local edit in the Hub
	skillFile := filepath.Join(hubRoot, "skills", "guard", "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("# Local Uncommitted Edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. sync without force should abort with dirty error
	_, err := syncVendorEntry(hubRoot, v, false)
	if err == nil {
		t.Fatal("expected error due to uncommitted local changes")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("expected error mentioning 'uncommitted changes', got: %v", err)
	}

	// Verify local file was NOT overwritten
	content, _ := os.ReadFile(skillFile)
	if string(content) != "# Local Uncommitted Edit\n" {
		t.Errorf("dirty file was overwritten despite guard: %s", string(content))
	}

	// 4. sync with force=true should succeed and overwrite
	_, err = syncVendorEntry(hubRoot, v, true)
	if err != nil {
		t.Fatalf("sync with force failed: %v", err)
	}
	content, _ = os.ReadFile(skillFile)
	if string(content) != "# Guard\n" {
		t.Errorf("file should have been overwritten with force=true, got: %s", string(content))
	}
}

func TestRunVendorSync_AutoMigratesLegacyVendors(t *testing.T) {
	resetVendorCache(t)

	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(key, home)
	}

	hubRoot := filepath.Join(home, "hub")
	if err := os.MkdirAll(filepath.Join(hubRoot, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	srcRepo := makeLocalVendorRepo(t, "skills/mig", "SKILL.md", "# Mig\n")

	axonDir := filepath.Join(home, ".axon")
	if err := os.MkdirAll(axonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgYAML := "repo_path: " + hubRoot + "\nvendors:\n" +
		"  - name: mig-skill\n    repo: " + srcRepo + "\n    subdir: skills/mig\n    dest: skills/mig\n    ref: master\n"
	if err := os.WriteFile(filepath.Join(axonDir, "axon.yaml"), []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := vendor.RsyncAvailable
	vendor.RsyncAvailable = func() bool { return false }
	defer func() { vendor.RsyncAvailable = orig }()

	// Running vendor sync should auto-migrate the entry into axon.vendors.yaml
	if err := runVendorSync(nil, nil); err != nil {
		t.Fatalf("runVendorSync failed: %v", err)
	}

	// 1. Verify manifest exists in Hub
	manifest, err := vendor.ReadManifest(hubRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 1 || manifest[0].Name != "mig-skill" {
		t.Fatalf("manifest = %+v, want 1 entry mig-skill", manifest)
	}

	// 2. Verify axon.yaml has vendors cleared
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Vendors) != 0 {
		t.Errorf("reloaded.Vendors = %+v, want empty", reloaded.Vendors)
	}
}


