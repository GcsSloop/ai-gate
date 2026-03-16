package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Stream   bool      `json:"stream"`
	Messages []Message `json:"messages"`
}

func ParseChatCompletionRequest(reader io.Reader) (ChatCompletionRequest, error) {
	var req ChatCompletionRequest
	if err := json.NewDecoder(reader).Decode(&req); err != nil {
		return ChatCompletionRequest{}, fmt.Errorf("decode chat completion request: %w", err)
	}
	if req.Model == "" {
		return ChatCompletionRequest{}, fmt.Errorf("model is required")
	}
	return req, nil
}

func ExtractMessageText(raw json.RawMessage) []string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}

	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		plain = strings.TrimSpace(plain)
		if plain == "" {
			return nil
		}
		return []string{plain}
	}

	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err == nil {
		texts := make([]string, 0, len(items))
		for _, item := range items {
			text, _ := item["text"].(string)
			text = strings.TrimSpace(text)
			if text != "" {
				texts = append(texts, text)
			}
		}
		return texts
	}

	return nil
}
