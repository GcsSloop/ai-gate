package routing

import "github.com/gcssloop/codex-router/backend/internal/usage"

type TokenBudget struct {
	ProjectedInputTokens  float64
	ProjectedOutputTokens float64
	SafetyFactor          float64
	EstimatedCost         float64
}

func IsFeasible(budget TokenBudget, snapshot usage.Snapshot) bool {
	if !usesWindowPercentLimits(snapshot) && snapshot.Balance < budget.EstimatedCost {
		return false
	}
	if snapshot.RPMRemaining < 1 {
		return false
	}

	requiredTokens := (budget.ProjectedInputTokens + budget.ProjectedOutputTokens) * max(1, budget.SafetyFactor)
	if usesWindowPercentLimits(snapshot) {
		return snapshot.TPMRemaining >= 1
	}
	if snapshot.QuotaRemaining < requiredTokens {
		return false
	}
	if snapshot.TPMRemaining < requiredTokens {
		return false
	}

	return true
}

func usesWindowPercentLimits(snapshot usage.Snapshot) bool {
	return snapshot.PrimaryResetsAt != nil ||
		snapshot.SecondaryResetsAt != nil ||
		snapshot.PrimaryUsedPercent > 0 ||
		snapshot.SecondaryUsedPercent > 0
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
