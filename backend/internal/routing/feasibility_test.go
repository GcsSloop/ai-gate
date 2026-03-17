package routing_test

import (
	"testing"
	"time"

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
		{
			name: "official window based limits stay feasible when percentages remain",
			budget: routing.TokenBudget{
				ProjectedInputTokens:  3000,
				ProjectedOutputTokens: 2000,
				SafetyFactor:          1.2,
				EstimatedCost:         0.01,
			},
			snapshot: usage.Snapshot{
				Balance:              0,
				QuotaRemaining:       0,
				RPMRemaining:         69,
				TPMRemaining:         25,
				PrimaryUsedPercent:   31,
				SecondaryUsedPercent: 75,
				PrimaryResetsAt:      timePtr("2026-03-07T20:06:00Z"),
				SecondaryResetsAt:    timePtr("2026-03-12T16:20:30Z"),
			},
			want: true,
		},
		{
			name: "third-party quota and rate limits stay feasible without balance field",
			budget: routing.TokenBudget{
				ProjectedInputTokens:  1500,
				ProjectedOutputTokens: 1500,
				SafetyFactor:          1.2,
				EstimatedCost:         10,
			},
			snapshot: usage.Snapshot{
				Balance:              0,
				QuotaRemaining:       10000,
				RPMRemaining:         20,
				TPMRemaining:         10000,
				ProviderSnapshotJSON: `{"capacity_model":"quota_rate"}`,
			},
			want: true,
		},
		{
			name: "balance-only providers should not be rejected by missing quota rates",
			budget: routing.TokenBudget{
				ProjectedInputTokens:  600,
				ProjectedOutputTokens: 400,
				SafetyFactor:          1.0,
				EstimatedCost:         2,
			},
			snapshot: usage.Snapshot{
				Balance:              8,
				QuotaRemaining:       0,
				RPMRemaining:         0,
				TPMRemaining:         0,
				ProviderSnapshotJSON: `{"capacity_model":"balance_only"}`,
			},
			want: true,
		},
		{
			name: "exhausted quota-rate snapshot with explicit model should be rejected",
			budget: routing.TokenBudget{
				ProjectedInputTokens:  200,
				ProjectedOutputTokens: 200,
				SafetyFactor:          1,
				EstimatedCost:         1,
			},
			snapshot: usage.Snapshot{
				Balance:              0,
				QuotaRemaining:       0,
				RPMRemaining:         0,
				TPMRemaining:         0,
				ProviderSnapshotJSON: `{"capacity_model":"quota_rate"}`,
			},
			want: false,
		},
		{
			name: "unknown manual snapshot without model stays feasible",
			budget: routing.TokenBudget{
				ProjectedInputTokens:  200,
				ProjectedOutputTokens: 200,
				SafetyFactor:          1,
				EstimatedCost:         1,
			},
			snapshot: usage.Snapshot{
				Balance:        0,
				QuotaRemaining: 0,
				RPMRemaining:   0,
				TPMRemaining:   0,
			},
			want: true,
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

func timePtr(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return &parsed
}
