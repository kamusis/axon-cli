package index

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_IndexHappyPath(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{
		IndexVersion: 1,
		CreatedAt:    "2026-01-01T00:00:00Z",
		HubRevision:  "rev",
		ModelID:      "openai:test",
		Dim:          2,
		Normalize:    true,
		VectorFile:   "vectors.f32",
		SkillsFile:   "skills.jsonl",
	}
	mb, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "index_manifest.json"), mb, 0o644); err != nil {
		t.Fatal(err)
	}

	skills := []SkillEntry{
		{ID: "a", Path: "skills/a", Name: "a", Description: "A"},
		{ID: "b", Path: "skills/b", Name: "b", Description: "B"},
	}
	var skillsLines []byte
	for _, s := range skills {
		b, _ := json.Marshal(s)
		skillsLines = append(skillsLines, b...)
		skillsLines = append(skillsLines, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "skills.jsonl"), skillsLines, 0o644); err != nil {
		t.Fatal(err)
	}

	vecFile, err := os.Create(filepath.Join(dir, "vectors.f32"))
	if err != nil {
		t.Fatal(err)
	}
	vectors := []float32{1, 0, 0, 1}
	if err := binary.Write(vecFile, binary.LittleEndian, vectors); err != nil {
		_ = vecFile.Close()
		t.Fatal(err)
	}
	_ = vecFile.Close()

	idx, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if idx.Manifest.Dim != 2 {
		t.Fatalf("dim mismatch")
	}
	if len(idx.Skills) != 2 {
		t.Fatalf("skills mismatch")
	}
	if len(idx.Vectors) != 4 {
		t.Fatalf("vectors mismatch")
	}
}
