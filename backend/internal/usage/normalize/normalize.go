package normalize

import (
	"encoding/json"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/usage"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
)

type CapacityModel string

const (
	CapacityModelOfficialWindow CapacityModel = "official_window"
	CapacityModelQuotaRate      CapacityModel = "quota_rate"
	CapacityModelBalanceOnly    CapacityModel = "balance_only"
	CapacityModelManual         CapacityModel = "manual"
)

type LimitPresence struct {
	HasBalance bool
	HasQuota   bool
	HasRPM     bool
	HasTPM     bool
}

func FromRaw(accountID int64, result usagedrv.RawUsageResult, checkedAt time.Time) usage.Snapshot {
	snapshot := usage.Snapshot{
		AccountID:  accountID,
		Source:     defaultSource(result.Source),
		Confidence: defaultConfidence(result.Confidence),
		CheckedAt:  checkedAt.UTC(),
	}

	if stale, ok := result.Meta["stale"].(bool); ok {
		snapshot.Stale = stale
	}
	if lastError, ok := result.Meta["last_error"].(string); ok {
		snapshot.LastError = lastError
	}

	if result.Limits.Balance != nil {
		snapshot.Balance = *result.Limits.Balance
	}
	if result.Limits.QuotaRemaining != nil {
		snapshot.QuotaRemaining = *result.Limits.QuotaRemaining
	}
	if result.Limits.RPMRemaining != nil {
		snapshot.RPMRemaining = *result.Limits.RPMRemaining
	}
	if result.Limits.TPMRemaining != nil {
		snapshot.TPMRemaining = *result.Limits.TPMRemaining
	}
	if result.Limits.PrimaryUsedPercent != nil {
		snapshot.PrimaryUsedPercent = *result.Limits.PrimaryUsedPercent
	}
	if result.Limits.SecondaryUsedPercent != nil {
		snapshot.SecondaryUsedPercent = *result.Limits.SecondaryUsedPercent
	}
	snapshot.PrimaryResetsAt = result.Limits.PrimaryResetsAt
	snapshot.SecondaryResetsAt = result.Limits.SecondaryResetsAt

	// For official-style window limits, derive remaining capacity from used percent
	// when the remaining fields were omitted by the provider.
	if usesWindowPercentLimits(snapshot) {
		if snapshot.RPMRemaining == 0 {
			snapshot.RPMRemaining = max(100-snapshot.PrimaryUsedPercent, 0)
		}
		if snapshot.TPMRemaining == 0 {
			snapshot.TPMRemaining = max(100-snapshot.SecondaryUsedPercent, 0)
		}
		if snapshot.HealthScore == 0 {
			snapshot.HealthScore = (snapshot.RPMRemaining + snapshot.TPMRemaining) / 200
		}
	}

	if throttled, ok := result.Meta["limit_reached"].(bool); ok && throttled {
		snapshot.ThrottledRecently = true
	}
	if allowed, ok := result.Meta["allowed"].(bool); ok && !allowed {
		snapshot.ThrottledRecently = true
	}

	if !usesWindowPercentLimits(snapshot) && snapshot.QuotaRemaining == 0 && snapshot.RPMRemaining == 0 && snapshot.TPMRemaining == 0 && snapshot.Balance > 0 {
		snapshot.HealthScore = 1
	}
	snapshot.ProviderSnapshotJSON = marshalProviderSnapshot(
		result.Payload,
		capacityModelFromRaw(result, snapshot),
		LimitPresence{
			HasBalance: result.Limits.Balance != nil,
			HasQuota:   result.Limits.QuotaRemaining != nil,
			HasRPM:     result.Limits.RPMRemaining != nil,
			HasTPM:     result.Limits.TPMRemaining != nil,
		},
	)

	return snapshot
}

func DefaultFallbackSnapshot(accountID int64) usage.Snapshot {
	return usage.Snapshot{
		AccountID:            accountID,
		Source:               "inferred",
		Confidence:           "low",
		ProviderSnapshotJSON: marshalProviderSnapshot(nil, CapacityModelManual, LimitPresence{}),
		Balance:              1,
		QuotaRemaining:       1_000_000,
		RPMRemaining:         100,
		TPMRemaining:         1_000_000,
		HealthScore:          0.5,
		RecentErrorRate:      0,
	}
}

func CapacityModelFromSnapshot(snapshot usage.Snapshot) CapacityModel {
	if decoded, ok := parseProviderSnapshot(snapshot.ProviderSnapshotJSON); ok {
		return decoded.CapacityModel
	}
	// Backward-compatible inference for legacy snapshots without explicit marker.
	if usesWindowPercentLimits(snapshot) {
		return CapacityModelOfficialWindow
	}
	if snapshot.QuotaRemaining > 0 || snapshot.RPMRemaining > 0 || snapshot.TPMRemaining > 0 {
		return CapacityModelQuotaRate
	}
	if snapshot.Balance > 0 {
		return CapacityModelBalanceOnly
	}
	return CapacityModelManual
}

func LimitPresenceFromSnapshot(snapshot usage.Snapshot) LimitPresence {
	if decoded, ok := parseProviderSnapshot(snapshot.ProviderSnapshotJSON); ok {
		return LimitPresence{
			HasBalance: decoded.HasBalance,
			HasQuota:   decoded.HasQuota,
			HasRPM:     decoded.HasRPM,
			HasTPM:     decoded.HasTPM,
		}
	}
	// Legacy snapshots: infer only positive-value presence.
	return LimitPresence{
		HasBalance: snapshot.Balance > 0,
		HasQuota:   snapshot.QuotaRemaining > 0,
		HasRPM:     snapshot.RPMRemaining > 0,
		HasTPM:     snapshot.TPMRemaining > 0,
	}
}

type providerSnapshot struct {
	CapacityModel CapacityModel  `json:"capacity_model,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
	HasBalance    bool           `json:"has_balance,omitempty"`
	HasQuota      bool           `json:"has_quota,omitempty"`
	HasRPM        bool           `json:"has_rpm,omitempty"`
	HasTPM        bool           `json:"has_tpm,omitempty"`
}

func parseProviderSnapshot(raw string) (providerSnapshot, bool) {
	if raw == "" {
		return providerSnapshot{}, false
	}
	var decoded providerSnapshot
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return providerSnapshot{}, false
	}
	switch decoded.CapacityModel {
	case CapacityModelOfficialWindow, CapacityModelQuotaRate, CapacityModelBalanceOnly, CapacityModelManual:
		return decoded, true
	}
	return providerSnapshot{}, false
}

func usesWindowPercentLimits(snapshot usage.Snapshot) bool {
	return snapshot.PrimaryResetsAt != nil ||
		snapshot.SecondaryResetsAt != nil ||
		snapshot.PrimaryUsedPercent > 0 ||
		snapshot.SecondaryUsedPercent > 0
}

func defaultSource(value string) string {
	if value == "" {
		return "remote"
	}
	return value
}

func defaultConfidence(value string) string {
	if value == "" {
		return "medium"
	}
	return value
}

func capacityModelFromRaw(result usagedrv.RawUsageResult, snapshot usage.Snapshot) CapacityModel {
	if usesWindowPercentLimits(snapshot) {
		return CapacityModelOfficialWindow
	}
	if result.Limits.QuotaRemaining != nil || result.Limits.RPMRemaining != nil || result.Limits.TPMRemaining != nil {
		return CapacityModelQuotaRate
	}
	if result.Limits.Balance != nil {
		return CapacityModelBalanceOnly
	}
	return CapacityModelManual
}

func marshalProviderSnapshot(payload map[string]any, capacityModel CapacityModel, presence LimitPresence) string {
	raw, err := json.Marshal(providerSnapshot{
		CapacityModel: capacityModel,
		Payload:       payload,
		HasBalance:    presence.HasBalance,
		HasQuota:      presence.HasQuota,
		HasRPM:        presence.HasRPM,
		HasTPM:        presence.HasTPM,
	})
	if err != nil {
		return ""
	}
	return string(raw)
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
