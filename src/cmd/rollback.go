package cmd

import (
	"fmt"
	"strings"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/spf13/cobra"
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback [skill-name]",
	Short: "Roll back a skill or the entire Hub to a previous Git state",
	Long: `Roll back a skill directory or the entire Hub to a previous commit.

Examples:
  axon rollback humanizer                    # revert humanizer to the previous commit
  axon rollback humanizer --revision abc123  # revert humanizer to a specific SHA
  axon rollback --all                        # revert entire Hub one commit back
  axon rollback --all --revision abc123      # revert entire Hub to a specific SHA

The command refuses to run if there are uncommitted changes in the Hub.
After rolling back, run 'axon sync' to propagate the change to other machines.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRollback,
}

var (
	rollbackAll      bool
	rollbackRevision string
)

func init() {
	rollbackCmd.Flags().BoolVar(&rollbackAll, "all", false, "Roll back the entire Hub (not a single skill)")
	rollbackCmd.Flags().StringVar(&rollbackRevision, "revision", "", "Target Git SHA, tag, or branch")
	rootCmd.AddCommand(rollbackCmd)
}

func runRollback(cmd *cobra.Command, args []string) error {
	if err := checkGitAvailable(); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	// Validate flag combinations.
	if rollbackAll && len(args) > 0 {
		return fmt.Errorf("--all and a skill name are mutually exclusive")
	}
	if !rollbackAll && len(args) == 0 {
		return fmt.Errorf("specify a skill name or use --all\n\n  axon rollback <skill>          # revert one skill\n  axon rollback --all            # revert entire Hub")
	}

	// Safety: refuse to operate on a dirty repo.
	dirty, err := gitIsDirty(cfg.RepoPath)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("uncommitted changes in Hub — please commit or stash first\n  Run: git -C %s status", cfg.RepoPath)
	}

	if rollbackAll {
		return rollbackHubAll(cfg.RepoPath, rollbackRevision)
	}
	return rollbackSkill(cfg.RepoPath, args[0], rollbackRevision)
}

// rollbackSkill reverts a single skill directory to a previous commit.
func rollbackSkill(repoPath, skillName, revision string) error {
	// Resolve the skill path relative to the repo root.
	skillPath, err := resolveSkillPath(repoPath, skillName)
	if err != nil {
		return err
	}

	// Determine the target SHA.
	var targetSHA string
	if revision != "" {
		// Validate that the revision exists.
		sha, err := gitOutput(repoPath, "rev-parse", "--verify", revision+"^{commit}")
		if err != nil {
			return fmt.Errorf("unknown revision %q: %w", revision, err)
		}
		targetSHA = strings.TrimSpace(sha)
	} else {
		// Default: the commit just before the most recent commit touching this skill.
		entries, err := gitLogEntries(repoPath, skillPath, 1, 1)
		if err != nil || len(entries) == 0 {
			return fmt.Errorf("no previous version found for skill %q\n  (It may have only one commit, or the path may be incorrect.)", skillName)
		}
		targetSHA = entries[0].fullSHA
	}

	// Fetch commit info for Current and Target.
	currentInfo, err := gitCommitInfo(repoPath, "HEAD", skillPath)
	if err != nil {
		return fmt.Errorf("cannot read current commit info: %w", err)
	}
	targetInfo, err := gitCommitInfo(repoPath, targetSHA, skillPath)
	if err != nil {
		return fmt.Errorf("cannot read target commit info: %w", err)
	}

	// Print the summary block.
	fmt.Println("\n[ Rollback ]")
	fmt.Printf("  Skill:   %s\n", skillName)
	fmt.Printf("  Current: %s (%s)\n", currentInfo.subject, currentInfo.date)
	fmt.Printf("  Target:  %s (%s)\n", targetInfo.subject, targetInfo.date)
	fmt.Println()
	fmt.Printf("  Reverting %s...", skillPath)

	// Restore the skill tree to the target SHA.
	if err := gitRun("-C", repoPath, "checkout", targetSHA, "--", skillPath); err != nil {
		fmt.Println(" FAILED")
		return fmt.Errorf("git checkout failed: %w", err)
	}
	fmt.Println(" DONE")

	// Create a rollback commit.
	shortSHA := targetSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	msg := fmt.Sprintf("axon: rollback %s to %s", skillName, shortSHA)
	if err := gitRun("-C", repoPath, "add", skillPath); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}
	if err := gitRun("-C", repoPath, "commit", "-m", msg); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	printOK("", fmt.Sprintf("Skill %q rolled back to %s. Run 'axon sync' to propagate.", skillName, shortSHA))
	return nil
}

// ── Hub-wide rollback ─────────────────────────────────────────────────────────

// rollbackHubAll reverts the entire Hub to the state before HEAD (or before a
// specific revision) by creating a new forward revert commit — never rewriting
// history, so axon sync can safely push the result to origin.
func rollbackHubAll(repoPath, revision string) error {
	// Determine the target state (the commit whose content we want to restore).
	var targetSHA string
	if revision != "" {
		sha, err := gitOutput(repoPath, "rev-parse", "--verify", revision+"^{commit}")
		if err != nil {
			return fmt.Errorf("unknown revision %q: %w", revision, err)
		}
		targetSHA = strings.TrimSpace(sha)
	} else {
		sha, err := gitOutput(repoPath, "rev-parse", "HEAD~1")
		if err != nil {
			return fmt.Errorf("cannot resolve HEAD~1 (Hub may have only one commit): %w", err)
		}
		targetSHA = strings.TrimSpace(sha)
	}

	shortSHA := targetSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}

	// Show what we're about to do.
	currentInfo, err := gitCommitInfo(repoPath, "HEAD", "")
	if err != nil {
		return fmt.Errorf("cannot read current commit info: %w", err)
	}
	targetInfo, err := gitCommitInfo(repoPath, targetSHA, "")
	if err != nil {
		return fmt.Errorf("cannot read target commit info: %w", err)
	}

	// Warn if the last commit isn't an axon: commit (i.e. user-made).
	if !strings.HasPrefix(currentInfo.subject, "axon:") {
		printWarn("", fmt.Sprintf("Current HEAD is a non-axon commit: %q", currentInfo.subject))
		printWarn("", "Reverting it will undo its changes.")
	}

	fmt.Println("\n[ Rollback Hub ]")
	fmt.Printf("  Current: %s (%s)\n", currentInfo.subject, currentInfo.date)
	fmt.Printf("  Target:  %s (%s)\n", targetInfo.subject, targetInfo.date)
	fmt.Println()
	fmt.Print("  Reverting Hub...")

	// Build the revert range:
	//   default:      revert only HEAD         → "HEAD"
	//   --revision:   revert targetSHA..HEAD   → all commits above targetSHA
	revertRange := "HEAD"
	if revision != "" {
		revertRange = targetSHA + "..HEAD"
	}

	// Stage the reversal(s) without committing so we can write a custom message.
	if err := gitRun("-C", repoPath, "revert", "--no-commit", revertRange); err != nil {
		fmt.Println(" FAILED")
		_ = gitRun("-C", repoPath, "revert", "--abort")
		return fmt.Errorf("git revert failed: %w\n  (A conflict may have occurred; the revert has been aborted.)", err)
	}

	msg := fmt.Sprintf("axon: rollback hub to %s", shortSHA)
	if err := gitRun("-C", repoPath, "commit", "-m", msg); err != nil {
		fmt.Println(" FAILED")
		_ = gitRun("-C", repoPath, "revert", "--abort")
		return fmt.Errorf("git commit failed: %w", err)
	}
	fmt.Println(" DONE")

	if sha, err := gitCurrentSHA(repoPath); err == nil {
		printOK("", fmt.Sprintf("Hub rolled back to %s. Run 'axon sync' to propagate.", sha))
	} else {
		printOK("", "Hub rolled back. Run 'axon sync' to propagate.")
	}
	return nil
}
