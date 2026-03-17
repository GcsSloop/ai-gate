package api

import (
	"sort"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/routing"
	"github.com/gcssloop/codex-router/backend/internal/settings"
)

type activeAccountWriter interface {
	SetActive(id int64) error
}

func autoFailoverEnabled(repo settings.ReadRepository) bool {
	if repo == nil {
		return false
	}
	appSettings, err := repo.GetAppSettings()
	if err != nil {
		return false
	}
	return appSettings.AutoFailoverEnabled
}

func orderCandidatesByPriority(candidates []routing.Candidate) []routing.Candidate {
	ordered := make([]routing.Candidate, len(candidates))
	copy(ordered, candidates)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Account.Priority == ordered[j].Account.Priority {
			return ordered[i].Account.ID < ordered[j].Account.ID
		}
		return ordered[i].Account.Priority > ordered[j].Account.Priority
	})
	return ordered
}

func orderCandidatesActiveFirst(candidates []routing.Candidate) []routing.Candidate {
	ordered := orderCandidatesByPriority(candidates)
	activeIndex := -1
	for index, candidate := range ordered {
		if candidate.Account.IsActive {
			activeIndex = index
			break
		}
	}
	if activeIndex <= 0 {
		return ordered
	}

	active := ordered[activeIndex]
	copy(ordered[1:activeIndex+1], ordered[0:activeIndex])
	ordered[0] = active
	return ordered
}

func activeCandidate(candidates []routing.Candidate) (routing.Candidate, bool) {
	for _, candidate := range candidates {
		if candidate.Account.IsActive {
			return candidate, true
		}
	}
	return routing.Candidate{}, false
}

func syncActiveAccount(repo activeAccountWriter, account accounts.Account) (bool, error) {
	if repo == nil || account.IsActive {
		return false, nil
	}
	if err := repo.SetActive(account.ID); err != nil {
		return false, err
	}
	return true, nil
}
