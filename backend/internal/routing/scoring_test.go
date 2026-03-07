package routing_test

import (
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/routing"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestScoreCandidates(t *testing.T) {
	t.Parallel()

	candidates := []routing.Candidate{
		{
			Account: accounts.Account{
				AccountName: "priority-winner",
				Priority:    100,
				Status:      accounts.StatusActive,
			},
			Snapshot: usage.Snapshot{
				HealthScore:     0.7,
				AvgLatencyMS:    400,
				RecentErrorRate: 0.01,
			},
		},
		{
			Account: accounts.Account{
				AccountName: "healthy-fast",
				Priority:    50,
				Status:      accounts.StatusActive,
			},
			Snapshot: usage.Snapshot{
				HealthScore:     0.95,
				AvgLatencyMS:    120,
				RecentErrorRate: 0.01,
			},
		},
		{
			Account: accounts.Account{
				AccountName: "degraded",
				Priority:    120,
				Status:      accounts.StatusDegraded,
			},
			Snapshot: usage.Snapshot{
				HealthScore:     0.99,
				AvgLatencyMS:    80,
				RecentErrorRate: 0.01,
			},
		},
		{
			Account: accounts.Account{
				AccountName: "throttled",
				Priority:    150,
				Status:      accounts.StatusActive,
			},
			Snapshot: usage.Snapshot{
				HealthScore:       0.99,
				AvgLatencyMS:      80,
				RecentErrorRate:   0.01,
				ThrottledRecently: true,
			},
		},
	}

	scored := routing.ScoreCandidates(candidates)
	if len(scored) != 4 {
		t.Fatalf("ScoreCandidates returned %d items, want 4", len(scored))
	}

	if scored[0].Account.AccountName != "healthy-fast" {
		t.Fatalf("top candidate = %q, want %q", scored[0].Account.AccountName, "healthy-fast")
	}
	if scored[len(scored)-1].Account.AccountName != "throttled" {
		t.Fatalf("last candidate = %q, want %q", scored[len(scored)-1].Account.AccountName, "throttled")
	}
	if scored[0].Score <= scored[1].Score {
		t.Fatal("top candidate score should be strictly greater than runner-up")
	}
}
