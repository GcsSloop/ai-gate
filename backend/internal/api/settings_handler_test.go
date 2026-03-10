package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestSettingsHandlerGetAndPutAppSettings(t *testing.T) {
	handler, repo := newSettingsHandler(t)

	getReq := httptest.NewRequest(http.MethodGet, "/settings/app", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /settings/app status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var got settings.AppSettings
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal app settings: %v", err)
	}
	if got != settings.DefaultAppSettings() {
		t.Fatalf("default settings = %+v, want %+v", got, settings.DefaultAppSettings())
	}

	body := bytes.NewBufferString(`{
		"launch_at_login": true,
		"silent_start": true,
		"close_to_tray": false,
		"show_proxy_switch_on_home": false,
		"proxy_host": "localhost",
		"proxy_port": 15721,
		"auto_failover_enabled": true,
		"auto_backup_interval_hours": 12,
		"backup_retention_count": 7
	}`)
	putReq := httptest.NewRequest(http.MethodPut, "/settings/app", body)
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT /settings/app status = %d, want %d; body=%s", putRec.Code, http.StatusOK, putRec.Body.String())
	}

	stored, err := repo.GetAppSettings()
	if err != nil {
		t.Fatalf("GetAppSettings returned error: %v", err)
	}
	if !stored.LaunchAtLogin || !stored.SilentStart || stored.CloseToTray || stored.ShowProxySwitchOnHome || stored.ProxyHost != "localhost" || stored.ProxyPort != 15721 || !stored.AutoFailoverEnabled || stored.AutoBackupIntervalHours != 12 || stored.BackupRetentionCount != 7 {
		t.Fatalf("stored settings = %+v, want updated values", stored)
	}
}

func TestSettingsHandlerGetAndPutFailoverQueue(t *testing.T) {
	handler, repo := newSettingsHandler(t)

	getReq := httptest.NewRequest(http.MethodGet, "/settings/failover-queue", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /settings/failover-queue status = %d, want %d", getRec.Code, http.StatusOK)
	}
	if strings.TrimSpace(getRec.Body.String()) != "[]" {
		t.Fatalf("GET /settings/failover-queue body = %s, want []", getRec.Body.String())
	}

	putReq := httptest.NewRequest(http.MethodPut, "/settings/failover-queue", bytes.NewBufferString(`{"account_ids":[3,1,8]}`))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT /settings/failover-queue status = %d, want %d; body=%s", putRec.Code, http.StatusOK, putRec.Body.String())
	}

	queue, err := repo.ListFailoverQueue()
	if err != nil {
		t.Fatalf("ListFailoverQueue returned error: %v", err)
	}
	if !equalInt64s(queue, []int64{3, 1, 8}) {
		t.Fatalf("stored queue = %v, want [3 1 8]", queue)
	}
}

func TestSettingsHandlerProxyStatusIncludesConfiguredAddress(t *testing.T) {
	handler, repo := newSettingsHandler(t)

	current := settings.DefaultAppSettings()
	current.ProxyHost = "localhost"
	current.ProxyPort = 15721
	if err := repo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/proxy/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/proxy/status status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status response: %v", err)
	}
	if payload["host"] != "localhost" {
		t.Fatalf("host = %v, want localhost", payload["host"])
	}
	if payload["port"] != float64(15721) {
		t.Fatalf("port = %v, want 15721", payload["port"])
	}
}

func TestSettingsHandlerExportsAndImportsSQL(t *testing.T) {
	handler, repo := newSettingsHandler(t)

	initial := settings.DefaultAppSettings()
	initial.ProxyPort = 15721
	if err := repo.SaveAppSettings(initial); err != nil {
		t.Fatalf("SaveAppSettings(initial) returned error: %v", err)
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/settings/database/sql-export", nil)
	exportRec := httptest.NewRecorder()
	handler.ServeHTTP(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("GET /settings/database/sql-export status = %d, want %d; body=%s", exportRec.Code, http.StatusOK, exportRec.Body.String())
	}
	exported := exportRec.Body.Bytes()
	if !bytes.Contains(exported, []byte(`INSERT INTO "app_settings"`)) {
		t.Fatalf("exported SQL missing app_settings insert: %s", string(exported))
	}

	changed := initial
	changed.ProxyPort = 16888
	if err := repo.SaveAppSettings(changed); err != nil {
		t.Fatalf("SaveAppSettings(changed) returned error: %v", err)
	}

	importReq := httptest.NewRequest(http.MethodPost, "/settings/database/sql-import", bytes.NewReader(exported))
	importRec := httptest.NewRecorder()
	handler.ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("POST /settings/database/sql-import status = %d, want %d; body=%s", importRec.Code, http.StatusOK, importRec.Body.String())
	}

	restored, err := repo.GetAppSettings()
	if err != nil {
		t.Fatalf("GetAppSettings returned error: %v", err)
	}
	if restored.ProxyPort != 15721 {
		t.Fatalf("restored ProxyPort = %d, want 15721", restored.ProxyPort)
	}
}

func TestSettingsHandlerCreatesListsAndRestoresDatabaseBackups(t *testing.T) {
	handler, repo := newSettingsHandler(t)

	initial := settings.DefaultAppSettings()
	initial.ProxyPort = 15721
	if err := repo.SaveAppSettings(initial); err != nil {
		t.Fatalf("SaveAppSettings(initial) returned error: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/settings/database/backup", nil)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST /settings/database/backup status = %d, want %d; body=%s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}

	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created backup: %v", err)
	}
	backupID, _ := created["backup_id"].(string)
	if strings.TrimSpace(backupID) == "" {
		t.Fatal("backup_id is empty")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/settings/database/backups", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /settings/database/backups status = %d, want %d", listRec.Code, http.StatusOK)
	}
	var listed []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal listed backups: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("len(listed) = %d, want 1", len(listed))
	}

	changed := initial
	changed.ProxyPort = 16888
	if err := repo.SaveAppSettings(changed); err != nil {
		t.Fatalf("SaveAppSettings(changed) returned error: %v", err)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/settings/database/restore", bytes.NewBufferString(`{"backup_id":"`+backupID+`"}`))
	restoreReq.Header.Set("Content-Type", "application/json")
	restoreRec := httptest.NewRecorder()
	handler.ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("POST /settings/database/restore status = %d, want %d; body=%s", restoreRec.Code, http.StatusOK, restoreRec.Body.String())
	}

	restored, err := repo.GetAppSettings()
	if err != nil {
		t.Fatalf("GetAppSettings returned error: %v", err)
	}
	if restored.ProxyPort != 15721 {
		t.Fatalf("restored ProxyPort = %d, want 15721", restored.ProxyPort)
	}
}

func TestSettingsHandlerBackupAndList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "provider = \"router\"\n", `{"access_token":"token-a"}`)

	handler, _ := newSettingsHandler(t)

	backupReq := httptest.NewRequest(http.MethodPost, "/settings/codex/backup", nil)
	backupRec := httptest.NewRecorder()
	handler.ServeHTTP(backupRec, backupReq)
	if backupRec.Code != http.StatusCreated {
		t.Fatalf("POST /settings/codex/backup status = %d, want %d", backupRec.Code, http.StatusCreated)
	}

	var backupResp map[string]any
	if err := json.Unmarshal(backupRec.Body.Bytes(), &backupResp); err != nil {
		t.Fatalf("unmarshal backup response: %v", err)
	}
	backupID, _ := backupResp["backup_id"].(string)
	if strings.TrimSpace(backupID) == "" {
		t.Fatal("backup_id is empty")
	}

	backupDir := filepath.Join(home, ".aigate", "data", "codex", "backup", backupID)
	assertFileContains(t, filepath.Join(backupDir, "config.toml"), `provider = "router"`)
	assertFileContains(t, filepath.Join(backupDir, "auth.json"), `"token-a"`)
	assertFileContains(t, filepath.Join(backupDir, "manifest.json"), `"source":"~/.codex/config.toml"`)
	assertFileContains(t, filepath.Join(backupDir, "manifest.json"), `"backup_source":"manual"`)

	listReq := httptest.NewRequest(http.MethodGet, "/settings/codex/backups", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /settings/codex/backups status = %d, want %d", listRec.Code, http.StatusOK)
	}

	var listed []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("len(listed) = %d, want 1", len(listed))
	}
	if listed[0]["backup_id"] != backupID {
		t.Fatalf("backup id = %v, want %s", listed[0]["backup_id"], backupID)
	}

	filesReq := httptest.NewRequest(http.MethodGet, "/settings/codex/backups/"+backupID+"/files", nil)
	filesRec := httptest.NewRecorder()
	handler.ServeHTTP(filesRec, filesReq)
	if filesRec.Code != http.StatusOK {
		t.Fatalf("GET /settings/codex/backups/{id}/files status = %d, want %d", filesRec.Code, http.StatusOK)
	}
	var filesPayload map[string]any
	if err := json.Unmarshal(filesRec.Body.Bytes(), &filesPayload); err != nil {
		t.Fatalf("unmarshal files response: %v", err)
	}
	files, _ := filesPayload["files"].(map[string]any)
	if files == nil {
		t.Fatal("files map is missing")
	}
	if files["config.toml"] == "" || files["auth.json"] == "" || files["manifest.json"] == "" {
		t.Fatalf("backup files content missing: %#v", files)
	}
}

func TestSettingsHandlerBackupRetentionBySource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "provider = \"router\"\n", `{"access_token":"token-a"}`)

	backupRoot := filepath.Join(home, ".aigate", "data", "codex", "backup")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		t.Fatalf("mkdir backup root: %v", err)
	}
	for i := 0; i < 11; i++ {
		seedCodexBackupDir(t, backupRoot, "20260101-00000"+strconv.Itoa(i)+".000", "auto")
		seedCodexBackupDir(t, backupRoot, "20260101-10000"+strconv.Itoa(i)+".000", "proxy_enable")
	}
	for i := 0; i < 3; i++ {
		seedCodexBackupDir(t, backupRoot, "20260102-20000"+strconv.Itoa(i)+".000", "manual")
	}

	handler, _ := newSettingsHandler(t)
	backupReq := httptest.NewRequest(http.MethodPost, "/settings/codex/backup", nil)
	backupRec := httptest.NewRecorder()
	handler.ServeHTTP(backupRec, backupReq)
	if backupRec.Code != http.StatusCreated {
		t.Fatalf("manual backup status = %d, want %d", backupRec.Code, http.StatusCreated)
	}

	autoCount, err := countBackupsBySource(backupRoot, "auto")
	if err != nil {
		t.Fatalf("count auto backups: %v", err)
	}
	if autoCount != 10 {
		t.Fatalf("auto backup count = %d, want 10", autoCount)
	}
	proxyCount, err := countBackupsBySource(backupRoot, "proxy_enable")
	if err != nil {
		t.Fatalf("count proxy_enable backups: %v", err)
	}
	if proxyCount != 10 {
		t.Fatalf("proxy_enable backup count = %d, want 10", proxyCount)
	}
	manualCount, err := countBackupsBySource(backupRoot, "manual")
	if err != nil {
		t.Fatalf("count manual backups: %v", err)
	}
	if manualCount != 4 {
		t.Fatalf("manual backup count = %d, want 4", manualCount)
	}
}

func TestSettingsHandlerRestoreCreatesPreRestoreBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "provider = \"before\"\n", `{"access_token":"token-before"}`)

	handler, _ := newSettingsHandler(t)

	firstBackupReq := httptest.NewRequest(http.MethodPost, "/settings/codex/backup", nil)
	firstBackupRec := httptest.NewRecorder()
	handler.ServeHTTP(firstBackupRec, firstBackupReq)
	if firstBackupRec.Code != http.StatusCreated {
		t.Fatalf("first backup status = %d, want %d", firstBackupRec.Code, http.StatusCreated)
	}
	var first map[string]any
	if err := json.Unmarshal(firstBackupRec.Body.Bytes(), &first); err != nil {
		t.Fatalf("unmarshal first backup: %v", err)
	}
	backupID, _ := first["backup_id"].(string)
	if backupID == "" {
		t.Fatal("first backup_id is empty")
	}

	prepareCodexFiles(t, home, "provider = \"current\"\n", `{"access_token":"token-current"}`)
	time.Sleep(10 * time.Millisecond)

	restoreBody := bytes.NewBufferString(`{"backup_id":"` + backupID + `"}`)
	restoreReq := httptest.NewRequest(http.MethodPost, "/settings/codex/restore", restoreBody)
	restoreReq.Header.Set("Content-Type", "application/json")
	restoreRec := httptest.NewRecorder()
	handler.ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("POST /settings/codex/restore status = %d, want %d; body=%s", restoreRec.Code, http.StatusOK, restoreRec.Body.String())
	}

	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `provider = "before"`)
	assertFileContains(t, filepath.Join(home, ".codex", "auth.json"), `"token-before"`)

	preDirs, err := os.ReadDir(filepath.Join(home, ".aigate", "data", "codex", "pre-restore"))
	if err != nil {
		t.Fatalf("read pre-restore backups dir: %v", err)
	}
	if len(preDirs) == 0 {
		t.Fatal("expected at least one pre-restore backup")
	}
	names := make([]string, 0, len(preDirs))
	for _, item := range preDirs {
		if item.IsDir() {
			names = append(names, item.Name())
		}
	}
	sort.Strings(names)
	latest := names[len(names)-1]
	assertFileContains(t, filepath.Join(home, ".aigate", "data", "codex", "pre-restore", latest, "config.toml"), `provider = "current"`)
	assertFileContains(t, filepath.Join(home, ".aigate", "data", "codex", "pre-restore", latest, "auth.json"), `"token-current"`)
}

func TestSettingsHandlerProxyEnableDisable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "model_provider = \"openai\"\n", `{"access_token":"token-before"}`)

	handler, _ := newSettingsHandler(t)

	enableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/enable", nil)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("POST /settings/proxy/enable status = %d, want %d", enableRec.Code, http.StatusOK)
	}
	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `[model_providers.aigate]`)
	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `model_provider = "aigate"`)

	statusReq := httptest.NewRequest(http.MethodGet, "/settings/proxy/status", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("GET /settings/proxy/status status = %d, want %d", statusRec.Code, http.StatusOK)
	}
	var statusPayload map[string]any
	if err := json.Unmarshal(statusRec.Body.Bytes(), &statusPayload); err != nil {
		t.Fatalf("unmarshal status response: %v", err)
	}
	if statusPayload["enabled"] != true {
		t.Fatalf("enabled = %v, want true", statusPayload["enabled"])
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/disable", nil)
	disableRec := httptest.NewRecorder()
	handler.ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("POST /settings/proxy/disable status = %d, want %d", disableRec.Code, http.StatusOK)
	}
	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `model_provider = "openai"`)
	assertFileContains(t, filepath.Join(home, ".codex", "auth.json"), `"token-before"`)
}

func TestSettingsHandlerProxyDisableRestoreDoesNotRestoreAuthJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "model_provider = \"openai\"\n", `{"access_token":"token-before"}`)

	handler, _ := newSettingsHandler(t)

	enableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/enable", nil)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("POST /settings/proxy/enable status = %d, want %d", enableRec.Code, http.StatusOK)
	}

	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{"access_token":"token-updated"}`), 0o600); err != nil {
		t.Fatalf("write auth.json after enable: %v", err)
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/disable", nil)
	disableRec := httptest.NewRecorder()
	handler.ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("POST /settings/proxy/disable status = %d, want %d", disableRec.Code, http.StatusOK)
	}

	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `model_provider = "openai"`)
	assertFileContains(t, filepath.Join(home, ".codex", "auth.json"), `"token-updated"`)
	assertFileNotContains(t, filepath.Join(home, ".codex", "auth.json"), `"token-before"`)
}

func TestSettingsHandlerUpdatingProxyAddressRewritesEnabledConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "model_provider = \"openai\"\n", `{"access_token":"token-before"}`)

	handler, _ := newSettingsHandler(t)

	enableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/enable", nil)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("POST /settings/proxy/enable status = %d, want %d", enableRec.Code, http.StatusOK)
	}

	putReq := httptest.NewRequest(http.MethodPut, "/settings/app", bytes.NewBufferString(`{
		"launch_at_login": false,
		"silent_start": false,
		"close_to_tray": true,
		"show_proxy_switch_on_home": true,
		"proxy_host": "localhost",
		"proxy_port": 15721,
		"auto_failover_enabled": false,
		"auto_backup_interval_hours": 24,
		"backup_retention_count": 10
	}`))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT /settings/app status = %d, want %d; body=%s", putRec.Code, http.StatusOK, putRec.Body.String())
	}

	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `base_url = "http://localhost:15721/ai-router/api"`)
}

func TestSettingsHandlerProxyDisableConflictWhenConfigChanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "model_provider = \"openai\"\n", `{"access_token":"token-before"}`)

	handler, _ := newSettingsHandler(t)

	enableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/enable", nil)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want %d", enableRec.Code, http.StatusOK)
	}

	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte(`model_provider = "aigate"

[model_providers.aigate]
name = "aigate"
base_url = "http://127.0.0.1:6789/ai-router/api"
wire_api = "responses"
`), 0o600); err != nil {
		t.Fatalf("write config for conflict: %v", err)
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/disable", nil)
	disableRec := httptest.NewRecorder()
	handler.ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusConflict {
		t.Fatalf("disable status = %d, want %d", disableRec.Code, http.StatusConflict)
	}

	skipReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/disable?skip_restore=1", nil)
	skipRec := httptest.NewRecorder()
	handler.ServeHTTP(skipRec, skipReq)
	if skipRec.Code != http.StatusOK {
		t.Fatalf("skip-restore disable status = %d, want %d", skipRec.Code, http.StatusOK)
	}
	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `model_provider = "openai"`)
	assertFileNotContains(t, filepath.Join(home, ".codex", "config.toml"), `[model_providers.aigate]`)
	assertFileContains(t, filepath.Join(home, ".codex", "auth.json"), `"token-before"`)
}

func TestSettingsHandlerProxyDisableForceRestoresWhenConfigChanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "model_provider = \"openai\"\n", `{"access_token":"token-before"}`)

	handler, _ := newSettingsHandler(t)

	enableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/enable", nil)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want %d", enableRec.Code, http.StatusOK)
	}

	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("model_provider = \"changed\"\n"), 0o600); err != nil {
		t.Fatalf("write config for conflict: %v", err)
	}

	forceReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/disable?force=1", nil)
	forceRec := httptest.NewRecorder()
	handler.ServeHTTP(forceRec, forceReq)
	if forceRec.Code != http.StatusOK {
		t.Fatalf("force disable status = %d, want %d", forceRec.Code, http.StatusOK)
	}
	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `model_provider = "openai"`)
	assertFileContains(t, filepath.Join(home, ".codex", "auth.json"), `"token-before"`)
}

func TestSettingsHandlerEnablePatchesThirdPartyProviderAndDetachDoesNotOverwriteConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, `model_provider = "ppchat"

[model_providers.ppchat]
name = "ppchat"
base_url = "https://code.ppchat.vip/v1"
wire_api = "responses"
`, `{"access_token":"token-before"}`)

	handler, _ := newSettingsHandler(t)

	enableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/enable", nil)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want %d", enableRec.Code, http.StatusOK)
	}
	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `model_provider = "ppchat"`)
	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `base_url = "http://127.0.0.1:6789/ai-router/api"`)

	statusReq := httptest.NewRequest(http.MethodGet, "/settings/proxy/status", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", statusRec.Code, http.StatusOK)
	}
	var statusPayload map[string]any
	if err := json.Unmarshal(statusRec.Body.Bytes(), &statusPayload); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if statusPayload["mode"] != "patched_existing_provider" {
		t.Fatalf("mode = %v, want patched_existing_provider", statusPayload["mode"])
	}
	if statusPayload["target_provider"] != "ppchat" {
		t.Fatalf("target_provider = %v, want ppchat", statusPayload["target_provider"])
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/disable?mode=detach", nil)
	disableRec := httptest.NewRecorder()
	handler.ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("detach disable status = %d, want %d", disableRec.Code, http.StatusOK)
	}
	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `base_url = "http://127.0.0.1:6789/ai-router/api"`)
}

func TestSettingsHandlerEnableNormalizesDuplicateAigateProviderSections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, `model_provider = "aigate"

[model_providers.aigate]
name = "aigate"
base_url = "https://old-a.example/v1"
wire_api = "chat_completions"

[model_providers.aigate]
name = "aigate"
base_url = "https://old-b.example/v1"
wire_api = "chat_completions"
`, `{"access_token":"token-before"}`)

	handler, _ := newSettingsHandler(t)
	enableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/enable", nil)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want %d; body=%s", enableRec.Code, http.StatusOK, enableRec.Body.String())
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read %s: %v", configPath, err)
	}
	body := string(raw)
	if got := strings.Count(body, "[model_providers.aigate]"); got != 1 {
		t.Fatalf("aigate section count = %d, want 1; config=%q", got, body)
	}
	assertFileContains(t, configPath, `base_url = "http://127.0.0.1:6789/ai-router/api"`)
	assertFileContains(t, configPath, `wire_api = "responses"`)
	assertFileContains(t, configPath, `requires_openai_auth = true`)
	assertFileContains(t, configPath, `store = false`)
}

func TestSettingsHandlerEnableRemovesLegacyAigateDefinitions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, `model_provider = "openai"

[model_providers]
aigate = { name = "aigate", base_url = "https://legacy-inline.example/v1", wire_api = "chat_completions" }

model_providers.aigate.base_url = "https://legacy-dotted.example/v1"
model_providers.aigate.name = "aigate"

[model_providers.aigate]
name = "aigate"
base_url = "https://legacy-section.example/v1"
wire_api = "chat_completions"
`, `{"access_token":"token-before"}`)

	handler, _ := newSettingsHandler(t)
	enableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/enable", nil)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want %d; body=%s", enableRec.Code, http.StatusOK, enableRec.Body.String())
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read %s: %v", configPath, err)
	}
	body := string(raw)
	if got := strings.Count(body, "[model_providers.aigate]"); got != 1 {
		t.Fatalf("aigate section count = %d, want 1; config=%q", got, body)
	}
	if strings.Contains(body, "model_providers.aigate.") {
		t.Fatalf("legacy dotted aigate keys still exist; config=%q", body)
	}
	if strings.Contains(body, "aigate = {") {
		t.Fatalf("legacy inline aigate definition still exists; config=%q", body)
	}
	assertFileContains(t, configPath, `base_url = "http://127.0.0.1:6789/ai-router/api"`)
	assertFileContains(t, configPath, `wire_api = "responses"`)
	assertFileContains(t, configPath, `requires_openai_auth = true`)
	assertFileContains(t, configPath, `store = false`)
}

func prepareCodexFiles(t *testing.T, home string, configBody string, authBody string) {
	t.Helper()
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(configBody), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(authBody), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
}

func assertFileContains(t *testing.T, path string, expected string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(raw)
	if !strings.Contains(body, expected) {
		t.Fatalf("%s does not contain %q. got=%q", path, expected, body)
	}
}

func assertFileNotContains(t *testing.T, path string, unwanted string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(raw)
	if strings.Contains(body, unwanted) {
		t.Fatalf("%s unexpectedly contains %q. got=%q", path, unwanted, body)
	}
}

func seedCodexBackupDir(t *testing.T, backupRoot string, backupID string, backupSource string) {
	t.Helper()
	backupDir := filepath.Join(backupRoot, backupID)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backup dir %s: %v", backupDir, err)
	}
	manifest := `{"backup_id":"` + backupID + `","kind":"backup","backup_source":"` + backupSource + `","created_at":"2026-01-01T00:00:00Z","files":[{"name":"config.toml","source":"~/.codex/config.toml"},{"name":"auth.json","source":"~/.codex/auth.json"}]}`
	if err := os.WriteFile(filepath.Join(backupDir, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func countBackupsBySource(backupRoot string, source string) (int, error) {
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(backupRoot, entry.Name(), "manifest.json"))
		if err != nil {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		if payload["backup_source"] == source {
			count++
		}
	}
	return count, nil
}

func newSettingsHandler(t *testing.T) (*api.SettingsHandler, *settings.SQLiteRepository) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "router.sqlite")
	store, err := sqlitestore.Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := settings.NewSQLiteRepository(store.DB())
	return api.NewSettingsHandler(repo, api.WithSettingsDatabase(store.DB(), dbPath)), repo
}

func equalInt64s(left []int64, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
