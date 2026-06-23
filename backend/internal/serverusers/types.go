package serverusers

import "time"

const StatusActive = "active"
const StatusDisabled = "disabled"
const RoleUser = "user"
const RoleAdmin = "admin"

type User struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Username           string     `json:"username"`
	TokenHash          string     `json:"-"`
	Role               string     `json:"role"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
	PreferredAccountID *int64     `json:"preferred_account_id,omitempty"`
	RouteLocked        bool       `json:"route_locked"`
	RequestCount       int64      `json:"request_count,omitempty"`
	TotalTokens        int64      `json:"total_tokens,omitempty"`
}

type CreatedUser struct {
	User  User   `json:"user"`
	Token string `json:"token"`
}
