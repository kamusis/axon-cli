package search

import (
	"strings"

	"gopkg.in/yaml.v3"
)

func splitFrontmatter(content string) (map[string]string, string) {
	s := strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(s, "---") {
		return map[string]string{}, content
	}

	parts := strings.SplitN(s, "---", 3)
	if len(parts) < 3 {
		return map[string]string{}, content
	}

	fmText := strings.TrimSpace(parts[1])
	body := strings.TrimPrefix(parts[2], "\n")

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(fmText), &raw); err != nil {
		return map[string]string{}, content
	}

	out := make(map[string]string)
	for k, v := range raw {
		if sv, ok := v.(string); ok {
			out[strings.ToLower(k)] = sv
		}
	}
	return out, body
}
