package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfig_WithoutVendors(t *testing.T) {
	raw := `repo_path: /tmp/repo
sync_mode: read-write
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.Vendors) != 0 {
		t.Errorf("expected 0 vendors, got %d", len(cfg.Vendors))
	}
}

func TestConfig_WithVendors(t *testing.T) {
	raw := `repo_path: /tmp/repo
vendors:
  - name: my-skill
    repo: https://github.com/example/repo.git
    subdir: skills/my-skill
    dest: skills/my-skill
    ref: v1.0
  - name: other-skill
    repo: https://github.com/example/other.git
    subdir: tools/other
    dest: skills/other
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(cfg.Vendors) != 2 {
		t.Fatalf("expected 2 vendors, got %d", len(cfg.Vendors))
	}

	v := cfg.Vendors[0]
	if v.Name != "my-skill" {
		t.Errorf("name: got %q, want %q", v.Name, "my-skill")
	}
	if v.Repo != "https://github.com/example/repo.git" {
		t.Errorf("repo: got %q", v.Repo)
	}
	if v.Subdir != "skills/my-skill" {
		t.Errorf("subdir: got %q", v.Subdir)
	}
	if v.Dest != "skills/my-skill" {
		t.Errorf("dest: got %q", v.Dest)
	}
	if v.Ref != "v1.0" {
		t.Errorf("ref: got %q, want %q", v.Ref, "v1.0")
	}

	// Second entry: ref omitted → empty string (default applied at execution time).
	if cfg.Vendors[1].Ref != "" {
		t.Errorf("expected empty ref for second vendor, got %q", cfg.Vendors[1].Ref)
	}
}

func TestDefaultConfig_NoVendors(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	if len(cfg.Vendors) != 0 {
		t.Errorf("DefaultConfig should have 0 vendors, got %d", len(cfg.Vendors))
	}
}
