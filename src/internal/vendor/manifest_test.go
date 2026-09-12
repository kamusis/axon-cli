package vendor

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
)

func TestReadManifest_MissingFile(t *testing.T) {
	hubRoot := t.TempDir()
	v, err := ReadManifest(hubRoot)
	if err != nil {
		t.Fatalf("ReadManifest() unexpected error: %v", err)
	}
	if v != nil {
		t.Errorf("ReadManifest() = %v, want nil", v)
	}
	if ManifestExists(hubRoot) {
		t.Errorf("ManifestExists() = true, want false")
	}
}

func TestWriteAndReadManifest_RoundTrip(t *testing.T) {
	hubRoot := t.TempDir()
	want := []config.Vendor{
		{Name: "v1", Repo: "https://example.com/a/b.git", Subdir: ".", Dest: "skills/v1", Ref: "main"},
		{Name: "v2", Repo: "https://example.com/a/c.git", Subdir: "tools", Dest: "skills/v2", Ref: "v1.0.0"},
	}

	if err := WriteManifest(hubRoot, want); err != nil {
		t.Fatalf("WriteManifest() failed: %v", err)
	}

	if !ManifestExists(hubRoot) {
		t.Fatalf("ManifestExists() = false, want true")
	}

	got, err := ReadManifest(hubRoot)
	if err != nil {
		t.Fatalf("ReadManifest() failed: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadManifest() = %+v, want %+v", got, want)
	}
}

func TestReadManifest_InvalidYAML(t *testing.T) {
	hubRoot := t.TempDir()
	path := ManifestPath(hubRoot)
	if err := os.WriteFile(path, []byte("vendors: [not valid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadManifest(hubRoot)
	if err == nil {
		t.Errorf("ReadManifest() expected error for invalid YAML, got nil")
	}
}

func TestLoadEffectiveVendors(t *testing.T) {
	hubRoot := t.TempDir()
	localCfg := &config.Config{
		Vendors: []config.Vendor{
			{Name: "local-v1", Repo: "https://example.com/l/v1.git", Subdir: ".", Dest: "skills/l1"},
		},
	}

	// 1. Fallback to local when manifest does not exist.
	v, isHub, err := LoadEffectiveVendors(hubRoot, localCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isHub {
		t.Errorf("isHub = true, want false for fallback")
	}
	if len(v) != 1 || v[0].Name != "local-v1" {
		t.Errorf("got %v, want local-v1", v)
	}

	// 2. Hub manifest overrides local config.
	hubVendors := []config.Vendor{
		{Name: "hub-v1", Repo: "https://example.com/h/v1.git", Subdir: ".", Dest: "skills/h1"},
	}
	if err := WriteManifest(hubRoot, hubVendors); err != nil {
		t.Fatal(err)
	}

	v, isHub, err = LoadEffectiveVendors(hubRoot, localCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isHub {
		t.Errorf("isHub = false, want true when manifest exists")
	}
	if len(v) != 1 || v[0].Name != "hub-v1" {
		t.Errorf("got %v, want hub-v1", v)
	}

	// 3. Empty when both empty.
	emptyHub := t.TempDir()
	v, isHub, err = LoadEffectiveVendors(emptyHub, &config.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isHub || len(v) != 0 {
		t.Errorf("expected empty vendors and isHub=false, got %v, %v", v, isHub)
	}
}

func TestMigrateLegacyVendors(t *testing.T) {
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(key, home)
	}
	if err := os.MkdirAll(filepath.Join(home, ".axon"), 0o755); err != nil {
		t.Fatal(err)
	}

	hubRoot := t.TempDir()

	localCfg := &config.Config{
		RepoPath: hubRoot,
		Vendors: []config.Vendor{
			{Name: "v1", Repo: "https://example.com/1.git", Subdir: ".", Dest: "skills/1"},
			{Name: "v2", Repo: "https://example.com/2.git", Subdir: ".", Dest: "skills/2"},
		},
	}

	// Write empty/nil initial manifest
	added, err := MigrateLegacyVendors(hubRoot, localCfg)
	if err != nil {
		t.Fatalf("MigrateLegacyVendors failed: %v", err)
	}
	if added != 2 {
		t.Errorf("added = %d, want 2", added)
	}

	// Verify manifest in hub
	manifest, err := ReadManifest(hubRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 2 {
		t.Errorf("manifest count = %d, want 2", len(manifest))
	}

	// Verify localCfg.Vendors is cleared
	if len(localCfg.Vendors) != 0 {
		t.Errorf("localCfg.Vendors = %v, want nil/empty", localCfg.Vendors)
	}
}
