package accounts

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/secrets"
)

type Repository interface {
	Create(account Account) error
	List() ([]Account, error)
	GetByID(id int64) (Account, error)
	Update(account Account) error
	Delete(id int64) error
	SetActive(id int64) error
	UpdateStatus(id int64, status Status) error
	UpdateCooldown(id int64, until *time.Time) error
}

type SQLiteRepository struct {
	db     *sql.DB
	cipher *secrets.Cipher
}

func NewSQLiteRepository(db *sql.DB, cipher ...*secrets.Cipher) *SQLiteRepository {
	repo := &SQLiteRepository{db: db}
	if len(cipher) > 0 {
		repo.cipher = cipher[0]
	}
	return repo
}

func (r *SQLiteRepository) Create(account Account) error {
	credentialRef, err := r.encrypt(account.CredentialRef)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(
		`INSERT INTO accounts (provider_type, account_name, auth_mode, credential_ref, base_url, status, priority, is_active, allow_chat_fallback, cooldown_until)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		account.ProviderType,
		account.AccountName,
		account.AuthMode,
		credentialRef,
		account.BaseURL,
		account.Status,
		account.Priority,
		boolToInt(account.IsActive),
		boolToInt(account.AllowChatFallback),
		nullTime(account.CooldownUntil),
	)
	if err != nil {
		return fmt.Errorf("insert account: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) List() ([]Account, error) {
	records, err := r.db.Query(
		`SELECT id, provider_type, account_name, auth_mode, credential_ref, base_url, status, priority, is_active, allow_chat_fallback, cooldown_until, created_at
		 FROM accounts
		 ORDER BY priority DESC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer records.Close()

	var accounts []Account
	for records.Next() {
		var account Account
		var cooldown sql.NullTime
		var isActive int
		var allowChatFallback int

		if err := records.Scan(
			&account.ID,
			&account.ProviderType,
			&account.AccountName,
			&account.AuthMode,
			&account.CredentialRef,
			&account.BaseURL,
			&account.Status,
			&account.Priority,
			&isActive,
			&allowChatFallback,
			&cooldown,
			&account.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		account.IsActive = isActive == 1
		account.AllowChatFallback = allowChatFallback == 1

		if cooldown.Valid {
			value := cooldown.Time.UTC()
			account.CooldownUntil = &value
		}
		account.CredentialRef, err = r.decrypt(account.CredentialRef)
		if err != nil {
			return nil, err
		}

		accounts = append(accounts, account)
	}

	if err := records.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}

	return accounts, nil
}

func (r *SQLiteRepository) GetByID(id int64) (Account, error) {
	row := r.db.QueryRow(
		`SELECT id, provider_type, account_name, auth_mode, credential_ref, base_url, status, priority, is_active, allow_chat_fallback, cooldown_until, created_at
		 FROM accounts WHERE id = ?`,
		id,
	)

	var account Account
	var cooldown sql.NullTime
	var isActive int
	var allowChatFallback int
	if err := row.Scan(
		&account.ID,
		&account.ProviderType,
		&account.AccountName,
		&account.AuthMode,
		&account.CredentialRef,
		&account.BaseURL,
		&account.Status,
		&account.Priority,
		&isActive,
		&allowChatFallback,
		&cooldown,
		&account.CreatedAt,
	); err != nil {
		return Account{}, fmt.Errorf("select account: %w", err)
	}
	account.IsActive = isActive == 1
	account.AllowChatFallback = allowChatFallback == 1
	if cooldown.Valid {
		value := cooldown.Time.UTC()
		account.CooldownUntil = &value
	}
	var err error
	account.CredentialRef, err = r.decrypt(account.CredentialRef)
	if err != nil {
		return Account{}, err
	}
	return account, nil
}

func (r *SQLiteRepository) Update(account Account) error {
	credentialRef, err := r.encrypt(account.CredentialRef)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(
		`UPDATE accounts
		 SET account_name = ?, base_url = ?, credential_ref = ?, status = ?, priority = ?, is_active = ?, allow_chat_fallback = ?, cooldown_until = ?
		 WHERE id = ?`,
		account.AccountName,
		account.BaseURL,
		credentialRef,
		account.Status,
		account.Priority,
		boolToInt(account.IsActive),
		boolToInt(account.AllowChatFallback),
		nullTime(account.CooldownUntil),
		account.ID,
	)
	if err != nil {
		return fmt.Errorf("update account: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) SetActive(id int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin set active transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`UPDATE accounts SET is_active = 0 WHERE is_active = 1`); err != nil {
		return fmt.Errorf("reset active account: %w", err)
	}
	result, err := tx.Exec(`UPDATE accounts SET is_active = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("set active account: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read set active rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("set active account: id %d not found", id)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit set active transaction: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) encrypt(value string) (string, error) {
	if r.cipher == nil || value == "" {
		return value, nil
	}

	encrypted, err := r.cipher.EncryptString(value)
	if err != nil {
		return "", fmt.Errorf("encrypt credential_ref: %w", err)
	}
	return encrypted, nil
}

func (r *SQLiteRepository) decrypt(value string) (string, error) {
	if r.cipher == nil || value == "" {
		return value, nil
	}

	// Backward compatibility: rows created before credential encryption was enabled
	// still contain plaintext values. If the payload is not valid base64, treat it as
	// legacy plaintext instead of failing the account list endpoint.
	if _, err := base64.StdEncoding.DecodeString(value); err != nil {
		return value, nil
	}

	decrypted, err := r.cipher.DecryptString(value)
	if err != nil {
		return "", fmt.Errorf("decrypt credential_ref: %w", err)
	}
	return decrypted, nil
}

func (r *SQLiteRepository) UpdateStatus(id int64, status Status) error {
	_, err := r.db.Exec(`UPDATE accounts SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("update account status: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) UpdateCooldown(id int64, until *time.Time) error {
	_, err := r.db.Exec(`UPDATE accounts SET cooldown_until = ? WHERE id = ?`, nullTime(until), id)
	if err != nil {
		return fmt.Errorf("update account cooldown: %w", err)
	}
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value.UTC(), Valid: true}
}
