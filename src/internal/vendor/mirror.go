package vendor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// ValidateDest ensures dest is a safe Hub-relative path:
//   - must not be absolute
//   - must not escape the Hub root via ".." traversal
//
// Returns the cleaned relative path on success.
func ValidateDest(dest string) (string, error) {
	// Normalise to forward slashes before any checks — dest is a Hub-relative
	// path used as a git subpath, so it must stay POSIX-style.
	dest = strings.ReplaceAll(dest, "\\", "/")
	if strings.HasPrefix(dest, "/") {
		return "", fmt.Errorf("vendor dest must be a relative path, got: %q", dest)
	}
	clean := path.Clean(dest)
	// After cleaning, a path that escapes the root starts with "..".
	if clean == ".." || strings.HasPrefix(clean, "../") {
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

	cmd := exec.Command("rsync", "-a", "--delete", "--exclude=.git", srcSlash, destSlash)
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
	// Copy source tree into destination using pure Go (cross-platform).
	return copyDirContents(src, dest)
}

// copyDirContents recursively copies all files and subdirectories from src into
// dest.  Both src and dest must already exist.  File permissions are preserved.
func copyDirContents(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)

		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode())
		}

		// Regular file — copy with original permissions.
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

// copyFile copies a single file from src to dest, setting the given permission bits.
func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create %q: %w", dest, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %q → %q: %w", src, dest, err)
	}
	return nil
}
