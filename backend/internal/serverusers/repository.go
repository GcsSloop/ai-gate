package serverusers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(name string) (CreatedUser, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return CreatedUser{}, fmt.Errorf("user name is required")
	}
	token, err := newToken()
	if err != nil {
		return CreatedUser{}, err
	}
	now := time.Now().UTC()
	result, err := r.db.Exec(
		`INSERT INTO server_users (name, username, token_hash, role, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		normalized, normalized, HashToken(token), RoleUser, StatusActive, now,
	)
	if err != nil {
		return CreatedUser{}, fmt.Errorf("insert server user: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return CreatedUser{}, fmt.Errorf("read server user id: %w", err)
	}
	return CreatedUser{User: User{ID: id, Name: normalized, Username: normalized, Role: RoleUser, Status: StatusActive, CreatedAt: now}, Token: token}, nil
}

func (r *Repository) List() ([]User, error) {
	rows, err := r.db.Query(
		`SELECT u.id, u.name, u.username, u.token_hash, u.role, u.status, u.created_at, u.last_used_at,
			COUNT(e.id) AS request_count, COALESCE(SUM(e.total_tokens), 0) AS total_tokens
		 FROM server_users u
		 LEFT JOIN usage_events e ON e.server_user_id = u.id
		 GROUP BY u.id
		 ORDER BY u.id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query server users: %w", err)
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Name, &user.Username, &user.TokenHash, &user.Role, &user.Status, &user.CreatedAt, nullTimeDest(&user.LastUsedAt), &user.RequestCount, &user.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan server user: %w", err)
		}
		normalizeUserFields(&user)
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate server users: %w", err)
	}
	return users, nil
}

func (r *Repository) Authenticate(token string) (User, error) {
	hash := HashToken(token)
	var user User
	err := r.db.QueryRow(
		`SELECT id, name, username, token_hash, role, status, created_at, last_used_at FROM server_users WHERE token_hash = ?`,
		hash,
	).Scan(&user.ID, &user.Name, &user.Username, &user.TokenHash, &user.Role, &user.Status, &user.CreatedAt, nullTimeDest(&user.LastUsedAt))
	if err != nil {
		return User{}, fmt.Errorf("select server user by token: %w", err)
	}
	normalizeUserFields(&user)
	if user.Status != StatusActive {
		return User{}, fmt.Errorf("server user is disabled")
	}
	now := time.Now().UTC()
	_, _ = r.db.Exec(`UPDATE server_users SET last_used_at = ? WHERE id = ?`, now, user.ID)
	user.LastUsedAt = &now
	return user, nil
}

func (r *Repository) AuthenticateLogin(username string, token string) (User, error) {
	normalized := strings.TrimSpace(username)
	if normalized == "" {
		return User{}, fmt.Errorf("username is required")
	}
	hash := HashToken(token)
	var user User
	err := r.db.QueryRow(
		`SELECT id, name, username, token_hash, role, status, created_at, last_used_at
		 FROM server_users
		 WHERE username = ? AND token_hash = ?`,
		normalized,
		hash,
	).Scan(&user.ID, &user.Name, &user.Username, &user.TokenHash, &user.Role, &user.Status, &user.CreatedAt, nullTimeDest(&user.LastUsedAt))
	if err != nil {
		return User{}, fmt.Errorf("select server user by username and token: %w", err)
	}
	normalizeUserFields(&user)
	if user.Status != StatusActive {
		return User{}, fmt.Errorf("server user is disabled")
	}
	now := time.Now().UTC()
	_, _ = r.db.Exec(`UPDATE server_users SET last_used_at = ? WHERE id = ?`, now, user.ID)
	user.LastUsedAt = &now
	return user, nil
}

func (r *Repository) Disable(id int64) error {
	result, err := r.db.Exec(`UPDATE server_users SET status = ? WHERE id = ?`, StatusDisabled, id)
	if err != nil {
		return fmt.Errorf("disable server user: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read disabled server user rows: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) RotateToken(id int64) (CreatedUser, error) {
	token, err := newToken()
	if err != nil {
		return CreatedUser{}, err
	}
	result, err := r.db.Exec(`UPDATE server_users SET token_hash = ?, status = ? WHERE id = ?`, HashToken(token), StatusActive, id)
	if err != nil {
		return CreatedUser{}, fmt.Errorf("rotate server user token: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return CreatedUser{}, fmt.Errorf("read rotated server user rows: %w", err)
	}
	if affected == 0 {
		return CreatedUser{}, sql.ErrNoRows
	}
	users, err := r.List()
	if err != nil {
		return CreatedUser{}, err
	}
	for _, user := range users {
		if user.ID == id {
			return CreatedUser{User: user, Token: token}, nil
		}
	}
	return CreatedUser{}, sql.ErrNoRows
}

func (r *Repository) ListAssignedAccounts(userID int64) ([]AssignedAccount, error) {
	rows, err := r.db.Query(
		`SELECT sua.user_id, a.id, a.account_name, a.provider_type, a.source_icon, a.base_url,
			a.status, sua.position, sua.is_active, sua.is_locked, a.supports_responses, a.cooldown_until, a.cooldown_reason
		 FROM server_user_accounts sua
		 JOIN accounts a ON a.id = sua.account_id
		 WHERE sua.user_id = ?
		 ORDER BY sua.position ASC, a.id ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query assigned accounts: %w", err)
	}
	defer rows.Close()

	assigned := make([]AssignedAccount, 0)
	for rows.Next() {
		var item AssignedAccount
		var isActive int
		var isLocked int
		var supportsResponses int
		var cooldown sql.NullTime
		if err := rows.Scan(
			&item.UserID,
			&item.AccountID,
			&item.AccountName,
			&item.ProviderType,
			&item.SourceIcon,
			&item.BaseURL,
			&item.Status,
			&item.Position,
			&isActive,
			&isLocked,
			&supportsResponses,
			&cooldown,
			&item.CooldownReason,
		); err != nil {
			return nil, fmt.Errorf("scan assigned account: %w", err)
		}
		item.IsActive = isActive == 1
		item.IsLocked = isLocked == 1
		item.SupportsResponses = supportsResponses == 1
		if cooldown.Valid {
			value := cooldown.Time.UTC()
			item.CooldownUntil = &value
		}
		assigned = append(assigned, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate assigned accounts: %w", err)
	}
	return assigned, nil
}

func (r *Repository) SetAccountAssignments(userID int64, accountIDs []int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin account assignment transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec(`DELETE FROM server_user_accounts WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("clear account assignments: %w", err)
	}
	now := time.Now().UTC()
	seen := map[int64]bool{}
	for position, accountID := range accountIDs {
		if accountID <= 0 || seen[accountID] {
			continue
		}
		seen[accountID] = true
		if _, err := tx.Exec(
			`INSERT INTO server_user_accounts (user_id, account_id, position, is_active, is_locked, created_at, updated_at)
			 VALUES (?, ?, ?, 0, 0, ?, ?)`,
			userID,
			accountID,
			position,
			now,
			now,
		); err != nil {
			return fmt.Errorf("insert account assignment: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit account assignments: %w", err)
	}
	return nil
}

func (r *Repository) UpdateAccountState(userID int64, accountID int64, update AccountStateUpdate) error {
	result, err := r.db.Exec(
		`UPDATE server_user_accounts
		 SET position = ?, is_active = ?, is_locked = ?, updated_at = ?
		 WHERE user_id = ? AND account_id = ?`,
		update.Position,
		boolToInt(update.IsActive),
		boolToInt(update.IsLocked),
		time.Now().UTC(),
		userID,
		accountID,
	)
	if err != nil {
		return fmt.Errorf("update assigned account state: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read assigned account state rows: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "agt_" + hex.EncodeToString(raw), nil
}

func normalizeUserFields(user *User) {
	if user.Username == "" {
		user.Username = user.Name
	}
	if user.Name == "" {
		user.Name = user.Username
	}
	if user.Role == "" {
		user.Role = RoleUser
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullTimeDest(target **time.Time) any {
	return &nullableTimeScanner{target: target}
}

type nullableTimeScanner struct {
	target **time.Time
}

func (s *nullableTimeScanner) Scan(value any) error {
	if value == nil {
		*s.target = nil
		return nil
	}
	switch typed := value.(type) {
	case time.Time:
		copy := typed.UTC()
		*s.target = &copy
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, typed)
		if err != nil {
			parsed, err = time.Parse("2006-01-02 15:04:05", typed)
			if err != nil {
				return err
			}
		}
		parsed = parsed.UTC()
		*s.target = &parsed
	case []byte:
		return s.Scan(string(typed))
	default:
		return fmt.Errorf("unsupported nullable time type %T", value)
	}
	return nil
}
