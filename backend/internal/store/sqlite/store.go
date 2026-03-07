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
	return nil
}
