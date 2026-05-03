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
	if err := windowsSymlinkPreflight(); err != nil {
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
				printBackup(r.name, r.detail)
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
			printBackup(r.name, r.detail)
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
			printSkip("", name)
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

// linkTarget applies the linking logic for a single target.
// Returns (state, detail, notInstalledToolName).
// If notInstalledToolName is non-empty, the tool is not installed and the
// caller should group it separately; state/detail are meaningless in that case.
//
// File-type targets (t.Type == "file") link a single Hub file to a destination
// path with a possibly different filename (e.g. global_rules.md → ~/.codex/AGENTS.md).
// Directory-type targets (default) link a Hub directory to a destination directory.
func linkTarget(cfg *config.Config, t config.Target) (state, detail, notInstalled string) {
	dest, err := config.ExpandPath(t.Destination)
	if err != nil {
		return "error", err.Error(), ""
	}
	hubPath := filepath.Join(cfg.RepoPath, t.Source)

	if t.IsFile() {
		return linkFileTarget(cfg, t, hubPath, dest)
	}
	return linkDirectoryTarget(cfg, t, hubPath, dest)
}

// linkDirectoryTarget handles the linking logic when the target source is a directory.
func linkDirectoryTarget(cfg *config.Config, t config.Target, hubPath, dest string) (state, detail, notInstalled string) {
	// Ensure Hub source directory exists.
	if err := os.MkdirAll(hubPath, 0o755); err != nil {
		return "error", fmt.Sprintf("cannot create hub path: %v", err), ""
	}

	info, lstatErr := os.Lstat(dest)

	// Case: dest does not exist.
	if os.IsNotExist(lstatErr) {
		if name := notInstalledToolName(t, dest); name != "" {
			return "", "", name
		}
		if err := createSymlink(hubPath, dest, t.Name); err != nil {
			return "error", err.Error(), ""
		}
		return "linked", fmt.Sprintf("%s → %s", dest, hubPath), ""
	}
	if lstatErr != nil {
		return "error", fmt.Sprintf("stat: %v", lstatErr), ""
	}

	// Case: dest is a symlink.
	if info.Mode()&os.ModeSymlink != 0 {
		return relinkIfWrong(hubPath, dest, t.Name)
	}

	// Case: dest is a real path. Directory targets only accept directories here.
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
	bkp, err := backupPath(cfg, t.Name)
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

// linkFileTarget handles the linking logic when the target source is a single file.
// The Hub side ensures the source file's parent directory exists and creates an
// empty placeholder file if the source is missing (so the symlink is never
// dangling). The destination side handles the same not-exist / symlink /
// real-file cases as directory targets, but a real directory at the destination
// is treated as an unexpected error rather than backed up.
func linkFileTarget(cfg *config.Config, t config.Target, hubPath, dest string) (state, detail, notInstalled string) {
	// Ensure Hub parent dir exists.
	if err := os.MkdirAll(filepath.Dir(hubPath), 0o755); err != nil {
		return "error", fmt.Sprintf("cannot create hub path: %v", err), ""
	}
	// Touch the Hub file if it doesn't exist so the symlink resolves.
	if _, err := os.Stat(hubPath); os.IsNotExist(err) {
		f, err := os.OpenFile(hubPath, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			return "error", fmt.Sprintf("cannot create hub file: %v", err), ""
		}
		_ = f.Close()
	} else if err != nil {
		return "error", fmt.Sprintf("stat hub file: %v", err), ""
	}

	info, lstatErr := os.Lstat(dest)

	// Case: dest does not exist.
	if os.IsNotExist(lstatErr) {
		if name := notInstalledToolName(t, dest); name != "" {
			return "", "", name
		}
		if err := createSymlink(hubPath, dest, t.Name); err != nil {
			return "error", err.Error(), ""
		}
		return "linked", fmt.Sprintf("%s → %s", dest, hubPath), ""
	}
	if lstatErr != nil {
		return "error", fmt.Sprintf("stat: %v", lstatErr), ""
	}

	// Case: dest is a symlink.
	if info.Mode()&os.ModeSymlink != 0 {
		return relinkIfWrong(hubPath, dest, t.Name)
	}

	// Case: dest is a real directory at a file-type destination — refuse.
	if info.IsDir() {
		return "error", fmt.Sprintf("%s is a directory but target type is file", dest), ""
	}

	// Case: dest is a real file — backup then link.
	bkp, err := backupPath(cfg, t.Name)
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

// notInstalledToolName returns the tool's base name when the destination's
// parent directory does not exist (signal that the tool is not installed),
// or empty string when the parent exists.
func notInstalledToolName(t config.Target, dest string) string {
	parent := filepath.Dir(dest)
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		return ""
	}
	baseName := t.Name
	if idx := strings.LastIndex(t.Name, "-"); idx != -1 {
		baseName = t.Name[:idx]
	}
	return baseName
}

// relinkIfWrong handles a destination that is already a symlink: returns
// "already" when it points at hubPath, otherwise removes the old symlink and
// re-creates one pointing at hubPath.
func relinkIfWrong(hubPath, dest, name string) (state, detail, notInstalled string) {
	current, err := os.Readlink(dest)
	if err != nil {
		return "error", fmt.Sprintf("readlink: %v", err), ""
	}
	if current == hubPath {
		return "already", "", ""
	}
	if err := os.Remove(dest); err != nil {
		return "error", fmt.Sprintf("cannot remove old symlink: %v", err), ""
	}
	if err := createSymlink(hubPath, dest, name); err != nil {
		return "error", err.Error(), ""
	}
	return "relinked", fmt.Sprintf("was → %s", current), ""
}

// createSymlink creates dest → hub.
// On Windows, symlink permission is verified upfront by windowsSymlinkPreflight
// before this function is ever called.
func createSymlink(hub, dest, name string) error {
	_ = name
	if err := os.Symlink(hub, dest); err != nil {
		return fmt.Errorf("symlink %s → %s: %w", dest, hub, err)
	}
	return nil
}

// backupPath returns (and ensures the parent of) the timestamped backup path
// for a target. The path shape is the same for both file and directory
// targets: ~/.axon/backups/{targetName}_{timestamp}.
func backupPath(_ *config.Config, targetName string) (string, error) {
	axonDir, err := config.AxonDir()
	if err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102150405")
	path := filepath.Join(axonDir, "backups", targetName+"_"+ts)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("cannot create backups dir: %w", err)
	}
	return path, nil
}
