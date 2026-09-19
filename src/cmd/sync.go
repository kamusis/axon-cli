package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/spf13/cobra"
)

var syncVerbose bool

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
	syncCmd.Flags().BoolVar(&syncVerbose, "verbose", false, "Show detailed Git output and individual file diffs")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	if err := checkGitAvailable(); err != nil {
		return err
	}
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

	verbose, _ := cmd.Flags().GetBool("verbose")

	switch cfg.SyncMode {
	case "read-only":
		return syncReadOnly(cfg, verbose)
	default:
		return syncReadWrite(cfg, verbose)
	}
}

// currentBranchName returns the current checked-out Git branch name, defaulting to "master".
func currentBranchName(repo string) string {
	branch, err := gitOutput(repo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "master"
	}
	b := strings.TrimSpace(branch)
	if b == "" || b == "HEAD" {
		return "master"
	}
	return b
}

// syncReadWrite: filter → add → commit → pull --rebase → push
func syncReadWrite(cfg *config.Config, verbose bool) error {
	repo := cfg.RepoPath

	identityOK, identityErr := gitIdentityConfigured(repo)
	if identityErr != nil {
		return identityErr
	}
	if !identityOK {
		return fmt.Errorf(
			"Author identity unknown\n\n" +
				"*** Please tell me who you are.\n\n" +
				"Run\n\n" +
				"  git config --global user.email \"you@example.com\"\n" +
				"  git config --global user.name \"Your Name\"\n\n" +
				"to set your account's default identity.\n" +
				"Omit --global to set the identity only in this repository.")
	}

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

	branch := currentBranchName(repo)

	// Stage local files
	if verbose {
		printInfo("", "git add .")
	}
	if addOut, err := gitOutput(repo, "add", "."); err != nil {
		return fmt.Errorf("git add failed: %w\n%s", err, addOut)
	}

	// Commit local changes if working tree was dirty
	hostname, _ := os.Hostname()
	msg := fmt.Sprintf("axon: sync from %s", hostname)
	if verbose {
		printInfo("", fmt.Sprintf("git commit -m %q", msg))
	}
	commitOut, commitErr := gitOutput(repo, "commit", "-m", msg)
	if commitErr != nil {
		if !strings.Contains(commitOut, "nothing to commit") &&
			!strings.Contains(commitOut, "nothing added to commit") {
			return fmt.Errorf("git commit failed: %w\n%s", commitErr, commitOut)
		}
	}

	if !hasRemote {
		// If there is no remote configured, show what was committed locally.
		if commitErr == nil {
			diffOut, _ := gitOutput(repo, "diff", "--name-status", "HEAD~1", "HEAD")
			localAssets := parseDiffNameStatus(diffOut)
			if len(localAssets) > 0 {
				fmt.Printf("\n↑ Local commit (no remote configured):\n")
				printAssetCategoryGroups(os.Stdout, localAssets, verbose)
			}
		}
		printOK("", "Local commit done (no remote configured; run 'axon remote set <url>' to push).")
		return nil
	}

	// Detect whether the remote has any commits yet (empty repo = first push).
	remoteEmpty := gitRemoteIsEmpty(repo)
	if remoteEmpty {
		if verbose {
			printInfo("", fmt.Sprintf("git push -u origin %s (initial push to empty remote)", branch))
		}
		diffOut, _ := gitOutput(repo, "diff", "--name-status", "--root", "HEAD")
		pushedAssets := parseDiffNameStatus(diffOut)

		pushOut, pushErr := gitOutput(repo, "push", "-u", "origin", branch)
		if pushErr != nil {
			return fmt.Errorf("git push failed: %w\n%s", pushErr, pushOut)
		}
		if len(pushedAssets) > 0 {
			fmt.Printf("\n↑ Pushed to remote (initial push):\n")
			printAssetCategoryGroups(os.Stdout, pushedAssets, verbose)
		}
		printOK("", "Sync complete (initial push).")
		return nil
	}

	// Step 1: Quietly fetch remote updates to inspect incoming changes.
	if verbose {
		printInfo("", fmt.Sprintf("git fetch --prune origin %s", branch))
	}
	fetchOut, fetchErr := gitOutput(repo, "fetch", "--prune", "origin", branch)
	if fetchErr != nil {
		return fmt.Errorf("git fetch failed: %w\n%s", fetchErr, fetchOut)
	}

	fetchHead, err := gitOutput(repo, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return fmt.Errorf("git rev-parse FETCH_HEAD: %w", err)
	}
	fetchHead = strings.TrimSpace(fetchHead)

	currentHead, err := gitOutput(repo, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	currentHead = strings.TrimSpace(currentHead)

	base, err := gitOutput(repo, "merge-base", currentHead, fetchHead)
	if err != nil {
		return fmt.Errorf("git merge-base failed: %w", err)
	}
	base = strings.TrimSpace(base)

	// Calculate incoming changes from remote (base..FETCH_HEAD)
	var pulledAssets []CategoryGroup
	pullCount := 0
	if base != fetchHead {
		remoteDiff, _ := gitOutput(repo, "diff", "--name-status", base, fetchHead)
		pulledAssets = parseDiffNameStatus(remoteDiff)
		countOut, _ := gitOutput(repo, "rev-list", "--count", base+".."+fetchHead)
		pullCount, _ = strconv.Atoi(strings.TrimSpace(countOut))
	}

	// Calculate outgoing local changes (base..HEAD)
	var pushedAssets []CategoryGroup
	pushCount := 0
	if base != currentHead {
		localDiff, _ := gitOutput(repo, "diff", "--name-status", base, currentHead)
		pushedAssets = parseDiffNameStatus(localDiff)
		countOut, _ := gitOutput(repo, "rev-list", "--count", base+".."+currentHead)
		pushCount, _ = strconv.Atoi(strings.TrimSpace(countOut))
	}

	// If neither side has changes, Hub is already up to date
	if len(pulledAssets) == 0 && len(pushedAssets) == 0 {
		printOK("", fmt.Sprintf("Hub is already up to date with origin/%s.", branch))
		return nil
	}

	// Step 2: Rebase onto FETCH_HEAD if remote has new commits
	autoResolved := false
	if base != fetchHead {
		if verbose {
			printInfo("", fmt.Sprintf("git rebase -X theirs FETCH_HEAD"))
		}
		rebaseOut, rebaseErr := gitOutput(repo, "rebase", "-X", "theirs", "FETCH_HEAD")
		if rebaseErr != nil {
			if verbose {
				printWarn("", "rebase auto-resolve failed; aborting and retrying with merge strategy")
			}
			_ = gitRun("-C", repo, "rebase", "--abort")

			if verbose {
				printInfo("", fmt.Sprintf("git merge -X theirs origin/%s", branch))
			}
			mergeOut, mergeErr := gitOutput(repo, "merge", "-X", "theirs", "FETCH_HEAD")
			if mergeErr != nil {
				_ = gitRun("-C", repo, "merge", "--abort")
				return fmt.Errorf(
					"sync conflict could not be auto-resolved.\n"+
						"   Please resolve manually in %s:\n"+
						"     git -C %s status\n"+
						"   Then commit and run 'axon sync' again.\n%s", repo, repo, mergeOut)
			}
			autoResolved = true
		} else if strings.Contains(rebaseOut, "CONFLICT") || strings.Contains(rebaseOut, "Auto-merging") {
			autoResolved = true
		}
	}

	// Step 3: Push to remote if local has unpushed commits
	if base != currentHead {
		if verbose {
			printInfo("", fmt.Sprintf("git push origin %s", branch))
		}
		pushOut, pushErr := gitOutput(repo, "push", "origin", branch)
		if pushErr != nil {
			return fmt.Errorf("git push failed: %w\n%s", pushErr, pushOut)
		}
	}

	// Step 4: Display clean, structured summaries
	if autoResolved {
		printWarn("", "Auto-resolved conflicts in favor of remote")
	}

	if len(pulledAssets) > 0 {
		commitLabel := "commit"
		if pullCount > 1 {
			commitLabel = "commits"
		}
		fmt.Printf("\n↓ Pulled from remote (%d %s):\n", pullCount, commitLabel)
		printAssetCategoryGroups(os.Stdout, pulledAssets, verbose)
	}

	if len(pushedAssets) > 0 {
		commitLabel := "commit"
		if pushCount > 1 {
			commitLabel = "commits"
		}
		fmt.Printf("\n↑ Pushed to remote (%d %s):\n", pushCount, commitLabel)
		printAssetCategoryGroups(os.Stdout, pushedAssets, verbose)
	}

	numPulled := countAssets(pulledAssets)
	numPushed := countAssets(pushedAssets)

	if numPulled > 0 && numPushed > 0 {
		printOK("", fmt.Sprintf("Sync complete: %d asset(s) pulled, %d asset(s) pushed.", numPulled, numPushed))
	} else if numPulled > 0 {
		printOK("", fmt.Sprintf("Sync complete: %d asset(s) pulled, Hub is up to date.", numPulled))
	} else if numPushed > 0 {
		printOK("", fmt.Sprintf("Sync complete: %d asset(s) pushed to origin/%s.", numPushed, branch))
	} else {
		printOK("", "Sync complete (read-write).")
	}

	return nil
}

// syncReadOnly: warn on local edits, then pull fast-forward only.
func syncReadOnly(cfg *config.Config, verbose bool) error {
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

	branch := currentBranchName(repo)

	// Fetch remote quietly
	if verbose {
		printInfo("", fmt.Sprintf("git fetch --prune origin %s", branch))
	}
	fetchOut, fetchErr := gitOutput(repo, "fetch", "--prune", "origin", branch)
	if fetchErr != nil {
		return fmt.Errorf("git fetch failed: %w\n%s", fetchErr, fetchOut)
	}

	fetchHead, err := gitOutput(repo, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return fmt.Errorf("git rev-parse FETCH_HEAD: %w", err)
	}
	fetchHead = strings.TrimSpace(fetchHead)

	currentHead, err := gitOutput(repo, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	currentHead = strings.TrimSpace(currentHead)

	if currentHead == fetchHead {
		printOK("", fmt.Sprintf("Hub is already up to date with origin/%s.", branch))
		return nil
	}

	remoteDiff, _ := gitOutput(repo, "diff", "--name-status", currentHead, fetchHead)
	pulledAssets := parseDiffNameStatus(remoteDiff)
	countOut, _ := gitOutput(repo, "rev-list", "--count", currentHead+".."+fetchHead)
	pullCount, _ := strconv.Atoi(strings.TrimSpace(countOut))

	if verbose {
		printInfo("", fmt.Sprintf("git merge --ff-only FETCH_HEAD"))
	}
	pullOut, pullErr := gitOutput(repo, "merge", "--ff-only", "FETCH_HEAD")
	if pullErr != nil {
		return fmt.Errorf("git pull failed (fast-forward only enforced in read-only mode): %w\n%s", pullErr, pullOut)
	}

	if len(pulledAssets) > 0 {
		commitLabel := "commit"
		if pullCount > 1 {
			commitLabel = "commits"
		}
		fmt.Printf("\n↓ Pulled from remote (%d %s):\n", pullCount, commitLabel)
		printAssetCategoryGroups(os.Stdout, pulledAssets, verbose)
	}

	numPulled := countAssets(pulledAssets)
	printOK("", fmt.Sprintf("Sync complete (read-only): %d asset(s) pulled.", numPulled))
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

// stripNestedGitDirs walks the Hub working tree and removes any .git directory
// that does NOT belong to the root repo itself.
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
