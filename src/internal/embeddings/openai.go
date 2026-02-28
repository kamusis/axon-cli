package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type openAIProvider struct {
	model   string
	apiKey  string
	baseURL string
	client  *http.Client
	dim     int
}

// NewOpenAI constructs an OpenAI-compatible embeddings provider.
//
// It uses the REST endpoint:
//   POST {baseURL}/embeddings
// with JSON body:
//   {"model": "...", "input": "..."}
func NewOpenAI(cfg *Config) Provider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	return &openAIProvider{
		model:   cfg.Model,
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
		dim:     0,
	}
}

func (p *openAIProvider) ModelID() string {
	return "openai:" + p.model
}

func (p *openAIProvider) Dim() int {
	return p.dim
}

func (p *openAIProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if p.model == "" {
		return nil, fmt.Errorf("embeddings model is not configured (set AXON_EMBEDDINGS_MODEL)")
	}
	if p.apiKey == "" {
		return nil, fmt.Errorf("embeddings API key is not configured (set AXON_EMBEDDINGS_API_KEY)")
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("cannot embed empty text")
	}

	reqBody := map[string]any{
		"model": p.model,
		"input": text,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embeddings request failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("cannot parse embeddings response: %w", err)
	}
	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embeddings response missing embedding")
	}

	emb64 := parsed.Data[0].Embedding
	out := make([]float32, len(emb64))
	for i, v := range emb64 {
		out[i] = float32(v)
	}
	p.dim = len(out)
	return out, nil
}
