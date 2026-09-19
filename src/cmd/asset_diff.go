package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// AssetChangeType represents the type of change applied to an asset.
type AssetChangeType string

const (
	ChangeNew     AssetChangeType = "new"
	ChangeUpdated AssetChangeType = "updated"
	ChangeDeleted AssetChangeType = "deleted"
)

// FileDiff represents a single file status and path from git diff or status.
type FileDiff struct {
	Status string // "A", "M", "D", "R", "??"
	Path   string
}

// AssetChange represents aggregated changes for an individual asset.
type AssetChange struct {
	Category string
	Name     string
	Type     AssetChangeType
	Files    []FileDiff
}

// CategoryGroup groups assets under an asset category (e.g. Skills, Rules).
type CategoryGroup struct {
	Category string
	Assets   []AssetChange
}

var canonicalCategoryOrder = []string{"Skills", "Rules", "Workflows", "Commands", "Config", "Other"}

// classifyAssetPath determines the category and asset name from a repository file path.
func classifyAssetPath(filePath string) (string, string) {
	// Normalize path separators to forward slash.
	p := filepath.ToSlash(strings.TrimSpace(filePath))
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")

	parts := strings.Split(p, "/")
	if len(parts) == 0 || p == "" {
		return "Other", filePath
	}

	switch strings.ToLower(parts[0]) {
	case "skills":
		if len(parts) >= 2 && parts[1] != "" {
			return "Skills", parts[1]
		}
		return "Skills", parts[0]
	case "rules":
		if len(parts) >= 2 && parts[1] != "" {
			return "Rules", parts[1]
		}
		return "Rules", parts[0]
	case "workflows":
		if len(parts) >= 2 && parts[1] != "" {
			return "Workflows", strings.TrimSuffix(parts[1], ".md")
		}
		return "Workflows", strings.TrimSuffix(parts[0], ".md")
	case "commands":
		if len(parts) >= 2 && parts[1] != "" {
			return "Commands", strings.TrimSuffix(parts[1], ".md")
		}
		return "Commands", strings.TrimSuffix(parts[0], ".md")
	default:
		if p == "axon.yaml" || p == ".gitignore" || strings.HasPrefix(p, ".axon") {
			return "Config", p
		}
		return "Other", p
	}
}

// determineAssetChangeType computes aggregated change type from its constituent files.
func determineAssetChangeType(files []FileDiff) AssetChangeType {
	if len(files) == 0 {
		return ChangeUpdated
	}
	allNew := true
	allDel := true
	for _, f := range files {
		st := strings.ToUpper(strings.TrimSpace(f.Status))
		if st != "A" && st != "??" {
			allNew = false
		}
		if st != "D" {
			allDel = false
		}
	}
	if allNew {
		return ChangeNew
	}
	if allDel {
		return ChangeDeleted
	}
	return ChangeUpdated
}

// parseDiffNameStatus parses raw git diff --name-status output into CategoryGroups.
func parseDiffNameStatus(diffOutput string) []CategoryGroup {
	lines := strings.Split(diffOutput, "\n")
	var diffs []FileDiff
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		rawStatus := strings.TrimSpace(fields[0])
		statusChar := "M"
		if len(rawStatus) > 0 {
			switch rawStatus[0] {
			case 'A':
				statusChar = "A"
			case 'D':
				statusChar = "D"
			case 'R':
				statusChar = "R"
			default:
				statusChar = "M"
			}
		}
		// For renames (R100\told\tnew), use the destination path (fields[len(fields)-1]).
		targetPath := strings.TrimSpace(fields[len(fields)-1])
		diffs = append(diffs, FileDiff{Status: statusChar, Path: targetPath})
	}
	return groupFileDiffs(diffs)
}

// parsePorcelainStatus parses raw git status --porcelain output into CategoryGroups.
func parsePorcelainStatus(porcelainOutput string) []CategoryGroup {
	lines := strings.Split(porcelainOutput, "\n")
	var diffs []FileDiff
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		statusCode := line[:2]
		rest := strings.TrimSpace(line[3:])
		if rest == "" {
			continue
		}
		if idx := strings.Index(rest, " -> "); idx != -1 {
			rest = strings.TrimSpace(rest[idx+4:])
		}
		// Trim quotes if path is quoted by git
		rest = strings.Trim(rest, "\"")

		statusChar := "M"
		if strings.Contains(statusCode, "??") || strings.Contains(statusCode, "A") {
			statusChar = "A"
		} else if strings.Contains(statusCode, "D") {
			statusChar = "D"
		} else if strings.Contains(statusCode, "R") {
			statusChar = "R"
		}

		diffs = append(diffs, FileDiff{Status: statusChar, Path: rest})
	}
	return groupFileDiffs(diffs)
}

// groupFileDiffs aggregates FileDiff entries by Category and AssetName.
func groupFileDiffs(diffs []FileDiff) []CategoryGroup {
	type assetKey struct {
		Category string
		Name     string
	}
	assetMap := make(map[assetKey][]FileDiff)
	for _, d := range diffs {
		cat, name := classifyAssetPath(d.Path)
		k := assetKey{Category: cat, Name: name}
		assetMap[k] = append(assetMap[k], d)
	}

	categoryMap := make(map[string][]AssetChange)
	for k, files := range assetMap {
		changeType := determineAssetChangeType(files)
		categoryMap[k.Category] = append(categoryMap[k.Category], AssetChange{
			Category: k.Category,
			Name:     k.Name,
			Type:     changeType,
			Files:    files,
		})
	}

	var groups []CategoryGroup
	for cat, assets := range categoryMap {
		sort.Slice(assets, func(i, j int) bool {
			return assets[i].Name < assets[j].Name
		})
		groups = append(groups, CategoryGroup{
			Category: cat,
			Assets:   assets,
		})
	}

	sortCategoryGroups(groups)
	return groups
}

// sortCategoryGroups sorts CategoryGroup slices in canonical category order.
func sortCategoryGroups(groups []CategoryGroup) {
	weight := func(cat string) int {
		for i, c := range canonicalCategoryOrder {
			if strings.EqualFold(cat, c) {
				return i
			}
		}
		return len(canonicalCategoryOrder) + 1
	}

	sort.Slice(groups, func(i, j int) bool {
		wI, wJ := weight(groups[i].Category), weight(groups[j].Category)
		if wI != wJ {
			return wI < wJ
		}
		return groups[i].Category < groups[j].Category
	})
}

// countAssets counts total unique assets across all category groups.
func countAssets(groups []CategoryGroup) int {
	total := 0
	for _, g := range groups {
		total += len(g.Assets)
	}
	return total
}

// mergeCategoryGroups combines multiple slices of CategoryGroups.
func mergeCategoryGroups(groupLists ...[]CategoryGroup) []CategoryGroup {
	var allDiffs []FileDiff
	seenPath := make(map[string]bool)
	for _, groups := range groupLists {
		for _, g := range groups {
			for _, a := range g.Assets {
				for _, f := range a.Files {
					if !seenPath[f.Path] {
						seenPath[f.Path] = true
						allDiffs = append(allDiffs, f)
					}
				}
			}
		}
	}
	return groupFileDiffs(allDiffs)
}

// printAssetCategoryGroups prints structured asset diffs.
func printAssetCategoryGroups(w io.Writer, groups []CategoryGroup, verbose bool) {
	for _, g := range groups {
		fmt.Fprintf(w, "  ● %s:\n", g.Category)
		for _, a := range aAssetsSorted(g.Assets) {
			icon := iconInfo
			typeLabel := "updated"
			switch a.Type {
			case ChangeNew:
				icon = iconDir
				typeLabel = "new"
			case ChangeDeleted:
				icon = iconMiss
				typeLabel = "deleted"
			}

			if len(a.Files) > 1 {
				fmt.Fprintf(w, "    %s  %s (%s, %d files)\n", icon, a.Name, typeLabel, len(a.Files))
			} else {
				fmt.Fprintf(w, "    %s  %s (%s)\n", icon, a.Name, typeLabel)
			}

			if verbose {
				for _, f := range a.Files {
					fmt.Fprintf(w, "         %s  %s\n", f.Status, f.Path)
				}
			}
		}
	}
}

func aAssetsSorted(assets []AssetChange) []AssetChange {
	out := make([]AssetChange, len(assets))
	copy(out, assets)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
