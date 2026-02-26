package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync the Hub with the remote Git repository",
	Long: `Sync behavior depends on sync_mode in axon.yaml:

  read-write (default):
    Apply exclude filtering → git add . → git commit → git pull --rebase → git push

  read-only:
    git pull (fast-forward only). Local edits are allowed but warned about.`,
	RunE: runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	// ── Apply exclude filtering (both modes) ──────────────────────────────────
	// Write excludes to .git/info/exclude — the per-repo, non-committed exclude
	// file. This is the Axon-layer guard (Layer 1) that operates independently
	// of the committed .gitignore (Layer 2).
	if err := writeGitExcludes(cfg); err != nil {
		return fmt.Errorf("cannot write git excludes: %w", err)
	}
	printOK("", fmt.Sprintf("Exclude filter applied (%d patterns)", len(cfg.Excludes)))

	switch cfg.SyncMode {
	case "read-only":
		return syncReadOnly(cfg)
	default:
		return syncReadWrite(cfg)
	}
}

// syncReadWrite: filter → add → commit → pull --rebase → push
func syncReadWrite(cfg *config.Config) error {
	repo := cfg.RepoPath

	// Check if there is a remote configured; push only if so.
	hasRemote := gitHasRemote(repo)

	// Strip any nested .git directories inside the Hub — skills are often
	// cloned from the internet and may contain their own .git dirs.
	// Leaving them in place causes git to treat them as submodules (embedded
	// repos), which breaks cross-machine sync.
	stripped, err := stripNestedGitDirs(repo)
	if err != nil {
		return fmt.Errorf("cannot strip nested .git dirs: %w", err)
	}
	if len(stripped) > 0 {
		printWarn("", fmt.Sprintf("stripped %d embedded .git dir(s) from skills (these were cloned repos):", len(stripped)))
		for _, p := range stripped {
			printInfo("", p)
		}
	}

	// git add .
	printInfo("", "git add .")
	if err := gitRun("-C", repo, "add", "."); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// git commit (skip if nothing to commit)
	hostname, _ := os.Hostname()
	msg := fmt.Sprintf("axon: sync from %s", hostname)
	printInfo("", fmt.Sprintf("git commit -m %q", msg))
	commitOut, commitErr := gitOutput(repo, "commit", "-m", msg)
	if commitErr != nil {
		if strings.Contains(commitOut, "nothing to commit") ||
			strings.Contains(commitOut, "nothing added to commit") {
			printSkip("", "nothing to commit")
		} else {
			return fmt.Errorf("git commit failed: %w\n%s", commitErr, commitOut)
		}
	}

	if !hasRemote {
		printOK("", "Local commit done (no remote configured; run 'axon remote set <url>' to push).")
		return nil
	}

	// Detect whether the remote has any commits yet (empty repo = first push).
	remoteEmpty := gitRemoteIsEmpty(repo)

	if remoteEmpty {
		// First push — no upstream branch to pull from yet.
		printInfo("", "git push -u origin master  (initial push to empty remote)")
		if err := gitRun("-C", repo, "push", "-u", "origin", "master"); err != nil {
			return fmt.Errorf("git push failed: %w", err)
		}
		printOK("", "Sync complete (initial push).")
		return nil
	}

	// git pull --rebase origin master
	printInfo("", "git pull --rebase origin master")
	if err := gitRun("-C", repo, "pull", "--rebase", "origin", "master"); err != nil {
		return fmt.Errorf(`git pull --rebase failed — this may be a merge conflict.
   Please resolve conflicts manually in %s, then run:
     git rebase --continue
   or abort with:
     git rebase --abort`, repo)
	}

	// git push origin master
	printInfo("", "git push origin master")
	if err := gitRun("-C", repo, "push", "origin", "master"); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	printOK("", "Sync complete (read-write).")
	return nil

}

// syncReadOnly: warn on local edits, then pull fast-forward only.
func syncReadOnly(cfg *config.Config) error {
	repo := cfg.RepoPath

	// Warn if there are local uncommitted edits.
	dirty, err := gitIsDirty(repo)
	if err != nil {
		return err
	}
	if dirty {
		printWarn("", "You have local edits in the Hub.")
		fmt.Println("   These will NOT be pushed (read-only mode) and may be overwritten on pull.")
		fmt.Println("   Stash or discard them if you don't need them.")
		fmt.Println()
	}

	printInfo("", "git pull --ff-only origin master")
	if err := gitRun("-C", repo, "pull", "--ff-only", "origin", "master"); err != nil {
		return fmt.Errorf("git pull failed (fast-forward only enforced in read-only mode): %w", err)
	}

	printOK("", "Sync complete (read-only).")
	return nil
}

// writeGitExcludes writes the Axon exclude patterns to .git/info/exclude,
// the per-repo non-committed exclude file analogous to .gitignore.
func writeGitExcludes(cfg *config.Config) error {
	excludeFile := filepath.Join(cfg.RepoPath, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludeFile), 0o755); err != nil {
		return err
	}

	header := "# Auto-generated by axon sync — do not edit manually.\n# Edit 'excludes:' in ~/.axon/axon.yaml instead.\n\n"
	body := strings.Join(cfg.Excludes, "\n") + "\n"

	return os.WriteFile(excludeFile, []byte(header+body), 0o644)
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

// stripNestedGitDirs walks the Hub working tree and removes any .git directory
// that does NOT belong to the root repo itself. Skills may have been cloned from
// the internet with their own .git, which confuses git into treating them as
// submodules and prevents their files from being committed to the Hub repo.
//
// For each nested .git found the function also runs `git rm --cached` to
// de-index any stale submodule entry before removal, so subsequent git add will
// add all files as regular content.
//
// Returns the relative paths of every .git directory that was removed.
func stripNestedGitDirs(repoPath string) ([]string, error) {
	rootGit := filepath.Join(repoPath, ".git")
	var stripped []string

	err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() || d.Name() != ".git" {
			return nil
		}
		// Skip the root repo's own .git.
		if path == rootGit {
			return filepath.SkipDir
		}

		// Relative path of the skill dir that owns this .git.
		skillDir := filepath.Dir(path)
		rel, err := filepath.Rel(repoPath, skillDir)
		if err != nil {
			rel = skillDir
		}

		// De-index any cached submodule entry (ignore errors — entry may not exist).
		// Use exec directly so git's "rm 'path'" stdout doesn't leak to the user.
		rmCmd := exec.Command("git", "-C", repoPath, "rm", "--cached", "-q", rel)
		_ = rmCmd.Run()

		// Remove the nested .git entirely.
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("cannot remove %s: %w", path, err)
		}
		stripped = append(stripped, rel)

		// Don't descend into the (now deleted) .git.
		return filepath.SkipDir
	})

	return stripped, err
}
