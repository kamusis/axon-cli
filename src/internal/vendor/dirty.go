package vendor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsGitRepo reports whether dir is inside a Git repository.
func IsGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err == nil && (info.IsDir() || !info.IsDir()) { // support regular file for worktrees/submodules
		return true
	}
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	return cmd.Run() == nil
}

// CheckDestDirty checks if cleanDest inside hubRoot has uncommitted Git changes
// (modified, deleted, or untracked files).
// If hubRoot is not a Git repo or cleanDest does not exist, returns (false, nil).
// If dirty, returns (true, nil).
func CheckDestDirty(hubRoot, cleanDest string) (bool, error) {
	if !IsGitRepo(hubRoot) {
		return false, nil
	}

	destAbs := filepath.Join(hubRoot, filepath.FromSlash(cleanDest))
	if _, err := os.Stat(destAbs); os.IsNotExist(err) {
		return false, nil
	}

	// Run git status --porcelain -- <cleanDest>
	cmd := exec.Command("git", "-C", hubRoot, "status", "--porcelain", "--", cleanDest)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("git status failed in %s: %w (%s)", hubRoot, err, strings.TrimSpace(stderr.String()))
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return false, nil
	}

	return true, nil
}
