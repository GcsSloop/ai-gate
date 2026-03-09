package settings

import (
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/routing"
)

func OrderCandidates(repo ReadRepository, candidates []routing.Candidate) ([]routing.Candidate, error) {
	if repo == nil {
		return routing.ScoreCandidates(candidates), nil
	}

	appSettings, err := repo.GetAppSettings()
	if err != nil {
		return nil, err
	}
	if !appSettings.AutoFailoverEnabled {
		return routing.ScoreCandidates(candidates), nil
	}

	queue, err := repo.ListFailoverQueue()
	if err != nil {
		return nil, err
	}
	if len(queue) == 0 {
		return routing.ScoreCandidates(candidates), nil
	}

	candidatesByID := make(map[int64]routing.Candidate, len(candidates))
	for _, candidate := range candidates {
		if !eligibleForExplicitQueue(candidate.Account) {
			continue
		}
		candidatesByID[candidate.Account.ID] = candidate
	}

	ordered := make([]routing.Candidate, 0, len(queue))
	for _, accountID := range queue {
		candidate, ok := candidatesByID[accountID]
		if !ok {
			continue
		}
		ordered = append(ordered, candidate)
	}
	if len(ordered) == 0 {
		return routing.ScoreCandidates(candidates), nil
	}
	return ordered, nil
}

func eligibleForExplicitQueue(account accounts.Account) bool {
	switch account.Status {
	case accounts.StatusDisabled, accounts.StatusInvalid, accounts.StatusCooldown:
		return false
	default:
		return true
	}
}
