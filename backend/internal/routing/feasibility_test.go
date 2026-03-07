package routing_test

import (
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/routing"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestIsFeasible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		budget   routing.TokenBudget
		snapshot usage.Snapshot
		want     bool
	}{
		{
			name: "enough balance and quota",
			budget: routing.TokenBudget{
				ProjectedInputTokens:  1200,
				ProjectedOutputTokens: 1800,
				SafetyFactor:          1.5,
				EstimatedCost:         2.5,
			},
			snapshot: usage.Snapshot{
				Balance:        10,
				QuotaRemaining: 10000,
				RPMRemaining:   20,
				TPMRemaining:   10000,
			},
			want: true,
		},
		{
			name: "insufficient balance",
			budget: routing.TokenBudget{
				ProjectedInputTokens:  1000,
				ProjectedOutputTokens: 1000,
				SafetyFactor:          1.2,
				EstimatedCost:         5,
			},
			snapshot: usage.Snapshot{
				Balance:        2,
				QuotaRemaining: 10000,
				RPMRemaining:   20,
				TPMRemaining:   10000,
			},
			want: false,
		},
		{
			name: "insufficient request budget",
			budget: routing.TokenBudget{
				ProjectedInputTokens:  600,
				ProjectedOutputTokens: 400,
				SafetyFactor:          1.0,
				EstimatedCost:         1,
			},
			snapshot: usage.Snapshot{
				Balance:        10,
				QuotaRemaining: 10000,
				RPMRemaining:   0,
				TPMRemaining:   10000,
			},
			want: false,
		},
		{
			name: "insufficient token budget after safety factor",
			budget: routing.TokenBudget{
				ProjectedInputTokens:  3000,
				ProjectedOutputTokens: 4000,
				SafetyFactor:          1.5,
				EstimatedCost:         1,
			},
			snapshot: usage.Snapshot{
				Balance:        10,
				QuotaRemaining: 9000,
				RPMRemaining:   20,
				TPMRemaining:   9000,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := routing.IsFeasible(tt.budget, tt.snapshot)
			if got != tt.want {
				t.Fatalf("IsFeasible() = %v, want %v", got, tt.want)
			}
		})
	}
}
