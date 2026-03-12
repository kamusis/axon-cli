package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kamusis/axon-cli/internal/gitutil"
)

// resolveSkillPath finds a skill/workflow/command by its shorthand name.
// Examples: "humanizer" -> "skills/humanizer", "git-release" -> "workflows/git-release".
// If multiple matches exist, it returns an error.
func resolveSkillPath(repoPath, name string) (string, error) {
	// 1. Direct match (absolute or already relative).
	full := filepath.Join(repoPath, name)
	if _, err := os.Stat(full); err == nil {
		return name, nil
	}

	// 2. Search in common directories.
	prefixes := []string{"skills", "workflows", "commands"}
	var matches []string
	for _, p := range prefixes {
		candidate := filepath.Join(p, name)
		fullCand := filepath.Join(repoPath, candidate)
		if _, err := os.Stat(fullCand); err == nil {
			matches = append(matches, candidate)
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("cannot find skill, workflow, or command %q in Hub", name)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous name %q matches multiple paths:\n  - %s\nPlease specify the full relative path.",
			name, strings.Join(matches, "\n  - "))
	}

	return matches[0], nil
}

// ── Git Helpers ─────────────────────────────────────────────────────────────

// checkGitAvailable returns a clear error if git is not found on PATH.
func checkGitAvailable() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is not installed or not on PATH\n" +
			"  Axon requires git to manage the Hub repository.\n" +
			"  Install git from https://git-scm.com and try again.")
	}
	return nil
}

// CheckGitMinVersion verifies that the installed git meets the minimum required
// version (major.minor). Returns a descriptive error if git is not found or the
// version is too old, and nil when the requirement is satisfied.
//
// Example: CheckGitMinVersion(2, 28) requires git >= 2.28.
func CheckGitMinVersion(requiredMajor, requiredMinor int) error {
	return gitutil.CheckMinVersion(requiredMajor, requiredMinor)
}

// gitRun executes a git sub-command and streams output to stdout/stderr.
func gitRun(args ...string) error {
	c := exec.Command("git", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// gitOutput runs a git sub-command and returns its combined stdout output.
func gitOutput(repoPath string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", fullArgs...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// gitIsDirty reports whether the repo has uncommitted changes.
func gitIsDirty(repoPath string) (bool, error) {
	out, err := gitOutput(repoPath, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

// gitHasRemote reports whether the repo has any remote configured.
func gitHasRemote(repoPath string) bool {
	out, err := gitOutput(repoPath, "remote")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// gitRemoteIsEmpty reports whether the remote has no refs at all (i.e. it is a
// brand-new empty repository that has never received a push).
func gitRemoteIsEmpty(repoPath string) bool {
	out, err := gitOutput(repoPath, "ls-remote", "--heads", "origin")
	if err != nil {
		// ls-remote failure (e.g. auth error) — treat as non-empty to be safe.
		return false
	}
	return strings.TrimSpace(out) == ""
}

// gitConfigValue returns the value of a git config key.
func gitConfigValue(repoPath, key string) (string, error) {
	out, err := gitOutput(repoPath, "config", "--get", key)
	if err != nil {
		// `git config --get` exits with 1 if the key is not found.
		// Treat that case as an empty value, but propagate other errors.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// gitIdentityConfigured checks if both user.name and user.email are set.
func gitIdentityConfigured(repoPath string) (bool, error) {
	name, err := gitConfigValue(repoPath, "user.name")
	if err != nil {
		return false, err
	}
	email, err := gitConfigValue(repoPath, "user.email")
	if err != nil {
		return false, err
	}
	return name != "" && email != "", nil
}

// commitInfo holds the one-line summary and formatted date of a commit.
type commitInfo struct {
	sha     string // abbreviated (7-char)
	fullSHA string // full 40-char SHA
	subject string
	date    string
}

// gitCommitInfo returns subject + author-date for a given commit and optional
// path filter.
func gitCommitInfo(repoPath, ref, path string) (commitInfo, error) {
	args := []string{"log", ref, "-1", "--format=%H|%s|%cd", "--date=format:%Y-%m-%d %H:%M"}
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := gitOutput(repoPath, args...)
	if err != nil || strings.TrimSpace(out) == "" {
		return commitInfo{}, fmt.Errorf("no commit info for %q (path=%q): %w", ref, path, err)
	}
	parts := strings.SplitN(strings.TrimSpace(out), "|", 3)
	if len(parts) != 3 {
		return commitInfo{}, fmt.Errorf("unexpected git log output: %q", out)
	}
	short := parts[0]
	if len(short) > 7 {
		short = short[:7]
	}
	return commitInfo{sha: short, fullSHA: parts[0], subject: parts[1], date: parts[2]}, nil
}

// gitLogEntries returns up to n commit log entries for a path in the repo,
// skipping the first skip entries. Use skip=0 for no skipping.
func gitLogEntries(repoPath, path string, skip, n int) ([]commitInfo, error) {
	args := []string{"log"}
	if skip > 0 {
		args = append(args, fmt.Sprintf("--skip=%d", skip))
	}
	args = append(args, fmt.Sprintf("-n%d", n), "--format=%H|%s|%cd", "--date=format:%Y-%m-%d %H:%M")
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := gitOutput(repoPath, args...)
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	var entries []commitInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		short := parts[0]
		if len(short) > 7 {
			short = short[:7]
		}
		entries = append(entries, commitInfo{sha: short, fullSHA: parts[0], subject: parts[1], date: parts[2]})
	}
	return entries, nil
}

// gitCurrentSHA returns the abbreviated SHA of HEAD.
func gitCurrentSHA(repoPath string) (string, error) {
	out, err := gitOutput(repoPath, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(out), nil
}
