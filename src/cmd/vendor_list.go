package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
	"github.com/spf13/cobra"
)

var vendorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured vendor entries and their sync health",
	Long: `vendor list prints every entry declared in the Hub's axon.vendors.yaml
(or legacy ~/.axon/axon.yaml) alongside its local sync status, without touching
the network.

STATUS is one of:
  synced (<sha>)  the last-mirrored commit, and the Hub destination exists
  pending         never synced (no recorded commit for this entry)
  missing dest    a commit was recorded, but the Hub destination is gone

Use 'axon vendor sync <name>' to (re-)sync a single entry shown here.`,
	Args: cobra.NoArgs,
	RunE: runVendorList,
}

func init() {
	vendorCmd.AddCommand(vendorListCmd)
}

func runVendorList(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	vendors, isHub, err := vendor.LoadEffectiveVendors(cfg.RepoPath, cfg)
	if err != nil {
		return fmt.Errorf("cannot load vendors: %w", err)
	}

	if len(vendors) == 0 {
		printWarn("", fmt.Sprintf("no vendors configured — add vendors with 'axon vendor add' or configure %s", vendor.ManifestPath(cfg.RepoPath)))
		return nil
	}

	if !isHub {
		printWarn("", "vendors are defined in legacy ~/.axon/axon.yaml — they will be automatically migrated into the Hub on your next 'axon vendor sync' or 'axon vendor add'")
	}

	if err := validateVendors(vendors); err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDEST\tREF\tSTATUS\tREPO")
	for _, v := range vendors {
		ref := v.Ref
		if ref == "" {
			ref = "main"
		}
		status, err := vendorSyncStatus(cfg.RepoPath, v)
		if err != nil {
			status = fmt.Sprintf("error: %v", err)
		}
		repoCol := v.Repo
		if v.Subdir != "." {
			repoCol = fmt.Sprintf("%s (%s)", v.Repo, v.Subdir)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", v.Name, v.Dest, ref, status, repoCol)
	}
	return w.Flush()
}

// vendorSyncStatus reports the local sync health of a single vendor entry
// without any network access: "synced (<sha>)", "legacy (<sha>)", "pending", or "missing dest".
func vendorSyncStatus(hubRoot string, v config.Vendor) (string, error) {
	cleanDest, err := vendor.ValidateDest(v.Dest)
	if err != nil {
		return "", err
	}
	destAbs := filepath.Join(hubRoot, cleanDest)
	info, statErr := os.Stat(destAbs)
	destExists := statErr == nil && info.IsDir()

	// 1. Check in-tree provenance metadata (.axon-vendor.yaml) if dest exists
	if destExists {
		prov, err := vendor.ReadProvenance(destAbs)
		if err == nil && prov != nil && prov.Commit != "" {
			return fmt.Sprintf("synced (%.8s)", prov.Commit), nil
		}
	}

	// 2. Fallback to local cache .sha
	storedSHA, err := vendor.ReadVendorSHA(v.Name)
	if err != nil {
		return "", fmt.Errorf("could not read stored SHA: %w", err)
	}

	if storedSHA == "" {
		return "pending", nil
	}

	if !destExists {
		return "missing dest", nil
	}

	return fmt.Sprintf("legacy (%.8s)", storedSHA), nil
}
