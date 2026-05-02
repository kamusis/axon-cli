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

func TestTarget_IsFileIsDirectory(t *testing.T) {
	cases := []struct {
		typ      string
		wantFile bool
		wantDir  bool
	}{
		{"file", true, false},
		{"directory", false, true},
		{"", false, true}, // empty defaults to directory for backwards compatibility
		{"unknown", false, false},
	}
	for _, c := range cases {
		tgt := Target{Type: c.typ}
		if got := tgt.IsFile(); got != c.wantFile {
			t.Errorf("Type=%q IsFile()=%v, want %v", c.typ, got, c.wantFile)
		}
		if got := tgt.IsDirectory(); got != c.wantDir {
			t.Errorf("Type=%q IsDirectory()=%v, want %v", c.typ, got, c.wantDir)
		}
	}
}

func TestEffectiveSearchRoots_FiltersFileTypeAndDeduplicates(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{Name: "a-skills", Source: "skills", Type: "directory"},
			{Name: "b-skills", Source: "skills", Type: "directory"}, // duplicate source
			{Name: "c-rules", Source: "global_rules.md", Type: "file"},
			{Name: "d-rules", Source: "global_rules.md", Type: "file"}, // duplicate file source
			{Name: "e-cmds", Source: "commands", Type: "directory"},
			{Name: "f-legacy", Source: "workflows"}, // empty type acts like directory
		},
	}
	roots := cfg.EffectiveSearchRoots()
	if len(roots) != 3 {
		t.Fatalf("got %d roots, want 3 (skills, commands, workflows): %v", len(roots), roots)
	}
	got := make(map[string]bool)
	for _, r := range roots {
		got[r] = true
	}
	for _, want := range []string{"skills", "commands", "workflows"} {
		if !got[want] {
			t.Errorf("expected root %q in %v", want, roots)
		}
	}
	if got["global_rules.md"] {
		t.Error("global_rules.md (file-type source) should be excluded from search roots")
	}
}

func TestEffectiveSearchRoots_AllFileTypeUsesDefaults(t *testing.T) {
	// When every target is file-type, no directory roots remain — fall back to defaults.
	cfg := &Config{
		Targets: []Target{
			{Name: "a-rules", Source: "global_rules.md", Type: "file"},
		},
	}
	roots := cfg.EffectiveSearchRoots()
	if len(roots) != 3 {
		t.Fatalf("expected fallback defaults (3 entries), got %d: %v", len(roots), roots)
	}
}
