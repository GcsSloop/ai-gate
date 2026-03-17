package registry

import (
	"fmt"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
)

type Registry struct {
	accountDrivers []accountdrv.AccountDriver
	usageDrivers   []usagedrv.UsageDriver
}

func New(accountDrivers []accountdrv.AccountDriver, usageDrivers []usagedrv.UsageDriver) (*Registry, error) {
	if err := ensureUniqueAccountDriverNames(accountDrivers); err != nil {
		return nil, err
	}
	if err := ensureUniqueUsageDriverNames(usageDrivers); err != nil {
		return nil, err
	}
	return &Registry{
		accountDrivers: append([]accountdrv.AccountDriver(nil), accountDrivers...),
		usageDrivers:   append([]usagedrv.UsageDriver(nil), usageDrivers...),
	}, nil
}

func (r *Registry) AccountDriverFor(account accounts.Account) (accountdrv.AccountDriver, error) {
	if account.AccountDriver != "" {
		for _, driver := range r.accountDrivers {
			if driver.Name() == account.AccountDriver {
				return driver, nil
			}
		}
		return nil, fmt.Errorf("account driver %q not registered", account.AccountDriver)
	}

	for _, driver := range r.accountDrivers {
		if driver.Supports(account) {
			// Fallback selection is deterministic: the first registered supporting
			// driver wins. Callers should use AccountDriver for explicit overrides.
			return driver, nil
		}
	}

	return nil, fmt.Errorf("no account driver registered for provider=%s auth_mode=%s", account.ProviderType, account.AuthMode)
}

func (r *Registry) UsageDriverFor(account accounts.Account) (usagedrv.UsageDriver, error) {
	if account.UsageDriver != "" {
		for _, driver := range r.usageDrivers {
			if driver.Name() == account.UsageDriver {
				return driver, nil
			}
		}
		return nil, fmt.Errorf("usage driver %q not registered", account.UsageDriver)
	}

	for _, driver := range r.usageDrivers {
		if driver.Supports(account) {
			// Fallback selection is deterministic: the first registered supporting
			// driver wins. Callers should use UsageDriver for explicit overrides.
			return driver, nil
		}
	}

	return nil, fmt.Errorf("no usage driver registered for provider=%s auth_mode=%s", account.ProviderType, account.AuthMode)
}

func ensureUniqueAccountDriverNames(drivers []accountdrv.AccountDriver) error {
	seen := make(map[string]struct{}, len(drivers))
	for _, driver := range drivers {
		name := driver.Name()
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate account driver %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func ensureUniqueUsageDriverNames(drivers []usagedrv.UsageDriver) error {
	seen := make(map[string]struct{}, len(drivers))
	for _, driver := range drivers {
		name := driver.Name()
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate usage driver %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
