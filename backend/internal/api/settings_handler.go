package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type SettingsHandler struct{}

func NewSettingsHandler() *SettingsHandler {
	return &SettingsHandler{}
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

type codexBackupFilesResponse struct {
	BackupID string            `json:"backup_id"`
	Files    map[string]string `json:"files"`
}

type proxyState struct {
	LastBackupID string `json:"last_backup_id"`
	SessionID    string `json:"session_id,omitempty"`
}

type proxySession struct {
	SessionID         string `json:"session_id"`
	BackupID          string `json:"backup_id"`
	EnabledConfigHash string `json:"enabled_config_hash"`
	CreatedAt         string `json:"created_at"`
}

type proxyStatusResponse struct {
	Enabled      bool   `json:"enabled"`
	LastBackupID string `json:"last_backup_id,omitempty"`
}

func (h *SettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
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
	if strings.Contains(content, "[model_providers.aigate]") {
		http.Error(w, "config.toml already contains [model_providers.aigate]", http.StatusConflict)
		return
	}
	updated := setModelProvider(content, "aigate")
	updated = strings.TrimSpace(updated) + "\n\n" + aigateProviderBlock()
	if err := writeAtomic(configPath, []byte(updated), 0o600); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	session := proxySession{
		SessionID:         time.Now().Format("20060102-150405.000"),
		BackupID:          backupID,
		EnabledConfigHash: hashString(updated),
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
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
		Enabled:      true,
		LastBackupID: backupID,
	})
}

func (h *SettingsHandler) disableProxy(w http.ResponseWriter, r *http.Request) {
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
	if hashBytes(raw) != session.EnabledConfigHash {
		if isForceDisable(r.URL.Query().Get("force")) {
			_ = os.Remove(proxySessionPath(home))
			state, _ := readProxyState(home)
			state.SessionID = ""
			_ = writeProxyState(home, state)
			writeJSON(w, http.StatusOK, proxyStatusResponse{
				Enabled:      false,
				LastBackupID: state.LastBackupID,
			})
			return
		}
		http.Error(w, "config.toml changed externally; skip auto-restore to avoid overwrite", http.StatusConflict)
		return
	}
	backupDir := filepath.Join(codexBackupRoot(home), session.BackupID)
	for _, name := range []string{"config.toml", "auth.json"} {
		source := filepath.Join(backupDir, name)
		target := filepath.Join(home, ".codex", name)
		if err := copyRequiredFile(source, target); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	_ = os.Remove(proxySessionPath(home))
	state, _ := readProxyState(home)
	state.SessionID = ""
	_ = writeProxyState(home, state)
	writeJSON(w, http.StatusOK, proxyStatusResponse{
		Enabled:      false,
		LastBackupID: state.LastBackupID,
	})
}

func (h *SettingsHandler) getProxyStatus(w http.ResponseWriter) {
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, _ := readProxyState(home)
	_, sessionErr := readProxySession(home)
	writeJSON(w, http.StatusOK, proxyStatusResponse{
		Enabled:      sessionErr == nil,
		LastBackupID: state.LastBackupID,
	})
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

func aigateProviderBlock() string {
	return `[model_providers.aigate]
name = "aigate"
base_url = "http://127.0.0.1:6789/ai-router/api"
wire_api = "responses"
requires_openai_auth = true`
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
