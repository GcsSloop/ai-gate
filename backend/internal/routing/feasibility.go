package routing

import "github.com/gcssloop/codex-router/backend/internal/usage"

type TokenBudget struct {
	ProjectedInputTokens  float64
	ProjectedOutputTokens float64
	SafetyFactor          float64
	EstimatedCost         float64
}

func IsFeasible(budget TokenBudget, snapshot usage.Snapshot) bool {
	if snapshot.Balance < budget.EstimatedCost {
		return false
	}
	if snapshot.RPMRemaining < 1 {
		return false
	}

	requiredTokens := (budget.ProjectedInputTokens + budget.ProjectedOutputTokens) * max(1, budget.SafetyFactor)
	if snapshot.QuotaRemaining < requiredTokens {
		return false
	}
	if snapshot.TPMRemaining < requiredTokens {
		return false
	}

	return true
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
