package search

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DiscoverSkills scans repoRoot/skills/*/SKILL.md and returns parsed SkillDoc entries.
func DiscoverSkills(repoRoot string) ([]SkillDoc, error) {
	skillsDir := filepath.Join(repoRoot, "skills")
	info, err := os.Stat(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SkillDoc{}, nil
		}
		return nil, fmt.Errorf("cannot stat skills directory %s: %w", skillsDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skills path is not a directory: %s", skillsDir)
	}

	var out []SkillDoc
	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}

		rel, err := filepath.Rel(repoRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		id := filepath.Base(filepath.Dir(path))

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

		out = append(out, SkillDoc{
			ID:          id,
			Path:        filepath.ToSlash(rel),
			Name:        name,
			Description: desc,
			Keywords:    keywords,
		})
		return nil
	}

	if err := filepath.WalkDir(skillsDir, walkFn); err != nil {
		return nil, fmt.Errorf("cannot scan skills: %w", err)
	}
	return out, nil
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
