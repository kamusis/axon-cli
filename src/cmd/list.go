package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List local items grouped by category from axon.yaml",
	Long: `Scan the local Hub repo and print items grouped by category.

Categories are derived from the unique source paths defined in axon.yaml
(e.g. skills, workflows, commands). For each category, all immediate
children are listed by name — subdirectories for folder-based categories
like skills, or files for flat categories like workflows and commands.
No details are shown; use 'axon inspect <name>' for that.

Example:
  axon list`,
	Args: cobra.NoArgs,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

// itemInfo holds the name of an item and whether it is a directory.
type itemInfo struct {
	Name  string
	IsDir bool
}

// categoryItems holds a category label and its discovered items.
type categoryItems struct {
	Label string
	Items []itemInfo
}

// listItems derives unique categories from cfg.Targets and scans each
// source directory for immediate children.
func listItems(cfg *config.Config) []categoryItems {
	seen := make(map[string]bool)
	var result []categoryItems

	for _, t := range cfg.Targets {
		src := strings.TrimSpace(t.Source)
		if src == "" || seen[src] {
			continue
		}
		seen[src] = true

		label := filepath.Base(src)
		sourceDir := filepath.Join(cfg.RepoPath, src)

		entries, err := os.ReadDir(sourceDir)
		var items []itemInfo
		if err == nil {
			for _, e := range entries {
				name := e.Name()
				if strings.HasPrefix(name, ".") {
					continue // skip hidden entries
				}
				items = append(items, itemInfo{
					Name:  name,
					IsDir: e.IsDir(),
				})
			}
		}
		// Always include the category, even if empty or source dir missing.
		result = append(result, categoryItems{Label: label, Items: items})
	}
	return result
}

func runList(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	cats := listItems(cfg)
	if len(cats) == 0 {
		printWarn("", "No categories configured in axon.yaml.")
		return nil
	}

	printSection("Local Inventory")

	for _, cat := range cats {
		printBullet(strings.Title(cat.Label))
		if len(cat.Items) == 0 {
			printMiss("", "(empty)")
		} else {
			for _, item := range cat.Items {
				icon := "·"
				if item.IsDir {
					icon = "+"
				}
				printListItem(icon, item.Name)
			}
		}
	}
	return nil
}
