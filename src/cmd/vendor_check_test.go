package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
)

func TestVendorCheck(t *testing.T) {
	resetVendorCache(t)

	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(key, home)
	}

	hubRoot := filepath.Join(home, "hub")
	if err := os.MkdirAll(filepath.Join(hubRoot, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	axonDir := filepath.Join(home, ".axon")
	if err := os.MkdirAll(axonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgYAML := "repo_path: " + hubRoot + "\n"
	if err := os.WriteFile(filepath.Join(axonDir, "axon.yaml"), []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	srcRepo := makeLocalVendorRepo(t, "skills/chk", "SKILL.md", "# Check\n")

	// Write manifest
	v := config.Vendor{
		Name:   "chk-skill",
		Repo:   srcRepo,
		Subdir: "skills/chk",
		Dest:   "skills/chk",
		Ref:    "master",
	}
	if err := vendor.WriteManifest(hubRoot, []config.Vendor{v}); err != nil {
		t.Fatal(err)
	}

	// 1. Before sync, check should report cache not initialized
	if err := runVendorCheck(nil, nil); err != nil {
		t.Fatalf("runVendorCheck failed: %v", err)
	}

	// 2. Sync vendor
	orig := vendor.RsyncAvailable
	vendor.RsyncAvailable = func() bool { return false }
	defer func() { vendor.RsyncAvailable = orig }()

	if _, err := syncVendorEntry(hubRoot, v, false); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// 3. Check should report up to date
	if err := runVendorCheck(nil, nil); err != nil {
		t.Fatalf("runVendorCheck failed: %v", err)
	}

	// 4. Checking specific name
	if err := runVendorCheck(nil, []string{"chk-skill"}); err != nil {
		t.Fatalf("runVendorCheck for specific name failed: %v", err)
	}

	// 5. Checking non-existent name
	if err := runVendorCheck(nil, []string{"ghost"}); err == nil {
		t.Error("expected error for non-existent vendor name")
	}
}
