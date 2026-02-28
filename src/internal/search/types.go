package search

// SkillDoc represents the minimal searchable metadata for a skill.
type SkillDoc struct {
	ID          string
	Path        string
	Name        string
	Description string
	Keywords    string
}

// SearchResult represents one matched skill.
type SearchResult struct {
	Skill SkillDoc
	Score float64
	Why   string
}
