package api

import (
	"encoding/json"
	"math"
	"testing"
)

func TestResponsesUsageCollectorIgnoresInvalidUsageNumbers(t *testing.T) {
	t.Parallel()
	for _, value := range []any{nil, "invalid", "", json.Number("invalid"), math.NaN(), math.Inf(1), -1.0, 101.0, true} {
		collector := newResponsesUsageCollector(7)
		collector.applyRateLimits(map[string]any{"primary": map[string]any{"used_percent": 34.0}})
		collector.applyRateLimits(map[string]any{"primary": map[string]any{"used_percent": value}})
		if collector.snapshot.PrimaryUsedPercent != 34 || collector.snapshot.RPMRemaining != 66 {
			t.Errorf("invalid value %v overwrote primary window: %+v", value, collector.snapshot)
		}
	}
}

func TestResponsesUsageCollectorIgnoresPartialRateLimitWindows(t *testing.T) {
	t.Parallel()

	collector := newResponsesUsageCollector(7)
	collector.Observe(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
			"rate_limits": map[string]any{
				"primary":   map[string]any{"used_percent": 34.0, "resets_at": 1_800_000_000.0},
				"secondary": map[string]any{"used_percent": 58.0, "resets_at": 1_800_100_000.0},
			},
		},
	})

	// Newer Codex streams can emit a partial rate-limit update while a turn is
	// being finalized. Missing used_percent must not overwrite the last complete
	// snapshot with a default value.
	collector.Observe(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"rate_limits": map[string]any{
				"primary":   map[string]any{"resets_at": 1_800_000_000.0},
				"secondary": map[string]any{},
			},
		},
	})

	if collector.snapshot.PrimaryUsedPercent != 34 {
		t.Fatalf("PrimaryUsedPercent = %v, want 34 after partial update", collector.snapshot.PrimaryUsedPercent)
	}
	if collector.snapshot.SecondaryUsedPercent != 58 {
		t.Fatalf("SecondaryUsedPercent = %v, want 58 after partial update", collector.snapshot.SecondaryUsedPercent)
	}
	if collector.snapshot.RPMRemaining != 66 || collector.snapshot.TPMRemaining != 42 {
		t.Fatalf("remaining windows = (%v, %v), want (66, 42) after partial update", collector.snapshot.RPMRemaining, collector.snapshot.TPMRemaining)
	}
}

func TestResponsesUsageCollectorAcceptsExplicitZeroUsagePercent(t *testing.T) {
	t.Parallel()

	collector := newResponsesUsageCollector(7)
	collector.Observe(map[string]any{
		"type": "token_count",
		"rate_limits": map[string]any{
			"primary": map[string]any{"used_percent": 0.0},
		},
	})

	if !collector.observedPrimaryWindow {
		t.Fatal("explicit zero primary usage was not marked observed")
	}
	if collector.snapshot.PrimaryUsedPercent != 0 || collector.snapshot.RPMRemaining != 100 {
		t.Fatalf("primary window = (%v, %v), want (0, 100)", collector.snapshot.PrimaryUsedPercent, collector.snapshot.RPMRemaining)
	}
}

func TestResponsesUsageCollectorIgnoresAdditionalRateLimit(t *testing.T) {
	t.Parallel()

	collector := newResponsesUsageCollector(7)
	collector.Observe(map[string]any{
		"type": "token_count",
		"rate_limits": map[string]any{
			"limit_id":  "codex",
			"primary":   map[string]any{"used_percent": 34.0},
			"secondary": map[string]any{"used_percent": 58.0},
		},
	})
	collector.Observe(map[string]any{
		"type": "token_count",
		"rate_limits": map[string]any{
			"limit_id":  "codex_other",
			"primary":   map[string]any{"used_percent": 100.0},
			"secondary": map[string]any{"used_percent": 100.0},
		},
	})

	if collector.snapshot.PrimaryUsedPercent != 34 || collector.snapshot.SecondaryUsedPercent != 58 {
		t.Fatalf("main windows = (%v, %v), want (34, 58) after additional limit update", collector.snapshot.PrimaryUsedPercent, collector.snapshot.SecondaryUsedPercent)
	}
}
