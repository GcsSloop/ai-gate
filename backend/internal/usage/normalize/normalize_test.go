package normalize_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/usage/normalize"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
)

func TestFromRawOfficialWindowLimits(t *testing.T) {
	t.Parallel()

	checkedAt := time.Now().UTC().Round(time.Second)
	primaryReset := mustTime("2026-03-20T00:00:00Z")
	secondaryReset := mustTime("2026-03-27T00:00:00Z")
	result := usagedrv.RawUsageResult{
		Source:     "remote",
		Confidence: "high",
		Payload:    map[string]any{"plan": "plus"},
		Limits: usagedrv.UsageLimits{
			Balance:              floatPtr(5.39),
			PrimaryUsedPercent:   floatPtr(34),
			SecondaryUsedPercent: floatPtr(58),
			PrimaryResetsAt:      &primaryReset,
			SecondaryResetsAt:    &secondaryReset,
		},
		Meta: map[string]any{
			"allowed":       true,
			"limit_reached": false,
		},
	}

	snapshot := normalize.FromRaw(7, result, checkedAt)
	if snapshot.AccountID != 7 {
		t.Fatalf("AccountID = %d, want 7", snapshot.AccountID)
	}
	if snapshot.Balance != 5.39 {
		t.Fatalf("Balance = %v, want 5.39", snapshot.Balance)
	}
	if snapshot.RPMRemaining != 66 {
		t.Fatalf("RPMRemaining = %v, want 66", snapshot.RPMRemaining)
	}
	if snapshot.TPMRemaining != 42 {
		t.Fatalf("TPMRemaining = %v, want 42", snapshot.TPMRemaining)
	}
	if snapshot.PrimaryResetsAt == nil || !snapshot.PrimaryResetsAt.Equal(primaryReset) {
		t.Fatalf("PrimaryResetsAt = %v, want %v", snapshot.PrimaryResetsAt, primaryReset)
	}
	if snapshot.SecondaryResetsAt == nil || !snapshot.SecondaryResetsAt.Equal(secondaryReset) {
		t.Fatalf("SecondaryResetsAt = %v, want %v", snapshot.SecondaryResetsAt, secondaryReset)
	}
	if snapshot.ProviderSnapshotJSON == "" {
		t.Fatal("ProviderSnapshotJSON is empty")
	}
	if model := extractCapacityModel(t, snapshot.ProviderSnapshotJSON); model != "official_window" {
		t.Fatalf("capacity_model = %q, want %q", model, "official_window")
	}
}

func TestFromRawThirdPartyQuotaRateLimits(t *testing.T) {
	t.Parallel()

	result := usagedrv.RawUsageResult{
		Source:     "remote",
		Confidence: "high",
		Limits: usagedrv.UsageLimits{
			QuotaRemaining: floatPtr(20480),
			RPMRemaining:   floatPtr(60),
			TPMRemaining:   floatPtr(120000),
		},
	}
	snapshot := normalize.FromRaw(11, result, time.Now().UTC())
	if snapshot.QuotaRemaining != 20480 {
		t.Fatalf("QuotaRemaining = %v, want 20480", snapshot.QuotaRemaining)
	}
	if snapshot.RPMRemaining != 60 {
		t.Fatalf("RPMRemaining = %v, want 60", snapshot.RPMRemaining)
	}
	if snapshot.TPMRemaining != 120000 {
		t.Fatalf("TPMRemaining = %v, want 120000", snapshot.TPMRemaining)
	}
	if model := extractCapacityModel(t, snapshot.ProviderSnapshotJSON); model != "quota_rate" {
		t.Fatalf("capacity_model = %q, want %q", model, "quota_rate")
	}
}

func TestFromRawBalanceOnlyLimits(t *testing.T) {
	t.Parallel()

	result := usagedrv.RawUsageResult{
		Source:     "remote",
		Confidence: "medium",
		Limits: usagedrv.UsageLimits{
			Balance: floatPtr(32),
		},
	}
	snapshot := normalize.FromRaw(9, result, time.Now().UTC())
	if snapshot.Balance != 32 {
		t.Fatalf("Balance = %v, want 32", snapshot.Balance)
	}
	if snapshot.QuotaRemaining != 0 || snapshot.RPMRemaining != 0 || snapshot.TPMRemaining != 0 {
		t.Fatalf("quota/rate fields = (%v, %v, %v), want zero values", snapshot.QuotaRemaining, snapshot.RPMRemaining, snapshot.TPMRemaining)
	}
	if model := extractCapacityModel(t, snapshot.ProviderSnapshotJSON); model != "balance_only" {
		t.Fatalf("capacity_model = %q, want %q", model, "balance_only")
	}
}

func TestFromRawPreservesDisplayHints(t *testing.T) {
	t.Parallel()

	result := usagedrv.RawUsageResult{
		Source:     "remote",
		Confidence: "high",
		Limits: usagedrv.UsageLimits{
			Balance: floatPtr(61.96),
		},
		Display: map[string]any{
			"summary": map[string]any{
				"label": "余额",
				"value": "$61.96",
			},
			"detail_stats": []any{
				map[string]any{"label": "余额", "value": "$61.96"},
			},
		},
	}

	snapshot := normalize.FromRaw(12, result, time.Now().UTC())
	var decoded struct {
		Display map[string]any `json:"display"`
	}
	if err := json.Unmarshal([]byte(snapshot.ProviderSnapshotJSON), &decoded); err != nil {
		t.Fatalf("Unmarshal ProviderSnapshotJSON returned error: %v", err)
	}
	summary, ok := decoded.Display["summary"].(map[string]any)
	if !ok {
		t.Fatalf("display.summary = %#v, want object", decoded.Display["summary"])
	}
	if summary["label"] != "余额" || summary["value"] != "$61.96" {
		t.Fatalf("display.summary = %#v, want balance label/value", summary)
	}
}

func TestFromRawStaleLowConfidence(t *testing.T) {
	t.Parallel()

	result := usagedrv.RawUsageResult{
		Source:     "inferred",
		Confidence: "low",
		Meta: map[string]any{
			"stale":      true,
			"last_error": "usage endpoint timeout",
		},
		Limits: usagedrv.UsageLimits{
			Balance: floatPtr(1.5),
		},
	}

	snapshot := normalize.FromRaw(3, result, time.Now().UTC())
	if !snapshot.Stale {
		t.Fatal("Stale = false, want true")
	}
	if snapshot.Confidence != "low" {
		t.Fatalf("Confidence = %q, want %q", snapshot.Confidence, "low")
	}
	if snapshot.Source != "inferred" {
		t.Fatalf("Source = %q, want %q", snapshot.Source, "inferred")
	}
	if snapshot.LastError != "usage endpoint timeout" {
		t.Fatalf("LastError = %q, want %q", snapshot.LastError, "usage endpoint timeout")
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

func mustTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func extractCapacityModel(t *testing.T, raw string) string {
	t.Helper()
	var decoded struct {
		CapacityModel string `json:"capacity_model"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	return decoded.CapacityModel
}
