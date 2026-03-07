package api

import (
	"encoding/json"
	"math"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type usageSaver interface {
	Save(snapshot usage.Snapshot) error
}

type responsesUsageCollector struct {
	snapshot usage.Snapshot
	hasData  bool
	outputs  []map[string]any
}

func newResponsesUsageCollector(accountID int64) *responsesUsageCollector {
	return &responsesUsageCollector{
		snapshot: usage.Snapshot{
			AccountID:   accountID,
			CheckedAt:   time.Now().UTC(),
			HealthScore: 1,
		},
	}
}

func (c *responsesUsageCollector) Observe(frame map[string]any) {
	payload := unwrapResponsesFrame(frame)
	frameType, _ := payload["type"].(string)
	switch frameType {
	case "token_count":
		c.observeTokenCount(payload)
		c.applyPayloadRateLimits(payload)
	case "response.completed":
		c.observeCompletedResponse(payload)
		c.applyPayloadRateLimits(payload)
	}
}

func (c *responsesUsageCollector) Save(repo usageSaver) {
	if repo == nil || !c.hasData {
		return
	}
	if c.snapshot.CheckedAt.IsZero() {
		c.snapshot.CheckedAt = time.Now().UTC()
	}
	if c.snapshot.HealthScore == 0 {
		c.snapshot.HealthScore = 1
	}
	_ = repo.Save(c.snapshot)
}

func (c *responsesUsageCollector) snapshotOrDefault() usage.Snapshot {
	if !c.hasData {
		return emptyResponsesUsageSnapshot()
	}
	return c.snapshot
}

func (c *responsesUsageCollector) outputItems() []map[string]any {
	if len(c.outputs) == 0 {
		return nil
	}
	items := make([]map[string]any, 0, len(c.outputs))
	for _, item := range c.outputs {
		items = append(items, cloneJSONMap(item))
	}
	return items
}

func (c *responsesUsageCollector) outputText() string {
	return outputItemsText(c.outputs)
}

func (c *responsesUsageCollector) observeTokenCount(payload map[string]any) {
	c.hasData = true

	if info, ok := payload["info"].(map[string]any); ok {
		if total, ok := info["total_token_usage"].(map[string]any); ok {
			c.snapshot.LastTotalTokens = asFloat(total["total_tokens"])
			c.snapshot.LastInputTokens = asFloat(total["input_tokens"])
			c.snapshot.LastOutputTokens = asFloat(total["output_tokens"])
		}
		if contextWindow := asFloat(info["model_context_window"]); contextWindow > 0 {
			c.snapshot.ModelContextWindow = contextWindow
			if c.snapshot.LastTotalTokens > 0 {
				c.snapshot.QuotaRemaining = math.Max(contextWindow-c.snapshot.LastTotalTokens, 0)
			}
		}
	}
}

func (c *responsesUsageCollector) observeCompletedResponse(payload map[string]any) {
	response, ok := payload["response"].(map[string]any)
	if !ok {
		return
	}

	usagePayload, ok := response["usage"].(map[string]any)
	if ok {
		c.hasData = true
		c.snapshot.LastInputTokens = asFloat(usagePayload["input_tokens"])
		c.snapshot.LastOutputTokens = asFloat(usagePayload["output_tokens"])
		c.snapshot.LastTotalTokens = asFloat(usagePayload["total_tokens"])
	}

	if usageDetails, ok := usagePayload["input_tokens_details"].(map[string]any); ok {
		c.snapshot.LastInputTokens += asFloat(usageDetails["cached_tokens"])
	}

	if outputItems, ok := response["output"].([]any); ok && len(outputItems) > 0 {
		c.outputs = c.outputs[:0]
		for _, rawItem := range outputItems {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			c.outputs = append(c.outputs, cloneJSONMap(item))
		}
	}
}

func (c *responsesUsageCollector) applyPayloadRateLimits(payload map[string]any) {
	if limits, ok := payload["rate_limits"].(map[string]any); ok {
		c.applyRateLimits(limits)
		return
	}
	if response, ok := payload["response"].(map[string]any); ok {
		if limits, ok := response["rate_limits"].(map[string]any); ok {
			c.applyRateLimits(limits)
		}
	}
}

func (c *responsesUsageCollector) applyRateLimits(limits map[string]any) {
	if limits == nil {
		return
	}
	if credits, ok := limits["credits"].(map[string]any); ok {
		c.snapshot.Balance = asFloat(credits["balance"])
	}
	if primary, ok := limits["primary"].(map[string]any); ok {
		c.snapshot.PrimaryUsedPercent = asFloat(primary["used_percent"])
		c.snapshot.RPMRemaining = math.Max(100-c.snapshot.PrimaryUsedPercent, 0)
		c.snapshot.PrimaryResetsAt = asUnixTimePtr(primary["resets_at"])
	}
	if secondary, ok := limits["secondary"].(map[string]any); ok {
		c.snapshot.SecondaryUsedPercent = asFloat(secondary["used_percent"])
		c.snapshot.TPMRemaining = math.Max(100-c.snapshot.SecondaryUsedPercent, 0)
		c.snapshot.SecondaryResetsAt = asUnixTimePtr(secondary["resets_at"])
	}
	if c.snapshot.PrimaryUsedPercent > 0 || c.snapshot.SecondaryUsedPercent > 0 {
		primaryRemaining := math.Max(100-c.snapshot.PrimaryUsedPercent, 0)
		secondaryRemaining := math.Max(100-c.snapshot.SecondaryUsedPercent, 0)
		c.snapshot.HealthScore = (primaryRemaining + secondaryRemaining) / 200
		c.snapshot.ThrottledRecently = primaryRemaining < 10 || secondaryRemaining < 10
	}
}

func unwrapResponsesFrame(frame map[string]any) map[string]any {
	if payload, ok := frame["payload"].(map[string]any); ok {
		if payloadType, _ := payload["type"].(string); payloadType != "" {
			return payload
		}
	}
	return frame
}

func asFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		number, err := typed.Float64()
		if err == nil {
			return number
		}
	case string:
		number, err := json.Number(typed).Float64()
		if err == nil {
			return number
		}
	}
	return 0
}

func asUnixTimePtr(value any) *time.Time {
	seconds := int64(asFloat(value))
	if seconds <= 0 {
		return nil
	}
	parsed := time.Unix(seconds, 0).UTC()
	return &parsed
}

func cloneJSONMap(value map[string]any) map[string]any {
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
