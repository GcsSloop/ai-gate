package api

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	gatewayopenai "github.com/gcssloop/codex-router/backend/internal/gateway/openai"
)

const requestLogPreviewLimit = 120

func summarizeResponsesRequestLog(rawInput json.RawMessage, rawTools json.RawMessage, previousResponseID string, stream bool) string {
	items, err := gatewayopenai.ExtractResponsesInputItems(rawInput)
	if err != nil {
		return fmt.Sprintf("items=0 roles=unknown chars=0 preview=%q tools=%s prev=%t stream=%t", "", summarizeTools(rawTools), strings.TrimSpace(previousResponseID) != "", stream)
	}
	roles := make([]string, 0, len(items))
	textParts := make([]string, 0, len(items))
	totalChars := 0
	for _, item := range items {
		role := strings.TrimSpace(item.Role)
		if role == "" {
			role = "user"
		}
		roles = append(roles, role)
		text := strings.TrimSpace(item.Content)
		if text != "" {
			textParts = append(textParts, text)
			totalChars += len([]rune(text))
		}
	}
	return fmt.Sprintf(
		"stream=%t items=%d roles=%s chars=%d preview=%q tools=%s prev=%t",
		stream,
		len(items),
		joinOrFallback(roles, "none"),
		totalChars,
		previewText(strings.Join(textParts, " | ")),
		summarizeTools(rawTools),
		strings.TrimSpace(previousResponseID) != "",
	)
}

func summarizeChatRequestLog(messages []gatewayopenai.Message, stream bool) string {
	roles := make([]string, 0, len(messages))
	textParts := make([]string, 0, len(messages))
	totalChars := 0
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		roles = append(roles, role)
		content := strings.TrimSpace(message.Content)
		if content != "" {
			textParts = append(textParts, content)
			totalChars += len([]rune(content))
		}
	}
	return fmt.Sprintf(
		"stream=%t items=%d roles=%s chars=%d preview=%q",
		stream,
		len(messages),
		joinOrFallback(roles, "none"),
		totalChars,
		previewText(strings.Join(textParts, " | ")),
	)
}

func logRequestSummary(kind string, path string, method string, model string, remote string, summary string) {
	log.Printf("%s request path=%s method=%s %s model_req=%s remote=%q", kind, path, method, summary, model, remote)
}

func logUpstreamSummary(kind string, conversationID int64, account accounts.Account, endpoint string, model string) {
	log.Printf(
		"%s upstream conversation_id=%d account_id=%d account=%s provider=%s auth_mode=%s base_url=%s endpoint=%s model_upstream=%s",
		kind,
		conversationID,
		account.ID,
		account.AccountName,
		string(account.ProviderType),
		account.AuthMode,
		resolveAccountBaseURL(account),
		endpoint,
		model,
	)
}

func logResultSummary(kind string, conversationID int64, accountID int64, status int, startedAt time.Time, output string) {
	log.Printf(
		"%s result conversation_id=%d account_id=%d status=%d duration_ms=%d output_preview=%q",
		kind,
		conversationID,
		accountID,
		status,
		time.Since(startedAt).Milliseconds(),
		previewText(output),
	)
}

func logFailureSummary(kind string, conversationID int64, accountID int64, stage string, startedAt time.Time, err error) {
	log.Printf(
		"%s failure conversation_id=%d account_id=%d stage=%s duration_ms=%d err=%q",
		kind,
		conversationID,
		accountID,
		stage,
		time.Since(startedAt).Milliseconds(),
		err,
	)
}

func logThinGatewayCandidate(account accounts.Account, action string, reason string) {
	log.Printf(
		"responses candidate account_id=%d account=%s active=%t supports_responses=%t provider=%s action=%s reason=%s",
		account.ID,
		account.AccountName,
		account.IsActive,
		account.NativeResponsesCapable(),
		string(account.ProviderType),
		action,
		reason,
	)
}

func summarizeTools(raw json.RawMessage) string {
	decoded, ok := decodeRawJSON(raw)
	if !ok {
		return "none"
	}
	items, ok := decoded.([]any)
	if !ok || len(items) == 0 {
		return "none"
	}
	types := make([]string, 0, len(items))
	for _, item := range items {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		toolType, _ := tool["type"].(string)
		if strings.TrimSpace(toolType) != "" {
			types = append(types, toolType)
		}
	}
	if len(types) == 0 {
		return "present"
	}
	return strings.Join(types, ",")
}

func joinOrFallback(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	return strings.Join(items, ",")
}

func previewText(text string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if normalized == "" {
		return ""
	}
	runes := []rune(normalized)
	if len(runes) <= requestLogPreviewLimit {
		return normalized
	}
	return string(runes[:requestLogPreviewLimit]) + "..."
}
