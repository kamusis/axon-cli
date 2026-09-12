package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
	"github.com/spf13/cobra"
)

var vendorEjectCmd = &cobra.Command{
	Use:   "eject <name>",
	Short: "Convert a vendored skill into a first-party skill",
	Long: `vendor eject removes an external repository source from axon.vendors.yaml
and deletes its in-tree provenance marker (.axon-vendor.yaml).

The skill's files remain in place inside the Hub, transforming it into a
first-party skill that you can freely edit without risk of being overwritten
by future 'axon vendor sync' runs.`,
	Args:              cobra.ExactArgs(1),
	RunE:              runVendorEject,
	ValidArgsFunction: completeVendorNames,
}

func init() {
	vendorCmd.AddCommand(vendorEjectCmd)
}

func runVendorEject(_ *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	manifestVendors, err := vendor.ReadManifest(cfg.RepoPath)
	if err != nil {
		return fmt.Errorf("cannot read vendor manifest: %w", err)
	}

	var found *config.Vendor
	var remaining []config.Vendor
	for _, v := range manifestVendors {
		if v.Name == name {
			entry := v
			found = &entry
		} else {
			remaining = append(remaining, v)
		}
	}

	if found == nil {
		// Check if it's in legacy local axon.yaml
		for _, v := range cfg.Vendors {
			if v.Name == name {
				return fmt.Errorf("vendor %q is in legacy ~/.axon/axon.yaml — run 'axon vendor migrate' first", name)
			}
		}
		return fmt.Errorf("no vendor named %q found in %s", name, vendor.ManifestPath(cfg.RepoPath))
	}

	// Update manifest
	if err := vendor.WriteManifest(cfg.RepoPath, remaining); err != nil {
		return fmt.Errorf("cannot update vendor manifest: %w", err)
	}

	// Remove .axon-vendor.yaml in dest
	cleanDest, err := vendor.ValidateDest(found.Dest)
	if err == nil {
		provPath := filepath.Join(cfg.RepoPath, cleanDest, vendor.ProvenanceFileName)
		_ = os.Remove(provPath)
	}

	printOK(name, fmt.Sprintf("ejected from %s — files in %s preserved as first-party skill", vendor.ManifestFileName, found.Dest))
	return nil
}
