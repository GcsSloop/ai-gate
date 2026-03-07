package accounts

import (
	"database/sql"
	"fmt"
	"time"
)

type Repository interface {
	Create(account Account) error
	List() ([]Account, error)
	UpdateStatus(id int64, status Status) error
}

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func (r *SQLiteRepository) Create(account Account) error {
	_, err := r.db.Exec(
		`INSERT INTO accounts (provider_type, account_name, auth_mode, credential_ref, base_url, status, priority, cooldown_until)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		account.ProviderType,
		account.AccountName,
		account.AuthMode,
		account.CredentialRef,
		account.BaseURL,
		account.Status,
		account.Priority,
		nullTime(account.CooldownUntil),
	)
	if err != nil {
		return fmt.Errorf("insert account: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) List() ([]Account, error) {
	records, err := r.db.Query(
		`SELECT id, provider_type, account_name, auth_mode, credential_ref, base_url, status, priority, cooldown_until, created_at
		 FROM accounts
		 ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer records.Close()

	var accounts []Account
	for records.Next() {
		var account Account
		var cooldown sql.NullTime

		if err := records.Scan(
			&account.ID,
			&account.ProviderType,
			&account.AccountName,
			&account.AuthMode,
			&account.CredentialRef,
			&account.BaseURL,
			&account.Status,
			&account.Priority,
			&cooldown,
			&account.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}

		if cooldown.Valid {
			value := cooldown.Time.UTC()
			account.CooldownUntil = &value
		}

		accounts = append(accounts, account)
	}

	if err := records.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}

	return accounts, nil
}

func (r *SQLiteRepository) UpdateStatus(id int64, status Status) error {
	_, err := r.db.Exec(`UPDATE accounts SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("update account status: %w", err)
	}
	return nil
}

func nullTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value.UTC(), Valid: true}
}
