package openai

import (
	"strings"
	"testing"
)

func TestParseResponsesRequestPreservesExtendedFields(t *testing.T) {
	t.Parallel()

	req, err := ParseResponsesRequest(strings.NewReader(`{
		"model":"gpt-5.4",
		"stream":true,
		"store":true,
		"input":"ping",
		"instructions":"be precise",
		"text":{"format":{"type":"json_object"}},
		"tools":[{"type":"function","name":"grep","description":"search files","parameters":{"type":"object"}}],
		"tool_choice":{"type":"function","name":"grep"},
		"parallel_tool_calls":true,
		"reasoning":{"effort":"medium"},
		"include":["reasoning.encrypted_content"],
		"metadata":{"session":"abc"},
		"max_output_tokens":2048
	}`))
	if err != nil {
		t.Fatalf("ParseResponsesRequest returned error: %v", err)
	}

	if string(req.Tools) == "" || string(req.Tools) == "null" {
		t.Fatal("Tools is empty, want preserved tools payload")
	}
	if req.Store == nil || !*req.Store {
		t.Fatalf("Store = %#v, want true", req.Store)
	}
	if string(req.Text) == "" || string(req.Text) == "null" {
		t.Fatal("Text is empty, want preserved text payload")
	}
	if string(req.ToolChoice) == "" || string(req.ToolChoice) == "null" {
		t.Fatal("ToolChoice is empty, want preserved tool_choice payload")
	}
	if req.ParallelToolCalls == nil || !*req.ParallelToolCalls {
		t.Fatalf("ParallelToolCalls = %#v, want true", req.ParallelToolCalls)
	}
	if string(req.Reasoning) == "" || string(req.Reasoning) == "null" {
		t.Fatal("Reasoning is empty, want preserved reasoning payload")
	}
	if string(req.Include) == "" || string(req.Include) == "null" {
		t.Fatal("Include is empty, want preserved include payload")
	}
	if string(req.Metadata) == "" || string(req.Metadata) == "null" {
		t.Fatal("Metadata is empty, want preserved metadata payload")
	}
	if req.MaxOutputTokens == nil || *req.MaxOutputTokens != 2048 {
		t.Fatalf("MaxOutputTokens = %#v, want 2048", req.MaxOutputTokens)
	}
}
