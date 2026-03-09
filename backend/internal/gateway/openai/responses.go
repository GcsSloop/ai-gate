package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type ResponsesRequest struct {
	Model              string          `json:"model"`
	Stream             bool            `json:"stream"`
	Store              *bool           `json:"store"`
	Input              json.RawMessage `json:"input"`
	PreviousResponseID string          `json:"previous_response_id"`
	Instructions       string          `json:"instructions"`
	Text               json.RawMessage `json:"text"`
	Tools              json.RawMessage `json:"tools"`
	ToolChoice         json.RawMessage `json:"tool_choice"`
	ParallelToolCalls  *bool           `json:"parallel_tool_calls"`
	Reasoning          json.RawMessage `json:"reasoning"`
	Include            json.RawMessage `json:"include"`
	Metadata           json.RawMessage `json:"metadata"`
	MaxOutputTokens    *int            `json:"max_output_tokens"`
}

type ResponsesInputItem struct {
	Role    string
	Content string
	Raw     map[string]any
}

func ParseResponsesRequest(reader io.Reader) (ResponsesRequest, error) {
	var req ResponsesRequest
	if err := json.NewDecoder(reader).Decode(&req); err != nil {
		return ResponsesRequest{}, fmt.Errorf("decode responses request: %w", err)
	}
	if strings.TrimSpace(req.Model) == "" {
		return ResponsesRequest{}, fmt.Errorf("model is required")
	}
	return req, nil
}

func ExtractResponsesInputItems(raw json.RawMessage) ([]ResponsesInputItem, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("decode responses string input: %w", err)
		}
		return []ResponsesInputItem{{Role: "user", Content: text}}, nil
	}

	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("decode responses input array: %w", err)
	}

	result := make([]ResponsesInputItem, 0, len(items))
	for _, item := range items {
		role, _ := item["role"].(string)
		if role == "" {
			role = "user"
		}
		text := extractResponsesText(item)
		rawItem := cloneMap(item)
		if strings.TrimSpace(text) == "" && !hasPreservableResponsesItem(item) {
			continue
		}
		result = append(result, ResponsesInputItem{
			Role:    role,
			Content: text,
			Raw:     rawItem,
		})
	}
	return result, nil
}

func extractResponsesText(item map[string]any) string {
	if text, ok := item["text"].(string); ok {
		return text
	}
	if output, ok := item["output"].(string); ok {
		return output
	}
	content, ok := item["content"].([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(content))
	for _, rawPart := range content {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		switch value := part["text"].(type) {
		case string:
			parts = append(parts, value)
		case map[string]any:
			if text, ok := value["value"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func hasPreservableResponsesItem(item map[string]any) bool {
	itemType, _ := item["type"].(string)
	return strings.TrimSpace(itemType) != ""
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil
	}
	return cloned
}
