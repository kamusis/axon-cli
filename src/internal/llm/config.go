package llm

import (
	"fmt"

	"github.com/kamusis/axon-cli/internal/config"
)

// LoadProviderFromConfig loads an LLM provider from environment/config.
// Returns nil if not configured (graceful fallback).
func LoadProviderFromConfig() (Provider, error) {
	provider, _ := config.GetConfigValue("AXON_AUDIT_PROVIDER")
	if provider == "" {
		return nil, nil // Not configured, graceful fallback
	}

	apiKey, _ := config.GetConfigValue("AXON_AUDIT_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("AXON_AUDIT_API_KEY is required when AXON_AUDIT_PROVIDER is set")
	}

	model, _ := config.GetConfigValue("AXON_AUDIT_MODEL")
	baseURL, _ := config.GetConfigValue("AXON_AUDIT_BASE_URL")

	switch provider {
	case "openai":
		return NewOpenAIProvider(apiKey, baseURL, model), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s (supported: openai)", provider)
	}
}
