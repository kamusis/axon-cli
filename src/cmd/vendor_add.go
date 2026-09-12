package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
	"github.com/spf13/cobra"
)

var (
	vendorAddSubdir string
	vendorAddDest   string
	vendorAddRef    string
	vendorAddNoSync bool
)

var vendorAddCmd = &cobra.Command{
	Use:   "add <name> <repo>",
	Short: "Add a new vendor entry to the Hub manifest",
	Long: `Add a new external repository source into axon.vendors.yaml in the Hub.
By default, immediately mirrors the vendor content into the Hub. Pass --no-sync
to only record the entry without syncing.`,
	Args: cobra.ExactArgs(2),
	RunE: runVendorAdd,
}

func init() {
	vendorAddCmd.Flags().StringVarP(&vendorAddSubdir, "subdir", "s", ".", "Subdirectory inside the external repo to mirror")
	vendorAddCmd.Flags().StringVarP(&vendorAddDest, "dest", "d", "", "Destination path inside Hub (defaults to skills/<name>)")
	vendorAddCmd.Flags().StringVarP(&vendorAddRef, "ref", "r", "main", "Git branch, tag, or commit ref to track")
	vendorAddCmd.Flags().BoolVar(&vendorAddNoSync, "no-sync", false, "Do not immediately mirror files after adding")
	vendorCmd.AddCommand(vendorAddCmd)
}

func runVendorAdd(_ *cobra.Command, args []string) error {
	name := args[0]
	repo := args[1]

	dest := vendorAddDest
	if dest == "" {
		dest = filepath.ToSlash(filepath.Join("skills", name))
	}

	cleanDest, err := vendor.ValidateDest(dest)
	if err != nil {
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

	// Read or initialize Hub manifest.
	existing, err := vendor.ReadManifest(cfg.RepoPath)
	if err != nil {
		return fmt.Errorf("cannot read vendor manifest: %w", err)
	}

	for _, v := range existing {
		if v.Name == name {
			return fmt.Errorf("vendor %q already exists in %s", name, vendor.ManifestPath(cfg.RepoPath))
		}
	}

	newEntry := config.Vendor{
		Name:   name,
		Repo:   repo,
		Subdir: vendorAddSubdir,
		Dest:   cleanDest,
		Ref:    vendorAddRef,
	}

	updated := append(existing, newEntry)
	if err := validateVendors(updated); err != nil {
		return err
	}

	if err := vendor.WriteManifest(cfg.RepoPath, updated); err != nil {
		return fmt.Errorf("cannot save vendor manifest: %w", err)
	}

	printOK(name, fmt.Sprintf("added to %s (dest=%s, repo=%s, ref=%s)", vendor.ManifestFileName, cleanDest, repo, vendorAddRef))

	if vendorAddNoSync {
		printBullet("Sync skipped (--no-sync specified). Run 'axon vendor sync " + name + "' when ready.")
		return nil
	}

	_, release, err := vendor.AcquireSyncLock()
	if err != nil {
		return err
	}
	defer release()

	printSection("Vendor Sync")
	_, _, syncErr := syncVendorEntry(cfg.RepoPath, newEntry, vendorSyncForce)
	return syncErr
}
