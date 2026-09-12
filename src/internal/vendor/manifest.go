package vendor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamusis/axon-cli/internal/config"
	"gopkg.in/yaml.v3"
)

// ManifestFileName is the Hub-level file declaring vendor dependencies.
const ManifestFileName = "axon.vendors.yaml"

type manifestFile struct {
	Vendors []config.Vendor `yaml:"vendors"`
}

// ManifestPath returns the absolute path to axon.vendors.yaml in hubRoot.
func ManifestPath(hubRoot string) string {
	return filepath.Join(hubRoot, ManifestFileName)
}

// ManifestExists reports whether axon.vendors.yaml exists in hubRoot.
func ManifestExists(hubRoot string) bool {
	info, err := os.Stat(ManifestPath(hubRoot))
	return err == nil && !info.IsDir()
}

// ReadManifest reads and unmarshals axon.vendors.yaml from hubRoot.
// Returns (nil, nil) if the file does not exist.
func ReadManifest(hubRoot string) ([]config.Vendor, error) {
	path := ManifestPath(hubRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read vendor manifest %s: %w", path, err)
	}

	var m manifestFile
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid YAML in vendor manifest %s: %w", path, err)
	}
	return m.Vendors, nil
}

// WriteManifest marshals vendors and writes to axon.vendors.yaml in hubRoot.
func WriteManifest(hubRoot string, vendors []config.Vendor) error {
	data, err := yaml.Marshal(manifestFile{Vendors: vendors})
	if err != nil {
		return fmt.Errorf("cannot marshal vendor manifest: %w", err)
	}

	path := ManifestPath(hubRoot)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("cannot write vendor manifest %s: %w", path, err)
	}
	return nil
}

// LoadEffectiveVendors returns the active vendor entries for the given hub.
// Priority:
// 1. If <hubRoot>/axon.vendors.yaml exists, it is loaded (isHubManaged = true).
// 2. Otherwise, if localCfg has vendors configured, those are used (isHubManaged = false).
// 3. Otherwise, returns nil slice (isHubManaged = false).
func LoadEffectiveVendors(hubRoot string, localCfg *config.Config) (vendors []config.Vendor, isHubManaged bool, err error) {
	if ManifestExists(hubRoot) {
		v, err := ReadManifest(hubRoot)
		if err != nil {
			return nil, false, err
		}
		return v, true, nil
	}

	if localCfg != nil && len(localCfg.Vendors) > 0 {
		return localCfg.Vendors, false, nil
	}

	return nil, false, nil
}

// MigrateLegacyVendors moves any legacy vendor entries from localCfg into
// <hubRoot>/axon.vendors.yaml, clears localCfg.Vendors, and writes both files.
// Returns the number of newly migrated entries, or 0 if localCfg has no vendors.
func MigrateLegacyVendors(hubRoot string, localCfg *config.Config) (int, error) {
	if localCfg == nil || len(localCfg.Vendors) == 0 {
		return 0, nil
	}

	existing, err := ReadManifest(hubRoot)
	if err != nil {
		return 0, fmt.Errorf("cannot read vendor manifest for migration: %w", err)
	}

	seen := make(map[string]struct{}, len(existing))
	for _, v := range existing {
		seen[v.Name] = struct{}{}
	}

	var added int
	merged := append([]config.Vendor{}, existing...)
	for _, v := range localCfg.Vendors {
		if _, exists := seen[v.Name]; !exists {
			merged = append(merged, v)
			seen[v.Name] = struct{}{}
			added++
		}
	}

	if err := WriteManifest(hubRoot, merged); err != nil {
		return 0, fmt.Errorf("cannot write vendor manifest during migration: %w", err)
	}

	localCfg.Vendors = nil
	if err := config.Save(localCfg); err != nil {
		return 0, fmt.Errorf("cannot update config after migration: %w", err)
	}

	return added, nil
}
