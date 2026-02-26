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
			{Name: "windsurf-skills", Source: "skills", Destination: j(".codeium", "windsurf", "skills"), Type: "directory"},
			{Name: "antigravity-skills", Source: "skills", Destination: j(".gemini", "antigravity", "global_skills"), Type: "directory"},
			{Name: "openclaw-skills", Source: "skills", Destination: j(".openclaw", "skills"), Type: "directory"},
			{Name: "cursor-skills", Source: "skills", Destination: j(".cursor", "skills"), Type: "directory"},
			{Name: "opencode-skills", Source: "skills", Destination: j(".opencode", "skills"), Type: "directory"},
			{Name: "neovate-skills", Source: "skills", Destination: j(".neovate", "skills"), Type: "directory"},
			{Name: "claude-code-skills", Source: "skills", Destination: j(".claude", "skills"), Type: "directory"},
			{Name: "codex-skills", Source: "skills", Destination: j(".codex", "skills"), Type: "directory"},
			{Name: "gemini-skills", Source: "skills", Destination: j(".gemini", "skills"), Type: "directory"},
			{Name: "pearai-skills", Source: "skills", Destination: j(".pearai", "skills"), Type: "directory"},
			// === WORKFLOWS (The Structured Task Chains) ===
			{Name: "windsurf-workflows", Source: "workflows", Destination: j(".codeium", "windsurf", "global_workflows"), Type: "directory"},
			{Name: "antigravity-workflows", Source: "workflows", Destination: j(".gemini", "antigravity", "global_workflows"), Type: "directory"},
			{Name: "codex-workflows", Source: "workflows", Destination: j(".codex", "workflows"), Type: "directory"},
			{Name: "gemini-workflows", Source: "workflows", Destination: j(".gemini", "workflows"), Type: "directory"},
			{Name: "openclaw-workflows", Source: "workflows", Destination: j(".openclaw", "workflows"), Type: "directory"},
			// === COMMANDS & ACTIONS (The Executable Extensions) ===
			{Name: "windsurf-commands", Source: "commands", Destination: j(".codeium", "windsurf", "commands"), Type: "directory"},
			{Name: "openclaw-commands", Source: "commands", Destination: j(".openclaw", "commands"), Type: "directory"},
			{Name: "codex-commands", Source: "commands", Destination: j(".codex", "commands"), Type: "directory"},
			{Name: "gemini-commands", Source: "commands", Destination: j(".gemini", "commands"), Type: "directory"},
			{Name: "claude-code-tools", Source: "commands", Destination: j(".anthropic", "claude-code", "tools"), Type: "directory"},
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
