package index

// Manifest describes a semantic index and how to interpret it.
type Manifest struct {
	IndexVersion int    `json:"index_version"`
	CreatedAt    string `json:"created_at"`
	HubRevision  string `json:"hub_revision"`
	ModelID      string `json:"model_id"`
	Dim          int    `json:"dim"`
	Normalize    bool   `json:"normalize"`
	VectorFile   string `json:"vector_file"`
	SkillsFile   string `json:"skills_file"`
}

// SkillEntry represents one skill row in skills.jsonl.
type SkillEntry struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Name        string `json:"name"`
	Description string `json:"description"`
	TextHash    string `json:"text_hash"`
	UpdatedAt   string `json:"updated_at"`
}

// Index is a loaded semantic index.
type Index struct {
	Manifest Manifest
	Skills   []SkillEntry
	Vectors  []float32
}
