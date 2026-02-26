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

func init() {
	doctorCmd.AddCommand(doctorFixCmd)
	rootCmd.AddCommand(doctorCmd)
}

var doctorFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Automatically fix detected issues",
	Long: `Fix detected issues in the Axon environment.

Currently fixes:
  - Unresolved import conflicts: deletes all .conflict-* files from the Hub

Run 'axon doctor' first to see what will be fixed.`,
	RunE: runDoctorFix,
}

func runDoctorFix(_ *cobra.Command, _ []string) error {
	if err := checkGitAvailable(); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	printSection("axon doctor fix")

	// ── Fix: delete all .conflict-* files ─────────────────────────────────────
	fmt.Println("\n[ Unresolved conflicts ]")
	conflicts := findConflictFiles(cfg.RepoPath)
	if len(conflicts) == 0 {
		printOK("", "no conflict files found — nothing to fix")
		return nil
	}

	var failed int
	for _, rel := range conflicts {
		full := filepath.Join(cfg.RepoPath, rel)
		if err := os.Remove(full); err != nil {
			printErr("", fmt.Sprintf("cannot delete %s: %v", rel, err))
			failed++
		} else {
			printOK("", fmt.Sprintf("deleted %s", rel))
		}
	}

	fmt.Println()
	if failed > 0 {
		return fmt.Errorf("%d file(s) could not be deleted", failed)
	}
	fmt.Printf("  ✓  %d conflict file(s) removed. Run 'axon sync' to commit the cleanup.\n", len(conflicts))
	return nil
}

func runDoctor(_ *cobra.Command, _ []string) error {
	allOK := true
	failD := func(format string, args ...any) {
		printErr("", fmt.Sprintf(format, args...))
		allOK = false
	}

	printSection("axon doctor")
	fmt.Println()

	// ── Check 1: git installed ────────────────────────────────────────────
	fmt.Println("[ git ]")
	if out, err := exec.Command("git", "--version").Output(); err != nil {
		failD("git not found — please install Git: https://git-scm.com/downloads")
	} else {
		printOK("", string(out[:len(out)-1])) // trim newline
	}
	fmt.Println()

	// ── Check 2: Hub directory exists ─────────────────────────────────────────
	fmt.Println("[ Hub directory ]")
	axonDir, err := config.AxonDir()
	if err != nil {
		failD("cannot determine home directory: %v", err)
	} else {
		cfgPath, _ := config.ConfigPath()
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			failD("~/.axon/axon.yaml not found — run 'axon init' first")
		} else {
			printOK("", fmt.Sprintf("~/.axon/ exists: %s", axonDir))
		}
	}
	fmt.Println()

	// ── Check 3: axon.yaml is valid ─────────────────────────────────────────
	fmt.Println("[ axon.yaml ]")
	cfg, loadErr := config.Load()
	if loadErr != nil {
		failD("cannot parse axon.yaml: %v", loadErr)
	} else {
		printOK("", fmt.Sprintf("valid YAML — %d target(s) defined", len(cfg.Targets)))
		if cfg.RepoPath == "" {
			failD("repo_path is empty")
		}
		if cfg.SyncMode != "read-write" && cfg.SyncMode != "read-only" {
			printWarn("", fmt.Sprintf("unknown sync_mode %q — expected read-write or read-only", cfg.SyncMode))
		}
	}
	fmt.Println()

	// ── Check 4: Hub repo exists ───────────────────────────────────────────────
	fmt.Println("[ Hub repo ]")
	if loadErr == nil {
		gitDir := filepath.Join(cfg.RepoPath, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			failD("Hub repo not initialised at %s — run 'axon init'", cfg.RepoPath)
		} else {
			printOK("", fmt.Sprintf("Git repo ready: %s", cfg.RepoPath))
		}
	} else {
		printWarn("", "skipped (axon.yaml not loaded)")
	}
	fmt.Println()

	// ── Check 5: All symlinks healthy ─────────────────────────────────────────────
	fmt.Println("[ Symlinks ]")
	if loadErr == nil {
		targets := make([]config.Target, len(cfg.Targets))
		copy(targets, cfg.Targets)
		sort.Slice(targets, func(i, j int) bool {
			return targets[i].Name < targets[j].Name
		})

		symlinkOK := true
		for _, t := range targets {
			dest, err := config.ExpandPath(t.Destination)
			if err != nil {
				failD("[%s] cannot expand path: %v", t.Name, err)
				symlinkOK = false
				continue
			}

			// If parent doesn't exist, tool isn't installed.
			parent := filepath.Dir(dest)
			if _, parentErr := os.Stat(parent); os.IsNotExist(parentErr) {
				continue // Skip silently in doctor, status handles this verbosely
			}

			info, err := os.Lstat(dest)
			if os.IsNotExist(err) {
				printWarn(t.Name, fmt.Sprintf("not linked yet (run 'axon link %s')", t.Name))
				symlinkOK = false
				continue
			}
			if err != nil {
				failD("[%s] stat error: %v", t.Name, err)
				symlinkOK = false
				continue
			}
			if info.Mode()&os.ModeSymlink == 0 {
				printWarn(t.Name, fmt.Sprintf("real directory present at %s (run 'axon link %s' to convert)", dest, t.Name))
				symlinkOK = false
				continue
			}
			expected := filepath.Join(cfg.RepoPath, t.Source)
			actual, _ := os.Readlink(dest)
			if actual != expected {
				failD("[%s] wrong target:\n      got:  %s\n      want: %s", t.Name, actual, expected)
				symlinkOK = false
				continue
			}
			printOK(t.Name, "OK")
		}
		if symlinkOK {
			fmt.Println("  All configured symlinks are healthy.")
		}
	} else {
		printWarn("", "skipped (axon.yaml not loaded)")
	}
	fmt.Println()

	// ── Check 6: Unresolved import conflicts ──────────────────────────────────
	fmt.Println("[ Unresolved conflicts ]")
	if loadErr == nil {
		conflicts := findConflictFiles(cfg.RepoPath)
		if len(conflicts) == 0 {
			printOK("", "no unresolved conflict files found")
		} else {
			for _, c := range conflicts {
				printWarn("", c)
			}
			fmt.Printf("\n  ⚠  %d unresolved conflict file(s) found in Hub.\n", len(conflicts))
			fmt.Println("     Review and delete the .conflict-* files you no longer need,")
			fmt.Println("     then run 'axon sync' to commit the resolution.")
			allOK = false
		}
	} else {
		printWarn("", "skipped (axon.yaml not loaded)")
	}
	fmt.Println()

	// ── Check 7: Symlink creation permission (Windows only) ──────────────────────
	if runtime.GOOS == "windows" {
		fmt.Println("[ Windows symlink permission ]")
		if err := checkWindowsSymlinkPermission(); err != nil {
			failD("Symlink creation will fail in this terminal — Administrator rights required.\n" +
				"   Run axon in an Administrator terminal.\n" +
				"   WSL users are not affected by this restriction.")
		} else {
			printOK("", "symlink creation permitted")
		}
		fmt.Println()
	}

	// ── Summary ──────────────────────────────────────────────────────────────────
	fmt.Println("===================")
	if allOK {
		fmt.Println("✓  All checks passed. Axon is ready to use.")
	} else {
		fmt.Fprintln(os.Stderr, "✗  One or more checks failed. See details above.")
		return fmt.Errorf("doctor found issues")
	}
	return nil
}

// checkWindowsSymlinkPermission creates a throwaway symlink in the temp
// directory to probe whether the current process has symlink privileges.
func checkWindowsSymlinkPermission() error {
	tmp := os.TempDir()
	src := filepath.Join(tmp, "axon-doctor-src")
	dst := filepath.Join(tmp, "axon-doctor-link")

	// Create a temp source file.
	if err := os.WriteFile(src, []byte("probe"), 0o644); err != nil {
		return err
	}
	defer os.Remove(src)
	defer os.Remove(dst)

	return os.Symlink(src, dst)
}

// findConflictFiles walks repoPath and returns relative paths of all files
// whose name contains ".conflict-" — these are leftover from axon init import.
func findConflictFiles(repoPath string) []string {
	var found []string
	_ = filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		// Match files like: foo.conflict-gemini-skills.md
		if strings.Contains(name, ".conflict-") {
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
