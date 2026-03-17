package llm

import "context"

// Message represents a chat message with role and content.
type Message struct {
	Role    string // "system", "user", or "assistant"
	Content string
}

// Response represents the LLM's response.
type Response struct {
	Content string
	Model   string
}

// Provider defines the interface for LLM providers (OpenAI, Anthropic, Ollama, etc.).
type Provider interface {
	// Chat sends messages to the LLM and returns the response.
	Chat(ctx context.Context, messages []Message) (*Response, error)

	// Name returns the provider name (e.g., "openai", "anthropic").
	Name() string
}
