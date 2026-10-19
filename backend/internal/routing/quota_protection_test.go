package routing_test

import (
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/routing"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestQuotaProtection(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	past := now.Add(-time.Second)
	for _, tt := range []struct {
		name                         string
		primary, secondary           float64
		primaryReset, secondaryReset *time.Time
		disabled, thirdParty         bool
		want                         bool
	}{
		{name: "unknown usage"},
		{name: "above reserve", primary: 96.99, secondary: 96.99},
		{name: "five hour boundary", primary: 97, want: true},
		{name: "overall boundary", secondary: 97, want: true},
		{name: "both exhausted", primary: 100, secondary: 100, want: true},
		{name: "disabled", primary: 100, secondary: 100, disabled: true},
		{name: "third party", primary: 100, secondary: 100, thirdParty: true},
		{name: "five hour reset", primary: 98, primaryReset: &past},
		{name: "reset does not release other window", primary: 98, primaryReset: &past, secondary: 99, secondaryReset: &future, want: true},
		{name: "overall reset", secondary: 98, secondaryReset: &past},
		{name: "reset boundary", primary: 98, primaryReset: &now},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider := accounts.ProviderOpenAIOfficial
			if tt.thirdParty {
				provider = accounts.ProviderOpenAICompatible
			}
			candidate := routing.Candidate{
				Account:  accounts.Account{ProviderType: provider, QuotaProtectionEnabled: !tt.disabled, IsActive: true},
				Snapshot: usage.Snapshot{PrimaryUsedPercent: tt.primary, SecondaryUsedPercent: tt.secondary, PrimaryResetsAt: tt.primaryReset, SecondaryResetsAt: tt.secondaryReset},
			}
			if got := routing.QuotaProtected(candidate, now); got != tt.want {
				t.Fatalf("QuotaProtected = %v, want %v", got, tt.want)
			}
		})
	}
}
