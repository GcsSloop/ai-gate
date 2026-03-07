package api

import (
	"net/http"

	gatewayopenai "github.com/gcssloop/codex-router/backend/internal/gateway/openai"
)

type GatewayHandler struct{}

func NewGatewayHandler() *GatewayHandler {
	return &GatewayHandler{}
}

func (h *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}

	req, err := gatewayopenai.ParseChatCompletionRequest(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object":  "chat.completion",
		"model":   req.Model,
		"stream":  req.Stream,
		"message_count": len(req.Messages),
	})
}
