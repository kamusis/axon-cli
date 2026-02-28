package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kamusis/axon-cli/internal/config"
	"github.com/kamusis/axon-cli/internal/embeddings"
	"github.com/kamusis/axon-cli/internal/search"
	searchindex "github.com/kamusis/axon-cli/internal/search/index"
	"github.com/spf13/cobra"
)

var (
	flagSearchIndex    bool
	flagSearchKeyword  bool
	flagSearchSemantic bool
	flagSearchK        int
	flagSearchMinScore float64
	flagSearchDebug    bool
	flagSearchForce    bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search skills by keyword or semantic similarity",
	Args:  cobra.MinimumNArgs(0),
	RunE:  runSearch,
}

func init() {
	searchCmd.Flags().BoolVar(&flagSearchIndex, "index", false, "Build/update a local semantic index (~/.axon/search)")
	searchCmd.Flags().BoolVar(&flagSearchKeyword, "keyword", false, "Force keyword search only")
	searchCmd.Flags().BoolVar(&flagSearchSemantic, "semantic", false, "Force semantic search only (error if unavailable)")
	searchCmd.Flags().IntVar(&flagSearchK, "k", 5, "Number of results to show")
	searchCmd.Flags().Float64Var(&flagSearchMinScore, "min-score", 0, "Minimum cosine similarity score to include (semantic only)")
	searchCmd.Flags().BoolVar(&flagSearchDebug, "debug", false, "Print debug information")
	searchCmd.Flags().BoolVar(&flagSearchForce, "force", false, "Force re-indexing even if no changes detected")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cannot load config: %w\nRun 'axon init' first.", err)
	}

	minScore := resolveSemanticMinScore(cmd)

	if flagSearchIndex {
		return runSearchIndex(cmd, cfg)
	}

	if len(args) == 0 {
		return cmd.Help()
	}
	query := strings.Join(args, " ")

	// Keyword-only mode.
	if flagSearchKeyword {
		return runSearchKeyword(cfg, query)
	}

	// Default: attempt semantic; fallback to keyword on failure.
	if flagSearchSemantic {
		return runSearchSemanticStrict(cfg, query, minScore)
	}

	if err := runSearchSemanticBestEffort(cfg, query, minScore); err == nil {
		return nil
	}
	return runSearchKeyword(cfg, query)
}

func runSearchKeyword(cfg *config.Config, query string) error {
	docs, err := search.DiscoverDocuments(cfg.RepoPath, cfg.EffectiveSearchRoots())
	if err != nil {
		return err
	}
	results := search.KeywordSearch(docs, query, flagSearchK)
	printSearchResults(query, results)
	return nil
}

func runSearchSemanticBestEffort(cfg *config.Config, query string, minScore float64) error {
	res, err := semanticSearch(cfg, query, minScore)
	if err != nil {
		if flagSearchDebug {
			printInfo("", fmt.Sprintf("semantic search unavailable, falling back to keyword: %v", err))
		}
		return err
	}
	printSearchResults(query, res)
	return nil
}

func runSearchSemanticStrict(cfg *config.Config, query string, minScore float64) error {
	res, err := semanticSearch(cfg, query, minScore)
	if err != nil {
		return err
	}
	printSearchResults(query, res)
	return nil
}

func semanticSearch(cfg *config.Config, query string, minScore float64) ([]search.SearchResult, error) {
	idx, idxDir, err := selectSemanticIndex(cfg)
	if err != nil {
		return nil, err
	}

	embCfg, err := embeddings.LoadConfig()
	if err != nil {
		return nil, err
	}
	prov, err := embeddings.NewFromConfig(embCfg)
	if err != nil {
		return nil, err
	}
	if prov.ModelID() != idx.Manifest.ModelID {
		return nil, fmt.Errorf("embeddings model mismatch: index=%s provider=%s (index dir %s)", idx.Manifest.ModelID, prov.ModelID(), idxDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	qv, err := prov.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(qv) != idx.Manifest.Dim {
		return nil, fmt.Errorf("query embedding dim mismatch: got %d want %d", len(qv), idx.Manifest.Dim)
	}
	if idx.Manifest.Normalize {
		qv = searchindex.NormalizeL2(qv)
	}

	results := make([]search.SearchResult, 0, len(idx.Skills))
	for i, s := range idx.Skills {
		start := i * idx.Manifest.Dim
		end := start + idx.Manifest.Dim
		sv := idx.Vectors[start:end]
		score, err := searchindex.Cosine(qv, sv)
		if err != nil {
			return nil, err
		}
		if minScore > 0 && score < minScore {
			continue
		}
		results = append(results, search.SearchResult{
			Skill: search.SkillDoc{
				ID:          s.ID,
				Path:        s.Path,
				Name:        s.Name,
				Description: s.Description,
			},
			Score: score,
			Why:   "semantic",
		})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no semantic results above min score %.3f", minScore)
	}

	// Sort by score desc.
	search.SortResults(results)
	if flagSearchK > 0 && len(results) > flagSearchK {
		results = results[:flagSearchK]
	}

	if flagSearchDebug {
		printInfo("", fmt.Sprintf("semantic index used: %s", idxDir))
	}
	return results, nil
}

func resolveSemanticMinScore(cmd *cobra.Command) float64 {
	const defaultMinScore = 0.30

	// If user explicitly sets --min-score, always honor it.
	if cmd.Flags().Changed("min-score") {
		return flagSearchMinScore
	}

	// If user explicitly sets --k, do not apply any default filtering.
	if cmd.Flags().Changed("k") {
		return 0
	}

	// Otherwise apply a default threshold to avoid irrelevant tail results.
	return defaultMinScore
}

func selectSemanticIndex(cfg *config.Config) (*searchindex.Index, string, error) {
	axonDir, err := config.AxonDir()
	if err != nil {
		return nil, "", err
	}
	userDir := filepath.Join(axonDir, "search")
	repoDir := filepath.Join(cfg.RepoPath, "search")

	// Prefer user index if it loads.
	if idx, err := tryLoadIndex(userDir); err == nil {
		return idx, userDir, nil
	}
	if idx, err := tryLoadIndex(repoDir); err == nil {
		return idx, repoDir, nil
	}
	return nil, "", fmt.Errorf("no valid semantic index found (checked %s and %s)", userDir, repoDir)
}

func tryLoadIndex(dir string) (*searchindex.Index, error) {
	idx, err := searchindex.Load(dir)
	if err != nil {
		return nil, err
	}
	return idx, nil
}

func printSearchResults(query string, results []search.SearchResult) {
	fmt.Printf("\naxon search %q\n\n", query)
	fmt.Printf("Results (%d found):\n", len(results))
	if len(results) == 0 {
		return
	}

	grouped := make(map[string][]search.SearchResult)
	orderSeen := make(map[string]struct{})
	groupOrder := make([]string, 0, 8)
	for _, r := range results {
		root := r.Skill.Path
		if root == "" {
			root = "(unknown)"
		} else {
			if i := strings.IndexByte(root, '/'); i >= 0 {
				root = root[:i]
			}
		}
		if _, ok := orderSeen[root]; !ok {
			orderSeen[root] = struct{}{}
			groupOrder = append(groupOrder, root)
		}
		grouped[root] = append(grouped[root], r)
	}

	priority := map[string]int{"skills": 0, "workflows": 1, "commands": 2}
	sort.SliceStable(groupOrder, func(i, j int) bool {
		pi, okI := priority[groupOrder[i]]
		pj, okJ := priority[groupOrder[j]]
		if okI && okJ {
			return pi < pj
		}
		if okI {
			return true
		}
		if okJ {
			return false
		}
		return groupOrder[i] < groupOrder[j]
	})

	for _, g := range groupOrder {
		items := grouped[g]
		fmt.Printf("\n%s (%d):\n", g, len(items))

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for i, r := range items {
			displayID := r.Skill.ID
			if g != "skills" {
				prefix := g + ":"
				displayID = strings.TrimPrefix(displayID, prefix)
				displayID = strings.ReplaceAll(displayID, ":", "/")
			}

			score := ""
			if r.Why == "semantic" {
				score = fmt.Sprintf("[%.3f]", r.Score)
			}

			fmt.Fprintf(w, "  %d.\t%s\t%s\n", i+1, score, displayID)
			fmt.Fprintf(w, "  - %s\n", strings.TrimSpace(r.Skill.Description))
		}
		_ = w.Flush()
	}
}

func runSearchIndex(cmd *cobra.Command, cfg *config.Config) error {
	_ = cmd

	// We require embeddings config for indexing.
	embCfg, err := embeddings.LoadConfig()
	if err != nil {
		return err
	}
	prov, err := embeddings.NewFromConfig(embCfg)
	if err != nil {
		return err
	}
	if prov.ModelID() == "" {
		return errors.New("embeddings provider is not configured")
	}

	axonDir, err := config.AxonDir()
	if err != nil {
		return err
	}
	userDir := filepath.Join(axonDir, "search")
	tmpBase := filepath.Join(axonDir, "tmp")
	if err := os.MkdirAll(tmpBase, 0o755); err != nil {
		return fmt.Errorf("cannot create temp dir %s: %w", tmpBase, err)
	}
	tmpDir, err := os.MkdirTemp(tmpBase, "search-index-*")
	if err != nil {
		return fmt.Errorf("cannot create temp index dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	printInfo("", fmt.Sprintf("building semantic index using %s", prov.ModelID()))
	_, err = searchindex.BuildUserIndex(ctx, prov, searchindex.BuildOptions{
		RepoPath:  cfg.RepoPath,
		OutDir:    tmpDir,
		Roots:     cfg.EffectiveSearchRoots(),
		Force:     flagSearchForce,
		Normalize: true,
	})
	if err != nil {
		return fmt.Errorf("index build failed: %w", err)
	}

	if err := searchindex.AtomicSwap(tmpDir, userDir); err != nil {
		return fmt.Errorf("cannot install index: %w", err)
	}
	printOK("", fmt.Sprintf("semantic index written: %s", userDir))
	return nil
}
