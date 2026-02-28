package index

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kamusis/axon-cli/internal/search"
)

// Write writes index artifacts to dir.
func Write(dir string, manifest Manifest, skills []SkillEntry, vectors []float32) error {
	if manifest.Dim <= 0 {
		return fmt.Errorf("invalid dim: %d", manifest.Dim)
	}
	if len(skills) == 0 {
		return fmt.Errorf("no skills to write")
	}
	if len(vectors) != len(skills)*manifest.Dim {
		return fmt.Errorf("vector length mismatch: got %d want %d", len(vectors), len(skills)*manifest.Dim)
	}
	if manifest.VectorFile == "" {
		manifest.VectorFile = "vectors.f32"
	}
	if manifest.SkillsFile == "" {
		manifest.SkillsFile = "skills.jsonl"
	}
	if manifest.CreatedAt == "" {
		manifest.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create index dir %s: %w", dir, err)
	}

	// manifest
	mb, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "index_manifest.json"), mb, 0o644); err != nil {
		return fmt.Errorf("cannot write manifest: %w", err)
	}

	// skills jsonl
	sf, err := os.Create(filepath.Join(dir, manifest.SkillsFile))
	if err != nil {
		return fmt.Errorf("cannot create skills file: %w", err)
	}
	bw := bufio.NewWriter(sf)
	for _, s := range skills {
		line, err := json.Marshal(s)
		if err != nil {
			_ = sf.Close()
			return err
		}
		if _, err := bw.Write(line); err != nil {
			_ = sf.Close()
			return err
		}
		if err := bw.WriteByte('\n'); err != nil {
			_ = sf.Close()
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		_ = sf.Close()
		return err
	}
	if err := sf.Close(); err != nil {
		return err
	}

	// vectors
	vf, err := os.Create(filepath.Join(dir, manifest.VectorFile))
	if err != nil {
		return fmt.Errorf("cannot create vectors file: %w", err)
	}
	if err := binary.Write(vf, binary.LittleEndian, vectors); err != nil {
		_ = vf.Close()
		return fmt.Errorf("cannot write vectors: %w", err)
	}
	if err := vf.Close(); err != nil {
		return err
	}

	return nil
}

// SkillToEntry converts SkillDoc to SkillEntry for index writing.
func SkillToEntry(s search.SkillDoc, textHash string) SkillEntry {
	return SkillEntry{
		ID:          s.ID,
		Path:        s.Path,
		Name:        s.Name,
		Description: s.Description,
		TextHash:    textHash,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
}
