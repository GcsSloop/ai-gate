package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) HasTable(name string) (bool, error) {
	const query = `SELECT EXISTS(
		SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?
	);`

	var exists bool
	if err := s.db.QueryRow(query, name).Scan(&exists); err != nil {
		return false, fmt.Errorf("check table %q: %w", name, err)
	}

	return exists, nil
}

func (s *Store) migrate() error {
	for _, statement := range schemaStatements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("run migration: %w", err)
		}
	}
	for _, column := range []struct {
		table      string
		name       string
		definition string
	}{
		{table: "messages", name: "item_type", definition: "TEXT NOT NULL DEFAULT 'message'"},
		{table: "messages", name: "raw_item_json", definition: "TEXT NOT NULL DEFAULT ''"},
		{table: "messages", name: "content_preview", definition: "TEXT NOT NULL DEFAULT ''"},
		{table: "messages", name: "content_sha256", definition: "TEXT NOT NULL DEFAULT ''"},
		{table: "messages", name: "content_bytes", definition: "INTEGER NOT NULL DEFAULT 0"},
		{table: "messages", name: "raw_preview", definition: "TEXT NOT NULL DEFAULT ''"},
		{table: "messages", name: "raw_sha256", definition: "TEXT NOT NULL DEFAULT ''"},
		{table: "messages", name: "raw_bytes", definition: "INTEGER NOT NULL DEFAULT 0"},
		{table: "messages", name: "tool_name", definition: "TEXT NOT NULL DEFAULT ''"},
		{table: "messages", name: "tool_call_id", definition: "TEXT NOT NULL DEFAULT ''"},
		{table: "messages", name: "summary_json", definition: "TEXT NOT NULL DEFAULT ''"},
		{table: "messages", name: "storage_mode", definition: "TEXT NOT NULL DEFAULT 'full'"},
		{table: "account_usage_snapshots", name: "health_score", definition: "REAL"},
		{table: "account_usage_snapshots", name: "throttled_recently", definition: "INTEGER NOT NULL DEFAULT 0"},
		{table: "account_usage_snapshots", name: "last_total_tokens", definition: "REAL"},
		{table: "account_usage_snapshots", name: "last_input_tokens", definition: "REAL"},
		{table: "account_usage_snapshots", name: "last_output_tokens", definition: "REAL"},
		{table: "account_usage_snapshots", name: "model_context_window", definition: "REAL"},
		{table: "account_usage_snapshots", name: "primary_used_percent", definition: "REAL"},
		{table: "account_usage_snapshots", name: "secondary_used_percent", definition: "REAL"},
		{table: "account_usage_snapshots", name: "primary_resets_at", definition: "DATETIME"},
		{table: "account_usage_snapshots", name: "secondary_resets_at", definition: "DATETIME"},
		{table: "accounts", name: "is_active", definition: "INTEGER NOT NULL DEFAULT 0"},
		{table: "accounts", name: "supports_responses", definition: "INTEGER NOT NULL DEFAULT 0"},
		{table: "accounts", name: "source_icon", definition: "TEXT NOT NULL DEFAULT 'openai'"},
		{table: "app_settings", name: "show_home_update_indicator", definition: "INTEGER NOT NULL DEFAULT 1"},
		{table: "app_settings", name: "status_refresh_interval_seconds", definition: "INTEGER NOT NULL DEFAULT 60"},
		{table: "app_settings", name: "audit_limit_message", definition: "INTEGER NOT NULL DEFAULT 200"},
		{table: "app_settings", name: "audit_limit_function_call", definition: "INTEGER NOT NULL DEFAULT 100"},
		{table: "app_settings", name: "audit_limit_function_call_output", definition: "INTEGER NOT NULL DEFAULT 100"},
		{table: "app_settings", name: "audit_limit_reasoning", definition: "INTEGER NOT NULL DEFAULT 40"},
		{table: "app_settings", name: "audit_limit_custom_tool_call", definition: "INTEGER NOT NULL DEFAULT 100"},
		{table: "app_settings", name: "audit_limit_custom_tool_call_output", definition: "INTEGER NOT NULL DEFAULT 100"},
		{table: "app_settings", name: "language", definition: "TEXT NOT NULL DEFAULT 'zh-CN'"},
		{table: "app_settings", name: "theme_mode", definition: "TEXT NOT NULL DEFAULT 'system'"},
		{table: "runs", name: "model", definition: "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := s.addColumnIfMissing(column.table, column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) addColumnIfMissing(table string, column string, definition string) error {
	exists, err := s.hasColumn(table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}

func (s *Store) hasColumn(table string, column string) (bool, error) {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("inspect table %s columns: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scan table info for %s: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate table info for %s: %w", table, err)
	}
	return false, nil
}
