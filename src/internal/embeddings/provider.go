package embeddings

import (
	"context"
	"fmt"

	"github.com/kamusis/axon-cli/internal/config"
)

// Provider embeds text into a fixed-length float vector.
//
// Implementations must be deterministic for the same input text and model.
type Provider interface {
	ModelID() string
	Dim() int
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Config contains the resolved embeddings configuration.
type Config struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
}

// LoadConfig resolves embeddings config from environment variables first, then ~/.axon/.env.
func LoadConfig() (*Config, error) {
	provider, err := config.GetConfigValue("AXON_EMBEDDINGS_PROVIDER")
	if err != nil {
		return nil, err
	}
	model, err := config.GetConfigValue("AXON_EMBEDDINGS_MODEL")
	if err != nil {
		return nil, err
	}
	apiKey, err := config.GetConfigValue("AXON_EMBEDDINGS_API_KEY")
	if err != nil {
		return nil, err
	}
	baseURL, err := config.GetConfigValue("AXON_EMBEDDINGS_BASE_URL")
	if err != nil {
		return nil, err
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return &Config{
		Provider: provider,
		Model:    model,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	}, nil
}

// NewFromConfig returns an embeddings provider.
func NewFromConfig(cfg *Config) (Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("embeddings config is nil")
	}
	if cfg.Provider == "" {
		return nil, fmt.Errorf("embeddings provider is not configured (set AXON_EMBEDDINGS_PROVIDER)")
	}
	switch cfg.Provider {
	case "openai":
		return NewOpenAI(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported embeddings provider: %s", cfg.Provider)
	}
}
