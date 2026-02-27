package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/spf13/cobra"
)

var linkCmd = &cobra.Command{
	Use:   "link [target-name | all]",
	Short: "Create symlinks from tool destinations to the Hub",
	Long: `Create symbolic links so each AI tool's skill/workflow/command directory
points to the central Hub at ~/.axon/repo/.

  axon link              Link all targets defined in axon.yaml (default)
  axon link all          Same as above
  axon link windsurf-skills  Link a single target by name`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLink,
}

func init() {
	rootCmd.AddCommand(linkCmd)
}

func runLink(cmd *cobra.Command, args []string) error {
	if err := checkGitAvailable(); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	// Determine which targets to process.
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
	type linkResult struct {
		name   string
		state  string // "linked","already","relinked","backed_up","error"
		detail string
	}
	var results []linkResult
	notInstalledMap := make(map[string]bool)

	for _, t := range targets {
		state, detail, notInstalled := linkTarget(cfg, t)
		if notInstalled != "" {
			notInstalledMap[notInstalled] = true
			continue
		}
		results = append(results, linkResult{t.Name, state, detail})
	}

	// ── Print results ──────────────────────────────────────────────────────────
	if singleTarget {
		if len(results) == 1 {
			r := results[0]
			switch r.state {
			case "linked":
				printOK(r.name, r.detail)
			case "already":
				printSkip(r.name, "already linked")
			case "relinked":
				printInfo(r.name, "re-linked ("+r.detail+")")
			case "backed_up":
				printOK(r.name, r.detail)
			case "error":
				printErr(r.name, r.detail)
				return fmt.Errorf("link failed")
			}
		}
		return nil
	}

	// Multi-target: grouped sections.
	printSection("Link")

	var linked, already, relinked, backedUp, errors []linkResult
	for _, r := range results {
		switch r.state {
		case "linked":
			linked = append(linked, r)
		case "already":
			already = append(already, r)
		case "relinked":
			relinked = append(relinked, r)
		case "backed_up":
			backedUp = append(backedUp, r)
		case "error":
			errors = append(errors, r)
		}
	}

	if len(linked) > 0 {
		printBullet("Linked:")
		for _, r := range linked {
			printOK(r.name, r.detail)
		}
	}
	if len(backedUp) > 0 {
		printBullet("Linked (original backed up):")
		for _, r := range backedUp {
			printOK(r.name, r.detail)
		}
	}
	if len(relinked) > 0 {
		printBullet("Re-linked (wrong target corrected):")
		for _, r := range relinked {
			printInfo(r.name, r.detail)
		}
	}
	if len(already) > 0 {
		printBullet("Already linked:")
		for _, r := range already {
			printSkip(r.name, "")
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
			fmt.Printf("  ○  %s\n", name)
		}
	}
	if len(errors) > 0 {
		printBullet("Errors:")
		for _, r := range errors {
			printErr(r.name, r.detail)
		}
		return fmt.Errorf("%d target(s) failed to link", len(errors))
	}

	return nil
}

// linkTarget applies the 5-case linking logic for a single target.
// Returns (state, detail, notInstalledToolName).
// If notInstalledToolName is non-empty, the tool is not installed and the
// caller should group it separately; state/detail are meaningless in that case.
func linkTarget(cfg *config.Config, t config.Target) (state, detail, notInstalled string) {
	dest, err := config.ExpandPath(t.Destination)
	if err != nil {
		return "error", err.Error(), ""
	}
	hubPath := filepath.Join(cfg.RepoPath, t.Source)

	// Ensure Hub source directory exists.
	if err := os.MkdirAll(hubPath, 0o755); err != nil {
		return "error", fmt.Sprintf("cannot create hub path: %v", err), ""
	}

	info, lstatErr := os.Lstat(dest)

	// ── Case: Does not exist ───────────────────────────────────────────────────
	if os.IsNotExist(lstatErr) {
		parent := filepath.Dir(dest)
		if _, parentErr := os.Stat(parent); os.IsNotExist(parentErr) {
			baseName := t.Name
			if idx := strings.LastIndex(t.Name, "-"); idx != -1 {
				baseName = t.Name[:idx]
			}
			return "", "", baseName
		}
		if err := createSymlink(hubPath, dest, t.Name); err != nil {
			return "error", err.Error(), ""
		}
		return "linked", fmt.Sprintf("%s → %s", dest, hubPath), ""
	}
	if lstatErr != nil {
		return "error", fmt.Sprintf("stat: %v", lstatErr), ""
	}

	// ── Symlink cases ──────────────────────────────────────────────────────────
	if info.Mode()&os.ModeSymlink != 0 {
		current, err := os.Readlink(dest)
		if err != nil {
			return "error", fmt.Sprintf("readlink: %v", err), ""
		}
		if current == hubPath {
			return "already", "", ""
		}
		// Wrong symlink — remove and re-create.
		if err := os.Remove(dest); err != nil {
			return "error", fmt.Sprintf("cannot remove old symlink: %v", err), ""
		}
		if err := createSymlink(hubPath, dest, t.Name); err != nil {
			return "error", err.Error(), ""
		}
		return "relinked", fmt.Sprintf("was → %s", current), ""
	}

	// ── Real directory ─────────────────────────────────────────────────────────
	if !info.IsDir() {
		return "error", fmt.Sprintf("%s is not a directory or symlink", dest), ""
	}

	entries, err := os.ReadDir(dest)
	if err != nil {
		return "error", fmt.Sprintf("readdir: %v", err), ""
	}

	// Empty directory — remove and link.
	if len(entries) == 0 {
		if err := os.Remove(dest); err != nil {
			return "error", fmt.Sprintf("cannot remove empty dir: %v", err), ""
		}
		if err := createSymlink(hubPath, dest, t.Name); err != nil {
			return "error", err.Error(), ""
		}
		return "linked", fmt.Sprintf("%s → %s", dest, hubPath), ""
	}

	// Non-empty directory — backup then link.
	bkp, err := backupDir(cfg, t.Name)
	if err != nil {
		return "error", err.Error(), ""
	}
	if err := os.Rename(dest, bkp); err != nil {
		return "error", fmt.Sprintf("backup failed: %v", err), ""
	}
	if err := createSymlink(hubPath, dest, t.Name); err != nil {
		return "error", err.Error(), ""
	}
	return "backed_up", fmt.Sprintf("backed up → %s", bkp), ""
}

// createSymlink creates dest → hub, handling platform differences.
func createSymlink(hub, dest, name string) error {
	_ = name
	var err error
	if runtime.GOOS == "windows" {
		err = os.Symlink(hub, dest)
		if err != nil {
			return fmt.Errorf(
				"symlink failed on Windows — run 'axon doctor' for remediation.\n  Underlying error: %w", err)
		}
	} else {
		err = os.Symlink(hub, dest)
	}
	if err != nil {
		return fmt.Errorf("symlink %s → %s: %w", dest, hub, err)
	}
	return nil
}

// backupDir returns (and creates) the timestamped backup path for a target.
func backupDir(_ *config.Config, targetName string) (string, error) {
	axonDir, err := config.AxonDir()
	if err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102150405")
	dir := filepath.Join(axonDir, "backups", targetName+"_"+ts)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fmt.Errorf("cannot create backups dir: %w", err)
	}
	return dir, nil
}
