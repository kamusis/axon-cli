package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSkills_ParsesFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	skillDir := filepath.Join(repo, "skills", "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: demo-skill\ndescription: Hello world\n---\n\n# Body\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	skills, err := DiscoverSkills(repo)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "demo-skill" {
		t.Fatalf("unexpected name: %q", skills[0].Name)
	}
	if skills[0].Description != "Hello world" {
		t.Fatalf("unexpected description: %q", skills[0].Description)
	}
}
