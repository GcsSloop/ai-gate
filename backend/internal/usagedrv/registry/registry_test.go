package registry_test

import (
	"context"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv/registry"
)

type stubAccountDriver struct {
	name     string
	supports func(accounts.Account) bool
}

func (d stubAccountDriver) Name() string {
	return d.name
}

func (d stubAccountDriver) Supports(account accounts.Account) bool {
	return d.supports(account)
}

func (d stubAccountDriver) Resolve(context.Context, accounts.Account) (accountdrv.ResolvedCredential, error) {
	return accountdrv.ResolvedCredential{Kind: d.name}, nil
}

type stubUsageDriver struct {
	name     string
	supports func(accounts.Account) bool
}

func (d stubUsageDriver) Name() string {
	return d.name
}

func (d stubUsageDriver) Supports(account accounts.Account) bool {
	return d.supports(account)
}

func (d stubUsageDriver) Fetch(context.Context, accounts.Account, accountdrv.ResolvedCredential) (usagedrv.RawUsageResult, error) {
	return usagedrv.RawUsageResult{Source: d.name}, nil
}

func TestRegistrySelectsExplicitDriversByName(t *testing.T) {
	t.Parallel()

	reg, err := registry.New(
		[]accountdrv.AccountDriver{
			stubAccountDriver{name: "apikey", supports: func(accounts.Account) bool { return false }},
			stubAccountDriver{name: "official-session", supports: func(accounts.Account) bool { return false }},
		},
		[]usagedrv.UsageDriver{
			stubUsageDriver{name: "lua", supports: func(accounts.Account) bool { return false }},
			stubUsageDriver{name: "builtin_openai_official", supports: func(accounts.Account) bool { return false }},
		},
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	account := accounts.Account{
		AccountDriver: "official-session",
		UsageDriver:   "lua",
	}

	accountDriver, err := reg.AccountDriverFor(account)
	if err != nil {
		t.Fatalf("AccountDriverFor returned error: %v", err)
	}
	if accountDriver.Name() != "official-session" {
		t.Fatalf("AccountDriverFor = %q, want %q", accountDriver.Name(), "official-session")
	}

	usageDriver, err := reg.UsageDriverFor(account)
	if err != nil {
		t.Fatalf("UsageDriverFor returned error: %v", err)
	}
	if usageDriver.Name() != "lua" {
		t.Fatalf("UsageDriverFor = %q, want %q", usageDriver.Name(), "lua")
	}
}

func TestRegistryFallsBackToSupportingBuiltInDrivers(t *testing.T) {
	t.Parallel()

	reg, err := registry.New(
		[]accountdrv.AccountDriver{
			stubAccountDriver{
				name: "apikey",
				supports: func(account accounts.Account) bool {
					return account.AuthMode == accounts.AuthModeAPIKey
				},
			},
			stubAccountDriver{
				name: "official-session",
				supports: func(account accounts.Account) bool {
					return account.ProviderType == accounts.ProviderOpenAIOfficial || account.AuthMode == accounts.AuthModeLocalImport
				},
			},
		},
		[]usagedrv.UsageDriver{
			stubUsageDriver{
				name: "builtin_openai_official",
				supports: func(account accounts.Account) bool {
					return account.ProviderType == accounts.ProviderOpenAIOfficial || account.AuthMode == accounts.AuthModeLocalImport
				},
			},
			stubUsageDriver{
				name: "builtin_openai_compatible",
				supports: func(account accounts.Account) bool {
					return account.ProviderType == accounts.ProviderOpenAICompatible
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	official := accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AuthMode:     accounts.AuthModeLocalImport,
	}

	accountDriver, err := reg.AccountDriverFor(official)
	if err != nil {
		t.Fatalf("AccountDriverFor returned error: %v", err)
	}
	if accountDriver.Name() != "official-session" {
		t.Fatalf("AccountDriverFor = %q, want %q", accountDriver.Name(), "official-session")
	}

	usageDriver, err := reg.UsageDriverFor(official)
	if err != nil {
		t.Fatalf("UsageDriverFor returned error: %v", err)
	}
	if usageDriver.Name() != "builtin_openai_official" {
		t.Fatalf("UsageDriverFor = %q, want %q", usageDriver.Name(), "builtin_openai_official")
	}
}

func TestRegistryFallbackUsesFirstSupportingDriverByRegistrationOrder(t *testing.T) {
	t.Parallel()

	reg, err := registry.New(
		[]accountdrv.AccountDriver{
			stubAccountDriver{
				name:     "first",
				supports: func(accounts.Account) bool { return true },
			},
			stubAccountDriver{
				name:     "second",
				supports: func(accounts.Account) bool { return true },
			},
		},
		[]usagedrv.UsageDriver{
			stubUsageDriver{
				name:     "builtin-first",
				supports: func(accounts.Account) bool { return true },
			},
			stubUsageDriver{
				name:     "builtin-second",
				supports: func(accounts.Account) bool { return true },
			},
		},
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	account := accounts.Account{ProviderType: accounts.ProviderOpenAICompatible, AuthMode: accounts.AuthModeAPIKey}

	accountDriver, err := reg.AccountDriverFor(account)
	if err != nil {
		t.Fatalf("AccountDriverFor returned error: %v", err)
	}
	if accountDriver.Name() != "first" {
		t.Fatalf("AccountDriverFor = %q, want %q", accountDriver.Name(), "first")
	}

	usageDriver, err := reg.UsageDriverFor(account)
	if err != nil {
		t.Fatalf("UsageDriverFor returned error: %v", err)
	}
	if usageDriver.Name() != "builtin-first" {
		t.Fatalf("UsageDriverFor = %q, want %q", usageDriver.Name(), "builtin-first")
	}
}

func TestRegistryReturnsErrorsWhenNoDriverMatches(t *testing.T) {
	t.Parallel()

	reg, err := registry.New(nil, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	account := accounts.Account{
		ProviderType: accounts.ProviderOpenAICompatible,
		AuthMode:     accounts.AuthModeAPIKey,
	}

	if _, err := reg.AccountDriverFor(account); err == nil {
		t.Fatal("AccountDriverFor returned nil error, want error")
	}
	if _, err := reg.UsageDriverFor(account); err == nil {
		t.Fatal("UsageDriverFor returned nil error, want error")
	}
}

func TestRegistryNewRejectsDuplicateDriverNames(t *testing.T) {
	t.Parallel()

	if _, err := registry.New(
		[]accountdrv.AccountDriver{
			stubAccountDriver{name: "apikey", supports: func(accounts.Account) bool { return true }},
			stubAccountDriver{name: "apikey", supports: func(accounts.Account) bool { return false }},
		},
		nil,
	); err == nil {
		t.Fatal("New returned nil error for duplicate account driver names")
	}

	if _, err := registry.New(
		nil,
		[]usagedrv.UsageDriver{
			stubUsageDriver{name: "lua", supports: func(accounts.Account) bool { return true }},
			stubUsageDriver{name: "lua", supports: func(accounts.Account) bool { return false }},
		},
	); err == nil {
		t.Fatal("New returned nil error for duplicate usage driver names")
	}
}

func TestRegistryReturnsErrorsWhenExplicitDriverNamesAreMissing(t *testing.T) {
	t.Parallel()

	reg, err := registry.New(nil, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	account := accounts.Account{
		AccountDriver: "official-session",
		UsageDriver:   "lua",
	}

	if _, err := reg.AccountDriverFor(account); err == nil {
		t.Fatal("AccountDriverFor returned nil error for missing explicit account driver")
	}
	if _, err := reg.UsageDriverFor(account); err == nil {
		t.Fatal("UsageDriverFor returned nil error for missing explicit usage driver")
	}
}
