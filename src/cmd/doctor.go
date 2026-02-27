package cmd

import (
"fmt"
"os"
"os/exec"
"path/filepath"
"runtime"
"sort"
"strings"

"github.com/kamusis/axon-cli/internal/config"
"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run pre-flight environment checks",
	Long: `Check that Axon's dependencies and environment are correctly configured.
Run this command when something seems wrong, or before filing a bug report.`,
	RunE: runDoctor,
}

var doctorFix bool

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Automatically fix detected issues where possible")
	rootCmd.AddCommand(doctorCmd)
}

type DiagnosticResult struct {
	Category    string
	Item        string
	Passed      bool
	Message     string
	Remediation string
	CanFix      bool
	FixAction   func() error
}

func runDoctor(_ *cobra.Command, _ []string) error {
	printSection("axon doctor")
	fmt.Println()

	results := gatherDiagnostics()

	if doctorFix {
		return runFixes(results)
	}

	allOK := true
	var currentCategory string

	for _, r := range results {
		if r.Category != currentCategory {
			if currentCategory != "" {
				fmt.Println()
			}
			fmt.Printf("[ %s ]\n", r.Category)
			currentCategory = r.Category
		}

		if r.Passed {
			printOK(r.Item, r.Message)
		} else {
			allOK = false
			printErr(r.Item, r.Message)
			if r.Remediation != "" {
				fmt.Printf("      Fix: %s\n", r.Remediation)
			}
		}
	}
	fmt.Println()

	fmt.Println("===================")
	if allOK {
		fmt.Println("✓  All checks passed. Axon is ready to use.")
	} else {
		fmt.Fprintln(os.Stderr, "✗  One or more checks failed. See details above.")
		return fmt.Errorf("doctor found issues")
	}
	return nil
}

func runFixes(results []DiagnosticResult) error {
	if err := checkGitAvailable(); err != nil {
		return err
	}

	var fixedCount int
	var failedCount int

	for _, r := range results {
		if !r.Passed && r.CanFix && r.FixAction != nil {
			fmt.Printf("Fixing %s", r.Category)
			if r.Item != "" {
				fmt.Printf(" > %s", r.Item)
			}
			fmt.Print("... ")

			if err := r.FixAction(); err != nil {
				fmt.Printf("FAILED: %v\n", err)
				failedCount++
			} else {
				fmt.Println("OK")
				fixedCount++
			}
		}
	}

	fmt.Println()
	if fixedCount == 0 && failedCount == 0 {
		printOK("", "No fixable issues found.")
		return nil
	}

	if failedCount > 0 {
		return fmt.Errorf("%d issue(s) could not be fixed", failedCount)
	}

	fmt.Printf("  ✓  %d issue(s) fixed successfully.\n", fixedCount)
	return nil
}

func gatherDiagnostics() []DiagnosticResult {
	var results []DiagnosticResult

	// 1. Git
	results = append(results, checkGitDoctor()...)

	// 2. Hub directory & config
	cfgRes, cfg, loadErr := checkHubAndConfig()
	results = append(results, cfgRes...)

	if loadErr == nil && cfg != nil {
		// 3. Hub Repo
		results = append(results, checkHubRepo(cfg)...)

		// 4. Git Health
		results = append(results, checkGitHealth(cfg)...)

		// 5. Symlinks
		results = append(results, checkSymlinks(cfg)...)

		// 6. Conflicts
		results = append(results, checkConflicts(cfg)...)

		// 7. Permission Sentinel
		results = append(results, checkPermissions(cfg)...)

		// 8. Binary Dependencies
		results = append(results, checkBinaryDeps(cfg)...)
	}

	// 9. Windows symlink permission
	if runtime.GOOS == "windows" {
		results = append(results, checkWindowsSymlink()...)
	}

	return results
}

func checkGitDoctor() []DiagnosticResult {
	cat := "git"
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		return []DiagnosticResult{{
			Category:    cat,
			Passed:      false,
			Message:     "git not found",
			Remediation: "install Git: https://git-scm.com/downloads",
		}}
	}
	return []DiagnosticResult{{
		Category: cat,
		Passed:   true,
		Message:  strings.TrimSpace(string(out)),
	}}
}

func checkHubAndConfig() ([]DiagnosticResult, *config.Config, error) {
	catDir := "Hub directory"
	catCfg := "axon.yaml"
	var res []DiagnosticResult

	axonDir, err := config.AxonDir()
	if err != nil {
		res = append(res, DiagnosticResult{
			Category: catDir, Passed: false, Message: fmt.Sprintf("cannot determine home directory: %v", err),
		})
		return res, nil, err
	}

	cfgPath, _ := config.ConfigPath()
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		res = append(res, DiagnosticResult{
			Category: catDir, Passed: false, Message: "~/.axon/axon.yaml not found", Remediation: "run 'axon init'",
		})
		return res, nil, err
	}
	res = append(res, DiagnosticResult{Category: catDir, Passed: true, Message: fmt.Sprintf("~/.axon/ exists: %s", axonDir)})

	cfg, loadErr := config.Load()
	if loadErr != nil {
		res = append(res, DiagnosticResult{
			Category: catCfg, Passed: false, Message: fmt.Sprintf("cannot parse axon.yaml: %v", loadErr), Remediation: "fix syntax in axon.yaml",
		})
		return res, nil, loadErr
	}

	res = append(res, DiagnosticResult{Category: catCfg, Passed: true, Message: fmt.Sprintf("valid YAML — %d target(s) defined", len(cfg.Targets))})

	if cfg.RepoPath == "" {
		res = append(res, DiagnosticResult{Category: catCfg, Passed: false, Message: "repo_path is empty", Remediation: "add repo_path to axon.yaml"})
	}

	return res, cfg, nil
}

func checkHubRepo(cfg *config.Config) []DiagnosticResult {
	cat := "Hub repo"
	gitDir := filepath.Join(cfg.RepoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return []DiagnosticResult{{
			Category: cat, Passed: false, Message: fmt.Sprintf("Hub repo not initialised at %s", cfg.RepoPath), Remediation: "run 'axon init'",
		}}
	}
	return []DiagnosticResult{{
		Category: cat, Passed: true, Message: fmt.Sprintf("Git repo ready: %s", cfg.RepoPath),
	}}
}

func checkGitHealth(cfg *config.Config) []DiagnosticResult {
	cat := "Git Health"
	var res []DiagnosticResult

	// Check detached HEAD
	cmdHead := exec.Command("git", "symbolic-ref", "-q", "HEAD")
	cmdHead.Dir = cfg.RepoPath
	if err := cmdHead.Run(); err != nil {
		// Possibly detached HEAD
		res = append(res, DiagnosticResult{
			Category:    cat,
			Passed:      false,
			Message:     "Repository is in a detached HEAD state",
			Remediation: "run 'git checkout main' (or the default branch) in the Hub directory",
		})
	} else {
		res = append(res, DiagnosticResult{
			Category: cat, Passed: true, Message: "HEAD is attached to a branch",
		})
	}

	// Check diverged branch
	cmdStatus := exec.Command("git", "status", "-sb")
	cmdStatus.Dir = cfg.RepoPath
	out, err := cmdStatus.Output()
	if err == nil {
		statusStr := string(out)
		if strings.Contains(statusStr, "diverged") {
			res = append(res, DiagnosticResult{
				Category:    cat,
				Passed:      false,
				Message:     "Branch has diverged from upstream tracking branch",
				Remediation: "run 'git pull --rebase' or resolve origin manually in the Hub directory",
			})
		}
	}

	return res
}

func checkSymlinks(cfg *config.Config) []DiagnosticResult {
	cat := "Symlinks"
	var res []DiagnosticResult

	targets := make([]config.Target, len(cfg.Targets))
	copy(targets, cfg.Targets)
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})

	for _, t := range targets {
		dest, err := config.ExpandPath(t.Destination)
		if err != nil {
			res = append(res, DiagnosticResult{Category: cat, Item: t.Name, Passed: false, Message: fmt.Sprintf("cannot expand path: %v", err)})
			continue
		}

		parent := filepath.Dir(dest)
		if _, parentErr := os.Stat(parent); os.IsNotExist(parentErr) {
			continue // Skip silently in doctor, target not installed
		}

		info, err := os.Lstat(dest)
		if os.IsNotExist(err) {
			targetName := t.Name // capture loop var
			res = append(res, DiagnosticResult{
				Category:    cat,
				Item:        t.Name,
				Passed:      false,
				Message:     "not linked yet",
				Remediation: fmt.Sprintf("run 'axon link %s'", targetName),
				CanFix:      true,
				FixAction: func() error {
					return runLink(nil, []string{targetName})
				},
			})
			continue
		}
		if err != nil {
			res = append(res, DiagnosticResult{Category: cat, Item: t.Name, Passed: false, Message: fmt.Sprintf("stat error: %v", err)})
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			res = append(res, DiagnosticResult{
				Category:    cat,
				Item:        t.Name,
				Passed:      false,
				Message:     fmt.Sprintf("real directory present at %s", dest),
				Remediation: fmt.Sprintf("delete the folder and run 'axon link %s'", t.Name),
			})
			continue
		}
		expected := filepath.Join(cfg.RepoPath, t.Source)
		actual, _ := os.Readlink(dest)
		if actual != expected {
			targetName := t.Name // capture
			res = append(res, DiagnosticResult{
				Category:    cat,
				Item:        t.Name,
				Passed:      false,
				Message:     fmt.Sprintf("wrong target:\n      got:  %s\n      want: %s", actual, expected),
				Remediation: fmt.Sprintf("run 'axon link %s'", targetName),
				CanFix:      true,
				FixAction: func() error {
					return runLink(nil, []string{targetName})
				},
			})
			continue
		}
		res = append(res, DiagnosticResult{Category: cat, Item: t.Name, Passed: true, Message: "OK"})
	}

	if len(res) == 0 {
		res = append(res, DiagnosticResult{Category: cat, Passed: true, Message: "No active symlinks to check."})
	}
	return res
}

func checkConflicts(cfg *config.Config) []DiagnosticResult {
	cat := "Unresolved conflicts"
	var res []DiagnosticResult

	conflicts := findConflictFiles(cfg.RepoPath)
	if len(conflicts) == 0 {
		return []DiagnosticResult{{Category: cat, Passed: true, Message: "no unresolved conflict files found"}}
	}

	for _, c := range conflicts {
		relPath := c // capture
		fullPath := filepath.Join(cfg.RepoPath, relPath)
		res = append(res, DiagnosticResult{
			Category:    cat,
			Passed:      false,
			Message:     fmt.Sprintf("unresolved conflict: %s", relPath),
			Remediation: "run 'axon doctor --fix' to delete",
			CanFix:      true,
			FixAction: func() error {
				return os.Remove(fullPath)
			},
		})
	}
	return res
}

func checkPermissions(cfg *config.Config) []DiagnosticResult {
cat := "Permission Sentinel"
var res []DiagnosticResult

for _, t := range cfg.Targets {
dest, err := config.ExpandPath(t.Destination)
if err != nil {
continue
}
parent := filepath.Dir(dest)
if info, parentErr := os.Stat(parent); parentErr == nil && info.IsDir() {
// Try writing a harmless temp file to the parent dir
probePath := filepath.Join(parent, ".axon-probe-perms")
err := os.WriteFile(probePath, []byte(""), 0644)
if err != nil {
res = append(res, DiagnosticResult{
Category:    cat,
Item:        t.Name,
Passed:      false,
Message:     fmt.Sprintf("no write permission in %s", parent),
Remediation: fmt.Sprintf("fix permissions for %s to allow symlink creation", parent),
})
} else {
os.Remove(probePath)
res = append(res, DiagnosticResult{Category: cat, Item: t.Name, Passed: true, Message: "write permitted"})
}
}
}

if len(res) == 0 {
res = append(res, DiagnosticResult{Category: cat, Passed: true, Message: "No directories to check permissions for."})
}
return res
}

func checkBinaryDeps(cfg *config.Config) []DiagnosticResult {
	cat := "Binary Dependencies"
	var res []DiagnosticResult

	foundAny := false
	seenBins := make(map[string]bool)

	// Since the Hub is centralized, we just scan all SKILL.md files in the repository.
	_ = filepath.WalkDir(cfg.RepoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "SKILL.md" {
			meta, hasMeta := parseSkillMeta(path)
			if !hasMeta {
				return nil
			}

			skillName := filepath.Base(filepath.Dir(path))
			if meta.Name != "" {
				skillName = meta.Name
			}

			for _, bin := range meta.GetRequiresBins() {
				foundAny = true

				// Deduplicate by bin + skillName
				key := bin + "|" + skillName
				if seenBins[key] {
					continue
				}
				seenBins[key] = true

				if _, err := exec.LookPath(bin); err != nil {
					res = append(res, DiagnosticResult{
						Category:    cat,
						Item:        fmt.Sprintf("%s (%s)", bin, skillName),
						Passed:      false,
						Message:     fmt.Sprintf("binary '%s' not found in $PATH", bin),
						Remediation: fmt.Sprintf("install %s and ensure it is in your PATH", bin),
					})
				} else {
					res = append(res, DiagnosticResult{
						Category: cat,
						Item:     fmt.Sprintf("%s (%s)", bin, skillName),
						Passed:   true,
						Message:  "found in $PATH",
					})
				}
			}
		}
		return nil
	})

	if !foundAny {
		res = append(res, DiagnosticResult{Category: cat, Passed: true, Message: "no binary dependencies declared"})
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].Item < res[j].Item
	})

	return res
}

func checkWindowsSymlink() []DiagnosticResult {
cat := "Windows symlink permission"
if err := checkWindowsSymlinkPermission(); err != nil {
return []DiagnosticResult{{
Category:    cat,
Passed:      false,
Message:     "Administrator rights required to create symlinks",
Remediation: "Run axon in an Administrator terminal. WSL users are not affected.",
}}
}
return []DiagnosticResult{{Category: cat, Passed: true, Message: "symlink creation permitted"}}
}

func checkWindowsSymlinkPermission() error {
tmp := os.TempDir()
src := filepath.Join(tmp, "axon-doctor-src")
dst := filepath.Join(tmp, "axon-doctor-link")

if err := os.WriteFile(src, []byte("probe"), 0o644); err != nil {
return err
}
defer os.Remove(src)
defer os.Remove(dst)

return os.Symlink(src, dst)
}

func findConflictFiles(repoPath string) []string {
var found []string
_ = filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
if err != nil || d.IsDir() {
return err
}
if strings.Contains(d.Name(), ".conflict-") {
rel, relErr := filepath.Rel(repoPath, path)
if relErr != nil {
rel = path
}
found = append(found, rel)
}
return nil
})
return found
}
