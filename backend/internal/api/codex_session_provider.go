package api

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

type codexSessionMigration struct {
	MigratedSessions int    `json:"migrated_sessions"`
	ReloadRequired   bool   `json:"reload_required"`
	BackupPath       string `json:"backup_path,omitempty"`
}

type codexSessionMigrationSummary struct {
	MigratedSessions int  `json:"migrated_sessions"`
	ReloadRequired   bool `json:"reload_required"`
}

func summarizeCodexSessionMigration(value *codexSessionMigration) *codexSessionMigrationSummary {
	if value == nil {
		return nil
	}
	return &codexSessionMigrationSummary{MigratedSessions: value.MigratedSessions, ReloadRequired: value.ReloadRequired}
}

// The journal backs up only provider metadata. Replaying a whole database or rollout
// would discard messages Codex may have appended after the proxy was enabled.
type codexProviderJournal struct {
	CodexHome string                 `json:"codex_home"`
	Database  string                 `json:"database,omitempty"`
	ThreadIDs []string               `json:"thread_ids"`
	Rollouts  []codexProviderRollout `json:"rollouts"`
}

type codexProviderRollout struct {
	Path   string `json:"path"`
	ID     string `json:"id"`
	Header []byte `json:"original_header"`
}

type codexProviderHeader struct {
	ID       string
	Provider string
	Offset   int64
}

func codexStateDatabase(home string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(home, "state_*.sqlite"))
	if err != nil {
		return "", err
	}
	version, selected := -1, ""
	for _, path := range paths {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "state_"), ".sqlite")
		n, err := strconv.Atoi(name)
		if err == nil && n > version {
			version, selected = n, path
		}
	}
	return selected, nil
}

func openCodexStateDatabase(path string) (*sql.DB, error) {
	if path == "" {
		return nil, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Codex state database is not a regular file")
	}
	db, err := sql.Open("sqlite", codexStateDatabaseDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open Codex state database: %w", err)
	}
	db.SetMaxOpenConns(1)
	// sql.Open is lazy; report connection failures before querying thread metadata.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open Codex state database: %w", err)
	}
	return db, nil
}

func codexStateDatabaseDSN(path string) string {
	path = filepath.ToSlash(path)
	// A drive path needs a leading slash: file:///C:/..., not file://C:/... .
	// Otherwise SQLite treats the drive letter as an unsupported URI authority.
	if len(path) >= 3 && path[1] == ':' && path[2] == '/' {
		path = "/" + path
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "rw") // Never create or initialize a Codex-owned database.
	q.Set("_pragma", "busy_timeout(1500)")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	return u.String()
}

func jsonFieldSpan(raw []byte, name string) (int, []byte, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return 0, nil, fmt.Errorf("expected JSON object")
	}
	start := 0
	var found []byte
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return 0, nil, err
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return 0, nil, err
		}
		if key == name {
			start, found = int(d.InputOffset())-len(value), value
		}
	}
	if found == nil {
		return 0, nil, fmt.Errorf("missing JSON field %s", name)
	}
	return start, found, nil
}

// errEmptyCodexSession marks a rollout Codex created but never wrote to. Some
// aborted sessions leave a zero-byte file behind; it carries no provider to
// patch and must not block the migration of the remaining sessions.
var errEmptyCodexSession = errors.New("empty Codex session header")

func readCodexProviderHeader(file *os.File) ([]byte, codexProviderHeader, error) {
	// Session metadata can include instructions; cap reads without loading history.
	line, err := bufio.NewReader(io.LimitReader(file, 4<<20)).ReadBytes('\n')
	if len(bytes.TrimSpace(line)) == 0 {
		if err == nil || errors.Is(err, io.EOF) {
			return nil, codexProviderHeader{}, errEmptyCodexSession
		}
		return nil, codexProviderHeader{}, fmt.Errorf("read Codex session header: %w", err)
	}
	// A header without its trailing newline is still patchable when the JSON is
	// complete; only a partial write should fail.
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, codexProviderHeader{}, fmt.Errorf("read Codex session header: %w", err)
	}
	if !json.Valid(line) {
		return nil, codexProviderHeader{}, fmt.Errorf("invalid Codex session header")
	}
	base, payload, err := jsonFieldSpan(line, "payload")
	var kind string
	if _, raw, e := jsonFieldSpan(line, "type"); e == nil {
		_ = json.Unmarshal(raw, &kind)
	}
	if err != nil || kind != "session_meta" {
		outer, item, e := jsonFieldSpan(line, "item")
		if e != nil {
			return nil, codexProviderHeader{}, fmt.Errorf("missing Codex session metadata")
		}
		_, raw, e := jsonFieldSpan(item, "type")
		if e != nil || json.Unmarshal(raw, &kind) != nil || kind != "session_meta" {
			return nil, codexProviderHeader{}, fmt.Errorf("invalid Codex session metadata")
		}
		inner, meta, e := jsonFieldSpan(item, "meta")
		if e != nil {
			return nil, codexProviderHeader{}, e
		}
		base, payload = outer+inner, meta
	}
	var header codexProviderHeader
	_, rawID, err := jsonFieldSpan(payload, "id")
	if err != nil || json.Unmarshal(rawID, &header.ID) != nil || header.ID == "" {
		return nil, header, fmt.Errorf("missing Codex session ID")
	}
	offset, rawProvider, err := jsonFieldSpan(payload, "model_provider")
	if err != nil {
		return line, header, nil
	} // Older metadata may omit the optional provider.
	if err := json.Unmarshal(rawProvider, &header.Provider); err != nil {
		return nil, header, err
	}
	// Both provider IDs have the same encoded length. Reject escaped spellings rather
	// than shift bytes under Codex's open append file descriptors.
	if (header.Provider == "openai" || header.Provider == "aigate") && len(rawProvider) != len(`"openai"`) {
		return nil, header, fmt.Errorf("unsupported escaped Codex provider")
	}
	header.Offset = int64(base + offset)
	return line, header, nil
}

func localCodexPath(home, path string) bool {
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	// Do not follow directory or file symlinks out of the Codex home.
	current := home
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

func writeCodexProvider(home string, rollout codexProviderRollout, from, to string) (bool, error) {
	if !localCodexPath(home, rollout.Path) {
		return false, fmt.Errorf("Codex rollout path is unavailable or unsafe")
	}
	f, err := os.OpenFile(rollout.Path, os.O_RDWR, 0)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, header, err := readCodexProviderHeader(f)
	if err != nil {
		return false, err
	}
	if header.ID != rollout.ID {
		return false, fmt.Errorf("Codex session ID changed during migration")
	}
	if header.Provider != from {
		return false, nil
	} // Preserve later custom-provider edits.
	old := []byte(`"` + from + `"`)
	next := []byte(`"` + to + `"`)
	check := make([]byte, len(old))
	if _, err := f.ReadAt(check, header.Offset); err != nil {
		return false, err
	}
	if !bytes.Equal(check, old) {
		return false, fmt.Errorf("Codex provider changed during migration")
	}
	if _, err := f.WriteAt(next, header.Offset); err != nil {
		return false, err
	}
	return true, f.Sync()
}

func migrateCodexSessionProviders(home, backupDir string) (result codexSessionMigration, returnErr error) {
	journal := codexProviderJournal{CodexHome: home}
	path, err := codexStateDatabase(home)
	if err != nil {
		return result, err
	}
	journal.Database = path
	db, err := openCodexStateDatabase(path)
	if err != nil {
		return result, err
	}
	providers := map[string]string{}
	if db != nil {
		defer db.Close()
		rows, err := db.Query(`SELECT id, model_provider FROM threads`)
		if err != nil {
			return result, fmt.Errorf("read Codex thread providers: %w", err)
		}
		for rows.Next() {
			var id, provider string
			if err := rows.Scan(&id, &provider); err != nil {
				rows.Close()
				return result, err
			}
			providers[id] = provider
			if provider == "openai" {
				journal.ThreadIDs = append(journal.ThreadIDs, id)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
	}
	ids := map[string]bool{}
	for _, id := range journal.ThreadIDs {
		ids[id] = true
	}
	for _, root := range []string{"sessions", "archived_sessions"} {
		rootPath := filepath.Join(home, root)
		if _, err := os.Lstat(rootPath); errors.Is(err, os.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(rootPath, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("Codex session symlinks are unsupported")
			}
			if !strings.HasPrefix(entry.Name(), "rollout-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
				return nil
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			line, header, err := readCodexProviderHeader(f)
			f.Close()
			if errors.Is(err, errEmptyCodexSession) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("inspect Codex rollout %s: %w", filepath.Base(path), err)
			}
			if header.Provider == "" && providers[header.ID] == "openai" {
				return fmt.Errorf("Codex session %s has no patchable provider; close Codex and retry after updating its history", header.ID)
			}
			if provider := providers[header.ID]; provider != "" && provider != "openai" && provider != "aigate" {
				return nil
			}
			if header.Provider != "openai" {
				return nil
			}
			journal.Rollouts = append(journal.Rollouts, codexProviderRollout{Path: path, ID: header.ID, Header: line})
			ids[header.ID] = true
			return nil
		})
		if err != nil {
			return result, err
		}
	}
	if len(ids) == 0 {
		return codexSessionMigration{}, nil
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return result, err
	}
	manifest, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return result, err
	}
	backupPath := filepath.Join(backupDir, "manifest.json")
	if err := writeAtomic(backupPath, manifest, 0o600); err != nil {
		return result, err
	}
	var tx *sql.Tx
	if db != nil {
		tx, err = db.Begin()
		if err != nil {
			return result, fmt.Errorf("lock Codex state database; close Codex and retry: %w", err)
		}
		defer tx.Rollback()
	}
	var patched []codexProviderRollout
	defer func() {
		if returnErr == nil {
			return
		}
		for i := len(patched) - 1; i >= 0; i-- {
			_, restoreErr := writeCodexProvider(home, patched[i], "aigate", "openai")
			returnErr = errors.Join(returnErr, restoreErr)
		}
	}()
	if tx != nil {
		for _, id := range journal.ThreadIDs {
			res, err := tx.Exec(`UPDATE threads SET model_provider = 'aigate' WHERE id = ? AND model_provider = 'openai'`, id)
			if err != nil {
				return result, err
			}
			n, err := res.RowsAffected()
			if err != nil || n != 1 {
				return result, fmt.Errorf("Codex session provider changed during migration; retry")
			}
		}
	}
	for _, rollout := range journal.Rollouts {
		changed, err := writeCodexProvider(home, rollout, "openai", "aigate")
		if changed {
			patched = append(patched, rollout)
		}
		if err != nil {
			return result, err
		}
		if !changed {
			return result, fmt.Errorf("Codex session provider changed during migration; retry")
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return result, err
		}
	}
	return codexSessionMigration{MigratedSessions: len(ids), ReloadRequired: true, BackupPath: backupPath}, nil
}

func restoreCodexSessionProviders(home, backupPath string) (returnErr error) {
	if backupPath == "" {
		return nil
	}
	raw, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	var journal codexProviderJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return err
	}
	if journal.CodexHome != home {
		return fmt.Errorf("Codex provider backup belongs to another home")
	}
	if journal.Database != "" && (!localCodexPath(home, journal.Database) || filepath.Dir(journal.Database) != home) {
		return fmt.Errorf("Codex provider backup database is unavailable or unsafe")
	}
	db, err := openCodexStateDatabase(journal.Database)
	if err != nil {
		return err
	}
	var tx *sql.Tx
	if db != nil {
		defer db.Close()
		tx, err = db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, id := range journal.ThreadIDs {
			if _, err := tx.Exec(`UPDATE threads SET model_provider = 'openai' WHERE id = ? AND model_provider = 'aigate'`, id); err != nil {
				return err
			}
		}
	}
	var patched []codexProviderRollout
	defer func() {
		if returnErr == nil {
			return
		}
		for i := len(patched) - 1; i >= 0; i-- {
			_, restoreErr := writeCodexProvider(home, patched[i], "openai", "aigate")
			returnErr = errors.Join(returnErr, restoreErr)
		}
	}()
	for _, rollout := range journal.Rollouts {
		// Deleted sessions do not prevent disabling the proxy.
		if _, err := os.Lstat(rollout.Path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		changed, err := writeCodexProvider(home, rollout, "aigate", "openai")
		if changed {
			patched = append(patched, rollout)
		}
		if err != nil {
			return err
		}
	}
	if tx != nil {
		return tx.Commit()
	}
	return nil
}
