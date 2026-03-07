package routing_test

import (
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/routing"
)

func TestComputeCooldownUntil(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		reason    routing.CooldownReason
		resetAt   *time.Time
		window    time.Duration
		wantUntil time.Time
	}{
		{
			name:      "explicit provider reset wins",
			reason:    routing.CooldownReasonRateLimit,
			resetAt:   ptrTime(now.Add(2 * time.Hour)),
			window:    6 * time.Hour,
			wantUntil: now.Add(2 * time.Hour),
		},
		{
			name:      "rolling window fallback",
			reason:    routing.CooldownReasonCapacity,
			window:    5 * time.Hour,
			wantUntil: now.Add(5 * time.Hour),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := routing.ComputeCooldownUntil(now, tt.reason, tt.resetAt, tt.window)
			if !got.Equal(tt.wantUntil) {
				t.Fatalf("ComputeCooldownUntil() = %v, want %v", got, tt.wantUntil)
			}
		})
	}
}

func TestShouldProbeRecovery(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-1 * time.Minute)
	future := now.Add(1 * time.Minute)

	if !routing.ShouldProbeRecovery(accounts.Account{Status: accounts.StatusCooldown, CooldownUntil: &expired}, now) {
		t.Fatal("ShouldProbeRecovery() = false, want true for expired cooldown")
	}
	if routing.ShouldProbeRecovery(accounts.Account{Status: accounts.StatusCooldown, CooldownUntil: &future}, now) {
		t.Fatal("ShouldProbeRecovery() = true, want false for active cooldown")
	}
	if routing.ShouldProbeRecovery(accounts.Account{Status: accounts.StatusDisabled, CooldownUntil: &expired}, now) {
		t.Fatal("ShouldProbeRecovery() = true, want false for disabled account")
	}
	if routing.ShouldProbeRecovery(accounts.Account{Status: accounts.StatusInvalid, CooldownUntil: &expired}, now) {
		t.Fatal("ShouldProbeRecovery() = true, want false for invalid account")
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
