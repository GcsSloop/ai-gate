package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
)

const toolingConfigFilename = "tooling.json"

var toolingSupportedApps = []string{"codex"}

type ToolingHandler struct {
	mu sync.Mutex
}

func NewToolingHandler() *ToolingHandler {
	return &ToolingHandler{}
}

type toolingConfig struct {
	SkillSyncMethod string             `json:"skill_sync_method"`
	SkillRepos      []skillRepoRecord  `json:"skill_repos"`
	McpServers      []managedMcpServer `json:"mcp_servers"`
}

type skillRepoRecord struct {
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	Branch     string `json:"branch"`
	Enabled    bool   `json:"enabled"`
	SkillCount int    `json:"skill_count"`
}

type managedMcpServer struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	TemplateID  string          `json:"template_id,omitempty"`
	EnabledApps map[string]bool `json:"enabled_apps"`
	Spec        map[string]any  `json:"spec"`
}

type toolingStateResponse struct {
	SkillSyncMethod      string                           `json:"skill_sync_method"`
	Clients              []toolingClientState             `json:"clients"`
	SkillStats           skillStatsResponse               `json:"skill_stats"`
	SkillRepos           []skillRepoRecord                `json:"skill_repos"`
	InstalledSkills      []managedSkillRecord             `json:"installed_skills"`
	RepoSearchResults    []repoSearchResult               `json:"repo_search_results"`
	DiscoveredMcpServers []toolingDiscoveredMcpServerView `json:"discovered_mcp_servers"`
	McpTemplates         []mcpTemplateRecord              `json:"mcp_templates"`
	McpServers           []toolingMcpServerView           `json:"mcp_servers"`
}

type toolingClientState struct {
	App         string `json:"app"`
	Label       string `json:"label"`
	SkillsDir   string `json:"skills_dir"`
	McpPath     string `json:"mcp_path"`
	SkillsCount int    `json:"skills_count"`
	McpStatus   string `json:"mcp_status"`
}

type skillStatsResponse struct {
	Total    int            `json:"total"`
	BySource map[string]int `json:"by_source"`
}

type managedSkillRecord struct {
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	Directory     string          `json:"directory"`
	SourceClient  string          `json:"source_client,omitempty"`
	SourceRepo    string          `json:"source_repo,omitempty"`
	SourceKind    string          `json:"source_kind"`
	ManagedPath   string          `json:"managed_path"`
	InstalledApps map[string]bool `json:"installed_apps"`
}

type repoSearchResult struct {
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Branch      string `json:"branch"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type mcpTemplateRecord struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Command     string   `json:"command,omitempty"`
	Args        []string `json:"args,omitempty"`
	URL         string   `json:"url,omitempty"`
}

type toolingMcpServerView struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	TemplateID    string            `json:"template_id,omitempty"`
	EnabledApps   map[string]bool   `json:"enabled_apps"`
	ClientStatus  map[string]string `json:"client_status"`
	DeleteAllowed bool              `json:"delete_allowed"`
	DeleteReason  string            `json:"delete_reason,omitempty"`
	DeleteTargets []string          `json:"delete_targets,omitempty"`
	Spec          map[string]any    `json:"spec"`
}

type toolingMcpDeletePlan struct {
	Allowed      bool
	Reason       string
	CleanupRoots []string
}

type toolingDiscoveredMcpServerView struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	SourceApps   map[string]bool   `json:"source_apps"`
	ClientStatus map[string]string `json:"client_status"`
	Spec         map[string]any    `json:"spec"`
}

type toolingImportRequest struct {
	Source string `json:"source"`
}

type toolingApplyRequest struct {
	Apps   []string `json:"apps"`
	Method string   `json:"method,omitempty"`
}

type toolingSkillUpdateRequest struct {
	Apps    []string `json:"apps"`
	Method  string   `json:"method,omitempty"`
	Enabled bool     `json:"enabled"`
}

type toolingRepoRequest struct {
	Owner  string `json:"owner"`
	Name   string `json:"name"`
	Branch string `json:"branch"`
}

type toolingRepoSearchResponse struct {
	Items []repoSearchResult `json:"items"`
}

type toolingMcpInstallRequest struct {
	ID          string          `json:"id"`
	TemplateID  string          `json:"template_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	EnabledApps map[string]bool `json:"enabled_apps"`
}

type mcpConfigPayload struct {
	McpServers map[string]any `json:"mcpServers"`
	Mcp        map[string]any `json:"mcp"`
}

var toolingDefaultRepos = []skillRepoRecord{}

var toolingTemplates = []mcpTemplateRecord{
	{ID: "fetch", Name: "mcp-server-fetch", Description: "Quick template: mcp-fetch", Type: "stdio", Command: "uvx", Args: []string{"mcp-server-fetch"}},
	{ID: "time", Name: "@modelcontextprotocol/server-time", Description: "Quick template: time server", Type: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-time"}},
	{ID: "memory", Name: "@modelcontextprotocol/server-memory", Description: "Quick template: memory server", Type: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-memory"}},
	{ID: "sequential-thinking", Name: "@modelcontextprotocol/server-sequential-thinking", Description: "Quick template: sequential thinking", Type: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-sequential-thinking"}},
	{ID: "context7", Name: "@upstash/context7-mcp", Description: "Quick template: context7", Type: "stdio", Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
}

func (h *ToolingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/tooling/state":
		h.getState(w)
	case r.Method == http.MethodPut && r.URL.Path == "/tooling/settings":
		h.updateSettings(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/skills/import":
		h.importSkills(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/skills/apply":
		h.applySkills(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/tooling/skills/"):
		h.updateSkill(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/tooling/skills/repos":
		h.listSkillRepos(w)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/skills/repos":
		h.addSkillRepo(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/tooling/skills/repos/"):
		h.removeSkillRepo(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/tooling/skills/"):
		h.deleteSkill(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/tooling/skills/repos/search":
		h.searchSkillRepos(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/tooling/mcp/templates":
		h.listMcpTemplates(w)
	case r.Method == http.MethodGet && r.URL.Path == "/tooling/mcp/servers":
		h.listMcpServers(w)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/mcp/import":
		h.importMcpServers(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/mcp/install":
		h.installMcpServer(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/tooling/mcp/servers/"):
		h.updateMcpServer(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/tooling/mcp/servers/"):
		h.deleteMcpServer(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/mcp/apply":
		h.applyMcpServer(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *ToolingHandler) getState(w http.ResponseWriter) {
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cfg := h.loadConfig(home)
	cfg, _ = h.syncManagedCodexMcpServers(home, cfg)
	clients := toolingClientStates(home)
	skills := scanManagedSkills(home, clients)
	repoResults := defaultRepoSearchResults()
	discoveredServers := discoverMcpServers(home)
	servers := h.buildMcpViews(home, cfg)

	writeJSON(w, http.StatusOK, toolingStateResponse{
		SkillSyncMethod:      normalizeSkillSyncMethod(cfg.SkillSyncMethod),
		Clients:              clients,
		SkillStats:           buildSkillStats(skills),
		SkillRepos:           cfg.SkillRepos,
		InstalledSkills:      skills,
		RepoSearchResults:    repoResults,
		DiscoveredMcpServers: discoveredServers,
		McpTemplates:         toolingTemplates,
		McpServers:           servers,
	})
}

func (h *ToolingHandler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SkillSyncMethod string `json:"skill_sync_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := h.loadConfig(home)
	cfg.SkillSyncMethod = normalizeSkillSyncMethod(req.SkillSyncMethod)
	if err := h.saveConfig(home, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skill_sync_method": cfg.SkillSyncMethod})
}

func (h *ToolingHandler) importSkills(w http.ResponseWriter, r *http.Request) {
	var req toolingImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	imported, err := importSkillsFromClient(home, req.Source)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported})
}

func (h *ToolingHandler) applySkills(w http.ResponseWriter, r *http.Request) {
	var req toolingApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := h.loadConfig(home)
	method := normalizeSkillSyncMethod(req.Method)
	if method == "" {
		method = normalizeSkillSyncMethod(cfg.SkillSyncMethod)
	}
	applied, err := applyManagedSkills(home, method, reqApps(req.Apps))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg.SkillSyncMethod = method
	if err := h.saveConfig(home, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"applied": applied, "skill_sync_method": method})
}

func (h *ToolingHandler) updateSkill(w http.ResponseWriter, r *http.Request) {
	name, err := decodeToolingSkillName(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req toolingSkillUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req.Enabled {
		cfg := h.loadConfig(home)
		method := normalizeSkillSyncMethod(req.Method)
		if method == "" {
			method = normalizeSkillSyncMethod(cfg.SkillSyncMethod)
		}
		applied, err := applyManagedSkillCollection(home, name, method, reqApps(req.Apps))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Method != "" {
			cfg.SkillSyncMethod = method
			if err := h.saveConfig(home, cfg); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"applied": applied, "enabled": true, "skill_sync_method": method})
		return
	}
	if err := removeSkillCollectionFromClients(home, name, reqApps(req.Apps)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"applied": 0, "enabled": false})
}

func (h *ToolingHandler) deleteSkill(w http.ResponseWriter, r *http.Request) {
	name, err := decodeToolingSkillName(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := deleteManagedSkillCollection(home, name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *ToolingHandler) listSkillRepos(w http.ResponseWriter) {
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := h.loadConfig(home)
	writeJSON(w, http.StatusOK, cfg.SkillRepos)
}

func (h *ToolingHandler) addSkillRepo(w http.ResponseWriter, r *http.Request) {
	var req toolingRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Owner) == "" || strings.TrimSpace(req.Name) == "" {
		http.Error(w, "owner and name are required", http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := h.loadConfig(home)
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}
	next := skillRepoRecord{Owner: strings.TrimSpace(req.Owner), Name: strings.TrimSpace(req.Name), Branch: branch, Enabled: true}
	for idx, repo := range cfg.SkillRepos {
		if strings.EqualFold(repo.Owner, next.Owner) && strings.EqualFold(repo.Name, next.Name) {
			cfg.SkillRepos[idx] = next
			if err := h.saveConfig(home, cfg); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, next)
			return
		}
	}
	cfg.SkillRepos = append(cfg.SkillRepos, next)
	if err := h.saveConfig(home, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, next)
}

func (h *ToolingHandler) removeSkillRepo(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/tooling/skills/repos/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		http.Error(w, "invalid repo path", http.StatusBadRequest)
		return
	}
	owner := parts[0]
	name := parts[1]
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := h.loadConfig(home)
	next := make([]skillRepoRecord, 0, len(cfg.SkillRepos))
	removed := false
	for _, repo := range cfg.SkillRepos {
		if strings.EqualFold(repo.Owner, owner) && strings.EqualFold(repo.Name, name) {
			removed = true
			continue
		}
		next = append(next, repo)
	}
	cfg.SkillRepos = next
	if removed {
		if err := h.saveConfig(home, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
}

func (h *ToolingHandler) searchSkillRepos(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusOK, toolingRepoSearchResponse{Items: defaultRepoSearchResults()})
		return
	}
	items, err := searchGitHubRepos(query)
	if err != nil {
		writeJSON(w, http.StatusOK, toolingRepoSearchResponse{Items: defaultRepoSearchResults()})
		return
	}
	writeJSON(w, http.StatusOK, toolingRepoSearchResponse{Items: items})
}

func (h *ToolingHandler) listMcpTemplates(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, toolingTemplates)
}

func (h *ToolingHandler) listMcpServers(w http.ResponseWriter) {
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := h.loadConfig(home)
	writeJSON(w, http.StatusOK, h.buildMcpViews(home, cfg))
}

func (h *ToolingHandler) importMcpServers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	imported, err := importMcpServersFromClients(home, req.Source)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported})
}

func (h *ToolingHandler) installMcpServer(w http.ResponseWriter, r *http.Request) {
	var req toolingMcpInstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	template, ok := findTemplate(req.TemplateID)
	if !ok {
		http.Error(w, "template not found", http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := h.loadConfig(home)
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = template.ID
	}
	server := managedMcpServer{
		ID:          id,
		Name:        firstNonEmpty(strings.TrimSpace(req.Name), template.Name, id),
		Description: firstNonEmpty(strings.TrimSpace(req.Description), template.Description),
		TemplateID:  template.ID,
		EnabledApps: defaultEnabledApps(req.EnabledApps),
		Spec:        templateToSpec(template),
	}
	cfg.upsertServer(server)
	if err := h.saveConfig(home, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := applyMcpServerToClients(home, server, enabledAppsList(server.EnabledApps)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, server)
}

func importMcpServersFromClients(home string, source string) (int, error) {
	sources := toolingSupportedApps
	if trimmed := strings.ToLower(strings.TrimSpace(source)); trimmed != "" {
		if trimmed != "codex" {
			return 0, fmt.Errorf("unknown mcp source: %s", source)
		}
		sources = []string{trimmed}
	}
	cfg := loadToolingConfig(home)
	imported := 0
	for _, app := range sources {
		client := toolingClients(home)[app]
		servers, err := readClientMcpServers(app, client.mcpPath)
		if err != nil {
			return imported, err
		}
		for id, spec := range servers {
			name := firstNonEmpty(serverNameFromSpec(spec), id)
			desc := serverDescriptionFromSpec(spec)
			next := managedMcpServer{
				ID:          id,
				Name:        name,
				Description: desc,
				EnabledApps: map[string]bool{app: true},
				Spec:        spec,
			}
			if existing, ok := cfg.serverByID(id); ok {
				next = mergeManagedMcpServer(existing, next)
			}
			cfg.upsertServer(next)
			imported++
		}
	}
	if err := saveToolingConfig(home, cfg); err != nil {
		return imported, err
	}
	return imported, nil
}

func (h *ToolingHandler) updateMcpServer(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/tooling/mcp/servers/"))
	if id == "" {
		http.Error(w, "missing server id", http.StatusBadRequest)
		return
	}
	var req managedMcpServer
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := h.loadConfig(home)
	req.ID = id
	req.EnabledApps = defaultEnabledApps(req.EnabledApps)
	if req.Spec == nil {
		req.Spec = map[string]any{}
	}
	cfg.upsertServer(req)
	if err := h.saveConfig(home, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := applyMcpServerToClients(home, req, enabledAppsList(req.EnabledApps)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (h *ToolingHandler) deleteMcpServer(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/tooling/mcp/servers/"))
	if id == "" {
		http.Error(w, "missing server id", http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := h.loadConfig(home)
	server, ok := cfg.serverByID(id)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	deletePlan := buildMcpDeletePlan(home, server)
	if !deletePlan.Allowed {
		http.Error(w, deletePlan.Reason, http.StatusBadRequest)
		return
	}
	cleanupLocalFiles := parseBoolQueryValue(r.URL.Query().Get("cleanup_local_files"))
	cfg.removeServer(id)
	if err := h.saveConfig(home, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := removeMcpServerFromClients(home, id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cleanupLocalFiles {
		if err := removeManagedMcpRoots(deletePlan.CleanupRoots); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *ToolingHandler) applyMcpServer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string   `json:"id"`
		Apps    []string `json:"apps"`
		Enabled *bool    `json:"enabled,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := h.loadConfig(home)
	server, ok := cfg.serverByID(req.ID)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	apps := enabledAppsList(server.EnabledApps)
	if len(req.Apps) > 0 {
		apps = reqApps(req.Apps)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if enabled {
		for _, app := range apps {
			server.EnabledApps[app] = true
		}
		cfg.upsertServer(server)
		if err := h.saveConfig(home, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := applyMcpServerToClients(home, server, apps); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"applied": true})
		return
	}
	for _, app := range apps {
		server.EnabledApps[app] = false
	}
	cfg.upsertServer(server)
	if err := h.saveConfig(home, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, app := range apps {
		client, ok := toolingClients(home)[app]
		if !ok {
			continue
		}
		switch app {
		case "codex":
			if err := removeCodexMcp(client.mcpPath, server.ID); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"applied": false})
}

func (c toolingConfig) serverByID(id string) (managedMcpServer, bool) {
	for _, server := range c.McpServers {
		if strings.EqualFold(server.ID, id) {
			return server, true
		}
	}
	return managedMcpServer{}, false
}

func (c *toolingConfig) upsertServer(server managedMcpServer) {
	for idx, existing := range c.McpServers {
		if strings.EqualFold(existing.ID, server.ID) {
			c.McpServers[idx] = server
			return
		}
	}
	c.McpServers = append(c.McpServers, server)
}

func (c *toolingConfig) removeServer(id string) {
	next := c.McpServers[:0]
	for _, server := range c.McpServers {
		if !strings.EqualFold(server.ID, id) {
			next = append(next, server)
		}
	}
	c.McpServers = next
}

func mergeManagedMcpServer(existing managedMcpServer, imported managedMcpServer) managedMcpServer {
	merged := existing
	if strings.TrimSpace(merged.Name) == "" {
		merged.Name = imported.Name
	}
	if strings.TrimSpace(merged.Description) == "" {
		merged.Description = imported.Description
	}
	if strings.TrimSpace(merged.TemplateID) == "" {
		merged.TemplateID = imported.TemplateID
	}
	if merged.Spec == nil || len(merged.Spec) == 0 {
		merged.Spec = imported.Spec
	}
	if merged.EnabledApps == nil {
		merged.EnabledApps = map[string]bool{}
	}
	for key, value := range imported.EnabledApps {
		if value {
			merged.EnabledApps[key] = true
		}
	}
	return merged
}

func loadToolingConfig(home string) toolingConfig {
	return new(ToolingHandler).loadConfig(home)
}

func saveToolingConfig(home string, cfg toolingConfig) error {
	return new(ToolingHandler).saveConfig(home, cfg)
}

func (h *ToolingHandler) loadConfig(home string) toolingConfig {
	path := toolingConfigPath(home)
	raw, err := os.ReadFile(path)
	if err != nil {
		return toolingConfig{
			SkillSyncMethod: "symlink",
			SkillRepos:      append([]skillRepoRecord{}, toolingDefaultRepos...),
			McpServers:      []managedMcpServer{},
		}
	}
	var cfg toolingConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return toolingConfig{
			SkillSyncMethod: "symlink",
			SkillRepos:      append([]skillRepoRecord{}, toolingDefaultRepos...),
			McpServers:      []managedMcpServer{},
		}
	}
	cfg.SkillSyncMethod = normalizeSkillSyncMethod(cfg.SkillSyncMethod)
	if len(cfg.SkillRepos) == 0 {
		cfg.SkillRepos = append([]skillRepoRecord{}, toolingDefaultRepos...)
	}
	return cfg
}

func (h *ToolingHandler) saveConfig(home string, cfg toolingConfig) error {
	cfg.SkillSyncMethod = normalizeSkillSyncMethod(cfg.SkillSyncMethod)
	if len(cfg.SkillRepos) == 0 {
		cfg.SkillRepos = append([]skillRepoRecord{}, toolingDefaultRepos...)
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := toolingConfigPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeAtomic(path, append(raw, '\n'), 0o600)
}

func toolingConfigPath(home string) string {
	return filepath.Join(aigateDataRoot(home), "tooling", toolingConfigFilename)
}

func toolingClients(home string) map[string]struct {
	label     string
	skillsDir string
	mcpPath   string
} {
	return map[string]struct {
		label     string
		skillsDir string
		mcpPath   string
	}{
		"codex": {
			label:     "Codex",
			skillsDir: filepath.Join(home, ".codex", "skills"),
			mcpPath:   filepath.Join(home, ".codex", "config.toml"),
		},
	}
}

func toolingClientStates(home string) []toolingClientState {
	clients := toolingClients(home)
	result := make([]toolingClientState, 0, len(toolingSupportedApps))
	for _, key := range toolingSupportedApps {
		client := clients[key]
		skillsCount := countSkillDirs(client.skillsDir)
		mcpStatus := "missing"
		if pathExists(client.mcpPath) {
			mcpStatus = "ready"
		}
		result = append(result, toolingClientState{
			App:         key,
			Label:       client.label,
			SkillsDir:   client.skillsDir,
			McpPath:     client.mcpPath,
			SkillsCount: skillsCount,
			McpStatus:   mcpStatus,
		})
	}
	return result
}

func normalizeSkillSyncMethod(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "copy":
		return "copy"
	default:
		return "symlink"
	}
}

func reqApps(apps []string) []string {
	result := make([]string, 0, len(apps))
	for _, app := range apps {
		app = strings.ToLower(strings.TrimSpace(app))
		switch app {
		case "codex":
			result = append(result, app)
		}
	}
	return result
}

func enabledAppsList(apps map[string]bool) []string {
	result := make([]string, 0, len(toolingSupportedApps))
	for _, key := range toolingSupportedApps {
		if apps[key] {
			result = append(result, key)
		}
	}
	return result
}

func defaultEnabledApps(enabled map[string]bool) map[string]bool {
	result := map[string]bool{
		"codex": true,
	}
	if enabled == nil {
		return result
	}
	for _, key := range toolingSupportedApps {
		if _, ok := enabled[key]; ok {
			result[key] = enabled[key]
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildSkillStats(skills []managedSkillRecord) skillStatsResponse {
	stats := skillStatsResponse{Total: len(skills), BySource: map[string]int{}}
	for _, skill := range skills {
		source := skill.SourceKind
		if source == "" {
			source = "manual"
		}
		stats.BySource[source]++
	}
	return stats
}

func scanManagedSkills(home string, clients []toolingClientState) []managedSkillRecord {
	index := map[string]managedSkillRecord{}
	skillsDir := managedSkillsRoot(home)
	if !pathExists(skillsDir) {
		return nil
	}
	records, _ := listSkillCollections(skillsDir)
	for _, entry := range records {
		meta := readSkillMetadata(entry.FullPath)
		record := managedSkillRecord{
			Name:          firstNonEmpty(meta.Name, entry.RelativePath),
			Description:   describeSkillCollection(entry.FullPath),
			Directory:     entry.FullPath,
			SourceClient:  meta.SourceClient,
			SourceRepo:    meta.SourceRepo,
			SourceKind:    firstNonEmpty(meta.SourceKind, "manual"),
			ManagedPath:   entry.FullPath,
			InstalledApps: map[string]bool{},
		}
		for _, client := range clients {
			if pathExists(filepath.Join(client.SkillsDir, filepath.FromSlash(entry.RelativePath))) {
				record.InstalledApps[client.App] = true
			}
		}
		index[skillKey(entry.FullPath)] = record
	}
	result := make([]managedSkillRecord, 0, len(index))
	for _, item := range index {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

type skillMetadata struct {
	Name         string `json:"name"`
	SourceClient string `json:"source_client"`
	SourceRepo   string `json:"source_repo"`
	SourceKind   string `json:"source_kind"`
}

func readSkillMetadata(dir string) skillMetadata {
	raw, err := os.ReadFile(filepath.Join(dir, ".aigate-skill.json"))
	if err != nil {
		return skillMetadata{}
	}
	var meta skillMetadata
	_ = json.Unmarshal(raw, &meta)
	return meta
}

func writeSkillMetadata(dir string, meta skillMetadata) error {
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, ".aigate-skill.json"), append(raw, '\n'), 0o600)
}

func skillKey(dir string) string {
	return strings.ToLower(filepath.Clean(dir))
}

func managedSkillsRoot(home string) string {
	return filepath.Join(aigateDataRoot(home), "tooling", "skills")
}

type skillCollectionRecord struct {
	RelativePath string
	FullPath     string
}

func listSkillCollections(root string) ([]skillCollectionRecord, error) {
	if !pathExists(root) {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	records := []skillCollectionRecord{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		fullPath := filepath.Join(root, entry.Name())
		info, err := os.Stat(fullPath)
		if err != nil || !info.IsDir() {
			continue
		}
		hasSkill, err := containsSkillFile(fullPath)
		if err != nil {
			return nil, err
		}
		if !hasSkill {
			continue
		}
		records = append(records, skillCollectionRecord{
			RelativePath: entry.Name(),
			FullPath:     fullPath,
		})
	}
	sort.Slice(records, func(i, j int) bool {
		return strings.ToLower(records[i].RelativePath) < strings.ToLower(records[j].RelativePath)
	})
	return records, nil
}

func containsSkillFile(root string) (bool, error) {
	scanRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		scanRoot = resolved
	}
	found := false
	err := filepath.WalkDir(scanRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != scanRoot && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "SKILL.md" {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && !errors.Is(err, filepath.SkipAll) {
		return false, err
	}
	return found, nil
}

func describeSkillCollection(root string) string {
	scanRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		scanRoot = resolved
	}
	directDesc := readSkillDescription(filepath.Join(scanRoot, "SKILL.md"))
	if directDesc != "" {
		return directDesc
	}
	count := 0
	firstDesc := ""
	_ = filepath.WalkDir(scanRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != scanRoot && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}
		count++
		if firstDesc == "" {
			firstDesc = readSkillDescription(path)
		}
		return nil
	})
	if count > 1 {
		return fmt.Sprintf("包含 %d 个技能", count)
	}
	return firstDesc
}

func readSkillDescription(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(raw), "\n")
	inFrontmatter := false
	frontmatterSeen := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" && !frontmatterSeen {
			inFrontmatter = true
			frontmatterSeen = true
			continue
		}
		if trimmed == "---" && inFrontmatter {
			inFrontmatter = false
			continue
		}
		if inFrontmatter {
			if strings.HasPrefix(trimmed, "description:") {
				value := strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
				return strings.Trim(value, `"'`)
			}
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return trimmed
	}
	return ""
}

func importSkillsFromClient(home string, source string) (int, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	clients := toolingClients(home)
	client, ok := clients[source]
	if !ok {
		return 0, fmt.Errorf("unknown skill source: %s", source)
	}
	if !pathExists(client.skillsDir) {
		return 0, fmt.Errorf("skills directory not found: %s", client.skillsDir)
	}
	targetRoot := managedSkillsRoot(home)
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return 0, err
	}
	records, err := listSkillCollections(client.skillsDir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range records {
		dstDir := filepath.Join(targetRoot, filepath.FromSlash(entry.RelativePath))
		if err := copyOrSymlinkDir(entry.FullPath, dstDir, "copy"); err != nil {
			return count, err
		}
		meta := skillMetadata{Name: entry.RelativePath, SourceClient: source, SourceKind: source}
		if err := writeSkillMetadata(dstDir, meta); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func applyManagedSkills(home string, method string, apps []string) (int, error) {
	method = normalizeSkillSyncMethod(method)
	targetApps := apps
	if len(targetApps) == 0 {
		targetApps = append([]string{}, toolingSupportedApps...)
	}
	skillsRoot := managedSkillsRoot(home)
	if !pathExists(skillsRoot) {
		return 0, nil
	}
	records, err := listSkillCollections(skillsRoot)
	if err != nil {
		return 0, err
	}
	count := 0
	clients := toolingClients(home)
	for _, entry := range records {
		for _, app := range targetApps {
			client, ok := clients[app]
			if !ok {
				continue
			}
			dstDir := filepath.Join(client.skillsDir, filepath.FromSlash(entry.RelativePath))
			if err := copyOrSymlinkDir(entry.FullPath, dstDir, method); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

func applyManagedSkillCollection(home string, name string, method string, apps []string) (int, error) {
	method = normalizeSkillSyncMethod(method)
	targetApps := apps
	if len(targetApps) == 0 {
		targetApps = append([]string{}, toolingSupportedApps...)
	}
	srcDir := filepath.Join(managedSkillsRoot(home), filepath.FromSlash(name))
	if !pathExists(srcDir) {
		return 0, os.ErrNotExist
	}
	count := 0
	clients := toolingClients(home)
	for _, app := range targetApps {
		client, ok := clients[app]
		if !ok {
			continue
		}
		dstDir := filepath.Join(client.skillsDir, filepath.FromSlash(name))
		if err := copyOrSymlinkDir(srcDir, dstDir, method); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func removeSkillCollectionFromClients(home string, name string, apps []string) error {
	targetApps := apps
	if len(targetApps) == 0 {
		targetApps = append([]string{}, toolingSupportedApps...)
	}
	clients := toolingClients(home)
	for _, app := range targetApps {
		client, ok := clients[app]
		if !ok {
			continue
		}
		dstDir := filepath.Join(client.skillsDir, filepath.FromSlash(name))
		if err := os.RemoveAll(dstDir); err != nil {
			return err
		}
	}
	return nil
}

func deleteManagedSkillCollection(home string, name string) error {
	srcDir := filepath.Join(managedSkillsRoot(home), filepath.FromSlash(name))
	if !pathExists(srcDir) {
		return os.ErrNotExist
	}
	if err := removeSkillCollectionFromClients(home, name, nil); err != nil {
		return err
	}
	return os.RemoveAll(srcDir)
}

func copyOrSymlinkDir(source string, target string, method string) error {
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if method == "symlink" && runtime.GOOS != "windows" {
		if err := os.Symlink(source, target); err == nil {
			return nil
		}
	}
	return copyDir(source, target)
}

func copyDir(source string, target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			return os.MkdirAll(dst, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, dst)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, raw, 0o644)
	})
}

func countSkillDirs(dir string) int {
	records, err := listSkillCollections(dir)
	if err != nil {
		return 0
	}
	return len(records)
}

func decodeToolingSkillName(path string) (string, error) {
	trimmed := strings.TrimPrefix(path, "/tooling/skills/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return "", errors.New("invalid skill path")
	}
	name, err := url.PathUnescape(trimmed)
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("missing skill name")
	}
	return name, nil
}

func isSkillDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findTemplate(id string) (mcpTemplateRecord, bool) {
	for _, template := range toolingTemplates {
		if strings.EqualFold(template.ID, id) {
			return template, true
		}
	}
	return mcpTemplateRecord{}, false
}

func templateToSpec(template mcpTemplateRecord) map[string]any {
	switch template.Type {
	case "stdio":
		return map[string]any{
			"type":    "stdio",
			"command": template.Command,
			"args":    append([]string{}, template.Args...),
		}
	default:
		return map[string]any{}
	}
}

func applyMcpServerToClients(home string, server managedMcpServer, apps []string) error {
	clients := toolingClients(home)
	for _, app := range apps {
		client, ok := clients[app]
		if !ok {
			continue
		}
		switch app {
		case "codex":
			if err := writeCodexMcp(client.mcpPath, server.ID, server.Spec); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeMcpServerFromClients(home string, id string) error {
	clients := toolingClients(home)
	for app, client := range clients {
		switch app {
		case "codex":
			if err := removeCodexMcp(client.mcpPath, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *ToolingHandler) buildMcpViews(home string, cfg toolingConfig) []toolingMcpServerView {
	clients := toolingClients(home)
	views := make([]toolingMcpServerView, 0, len(cfg.McpServers))
	for _, server := range cfg.McpServers {
		deletePlan := buildMcpDeletePlan(home, server)
		status := map[string]string{}
		for _, key := range toolingSupportedApps {
			client := clients[key]
			if !pathExists(client.mcpPath) {
				status[key] = "missing"
				continue
			}
			if server.EnabledApps[key] {
				status[key] = "enabled"
			} else {
				status[key] = "disabled"
			}
		}
		views = append(views, toolingMcpServerView{
			ID:            server.ID,
			Name:          server.Name,
			Description:   server.Description,
			TemplateID:    server.TemplateID,
			EnabledApps:   defaultEnabledApps(server.EnabledApps),
			ClientStatus:  status,
			DeleteAllowed: deletePlan.Allowed,
			DeleteReason:  deletePlan.Reason,
			DeleteTargets: append([]string{}, deletePlan.CleanupRoots...),
			Spec:          server.Spec,
		})
	}
	sort.Slice(views, func(i, j int) bool {
		return strings.ToLower(views[i].Name) < strings.ToLower(views[j].Name)
	})
	return views
}

func discoverMcpServers(home string) []toolingDiscoveredMcpServerView {
	clients := toolingClients(home)
	index := map[string]*toolingDiscoveredMcpServerView{}
	for _, app := range toolingSupportedApps {
		client := clients[app]
		servers, err := readClientMcpServers(app, client.mcpPath)
		if err != nil {
			continue
		}
		for id, spec := range servers {
			item, ok := index[id]
			if !ok {
				item = &toolingDiscoveredMcpServerView{
					ID:           id,
					Name:         firstNonEmpty(serverNameFromSpec(spec), id),
					Description:  serverDescriptionFromSpec(spec),
					SourceApps:   map[string]bool{},
					ClientStatus: map[string]string{},
					Spec:         spec,
				}
				index[id] = item
			}
			item.SourceApps[app] = true
			item.ClientStatus[app] = "enabled"
			if len(item.Spec) == 0 {
				item.Spec = spec
			}
		}
	}
	result := make([]toolingDiscoveredMcpServerView, 0, len(index))
	for _, item := range index {
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func readClientMcpServers(app string, path string) (map[string]map[string]any, error) {
	switch app {
	case "codex":
		doc, err := readTOMLDocument(path)
		if err != nil {
			return nil, err
		}
		return extractObjectMap(doc["mcp_servers"]), nil
	default:
		return map[string]map[string]any{}, nil
	}
}

func extractObjectMap(raw any) map[string]map[string]any {
	result := map[string]map[string]any{}
	items, ok := raw.(map[string]any)
	if !ok {
		return result
	}
	for key, value := range items {
		if spec, ok := value.(map[string]any); ok {
			result[key] = spec
		}
	}
	return result
}

func serverNameFromSpec(spec map[string]any) string {
	if name, ok := spec["name"].(string); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if command, ok := spec["command"].(string); ok && strings.TrimSpace(command) != "" {
		return strings.TrimSpace(command)
	}
	if url, ok := spec["url"].(string); ok && strings.TrimSpace(url) != "" {
		return strings.TrimSpace(url)
	}
	return ""
}

func serverDescriptionFromSpec(spec map[string]any) string {
	if desc, ok := spec["description"].(string); ok && strings.TrimSpace(desc) != "" {
		return strings.TrimSpace(desc)
	}
	if args, ok := spec["args"].([]any); ok && len(args) > 0 {
		return fmt.Sprintf("%v", args[0])
	}
	return ""
}

func writeGeminiMcp(path string, id string, spec map[string]any) error {
	data, err := readJSONObject(path)
	if err != nil {
		return err
	}
	mcpServers, _ := data["mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}
	mcpServers[id] = spec
	data["mcpServers"] = mcpServers
	return writeJSONObject(path, data)
}

func removeGeminiMcp(path string, id string) error {
	data, err := readJSONObject(path)
	if err != nil {
		return err
	}
	if mcpServers, ok := data["mcpServers"].(map[string]any); ok {
		delete(mcpServers, id)
		data["mcpServers"] = mcpServers
	}
	return writeJSONObject(path, data)
}

func writeOpenCodeMcp(path string, id string, spec map[string]any) error {
	data, err := readJSONObject(path)
	if err != nil {
		return err
	}
	mcp, _ := data["mcp"].(map[string]any)
	if mcp == nil {
		mcp = map[string]any{}
	}
	mcp[id] = convertSpecForOpenCode(spec)
	data["mcp"] = mcp
	return writeJSONObject(path, data)
}

func removeOpenCodeMcp(path string, id string) error {
	data, err := readJSONObject(path)
	if err != nil {
		return err
	}
	if mcp, ok := data["mcp"].(map[string]any); ok {
		delete(mcp, id)
		data["mcp"] = mcp
	}
	return writeJSONObject(path, data)
}

func writeCodexMcp(path string, id string, spec map[string]any) error {
	doc, err := readTOMLDocument(path)
	if err != nil {
		return err
	}
	table, ok := specToMap(spec)
	if !ok {
		return errors.New("invalid codex mcp spec")
	}
	root := doc
	mcpServers, ok := root["mcp_servers"].(map[string]any)
	if !ok {
		mcpServers = map[string]any{}
	}
	mcpServers[id] = table
	root["mcp_servers"] = mcpServers
	return writeTOMLDocument(path, root)
}

func removeCodexMcp(path string, id string) error {
	doc, err := readTOMLDocument(path)
	if err != nil {
		return err
	}
	if mcpServers, ok := doc["mcp_servers"].(map[string]any); ok {
		delete(mcpServers, id)
		doc["mcp_servers"] = mcpServers
	}
	return writeTOMLDocument(path, doc)
}

func buildMcpDeletePlan(home string, server managedMcpServer) toolingMcpDeletePlan {
	if strings.TrimSpace(server.TemplateID) != "" {
		return toolingMcpDeletePlan{
			Allowed: false,
			Reason:  "该 MCP 由 AI Gate 提供，需在来源侧管理，不能在这里删除。",
		}
	}
	cleanupRoots := codexManagedMcpCleanupRoots(home, server.Spec)
	if len(cleanupRoots) > 0 {
		return toolingMcpDeletePlan{
			Allowed:      true,
			CleanupRoots: cleanupRoots,
		}
	}
	return toolingMcpDeletePlan{
		Allowed: false,
		Reason:  "该 MCP 不是由 Codex 直接托管的本地目录，可能是手动安装或外部服务，不能安全删除。",
	}
}

func codexManagedMcpCleanupRoots(home string, spec map[string]any) []string {
	managedRoot := filepath.Clean(filepath.Join(home, ".codex", "mcp"))
	values := make([]string, 0, 8)
	collectSpecStrings(spec, &values)
	seen := map[string]struct{}{}
	roots := make([]string, 0, len(values))
	for _, raw := range values {
		root, ok := codexManagedMcpRootFromValue(home, managedRoot, raw)
		if !ok {
			continue
		}
		if _, exists := seen[root]; exists {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

func collectSpecStrings(value any, out *[]string) {
	switch typed := value.(type) {
	case string:
		*out = append(*out, typed)
	case []string:
		*out = append(*out, typed...)
	case []any:
		for _, item := range typed {
			collectSpecStrings(item, out)
		}
	case map[string]any:
		for _, item := range typed {
			collectSpecStrings(item, out)
		}
	}
}

func codexManagedMcpRootFromValue(home string, managedRoot string, raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	candidates := []string{}
	switch {
	case filepath.IsAbs(trimmed):
		candidates = append(candidates, trimmed)
	case strings.HasPrefix(trimmed, "~/"):
		candidates = append(candidates, filepath.Join(home, trimmed[2:]))
	case strings.HasPrefix(trimmed, ".codex/"):
		candidates = append(candidates, filepath.Join(home, trimmed))
	case strings.HasPrefix(trimmed, "./.codex/"):
		candidates = append(candidates, filepath.Join(home, strings.TrimPrefix(trimmed, "./")))
	}
	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		rel, err := filepath.Rel(managedRoot, cleaned)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) == 0 || parts[0] == "" || parts[0] == "." || parts[0] == ".." {
			continue
		}
		return filepath.Join(managedRoot, parts[0]), true
	}
	return "", false
}

func removeManagedMcpRoots(roots []string) error {
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		if err := os.RemoveAll(root); err != nil {
			return err
		}
	}
	return nil
}

func parseBoolQueryValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func readJSONObject(path string) (map[string]any, error) {
	if !pathExists(path) {
		return map[string]any{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeJSONObject(path string, payload map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(raw, '\n'), 0o600)
}

func readTOMLDocument(path string) (map[string]any, error) {
	if !pathExists(path) {
		return map[string]any{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := toml.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeTOMLDocument(path string, payload map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := toml.Marshal(payload)
	if err != nil {
		return err
	}
	return writeAtomic(path, raw, 0o600)
}

func specToMap(spec map[string]any) (map[string]any, bool) {
	if spec == nil {
		return map[string]any{}, true
	}
	converted := make(map[string]any, len(spec))
	for key, value := range spec {
		converted[key] = value
	}
	return converted, true
}

func convertSpecForOpenCode(spec map[string]any) map[string]any {
	converted := map[string]any{}
	typ, _ := spec["type"].(string)
	switch typ {
	case "http", "sse":
		converted["type"] = "remote"
		if url, ok := spec["url"]; ok {
			converted["url"] = url
		}
	default:
		converted["type"] = "local"
		if command, ok := spec["command"]; ok {
			switch commandValue := command.(type) {
			case string:
				arr := []any{commandValue}
				if args, ok := spec["args"].([]any); ok {
					arr = append(arr, args...)
				}
				converted["command"] = arr
			}
		}
	}
	return converted
}

func defaultRepoSearchResults() []repoSearchResult {
	results := make([]repoSearchResult, 0, len(toolingDefaultRepos))
	for _, repo := range toolingDefaultRepos {
		results = append(results, repoSearchResult{
			Owner:  repo.Owner,
			Name:   repo.Name,
			Branch: repo.Branch,
			URL:    fmt.Sprintf("https://github.com/%s/%s", repo.Owner, repo.Name),
		})
	}
	return results
}

func (h *ToolingHandler) syncManagedCodexMcpServers(home string, cfg toolingConfig) (toolingConfig, error) {
	client, ok := toolingClients(home)["codex"]
	if !ok {
		return cfg, nil
	}
	servers, err := readClientMcpServers("codex", client.mcpPath)
	if err != nil {
		return cfg, err
	}
	changed := false
	for id, spec := range servers {
		imported := managedMcpServer{
			ID:          id,
			Name:        firstNonEmpty(serverNameFromSpec(spec), id),
			Description: serverDescriptionFromSpec(spec),
			EnabledApps: map[string]bool{"codex": true},
			Spec:        spec,
		}
		if existing, ok := cfg.serverByID(id); ok {
			merged := mergeManagedMcpServer(existing, imported)
			if !reflect.DeepEqual(existing, merged) {
				cfg.upsertServer(merged)
				changed = true
			}
			continue
		}
		cfg.upsertServer(imported)
		changed = true
	}
	if changed {
		if err := h.saveConfig(home, cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func searchGitHubRepos(query string) ([]repoSearchResult, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/search/repositories", nil)
	if err != nil {
		return nil, err
	}
	params := req.URL.Query()
	params.Set("q", query+" skill")
	params.Set("per_page", "8")
	req.URL.RawQuery = params.Encode()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github search failed: %s", resp.Status)
	}
	var payload struct {
		Items []struct {
			FullName      string `json:"full_name"`
			HTMLURL       string `json:"html_url"`
			Description   string `json:"description"`
			DefaultBranch string `json:"default_branch"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	results := make([]repoSearchResult, 0, len(payload.Items))
	for _, item := range payload.Items {
		owner, name, found := strings.Cut(item.FullName, "/")
		if !found {
			continue
		}
		results = append(results, repoSearchResult{
			Owner:       owner,
			Name:        name,
			Branch:      firstNonEmpty(item.DefaultBranch, "main"),
			URL:         item.HTMLURL,
			Description: item.Description,
		})
	}
	return results, nil
}
