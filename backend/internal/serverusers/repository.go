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
		`INSERT INTO server_users (name, token_hash, status, created_at) VALUES (?, ?, ?, ?)`,
		normalized, HashToken(token), StatusActive, now,
	)
	if err != nil {
		return CreatedUser{}, fmt.Errorf("insert server user: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return CreatedUser{}, fmt.Errorf("read server user id: %w", err)
	}
	return CreatedUser{User: User{ID: id, Name: normalized, Status: StatusActive, CreatedAt: now}, Token: token}, nil
}

func (r *Repository) List() ([]User, error) {
	rows, err := r.db.Query(
		`SELECT u.id, u.name, u.token_hash, u.status, u.created_at, u.last_used_at,
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
		if err := rows.Scan(&user.ID, &user.Name, &user.TokenHash, &user.Status, &user.CreatedAt, nullTimeDest(&user.LastUsedAt), &user.RequestCount, &user.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan server user: %w", err)
		}
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
		`SELECT id, name, token_hash, status, created_at, last_used_at FROM server_users WHERE token_hash = ?`,
		hash,
	).Scan(&user.ID, &user.Name, &user.TokenHash, &user.Status, &user.CreatedAt, nullTimeDest(&user.LastUsedAt))
	if err != nil {
		return User{}, fmt.Errorf("select server user by token: %w", err)
	}
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
