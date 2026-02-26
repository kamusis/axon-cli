package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/spf13/cobra"
)

var unlinkCmd = &cobra.Command{
	Use:   "unlink [target-name | all]",
	Short: "Remove symlinks and optionally restore from backup",
	Long: `Remove the symbolic link at each target's destination.
If a backup exists (created by axon link), the most recent backup is restored.

  axon unlink              Unlink all targets
  axon unlink windsurf-skills  Unlink a single target`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUnlink,
}

func init() {
	rootCmd.AddCommand(unlinkCmd)
}

func runUnlink(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	var targets []config.Target
	singleTarget := false
	if len(args) == 0 || args[0] == "all" {
		targets = make([]config.Target, len(cfg.Targets))
		copy(targets, cfg.Targets)
		sort.Slice(targets, func(i, j int) bool {
			return targets[i].Name < targets[j].Name
		})
	} else {
		name := args[0]
		for _, t := range cfg.Targets {
			if t.Name == name {
				targets = append(targets, t)
				break
			}
		}
		if len(targets) == 0 {
			return fmt.Errorf("target %q not found in axon.yaml", name)
		}
		singleTarget = true
	}

	// ── Collect results ────────────────────────────────────────────────────────
	type unlinkResult struct {
		name   string
		state  string // "restored", "removed", "not_symlink", "not_exist", "not_installed", "error"
		detail string
	}
	var results []unlinkResult

	notInstalledMap := make(map[string]bool)

	for _, t := range targets {
		dest, err := config.ExpandPath(t.Destination)
		if err != nil {
			results = append(results, unlinkResult{t.Name, "error", err.Error()})
			continue
		}

		// If parent doesn't exist, tool isn't installed.
		parent := filepath.Dir(dest)
		if _, parentErr := os.Stat(parent); os.IsNotExist(parentErr) {
			baseName := t.Name
			if idx := strings.LastIndex(t.Name, "-"); idx != -1 {
				baseName = t.Name[:idx]
			}
			notInstalledMap[baseName] = true
			continue
		}

		info, err := os.Lstat(dest)
		if os.IsNotExist(err) {
			results = append(results, unlinkResult{t.Name, "not_exist", ""})
			continue
		}
		if err != nil {
			results = append(results, unlinkResult{t.Name, "error",
				fmt.Sprintf("stat: %v", err)})
			continue
		}

		if info.Mode()&os.ModeSymlink == 0 {
			results = append(results, unlinkResult{t.Name, "not_symlink",
				fmt.Sprintf("%s is not a symlink", dest)})
			continue
		}

		if err := os.Remove(dest); err != nil {
			results = append(results, unlinkResult{t.Name, "error",
				fmt.Sprintf("cannot remove symlink: %v", err)})
			continue
		}

		backup, err := latestBackup(cfg, t.Name)
		if err != nil || backup == "" {
			results = append(results, unlinkResult{t.Name, "removed", "no backup found"})
			continue
		}

		if err := os.Rename(backup, dest); err != nil {
			results = append(results, unlinkResult{t.Name, "error",
				fmt.Sprintf("cannot restore backup %s: %v", backup, err)})
			continue
		}
		results = append(results, unlinkResult{t.Name, "restored",
			fmt.Sprintf("%s → %s", backup, dest)})
	}

	// ── Print results ──────────────────────────────────────────────────────────
	if singleTarget {
		if len(results) == 1 {
			r := results[0]
			switch r.state {
			case "restored":
				printOK(r.name, "restored: "+r.detail)
			case "removed":
				printSkip(r.name, "symlink removed, "+r.detail)
			case "not_exist":
				printMiss(r.name, "destination does not exist, nothing to unlink")
			case "not_symlink":
				printWarn(r.name, r.detail+" — refusing to delete real data")
			case "error":
				printErr(r.name, r.detail)
				return fmt.Errorf("unlink failed")
			}
		}
		return nil
	}

	// Multi-target: grouped sections.
	printSection("Unlink")

	var restored, removed, notExist, notSymlink, errors []unlinkResult
	for _, r := range results {
		switch r.state {
		case "restored":
			restored = append(restored, r)
		case "removed":
			removed = append(removed, r)
		case "not_exist":
			notExist = append(notExist, r)
		case "not_symlink":
			notSymlink = append(notSymlink, r)
		case "error":
			errors = append(errors, r)
		}
	}

	if len(restored) > 0 {
		printBullet("Restored from backup:")
		for _, r := range restored {
			printOK(r.name, r.detail)
		}
	}
	if len(removed) > 0 {
		printBullet("Symlink removed (no backup):")
		for _, r := range removed {
			printSkip(r.name, r.detail)
		}
	}
	if len(notExist) > 0 {
		printBullet("Nothing to unlink:")
		for _, r := range notExist {
			printMiss(r.name, "destination does not exist")
		}
	}
	if len(notSymlink) > 0 {
		printBullet("Skipped (not a symlink — real data protected):")
		for _, r := range notSymlink {
			printWarn(r.name, r.detail)
		}
	}
	if len(notInstalledMap) > 0 {
		var tools []string
		for k := range notInstalledMap {
			tools = append(tools, k)
		}
		sort.Strings(tools)
		printBullet("Not installed (skipped):")
		for _, name := range tools {
			printSkip(name, "")
		}
	}
	if len(errors) > 0 {
		printBullet("Errors:")
		for _, r := range errors {
			printErr(r.name, r.detail)
		}
		return fmt.Errorf("%d target(s) failed to unlink", len(errors))
	}

	return nil
}

// latestBackup returns the path of the most recent backup directory for a
// target, or "" if none exist.
func latestBackup(cfg *config.Config, targetName string) (string, error) {
	axonDir, err := config.AxonDir()
	if err != nil {
		return "", err
	}
	backupsDir := filepath.Join(axonDir, "backups")

	entries, err := os.ReadDir(backupsDir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	prefix := targetName + "_"
	layout := "20060102150405"

	type candidate struct {
		path string
		t    time.Time
	}
	var candidates []candidate

	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		ts := strings.TrimPrefix(e.Name(), prefix)
		t, err := time.Parse(layout, ts)
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			path: filepath.Join(backupsDir, e.Name()),
			t:    t,
		})
	}

	if len(candidates) == 0 {
		return "", nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].t.After(candidates[j].t)
	})
	return candidates[0].path, nil
}
