package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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

	if _, _, _, err := printStatusSymlinkHealth(os.Stdout, cfg); err != nil {
		return err
	}

	if err := printStatusVendorHealth(os.Stdout, cfg); err != nil {
		return err
	}

	printSection("Hub Git Status")
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

	printSection(fmt.Sprintf("Skill: %s", skillName))
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
		printOK("Linked", "yes")
	} else {
		printWarn("Linked", "not found in axon.yaml targets")
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

	printBullet("Recent commits:")
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

// targetAssetCategory maps a target to its Hub asset category label.
// It uses target.Source and target.Type:
// - If t.IsFile() and source contains "rule" -> "Rules"
// - If t.IsFile() and does not contain "rule" -> "Files"
// - If directory and base contains "rule" -> "Rules"
// - Otherwise -> Title-cased base of t.Source (e.g. "Skills", "Workflows", "Commands")
func targetAssetCategory(t config.Target) string {
	if t.IsFile() {
		srcLower := strings.ToLower(t.Source)
		if strings.Contains(srcLower, "rule") {
			return "Rules"
		}
		return "Files"
	}
	base := filepath.Base(strings.TrimSpace(t.Source))
	baseLower := strings.ToLower(base)
	if strings.Contains(baseLower, "rule") {
		return "Rules"
	}
	return cases.Title(language.Und).String(base)
}

var canonicalCategories = []string{"Skills", "Rules", "Workflows", "Commands", "Files"}

// sortCategories sorts categories giving priority to canonical categories
// (Skills, Rules, Workflows, Commands, Files), followed by any custom categories alphabetically.
func sortCategories(cats []string) {
	categoryWeight := func(cat string) int {
		for i, c := range canonicalCategories {
			if strings.EqualFold(cat, c) {
				return i
			}
		}
		return len(canonicalCategories) + 1
	}

	sort.Slice(cats, func(i, j int) bool {
		wI, wJ := categoryWeight(cats[i]), categoryWeight(cats[j])
		if wI != wJ {
			return wI < wJ
		}
		return cats[i] < cats[j]
	})
}

// formatTargetList formats a list of item names with comma separators and line wrapping.
// The first line is prefixed with indentFirst, and subsequent lines with indentRest.
func formatTargetList(names []string, indentFirst, indentRest string, maxCol int) string {
	if len(names) == 0 {
		return ""
	}
	var lines []string
	currentLine := indentFirst
	indentLen := utf8.RuneCountInString(indentFirst)

	for i, name := range names {
		item := name
		if i < len(names)-1 {
			item += ","
		}

		currentLen := utf8.RuneCountInString(currentLine)
		itemLen := utf8.RuneCountInString(item)

		if currentLen == indentLen {
			currentLine += item
		} else if currentLen+1+itemLen <= maxCol {
			currentLine += " " + item
		} else {
			lines = append(lines, currentLine)
			currentLine = indentRest + item
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	return strings.Join(lines, "\n")
}

type statusIssueEntry struct {
	name string
	kind string // "real", "missing", "wrong", "error"
	msg  string
}

type categoryStatus struct {
	category string
	healthy  []string
	issues   []statusIssueEntry
}

// printStatusSymlinkHealth renders symlink validation results grouped by asset category.
func printStatusSymlinkHealth(w io.Writer, cfg *config.Config) (healthyCount, installedCount, issueCount int, err error) {
	fmt.Fprintf(w, "\n=== Symlink Health (by Asset) ===\n")

	// Sort targets alphabetically by name first for determinism
	targets := make([]config.Target, len(cfg.Targets))
	copy(targets, cfg.Targets)
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})

	catMap := make(map[string]*categoryStatus)
	var catNames []string
	notInstalledMap := make(map[string]bool)
	var notInstalled []string

	for _, t := range targets {
		dest, expandErr := config.ExpandPath(t.Destination)
		if expandErr != nil {
			cat := targetAssetCategory(t)
			if catMap[cat] == nil {
				catMap[cat] = &categoryStatus{category: cat}
				catNames = append(catNames, cat)
			}
			catMap[cat].issues = append(catMap[cat].issues, statusIssueEntry{
				name: t.Name,
				kind: "error",
				msg:  fmt.Sprintf("cannot expand path: %v", expandErr),
			})
			installedCount++
			issueCount++
			continue
		}

		// Check if the tool itself is installed
		if isParentMissing(dest) {
			baseName := toolBaseName(t.Name)
			if !notInstalledMap[baseName] {
				notInstalledMap[baseName] = true
				notInstalled = append(notInstalled, baseName)
			}
			continue
		}

		installedCount++
		cat := targetAssetCategory(t)
		if catMap[cat] == nil {
			catMap[cat] = &categoryStatus{category: cat}
			catNames = append(catNames, cat)
		}

		expected := filepath.Join(cfg.RepoPath, t.Source)
		state, _, actualTarget, statErr := checkSymlinkState(dest, expected)
		switch state {
		case symlinkCorrect:
			catMap[cat].healthy = append(catMap[cat].healthy, t.Name)
			healthyCount++
		case symlinkMissing:
			catMap[cat].issues = append(catMap[cat].issues, statusIssueEntry{
				name: t.Name,
				kind: "missing",
				msg:  fmt.Sprintf("not linked (run: axon link %s)", t.Name),
			})
			issueCount++
		case symlinkRealEntry:
			kind := "directory"
			if t.IsFile() {
				kind = "file"
			}
			catMap[cat].issues = append(catMap[cat].issues, statusIssueEntry{
				name: t.Name,
				kind: "real",
				msg:  fmt.Sprintf("real %s — run 'axon link %s' to convert (original will be backed up)", kind, t.Name),
			})
			issueCount++
		case symlinkWrong:
			catMap[cat].issues = append(catMap[cat].issues, statusIssueEntry{
				name: t.Name,
				kind: "wrong",
				msg:  fmt.Sprintf("wrong target:\n      got:  %s\n      want: %s", actualTarget, expected),
			})
			issueCount++
		case symlinkStatError:
			catMap[cat].issues = append(catMap[cat].issues, statusIssueEntry{
				name: t.Name,
				kind: "error",
				msg:  fmt.Sprintf("stat error: %v", statErr),
			})
			issueCount++
		}
	}

	sortCategories(catNames)

	var activeCategories []string
	for _, cat := range catNames {
		st := catMap[cat]
		if len(st.healthy) == 0 && len(st.issues) == 0 {
			continue
		}
		activeCategories = append(activeCategories, cat)

		if len(st.issues) == 0 {
			targetsWord := "targets"
			if len(st.healthy) == 1 {
				targetsWord = "target"
			}
			fmt.Fprintf(w, "\n● %s (%d %s linked):\n", cat, len(st.healthy), targetsWord)
			fmt.Fprintln(w, formatTargetList(st.healthy, fmt.Sprintf("  %s  ", iconOK), "     ", 76))
		} else {
			issuesWord := "issues"
			if len(st.issues) == 1 {
				issuesWord = "issue"
			}
			fmt.Fprintf(w, "\n● %s (%d linked, %d %s):\n", cat, len(st.healthy), len(st.issues), issuesWord)
			if len(st.healthy) > 0 {
				fmt.Fprintln(w, formatTargetList(st.healthy, fmt.Sprintf("  %s  ", iconOK), "     ", 76))
			}
			for _, iss := range st.issues {
				switch iss.kind {
				case "real":
					fmt.Fprintf(w, "  %s  [%s] %s\n", iconWarn, iss.name, iss.msg)
				case "missing":
					fmt.Fprintf(w, "  %s  [%s] %s\n", iconMiss, iss.name, iss.msg)
				case "wrong", "error":
					fmt.Fprintf(w, "  %s  [%s] %s\n", iconError, iss.name, iss.msg)
				}
			}
		}
	}

	if len(notInstalled) > 0 {
		sort.Strings(notInstalled)
		fmt.Fprintf(w, "\n● Not Installed Tools (skipped):\n")
		fmt.Fprintln(w, formatTargetList(notInstalled, fmt.Sprintf("  %s  ", iconSkip), "     ", 76))
	}

	numCats := len(activeCategories)
	catWord := "categories"
	if numCats == 1 {
		catWord = "category"
	}
	toolsWord := "tools"
	if len(notInstalled) == 1 {
		toolsWord = "tool"
	}

	if installedCount == 0 {
		if len(notInstalled) > 0 {
			fmt.Fprintf(w, "\n  0 targets installed (%d %s not installed)\n", len(notInstalled), toolsWord)
		} else {
			fmt.Fprintf(w, "\n  0 targets configured\n")
		}
	} else if issueCount == 0 {
		if len(notInstalled) > 0 {
			fmt.Fprintf(w, "\n  %d/%d links healthy across %d %s (%d %s not installed)\n", healthyCount, installedCount, numCats, catWord, len(notInstalled), toolsWord)
		} else {
			fmt.Fprintf(w, "\n  %d/%d links healthy across %d %s\n", healthyCount, installedCount, numCats, catWord)
		}
	} else {
		issuesWord := "issues"
		if issueCount == 1 {
			issuesWord = "issue"
		}
		if len(notInstalled) > 0 {
			fmt.Fprintf(w, "\n  %d/%d links healthy, %d %s across %d %s (%d %s not installed)\n", healthyCount, installedCount, issueCount, issuesWord, numCats, catWord, len(notInstalled), toolsWord)
		} else {
			fmt.Fprintf(w, "\n  %d/%d links healthy, %d %s across %d %s\n", healthyCount, installedCount, issueCount, issuesWord, numCats, catWord)
		}
	}

	return healthyCount, installedCount, issueCount, nil
}

// printStatusVendorHealth renders a summary of vendor dependencies and their local sync health.
func printStatusVendorHealth(w io.Writer, cfg *config.Config) error {
	fmt.Fprintf(w, "\n=== Hub Assets & Vendors ===\n")

	vendors, _, err := vendor.LoadEffectiveVendors(cfg.RepoPath, cfg)
	if err != nil {
		fmt.Fprintf(w, "  %s  Vendors: cannot load vendors: %v\n", iconWarn, err)
		return nil
	}

	if len(vendors) == 0 {
		fmt.Fprintf(w, "  %s  Vendors: none configured\n", iconInfo)
		return nil
	}

	var synced []string
	var missingDest []string
	var pending []string

	for _, v := range vendors {
		destPath := filepath.Join(cfg.RepoPath, v.Dest)
		if _, statErr := os.Stat(destPath); os.IsNotExist(statErr) {
			missingDest = append(missingDest, v.Name)
			continue
		}

		prov, _ := vendor.ReadProvenance(destPath)
		if prov != nil && prov.Commit != "" {
			shortSHA := prov.Commit
			if len(shortSHA) > 7 {
				shortSHA = shortSHA[:7]
			}
			synced = append(synced, fmt.Sprintf("%s@%s", v.Name, shortSHA))
		} else {
			pending = append(pending, v.Name)
		}
	}

	if len(missingDest) == 0 && len(pending) == 0 {
		fmt.Fprintf(w, "  %s  Vendors: %d synced (%s)\n", iconOK, len(synced), strings.Join(synced, ", "))
		return nil
	}

	var parts []string
	if len(synced) > 0 {
		parts = append(parts, fmt.Sprintf("%d synced", len(synced)))
	}
	if len(missingDest) > 0 {
		parts = append(parts, fmt.Sprintf("%d missing dest", len(missingDest)))
	}
	if len(pending) > 0 {
		parts = append(parts, fmt.Sprintf("%d pending", len(pending)))
	}

	fmt.Fprintf(w, "  %s  Vendors: %s\n", iconWarn, strings.Join(parts, ", "))
	for _, name := range missingDest {
		fmt.Fprintf(w, "     %s [%s] destination missing (run: axon vendor sync %s)\n", iconMiss, name, name)
	}
	for _, name := range pending {
		fmt.Fprintf(w, "     %s [%s] pending initial sync (run: axon vendor sync %s)\n", iconMiss, name, name)
	}

	return nil
}
