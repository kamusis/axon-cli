package search

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DiscoverSkills scans repoRoot/skills/*/SKILL.md and returns parsed SkillDoc entries.
//
// This function is kept for backwards-compatibility. New code should prefer
// DiscoverDocuments, which can scan multiple top-level directories.
func DiscoverSkills(repoRoot string) ([]SkillDoc, error) {
	return DiscoverDocuments(repoRoot, []string{"skills"})
}

// DiscoverDocuments scans a repo for searchable markdown documents.
//
// Supported roots:
//   - skills:     scans skills/*/SKILL.md
//   - workflows:  scans workflows/**/*.md
//   - commands:   scans commands/**/*.md
//
// Missing roots are ignored.
func DiscoverDocuments(repoRoot string, roots []string) ([]SkillDoc, error) {
	if len(roots) == 0 {
		roots = []string{"skills", "workflows", "commands"}
	}

	var out []SkillDoc
	for _, root := range roots {
		dir := filepath.Join(repoRoot, root)
		info, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("cannot stat %s directory %s: %w", root, dir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s path is not a directory: %s", root, dir)
		}

		walkFn := func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			if root == "skills" {
				if d.Name() != "SKILL.md" {
					return nil
				}
				return appendDocFromFile(repoRoot, path, root, &out)
			}

			// workflows/commands: include markdown files.
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				return nil
			}
			return appendDocFromFile(repoRoot, path, root, &out)
		}

		if err := filepath.WalkDir(dir, walkFn); err != nil {
			return nil, fmt.Errorf("cannot scan %s: %w", root, err)
		}
	}
	return out, nil
}

func appendDocFromFile(repoRoot, path, root string, out *[]SkillDoc) error {
	var (
		relDir string
		id     string
	)

	if root == "skills" {
		rel, err := filepath.Rel(repoRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		relDir = rel
		id = filepath.Base(filepath.Dir(path))
	} else {
		relFile, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relDir = filepath.Dir(relFile)
		base := strings.TrimSuffix(filepath.ToSlash(relFile), filepath.Ext(relFile))
		id = strings.ReplaceAll(base, "/", ":")
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	h, body := splitFrontmatter(string(b))

	name := strings.TrimSpace(h["name"])
	desc := strings.TrimSpace(h["description"])
	keywords := strings.TrimSpace(h["keywords"])
	if keywords == "" {
		keywords = strings.TrimSpace(h["tags"])
	}

	if name == "" {
		name = id
	}
	if desc == "" {
		desc = inferDescriptionFromBody(body)
	}

	*out = append(*out, SkillDoc{
		ID:          id,
		Path:        filepath.ToSlash(relDir),
		Name:        name,
		Description: desc,
		Keywords:    keywords,
	})
	return nil
}

func inferDescriptionFromBody(body string) string {
	lines := strings.Split(body, "\n")
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if strings.HasPrefix(ln, "#") {
			continue
		}
		return ln
	}
	return ""
}
