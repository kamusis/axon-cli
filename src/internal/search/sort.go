package search

import "sort"

// SortResults sorts results by score (descending), then by skill ID (ascending).
func SortResults(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Skill.ID < results[j].Skill.ID
		}
		return results[i].Score > results[j].Score
	})
}
