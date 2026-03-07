package routing

import (
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
)

type CooldownReason string

const (
	CooldownReasonCapacity  CooldownReason = "capacity"
	CooldownReasonRateLimit CooldownReason = "rate_limit"
)

func ComputeCooldownUntil(now time.Time, _ CooldownReason, resetAt *time.Time, fallbackWindow time.Duration) time.Time {
	if resetAt != nil {
		return resetAt.UTC()
	}
	return now.UTC().Add(fallbackWindow)
}

func ShouldProbeRecovery(account accounts.Account, now time.Time) bool {
	if account.Status != accounts.StatusCooldown {
		return false
	}
	if account.CooldownUntil == nil {
		return true
	}
	return !account.CooldownUntil.UTC().After(now.UTC())
}
