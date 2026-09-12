package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/vendor"
	"github.com/spf13/cobra"
)

var vendorCheckCmd = &cobra.Command{
	Use:   "check [name]",
	Short: "Check for upstream updates without modifying files",
	Long: `vendor check queries the remote repository for each vendor to detect
upstream changes. It fetches remote refs in the local cache, compares the
remote commit SHA against the locally mirrored commit, and reports whether
updates are available without altering any files in the Hub.`,
	Args:              cobra.MaximumNArgs(1),
	RunE:              runVendorCheck,
	ValidArgsFunction: completeVendorNames,
}

func init() {
	vendorCmd.AddCommand(vendorCheckCmd)
}

func runVendorCheck(_ *cobra.Command, args []string) error {
	if err := checkGitAvailable(); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	vendors, isHub, err := vendor.LoadEffectiveVendors(cfg.RepoPath, cfg)
	if err != nil {
		return fmt.Errorf("cannot load vendors: %w", err)
	}

	if len(vendors) == 0 {
		return fmt.Errorf("no vendors configured — add vendors with 'axon vendor add' or configure %s", vendor.ManifestPath(cfg.RepoPath))
	}

	if !isHub {
		printWarn("", "vendors are configured in legacy ~/.axon/axon.yaml — they will be automatically migrated into the Hub on your next 'axon vendor sync' or 'axon vendor add'")
	}

	if len(args) == 1 {
		name := args[0]
		vendors, err = selectVendorByName(vendors, name)
		if err != nil {
			return err
		}
	}

	printSection("Vendor Check")

	for _, v := range vendors {
		ref := v.Ref
		if ref == "" {
			ref = "main"
		}

		cachePath, err := vendor.CachePath(v.Repo)
		if err != nil {
			printErr(v.Name, fmt.Sprintf("cannot resolve cache: %v", err))
			continue
		}

		if !vendor.IsCloned(cachePath) {
			printWarn(v.Name, "cache not initialized — run 'axon vendor sync' to clone and mirror")
			continue
		}

		if err := vendor.Fetch(cachePath); err != nil {
			printErr(v.Name, fmt.Sprintf("fetch failed: %v", err))
			continue
		}

		remoteRef := "origin/" + ref
		remoteSHA, err := vendor.SubdirLatestSHA(cachePath, remoteRef, v.Subdir)
		if err != nil {
			printErr(v.Name, fmt.Sprintf("could not determine remote SHA: %v", err))
			continue
		}

		cleanDest, err := vendor.ValidateDest(v.Dest)
		if err != nil {
			printErr(v.Name, fmt.Sprintf("invalid dest: %v", err))
			continue
		}
		destAbs := filepath.Join(cfg.RepoPath, cleanDest)

		var localSHA string
		hasProvenance := false
		prov, provErr := vendor.ReadProvenance(destAbs)
		if provErr == nil && prov != nil && prov.Commit != "" {
			localSHA = prov.Commit
			hasProvenance = true
		}
		if localSHA == "" {
			localSHA, _ = vendor.ReadVendorSHA(v.Name)
		}

		if localSHA == "" {
			printWarn(v.Name, fmt.Sprintf("never synced (upstream is %.8s on %s)", remoteSHA, ref))
		} else if !hasProvenance {
			printWarn(v.Name, fmt.Sprintf("legacy cache %.8s — missing in-tree provenance (run 'axon vendor sync %s' to update)", localSHA, v.Name))
		} else if remoteSHA != "" && localSHA == remoteSHA {
			printOK(v.Name, fmt.Sprintf("up to date (%.8s on %s)", localSHA, ref))
		} else if remoteSHA != "" {
			printWarn(v.Name, fmt.Sprintf("outdated: local %.8s -> upstream %.8s (run 'axon vendor sync %s' to update)", localSHA, remoteSHA, v.Name))
		} else {
			printInfo(v.Name, fmt.Sprintf("local %.8s (upstream ref %s not found)", localSHA, ref))
		}
	}

	return nil
}
