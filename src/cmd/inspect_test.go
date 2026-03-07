package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
	"gopkg.in/yaml.v3"
)

func TestExtractTriggers(t *testing.T) {
	tests := []struct {
		name     string
		yamlData string
		want     []string
	}{
		{
			name:     "bare strings",
			yamlData: `["foo", "bar"]`,
			want:     []string{"foo", "bar"},
		},
		{
			name: "mapping nodes",
			yamlData: `
- pattern: "hello"
  description: "say hello"
- pattern: "world"
  description: "say world"`,
			want: []string{"hello", "world"},
		},
		{
			name:     "single scalar",
			yamlData: `"only-one"`,
			want:     []string{"only-one"},
		},
		{
			name:     "empty",
			yamlData: `[]`,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.yamlData), &node); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}
			// yaml.Unmarshal wraps the sequence in a DocumentNode
			actualNode := node
			if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
				actualNode = *node.Content[0]
			}
			got := extractTriggers(actualNode)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractTriggers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRequiresBins(t *testing.T) {
	m := &skillMeta{}
	m.Requires.Bins = []string{"git", "curl"}
	m.Metadata.Requires.Bins = []string{"gh", "git"}
	m.Metadata.OpenClaw.Requires.Bins = []string{"curl", "jq"}

	got := m.GetRequiresBins()
	want := []string{"git", "curl", "gh", "jq"}

	// Order might vary but dedupe is key.
	// Actually the implementation preserves order of first appearance.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetRequiresBins() = %v, want %v", got, want)
	}
}

func TestUniqueSourceRoots(t *testing.T) {
	repo := "/repo"
	cfg := &config.Config{
		RepoPath: repo,
		Targets: []config.Target{
			{Source: "skills"},
			{Source: "workflows/common"},
		},
	}

	got := uniqueSourceRoots(cfg)
	// roots are filepath.Dir(t.Source) + t.Source
	// 1. Dir("skills") = "." -> /repo/.
	// 2. "skills" -> /repo/skills
	// 3. Dir("workflows/common") = "workflows" -> /repo/workflows
	// 4. "workflows/common" -> /repo/workflows/common

	want := []string{
		filepath.Join(repo, "."),
		filepath.Join(repo, "skills"),
		filepath.Join(repo, "workflows"),
		filepath.Join(repo, "workflows/common"),
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d roots, got %d: %v", len(want), len(got), got)
	}

	seen := make(map[string]bool)
	for _, r := range got {
		seen[r] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("missing root: %s", w)
		}
	}
}

func TestParseSkillMeta(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "SKILL.md")
	content := `---
name: "test-skill"
description: "a test skill"
triggers: ["test"]
---
# Content`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, ok := parseSkillMeta(path)
	if !ok {
		t.Errorf("parseSkillMeta() failed")
	}
	if meta.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", meta.Name)
	}
}

func TestResolveInspectPaths(t *testing.T) {
	repo := t.TempDir()
	cfg := &config.Config{
		RepoPath: repo,
		Targets: []config.Target{
			{Name: "my-skills", Source: "skills"},
			{Name: "ops", Source: "workflows"},
		},
	}

	// Setup directories
	os.MkdirAll(filepath.Join(repo, "skills/humanizer"), 0o755)
	os.MkdirAll(filepath.Join(repo, "workflows"), 0o755)
	os.WriteFile(filepath.Join(repo, "workflows/deploy.md"), []byte(""), 0o644)

	t.Run("exact match skill", func(t *testing.T) {
		paths, err := resolveInspectPaths(cfg, "humanizer")
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != 1 || filepath.Base(paths[0]) != "humanizer" {
			t.Errorf("unexpected paths: %v", paths)
		}
	})

	t.Run("target name match", func(t *testing.T) {
		paths, err := resolveInspectPaths(cfg, "my-skills")
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != 1 || filepath.Base(paths[0]) != "skills" {
			t.Errorf("unexpected paths: %v", paths)
		}
	})

	t.Run("fuzzy match workflow", func(t *testing.T) {
		paths, err := resolveInspectPaths(cfg, "deploy.md")
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != 1 || filepath.Base(paths[0]) != "deploy.md" {
			t.Errorf("unexpected paths: %v", paths)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := resolveInspectPaths(cfg, "nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent item")
		}
	})
}
