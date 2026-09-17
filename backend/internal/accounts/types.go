package accounts

import "time"

type ProviderType string

const (
	ProviderOpenAIOfficial   ProviderType = "openai-official"
	ProviderOpenAICompatible ProviderType = "openai-compatible"
)

type AuthMode string

const (
	AuthModeOAuth       AuthMode = "oauth"
	AuthModeAPIKey      AuthMode = "api_key"
	AuthModeLocalImport AuthMode = "codex_local_import"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusCooldown Status = "cooldown"
	StatusDegraded Status = "degraded"
	StatusInvalid  Status = "invalid"
	StatusDisabled Status = "disabled"
)

// ProxyMode is the per-account override for the global upstream proxy setting.
type ProxyMode string

const (
	// ProxyModeInherit follows the global upstream proxy mode.
	ProxyModeInherit ProxyMode = ""
	// ProxyModeDirect never uses a proxy for this account.
	ProxyModeDirect ProxyMode = "direct"
	// ProxyModeProxy always uses a proxy for this account, reusing the global proxy address.
	ProxyModeProxy ProxyMode = "proxy"
)

func NormalizeProxyMode(raw string) ProxyMode {
	switch ProxyMode(raw) {
	case ProxyModeDirect, ProxyModeProxy:
		return ProxyMode(raw)
	default:
		return ProxyModeInherit
	}
}

type Account struct {
	ID                int64
	ProviderType      ProviderType
	AccountName       string
	SourceIcon        string
	AuthMode          AuthMode
	CredentialRef     string
	UsageDriver       string
	UsageConfigJSON   string
	AccountDriver     string
	BaseURL           string
	Status            Status
	Priority          int
	IsActive          bool
	IsLocked          bool
	SupportsResponses bool
	SkipTLSVerify     bool
	ProxyMode         ProxyMode
	CooldownUntil     *time.Time
	CooldownReason    string
	CreatedAt         time.Time
}

func (a Account) NativeResponsesCapable() bool {
	return a.SupportsResponses || a.ProviderType == ProviderOpenAIOfficial || a.AuthMode == AuthModeLocalImport
}

func (a Account) RoutingCooldownActive(now time.Time) bool {
	return a.CooldownUntil != nil && a.CooldownUntil.UTC().After(now.UTC())
}
