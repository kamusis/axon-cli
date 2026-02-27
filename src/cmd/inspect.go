package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect <skill-name>",
	Short: "Show metadata and structure of a skill or target",
	Long: `Display a formatted summary of a skill in the Hub, including its
description, triggers, scripts, and declared dependencies.

The argument can be either:
  - A skill folder name inside the Hub (e.g. humanizer)
  - A target name from axon.yaml (e.g. windsurf-skills)

Example:
  axon inspect humanizer
  axon inspect oracle-health-check`,
	Args: cobra.ExactArgs(1),
	RunE: runInspect,
}

func init() {
	rootCmd.AddCommand(inspectCmd)
}

// skillMeta holds the parsed YAML frontmatter from a SKILL.md file.
// We capture all known fields loosely — unknown fields are ignored.
type skillMeta struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Version     string   `yaml:"version"`
	License     string   `yaml:"license"`
	AllowedTools []string `yaml:"allowed-tools"`
	AutoInvoke  bool     `yaml:"auto_invoke"`

	// Triggers: list of {pattern, description} maps OR bare strings.
	// We unmarshal as []yaml.Node for maximum flexibility.
	Triggers yaml.Node `yaml:"triggers"`

	// Requires: {bins: [...], envs: [...]} dependency block.
	Requires struct {
		Bins []string `yaml:"bins"`
		Envs []string `yaml:"envs"`
	} `yaml:"requires"`

	// OpenClaw Metadata standard nested fields
	Metadata struct {
		Requires struct {
			Bins []string `yaml:"bins"`
		} `yaml:"requires"`
		OpenClaw struct {
			Requires struct {
				Bins []string `yaml:"bins"`
			} `yaml:"requires"`
		} `yaml:"openclaw"`
	} `yaml:"metadata"`
}

// GetRequiresBins merges bins from legacy format and deep metadata openclaw format
func (m *skillMeta) GetRequiresBins() []string {
	var bins []string
	bins = append(bins, m.Requires.Bins...)
	bins = append(bins, m.Metadata.Requires.Bins...)
	bins = append(bins, m.Metadata.OpenClaw.Requires.Bins...)

	// Dedupe
	seen := make(map[string]bool)
	var unique []string
	for _, b := range bins {
		if !seen[b] && b != "" {
			seen[b] = true
			unique = append(unique, b)
		}
	}
	return unique
}

func runInspect(_ *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	arg := args[0]

	dirs, err := resolveSkillDirs(cfg, arg)
	if err != nil {
		return err
	}

	for i, skillDir := range dirs {
		if i > 0 {
			fmt.Println(strings.Repeat("─", 50))
		}
		printInspect(skillDir)
	}
	return nil
}

// resolveSkillDirs finds all skill directories matching arg.
// Exact name match (skill folder or target name) is tried first.
// If no exact match, falls back to substring/fuzzy matching on all skill folders.
func resolveSkillDirs(cfg *config.Config, arg string) ([]string, error) {
	// Collect unique source roots.
	sourceRoots := uniqueSourceRoots(cfg)

	// 1. Exact match: look for a subfolder named exactly arg.
	for _, root := range sourceRoots {
		candidate := filepath.Join(root, arg)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return []string{candidate}, nil
		}
	}

	// 2. Exact match: target name.
	for _, t := range cfg.Targets {
		if t.Name == arg {
			dir := filepath.Join(cfg.RepoPath, t.Source)
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return []string{dir}, nil
			}
		}
	}

	// 3. Fuzzy: substring match on skill folder names (case-insensitive).
	lower := strings.ToLower(arg)
	seen := map[string]bool{}
	var matches []string
	for _, root := range sourceRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || e.Name() == ".git" {
				continue
			}
			if strings.Contains(strings.ToLower(e.Name()), lower) {
				full := filepath.Join(root, e.Name())
				if !seen[full] {
					seen[full] = true
					matches = append(matches, full)
				}
			}
		}
	}

	if len(matches) > 0 {
		return matches, nil
	}

	return nil, fmt.Errorf("skill or target %q not found in Hub.\nTip: run 'axon status' to see available targets.", arg)
}

// uniqueSourceRoots returns the unique parent directories of all target sources.
func uniqueSourceRoots(cfg *config.Config) []string {
	seen := map[string]bool{}
	var roots []string
	for _, t := range cfg.Targets {
		root := filepath.Join(cfg.RepoPath, filepath.Dir(t.Source))
		if !seen[root] {
			seen[root] = true
			roots = append(roots, root)
		}
		// Also the source itself (for target-level inspect).
		src := filepath.Join(cfg.RepoPath, t.Source)
		if !seen[src] {
			seen[src] = true
			roots = append(roots, src)
		}
	}
	return roots
}

// printInspect displays the formatted inspection output for one skill directory.
func printInspect(skillDir string) {
	meta, hasMeta := parseSkillMeta(filepath.Join(skillDir, "SKILL.md"))
	files := listSkillFiles(skillDir)
	scripts := listExecutables(filepath.Join(skillDir, "scripts"))

	name := filepath.Base(skillDir)
	if meta.Name != "" {
		name = meta.Name
	}

	fmt.Printf("📦 Skill: %s\n", name)
	if meta.Version != "" {
		fmt.Printf("Version:  %s\n", meta.Version)
	}
	if meta.Description != "" {
		desc := strings.ReplaceAll(strings.TrimSpace(meta.Description), "\n", " ")
		fmt.Printf("Summary:  %s\n", desc)
	}
	if !hasMeta {
		fmt.Printf("  (no SKILL.md found)\n")
	}

	if triggers := extractTriggers(meta.Triggers); len(triggers) > 0 {
		fmt.Println("\nTriggers:")
		for _, t := range triggers {
			fmt.Printf("  - %s\n", t)
		}
	}
	if len(meta.AllowedTools) > 0 {
		fmt.Printf("\nAllowed Tools: %s\n", strings.Join(meta.AllowedTools, ", "))
	}
	if len(files) > 0 {
		fmt.Println("\nFiles:")
		for _, f := range files {
			fmt.Printf("  - %s\n", f)
		}
	}
	if len(scripts) > 0 {
		fmt.Println("\nScripts:")
		for _, s := range scripts {
			fmt.Printf("  - scripts/%s (Executable)\n", s)
		}
	}
	if len(meta.Requires.Bins) > 0 || len(meta.Requires.Envs) > 0 {
		fmt.Println("\nDependencies (declared):")
		for _, b := range meta.Requires.Bins {
			status := "✓ Found"
			if _, err := exec.LookPath(b); err != nil {
				status = "✗ Not found"
			}
			fmt.Printf("  bin: %-20s %s\n", b, status)
		}
		for _, e := range meta.Requires.Envs {
			status := "✓ Set"
			if os.Getenv(e) == "" {
				status = "✗ Not set"
			}
			fmt.Printf("  env: %-20s %s\n", e, status)
		}
	}
	fmt.Printf("\nPath: %s\n", skillDir)
}

// parseSkillMeta reads and parses the YAML frontmatter from a SKILL.md file.
// Returns (meta, true) on success, (zero, false) if the file doesn't exist or
// has no frontmatter.
func parseSkillMeta(skillMDPath string) (skillMeta, bool) {
	f, err := os.Open(skillMDPath)
	if err != nil {
		return skillMeta{}, false
	}
	defer f.Close()

	// Frontmatter is delimited by --- lines.
	scanner := bufio.NewScanner(f)
	var inFrontmatter bool
	var yamlLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			// Closing --- reached.
			break
		}
		if inFrontmatter {
			yamlLines = append(yamlLines, line)
		}
	}

	if len(yamlLines) == 0 {
		return skillMeta{}, false
	}

	var meta skillMeta
	if err := yaml.Unmarshal([]byte(strings.Join(yamlLines, "\n")), &meta); err != nil {
		return skillMeta{}, false
	}
	return meta, true
}

// extractTriggers normalises the triggers YAML node into plain strings.
// Supports both:
//   - bare string list: ["foo", "bar"]
//   - map list: [{pattern: "foo", description: "bar"}, ...]
func extractTriggers(node yaml.Node) []string {
	if node.Kind == 0 {
		return nil
	}
	var out []string
	if node.Kind == yaml.SequenceNode {
		for _, item := range node.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				out = append(out, item.Value)
			case yaml.MappingNode:
				// Extract "pattern" key.
				for i := 0; i+1 < len(item.Content); i += 2 {
					if item.Content[i].Value == "pattern" {
						out = append(out, item.Content[i+1].Value)
					}
				}
			}
		}
	} else if node.Kind == yaml.ScalarNode {
		out = append(out, node.Value)
	}
	return out
}

// listSkillFiles returns a human-readable list of notable files in skillDir
// (SKILL.md, README*, scripts/). Ignores deeply nested paths.
func listSkillFiles(skillDir string) []string {
	entries, err := os.ReadDir(skillDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if name == ".git" {
			continue
		}
		label := name
		switch {
		case name == "SKILL.md":
			label = "SKILL.md (Instructions)"
		case strings.ToUpper(name) == "README.MD":
			label = name + " (Readme)"
		case e.IsDir() && name == "scripts":
			label = "scripts/ (Scripts directory)"
		case e.IsDir() && name == "examples":
			label = "examples/ (Examples)"
		case e.IsDir() && name == "resources":
			label = "resources/ (Resources)"
		}
		out = append(out, label)
	}
	return out
}

// listExecutables returns the names of executable files in the scripts/ subdir.
func listExecutables(scriptsDir string) []string {
	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// Include if it has any execute bit set, or is a known script extension.
		mode := info.Mode()
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		isScript := mode&0o111 != 0 ||
			ext == ".py" || ext == ".sh" || ext == ".js" || ext == ".ts" || ext == ".rb"
		if isScript {
			out = append(out, name)
		}
	}
	return out
}
