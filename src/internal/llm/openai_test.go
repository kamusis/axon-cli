package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIProvider_Chat(t *testing.T) {
	// Mock OpenAI API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header with Bearer token")
		}

		// Parse request body
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Verify model
		if reqBody["model"] != "gpt-4o-mini" {
			t.Errorf("expected model gpt-4o-mini, got %v", reqBody["model"])
		}

		// Send mock response
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "Test response",
					},
				},
			},
			"model": "gpt-4o-mini",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create provider with mock server
	provider := NewOpenAIProvider("test-key", server.URL, "gpt-4o-mini")

	// Test Chat
	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	resp, err := provider.Chat(context.Background(), messages)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if resp.Content != "Test response" {
		t.Errorf("expected 'Test response', got %q", resp.Content)
	}
	if resp.Model != "gpt-4o-mini" {
		t.Errorf("expected model 'gpt-4o-mini', got %q", resp.Model)
	}
}

func TestOpenAIProvider_Name(t *testing.T) {
	provider := NewOpenAIProvider("key", "", "")
	if provider.Name() != "openai" {
		t.Errorf("expected name 'openai', got %q", provider.Name())
	}
}

func TestOpenAIProvider_Defaults(t *testing.T) {
	provider := NewOpenAIProvider("key", "", "")
	if provider.baseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default baseURL, got %q", provider.baseURL)
	}
	if provider.model != "gpt-4o-mini" {
		t.Errorf("expected default model gpt-4o-mini, got %q", provider.model)
	}
}
