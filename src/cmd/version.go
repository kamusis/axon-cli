package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = ""
	buildDate = ""
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show Axon version and build information",
	RunE:  runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func runVersion(_ *cobra.Command, _ []string) error {
	fmt.Printf("Version:    %s\n", version)
	fmt.Printf("Commit:     %s\n", emptyAsNA(commit))
	fmt.Printf("Build Date: %s\n", emptyAsNA(buildDate))
	fmt.Printf("Go Version: %s\n", runtime.Version())
	fmt.Printf("OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return nil
}

func emptyAsNA(s string) string {
	if s == "" {
		return "n/a"
	}
	return s
}
