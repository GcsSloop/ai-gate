package routing

import (
	"github.com/gcssloop/codex-router/backend/internal/usage"
	usagenormalize "github.com/gcssloop/codex-router/backend/internal/usage/normalize"
)

type TokenBudget struct {
	ProjectedInputTokens  float64
	ProjectedOutputTokens float64
	SafetyFactor          float64
	EstimatedCost         float64
}

func IsFeasible(budget TokenBudget, snapshot usage.Snapshot) bool {
	requiredTokens := (budget.ProjectedInputTokens + budget.ProjectedOutputTokens) * max(1, budget.SafetyFactor)
	presence := usagenormalize.LimitPresenceFromSnapshot(snapshot)
	switch usagenormalize.CapacityModelFromSnapshot(snapshot) {
	case usagenormalize.CapacityModelOfficialWindow:
		if snapshot.RPMRemaining < 1 {
			return false
		}
		return snapshot.TPMRemaining >= 1
	case usagenormalize.CapacityModelQuotaRate:
		if presence.HasBalance && snapshot.Balance < budget.EstimatedCost {
			return false
		}
		if presence.HasRPM && snapshot.RPMRemaining < 1 {
			return false
		}
		if !presence.HasRPM && snapshot.RPMRemaining < 1 && (snapshot.QuotaRemaining > 0 || snapshot.TPMRemaining > 0) {
			// Backward-compatible fallback for legacy snapshots with quota/tpm data
			// but without explicit field presence metadata.
			return false
		}
		if presence.HasQuota && snapshot.QuotaRemaining < requiredTokens {
			return false
		}
		if presence.HasTPM && snapshot.TPMRemaining < requiredTokens {
			return false
		}
		if (presence.HasQuota || presence.HasRPM || presence.HasTPM) &&
			snapshot.QuotaRemaining == 0 && snapshot.RPMRemaining == 0 && snapshot.TPMRemaining == 0 {
			return false
		}
		if !presence.HasQuota && !presence.HasRPM && !presence.HasTPM &&
			snapshot.QuotaRemaining == 0 && snapshot.RPMRemaining == 0 && snapshot.TPMRemaining == 0 {
			// Explicit quota_rate model with no field-presence markers still means
			// this snapshot came from a quota/rate family; all-zero remaining
			// should be treated as exhausted, not unknown/manual.
			return false
		}
		return true
	case usagenormalize.CapacityModelBalanceOnly:
		return snapshot.Balance >= budget.EstimatedCost
	default:
		// Manual/state-only mode: no reliable limits are available yet, so routing
		// should not hard-fail the candidate purely on capacity checks.
		return true
	}
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
