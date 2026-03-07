package routing

import (
	"sort"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type Candidate struct {
	Account  accounts.Account
	Snapshot usage.Snapshot
	Score    float64
}

func ScoreCandidates(candidates []Candidate) []Candidate {
	scored := make([]Candidate, len(candidates))
	copy(scored, candidates)

	for i := range scored {
		scored[i].Score = scoreCandidate(scored[i])
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored
}

func scoreCandidate(candidate Candidate) float64 {
	if candidate.Account.IsActive {
		return 1_000_000_000
	}

	score := float64(candidate.Account.Priority) * 0.75
	score += candidate.Snapshot.HealthScore * 100
	score -= candidate.Snapshot.AvgLatencyMS / 10
	score -= candidate.Snapshot.RecentErrorRate * 100

	if candidate.Account.Status == accounts.StatusDegraded {
		score -= 100
	}
	if candidate.Snapshot.ThrottledRecently {
		score -= 150
	}

	return score
}
