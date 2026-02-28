package index

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/kamusis/axon-cli/internal/search"
)

// CanonicalText returns the canonical text used for embeddings generation.
func CanonicalText(s search.SkillDoc) string {
	parts := []string{
		"name: " + strings.TrimSpace(s.Name),
		"description: " + strings.TrimSpace(s.Description),
	}
	if strings.TrimSpace(s.Keywords) != "" {
		parts = append(parts, "keywords: "+strings.TrimSpace(s.Keywords))
	}
	return strings.Join(parts, "\n")
}

// TextHash returns a sha256 hash (hex) of the canonical text.
func TextHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}
