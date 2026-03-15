package llm

import (
	"testing"
)

func TestConvertMessages(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Hello"},
	}

	converted := convertMessages(messages)

	if len(converted) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(converted))
	}

	if converted[0]["role"] != "system" {
		t.Errorf("expected role 'system', got %q", converted[0]["role"])
	}
	if converted[0]["content"] != "You are a helpful assistant" {
		t.Errorf("unexpected content: %q", converted[0]["content"])
	}

	if converted[1]["role"] != "user" {
		t.Errorf("expected role 'user', got %q", converted[1]["role"])
	}
	if converted[1]["content"] != "Hello" {
		t.Errorf("unexpected content: %q", converted[1]["content"])
	}
}
