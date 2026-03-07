package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamusis/axon-cli/internal/config"
)

// makeDir creates a directory inside root and returns its path.
func makeDir(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
		t.Fatal(err)
	}
}

// makeFile creates an empty file inside root.
func makeFile(t *testing.T, root, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
}

func cfgWith(repoPath string, sources ...string) *config.Config {
	var targets []config.Target
	for _, src := range sources {
		targets = append(targets, config.Target{
			Name:        src + "-test",
			Source:      src,
			Destination: "/tmp/unused",
			Type:        "directory",
		})
	}
	return &config.Config{RepoPath: repoPath, Targets: targets}
}

// TestListItems_DirsAndFiles — category with mixed subdirs and files; both appear.
func TestListItems_DirsAndFiles(t *testing.T) {
	repo := t.TempDir()
	makeDir(t, repo, "skills/foo")
	makeDir(t, repo, "skills/bar")
	makeFile(t, repo, "skills/README.md")

	cats := listItems(cfgWith(repo, "skills"))

	if len(cats) != 1 {
		t.Fatalf("expected 1 category, got %d", len(cats))
	}
	cat := cats[0]
	if cat.Label != "skills" {
		t.Errorf("expected label 'skills', got %q", cat.Label)
	}
	want := map[string]bool{"foo": true, "bar": true, "README.md": true}
	if len(cat.Items) != len(want) {
		t.Errorf("expected %d items, got %d", len(want), len(cat.Items))
	}
	for _, item := range cat.Items {
		if !want[item.Name] {
			t.Errorf("unexpected item %q", item.Name)
		}
	}
}

// TestListItems_FlatCategory — category with only files (e.g. workflows/*.md).
func TestListItems_FlatCategory(t *testing.T) {
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, "workflows"), 0o755)
	makeFile(t, repo, "workflows/release.md")
	makeFile(t, repo, "workflows/access-database.md")

	cats := listItems(cfgWith(repo, "workflows"))

	if len(cats) != 1 {
		t.Fatalf("expected 1 category, got %d", len(cats))
	}
	cat := cats[0]
	want := map[string]bool{"release.md": true, "access-database.md": true}
	if len(cat.Items) != len(want) {
		t.Errorf("expected %d items, got %d", len(want), len(cat.Items))
	}
	for _, item := range cat.Items {
		if !want[item.Name] {
			t.Errorf("unexpected item %q", item.Name)
		}
	}
}

// TestListItems_EmptyCategory — source dir exists but has no children.
func TestListItems_EmptyCategory(t *testing.T) {
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, "commands"), 0o755)

	cats := listItems(cfgWith(repo, "commands"))

	if len(cats) != 1 {
		t.Fatalf("expected 1 category, got %d", len(cats))
	}
	if len(cats[0].Items) != 0 {
		t.Errorf("expected 0 items for empty category, got %d", len(cats[0].Items))
	}
}

// TestListItems_MissingSourceDir — source dir missing; category still appears with 0 items.
func TestListItems_MissingSourceDir(t *testing.T) {
	repo := t.TempDir()
	// Do NOT create the "skills" dir.

	cats := listItems(cfgWith(repo, "skills"))

	if len(cats) != 1 {
		t.Fatalf("expected 1 category even when dir is missing, got %d", len(cats))
	}
	if len(cats[0].Items) != 0 {
		t.Errorf("expected 0 items for missing source dir, got %d", len(cats[0].Items))
	}
}

// TestListItems_DeduplicatesCategories — multiple targets sharing one source appear once.
func TestListItems_DeduplicatesCategories(t *testing.T) {
	repo := t.TempDir()
	makeDir(t, repo, "skills/foo")

	// Two targets, both with Source: "skills".
	cfg := &config.Config{
		RepoPath: repo,
		Targets: []config.Target{
			{Name: "cursor-skills", Source: "skills", Destination: "/tmp/a", Type: "directory"},
			{Name: "windsurf-skills", Source: "skills", Destination: "/tmp/b", Type: "directory"},
		},
	}

	cats := listItems(cfg)

	if len(cats) != 1 {
		t.Errorf("expected 1 deduplicated category, got %d", len(cats))
	}
}

// TestListItems_SkipsHidden — hidden entries (dot-prefixed) must not appear.
func TestListItems_SkipsHidden(t *testing.T) {
	repo := t.TempDir()
	makeDir(t, repo, "skills/visible")
	makeDir(t, repo, "skills/.git")
	makeFile(t, repo, "skills/.DS_Store")

	cats := listItems(cfgWith(repo, "skills"))

	if len(cats) != 1 {
		t.Fatalf("expected 1 category, got %d", len(cats))
	}
	for _, item := range cats[0].Items {
		if item.Name == ".git" || item.Name == ".DS_Store" {
			t.Errorf("hidden entry %q must not be listed", item.Name)
		}
	}
	found := false
	for _, item := range cats[0].Items {
		if item.Name == "visible" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'visible' in items")
	}
}
