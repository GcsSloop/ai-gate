package api

import (
	"context"
	"fmt"

	"github.com/gcssloop/codex-router/backend/internal/routing"
)

func serverUserRouteScope(userID int64, requestKind string) string {
	if userID <= 0 || requestKind == "" {
		return ""
	}
	return fmt.Sprintf("%s:user:%d", requestKind, userID)
}

func gatewayStickyScope(ctx context.Context, requestKind string) string {
	if user, ok := ServerUserFromContext(ctx); ok && user.ID > 0 {
		return serverUserRouteScope(user.ID, requestKind)
	}
	return ""
}

func rememberServerUserSticky(sticky *routing.StickySelector, userID int64, accountID int64) {
	if sticky == nil || userID <= 0 || accountID <= 0 {
		return
	}
	sticky.Remember(serverUserRouteScope(userID, "responses"), accountID)
	sticky.Remember(serverUserRouteScope(userID, "chat_completions"), accountID)
}

func orderServerUserCandidates(ctx context.Context, sticky *routing.StickySelector, requestKind string, candidates []routing.Candidate) []routing.Candidate {
	scope := gatewayStickyScope(ctx, requestKind)
	ordered := sticky.Apply(scope, candidates)
	user, ok := ServerUserFromContext(ctx)
	if !ok || user.PreferredAccountID == nil || *user.PreferredAccountID <= 0 {
		return ordered
	}
	ordered = markServerUserManualCandidate(ordered, *user.PreferredAccountID)
	if user.RouteLocked {
		return moveServerUserCandidateToFront(ordered, *user.PreferredAccountID)
	}
	if sticky == nil {
		return moveServerUserCandidateToFront(ordered, *user.PreferredAccountID)
	}
	if _, ok := sticky.Current(scope); ok {
		return ordered
	}
	return moveServerUserCandidateToFront(ordered, *user.PreferredAccountID)
}

func markServerUserManualCandidate(candidates []routing.Candidate, accountID int64) []routing.Candidate {
	if accountID <= 0 || len(candidates) == 0 {
		return candidates
	}
	marked := make([]routing.Candidate, len(candidates))
	copy(marked, candidates)
	for index := range marked {
		if marked[index].Account.ID == accountID {
			marked[index].Account.IsActive = true
			break
		}
	}
	return marked
}

func moveServerUserCandidateToFront(candidates []routing.Candidate, accountID int64) []routing.Candidate {
	if accountID <= 0 || len(candidates) <= 1 {
		return candidates
	}
	ordered := make([]routing.Candidate, len(candidates))
	copy(ordered, candidates)
	for index, candidate := range ordered {
		if candidate.Account.ID != accountID {
			continue
		}
		if index == 0 {
			return ordered
		}
		selected := ordered[index]
		copy(ordered[1:index+1], ordered[0:index])
		ordered[0] = selected
		return ordered
	}
	return ordered
}

func serverUserFromContextExists(ctx context.Context) bool {
	_, ok := ServerUserFromContext(ctx)
	return ok
}
