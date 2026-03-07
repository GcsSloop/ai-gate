package openai

import (
	"encoding/json"
	"testing"
)

func TestExtractResponsesInputItemsPreservesFunctionCallOutput(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`[
		{"type":"function_call_output","call_id":"call_123","output":"done"},
		{"role":"user","content":[{"type":"input_text","text":"hello"}]}
	]`)

	items, err := ExtractResponsesInputItems(raw)
	if err != nil {
		t.Fatalf("ExtractResponsesInputItems returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Raw["type"] != "function_call_output" {
		t.Fatalf("first raw type = %v, want function_call_output", items[0].Raw["type"])
	}
	if items[0].Content != "done" {
		t.Fatalf("first content = %q, want done", items[0].Content)
	}
	if items[1].Role != "user" || items[1].Content != "hello" {
		t.Fatalf("second item = %+v, want user hello", items[1])
	}
}
