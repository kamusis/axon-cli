package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamusis/axon-cli/internal/vendor"
)

func TestVendorAdd_NoSync(t *testing.T) {
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(key, home)
	}

	hubRoot := filepath.Join(home, "hub")
	if err := os.MkdirAll(hubRoot, 0o755); err != nil {
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

	vendorAddSubdir = "."
	vendorAddDest = ""
	vendorAddRef = "main"
	vendorAddNoSync = true

	// Add vendor
	if err := runVendorAdd(nil, []string{"my-skill", "https://github.com/example/my-skill.git"}); err != nil {
		t.Fatalf("runVendorAdd failed: %v", err)
	}

	// Verify manifest
	manifest, err := vendor.ReadManifest(hubRoot)
	if err != nil {
		t.Fatalf("ReadManifest failed: %v", err)
	}
	if len(manifest) != 1 {
		t.Fatalf("manifest length = %d, want 1", len(manifest))
	}
	if manifest[0].Name != "my-skill" || manifest[0].Dest != "skills/my-skill" || manifest[0].Ref != "main" {
		t.Errorf("manifest[0] = %+v, unexpected fields", manifest[0])
	}

	// Adding duplicate name should fail
	if err := runVendorAdd(nil, []string{"my-skill", "https://github.com/example/other.git"}); err == nil {
		t.Error("expected error when adding duplicate vendor name")
	}
}

func TestVendorAdd_AutoMigratesLegacyVendors(t *testing.T) {
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(key, home)
	}

	hubRoot := filepath.Join(home, "hub")
	if err := os.MkdirAll(hubRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	axonDir := filepath.Join(home, ".axon")
	if err := os.MkdirAll(axonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgYAML := "repo_path: " + hubRoot + "\nvendors:\n" +
		"  - name: legacy-v1\n    repo: https://example.com/legacy.git\n    subdir: .\n    dest: skills/legacy\n"
	if err := os.WriteFile(filepath.Join(axonDir, "axon.yaml"), []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	vendorAddSubdir = "."
	vendorAddDest = ""
	vendorAddRef = "main"
	vendorAddNoSync = true

	if err := runVendorAdd(nil, []string{"new-v2", "https://example.com/new.git"}); err != nil {
		t.Fatalf("runVendorAdd failed: %v", err)
	}

	manifest, err := vendor.ReadManifest(hubRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 2 {
		t.Fatalf("manifest count = %d, want 2 (legacy-v1 and new-v2)", len(manifest))
	}
	if manifest[0].Name != "legacy-v1" || manifest[1].Name != "new-v2" {
		t.Errorf("manifest entries unexpected: %+v", manifest)
	}
}

