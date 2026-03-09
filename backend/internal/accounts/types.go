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

type Account struct {
	ID                int64
	ProviderType      ProviderType
	AccountName       string
	AuthMode          AuthMode
	CredentialRef     string
	BaseURL           string
	Status            Status
	Priority          int
	IsActive          bool
	SupportsResponses bool
	CooldownUntil     *time.Time
	CreatedAt         time.Time
}

func (a Account) NativeResponsesCapable() bool {
	return a.SupportsResponses || a.ProviderType == ProviderOpenAIOfficial || a.AuthMode == AuthModeLocalImport
}
