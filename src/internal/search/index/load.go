package index

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Load reads an index from dir containing manifest + skills + vectors.
func Load(dir string) (*Index, error) {
	manifestPath := filepath.Join(dir, "index_manifest.json")
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read manifest %s: %w", manifestPath, err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("invalid manifest JSON %s: %w", manifestPath, err)
	}
	if m.Dim <= 0 {
		return nil, fmt.Errorf("invalid dim in manifest: %d", m.Dim)
	}
	if m.VectorFile == "" {
		m.VectorFile = "vectors.f32"
	}
	if m.SkillsFile == "" {
		m.SkillsFile = "skills.jsonl"
	}

	skills, err := loadSkills(filepath.Join(dir, m.SkillsFile))
	if err != nil {
		return nil, err
	}
	vectors, err := loadVectors(filepath.Join(dir, m.VectorFile), len(skills), m.Dim)
	if err != nil {
		return nil, err
	}

	idx := &Index{Manifest: m, Skills: skills, Vectors: vectors}
	return idx, nil
}

func loadSkills(path string) ([]SkillEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open skills file %s: %w", path, err)
	}
	defer f.Close()

	var out []SkillEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e SkillEntry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("invalid skills JSONL %s: %w", path, err)
		}
		out = append(out, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cannot read skills file %s: %w", path, err)
	}
	return out, nil
}

func loadVectors(path string, nSkills, dim int) ([]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open vector file %s: %w", path, err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot stat vector file %s: %w", path, err)
	}
	if st.Size()%4 != 0 {
		return nil, fmt.Errorf("vector file size is not multiple of 4 bytes: %d", st.Size())
	}

	expected := int64(nSkills * dim * 4)
	if expected != st.Size() {
		return nil, fmt.Errorf("vector file size mismatch: got %d want %d (skills=%d dim=%d)", st.Size(), expected, nSkills, dim)
	}

	nFloats := nSkills * dim
	out := make([]float32, nFloats)

	if err := binary.Read(io.LimitReader(f, expected), binary.LittleEndian, out); err != nil {
		return nil, fmt.Errorf("cannot read vectors from %s: %w", path, err)
	}
	return out, nil
}
