package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
