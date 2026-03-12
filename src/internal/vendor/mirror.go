package vendor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ValidateDest ensures dest is a safe Hub-relative path:
//   - must not be absolute
//   - must not escape the Hub root via ".." traversal
//
// Returns the cleaned relative path on success.
func ValidateDest(dest string) (string, error) {
	if filepath.IsAbs(dest) {
		return "", fmt.Errorf("vendor dest must be a relative path, got: %q", dest)
	}
	clean := filepath.Clean(dest)
	// After cleaning, a path that escapes the root starts with "..".
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("vendor dest %q escapes the Hub root", dest)
	}
	return clean, nil
}

// Mirror copies src directory into dest directory inside hubRoot.
//
// Pre-conditions enforced here:
//  1. The immediate parent directory of dest must already exist (Hub must be initialised).
//  2. The leaf dest directory is auto-created if absent.
//
// Mirror strategy: rsync is preferred; falls back to remove-and-copy when unavailable.
// rsyncAvailable can be overridden in tests via RsyncAvailable.
func Mirror(hubRoot, cleanDest, src string) error {
	destAbs := filepath.Join(hubRoot, cleanDest)

	// Verify the immediate parent exists.
	parent := filepath.Dir(destAbs)
	if _, err := os.Stat(parent); os.IsNotExist(err) {
		return fmt.Errorf(
			"parent directory %q does not exist inside the Hub — run `axon init` first",
			parent,
		)
	}

	// Auto-create the leaf destination directory.
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		return fmt.Errorf("cannot create destination directory %q: %w", destAbs, err)
	}

	if RsyncAvailable() {
		return mirrorRsync(src, destAbs)
	}
	return mirrorFallback(src, destAbs)
}

// RsyncAvailable reports whether rsync is on the PATH.
// Exported as a variable so tests can override it.
var RsyncAvailable = func() bool {
	_, err := exec.LookPath("rsync")
	return err == nil
}

func mirrorRsync(src, dest string) error {
	// Ensure src ends with "/" so rsync syncs contents, not the directory itself.
	srcSlash := strings.TrimRight(src, string(os.PathSeparator)) + string(os.PathSeparator)
	destSlash := strings.TrimRight(dest, string(os.PathSeparator)) + string(os.PathSeparator)

	cmd := exec.Command("rsync", "-a", "--delete", srcSlash, destSlash)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync failed: %w", err)
	}
	return nil
}

func mirrorFallback(src, dest string) error {
	// Remove existing destination contents.
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("cannot remove existing destination %q: %w", dest, err)
	}
	// Re-create the destination dir.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("cannot recreate destination directory %q: %w", dest, err)
	}
	// Copy source contents into destination.
	cmd := exec.Command("cp", "-a", src+string(os.PathSeparator)+".", dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cp fallback failed: %w", err)
	}
	return nil
}
