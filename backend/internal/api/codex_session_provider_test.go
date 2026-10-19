package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sessionProviderFixture(t *testing.T) (string, *sql.DB) {
	t.Helper()
	home := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Dir(home))
	t.Setenv("USERPROFILE", filepath.Dir(home))
	db, err := sql.Open("sqlite", filepath.Join(home, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE threads (id TEXT PRIMARY KEY, model_provider TEXT NOT NULL, rollout_path TEXT, title TEXT);`); err != nil {
		t.Fatal(err)
	}
	return home, db
}

func TestCodexStateDatabaseDSN(t *testing.T) {
	for _, tc := range []struct {
		name     string
		path     string
		wantPath string
	}{
		{"unix", "/home/user/.codex/state_5.sqlite", "/home/user/.codex/state_5.sqlite"},
		{"windows", "C:/Users/user/.codex/state_5.sqlite", "/C:/Users/user/.codex/state_5.sqlite"},
		{"windows_special_characters", "D:/Users/用户 #1%/.codex/state_5.sqlite", "/D:/Users/用户 #1%/.codex/state_5.sqlite"},
		{"unix_special_characters", "/home/用户 #1%?/.codex/state_5.sqlite", "/home/用户 #1%?/.codex/state_5.sqlite"},
		{"unc", "//server/share/.codex/state_5.sqlite", "//server/share/.codex/state_5.sqlite"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := codexStateDatabaseDSN(tc.path)
			u, err := url.Parse(dsn)
			if err != nil {
				t.Fatal(err)
			}
			if u.Scheme != "file" || u.Host != "" || u.Path != tc.wantPath || u.Fragment != "" {
				t.Fatalf("invalid SQLite file URI: %s (host=%q path=%q)", dsn, u.Host, u.Path)
			}
			q := u.Query()
			if q.Get("mode") != "rw" || q.Get("_pragma") != "busy_timeout(1500)" || q.Get("_txlock") != "immediate" {
				t.Fatalf("database safeguards missing: %s", dsn)
			}
		})
	}
}

func TestCodexStateDatabaseOpenSpecialPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "用户 #1%", "state_5.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// Seed through a plain native path, then reopen through the migration helper.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE threads (id TEXT, model_provider TEXT); INSERT INTO threads VALUES ('official', 'openai')`); err != nil {
		t.Fatal(err)
	}
	opened, err := openCodexStateDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	var provider string
	if err := opened.QueryRow(`SELECT model_provider FROM threads WHERE id = 'official'`).Scan(&provider); err != nil {
		t.Fatal(err)
	}
	if provider != "openai" {
		t.Fatalf("provider=%q", provider)
	}
}

func TestCodexStateDatabaseDSNDoesNotCreateMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state_5.sqlite")
	db, err := sql.Open("sqlite", codexStateDatabaseDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err == nil {
		t.Fatal("opened a missing Codex database")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing database must not be created: %v", err)
	}
}

func TestSettingsHandlerProxyAutomaticallySwitchesAndRestoresSessionProviders(t *testing.T) {
	home, db := sessionProviderFixture(t)
	seedProviderSession(t, home, db, "official", "openai", false, false)
	seedProviderSession(t, home, db, "custom", "custom", false, false)
	config := filepath.Join(home, "config.toml")
	if err := os.WriteFile(config, []byte("model_provider = \"openai\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := &SettingsHandler{}
	request := func(method, path string) proxyStatusResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		var status proxyStatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		return status
	}
	status := request(http.MethodPost, "/settings/proxy/enable")
	if status.SessionMigration == nil || status.SessionMigration.MigratedSessions != 1 || !status.SessionMigration.ReloadRequired {
		t.Fatalf("migration summary=%+v", status)
	}
	assertStoredProvider(t, db, "official", "aigate")
	assertStoredProvider(t, db, "custom", "custom")
	status = request(http.MethodGet, "/settings/proxy/status")
	if status.SessionMigration == nil || status.SessionMigration.MigratedSessions != 1 {
		t.Fatal("migration summary not retained")
	}
	status = request(http.MethodPost, "/settings/proxy/disable")
	if status.Enabled {
		t.Fatal("proxy still enabled")
	}
	assertStoredProvider(t, db, "official", "openai")
	assertStoredProvider(t, db, "custom", "custom")
	if body, _ := os.ReadFile(config); strings.Contains(string(body), "aigate") {
		t.Fatal("proxy config not removed")
	}
}

func TestSettingsHandlerProxyMigrationFailureRestoresConfig(t *testing.T) {
	home, db := sessionProviderFixture(t)
	path := seedProviderSession(t, home, db, "official", "openai", false, false)
	if err := os.WriteFile(path, []byte("invalid session header\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := []byte("model_provider = \"openai\"\n")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	(&SettingsHandler{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/settings/proxy/enable", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	if got, _ := os.ReadFile(filepath.Join(home, "config.toml")); string(got) != string(config) {
		t.Fatal("config not rolled back")
	}
	assertStoredProvider(t, db, "official", "openai")
	if _, err := readProxySession(filepath.Dir(home)); !os.IsNotExist(err) {
		t.Fatalf("proxy session remains: %v", err)
	}
}

func seedProviderSession(t *testing.T, home string, db *sql.DB, id, provider string, archived, legacy bool) string {
	t.Helper()
	root := "sessions"
	if archived {
		root = "archived_sessions"
	}
	path := filepath.Join(home, root, "2026", "10", "17", "rollout-test-"+id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	meta := map[string]any{"id": id, "model_provider": provider, "source": "vscode", "unknown": map[string]any{"model_provider": "openai"}}
	header := map[string]any{"type": "session_meta", "payload": meta}
	if legacy {
		header = map[string]any{"item": map[string]any{"type": "session_meta", "meta": meta}}
	}
	line, _ := json.Marshal(header)
	if err := os.WriteFile(path, append(line, []byte("\n{\"type\":\"user_message\",\"text\":\"keep me byte for byte\"}\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO threads VALUES (?, ?, ?, 'unchanged title')`, id, provider, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertStoredProvider(t *testing.T, db *sql.DB, id, want string) {
	t.Helper()
	var provider, title string
	if err := db.QueryRow(`SELECT model_provider, title FROM threads WHERE id = ?`, id).Scan(&provider, &title); err != nil {
		t.Fatal(err)
	}
	if provider != want || title != "unchanged title" {
		t.Fatalf("%s: provider=%s title=%s", id, provider, title)
	}
}

func TestCodexSessionProviderSwitchAndRestorePreservesHistory(t *testing.T) {
	home, db := sessionProviderFixture(t)
	path := seedProviderSession(t, home, db, "official", "openai", false, false)
	archive := seedProviderSession(t, home, db, "archived", "openai", true, true)
	custom := seedProviderSession(t, home, db, "custom", "my_codex", false, false)
	original, _ := os.ReadFile(path)
	customOriginal, _ := os.ReadFile(custom)
	info, _ := os.Stat(path)
	result, err := migrateCodexSessionProviders(home, filepath.Join(t.TempDir(), "backup"))
	if err != nil {
		t.Fatal(err)
	}
	if result.MigratedSessions != 2 || result.BackupPath == "" || !result.ReloadRequired {
		t.Fatalf("result=%+v", result)
	}
	assertStoredProvider(t, db, "official", "aigate")
	assertStoredProvider(t, db, "archived", "aigate")
	assertStoredProvider(t, db, "custom", "my_codex")
	changed, _ := os.ReadFile(path)
	want := strings.Replace(string(original), `"model_provider":"openai"`, `"model_provider":"aigate"`, 1)
	if string(changed) != want {
		t.Fatalf("unexpected rewrite: %s", changed)
	}
	newInfo, _ := os.Stat(path)
	if !os.SameFile(info, newInfo) {
		t.Fatal("migration replaced the inode used by active appenders")
	}
	if data, _ := os.ReadFile(custom); string(data) != string(customOriginal) {
		t.Fatal("custom session modified")
	}
	if data, _ := os.ReadFile(archive); !strings.Contains(string(data), `"model_provider":"aigate"`) {
		t.Fatal("legacy archive not migrated")
	}
	// Codex may keep appending through its existing file descriptor during migration.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("appended after switch\n")
	f.Close()
	if err := restoreCodexSessionProviders(home, result.BackupPath); err != nil {
		t.Fatal(err)
	}
	assertStoredProvider(t, db, "official", "openai")
	if data, _ := os.ReadFile(path); string(data) != string(original)+"appended after switch\n" {
		t.Fatalf("history changed on restore: %s", data)
	}
}

func TestCodexSessionProviderMigrationIsIdempotentAndSupportsDBOnlyThreads(t *testing.T) {
	home, db := sessionProviderFixture(t)
	seedProviderSession(t, home, db, "existing", "aigate", false, false)
	if _, err := db.Exec(`INSERT INTO threads VALUES ('paginated', 'openai', NULL, 'unchanged title')`); err != nil {
		t.Fatal(err)
	}
	result, err := migrateCodexSessionProviders(home, filepath.Join(t.TempDir(), "backup"))
	if err != nil {
		t.Fatal(err)
	}
	if result.MigratedSessions != 1 {
		t.Fatalf("result=%+v", result)
	}
	assertStoredProvider(t, db, "paginated", "aigate")
	again, err := migrateCodexSessionProviders(home, filepath.Join(t.TempDir(), "unused"))
	if err != nil || again.MigratedSessions != 0 {
		t.Fatalf("repeat: %+v %v", again, err)
	}
	if err := restoreCodexSessionProviders(home, result.BackupPath); err != nil {
		t.Fatal(err)
	}
	assertStoredProvider(t, db, "existing", "aigate")
	assertStoredProvider(t, db, "paginated", "openai")
}

func TestCodexSessionProviderMigrationWithoutStateDB(t *testing.T) {
	home, db := sessionProviderFixture(t)
	path := seedProviderSession(t, home, db, "official", "openai", false, false)
	db.Close()
	if err := os.Remove(filepath.Join(home, "state_5.sqlite")); err != nil {
		t.Fatal(err)
	}
	result, err := migrateCodexSessionProviders(home, filepath.Join(t.TempDir(), "backup"))
	if err != nil || result.MigratedSessions != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err := restoreCodexSessionProviders(home, result.BackupPath); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), `"model_provider":"openai"`) {
		t.Fatal("file-only session not restored")
	}
}

func TestCodexSessionProviderBackupFailureLeavesSessionsUnchanged(t *testing.T) {
	home, db := sessionProviderFixture(t)
	path := seedProviderSession(t, home, db, "official", "openai", false, false)
	before, _ := os.ReadFile(path)
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, []byte("block"), 0o600)
	if _, err := migrateCodexSessionProviders(home, filepath.Join(blocker, "backup")); err == nil {
		t.Fatal("expected backup failure")
	}
	assertStoredProvider(t, db, "official", "openai")
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("file changed before backup succeeded")
	}
}

func TestCodexSessionProviderRestorePreservesExternalProviderChange(t *testing.T) {
	home, db := sessionProviderFixture(t)
	path := seedProviderSession(t, home, db, "official", "openai", false, false)
	result, err := migrateCodexSessionProviders(home, filepath.Join(t.TempDir(), "backup"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE threads SET model_provider = 'custom' WHERE id = 'official'`); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(data), `"model_provider":"aigate"`, `"model_provider":"custom"`, 1)), 0o600)
	if err := restoreCodexSessionProviders(home, result.BackupPath); err != nil {
		t.Fatal(err)
	}
	assertStoredProvider(t, db, "official", "custom")
	if after, _ := os.ReadFile(path); !strings.Contains(string(after), `"model_provider":"custom"`) {
		t.Fatal("external provider overwritten")
	}
}

func TestCodexSessionProviderMigrationSkipsEmptyRollout(t *testing.T) {
	home, db := sessionProviderFixture(t)
	seedProviderSession(t, home, db, "official", "openai", false, false)
	empty := filepath.Join(home, "sessions", "2026", "10", "09", "rollout-2026-10-09T09-56-40-01a11e60-7774-7e10-938b-6ee1cbec1c96.jsonl")
	if err := os.MkdirAll(filepath.Dir(empty), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := migrateCodexSessionProviders(home, filepath.Join(t.TempDir(), "backup"))
	if err != nil {
		t.Fatalf("empty rollout blocked migration: %v", err)
	}
	if result.MigratedSessions != 1 {
		t.Fatalf("result=%+v", result)
	}
	assertStoredProvider(t, db, "official", "aigate")
	info, err := os.Stat(empty)
	if err != nil || info.Size() != 0 {
		t.Fatalf("empty rollout changed: %+v, %v", info, err)
	}
	if err := restoreCodexSessionProviders(home, result.BackupPath); err != nil {
		t.Fatal(err)
	}
	assertStoredProvider(t, db, "official", "openai")
}

func TestCodexSessionProviderMigrationAcceptsHeaderWithoutNewline(t *testing.T) {
	home, db := sessionProviderFixture(t)
	path := seedProviderSession(t, home, db, "official", "openai", false, false)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	header := data[:strings.IndexByte(string(data), '\n')]
	if err := os.WriteFile(path, header, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := migrateCodexSessionProviders(home, filepath.Join(t.TempDir(), "backup"))
	if err != nil {
		t.Fatalf("header without newline blocked migration: %v", err)
	}
	assertStoredProvider(t, db, "official", "aigate")
	if got, _ := os.ReadFile(path); string(got) != strings.Replace(string(header), `"model_provider":"openai"`, `"model_provider":"aigate"`, 1) {
		t.Fatalf("unexpected rewrite: %s", got)
	}
	if err := restoreCodexSessionProviders(home, result.BackupPath); err != nil {
		t.Fatal(err)
	}
	assertStoredProvider(t, db, "official", "openai")
	if got, _ := os.ReadFile(path); string(got) != string(header) {
		t.Fatalf("header changed on restore: %s", got)
	}
}
