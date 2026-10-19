package routing

import (
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
)

// QuotaProtected reserves the final 3% of either official usage window.
// An expired window must not keep blocking requests based on an old snapshot.
func QuotaProtected(candidate Candidate, now time.Time) bool {
	if candidate.Account.ProviderType != accounts.ProviderOpenAIOfficial || !candidate.Account.QuotaProtectionEnabled {
		return false
	}
	return protectedWindow(candidate.Snapshot.PrimaryUsedPercent, candidate.Snapshot.PrimaryResetsAt, now) ||
		protectedWindow(candidate.Snapshot.SecondaryUsedPercent, candidate.Snapshot.SecondaryResetsAt, now)
}

func protectedWindow(usedPercent float64, resetsAt *time.Time, now time.Time) bool {
	return usedPercent >= 97 && (resetsAt == nil || resetsAt.After(now))
}
