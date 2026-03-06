package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [skill-name]",
	Short: "Validate symlinks and show Hub Git status",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().Bool("fetch", false, "Fetch remote updates for the Hub repo before showing status")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	// Skill-level mode: axon status <skill-name>
	if len(args) == 1 {
		if err := checkGitAvailable(); err != nil {
			return err
		}
		fetchFirst, _ := cmd.Flags().GetBool("fetch")
		return showSkillStatus(cfg, args[0], fetchFirst)
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
	if err := checkGitAvailable(); err != nil {
		printWarn("", "git not available — skipping Hub Git status.")
		return nil
	}

	fetchFirst, _ := cmd.Flags().GetBool("fetch")
	if fetchFirst {
		// Require a configured origin remote for fetch-based checks.
		if _, originErr := exec.Command("git", "-C", cfg.RepoPath, "remote", "get-url", "origin").Output(); originErr != nil {
			return fmt.Errorf("no remote 'origin' configured for Hub repo: %s", cfg.RepoPath)
		}

		printInfo("", "Fetching remote updates (origin)...")
		fetchOut, fetchErr := exec.Command("git", "-C", cfg.RepoPath, "fetch", "--prune", "origin").CombinedOutput()
		if fetchErr != nil {
			trimmed := strings.TrimSpace(string(fetchOut))
			if trimmed == "" {
				return fmt.Errorf("git fetch failed: %w", fetchErr)
			}
			return fmt.Errorf("git fetch failed:\n%s", trimmed)
		}
		printOK("", "Fetch complete.")
	}

	// Remote update summary (origin-based only).
	// We intentionally do not rely on Git's upstream tracking configuration (@{u}).
	originHead, originHeadErr := exec.Command("git", "-C", cfg.RepoPath, "rev-parse", "--abbrev-ref", "origin/HEAD").Output()
	if originHeadErr != nil {
		if fetchFirst {
			printWarn("", "Remote default branch not available (origin/HEAD). Re-run 'axon remote set <url>' to initialize the remote default branch reference.")
		}
	} else {
		compareRef := strings.TrimSpace(string(originHead))
		countsRaw, countsErr := exec.Command("git", "-C", cfg.RepoPath, "rev-list", "--left-right", "--count", "HEAD..."+compareRef).Output()
		if countsErr == nil {
			fields := strings.Fields(strings.TrimSpace(string(countsRaw)))
			if len(fields) >= 2 {
				ahead, aErr := strconv.Atoi(fields[0])
				behind, bErr := strconv.Atoi(fields[1])
				if aErr == nil && bErr == nil {
					printOK("", fmt.Sprintf("Remote: %s (ahead %d / behind %d)", compareRef, ahead, behind))
					if behind > 0 {
						printInfo("", fmt.Sprintf("Remote is newer by %d commit(s). Run 'axon sync' to pull updates.", behind))
					}
					if ahead > 0 {
						if cfg.SyncMode == "read-only" {
							printWarn("", fmt.Sprintf("Local is newer by %d commit(s), but sync_mode is read-only so changes will not be pushed.", ahead))
						} else {
							printInfo("", fmt.Sprintf("Local is newer by %d commit(s). Run 'axon sync' to publish your changes.", ahead))
						}
					}
				}
			}
		}
	}

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

// showSkillStatus prints focused status for a single skill: path, link state,
// recent commit history, and (with --fetch) a remote comparison.
func showSkillStatus(cfg *config.Config, skillName string, fetchFirst bool) error {
	// Resolve the skill path relative to the repo root.
	skillPath, err := resolveSkillPath(cfg.RepoPath, skillName)
	if err != nil {
		return err
	}
	absSkillPath := filepath.Join(cfg.RepoPath, skillPath)

	fmt.Printf("\n=== Skill: %s ===\n", skillName)
	fmt.Printf("  Path:    %s\n", absSkillPath)

	linked := false
	for _, t := range cfg.Targets {
		// A skill is "linked" if its path is exactly a target source,
		// or if its path is within a target source directory (e.g. source is "skills"),
		// or if a target source is within this skill directory.
		if t.Source == skillPath ||
			strings.HasPrefix(skillPath, t.Source+"/") ||
			strings.HasPrefix(t.Source, skillPath+"/") {
			linked = true
			break
		}
	}
	if linked {
		fmt.Printf("  Linked:  ✓\n")
	} else {
		fmt.Printf("  Linked:  ✗  (not found in axon.yaml targets)\n")
	}

	// Optionally fetch remote before comparing.
	if fetchFirst && gitHasRemote(cfg.RepoPath) {
		printInfo("", "Fetching remote updates (origin)...")
		fetchOut, fetchErr := exec.Command("git", "-C", cfg.RepoPath, "fetch", "--prune", "origin").CombinedOutput()
		if fetchErr != nil {
			trimmed := strings.TrimSpace(string(fetchOut))
			if trimmed == "" {
				return fmt.Errorf("git fetch failed: %w", fetchErr)
			}
			return fmt.Errorf("git fetch failed:\n%s", trimmed)
		}
		printOK("", "Fetch complete.")
	}

	// Recent commit history scoped to this skill path.
	entries, err := gitLogEntries(cfg.RepoPath, skillPath, 0, 10)
	if err != nil {
		return fmt.Errorf("cannot read commit history: %w", err)
	}

	fmt.Println("\n● Recent commits:")
	if len(entries) == 0 {
		fmt.Println("  (no commits found for this skill path)")
	} else {
		for i, e := range entries {
			fmt.Printf("  #%-2d  %s  %s   %s\n", i+1, e.sha, e.date, e.subject)
		}
	}

	// Remote comparison (requires --fetch).
	if fetchFirst && gitHasRemote(cfg.RepoPath) {
		originHead, originErr := exec.Command("git", "-C", cfg.RepoPath,
			"rev-parse", "--abbrev-ref", "origin/HEAD").Output()
		if originErr == nil {
			compareRef := strings.TrimSpace(string(originHead))
			// Count commits on each side that touch this skill path.
			// git rev-list does not support --left-right with path filters directly;
			// so we count separately.
			localCount, localErr := gitOutput(cfg.RepoPath, "rev-list", "--count", compareRef+"..HEAD", "--", skillPath)
			remoteCount, remoteErr := gitOutput(cfg.RepoPath, "rev-list", "--count", "HEAD.."+compareRef, "--", skillPath)
			if localErr == nil && remoteErr == nil {
				ahead := strings.TrimSpace(localCount)
				behind := strings.TrimSpace(remoteCount)
				if ahead != "" && behind != "" {
					fmt.Printf("\n  Remote: %s  (skill ahead %s / behind %s)\n", compareRef, ahead, behind)
				}
			}
		}
	}

	return nil
}
