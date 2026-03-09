package api

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/settings"
)

type SettingsRepository interface {
	GetAppSettings() (settings.AppSettings, error)
	SaveAppSettings(settings.AppSettings) error
	ListFailoverQueue() ([]int64, error)
	SaveFailoverQueue([]int64) error
}

type SettingsHandler struct {
	settings SettingsRepository
	db       *sql.DB
	dbPath   string
}

var proxyToggleMu sync.Mutex

type SettingsHandlerOption func(*SettingsHandler)

func WithSettingsDatabase(db *sql.DB, dbPath string) SettingsHandlerOption {
	return func(handler *SettingsHandler) {
		handler.db = db
		handler.dbPath = dbPath
	}
}

func NewSettingsHandler(repo SettingsRepository, opts ...SettingsHandlerOption) *SettingsHandler {
	handler := &SettingsHandler{settings: repo}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

type codexBackupManifest struct {
	BackupID  string                    `json:"backup_id"`
	Kind      string                    `json:"kind"`
	CreatedAt string                    `json:"created_at"`
	Files     []codexBackupManifestFile `json:"files"`
}

type codexBackupManifestFile struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type codexBackupItem struct {
	BackupID  string `json:"backup_id"`
	CreatedAt string `json:"created_at"`
}

type codexRestoreRequest struct {
	BackupID string `json:"backup_id"`
}

type failoverQueueRequest struct {
	AccountIDs []int64 `json:"account_ids"`
}

type codexBackupFilesResponse struct {
	BackupID string            `json:"backup_id"`
	Files    map[string]string `json:"files"`
}

type proxyState struct {
	LastBackupID string `json:"last_backup_id"`
	SessionID    string `json:"session_id,omitempty"`
}

type proxySession struct {
	SessionID             string `json:"session_id"`
	BackupID              string `json:"backup_id"`
	Mode                  string `json:"mode"`
	TargetProvider        string `json:"target_provider,omitempty"`
	PreviousModelProvider string `json:"previous_model_provider,omitempty"`
	OriginalBaseURL       string `json:"original_base_url,omitempty"`
	EnabledConfigHash     string `json:"enabled_config_hash"`
	CreatedAt             string `json:"created_at"`
}

type proxyStatusResponse struct {
	Enabled        bool   `json:"enabled"`
	LastBackupID   string `json:"last_backup_id,omitempty"`
	Mode           string `json:"mode,omitempty"`
	TargetProvider string `json:"target_provider,omitempty"`
	ConfigConflict bool   `json:"config_conflict,omitempty"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
}

func (h *SettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/settings/app":
		h.getAppSettings(w)
	case r.Method == http.MethodPut && r.URL.Path == "/settings/app":
		h.saveAppSettings(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/settings/failover-queue":
		h.getFailoverQueue(w)
	case r.Method == http.MethodPut && r.URL.Path == "/settings/failover-queue":
		h.saveFailoverQueue(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/settings/database/sql-export":
		h.exportDatabaseSQL(w)
	case r.Method == http.MethodPost && r.URL.Path == "/settings/database/sql-import":
		h.importDatabaseSQL(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/settings/database/backups":
		h.listDatabaseBackups(w)
	case r.Method == http.MethodPost && r.URL.Path == "/settings/database/backup":
		h.createDatabaseBackup(w)
	case r.Method == http.MethodPost && r.URL.Path == "/settings/database/restore":
		h.restoreDatabaseBackup(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/settings/codex/backup":
		h.createCodexBackup(w)
	case r.Method == http.MethodGet && r.URL.Path == "/settings/codex/backups":
		h.listCodexBackups(w)
	case r.Method == http.MethodPost && r.URL.Path == "/settings/codex/restore":
		h.restoreCodexBackup(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/settings/codex/backups/") && strings.HasSuffix(r.URL.Path, "/files"):
		h.getCodexBackupFiles(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/settings/proxy/status":
		h.getProxyStatus(w)
	case r.Method == http.MethodPost && r.URL.Path == "/settings/proxy/enable":
		h.enableProxy(w)
	case r.Method == http.MethodPost && r.URL.Path == "/settings/proxy/disable":
		h.disableProxy(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *SettingsHandler) createCodexBackup(w http.ResponseWriter) {
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	backupID, _, err := createCodexBackupSnapshot(home, "backup")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"backup_id":   backupID,
		"backup_path": filepath.Join(codexBackupRoot(home), backupID),
	})
}

func (h *SettingsHandler) exportDatabaseSQL(w http.ResponseWriter) {
	transfer, err := h.sqlTransfer()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	raw, err := transfer.Export()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (h *SettingsHandler) importDatabaseSQL(w http.ResponseWriter, r *http.Request) {
	transfer, err := h.sqlTransfer()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := transfer.Import(raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *SettingsHandler) listDatabaseBackups(w http.ResponseWriter) {
	manager, err := h.dbBackupManager()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := manager.ListBackups()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *SettingsHandler) createDatabaseBackup(w http.ResponseWriter) {
	manager, err := h.dbBackupManager()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	item, err := manager.CreateBackup(h.appSettings().BackupRetentionCount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *SettingsHandler) restoreDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	manager, err := h.dbBackupManager()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var payload codexRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid restore payload", http.StatusBadRequest)
		return
	}
	payload.BackupID = strings.TrimSpace(payload.BackupID)
	if payload.BackupID == "" {
		http.Error(w, "missing backup_id", http.StatusBadRequest)
		return
	}
	if err := manager.RestoreBackup(payload.BackupID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restored_from": payload.BackupID})
}

func (h *SettingsHandler) getAppSettings(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, h.appSettings())
}

func (h *SettingsHandler) saveAppSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		http.Error(w, "settings repository is not configured", http.StatusInternalServerError)
		return
	}

	current := h.appSettings()
	var payload settings.AppSettings
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid app settings payload", http.StatusBadRequest)
		return
	}
	payload = normalizeAppSettings(payload)
	if proxyEndpointChanged(current, payload) {
		if err := h.updateEnabledProxyConfig(payload); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	if err := h.settings.SaveAppSettings(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, h.appSettings())
}

func (h *SettingsHandler) getFailoverQueue(w http.ResponseWriter) {
	if h.settings == nil {
		writeJSON(w, http.StatusOK, []int64{})
		return
	}
	queue, err := h.settings.ListFailoverQueue()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if queue == nil {
		queue = []int64{}
	}
	writeJSON(w, http.StatusOK, queue)
}

func (h *SettingsHandler) saveFailoverQueue(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		http.Error(w, "settings repository is not configured", http.StatusInternalServerError)
		return
	}

	var payload failoverQueueRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid failover queue payload", http.StatusBadRequest)
		return
	}
	if err := h.settings.SaveFailoverQueue(payload.AccountIDs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func createCodexBackupSnapshot(home string, kind string) (string, string, error) {
	backupID := time.Now().Format("20060102-150405.000")
	targetRoot := codexBackupRoot(home)
	if kind == "pre_restore" {
		targetRoot = codexPreRestoreRoot(home)
	}
	targetDir := filepath.Join(targetRoot, backupID)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", "", err
	}

	files := []codexBackupManifestFile{
		{Name: "config.toml", Source: "~/.codex/config.toml"},
		{Name: "auth.json", Source: "~/.codex/auth.json"},
	}
	for _, file := range files {
		source := filepath.Join(home, ".codex", file.Name)
		target := filepath.Join(targetDir, file.Name)
		if kind == "pre_restore" {
			if err := copyOptionalFile(source, target); err != nil {
				return "", "", err
			}
			continue
		}
		if err := copyRequiredFile(source, target); err != nil {
			return "", "", err
		}
	}
	if err := writeManifest(targetDir, codexBackupManifest{
		BackupID:  backupID,
		Kind:      kind,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Files:     files,
	}); err != nil {
		return "", "", err
	}
	return backupID, targetDir, nil
}

func (h *SettingsHandler) enableProxy(w http.ResponseWriter) {
	proxyToggleMu.Lock()
	defer proxyToggleMu.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := readProxySession(home); err == nil {
		http.Error(w, "proxy is already enabled", http.StatusConflict)
		return
	}

	backupID, _, err := createCodexBackupSnapshot(home, "backup")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	content := string(raw)
	currentProvider := currentModelProvider(content)
	updated := content
	mode := "created_aigate_provider"
	targetProvider := "aigate"
	originalBaseURL := ""

	proxyBaseURL := h.proxyBaseURL()
	if isThirdPartyProvider(content, currentProvider) {
		baseURL, ok := getProviderValue(content, currentProvider, "base_url")
		if !ok {
			http.Error(w, "current provider missing base_url; cannot patch provider", http.StatusBadRequest)
			return
		}
		mode = "patched_existing_provider"
		targetProvider = currentProvider
		originalBaseURL = baseURL
		updated = setProviderValue(content, currentProvider, "base_url", strconv.Quote(proxyBaseURL))
	} else {
		updated = setModelProvider(content, "aigate")
		updated = ensureAigateProvider(updated, proxyBaseURL)
	}
	if err := writeAtomic(configPath, []byte(updated), 0o600); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	session := proxySession{
		SessionID:             time.Now().Format("20060102-150405.000"),
		BackupID:              backupID,
		Mode:                  mode,
		TargetProvider:        targetProvider,
		PreviousModelProvider: currentProvider,
		OriginalBaseURL:       originalBaseURL,
		EnabledConfigHash:     hashString(updated),
		CreatedAt:             time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeProxySession(home, session); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, _ := readProxyState(home)
	state.LastBackupID = backupID
	state.SessionID = session.SessionID
	_ = writeProxyState(home, state)

	writeJSON(w, http.StatusOK, proxyStatusResponse{
		Enabled:        true,
		LastBackupID:   backupID,
		Mode:           session.Mode,
		TargetProvider: session.TargetProvider,
		Host:           h.appSettings().ProxyHost,
		Port:           h.appSettings().ProxyPort,
	})
}

func (h *SettingsHandler) disableProxy(w http.ResponseWriter, r *http.Request) {
	proxyToggleMu.Lock()
	defer proxyToggleMu.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	session, err := readProxySession(home)
	if err != nil {
		http.Error(w, "proxy is not enabled", http.StatusConflict)
		return
	}
	configPath := filepath.Join(home, ".codex", "config.toml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	configChanged := hashBytes(raw) != session.EnabledConfigHash
	disableMode := resolveDisableMode(r)
	forcedRestore := isForceDisable(r.URL.Query().Get("force"))
	if disableMode == "restore" && configChanged && !forcedRestore {
		http.Error(w, "config.toml changed externally; skip auto-restore to avoid overwrite", http.StatusConflict)
		return
	}
	if disableMode == "detach" {
		// User chose "close without overwrite": keep current config untouched.
	} else {
		backupDir := filepath.Join(codexBackupRoot(home), session.BackupID)
		for _, name := range []string{"config.toml", "auth.json"} {
			source := filepath.Join(backupDir, name)
			target := filepath.Join(home, ".codex", name)
			if err := copyRequiredFile(source, target); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
	}
	clearProxySession(home)
	state, _ := readProxyState(home)
	writeJSON(w, http.StatusOK, proxyStatusResponse{
		Enabled:      false,
		LastBackupID: state.LastBackupID,
		Host:         h.appSettings().ProxyHost,
		Port:         h.appSettings().ProxyPort,
	})
}

func (h *SettingsHandler) getProxyStatus(w http.ResponseWriter) {
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, _ := readProxyState(home)
	session, sessionErr := readProxySession(home)
	appSettings := h.appSettings()
	resp := proxyStatusResponse{
		Enabled:      sessionErr == nil,
		LastBackupID: state.LastBackupID,
		Host:         appSettings.ProxyHost,
		Port:         appSettings.ProxyPort,
	}
	if sessionErr == nil {
		resp.Mode = session.Mode
		resp.TargetProvider = session.TargetProvider
		configPath := filepath.Join(home, ".codex", "config.toml")
		if raw, err := os.ReadFile(configPath); err == nil {
			resp.ConfigConflict = hashBytes(raw) != session.EnabledConfigHash
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *SettingsHandler) appSettings() settings.AppSettings {
	if h.settings == nil {
		return settings.DefaultAppSettings()
	}
	value, err := h.settings.GetAppSettings()
	if err != nil {
		return settings.DefaultAppSettings()
	}
	return value
}

func (h *SettingsHandler) proxyBaseURL() string {
	value := h.appSettings()
	return "http://" + net.JoinHostPort(value.ProxyHost, strconv.Itoa(value.ProxyPort)) + "/ai-router/api"
}

func proxyBaseURLForSettings(value settings.AppSettings) string {
	return "http://" + net.JoinHostPort(value.ProxyHost, strconv.Itoa(value.ProxyPort)) + "/ai-router/api"
}

func proxyEndpointChanged(current settings.AppSettings, next settings.AppSettings) bool {
	return strings.TrimSpace(current.ProxyHost) != strings.TrimSpace(next.ProxyHost) || current.ProxyPort != next.ProxyPort
}

func normalizeAppSettings(value settings.AppSettings) settings.AppSettings {
	defaults := settings.DefaultAppSettings()
	if strings.TrimSpace(value.ProxyHost) == "" {
		value.ProxyHost = defaults.ProxyHost
	}
	if value.ProxyPort <= 0 {
		value.ProxyPort = defaults.ProxyPort
	}
	if value.AutoBackupIntervalHours <= 0 {
		value.AutoBackupIntervalHours = defaults.AutoBackupIntervalHours
	}
	if value.BackupRetentionCount <= 0 {
		value.BackupRetentionCount = defaults.BackupRetentionCount
	}
	return value
}

func (h *SettingsHandler) updateEnabledProxyConfig(value settings.AppSettings) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}
	session, err := readProxySession(home)
	if err != nil {
		return nil
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read codex config: %w", err)
	}
	if hashBytes(raw) != session.EnabledConfigHash {
		return fmt.Errorf("proxy config changed externally; disable proxy before changing host or port")
	}

	updated := string(raw)
	proxyBaseURL := proxyBaseURLForSettings(value)
	if session.Mode == "patched_existing_provider" {
		targetProvider := strings.TrimSpace(session.TargetProvider)
		if targetProvider == "" {
			return fmt.Errorf("proxy session missing target provider")
		}
		updated = setProviderValue(updated, targetProvider, "base_url", strconv.Quote(proxyBaseURL))
	} else {
		updated = setModelProvider(updated, "aigate")
		updated = ensureAigateProvider(updated, proxyBaseURL)
	}

	if err := writeAtomic(configPath, []byte(updated), 0o600); err != nil {
		return err
	}
	session.EnabledConfigHash = hashString(updated)
	return writeProxySession(home, session)
}

func (h *SettingsHandler) sqlTransfer() (*settings.SQLTransfer, error) {
	if h.db == nil {
		return nil, errors.New("database is not configured")
	}
	return settings.NewSQLTransfer(h.db), nil
}

func (h *SettingsHandler) dbBackupManager() (*settings.DBBackupManager, error) {
	if h.db == nil || strings.TrimSpace(h.dbPath) == "" {
		return nil, errors.New("database is not configured")
	}
	return settings.NewDBBackupManager(h.db, h.dbPath), nil
}

func IsProxyEnabled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = readProxySession(home)
	return err == nil
}

func (h *SettingsHandler) listCodexBackups(w http.ResponseWriter) {
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	root := codexBackupRoot(home)
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusOK, []codexBackupItem{})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []codexBackupItem
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		backupID := entry.Name()
		manifestPath := filepath.Join(root, backupID, "manifest.json")
		manifestRaw, readErr := os.ReadFile(manifestPath)
		if readErr != nil {
			continue
		}
		var manifest codexBackupManifest
		if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
			continue
		}
		items = append(items, codexBackupItem{
			BackupID:  backupID,
			CreatedAt: manifest.CreatedAt,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].BackupID > items[j].BackupID
	})
	writeJSON(w, http.StatusOK, items)
}

func (h *SettingsHandler) restoreCodexBackup(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req codexRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.BackupID = strings.TrimSpace(req.BackupID)
	if req.BackupID == "" {
		http.Error(w, "missing backup_id", http.StatusBadRequest)
		return
	}

	backupDir := filepath.Join(codexBackupRoot(home), req.BackupID)
	if _, err := os.Stat(backupDir); err != nil {
		http.Error(w, "backup_id not found", http.StatusNotFound)
		return
	}

	preRestoreID, _, err := createCodexBackupSnapshot(home, "pre_restore")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	restoreFiles := []string{"config.toml", "auth.json"}
	for _, name := range restoreFiles {
		source := filepath.Join(backupDir, name)
		target := filepath.Join(codexDir, name)
		if err := copyRequiredFile(source, target); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"ok":             "true",
		"restored_from":  req.BackupID,
		"pre_restore_id": preRestoreID,
	})
}

func (h *SettingsHandler) getCodexBackupFiles(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	backupID, ok := extractBackupID(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	backupDir := filepath.Join(codexBackupRoot(home), backupID)
	files := map[string]string{}
	for _, name := range []string{"config.toml", "auth.json", "manifest.json"} {
		raw, err := os.ReadFile(filepath.Join(backupDir, name))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		files[name] = string(raw)
	}

	writeJSON(w, http.StatusOK, codexBackupFilesResponse{
		BackupID: backupID,
		Files:    files,
	})
}

func extractBackupID(path string) (string, bool) {
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	// settings/codex/backups/{backup_id}/files
	if len(parts) != 5 || parts[0] != "settings" || parts[1] != "codex" || parts[2] != "backups" || parts[4] != "files" {
		return "", false
	}
	id := strings.TrimSpace(parts[3])
	return id, id != ""
}

func aigateDataRoot(home string) string {
	return filepath.Join(home, ".aigate", "data")
}

func codexBackupRoot(home string) string {
	return filepath.Join(aigateDataRoot(home), "codex", "backup")
}

func codexPreRestoreRoot(home string) string {
	return filepath.Join(aigateDataRoot(home), "codex", "pre-restore")
}

func proxyStatePath(home string) string {
	return filepath.Join(aigateDataRoot(home), "codex", "proxy-state.json")
}

func proxySessionPath(home string) string {
	return filepath.Join(aigateDataRoot(home), "codex", "proxy-session.json")
}

func readProxyState(home string) (proxyState, error) {
	raw, err := os.ReadFile(proxyStatePath(home))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return proxyState{}, nil
		}
		return proxyState{}, err
	}
	var state proxyState
	if err := json.Unmarshal(raw, &state); err != nil {
		return proxyState{}, err
	}
	return state, nil
}

func writeProxyState(home string, state proxyState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(proxyStatePath(home)), 0o755); err != nil {
		return err
	}
	return writeAtomic(proxyStatePath(home), raw, 0o600)
}

func readProxySession(home string) (proxySession, error) {
	raw, err := os.ReadFile(proxySessionPath(home))
	if err != nil {
		return proxySession{}, err
	}
	var session proxySession
	if err := json.Unmarshal(raw, &session); err != nil {
		return proxySession{}, err
	}
	return session, nil
}

func writeProxySession(home string, session proxySession) error {
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(proxySessionPath(home)), 0o755); err != nil {
		return err
	}
	return writeAtomic(proxySessionPath(home), raw, 0o600)
}

func setModelProvider(content string, provider string) string {
	re := regexp.MustCompile(`(?m)^model_provider\s*=\s*".*"\s*$`)
	line := `model_provider = "` + provider + `"`
	if re.MatchString(content) {
		return re.ReplaceAllString(content, line)
	}
	return line + "\n\n" + strings.TrimLeft(content, "\n")
}

func aigateProviderBlock(proxyBaseURL string) string {
	return `[model_providers.aigate]
name = "aigate"
base_url = "` + proxyBaseURL + `"
wire_api = "responses"
requires_openai_auth = true
store = false`
}

func hashString(value string) string {
	return hashBytes([]byte(value))
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return fmt.Sprintf("%x", sum[:])
}

func writeAtomic(path string, raw []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp-" + time.Now().Format("150405.000")
	if err := os.WriteFile(tmp, raw, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func isForceDisable(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func isSkipRestore(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func clearProxySession(home string) {
	_ = os.Remove(proxySessionPath(home))
	state, _ := readProxyState(home)
	state.SessionID = ""
	_ = writeProxyState(home, state)
}

func resolveDisableMode(r *http.Request) string {
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "detach" {
		return "detach"
	}
	if isSkipRestore(r.URL.Query().Get("skip_restore")) {
		return "detach"
	}
	return "restore"
}

func currentModelProvider(content string) string {
	re := regexp.MustCompile(`(?m)^model_provider\s*=\s*"([^"]+)"\s*$`)
	match := re.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func isThirdPartyProvider(content string, provider string) bool {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return false
	}
	switch provider {
	case "openai", "aigate":
		return false
	}
	baseURL, ok := getProviderValue(content, provider, "base_url")
	if !ok {
		return false
	}
	lower := strings.ToLower(baseURL)
	if strings.Contains(lower, "chatgpt.com/backend-api/codex") || strings.Contains(lower, "api.openai.com") {
		return false
	}
	return true
}

func ensureAigateProvider(content string, proxyBaseURL string) string {
	// Always normalize to a single canonical [model_providers.aigate] section.
	// This avoids duplicate-section parse errors from historical/broken config files.
	withoutAigate := removeAigateProviderDefinitions(content)
	base := strings.TrimSpace(withoutAigate)
	if base == "" {
		return aigateProviderBlock(proxyBaseURL) + "\n"
	}
	return base + "\n\n" + aigateProviderBlock(proxyBaseURL) + "\n"
}

func hasProviderSection(content string, provider string) bool {
	header := "[model_providers." + provider + "]"
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == header {
			return true
		}
	}
	return false
}

func getProviderValue(content string, provider string, key string) (string, bool) {
	lines := strings.Split(content, "\n")
	header := "[model_providers." + provider + "]"
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = trimmed == header
			continue
		}
		if !inSection {
			continue
		}
		re := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `\s*=\s*(.+)$`)
		if match := re.FindStringSubmatch(trimmed); len(match) == 2 {
			value := strings.TrimSpace(match[1])
			value = strings.Trim(value, `"`)
			return value, true
		}
	}
	return "", false
}

func removeProviderSections(content string, provider string) string {
	lines := strings.Split(content, "\n")
	header := "[model_providers." + provider + "]"
	var kept []string
	inTarget := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if trimmed == header {
				inTarget = true
				continue
			}
			inTarget = false
		}
		if inTarget {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func removeAigateProviderDefinitions(content string) string {
	lines := strings.Split(content, "\n")
	const targetSection = "[model_providers.aigate]"
	aigateInlineRE := regexp.MustCompile(`^aigate\s*=`)
	inAigateSection := false
	inModelProvidersSection := false
	var kept []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inAigateSection = false
			inModelProvidersSection = false
			if trimmed == targetSection || strings.HasPrefix(trimmed, "[model_providers.aigate.") {
				inAigateSection = true
				continue
			}
			if trimmed == "[model_providers]" {
				inModelProvidersSection = true
			}
		}
		if inAigateSection {
			continue
		}
		// Remove dotted-key definitions like:
		// model_providers.aigate.base_url = "..."
		if strings.HasPrefix(trimmed, "model_providers.aigate.") {
			continue
		}
		// Remove inline table definitions under [model_providers]:
		// aigate = { ... }
		if inModelProvidersSection {
			if aigateInlineRE.MatchString(trimmed) {
				continue
			}
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func setProviderValue(content string, provider string, key string, valueExpr string) string {
	lines := strings.Split(content, "\n")
	header := "[model_providers." + provider + "]"
	inSection := false
	sectionStart := -1
	sectionEnd := len(lines)
	keyLine := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if inSection {
				sectionEnd = i
				break
			}
			if trimmed == header {
				inSection = true
				sectionStart = i
			}
			continue
		}
		if !inSection {
			continue
		}
		re := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `\s*=`)
		if re.MatchString(trimmed) {
			keyLine = i
		}
	}

	assign := key + " = " + valueExpr
	if sectionStart == -1 {
		body := strings.TrimSpace(content)
		if body == "" {
			return header + "\n" + assign + "\n"
		}
		return body + "\n\n" + header + "\n" + assign + "\n"
	}
	if keyLine >= 0 {
		lines[keyLine] = assign
		return strings.Join(lines, "\n")
	}
	insertAt := sectionEnd
	if insertAt < 0 || insertAt > len(lines) {
		insertAt = len(lines)
	}
	lines = append(lines[:insertAt], append([]string{assign}, lines[insertAt:]...)...)
	return strings.Join(lines, "\n")
}

func writeManifest(dir string, manifest codexBackupManifest) error {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o600)
}

func copyRequiredFile(source string, target string) error {
	raw, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", source, err)
	}
	if err := os.WriteFile(target, raw, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

func copyOptionalFile(source string, target string) error {
	raw, err := os.ReadFile(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", source, err)
	}
	if err := os.WriteFile(target, raw, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}
