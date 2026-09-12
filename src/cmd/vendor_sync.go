package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
	"github.com/spf13/cobra"
)

var vendorSyncForce bool

var vendorSyncCmd = &cobra.Command{
	Use:   "sync [name]",
	Short: "Sync configured vendor entries into the Hub",
	Long: `vendor sync fetches external repo/subdir sources defined in the Hub's
axon.vendors.yaml (or legacy ~/.axon/axon.yaml) and mirrors them as plain files
into the Hub.

With no argument, every configured vendor entry is synced. Given a name,
only the matching vendor entry is synced.

Vendor content writes in-tree provenance (.axon-vendor.yaml) for tracking.
If the destination folder has uncommitted changes in the Hub Git repository,
sync will abort to protect local edits, unless --force is specified.`,
	Args:              cobra.MaximumNArgs(1),
	RunE:              runVendorSync,
	ValidArgsFunction: completeVendorNames,
}

// completeVendorNames provides shell <TAB> completion for `axon vendor sync
// [name]`, suggesting configured vendor names.
func completeVendorNames(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	vendors, _, err := vendor.LoadEffectiveVendors(cfg.RepoPath, cfg)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var matches []string
	for _, name := range vendorNames(vendors) {
		if strings.HasPrefix(name, toComplete) {
			matches = append(matches, name)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	vendorSyncCmd.Flags().BoolVarP(&vendorSyncForce, "force", "f", false, "Force sync and overwrite destination even if local uncommitted changes exist")
	vendorCmd.AddCommand(vendorSyncCmd)
}

func runVendorSync(_ *cobra.Command, args []string) error {
	if err := checkGitAvailable(); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	// Auto-migrate legacy vendors if present in local axon.yaml
	if len(cfg.Vendors) > 0 {
		migratedCount, migErr := vendor.MigrateLegacyVendors(cfg.RepoPath, cfg)
		if migErr != nil {
			printWarn("", fmt.Sprintf("auto-migration of legacy vendors failed: %v", migErr))
		} else if migratedCount > 0 {
			printOK("", fmt.Sprintf("migrated %d legacy vendor(s) from ~/.axon/axon.yaml into %s — this will be synchronized to your remote Hub on the next 'axon sync'", migratedCount, vendor.ManifestFileName))
		}
	}

	vendors, _, err := vendor.LoadEffectiveVendors(cfg.RepoPath, cfg)
	if err != nil {
		return fmt.Errorf("cannot load vendors: %w", err)
	}

	if len(vendors) == 0 {
		return fmt.Errorf("no vendors configured — add a vendor with 'axon vendor add' or configure %s", vendor.ManifestPath(cfg.RepoPath))
	}

	if len(args) == 1 {
		name := args[0]
		vendors, err = selectVendorByName(vendors, name)
		if err != nil {
			return err
		}
	}

	// Warn if rsync is unavailable (we'll fall back to rm+cp).
	if _, err := exec.LookPath("rsync"); err != nil {
		printWarn("", "rsync not found — will use cp fallback for mirroring")
	}

	// Validate all entries up front before touching the filesystem.
	if err := validateVendors(vendors); err != nil {
		return err
	}

	// Guard the vendor cache and Hub against a concurrent `axon vendor sync`
	// run — git fetch/checkout and rsync --delete are not safe to run
	// concurrently against the same directories.
	_, release, err := vendor.AcquireSyncLock()
	if err != nil {
		return err
	}
	defer release()

	printSection("Vendor Sync")

	type failedEntry struct {
		name string
		msg  string
	}

	var mirrored, skipped, failed int
	var syncedNames, skippedNames []string
	var failedEntries []failedEntry

	for _, v := range vendors {
		ok, err := syncVendorEntry(cfg.RepoPath, v, vendorSyncForce)
		if err != nil {
			printErr(v.Name, "failed")
			failed++
			failedEntries = append(failedEntries, failedEntry{name: v.Name, msg: err.Error()})
			continue
		}
		if ok {
			mirrored++
			syncedNames = append(syncedNames, v.Name)
		} else {
			skipped++
			skippedNames = append(skippedNames, v.Name)
		}
	}

	// Print grouped summary — always shown regardless of errors.
	if failed > 0 {
		printErr("", fmt.Sprintf("%d error(s), %d mirrored, %d skipped", failed, mirrored, skipped))
	} else {
		printOK("", fmt.Sprintf("%d mirrored, %d skipped", mirrored, skipped))
	}
	if len(syncedNames) > 0 {
		printBullet("Synced:")
		for _, name := range syncedNames {
			printOK(name, "")
		}
	}
	if len(skippedNames) > 0 {
		printBullet("Skipped (already up to date):")
		for _, name := range skippedNames {
			printSkip(name, "")
		}
	}
	if len(failedEntries) > 0 {
		printBullet("Errors:")
		for _, e := range failedEntries {
			printErr(e.name, e.msg)
		}
	}

	if failed > 0 {
		return fmt.Errorf("vendor sync failed (%d mirrored, %d skipped, %d error)", mirrored, skipped, failed)
	}
	return nil
}

// validateVendors checks all entries for required fields and duplicate names.
func validateVendors(vendors []config.Vendor) error {
	seen := make(map[string]struct{}, len(vendors))
	for i, v := range vendors {
		if v.Name == "" {
			return fmt.Errorf("vendors[%d]: 'name' is required", i)
		}
		if v.Repo == "" {
			return fmt.Errorf("vendor %q: 'repo' is required", v.Name)
		}
		if v.Subdir == "" {
			return fmt.Errorf("vendor %q: 'subdir' is required", v.Name)
		}
		if v.Dest == "" {
			return fmt.Errorf("vendor %q: 'dest' is required", v.Name)
		}
		if _, dup := seen[v.Name]; dup {
			return fmt.Errorf("duplicate vendor name %q — each vendor entry must have a unique name", v.Name)
		}
		seen[v.Name] = struct{}{}
	}
	return nil
}

// vendorNames returns the configured vendor names, in order. Shared by
// selectVendorByName's error message and the sync command's shell completion.
func vendorNames(vendors []config.Vendor) []string {
	names := make([]string, len(vendors))
	for i, v := range vendors {
		names[i] = v.Name
	}
	return names
}

// selectVendorByName returns the single vendor entry matching name, or an
// error listing the configured names when no entry matches.
func selectVendorByName(vendors []config.Vendor, name string) ([]config.Vendor, error) {
	for _, v := range vendors {
		if v.Name == name {
			return []config.Vendor{v}, nil
		}
	}
	return nil, fmt.Errorf("no vendor named %q configured — available vendors: %s", name, strings.Join(vendorNames(vendors), ", "))
}

// syncVendorEntry runs the full sync flow for one vendor entry.
// Returns (true, nil) when content was mirrored, (false, nil) when skipped
// because the destination is already up to date, or (false, err) on failure.
func syncVendorEntry(hubRoot string, v config.Vendor, force bool) (bool, error) {
	ref := v.Ref
	if ref == "" {
		ref = "main"
	}

	printInfo(v.Name, fmt.Sprintf("repo=%s subdir=%s ref=%s", v.Repo, v.Subdir, ref))

	cleanDest, err := vendor.ValidateDest(v.Dest)
	if err != nil {
		return false, err
	}
	destAbs := filepath.Join(hubRoot, cleanDest)

	// Dirty check: protect local modifications from being clobbered by rsync.
	if !force {
		dirty, dirtyErr := vendor.CheckDestDirty(hubRoot, cleanDest)
		if dirtyErr != nil {
			return false, fmt.Errorf("dirty check failed: %w", dirtyErr)
		}
		if dirty {
			return false, fmt.Errorf("destination %q has uncommitted changes in the Hub — commit or stash them first, or run with --force", cleanDest)
		}
	}

	// 1. Resolve cache path.
	cachePath, err := vendor.CachePath(v.Repo)
	if err != nil {
		return false, fmt.Errorf("cannot resolve cache path: %w", err)
	}

	// 2. Clone if not already cached.
	alreadyCached := vendor.IsCloned(cachePath)
	if !alreadyCached {
		printInfo(v.Name, "cloning repository into cache…")
		if err := vendor.Clone(v.Repo, cachePath); err != nil {
			return false, err
		}
		// 3. Configure sparse-checkout after fresh clone.
		if err := vendor.EnableSparseCheckout(cachePath, v.Subdir); err != nil {
			return false, err
		}
	}

	// 4. Fetch latest refs.
	printInfo(v.Name, "fetching remote refs…")
	if err := vendor.Fetch(cachePath); err != nil {
		return false, err
	}

	// 5. Up-to-date check: check stored commit SHA from in-tree .axon-vendor.yaml
	// (or fallback to ~/.axon/cache/vendors/<name>.sha).
	remoteRef := "origin/" + ref
	remoteSHA, err := vendor.SubdirLatestSHA(cachePath, remoteRef, v.Subdir)
	if err != nil {
		printWarn(v.Name, fmt.Sprintf("could not determine remote SHA: %v", err))
	}

	destMissing := false
	if info, statErr := os.Stat(destAbs); statErr != nil {
		if !os.IsNotExist(statErr) {
			return false, fmt.Errorf("cannot stat destination %q: %w", destAbs, statErr)
		}
		destMissing = true
	} else if !info.IsDir() {
		destMissing = true
	}

	var storedSHA string
	if !destMissing {
		prov, provErr := vendor.ReadProvenance(destAbs)
		if provErr == nil && prov != nil {
			storedSHA = prov.Commit
		}
	}
	if storedSHA == "" {
		storedSHA, _ = vendor.ReadVendorSHA(v.Name)
	}

	if !force && storedSHA != "" && remoteSHA != "" && storedSHA == remoteSHA && !destMissing {
		printOK(v.Name, fmt.Sprintf(
			"already up to date (%.8s) — no changes in %s, skipping mirror",
			remoteSHA, v.Subdir,
		))
		return false, nil
	}
	if force && storedSHA != "" && remoteSHA != "" && storedSHA == remoteSHA && !destMissing {
		printInfo(v.Name, fmt.Sprintf(
			"force sync specified — re-mirroring despite unchanged SHA (%.8s)",
			remoteSHA,
		))
	}
	if destMissing && storedSHA != "" && remoteSHA != "" && storedSHA == remoteSHA {
		printInfo(v.Name, fmt.Sprintf(
			"destination %s missing — re-mirroring despite unchanged SHA (%.8s)",
			v.Dest, remoteSHA,
		))
	}

	// 6. Ensure this subdir is included in the sparse-checkout cone.
	if alreadyCached {
		if err := vendor.AddSparseCheckoutDir(cachePath, v.Subdir); err != nil {
			return false, err
		}
	}

	// 7. Checkout requested ref.
	printInfo(v.Name, fmt.Sprintf("checking out %s…", ref))
	if err := vendor.Checkout(cachePath, ref); err != nil {
		return false, err
	}

	// 8. Verify subdir exists in the checked-out tree.
	src, err := vendor.SourcePath(cachePath, v.Subdir)
	if err != nil {
		return false, err
	}

	// 9. Mirror into Hub.
	printInfo(v.Name, fmt.Sprintf("mirroring %s → %s…", v.Subdir, v.Dest))
	if err := vendor.Mirror(hubRoot, cleanDest, src); err != nil {
		return false, err
	}

	// 10. Write provenance metadata and cache SHA.
	if remoteSHA != "" {
		prov := vendor.Provenance{
			Vendor:   v.Name,
			Repo:     v.Repo,
			Subdir:   v.Subdir,
			Ref:      ref,
			Commit:   remoteSHA,
			SyncedAt: time.Now().UTC(),
		}
		if err := vendor.WriteProvenance(destAbs, prov); err != nil {
			printWarn(v.Name, fmt.Sprintf("could not write provenance: %v", err))
		}
		_ = vendor.WriteVendorSHA(v.Name, remoteSHA)
	}

	printOK(v.Name, fmt.Sprintf("successfully mirrored %s@%s → %s", v.Subdir, ref, v.Dest))
	return true, nil
}
