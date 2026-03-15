package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamusis/axon-cli/internal/config"
)

// ScanFiles discovers files to audit based on target and configuration.
// If target is empty, scans entire Hub. Otherwise resolves target and scans it.
func ScanFiles(repoPath string, target string, cfg *config.Config) ([]string, error) {
	// Parse allowed extensions
	allowedExts := parseAllowedExtensions()

	// Determine scan root
	var scanRoot string
	if target == "" {
		// Scan entire Hub
		scanRoot = repoPath
	} else {
		// Resolve target (could be skill name, file, or directory)
		resolved, err := resolveTarget(repoPath, target)
		if err != nil {
			return nil, err
		}
		scanRoot = resolved
	}

	// Check if scanRoot exists
	info, err := os.Stat(scanRoot)
	if err != nil {
		return nil, fmt.Errorf("cannot access %s: %w", scanRoot, err)
	}

	var files []string

	if info.IsDir() {
		// Walk directory recursively
		err = filepath.Walk(scanRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip .git directory
			if info.IsDir() && info.Name() == ".git" {
				return filepath.SkipDir
			}

			// Skip directories
			if info.IsDir() {
				return nil
			}

			// Check extension
			if !hasAllowedExtension(path, allowedExts) {
				return nil
			}

			// Check excludes patterns
			if cfg != nil && shouldExclude(path, repoPath, cfg.Excludes) {
				return nil
			}

			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk directory: %w", err)
		}
	} else {
		// Single file
		if !hasAllowedExtension(scanRoot, allowedExts) {
			return nil, fmt.Errorf("file extension not allowed: %s", scanRoot)
		}
		files = append(files, scanRoot)
	}

	return files, nil
}

// parseAllowedExtensions reads AXON_AUDIT_ALLOWED_EXTENSIONS from config.
func parseAllowedExtensions() []string {
	raw, _ := config.GetConfigValue("AXON_AUDIT_ALLOWED_EXTENSIONS")
	if raw == "" {
		return []string{".md", ".sh", ".py", ".js", ".ts", ".yaml", ".yml"}
	}
	parts := strings.Split(raw, ",")
	var exts []string
	for _, p := range parts {
		ext := strings.TrimSpace(p)
		if ext != "" {
			exts = append(exts, ext)
		}
	}
	return exts
}

// hasAllowedExtension checks if file has an allowed extension.
func hasAllowedExtension(path string, allowedExts []string) bool {
	ext := filepath.Ext(path)
	for _, allowed := range allowedExts {
		if ext == allowed {
			return true
		}
	}
	return false
}

// shouldExclude checks if path matches any exclude pattern.
func shouldExclude(path, repoPath string, excludes []string) bool {
	// Get relative path from repo root
	relPath, err := filepath.Rel(repoPath, path)
	if err != nil {
		return false
	}

	for _, pattern := range excludes {
		matched, err := filepath.Match(pattern, relPath)
		if err == nil && matched {
			return true
		}
		// Also try matching against basename
		matched, err = filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}
	}
	return false
}

// resolveTarget resolves a target string to an absolute path.
// Reuses logic similar to resolveSkillPath from cmd/git_utils.go.
func resolveTarget(repoPath, target string) (string, error) {
	// If absolute path, use as-is
	if filepath.IsAbs(target) {
		return target, nil
	}

	// Try as relative to repo
	candidate := filepath.Join(repoPath, target)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	// Try in skills/
	candidate = filepath.Join(repoPath, "skills", target)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	// Try in workflows/
	candidate = filepath.Join(repoPath, "workflows", target)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	// Try in commands/
	candidate = filepath.Join(repoPath, "commands", target)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	return "", fmt.Errorf("target not found: %s", target)
}

