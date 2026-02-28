package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Target represents a single tool entry in axon.yaml.
type Target struct {
	Name        string `yaml:"name"`
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
	Type        string `yaml:"type"`
}

// Config is the in-memory representation of ~/.axon/axon.yaml.
type Config struct {
	RepoPath string   `yaml:"repo_path"`
	SyncMode string   `yaml:"sync_mode,omitempty"`
	Upstream string   `yaml:"upstream,omitempty"`
	Excludes []string `yaml:"excludes,omitempty"`
	Targets  []Target `yaml:"targets,omitempty"`
}

// EffectiveSearchRoots derives the searchable top-level directories from configured targets.
//
// The intent is to avoid introducing a separate search_roots config item; instead, any new
// content directory (e.g. rules/) will naturally appear as a new Target.Source.
//
// If no targets are configured (older configs), a backwards-compatible default is returned.
func (c *Config) EffectiveSearchRoots() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(c.Targets))
	for _, t := range c.Targets {
		s := strings.TrimSpace(t.Source)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return []string{"skills", "workflows", "commands"}
	}
	return out
}

// AxonDir returns the absolute path to ~/.axon/.
func AxonDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".axon"), nil
}

// ConfigPath returns the absolute path to ~/.axon/axon.yaml.
func ConfigPath() (string, error) {
	dir, err := AxonDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "axon.yaml"), nil
}

// ExpandPath expands a leading ~ to the user's home directory.
func ExpandPath(p string) (string, error) {
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand ~: %w", err)
	}
	return filepath.Join(home, p[1:]), nil
}

// DefaultConfig returns the default Config written on first axon init.
func DefaultConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	j := func(parts ...string) string { return filepath.Join(append([]string{home}, parts...)...) }

	return &Config{
		RepoPath: j(".axon", "repo"),
		SyncMode: "read-write",
		Upstream: "https://github.com/kamusis/axon-hub.git",
		Excludes: []string{
			".DS_Store",
			"Thumbs.db",
			"*.tmp",
			"*.bak",
			"*~",
			".idea/",
			".vscode/",
			"__pycache__/",
			"*.log",
		},
		Targets: []Target{
			// === GLOBAL SKILLS (The Prompts & Instructions) ===
			{Name: "claude-code-skills", Source: "skills", Destination: j(".claude", "skills"), Type: "directory"},
			{Name: "codex-skills", Source: "skills", Destination: j(".codex", "skills"), Type: "directory"},
			{Name: "cursor-skills", Source: "skills", Destination: j(".cursor", "skills"), Type: "directory"},
			{Name: "gemini-skills", Source: "skills", Destination: j(".gemini", "skills"), Type: "directory"},
			{Name: "antigravity-skills", Source: "skills", Destination: j(".gemini", "antigravity", "skills"), Type: "directory"},
			{Name: "neovate-skills", Source: "skills", Destination: j(".neovate", "skills"), Type: "directory"},
			{Name: "openclaw-skills", Source: "skills", Destination: j(".openclaw", "skills"), Type: "directory"},
			{Name: "opencode-skills", Source: "skills", Destination: j(".opencode", "skills"), Type: "directory"},
			{Name: "qoder-skills", Source: "skills", Destination: j(".qoder", "skills"), Type: "directory"},
			{Name: "trae-skills", Source: "skills", Destination: j(".trae", "skills"), Type: "directory"},
			{Name: "vscode-skills", Source: "skills", Destination: j(".agent", "skills"), Type: "directory"},
			{Name: "windsurf-skills", Source: "skills", Destination: j(".codeium", "windsurf", "skills"), Type: "directory"},
			// === WORKFLOWS (The Structured Task Chains) ===
			{Name: "antigravity-workflows", Source: "workflows", Destination: j(".gemini", "antigravity", "global_workflows"), Type: "directory"},
			{Name: "windsurf-workflows", Source: "workflows", Destination: j(".codeium", "windsurf", "global_workflows"), Type: "directory"},
			// === COMMANDS & ACTIONS (The Executable Extensions) ===
			{Name: "claude-code-commands", Source: "commands", Destination: j(".claude", "commands"), Type: "directory"},
			{Name: "gemini-commands", Source: "commands", Destination: j(".gemini", "commands"), Type: "directory"},
			{Name: "qoder-commands", Source: "commands", Destination: j(".qoder", "commands"), Type: "directory"},
		},
	}, nil
}

// Load reads and parses ~/.axon/axon.yaml.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", path, err)
	}
	// Expand ~ in RepoPath at load time.
	cfg.RepoPath, err = ExpandPath(cfg.RepoPath)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save marshals cfg and writes it to ~/.axon/axon.yaml.
func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("cannot marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("cannot write config %s: %w", path, err)
	}
	return nil
}
