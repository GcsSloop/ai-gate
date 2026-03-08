package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/api"
)

func TestSettingsHandlerBackupAndList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "provider = \"router\"\n", `{"access_token":"token-a"}`)

	handler := api.NewSettingsHandler()

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

func TestSettingsHandlerRestoreCreatesPreRestoreBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "provider = \"before\"\n", `{"access_token":"token-before"}`)

	handler := api.NewSettingsHandler()

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

	handler := api.NewSettingsHandler()

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

func TestSettingsHandlerProxyDisableConflictWhenConfigChanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "model_provider = \"openai\"\n", `{"access_token":"token-before"}`)

	handler := api.NewSettingsHandler()

	enableReq := httptest.NewRequest(http.MethodPost, "/settings/proxy/enable", nil)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want %d", enableRec.Code, http.StatusOK)
	}

	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("model_provider = \"changed\"\n"), 0o600); err != nil {
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
	assertFileContains(t, filepath.Join(home, ".codex", "config.toml"), `model_provider = "changed"`)
	assertFileContains(t, filepath.Join(home, ".codex", "auth.json"), `"token-before"`)
}

func TestSettingsHandlerProxyDisableForceRestoresWhenConfigChanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prepareCodexFiles(t, home, "model_provider = \"openai\"\n", `{"access_token":"token-before"}`)

	handler := api.NewSettingsHandler()

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

	handler := api.NewSettingsHandler()

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

	handler := api.NewSettingsHandler()
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
