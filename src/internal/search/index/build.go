package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kamusis/axon-cli/internal/embeddings"
	"github.com/kamusis/axon-cli/internal/search"
)

// BuildOptions controls user index building.
type BuildOptions struct {
	RepoPath  string
	OutDir    string
	Roots     []string
	Force     bool
	Normalize bool
}

// BuildUserIndex builds a semantic index from skills found in repoPath and writes it to outDir.
//
// The build is incremental when an existing index is present in outDir (unless Force is true).
// It is the caller's responsibility to apply an atomic swap strategy.
func BuildUserIndex(ctx context.Context, prov embeddings.Provider, opts BuildOptions) (*Index, error) {
	if opts.RepoPath == "" {
		return nil, fmt.Errorf("repo path is required")
	}
	if opts.OutDir == "" {
		return nil, fmt.Errorf("out dir is required")
	}

	skills, err := search.DiscoverDocuments(opts.RepoPath, opts.Roots)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("no documents found under %s", opts.RepoPath)
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].ID < skills[j].ID })

	// Load existing index for reuse.
	old, _ := Load(opts.OutDir)
	reuse := map[string]SkillEntry{}
	reuseVec := map[string][]float32{}
	if old != nil && !opts.Force {
		for i, se := range old.Skills {
			start := i * old.Manifest.Dim
			end := start + old.Manifest.Dim
			if start >= 0 && end <= len(old.Vectors) {
				reuse[se.ID] = se
				v := make([]float32, old.Manifest.Dim)
				copy(v, old.Vectors[start:end])
				reuseVec[se.ID] = v
			}
		}
	}

	var (
		entries []SkillEntry
		vectors []float32
		dim     int
	)

	for _, s := range skills {
		text := CanonicalText(s)
		h := TextHash(text)

		if old != nil && !opts.Force {
			if prev, ok := reuse[s.ID]; ok {
				if prev.TextHash == h && prev.TextHash != "" {
					if v, ok := reuseVec[s.ID]; ok {
						entries = append(entries, prev)
						vectors = append(vectors, v...)
						if dim == 0 {
							dim = len(v)
						}
						continue
					}
				}
			}
		}

		emb, err := prov.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		if dim == 0 {
			dim = len(emb)
		}
		if len(emb) != dim {
			return nil, fmt.Errorf("embedding dim changed mid-run: got %d want %d", len(emb), dim)
		}
		if opts.Normalize {
			emb = NormalizeL2(emb)
		}

		entries = append(entries, SkillToEntry(s, h))
		vectors = append(vectors, emb...)
	}

	manifest := Manifest{
		IndexVersion: 1,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		HubRevision:  "",
		ModelID:      prov.ModelID(),
		Dim:          dim,
		Normalize:    opts.Normalize,
		VectorFile:   "vectors.f32",
		SkillsFile:   "skills.jsonl",
	}

	idx := &Index{Manifest: manifest, Skills: entries, Vectors: vectors}

	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create out dir: %w", err)
	}
	if err := Write(opts.OutDir, manifest, entries, vectors); err != nil {
		return nil, err
	}

	return idx, nil
}

// AtomicSwap replaces destDir with srcDir by renaming.
func AtomicSwap(srcDir, destDir string) error {
	parent := filepath.Dir(destDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	backup := destDir + ".bak"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(destDir); err == nil {
		if err := os.Rename(destDir, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(srcDir, destDir); err != nil {
		// rollback best-effort
		if _, stErr := os.Stat(backup); stErr == nil {
			_ = os.Rename(backup, destDir)
		}
		return err
	}
	_ = os.RemoveAll(backup)
	return nil
}
