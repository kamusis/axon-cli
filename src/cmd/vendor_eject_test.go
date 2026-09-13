package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
)

func TestVendorEject(t *testing.T) {
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

	// Create skill directory with files and provenance
	skillDir := filepath.Join(hubRoot, "skills", "ejectable")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillMD, []byte("# Ejectable Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provPath := filepath.Join(skillDir, vendor.ProvenanceFileName)
	if err := os.WriteFile(provPath, []byte("vendor: ejectable\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Seed manifest with two vendors
	initial := []config.Vendor{
		{Name: "keep-me", Repo: "https://example.com/keep.git", Subdir: ".", Dest: "skills/keep"},
		{Name: "ejectable", Repo: "https://example.com/eject.git", Subdir: ".", Dest: "skills/ejectable"},
	}
	if err := vendor.WriteManifest(hubRoot, initial); err != nil {
		t.Fatal(err)
	}

	// Eject "ejectable"
	if err := runVendorEject(nil, []string{"ejectable"}); err != nil {
		t.Fatalf("runVendorEject failed: %v", err)
	}

	// 1. Manifest should now only contain "keep-me"
	updated, err := vendor.ReadManifest(hubRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0].Name != "keep-me" {
		t.Errorf("expected 1 remaining vendor 'keep-me', got %+v", updated)
	}

	// 2. .axon-vendor.yaml should be removed
	if _, err := os.Stat(provPath); !os.IsNotExist(err) {
		t.Errorf(".axon-vendor.yaml was not removed")
	}

	// 3. SKILL.md must remain preserved
	content, err := os.ReadFile(skillMD)
	if err != nil {
		t.Fatalf("SKILL.md was removed: %v", err)
	}
	if string(content) != "# Ejectable Skill\n" {
		t.Errorf("SKILL.md content corrupted: %s", string(content))
	}

	// 4. Ejecting non-existent vendor should return error
	if err := runVendorEject(nil, []string{"not-found"}); err == nil {
		t.Error("expected error when ejecting non-existent vendor")
	}
}

func TestVendorEject_AutoMigratesLegacyVendors(t *testing.T) {
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

	// Legacy axon.yaml with vendors declared
	cfgYAML := `repo_path: ` + hubRoot + `
vendors:
  - name: obsidian
    repo: https://github.com/obsidianmd/skills.git
    subdir: skills/obsidian
    dest: skills/obsidian
    ref: main
`
	if err := os.WriteFile(filepath.Join(axonDir, "axon.yaml"), []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create skill directory with files and provenance
	skillDir := filepath.Join(hubRoot, "skills", "obsidian")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillMD, []byte("# Obsidian Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provPath := filepath.Join(skillDir, vendor.ProvenanceFileName)
	if err := os.WriteFile(provPath, []byte("vendor: obsidian\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Eject legacy vendor "obsidian"
	if err := runVendorEject(nil, []string{"obsidian"}); err != nil {
		t.Fatalf("runVendorEject failed for legacy vendor: %v", err)
	}

	// 1. Legacy axon.yaml should have vendors cleared
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Vendors) != 0 {
		t.Errorf("expected legacy vendors to be cleared from axon.yaml, got %d", len(cfg.Vendors))
	}

	// 2. Hub manifest should exist but have obsidian removed (ejected)
	manifest, err := vendor.ReadManifest(hubRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range manifest {
		if v.Name == "obsidian" {
			t.Errorf("obsidian should have been ejected from manifest, but was found: %+v", v)
		}
	}

	// 3. Provenance file must be removed
	if _, err := os.Stat(provPath); !os.IsNotExist(err) {
		t.Errorf(".axon-vendor.yaml was not removed after eject")
	}

	// 4. Content must be preserved
	content, err := os.ReadFile(skillMD)
	if err != nil {
		t.Fatalf("SKILL.md was removed: %v", err)
	}
	if string(content) != "# Obsidian Skill\n" {
		t.Errorf("SKILL.md content corrupted: %s", string(content))
	}
}
