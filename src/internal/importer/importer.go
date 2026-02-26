// Package importer handles copying existing skills into the Axon Hub,
// applying exclude filtering and MD5-based conflict resolution.
package importer

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ConflictPair records a conflict found during import.
type ConflictPair struct {
	Original string // path of the file already in the Hub
	Conflict string // path where the incoming conflicting version was stored
	Tool     string // source tool name
}

// Result is returned by ImportDir.
type Result struct {
	Conflicts []ConflictPair
	Imported  int // number of files actually copied
	Skipped   int // identical duplicates skipped

	// Skill-level counts (a "skill" is a top-level subdirectory of srcDir).
	SkillsImported  int // skills with ≥1 newly copied file
	SkillsSkipped   int // skills whose every file was an identical duplicate
	SkillsConflicts int // skills with ≥1 conflict
}

// ImportDir copies files from srcDir into dstDir, applying excludes and MD5
// conflict resolution.  toolName is used to build conflict file names.
func ImportDir(srcDir, dstDir, toolName string, excludes []string) (*Result, error) {
	result := &Result{}

	// Skill-level outcome sets — key is the top-level child name (skill dir).
	skillImported := map[string]bool{}
	skillSkipped  := map[string]bool{}
	skillConflict := map[string]bool{}

	err := filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Skip the root itself.
		if path == srcDir {
			return nil
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// ── Exclude filtering (Layer 1 guard) ────────────────────────────────
		if matchesExclude(rel, excludes) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		dst := filepath.Join(dstDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}

		// Top-level component = skill name (files at root get key ".").
		skillKey := strings.SplitN(rel, string(filepath.Separator), 2)[0]

		// ── MD5 conflict resolution ───────────────────────────────────────────
		if _, err := os.Stat(dst); err == nil {
			// Destination file already exists — compare fingerprints.
			srcMD5, err := fileMD5(path)
			if err != nil {
				return fmt.Errorf("md5 %s: %w", path, err)
			}
			dstMD5, err := fileMD5(dst)
			if err != nil {
				return fmt.Errorf("md5 %s: %w", dst, err)
			}
			if srcMD5 == dstMD5 {
				// Identical — skip silently.
				result.Skipped++
				skillSkipped[skillKey] = true
				return nil
			}
			// Different content — conflict-safe write.
			conflictDst := conflictPath(dst, toolName)
			if err := copyFile(path, conflictDst); err != nil {
				return fmt.Errorf("conflict copy %s → %s: %w", path, conflictDst, err)
			}
			result.Conflicts = append(result.Conflicts, ConflictPair{
				Original: dst,
				Conflict: conflictDst,
				Tool:     toolName,
			})
			result.Imported++
			skillConflict[skillKey] = true
			return nil
		}

		// Destination file does not exist — plain copy.
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyFile(path, dst); err != nil {
			return fmt.Errorf("copy %s → %s: %w", path, dst, err)
		}
		result.Imported++
		skillImported[skillKey] = true
		return nil
	})
	if err != nil {
		return result, err
	}

	// ── Derive skill-level counts ─────────────────────────────────────────────
	// A skill is "imported" if it had ≥1 new file.
	// A skill is "skipped"  if every file was a duplicate (no new, no conflict).
	// A skill is "conflict" if it had ≥1 conflicting file.
	// Note: categories can overlap (new + conflict in same skill).
	result.SkillsImported = len(skillImported)
	result.SkillsConflicts = len(skillConflict)
	for s := range skillSkipped {
		if !skillImported[s] && !skillConflict[s] {
			result.SkillsSkipped++
		}
	}

	return result, nil
}

// conflictPath builds the conflict filename for an incoming file.
// Strategy: insert .conflict-<tool> before the final extension.
//
//	oracle_expert.md         → oracle_expert.conflict-antigravity.md
//	oracle_expert.prompt.md  → oracle_expert.prompt.conflict-antigravity.md
func conflictPath(original, tool string) string {
	ext := filepath.Ext(original)
	base := strings.TrimSuffix(original, ext)
	return base + ".conflict-" + tool + ext
}

// matchesExclude reports whether relPath matches any of the given glob patterns.
func matchesExclude(relPath string, patterns []string) bool {
	name := filepath.Base(relPath)
	for _, pattern := range patterns {
		// Match against the full relative path AND just the basename.
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
	}
	return false
}

// fileMD5 returns the hex-encoded MD5 digest of the file at path.
func fileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// copyFile copies src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
