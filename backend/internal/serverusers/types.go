package serverusers

import "time"

const StatusActive = "active"
const StatusDisabled = "disabled"
const RoleUser = "user"
const RoleAdmin = "admin"

type User struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	Username         string     `json:"username"`
	TokenHash        string     `json:"-"`
	Role             string     `json:"role"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	RequestCount     int64      `json:"request_count,omitempty"`
	TotalTokens      int64      `json:"total_tokens,omitempty"`
	AssignedAccounts int64      `json:"assigned_accounts,omitempty"`
}

type CreatedUser struct {
	User  User   `json:"user"`
	Token string `json:"token"`
}

type AssignedAccount struct {
	UserID            int64      `json:"user_id"`
	AccountID         int64      `json:"account_id"`
	AccountName       string     `json:"account_name"`
	ProviderType      string     `json:"provider_type"`
	SourceIcon        string     `json:"source_icon"`
	BaseURL           string     `json:"base_url,omitempty"`
	Status            string     `json:"status"`
	Position          int        `json:"position"`
	IsActive          bool       `json:"is_active"`
	IsLocked          bool       `json:"is_locked"`
	SupportsResponses bool       `json:"supports_responses"`
	CooldownUntil     *time.Time `json:"cooldown_until,omitempty"`
	CooldownReason    string     `json:"cooldown_reason,omitempty"`
	CredentialRef     string     `json:"-"`
}

type AccountAssignment struct {
	AccountID         int64      `json:"account_id"`
	AccountName       string     `json:"account_name"`
	ProviderType      string     `json:"provider_type"`
	SourceIcon        string     `json:"source_icon"`
	BaseURL           string     `json:"base_url,omitempty"`
	Status            string     `json:"status"`
	Assigned          bool       `json:"assigned"`
	Position          int        `json:"position"`
	IsActive          bool       `json:"is_active"`
	IsLocked          bool       `json:"is_locked"`
	SupportsResponses bool       `json:"supports_responses"`
	CooldownUntil     *time.Time `json:"cooldown_until,omitempty"`
	CooldownReason    string     `json:"cooldown_reason,omitempty"`
}

type AccountStateUpdate struct {
	Position int  `json:"position"`
	IsActive bool `json:"is_active"`
	IsLocked bool `json:"is_locked"`
}
