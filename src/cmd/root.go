package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var flagVersion bool

var rootCmd = &cobra.Command{
	Use:           "axon",
	Short:         "Axon CLI — Hub-and-Spoke skill manager for AI editors",
	SilenceUsage:  true, // don't print usage on operational errors
	SilenceErrors: true, // we'll print errors once in Execute()
	Long: `Axon keeps your AI-editor skills and workflows in sync across machines
using a central Git-backed Hub at ~/.axon/repo/.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if flagVersion {
			fmt.Fprintln(os.Stdout, version)
			os.Exit(0)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagVersion {
			fmt.Fprintln(os.Stdout, version)
			return nil
		}
		return cmd.Help()
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&flagVersion, "version", "v", false, "Print axon version and exit")
}

// checkGitAvailable returns a clear error if git is not found on PATH.
func checkGitAvailable() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is not installed or not on PATH\n" +
			"  Axon requires git to manage the Hub repository.\n" +
			"  Install git from https://git-scm.com and try again.")
	}
	return nil
}

// Execute is called by main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// gitRun executes a git sub-command and streams output to stdout/stderr.
// It is a thin convenience wrapper used by multiple commands.
func gitRun(args ...string) error {
	c := exec.Command("git", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
