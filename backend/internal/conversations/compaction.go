package conversations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	messageStorageModeFull    = "full"
	messageStorageModeSummary = "summary"
	messagePreviewLimit       = 512
)

func compactMessageRecord(message Message, toSummary bool) Message {
	message.ContentPreview = previewText(message.Content, messagePreviewLimit)
	message.ContentBytes = len([]byte(message.Content))
	message.ContentSHA256 = sha256Hex(message.Content)
	message.RawPreview = previewText(message.RawItemJSON, messagePreviewLimit)
	message.RawBytes = len([]byte(message.RawItemJSON))
	message.RawSHA256 = sha256Hex(message.RawItemJSON)

	meta := summarizeRawPayload(message.ItemType, message.RawItemJSON)
	if message.ToolName == "" {
		message.ToolName = meta.ToolName
	}
	if message.ToolCallID == "" {
		message.ToolCallID = meta.ToolCallID
	}
	message.SummaryJSON = buildSummaryJSON(message, meta)
	if toSummary {
		message.StorageMode = messageStorageModeSummary
		message.Content = message.ContentPreview
		message.RawItemJSON = ""
	} else if strings.TrimSpace(message.StorageMode) == "" {
		message.StorageMode = messageStorageModeFull
	}
	return message
}

func previewText(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if limit <= 0 || len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit] + "..."
}

func sha256Hex(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type compactSummaryMeta struct {
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Type       string `json:"type,omitempty"`
}

func summarizeRawPayload(itemType string, raw string) compactSummaryMeta {
	meta := compactSummaryMeta{Type: itemType}
	if strings.TrimSpace(raw) == "" {
		return meta
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return meta
	}
	if toolName, _ := payload["name"].(string); toolName != "" {
		meta.ToolName = toolName
	}
	if toolName, _ := payload["tool_name"].(string); toolName != "" && meta.ToolName == "" {
		meta.ToolName = toolName
	}
	if callID, _ := payload["call_id"].(string); callID != "" {
		meta.ToolCallID = callID
	}
	if itemTypeValue, _ := payload["type"].(string); itemTypeValue != "" {
		meta.Type = itemTypeValue
	}
	return meta
}

func buildSummaryJSON(message Message, meta compactSummaryMeta) string {
	payload := map[string]any{
		"item_type":       message.ItemType,
		"role":            message.Role,
		"content_preview": message.ContentPreview,
		"content_bytes":   message.ContentBytes,
		"raw_preview":     message.RawPreview,
		"raw_bytes":       message.RawBytes,
	}
	if meta.Type != "" {
		payload["raw_type"] = meta.Type
	}
	if meta.ToolName != "" {
		payload["tool_name"] = meta.ToolName
	}
	if meta.ToolCallID != "" {
		payload["tool_call_id"] = meta.ToolCallID
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(raw)
}
