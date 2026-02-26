package cmd

import (
	"fmt"
	"strings"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/spf13/cobra"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage the Hub's remote Git repository",
}

var remoteSetCmd = &cobra.Command{
	Use:   "set <url>",
	Short: "Set (or update) the remote origin URL",
	Long: `Configure the Git remote origin for the Axon Hub.

If no remote is currently set, it adds 'origin'.
If a remote already exists, it updates the URL.

Run 'axon sync' afterwards to push local content to the remote.

Examples:
  axon remote set https://github.com/you/axon-hub.git
  axon remote set git@github.com:you/axon-hub.git`,
	Args: cobra.ExactArgs(1),
	RunE: runRemoteSet,
}

func init() {
	remoteCmd.AddCommand(remoteSetCmd)
	rootCmd.AddCommand(remoteCmd)
}

func runRemoteSet(cmd *cobra.Command, args []string) error {
	if err := checkGitAvailable(); err != nil {
		return err
	}
	url := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}
	repo := cfg.RepoPath

	existing, err := gitOutput(repo, "remote", "get-url", "origin")
	existing = strings.TrimSpace(existing)

	if err != nil || existing == "" {
		if err := gitRun("-C", repo, "remote", "add", "origin", url); err != nil {
			return fmt.Errorf("git remote add failed: %w", err)
		}
		printOK("", fmt.Sprintf("Remote origin added: %s", url))
	} else if existing == url {
		printSkip("", fmt.Sprintf("Remote origin already set to: %s", url))
	} else {
		if err := gitRun("-C", repo, "remote", "set-url", "origin", url); err != nil {
			return fmt.Errorf("git remote set-url failed: %w", err)
		}
		printOK("", fmt.Sprintf("Remote origin updated: %s → %s", existing, url))
	}

	// Best-effort: fetch origin and set origin/HEAD to the remote's default branch.
	// This improves UX for commands that rely on origin/HEAD (e.g. status --fetch).
	if out, err := gitOutput(repo, "fetch", "--prune", "origin"); err != nil {
		printWarn("", fmt.Sprintf("git fetch origin failed; remote default branch may be unknown:\n%s", strings.TrimSpace(out)))
	}
	if err := gitRun("-C", repo, "remote", "set-head", "origin", "-a"); err != nil {
		printWarn("", "could not set origin/HEAD automatically; remote default branch may be unknown")
	}

	printInfo("", "Run 'axon sync' to push local content to the remote.")
	return nil
}
