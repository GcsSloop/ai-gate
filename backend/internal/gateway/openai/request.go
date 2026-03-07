package openai

import (
	"encoding/json"
	"fmt"
	"io"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
