package cmd

import (
	"fmt"
	"os/exec"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
	"github.com/spf13/cobra"
)

var vendorSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync all configured vendor entries into the Hub",
	Long: `vendor sync fetches each external repo/subdir listed in the 'vendors'
block of ~/.axon/axon.yaml and mirrors it as plain files into the Hub.

Vendor content overwrites the Hub destination on every run (force-overwrite).
No nested .git directories are written inside the Hub.`,
	RunE: runVendorSync,
}

func init() {
	vendorCmd.AddCommand(vendorSyncCmd)
}

func runVendorSync(_ *cobra.Command, _ []string) error {
	if err := checkGitAvailable(); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	if len(cfg.Vendors) == 0 {
		return fmt.Errorf("no vendors configured — add a 'vendors' block to ~/.axon/axon.yaml")
	}

	// Warn if rsync is unavailable (we'll fall back to rm+cp).
	if _, err := exec.LookPath("rsync"); err != nil {
		printWarn("", "rsync not found — will use cp fallback for mirroring")
	}

	// Validate all entries up front before touching the filesystem.
	if err := validateVendors(cfg.Vendors); err != nil {
		return err
	}

	printSection("Vendor Sync")

	var mirrored, skipped, failed int
	for _, v := range cfg.Vendors {
		ok, err := syncVendorEntry(cfg.RepoPath, v)
		if err != nil {
			printErr(v.Name, err.Error())
			failed++
			// Stop on first hard failure (MVP behaviour per plan).
			break
		}
		if ok {
			mirrored++
		} else {
			skipped++
		}
	}

	if failed > 0 {
		return fmt.Errorf("vendor sync failed (%d mirrored, %d skipped, %d error)", mirrored, skipped, failed)
	}

	printOK("", fmt.Sprintf("%d mirrored, %d skipped, %d error", mirrored, skipped, failed))
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

// syncVendorEntry runs the full sync flow for one vendor entry.
// Returns (true, nil) when content was mirrored, (false, nil) when skipped
// because the destination is already up to date, or (false, err) on failure.
func syncVendorEntry(hubRoot string, v config.Vendor) (bool, error) {
	ref := v.Ref
	if ref == "" {
		ref = "main"
	}

	printInfo(v.Name, fmt.Sprintf("repo=%s subdir=%s ref=%s", v.Repo, v.Subdir, ref))

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

	// 5. Up-to-date check: compare the stored last-mirrored SHA against the
	//    current remote SHA for this subdir.  Using a per-entry stored SHA
	//    (rather than HEAD) avoids false "already up to date" results when
	//    multiple entries share the same repo cache — after the first entry is
	//    processed HEAD advances to origin/<ref>, making every subsequent
	//    entry appear current even if its subdir was never mirrored.
	remoteRef := "origin/" + ref
	remoteSHA, err := vendor.SubdirLatestSHA(cachePath, remoteRef, v.Subdir)
	if err != nil {
		// Log a warning if we can't get remote SHA, but keep going.
		printWarn(v.Name, fmt.Sprintf("could not determine remote SHA: %v", err))
	}
	storedSHA, err := vendor.ReadVendorSHA(v.Name)
	if err != nil {
		// Log a warning if we can't read stored SHA, but keep going.
		printWarn(v.Name, fmt.Sprintf("could not read stored SHA: %v", err))
	}

	if storedSHA != "" && remoteSHA != "" && storedSHA == remoteSHA {
		printOK(v.Name, fmt.Sprintf(
			"already up to date (%.8s) — no changes in %s, skipping mirror",
			remoteSHA, v.Subdir,
		))
		return false, nil
	}

	// 6. Ensure this subdir is included in the sparse-checkout cone.
	//    For fresh clones this was done in step 3; for cached repos we add the
	//    subdir here so that a second entry sharing the same repo cache gets
	//    its files checked out too (git sparse-checkout add is idempotent).
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

	// 9. Validate and mirror into Hub.
	cleanDest, err := vendor.ValidateDest(v.Dest)
	if err != nil {
		return false, err
	}

	printInfo(v.Name, fmt.Sprintf("mirroring %s → %s…", v.Subdir, v.Dest))
	if err := vendor.Mirror(hubRoot, cleanDest, src); err != nil {
		return false, err
	}

	// 10. Record the mirrored SHA so future runs can skip unchanged entries.
	//     Errors here are non-fatal — worst case the next run re-mirrors.
	if remoteSHA != "" {
		_ = vendor.WriteVendorSHA(v.Name, remoteSHA)
	}

	printOK(v.Name, fmt.Sprintf("successfully mirrored %s@%s → %s", v.Subdir, ref, v.Dest))
	return true, nil
}
