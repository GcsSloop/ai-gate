package serverusers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Repository struct {
	db *sql.DB
}

const lastUsedAtUpdateInterval = time.Minute

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
			u.preferred_account_id, u.route_locked,
			COALESCE(usage_stats.request_count, 0) AS request_count,
			COALESCE(usage_stats.total_tokens, 0) AS total_tokens
		 FROM server_users u
		 LEFT JOIN (
			SELECT server_user_id, COUNT(id) AS request_count, COALESCE(SUM(total_tokens), 0) AS total_tokens
			FROM usage_events
			GROUP BY server_user_id
		 ) usage_stats ON usage_stats.server_user_id = u.id
		 ORDER BY u.id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query server users: %w", err)
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		var user User
		var routeLocked int
		if err := rows.Scan(&user.ID, &user.Name, &user.Username, &user.TokenHash, &user.Role, &user.Status, &user.CreatedAt, nullTimeDest(&user.LastUsedAt), nullInt64Dest(&user.PreferredAccountID), &routeLocked, &user.RequestCount, &user.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan server user: %w", err)
		}
		user.RouteLocked = routeLocked == 1
		normalizeUserFields(&user)
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate server users: %w", err)
	}
	return users, nil
}

func (r *Repository) Get(id int64) (User, error) {
	users, err := r.List()
	if err != nil {
		return User{}, err
	}
	for _, user := range users {
		if user.ID == id {
			return user, nil
		}
	}
	return User{}, sql.ErrNoRows
}

func (r *Repository) Authenticate(token string) (User, error) {
	hash := HashToken(token)
	var user User
	var routeLocked int
	err := r.db.QueryRow(
		`SELECT id, name, username, token_hash, role, status, created_at, last_used_at, preferred_account_id, route_locked FROM server_users WHERE token_hash = ?`,
		hash,
	).Scan(&user.ID, &user.Name, &user.Username, &user.TokenHash, &user.Role, &user.Status, &user.CreatedAt, nullTimeDest(&user.LastUsedAt), nullInt64Dest(&user.PreferredAccountID), &routeLocked)
	if err != nil {
		return User{}, fmt.Errorf("select server user by token: %w", err)
	}
	user.RouteLocked = routeLocked == 1
	normalizeUserFields(&user)
	if user.Status != StatusActive {
		return User{}, fmt.Errorf("server user is disabled")
	}
	user.LastUsedAt = r.touchLastUsedAt(user.ID, user.LastUsedAt)
	return user, nil
}

func (r *Repository) AuthenticateLogin(username string, token string) (User, error) {
	normalized := strings.TrimSpace(username)
	if normalized == "" {
		return User{}, fmt.Errorf("username is required")
	}
	hash := HashToken(token)
	var user User
	var routeLocked int
	err := r.db.QueryRow(
		`SELECT id, name, username, token_hash, role, status, created_at, last_used_at, preferred_account_id, route_locked
		 FROM server_users
		 WHERE username = ? AND token_hash = ?`,
		normalized,
		hash,
	).Scan(&user.ID, &user.Name, &user.Username, &user.TokenHash, &user.Role, &user.Status, &user.CreatedAt, nullTimeDest(&user.LastUsedAt), nullInt64Dest(&user.PreferredAccountID), &routeLocked)
	if err != nil {
		return User{}, fmt.Errorf("select server user by username and token: %w", err)
	}
	user.RouteLocked = routeLocked == 1
	normalizeUserFields(&user)
	if user.Status != StatusActive {
		return User{}, fmt.Errorf("server user is disabled")
	}
	user.LastUsedAt = r.touchLastUsedAt(user.ID, user.LastUsedAt)
	return user, nil
}

func (r *Repository) touchLastUsedAt(userID int64, current *time.Time) *time.Time {
	now := time.Now().UTC()
	if current != nil && now.Sub(current.UTC()) < lastUsedAtUpdateInterval {
		return current
	}
	_, _ = r.db.Exec(`UPDATE server_users SET last_used_at = ? WHERE id = ?`, now, userID)
	return &now
}

func (r *Repository) UpdateRoute(id int64, accountID *int64, locked bool) (User, error) {
	result, err := r.db.Exec(`UPDATE server_users SET preferred_account_id = ?, route_locked = ? WHERE id = ?`, nullInt64(accountID), boolToInt(locked), id)
	if err != nil {
		return User{}, fmt.Errorf("update server user route: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return User{}, fmt.Errorf("read updated server user route rows: %w", err)
	}
	if affected == 0 {
		return User{}, sql.ErrNoRows
	}
	return r.Get(id)
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

func (r *Repository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM server_users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete server user: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted server user rows: %w", err)
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

func nullTimeDest(target **time.Time) any {
	return &nullableTimeScanner{target: target}
}

func nullInt64(value *int64) any {
	if value == nil || *value <= 0 {
		return nil
	}
	return *value
}

func nullInt64Dest(target **int64) any {
	return &nullableInt64Scanner{target: target}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type nullableInt64Scanner struct {
	target **int64
}

func (s *nullableInt64Scanner) Scan(value any) error {
	if value == nil {
		*s.target = nil
		return nil
	}
	switch typed := value.(type) {
	case int64:
		copy := typed
		*s.target = &copy
	case int:
		copy := int64(typed)
		*s.target = &copy
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		if err != nil {
			return err
		}
		*s.target = &parsed
	case []byte:
		return s.Scan(string(typed))
	default:
		return fmt.Errorf("unsupported nullable int64 type %T", value)
	}
	return nil
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
