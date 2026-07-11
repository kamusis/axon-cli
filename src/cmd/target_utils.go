package cmd

import (
	"os"
	"path/filepath"
	"strings"
)

// toolBaseName derives the tool's base name from a target name by stripping
// the suffix after the last hyphen ("claude-code-skills" → "claude-code",
// "windsurf-rules" → "windsurf"). When the target name has no hyphen the full
// name is returned unchanged.
//
// This is used to group "tool not installed" results by tool rather than by
// individual target so users see "claude (skipped)" once instead of once per
// claude-* entry in axon.yaml.
func toolBaseName(targetName string) string {
	if idx := strings.LastIndex(targetName, "-"); idx != -1 {
		return targetName[:idx]
	}
	return targetName
}

// isParentMissing reports whether the parent directory of dest does not exist.
// A missing parent is the canonical "tool not installed" signal across the cmd
// package — e.g. ~/.claude is missing means the Claude editor itself isn't
// installed, so any axon target rooted at ~/.claude/* is silently skipped.
//
// Errors other than "not exist" (permission denied, etc.) are conservatively
// reported as not-missing so the caller continues into the regular state
// machine and surfaces a more specific error.
func isParentMissing(dest string) bool {
	parent := filepath.Dir(dest)
	_, err := os.Stat(parent)
	return os.IsNotExist(err)
}

// symlinkState classifies a destination path's state relative to an expected
// hub target, as observed by Lstat + Readlink.
type symlinkState int

const (
	// symlinkMissing — dest does not exist.
	symlinkMissing symlinkState = iota
	// symlinkCorrect — dest is a symlink and points at expected (or expected
	// is empty and dest is any valid symlink).
	symlinkCorrect
	// symlinkWrong — dest is a symlink but points somewhere other than expected.
	symlinkWrong
	// symlinkRealEntry — dest exists but is a real file or directory, not a symlink.
	symlinkRealEntry
	// symlinkStatError — Lstat or Readlink returned an unexpected error.
	symlinkStatError
)

// checkSymlinkState examines dest and classifies its state relative to the
// expected hub target.
//
// Return values:
//   - state: one of the symlinkState constants above.
//   - info:  the Lstat result for dest. nil for symlinkMissing and for
//     symlinkStatError when Lstat itself failed; otherwise populated.
//   - actualTarget: the Readlink result. Populated for symlinkCorrect and
//     symlinkWrong; empty otherwise.
//   - err: the underlying error when state is symlinkStatError; nil otherwise.
//
// When expected is the empty string, any valid symlink is reported as
// symlinkCorrect. Callers that don't care about target correctness (e.g.
// unlink, which removes any symlink regardless of where it points) can pass
// "" and treat symlinkCorrect as "is a symlink".
func checkSymlinkState(dest, expected string) (state symlinkState, info os.FileInfo, actualTarget string, err error) {
	info, statErr := os.Lstat(dest)
	if os.IsNotExist(statErr) {
		return symlinkMissing, nil, "", nil
	}
	if statErr != nil {
		return symlinkStatError, nil, "", statErr
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return symlinkRealEntry, info, "", nil
	}
	actual, readErr := os.Readlink(dest)
	if readErr != nil {
		return symlinkStatError, info, "", readErr
	}
	if expected == "" || actual == expected {
		return symlinkCorrect, info, actual, nil
	}
	// The immediate target differs from expected, but the link may be a
	// chain (dest -> intermediate symlink -> hub) whose final hop lands on
	// expected. os.Readlink returns only the first hop, so resolve the full
	// chain and compare against the resolved expected path. This prevents
	// false-positive "wrong target" reports in status/doctor and stops link
	// from destroying an intentional intermediate symlink layer.
	if resolvedDest, destErr := filepath.EvalSymlinks(dest); destErr == nil {
		if resolvedExpected, expErr := filepath.EvalSymlinks(expected); expErr == nil {
			if resolvedDest == resolvedExpected {
				return symlinkCorrect, info, actual, nil
			}
		}
	}
	return symlinkWrong, info, actual, nil
}
