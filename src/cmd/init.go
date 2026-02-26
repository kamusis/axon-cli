package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/importer"
	"github.com/spf13/cobra"
)

// defaultGitignore is written into the Hub repo on first init.
const defaultGitignore = `.DS_Store
Thumbs.db
*.tmp
*.bak
*~
.idea/
.vscode/
`

// defaultGitattributes normalizes text file line endings across platforms.
// This prevents cross-platform CRLF/LF churn when syncing the Hub between Windows and Linux.
const defaultGitattributes = `* text=auto eol=lf
`

var initCmd = &cobra.Command{
	Use:   "init [repo-url]",
	Short: "Bootstrap the Axon Hub and import existing skills",
	Long: `Initialize the Axon Hub at ~/.axon/repo/.

Three modes:
  axon init                          Mode A — local-only Git repo
  axon init git@github.com:u/r.git   Mode B — personal remote repo
  axon init --upstream               Mode C — public upstream, read-only`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

var flagUpstream bool

func init() {
	initCmd.Flags().BoolVar(&flagUpstream, "upstream", false, "Clone the public upstream repo in read-only mode (Mode C)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	if err := checkGitAvailable(); err != nil {
		return err
	}
	// ── 1. Resolve ~/.axon directory ──────────────────────────────────────────
	axonDir, err := config.AxonDir()
	if err != nil {
		return err
	}

	cfgPath, err := config.ConfigPath()
	if err != nil {
		return err
	}

	// ── 2. Create ~/.axon/ if it doesn't exist ────────────────────────────────
	if err := os.MkdirAll(axonDir, 0o755); err != nil {
		return fmt.Errorf("cannot create %s: %w", axonDir, err)
	}
	printOK("", fmt.Sprintf("Axon directory ready: %s", axonDir))

	// ── 3. Write axon.yaml if missing ─────────────────────────────────────────
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfg, err := config.DefaultConfig()
		if err != nil {
			return err
		}
		if flagUpstream {
			cfg.SyncMode = "read-only"
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		printOK("", fmt.Sprintf("Config written: %s", cfgPath))
	} else {
		printSkip("", fmt.Sprintf("Config already exists: %s", cfgPath))
	}

	// ── 4. Load final config ──────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	repoPath := cfg.RepoPath

	// ── 5. Set up Hub repo ────────────────────────────────────────────────────
	// clonedFromRemote is true when we successfully cloned a non-empty remote
	// repo. In that case we skip the local import to avoid overwriting remote
	// data. Local-only init (Mode A) and empty-remote init (Mode B fallback)
	// both leave this false so that existing local skills are imported.
	var clonedFromRemote bool
	switch {
	case flagUpstream:
		upstream := cfg.Upstream
		if upstream == "" {
			return fmt.Errorf("no upstream URL configured in axon.yaml")
		}
		fmt.Printf("  Cloning upstream %s → %s\n", upstream, repoPath)
		if err := gitRun("clone", upstream, repoPath); err != nil {
			return fmt.Errorf("git clone failed: %w", err)
		}
		printOK("", "Upstream cloned (read-only mode).")
		clonedFromRemote = true

	case len(args) == 1:
		var cloned bool
		cloned, err = setupHubWithRemote(repoPath, args[0])
		if err != nil {
			return err
		}
		clonedFromRemote = cloned

	default:
		if err := setupHubLocal(repoPath); err != nil {
			return err
		}
	}

	// ── 6. Write default .gitignore (Layer 2 defense) ─────────────────────────
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		if err := os.WriteFile(gitignorePath, []byte(defaultGitignore), 0o644); err != nil {
			return fmt.Errorf("cannot write .gitignore: %w", err)
		}
		printOK("", fmt.Sprintf(".gitignore written: %s", gitignorePath))
	}

	// ── 6b. Write default .gitattributes (newline normalization) ──────────────
	gitattributesPath := filepath.Join(repoPath, ".gitattributes")
	if _, err := os.Stat(gitattributesPath); os.IsNotExist(err) {
		if err := os.WriteFile(gitattributesPath, []byte(defaultGitattributes), 0o644); err != nil {
			return fmt.Errorf("cannot write .gitattributes: %w", err)
		}
		printOK("", fmt.Sprintf(".gitattributes written: %s", gitattributesPath))
	}

	// ── 7. Import existing skills (Modes A & B only) ──────────────────────────
	// Skip entirely if the Hub was populated by a successful remote clone —
	// merging local edits on top of a cloned repo would risk data loss.
	if !clonedFromRemote {
		if err := importExistingSkills(cfg); err != nil {
			return err
		}
	}

	fmt.Println("\n✓  axon init complete. Run 'axon status' to verify your environment.")
	return nil
}

// importExistingSkills scans each target destination and copies real directories
// into the Hub, applying exclude filtering and MD5 conflict resolution.
func importExistingSkills(cfg *config.Config) error {
	// Sort targets alphabetically — mirrors status output ordering.
	targets := make([]config.Target, len(cfg.Targets))
	copy(targets, cfg.Targets)
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})

	// Collect results into display buckets.
	type importedEntry struct {
		name   string
		source string
		result *importer.Result
	}
	var (
		imported       []importedEntry
		alreadyLinked  []string
		notFound       []string
		totalConflicts []importer.ConflictPair
	)
	notInstalledMap := make(map[string]bool)
	var notInstalled []string

	for _, t := range targets {
		dest, err := config.ExpandPath(t.Destination)
		if err != nil {
			return err
		}

		// Check parent dir — if missing, the tool is not installed at all.
		parent := filepath.Dir(dest)
		if _, parentErr := os.Stat(parent); os.IsNotExist(parentErr) {
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

		// Only import from real (non-symlinked) directories.
		info, err := os.Lstat(dest)
		if os.IsNotExist(err) {
			notFound = append(notFound, t.Name)
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			alreadyLinked = append(alreadyLinked, t.Name)
			continue
		}
		if !info.IsDir() {
			notFound = append(notFound, t.Name)
			continue
		}

		// Hub target directory.
		hubDest := filepath.Join(cfg.RepoPath, t.Source)

		result, err := importer.ImportDir(dest, hubDest, t.Name, cfg.Excludes)
		if err != nil {
			return fmt.Errorf("import [%s]: %w", t.Name, err)
		}
		imported = append(imported, importedEntry{name: t.Name, source: t.Source, result: result})
		totalConflicts = append(totalConflicts, result.Conflicts...)
	}

	// ── Print grouped output ───────────────────────────────────────────────────
	fmt.Println("\n=== Import Existing Skills ===")

	if len(imported) > 0 {
		fmt.Println("\n● Imported:")
		for _, e := range imported {
			r := e.result
			// Derive singular label from source: "skills"→"skill", "workflows"→"workflow", etc.
			label := strings.TrimSuffix(e.source, "s")
			if label == "" {
				label = "item"
			}
			fmt.Printf("  ✓  [%s] %d %s(s) imported, %d skipped, %d conflict(s)  (%d file(s))\n",
				e.name,
				r.SkillsImported,
				label,
				r.SkillsSkipped,
				r.SkillsConflicts,
				r.Imported+r.Skipped)
		}
	}

	if len(alreadyLinked) > 0 {
		fmt.Println("\n● Already linked (Hub manages these):")
		for _, name := range alreadyLinked {
			fmt.Printf("  ○  [%s]\n", name)
		}
	}

	if len(notFound) > 0 {
		fmt.Println("\n● Destination not found:")
		for _, name := range notFound {
			fmt.Printf("  -  [%s]\n", name)
		}
	}

	if len(notInstalled) > 0 {
		sort.Strings(notInstalled)
		fmt.Println("\n● Not installed (skipped):")
		for _, name := range notInstalled {
			fmt.Printf("  ○  %s\n", name)
		}
	}

	// ── Post-import conflict report ────────────────────────────────────────────
	if len(totalConflicts) > 0 {
		fmt.Printf("\n⚠  %d conflict(s) detected during import.\n", len(totalConflicts))
		fmt.Printf("   All versions have been preserved in %s.\n", cfg.RepoPath)
		fmt.Println("   Please review and resolve the following files manually:")
		for _, c := range totalConflicts {
			fmt.Printf("     - %s  ← conflicts with %s\n", c.Conflict, c.Original)
		}
	}

	return nil
}


// dirHasContent reports whether dir exists and contains at least one entry.
func dirHasContent(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() != ".gitignore" && e.Name() != ".git" {
			return true
		}
	}
	return false
}

// ── Hub setup helpers ─────────────────────────────────────────────────────────

func setupHubLocal(repoPath string) error {
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		return fmt.Errorf("cannot create repo directory: %w", err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
		if err := gitRun("-C", repoPath, "init"); err != nil {
			return fmt.Errorf("git init failed: %w", err)
		}
		printOK("", fmt.Sprintf("Local Git repo initialised: %s", repoPath))
	} else {
		printSkip("", fmt.Sprintf("Git repo already exists: %s", repoPath))
	}
	return nil
}

// setupHubWithRemote sets up the Hub for Mode B (personal remote).
// Returns (clonedFromRemote=true) only when the remote existed and was
// successfully cloned with content — in that case the caller should skip
// the local skill import to avoid overwriting remote data.
func setupHubWithRemote(repoPath, remote string) (clonedFromRemote bool, err error) {
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		printInfo("", fmt.Sprintf("Cloning %s → %s", remote, repoPath))
		if err := gitRun("clone", remote, repoPath); err == nil {
			if dirHasContent(repoPath) {
				printOK("", "Remote cloned (read-write mode).")
				return true, nil
			}
			// Clone succeeded but repo was empty — fall through to local init.
			printInfo("", "Remote repo is empty; initialising locally.")
		}
	}
	if err := setupHubLocal(repoPath); err != nil {
		return false, err
	}
	_ = gitRun("-C", repoPath, "remote", "add", "origin", remote)
	printOK("", fmt.Sprintf("Remote origin set: %s", remote))

	// Best-effort: fetch origin and set origin/HEAD to the remote's default branch.
	// This helps commands like `axon status --fetch` rely on origin/HEAD without guesswork.
	if out, err := gitOutput(repoPath, "fetch", "--prune", "origin"); err != nil {
		printWarn("", fmt.Sprintf("git fetch origin failed; remote default branch may be unknown:\n%s", strings.TrimSpace(out)))
	}
	if err := gitRun("-C", repoPath, "remote", "set-head", "origin", "-a"); err != nil {
		printWarn("", "could not set origin/HEAD automatically; remote default branch may be unknown")
	}

	return false, nil
}
