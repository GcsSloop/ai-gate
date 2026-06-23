package routing_test

import (
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/routing"
)

func TestStickySelectorPrefersRememberedAccountWithinTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	selector := routing.NewStickySelector(time.Minute, func() time.Time { return now })
	candidates := []routing.Candidate{
		{Account: accounts.Account{ID: 1, AccountName: "best", Priority: 100, Status: accounts.StatusActive}},
		{Account: accounts.Account{ID: 2, AccountName: "remembered", Priority: 1, Status: accounts.StatusActive}},
	}

	selector.Remember("responses", 2)
	ordered := selector.Apply("responses", candidates)

	if ordered[0].Account.ID != 2 {
		t.Fatalf("first account = %d, want remembered account 2", ordered[0].Account.ID)
	}
}

func TestStickySelectorFallsBackAfterInvalidateOrExpiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	selector := routing.NewStickySelector(time.Minute, func() time.Time { return now })
	candidates := []routing.Candidate{
		{Account: accounts.Account{ID: 1, AccountName: "best", Priority: 100, Status: accounts.StatusActive}},
		{Account: accounts.Account{ID: 2, AccountName: "remembered", Priority: 1, Status: accounts.StatusActive}},
	}

	selector.Remember("responses", 2)
	selector.Invalidate("responses", 2)
	ordered := selector.Apply("responses", candidates)
	if ordered[0].Account.ID != 1 {
		t.Fatalf("first account after invalidate = %d, want best account 1", ordered[0].Account.ID)
	}

	selector.Remember("responses", 2)
	now = now.Add(2 * time.Minute)
	ordered = selector.Apply("responses", candidates)
	if ordered[0].Account.ID != 1 {
		t.Fatalf("first account after expiry = %d, want best account 1", ordered[0].Account.ID)
	}
}
