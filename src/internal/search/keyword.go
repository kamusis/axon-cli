package search

import (
	"sort"
	"strings"
)

// KeywordSearch searches skills by case-insensitive keyword matching over name, description,
// and keywords. All query tokens must match (AND semantics).
func KeywordSearch(skills []SkillDoc, query string, limit int) []SearchResult {
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return []SearchResult{}
	}

	var out []SearchResult
	for _, s := range skills {
		blob := strings.ToLower(strings.Join([]string{s.ID, s.Name, s.Description, s.Keywords}, "\n"))
		ok := true
		for _, tok := range tokens {
			if !strings.Contains(blob, tok) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		out = append(out, SearchResult{Skill: s, Score: 1, Why: "keyword"})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Skill.ID < out[j].Skill.ID
	})

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func tokenize(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	parts := strings.Fields(q)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
