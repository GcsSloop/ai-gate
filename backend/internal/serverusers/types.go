package serverusers

import "time"

const StatusActive = "active"
const StatusDisabled = "disabled"

type User struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	TokenHash    string     `json:"-"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	RequestCount int64      `json:"request_count,omitempty"`
	TotalTokens  int64      `json:"total_tokens,omitempty"`
}

type CreatedUser struct {
	User  User   `json:"user"`
	Token string `json:"token"`
}
