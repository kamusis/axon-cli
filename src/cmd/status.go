package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Validate symlinks and show Hub Git status",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	// Sort targets alphabetically by name.
	targets := make([]config.Target, len(cfg.Targets))
	copy(targets, cfg.Targets)
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})

	fmt.Println("=== Symlink Health ===")

	var linked, needLink, realDir, broken []string
	notInstalledMap := make(map[string]bool)
	var notInstalled []string
	var notInstalledCount int

	for _, t := range targets {
		dest, err := config.ExpandPath(t.Destination)
		if err != nil {
			broken = append(broken, fmt.Sprintf("  ✗  [%s] cannot expand path: %v", t.Name, err))
			continue
		}

		// Check parent dir first — if missing, the tool is not installed at all.
		parent := filepath.Dir(dest)
		if _, parentErr := os.Stat(parent); os.IsNotExist(parentErr) {
			notInstalledCount++
			baseName := t.Name
			if idx := strings.LastIndex(t.Name, "-"); idx != -1 {
				baseName = t.Name[:idx]
			}
			if !notInstalledMap[baseName] {
				notInstalledMap[baseName] = true
				notInstalled = append(notInstalled, baseName)
			}
			continue
		}

		expected := filepath.Join(cfg.RepoPath, t.Source)
		info, err := os.Lstat(dest)

		switch {
		case os.IsNotExist(err):
			needLink = append(needLink, fmt.Sprintf("  -  [%s] not linked  (run: axon link %s)", t.Name, t.Name))

		case err != nil:
			broken = append(broken, fmt.Sprintf("  ✗  [%s] stat error: %v", t.Name, err))

		case info.Mode()&os.ModeSymlink == 0:
			realDir = append(realDir, fmt.Sprintf("  !  [%s] real directory — run 'axon link %s' to convert (original will be backed up)", t.Name, t.Name))

		default:
			target, err := os.Readlink(dest)
			if err != nil {
				broken = append(broken, fmt.Sprintf("  ✗  [%s] cannot read symlink: %v", t.Name, err))
			} else if target != expected {
				broken = append(broken, fmt.Sprintf("  ✗  [%s] wrong target:\n      got:  %s\n      want: %s", t.Name, target, expected))
			} else {
				linked = append(linked, fmt.Sprintf("  ✓  [%s]", t.Name))
			}
		}
	}

	// Print grouped output.
	if len(linked) > 0 {
		fmt.Println("\n● Linked (healthy symlinks):")
		for _, s := range linked {
			fmt.Println(s)
		}
	}
	if len(realDir) > 0 {
		fmt.Println("\n● Real directories (not yet converted to symlinks):")
		for _, s := range realDir {
			fmt.Println(s)
		}
	}
	if len(needLink) > 0 {
		fmt.Println("\n● Installed but not linked:")
		for _, s := range needLink {
			fmt.Println(s)
		}
	}
	if len(broken) > 0 {
		fmt.Println("\n● Errors:")
		for _, s := range broken {
			fmt.Println(s)
		}
	}
	if len(notInstalled) > 0 {
		fmt.Println("\n● Not installed (skipped):")
		sort.Strings(notInstalled)
		for _, s := range notInstalled {
			fmt.Printf("  ○  %s\n", s)
		}
	}

	total := len(targets)
	fmt.Printf("\n  %d linked / %d real dir / %d not linked / %d not installed (tools) / %d error  (total: %d targets)\n",
		len(linked), len(realDir), len(needLink), len(notInstalled), len(broken), total)

	fmt.Println("\n=== Hub Git Status ===")
	out, err := exec.Command("git", "-C", cfg.RepoPath, "-c", "advice.statusHints=false", "status").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("git status failed:\n%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return fmt.Errorf("git status failed: %w", err)
	}
	fmt.Print(string(out))
	return nil
}
