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

func TestDiscoverDocuments_IncludesWorkflowsAndCommands(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")

	skillDir := filepath.Join(repo, "skills", "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: skill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	workflowDir := filepath.Join(repo, "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowContent := "---\nname: wf\ndescription: workflow\n---\n\nDo things\n"
	if err := os.WriteFile(filepath.Join(workflowDir, "w1.md"), []byte(workflowContent), 0o644); err != nil {
		t.Fatal(err)
	}

	commandsDir := filepath.Join(repo, "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commandsDir, "c1.md"), []byte("# Command\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	docs, err := DiscoverDocuments(repo, nil)
	if err != nil {
		t.Fatalf("DiscoverDocuments: %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("expected 3 documents, got %d", len(docs))
	}

	byID := map[string]SkillDoc{}
	for _, d := range docs {
		byID[d.ID] = d
	}

	if _, ok := byID["demo"]; !ok {
		t.Fatalf("expected skills doc with id demo")
	}

	wf, ok := byID["workflows:w1"]
	if !ok {
		t.Fatalf("expected workflows doc with id workflows:w1")
	}
	if wf.Path != "workflows" {
		t.Fatalf("unexpected workflows path: %q", wf.Path)
	}
	if wf.Name != "wf" {
		t.Fatalf("unexpected workflows name: %q", wf.Name)
	}
	if wf.Description != "workflow" {
		t.Fatalf("unexpected workflows description: %q", wf.Description)
	}

	cmd, ok := byID["commands:c1"]
	if !ok {
		t.Fatalf("expected commands doc with id commands:c1")
	}
	if cmd.Path != "commands" {
		t.Fatalf("unexpected commands path: %q", cmd.Path)
	}
}
