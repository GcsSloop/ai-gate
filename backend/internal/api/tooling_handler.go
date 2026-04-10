package api

import (
	"archive/zip"
	"bytes"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
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
	"strconv"
	"strings"
	"sync"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

const toolingConfigFilename = "tooling.json"
const (
	skillDiscoveryCacheMaxBytes        = 1024 * 1024
	skillDiscoveryDescriptionMaxLength = 600
	githubArchiveRetryMaxAttempts      = 3
	githubArchiveRetryBaseDelay        = 1500 * time.Millisecond
	githubRepoRequestInterval          = 250 * time.Millisecond
	skillDiscoveryAutoRefreshJitterMax = 3 * time.Hour
)

var toolingSupportedApps = []string{"codex"}

type defaultSkillRepoSeed struct {
	Platform string
	Owner    string
	Name     string
	Branch   string
	Stars    int
}

type ToolingHandler struct {
	mu                  sync.Mutex
	autoRefreshMu       sync.Mutex
	autoRefreshInFlight bool
	autoRefreshLastTry  time.Time
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
	Platform   string `json:"platform,omitempty"`
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	Branch     string `json:"branch"`
	Enabled    bool   `json:"enabled"`
	SkillCount int    `json:"skill_count"`
	StarCount  int    `json:"star_count,omitempty"`
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
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Directory       string          `json:"directory"`
	SourceClient    string          `json:"source_client,omitempty"`
	SourceRepo      string          `json:"source_repo,omitempty"`
	SourceKind      string          `json:"source_kind"`
	ManagedPath     string          `json:"managed_path"`
	InstalledApps   map[string]bool `json:"installed_apps"`
	UpdateAvailable bool            `json:"update_available"`
}

type repoSearchResult struct {
	Platform    string `json:"platform,omitempty"`
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Branch      string `json:"branch"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type discoveredSkillRecord struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Platform        string          `json:"platform"`
	RepoOwner       string          `json:"repo_owner"`
	RepoName        string          `json:"repo_name"`
	Branch          string          `json:"branch"`
	RepoURL         string          `json:"repo_url"`
	SourcePath      string          `json:"source_path"`
	SourceURL       string          `json:"source_url"`
	ManagedName     string          `json:"managed_name"`
	ContentHash     string          `json:"content_hash,omitempty"`
	InstalledHash   string          `json:"installed_hash,omitempty"`
	InstalledApps   map[string]bool `json:"installed_apps"`
	UpdateAvailable bool            `json:"update_available"`
}

type discoveredInstallState struct {
	Apps map[string]bool
	Hash string
}

type skillDiscoveryCache struct {
	FetchedAt          string                  `json:"fetched_at"`
	NextAutoRefreshAt  string                  `json:"next_auto_refresh_at,omitempty"`
	RepoStarsFetchedAt string                  `json:"repo_stars_fetched_at,omitempty"`
	Items              []discoveredSkillRecord `json:"items"`
}

type toolingSkillDiscoverResponse struct {
	Cached    bool                    `json:"cached"`
	FetchedAt string                  `json:"fetched_at,omitempty"`
	Total     int                     `json:"total"`
	Offset    int                     `json:"offset"`
	Limit     int                     `json:"limit"`
	Items     []discoveredSkillRecord `json:"items"`
}

type centralDiscoveredSkillResponse struct {
	FetchedAt string `json:"fetched_at"`
	Total     int    `json:"total_items"`
	Items     []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Platform   string `json:"platform"`
		RepoOwner  string `json:"repo_owner"`
		RepoName   string `json:"repo_name"`
		Branch     string `json:"branch"`
		RepoURL    string `json:"repo_url"`
		SourcePath string `json:"source_path"`
		SourceURL  string `json:"source_url"`
	} `json:"items"`
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

type toolingDiscoveredSkillInstallRequest struct {
	ID   string   `json:"id"`
	Apps []string `json:"apps"`
}

type toolingRepoRequest struct {
	Platform string `json:"platform"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	Branch   string `json:"branch"`
}

type toolingRepoOrderItem struct {
	Platform string `json:"platform"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
}

type toolingRepoOrderRequest struct {
	Items []toolingRepoOrderItem `json:"items"`
}

type toolingRepoResolveRequest struct {
	Input string `json:"input"`
}

type toolingRepoResolveResponse struct {
	Platform       string   `json:"platform"`
	Owner          string   `json:"owner"`
	Name           string   `json:"name"`
	RepoURL        string   `json:"repo_url"`
	BranchOptions  []string `json:"branch_options"`
	SelectedBranch string   `json:"selected_branch"`
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

var toolingDefaultRepos = buildToolingDefaultRepos()

var toolingDefaultRepoSeeds = []defaultSkillRepoSeed{
	// Synced from https://cc-ai.cn/skills-cn/frontend/index.html on 2026-04-10.
	{Platform: "github", Owner: "obra", Name: "superpowers", Branch: "main", Stars: 144505},
	{Platform: "github", Owner: "anthropics", Name: "skills", Branch: "main", Stars: 114242},
	{Platform: "github", Owner: "shadcn", Name: "ui", Branch: "main", Stars: 111969},
	{Platform: "github", Owner: "browser-use", Name: "browser-use", Branch: "main", Stars: 86895},
	{Platform: "github", Owner: "nextlevelbuilder", Name: "ui-ux-pro-max-skill", Branch: "main", Stars: 62185},
	{Platform: "github", Owner: "wshobson", Name: "agents", Branch: "main", Stars: 33305},
	{Platform: "github", Owner: "vercel-labs", Name: "agent-browser", Branch: "main", Stars: 28441},
	{Platform: "github", Owner: "vercel-labs", Name: "agent-skills", Branch: "main", Stars: 24819},
	{Platform: "github", Owner: "coreyhaines31", Name: "marketingskills", Branch: "main", Stars: 20027},
	{Platform: "github", Owner: "JimLiu", Name: "baoyu-skills", Branch: "main", Stars: 14014},
	{Platform: "github", Owner: "pbakaus", Name: "impeccable", Branch: "main", Stars: 17953},
	{Platform: "github", Owner: "vercel-labs", Name: "skills", Branch: "main", Stars: 13603},
	{Platform: "github", Owner: "ComposioHQ", Name: "awesome-claude-skills", Branch: "master", Stars: 52597},
	{Platform: "github", Owner: "larksuite", Name: "cli", Branch: "main", Stars: 7311},
	{Platform: "github", Owner: "google-labs-code", Name: "stitch-skills", Branch: "main", Stars: 4241},
	{Platform: "github", Owner: "remotion-dev", Name: "skills", Branch: "main", Stars: 2670},
	{Platform: "github", Owner: "cexll", Name: "myclaude", Branch: "master", Stars: 2579},
	{Platform: "github", Owner: "supabase", Name: "agent-skills", Branch: "main", Stars: 1879},
	{Platform: "github", Owner: "vercel-labs", Name: "next-skills", Branch: "main", Stars: 809},
	{Platform: "github", Owner: "microsoft", Name: "azure-skills", Branch: "main", Stars: 611},
	{Platform: "github", Owner: "sleekdotdesign", Name: "agent-skills", Branch: "main", Stars: 298},
	{Platform: "github", Owner: "microsoft", Name: "github-copilot-for-azure", Branch: "main", Stars: 184},
	{Platform: "github", Owner: "better-auth", Name: "skills", Branch: "main", Stars: 171},
	{Platform: "github", Owner: "lc2panda", Name: "wps-skills", Branch: "main", Stars: 144},
	{Platform: "github", Owner: "squirrelscan", Name: "skills", Branch: "main", Stars: 72},
	{Platform: "github", Owner: "openai", Name: "skills", Branch: "main", Stars: 0},
	{Platform: "github", Owner: "xixu-me", Name: "skills", Branch: "main", Stars: 32},
	{Platform: "github", Owner: "roin-orca", Name: "skills", Branch: "main", Stars: 3},
	{Platform: "github", Owner: "soultrace-ai", Name: "soultrace-skill", Branch: "main", Stars: 2},
}

var toolingTemplates = []mcpTemplateRecord{
	{ID: "fetch", Name: "mcp-server-fetch", Description: "Quick template: mcp-fetch", Type: "stdio", Command: "uvx", Args: []string{"mcp-server-fetch"}},
	{ID: "time", Name: "@modelcontextprotocol/server-time", Description: "Quick template: time server", Type: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-time"}},
	{ID: "memory", Name: "@modelcontextprotocol/server-memory", Description: "Quick template: memory server", Type: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-memory"}},
	{ID: "sequential-thinking", Name: "@modelcontextprotocol/server-sequential-thinking", Description: "Quick template: sequential thinking", Type: "stdio", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-sequential-thinking"}},
	{ID: "context7", Name: "@upstash/context7-mcp", Description: "Quick template: context7", Type: "stdio", Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
}

func buildToolingDefaultRepos() []skillRepoRecord {
	seeds := append([]defaultSkillRepoSeed{}, toolingDefaultRepoSeeds...)
	sort.SliceStable(seeds, func(i, j int) bool {
		if seeds[i].Stars == seeds[j].Stars {
			left := strings.ToLower(seeds[i].Owner + "/" + seeds[i].Name)
			right := strings.ToLower(seeds[j].Owner + "/" + seeds[j].Name)
			return left < right
		}
		return seeds[i].Stars > seeds[j].Stars
	})
	repos := make([]skillRepoRecord, 0, len(seeds))
	for _, seed := range seeds {
		repos = append(repos, normalizeSkillRepoRecord(skillRepoRecord{
			Platform:  seed.Platform,
			Owner:     seed.Owner,
			Name:      seed.Name,
			Branch:    seed.Branch,
			Enabled:   true,
			StarCount: seed.Stars,
		}))
	}
	return repos
}

func (h *ToolingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/tooling/state":
		h.getState(w)
	case r.Method == http.MethodPut && r.URL.Path == "/tooling/settings":
		h.updateSettings(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/tooling/skills/discover":
		h.getDiscoveredSkills(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/skills/discover/install":
		h.installDiscoveredSkill(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/skills/discover/refresh":
		h.refreshDiscoveredSkills(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/skills/import":
		h.importSkills(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/skills/apply":
		h.applySkills(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/tooling/skills/repos":
		h.listSkillRepos(w)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/skills/repos":
		h.addSkillRepo(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/tooling/skills/repos/resolve":
		h.resolveSkillRepo(w, r)
	case r.Method == http.MethodPut && r.URL.Path == "/tooling/skills/repos/order":
		h.reorderSkillRepos(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/tooling/skills/repos/"):
		h.updateSkillRepo(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/tooling/skills/repos/"):
		h.removeSkillRepo(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/tooling/skills/"):
		h.updateSkill(w, r)
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
	cache, _ := loadSkillDiscoveryCache(home)
	applySkillUpdateFlags(skills, cache.Items)
	_, _ = loadOrCreateToolingAnonymousID(home)
	repoResults := defaultRepoSearchResults()
	discoveredServers := discoverMcpServers(home)
	servers := h.buildMcpViews(home, cfg)
	h.maybeScheduleDailySkillDiscoveryRefresh(home, cache)

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

func (h *ToolingHandler) maybeScheduleDailySkillDiscoveryRefresh(home string, cache skillDiscoveryCache) {
	now := timeNowUTC()
	if nextAutoAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(cache.NextAutoRefreshAt)); !nextAutoAt.IsZero() && now.Before(nextAutoAt) {
		return
	}
	parsedFetchedAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(cache.FetchedAt))
	if !parsedFetchedAt.IsZero() && now.Sub(parsedFetchedAt) < 24*time.Hour {
		return
	}
	h.autoRefreshMu.Lock()
	defer h.autoRefreshMu.Unlock()
	if h.autoRefreshInFlight {
		return
	}
	if !h.autoRefreshLastTry.IsZero() && now.Sub(h.autoRefreshLastTry) < 5*time.Minute {
		return
	}
	h.autoRefreshInFlight = true
	h.autoRefreshLastTry = now
	go func() {
		defer func() {
			h.autoRefreshMu.Lock()
			h.autoRefreshInFlight = false
			h.autoRefreshMu.Unlock()
		}()
		_, _, _ = h.refreshSkillDiscovery(home)
	}()
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

func (h *ToolingHandler) getDiscoveredSkills(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	limit, offset := parseDiscoveredPaging(r.URL.Query())
	cache, ok := loadSkillDiscoveryCache(home)
	if ok {
		items, total := sliceDiscoveredItems(cache.Items, limit, offset)
		writeJSON(w, http.StatusOK, toolingSkillDiscoverResponse{
			Cached:    true,
			FetchedAt: cache.FetchedAt,
			Total:     total,
			Offset:    offset,
			Limit:     limit,
			Items:     items,
		})
		return
	}
	items, fetchedAt, err := h.refreshSkillDiscovery(home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	paged, total := sliceDiscoveredItems(items, limit, offset)
	writeJSON(w, http.StatusOK, toolingSkillDiscoverResponse{
		Cached:    false,
		FetchedAt: fetchedAt,
		Total:     total,
		Offset:    offset,
		Limit:     limit,
		Items:     paged,
	})
}

func (h *ToolingHandler) refreshDiscoveredSkills(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, fetchedAt, err := h.refreshSkillDiscovery(home)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	limit, offset := parseDiscoveredPaging(r.URL.Query())
	paged, total := sliceDiscoveredItems(items, limit, offset)
	writeJSON(w, http.StatusOK, toolingSkillDiscoverResponse{
		Cached:    false,
		FetchedAt: fetchedAt,
		Total:     total,
		Offset:    offset,
		Limit:     limit,
		Items:     paged,
	})
}

func (h *ToolingHandler) installDiscoveredSkill(w http.ResponseWriter, r *http.Request) {
	var req toolingDiscoveredSkillInstallRequest
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
	method := normalizeSkillSyncMethod(cfg.SkillSyncMethod)
	applied, err := installDiscoveredSkillCollection(home, req.ID, method, reqApps(req.Apps))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	go reportDiscoveredSkillInstall(home, req.ID)
	writeJSON(w, http.StatusOK, map[string]any{"applied": applied, "enabled": true, "skill_sync_method": method})
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
	next := normalizeSkillRepoRecord(skillRepoRecord{
		Platform: req.Platform,
		Owner:    strings.TrimSpace(req.Owner),
		Name:     strings.TrimSpace(req.Name),
		Branch:   strings.TrimSpace(req.Branch),
		Enabled:  true,
	})
	for idx, repo := range cfg.SkillRepos {
		if skillRepoMatches(repo, next.Platform, next.Owner, next.Name) {
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

func (h *ToolingHandler) resolveSkillRepo(w http.ResponseWriter, r *http.Request) {
	var req toolingRepoResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resolved, err := parseToolingRepoInput(req.Input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	branches, selectedBranch, err := resolveSkillRepoBranches(resolved.Platform, resolved.Owner, resolved.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, toolingRepoResolveResponse{
		Platform:       resolved.Platform,
		Owner:          resolved.Owner,
		Name:           resolved.Name,
		RepoURL:        buildRepoURL(resolved.Platform, resolved.Owner, resolved.Name),
		BranchOptions:  branches,
		SelectedBranch: selectedBranch,
	})
}

func (h *ToolingHandler) updateSkillRepo(w http.ResponseWriter, r *http.Request) {
	platform, owner, name, err := decodeToolingRepoPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req toolingRepoRequest
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
	next := normalizeSkillRepoRecord(skillRepoRecord{
		Platform: firstNonEmpty(req.Platform, platform),
		Owner:    firstNonEmpty(req.Owner, owner),
		Name:     firstNonEmpty(req.Name, name),
		Branch:   strings.TrimSpace(req.Branch),
		Enabled:  true,
	})
	for idx, repo := range cfg.SkillRepos {
		if !skillRepoMatches(repo, platform, owner, name) {
			continue
		}
		next.SkillCount = repo.SkillCount
		next.Enabled = repo.Enabled
		cfg.SkillRepos[idx] = next
		if err := h.saveConfig(home, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, next)
		return
	}
	http.Error(w, "repo not found", http.StatusNotFound)
}

func (h *ToolingHandler) reorderSkillRepos(w http.ResponseWriter, r *http.Request) {
	var req toolingRepoOrderRequest
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
	byKey := make(map[string]skillRepoRecord, len(cfg.SkillRepos))
	orderedKeys := make([]string, 0, len(cfg.SkillRepos))
	for _, repo := range cfg.SkillRepos {
		key := repoRecordKey(repo.Platform, repo.Owner, repo.Name)
		byKey[key] = repo
		orderedKeys = append(orderedKeys, key)
	}
	seen := map[string]bool{}
	reordered := make([]skillRepoRecord, 0, len(cfg.SkillRepos))
	for _, item := range req.Items {
		key := repoRecordKey(item.Platform, item.Owner, item.Name)
		if seen[key] {
			continue
		}
		repo, ok := byKey[key]
		if !ok {
			continue
		}
		reordered = append(reordered, repo)
		seen[key] = true
	}
	for _, key := range orderedKeys {
		if seen[key] {
			continue
		}
		reordered = append(reordered, byKey[key])
	}
	cfg.SkillRepos = reordered
	if err := h.saveConfig(home, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cfg.SkillRepos)
}

func (h *ToolingHandler) removeSkillRepo(w http.ResponseWriter, r *http.Request) {
	platform, owner, name, err := decodeToolingRepoPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := h.loadConfig(home)
	next := make([]skillRepoRecord, 0, len(cfg.SkillRepos))
	removed := false
	for _, repo := range cfg.SkillRepos {
		if skillRepoMatches(repo, platform, owner, name) {
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
	platform := normalizeSkillRepoPlatform(r.URL.Query().Get("platform"))
	if query == "" {
		writeJSON(w, http.StatusOK, toolingRepoSearchResponse{Items: defaultRepoSearchResults()})
		return
	}
	var (
		items []repoSearchResult
		err   error
	)
	switch platform {
	case "gitlab":
		items, err = searchGitLabRepos(query)
	default:
		items, err = searchGitHubRepos(query)
	}
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
	} else {
		for idx := range cfg.SkillRepos {
			cfg.SkillRepos[idx] = normalizeSkillRepoRecord(cfg.SkillRepos[idx])
		}
		cfg.SkillRepos = mergeDefaultSkillRepos(cfg.SkillRepos)
	}
	return cfg
}

func (h *ToolingHandler) saveConfig(home string, cfg toolingConfig) error {
	cfg.SkillSyncMethod = normalizeSkillSyncMethod(cfg.SkillSyncMethod)
	if len(cfg.SkillRepos) == 0 {
		cfg.SkillRepos = append([]skillRepoRecord{}, toolingDefaultRepos...)
	} else {
		for idx := range cfg.SkillRepos {
			cfg.SkillRepos[idx] = normalizeSkillRepoRecord(cfg.SkillRepos[idx])
		}
		cfg.SkillRepos = mergeDefaultSkillRepos(cfg.SkillRepos)
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

func normalizeSkillRepoPlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "gitlab":
		return "gitlab"
	default:
		return "github"
	}
}

func normalizeSkillRepoRecord(repo skillRepoRecord) skillRepoRecord {
	repo.Platform = normalizeSkillRepoPlatform(repo.Platform)
	repo.Owner = strings.TrimSpace(repo.Owner)
	repo.Name = strings.TrimSpace(repo.Name)
	repo.Branch = strings.TrimSpace(repo.Branch)
	if repo.Branch == "" {
		repo.Branch = "main"
	}
	if repo.StarCount < 0 {
		repo.StarCount = 0
	}
	return repo
}

func skillRepoMatches(repo skillRepoRecord, platform, owner, name string) bool {
	repo = normalizeSkillRepoRecord(repo)
	platform = normalizeSkillRepoPlatform(platform)
	return strings.EqualFold(repo.Platform, platform) &&
		strings.EqualFold(repo.Owner, owner) &&
		strings.EqualFold(repo.Name, name)
}

func decodeToolingRepoPath(path string) (platform string, owner string, name string, err error) {
	trimmed := strings.TrimPrefix(path, "/tooling/skills/repos/")
	parts := strings.Split(trimmed, "/")
	switch len(parts) {
	case 2:
		return "github", parts[0], parts[1], nil
	case 3:
		return normalizeSkillRepoPlatform(parts[0]), parts[1], parts[2], nil
	default:
		return "", "", "", errors.New("invalid repo path")
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

func mergeDefaultSkillRepos(repos []skillRepoRecord) []skillRepoRecord {
	result := make([]skillRepoRecord, 0, len(repos)+len(toolingDefaultRepos))
	result = append(result, repos...)
	for _, defaultRepo := range toolingDefaultRepos {
		exists := false
		for _, repo := range result {
			if skillRepoMatches(repo, defaultRepo.Platform, defaultRepo.Owner, defaultRepo.Name) {
				exists = true
				break
			}
		}
		if !exists {
			result = append(result, normalizeSkillRepoRecord(defaultRepo))
		}
	}
	return result
}

func repoRecordKey(platform string, owner string, name string) string {
	return fmt.Sprintf(
		"%s:%s/%s",
		normalizeSkillRepoPlatform(platform),
		strings.ToLower(strings.TrimSpace(owner)),
		strings.ToLower(strings.TrimSpace(name)),
	)
}

func parseToolingRepoInput(input string) (skillRepoRecord, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return skillRepoRecord{}, errors.New("repo input is required")
	}
	if !strings.Contains(trimmed, "://") {
		if strings.HasPrefix(strings.ToLower(trimmed), "github.com/") || strings.HasPrefix(strings.ToLower(trimmed), "gitlab.com/") {
			trimmed = "https://" + trimmed
		}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return skillRepoRecord{}, fmt.Errorf("invalid repo url: %w", err)
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
	var platform string
	switch host {
	case "github.com":
		platform = "github"
	case "gitlab.com":
		platform = "gitlab"
	default:
		return skillRepoRecord{}, fmt.Errorf("unsupported repo host: %s", host)
	}
	segments := strings.FieldsFunc(strings.Trim(parsed.Path, "/"), func(r rune) bool {
		return r == '/'
	})
	if len(segments) < 2 {
		return skillRepoRecord{}, errors.New("repo url must include owner and name")
	}
	owner := strings.TrimSpace(segments[0])
	name := strings.TrimSuffix(strings.TrimSpace(segments[1]), ".git")
	if owner == "" || name == "" {
		return skillRepoRecord{}, errors.New("repo url must include owner and name")
	}
	return normalizeSkillRepoRecord(skillRepoRecord{
		Platform: platform,
		Owner:    owner,
		Name:     name,
	}), nil
}

func resolveSkillRepoBranches(platform string, owner string, name string) ([]string, string, error) {
	switch normalizeSkillRepoPlatform(platform) {
	case "gitlab":
		return resolveGitLabRepoBranches(owner, name)
	default:
		return resolveGitHubRepoBranches(owner, name)
	}
}

func resolveGitHubRepoBranches(owner string, name string) ([]string, string, error) {
	defaultBranch, err := fetchGitHubDefaultBranch(owner, name)
	if err != nil {
		return nil, "", err
	}
	req, err := newGitHubAPIRequest(http.MethodGet, fmt.Sprintf("%s/repos/%s/%s/branches", githubAPIBase(), url.PathEscape(owner), url.PathEscape(name)))
	if err != nil {
		return nil, "", err
	}
	params := req.URL.Query()
	params.Set("per_page", "100")
	req.URL.RawQuery = params.Encode()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, "", describeGitHubHTTPError("branches", resp)
	}
	var payload []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", err
	}
	branches := make([]string, 0, len(payload))
	for _, item := range payload {
		if name := strings.TrimSpace(item.Name); name != "" {
			branches = append(branches, name)
		}
	}
	selected := selectPreferredBranch(branches, defaultBranch)
	return uniqueStrings(branches), selected, nil
}

func fetchGitHubDefaultBranch(owner string, name string) (string, error) {
	req, err := newGitHubAPIRequest(http.MethodGet, fmt.Sprintf("%s/repos/%s/%s", githubAPIBase(), url.PathEscape(owner), url.PathEscape(name)))
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", describeGitHubHTTPError("repo", resp)
	}
	var payload struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.DefaultBranch), nil
}

func resolveGitLabRepoBranches(owner string, name string) ([]string, string, error) {
	projectID := url.PathEscape(owner + "/" + name)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/projects/%s/repository/branches", gitlabAPIBase(), projectID), nil)
	if err != nil {
		return nil, "", err
	}
	params := req.URL.Query()
	params.Set("per_page", "100")
	req.URL.RawQuery = params.Encode()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("gitlab branches failed: %s", resp.Status)
	}
	var payload []struct {
		Name    string `json:"name"`
		Default bool   `json:"default"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", err
	}
	branches := make([]string, 0, len(payload))
	defaultBranch := ""
	for _, item := range payload {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		branches = append(branches, name)
		if item.Default && defaultBranch == "" {
			defaultBranch = name
		}
	}
	selected := selectPreferredBranch(branches, defaultBranch)
	return uniqueStrings(branches), selected, nil
}

func selectPreferredBranch(branches []string, fallback string) string {
	candidates := uniqueStrings(branches)
	for _, preferred := range []string{"main", "master"} {
		for _, branch := range candidates {
			if strings.EqualFold(branch, preferred) {
				return branch
			}
		}
	}
	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		for _, branch := range candidates {
			if strings.EqualFold(branch, fallback) {
				return branch
			}
		}
		return fallback
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return "main"
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result
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
			Name:            firstNonEmpty(meta.Name, entry.RelativePath),
			Description:     describeSkillCollection(entry.FullPath),
			Directory:       entry.FullPath,
			SourceClient:    meta.SourceClient,
			SourceRepo:      meta.SourceRepo,
			SourceKind:      firstNonEmpty(meta.SourceKind, "manual"),
			ManagedPath:     entry.FullPath,
			InstalledApps:   map[string]bool{},
			UpdateAvailable: false,
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

func applySkillUpdateFlags(skills []managedSkillRecord, items []discoveredSkillRecord) {
	if len(skills) == 0 || len(items) == 0 {
		return
	}
	updates := map[string]bool{}
	for _, item := range items {
		if !item.UpdateAvailable {
			continue
		}
		updates[strings.ToLower(strings.TrimSpace(item.ManagedName))] = true
	}
	for idx := range skills {
		collectionName := strings.ToLower(strings.TrimSpace(filepath.Base(skills[idx].ManagedPath)))
		if updates[collectionName] {
			skills[idx].UpdateAvailable = true
		}
	}
}

type skillMetadata struct {
	Name         string `json:"name"`
	SourceClient string `json:"source_client"`
	SourceRepo   string `json:"source_repo"`
	SourceKind   string `json:"source_kind"`
	Platform     string `json:"platform,omitempty"`
	Branch       string `json:"branch,omitempty"`
	SourcePath   string `json:"source_path,omitempty"`
	SourceURL    string `json:"source_url,omitempty"`
	UpstreamHash string `json:"upstream_hash,omitempty"`
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
		importSource, skip, err := prepareSkillImportSource(home, entry.FullPath, dstDir)
		if err != nil {
			return count, err
		}
		if skip {
			continue
		}
		if err := copyOrSymlinkDir(importSource, dstDir, "copy"); err != nil {
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

func prepareSkillImportSource(home string, source string, target string) (string, bool, error) {
	if pathExists(target) {
		return "", true, nil
	}
	info, err := os.Lstat(source)
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return source, false, nil
	}
	resolvedSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", false, err
	}
	if pathWithinRoot(managedSkillsRoot(home), resolvedSource) {
		return "", true, nil
	}
	return resolvedSource, false, nil
}

func pathWithinRoot(root string, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func (h *ToolingHandler) refreshSkillDiscovery(home string) ([]discoveredSkillRecord, string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	cfg := h.loadConfig(home)
	cfgDirty := false
	centralRepos, centralErr := fetchCentralTrackedSkillRepos()
	if centralErr == nil && mergeMissingTrackedRepos(&cfg, centralRepos) {
		cfgDirty = true
	}
	existingCache, _ := loadSkillDiscoveryCache(home)
	clients := toolingClientStates(home)
	items, err := discoverSkillsFromCentral(home, clients)
	if err != nil {
		items, err = discoverSkillsFromRepos(home, cfg.SkillRepos, clients)
		if err != nil {
			return nil, "", err
		}
	}
	repoStars := map[string]int{}
	refreshStars := repoStarsRefreshDue(existingCache.RepoStarsFetchedAt)
	if refreshStars {
		repoStars = fetchGitHubRepoStars(cfg.SkillRepos)
	}
	if applyRepoMetrics(&cfg, items, repoStars) {
		cfgDirty = true
	}
	if cfgDirty {
		if err := h.saveConfig(home, cfg); err != nil {
			return nil, "", err
		}
	}
	fetchedAtTime := timeNowUTC()
	fetchedAt := fetchedAtTime.Format(time.RFC3339)
	cache := skillDiscoveryCache{
		FetchedAt:          fetchedAt,
		NextAutoRefreshAt:  nextSkillDiscoveryAutoRefreshAt(fetchedAtTime).Format(time.RFC3339),
		RepoStarsFetchedAt: firstNonEmpty(existingCache.RepoStarsFetchedAt),
		Items:              items,
	}
	if refreshStars {
		cache.RepoStarsFetchedAt = fetchedAt
	}
	if err := saveSkillDiscoveryCache(home, cache); err != nil {
		return nil, "", err
	}
	return items, fetchedAt, nil
}

func mergeMissingTrackedRepos(cfg *toolingConfig, repos []skillRepoRecord) bool {
	changed := false
	for _, repo := range repos {
		repo = normalizeSkillRepoRecord(repo)
		if repo.Owner == "" || repo.Name == "" {
			continue
		}
		exists := false
		for _, existing := range cfg.SkillRepos {
			if skillRepoMatches(existing, repo.Platform, repo.Owner, repo.Name) {
				exists = true
				break
			}
		}
		if !exists {
			cfg.SkillRepos = append(cfg.SkillRepos, repo)
			changed = true
		}
	}
	return changed
}

func fetchCentralTrackedSkillRepos() ([]skillRepoRecord, error) {
	registryURL := strings.TrimSpace(os.Getenv("AIGATE_SKILL_REPO_REGISTRY_URL"))
	if registryURL == "" {
		return nil, nil
	}
	req, err := http.NewRequest(http.MethodGet, registryURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ai-gate")
	if token := strings.TrimSpace(os.Getenv("AIGATE_SKILL_REPO_REGISTRY_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tracked repos fetch failed: %s", resp.Status)
	}
	var payload struct {
		Items []struct {
			Platform  string `json:"platform"`
			Owner     string `json:"owner"`
			Name      string `json:"name"`
			Branch    string `json:"branch"`
			Enabled   bool   `json:"enabled"`
			SortOrder int    `json:"sort_order"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	type centralizedRepo struct {
		Repo      skillRepoRecord
		SortOrder int
	}
	normalized := make([]centralizedRepo, 0, len(payload.Items))
	for _, item := range payload.Items {
		repo := normalizeSkillRepoRecord(skillRepoRecord{
			Platform: item.Platform,
			Owner:    item.Owner,
			Name:     item.Name,
			Branch:   item.Branch,
			Enabled:  item.Enabled,
		})
		if repo.Owner == "" || repo.Name == "" {
			continue
		}
		normalized = append(normalized, centralizedRepo{
			Repo:      repo,
			SortOrder: item.SortOrder,
		})
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].SortOrder != normalized[j].SortOrder {
			return normalized[i].SortOrder < normalized[j].SortOrder
		}
		left := strings.ToLower(normalized[i].Repo.Platform + ":" + normalized[i].Repo.Owner + "/" + normalized[i].Repo.Name)
		right := strings.ToLower(normalized[j].Repo.Platform + ":" + normalized[j].Repo.Owner + "/" + normalized[j].Repo.Name)
		return left < right
	})
	result := make([]skillRepoRecord, 0, len(normalized))
	for _, item := range normalized {
		result = append(result, item.Repo)
	}
	return result, nil
}

func repoStarsRefreshDue(lastFetchedAt string) bool {
	lastFetchedAt = strings.TrimSpace(lastFetchedAt)
	if lastFetchedAt == "" {
		return true
	}
	parsed, err := time.Parse(time.RFC3339, lastFetchedAt)
	if err != nil {
		return true
	}
	return timeNowUTC().Sub(parsed) >= 24*time.Hour
}

func applyRepoMetrics(cfg *toolingConfig, items []discoveredSkillRecord, repoStars map[string]int) bool {
	repoSkillCounts := map[string]int{}
	for _, item := range items {
		repoSkillCounts[repoRecordKey(item.Platform, item.RepoOwner, item.RepoName)]++
	}
	changed := false
	for idx := range cfg.SkillRepos {
		key := repoRecordKey(cfg.SkillRepos[idx].Platform, cfg.SkillRepos[idx].Owner, cfg.SkillRepos[idx].Name)
		nextCount := repoSkillCounts[key]
		if cfg.SkillRepos[idx].SkillCount != nextCount {
			cfg.SkillRepos[idx].SkillCount = nextCount
			changed = true
		}
		if nextStars, ok := repoStars[key]; ok && cfg.SkillRepos[idx].StarCount != nextStars {
			cfg.SkillRepos[idx].StarCount = nextStars
			changed = true
		}
	}
	return changed
}

func fetchGitHubRepoStars(repos []skillRepoRecord) map[string]int {
	stars := map[string]int{}
	lastRequestAt := time.Time{}
	for _, repo := range repos {
		repo = normalizeSkillRepoRecord(repo)
		if repo.Platform != "github" {
			continue
		}
		if !lastRequestAt.IsZero() {
			waitFor := githubRepoRequestInterval - time.Since(lastRequestAt)
			if waitFor > 0 {
				time.Sleep(waitFor)
			}
		}
		count, err := fetchGitHubRepoStarsCount(repo.Owner, repo.Name)
		lastRequestAt = time.Now()
		if err != nil {
			continue
		}
		stars[repoRecordKey(repo.Platform, repo.Owner, repo.Name)] = count
	}
	return stars
}

func fetchGitHubRepoStarsCount(owner string, name string) (int, error) {
	req, err := newGitHubAPIRequest(http.MethodGet, fmt.Sprintf("%s/repos/%s/%s", githubAPIBase(), url.PathEscape(owner), url.PathEscape(name)))
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return 0, describeGitHubHTTPError("repo", resp)
	}
	var payload struct {
		StargazersCount int `json:"stargazers_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, err
	}
	if payload.StargazersCount < 0 {
		return 0, nil
	}
	return payload.StargazersCount, nil
}

func nextSkillDiscoveryAutoRefreshAt(now time.Time) time.Time {
	return now.Add(24*time.Hour + randomDuration(skillDiscoveryAutoRefreshJitterMax))
}

func randomDuration(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	var raw [8]byte
	if _, err := crand.Read(raw[:]); err == nil {
		return time.Duration(binary.BigEndian.Uint64(raw[:]) % uint64(max))
	}
	fallback := uint64(nowUnixNano())
	return time.Duration(fallback % uint64(max))
}

func nowUnixNano() int64 {
	return time.Now().UnixNano()
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
	cleanupRoots := codexManagedMcpCleanupRoots(home, server)
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

func codexManagedMcpCleanupRoots(home string, server managedMcpServer) []string {
	managedRoot := filepath.Clean(filepath.Join(home, ".codex", "mcp"))
	values := make([]string, 0, 8)
	collectSpecStrings(server.Spec, &values)
	tokens := mcpCleanupTokens(server)
	seen := map[string]struct{}{}
	roots := make([]string, 0, len(values))
	for _, raw := range values {
		root, ok := codexManagedMcpRootFromValue(home, managedRoot, raw, tokens)
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

func mcpCleanupTokens(server managedMcpServer) []string {
	seen := map[string]struct{}{}
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
	}
	add(server.ID)
	add(server.Name)
	add(filepath.Base(strings.TrimSpace(server.Name)))
	if command, ok := server.Spec["command"].(string); ok {
		add(filepath.Base(strings.TrimSpace(command)))
	}
	result := make([]string, 0, len(seen))
	for token := range seen {
		result = append(result, token)
	}
	sort.Strings(result)
	return result
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

func codexManagedMcpRootFromValue(home string, managedRoot string, raw string, tokens []string) (string, bool) {
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
		match := codexManagedMcpMatchPath(cleaned, managedRoot, tokens)
		if match != "" {
			return match, true
		}
	}
	return "", false
}

func codexManagedMcpMatchPath(cleaned string, managedRoot string, tokens []string) string {
	current := cleaned
	for {
		rel, err := filepath.Rel(managedRoot, current)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return ""
		}
		base := strings.ToLower(strings.TrimSpace(filepath.Base(current)))
		if !isSharedMcpPathSegment(base) && matchesMcpCleanupToken(base, tokens) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current || parent == managedRoot {
			return ""
		}
		current = parent
	}
}

func matchesMcpCleanupToken(base string, tokens []string) bool {
	for _, token := range tokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		if base == token || strings.Contains(base, token) {
			return true
		}
	}
	return false
}

func isSharedMcpPathSegment(base string) bool {
	switch base {
	case "", ".", "..", "mcp", "servers", "node_modules", ".bin", "bin", "lib", "dist", "out", "build":
		return true
	default:
		return false
	}
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
			Platform: repo.Platform,
			Owner:    repo.Owner,
			Name:     repo.Name,
			Branch:   repo.Branch,
			URL:      buildRepoURL(repo.Platform, repo.Owner, repo.Name),
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
	req, err := newGitHubAPIRequest(http.MethodGet, githubAPIBase()+"/search/repositories")
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
		return nil, describeGitHubHTTPError("search", resp)
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
			Platform:    "github",
			Owner:       owner,
			Name:        name,
			Branch:      firstNonEmpty(item.DefaultBranch, "main"),
			URL:         item.HTMLURL,
			Description: item.Description,
		})
	}
	return results, nil
}

func searchGitLabRepos(query string) ([]repoSearchResult, error) {
	req, err := http.NewRequest(http.MethodGet, gitlabAPIBase()+"/projects", nil)
	if err != nil {
		return nil, err
	}
	params := req.URL.Query()
	params.Set("search", query)
	params.Set("per_page", "8")
	req.URL.RawQuery = params.Encode()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitlab search failed: %s", resp.Status)
	}
	var payload []struct {
		PathWithNamespace string `json:"path_with_namespace"`
		WebURL            string `json:"web_url"`
		Description       string `json:"description"`
		DefaultBranch     string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	results := make([]repoSearchResult, 0, len(payload))
	for _, item := range payload {
		owner, name, found := strings.Cut(item.PathWithNamespace, "/")
		if !found {
			continue
		}
		results = append(results, repoSearchResult{
			Platform:    "gitlab",
			Owner:       owner,
			Name:        name,
			Branch:      firstNonEmpty(item.DefaultBranch, "main"),
			URL:         item.WebURL,
			Description: item.Description,
		})
	}
	return results, nil
}

func githubAPIBase() string {
	return strings.TrimRight(firstNonEmpty(os.Getenv("AIGATE_GITHUB_API_BASE"), "https://api.github.com"), "/")
}

func githubArchiveBase() string {
	return strings.TrimRight(firstNonEmpty(os.Getenv("AIGATE_GITHUB_ARCHIVE_BASE"), "https://github.com"), "/")
}

func gitlabAPIBase() string {
	return strings.TrimRight(firstNonEmpty(os.Getenv("AIGATE_GITLAB_API_BASE"), "https://gitlab.com/api/v4"), "/")
}

func githubAuthToken() string {
	return strings.TrimSpace(firstNonEmpty(
		os.Getenv("AIGATE_GITHUB_TOKEN"),
		os.Getenv("GITHUB_TOKEN"),
		os.Getenv("GH_TOKEN"),
		os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN"),
	))
}

func newGitHubAPIRequest(method string, targetURL string) (*http.Request, error) {
	req, err := http.NewRequest(method, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ai-gate")
	if token := githubAuthToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func newGitHubArchiveRequest(repo skillRepoRecord) (*http.Request, error) {
	archiveURL := fmt.Sprintf("%s/%s/%s/archive/refs/heads/%s.zip", githubArchiveBase(), url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(repo.Branch))
	req, err := http.NewRequest(http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ai-gate")
	return req, nil
}

func describeGitHubHTTPError(kind string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	bodyText := strings.TrimSpace(string(body))
	message := bodyText
	if strings.HasPrefix(bodyText, "{") {
		var payload struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &payload) == nil && strings.TrimSpace(payload.Message) != "" {
			message = strings.TrimSpace(payload.Message)
		}
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if resp.Header.Get("X-RateLimit-Remaining") == "0" || strings.Contains(strings.ToLower(message), "rate limit") {
			resetHint := ""
			if rawReset := strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")); rawReset != "" {
				if ts, err := strconv.ParseInt(rawReset, 10, 64); err == nil {
					resetHint = fmt.Sprintf("，预计 %s 重置", time.Unix(ts, 0).UTC().Format(time.RFC3339))
				}
			}
			if githubAuthToken() == "" {
				return fmt.Errorf("github %s 触发匿名请求限流%s，请稍后重试或配置 GitHub Token", kind, resetHint)
			}
			return fmt.Errorf("github %s 触发请求限流%s，请稍后重试", kind, resetHint)
		}
	}
	if message != "" && message != resp.Status {
		return fmt.Errorf("github %s failed: %s (%s)", kind, resp.Status, message)
	}
	return fmt.Errorf("github %s failed: %s", kind, resp.Status)
}

func loadSkillDiscoveryCache(home string) (skillDiscoveryCache, bool) {
	path := skillDiscoveryCachePath(home)
	raw, err := os.ReadFile(path)
	if err != nil {
		return skillDiscoveryCache{}, false
	}
	var cache skillDiscoveryCache
	if err := json.Unmarshal(raw, &cache); err != nil {
		return skillDiscoveryCache{}, false
	}
	return cache, true
}

func saveSkillDiscoveryCache(home string, cache skillDiscoveryCache) error {
	cache = compactSkillDiscoveryCache(cache)
	raw, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(skillDiscoveryCachePath(home), append(raw, '\n'), 0o600)
}

func skillDiscoveryCachePath(home string) string {
	return filepath.Join(aigateDataRoot(home), "tooling", "skill-discovery-cache.json")
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}

func compactSkillDiscoveryCache(cache skillDiscoveryCache) skillDiscoveryCache {
	normalizedItems := make([]discoveredSkillRecord, 0, len(cache.Items))
	for _, item := range cache.Items {
		item.Description = strings.TrimSpace(item.Description)
		if len(item.Description) > skillDiscoveryDescriptionMaxLength {
			item.Description = strings.TrimSpace(item.Description[:skillDiscoveryDescriptionMaxLength]) + "..."
		}
		if len(item.InstalledApps) == 0 {
			item.InstalledApps = nil
		}
		normalizedItems = append(normalizedItems, item)
	}
	cache.Items = normalizedItems
	if fitsSkillDiscoveryCacheLimit(cache) {
		return cache
	}
	for len(cache.Items) > 1 && !fitsSkillDiscoveryCacheLimit(cache) {
		nextLen := int(float64(len(cache.Items)) * 0.9)
		if nextLen >= len(cache.Items) {
			nextLen = len(cache.Items) - 1
		}
		if nextLen < 1 {
			nextLen = 1
		}
		cache.Items = cache.Items[:nextLen]
	}
	if !fitsSkillDiscoveryCacheLimit(cache) {
		cache.Items = nil
	}
	return cache
}

func fitsSkillDiscoveryCacheLimit(cache skillDiscoveryCache) bool {
	raw, err := json.Marshal(cache)
	if err != nil {
		return false
	}
	return len(raw) <= skillDiscoveryCacheMaxBytes
}

func parseDiscoveredPaging(query url.Values) (limit int, offset int) {
	limit = 100
	offset = 0
	if query == nil {
		return limit, offset
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	if limit > 500 {
		limit = 500
	}
	return limit, offset
}

func sliceDiscoveredItems(items []discoveredSkillRecord, limit int, offset int) ([]discoveredSkillRecord, int) {
	total := len(items)
	if offset >= total {
		return []discoveredSkillRecord{}, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return items[offset:end], total
}

func discoverSkillsFromCentral(home string, clients []toolingClientState) ([]discoveredSkillRecord, error) {
	baseURL := skillMetricsBaseURL()
	if baseURL == "" {
		return nil, errors.New("central skill discovery is not configured")
	}
	catalog, err := fetchCentralSkillsCatalog(baseURL)
	if err != nil {
		return nil, err
	}
	installed := discoveredInstalledAppsBySource(home, clients)
	hashCache := map[string]string{}
	items := make([]discoveredSkillRecord, 0, len(catalog.Items))
	for _, raw := range catalog.Items {
		repo := normalizeSkillRepoRecord(skillRepoRecord{
			Platform: raw.Platform,
			Owner:    raw.RepoOwner,
			Name:     raw.RepoName,
			Branch:   raw.Branch,
			Enabled:  true,
		})
		sourcePath := strings.Trim(strings.TrimSpace(raw.SourcePath), "/")
		if sourcePath == "" {
			sourcePath = strings.Trim(strings.TrimSpace(raw.Name), "/")
		}
		key := firstNonEmpty(strings.TrimSpace(raw.ID), discoveredSkillKey(repo.Platform, repo.Owner+"/"+repo.Name, repo.Branch, sourcePath))
		record := discoveredSkillRecord{
			ID:              key,
			Name:            firstNonEmpty(strings.TrimSpace(raw.Name), filepath.Base(sourcePath), "Skill"),
			Platform:        repo.Platform,
			RepoOwner:       repo.Owner,
			RepoName:        repo.Name,
			Branch:          repo.Branch,
			RepoURL:         firstNonEmpty(strings.TrimSpace(raw.RepoURL), buildRepoURL(repo.Platform, repo.Owner, repo.Name)),
			SourcePath:      sourcePath,
			SourceURL:       firstNonEmpty(strings.TrimSpace(raw.SourceURL), buildRepoTreeURL(repo.Platform, repo.Owner, repo.Name, repo.Branch, sourcePath)),
			ManagedName:     buildDiscoveredManagedName(repo, sourcePath),
			InstalledApps:   map[string]bool{},
			UpdateAvailable: false,
		}
		if state, ok := installed[key]; ok {
			record.InstalledHash = state.Hash
			for app, enabled := range state.Apps {
				record.InstalledApps[app] = enabled
			}
			if cachedHash, ok := hashCache[key]; ok {
				record.ContentHash = cachedHash
			} else {
				contentHash, hashErr := fetchDiscoveredSkillContentHash(repo, sourcePath)
				if hashErr == nil {
					record.ContentHash = contentHash
					hashCache[key] = contentHash
				}
			}
			record.UpdateAvailable = state.Hash != "" && record.ContentHash != "" && state.Hash != record.ContentHash
		}
		items = append(items, record)
	}
	recommended, _ := fetchCentralRecommendedTop10(baseURL)
	return reorderDiscoveredWithRecommendations(items, recommended), nil
}

func reorderDiscoveredWithRecommendations(items []discoveredSkillRecord, recommended []string) []discoveredSkillRecord {
	if len(items) == 0 {
		return items
	}
	byID := make(map[string]discoveredSkillRecord, len(items))
	byName := make(map[string][]string)
	orderedIDs := make([]string, 0, len(items))
	for _, item := range items {
		byID[item.ID] = item
		orderedIDs = append(orderedIDs, item.ID)
		nameKey := strings.ToLower(strings.TrimSpace(item.Name))
		byName[nameKey] = append(byName[nameKey], item.ID)
	}
	seen := map[string]bool{}
	result := make([]discoveredSkillRecord, 0, len(items))
	for _, name := range recommended {
		key := strings.ToLower(strings.TrimSpace(name))
		for _, id := range byName[key] {
			if seen[id] {
				continue
			}
			seen[id] = true
			result = append(result, byID[id])
		}
	}
	sort.SliceStable(orderedIDs, func(i, j int) bool {
		return strings.ToLower(byID[orderedIDs[i]].Name) < strings.ToLower(byID[orderedIDs[j]].Name)
	})
	for _, id := range orderedIDs {
		if seen[id] {
			continue
		}
		result = append(result, byID[id])
	}
	return result
}

func fetchCentralSkillsCatalog(baseURL string) (centralDiscoveredSkillResponse, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/skills/final?limit=5000&offset=0", nil)
	if err != nil {
		return centralDiscoveredSkillResponse{}, err
	}
	req.Header.Set("User-Agent", "ai-gate")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return centralDiscoveredSkillResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return centralDiscoveredSkillResponse{}, fmt.Errorf("central skills failed: %s", resp.Status)
	}
	var payload centralDiscoveredSkillResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return centralDiscoveredSkillResponse{}, err
	}
	return payload, nil
}

func fetchCentralRecommendedTop10(baseURL string) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/rankings/skills?limit=10", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ai-gate")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("central rankings failed: %s", resp.Status)
	}
	var payload struct {
		Items []struct {
			SkillName string `json:"skill_name"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(payload.Items))
	for _, item := range payload.Items {
		if strings.TrimSpace(item.SkillName) == "" {
			continue
		}
		names = append(names, item.SkillName)
	}
	return names, nil
}

func fetchDiscoveredSkillContentHash(repo skillRepoRecord, sourcePath string) (string, error) {
	files, err := fetchDiscoveredSkillFiles(repo, sourcePath)
	if err != nil {
		return "", err
	}
	return hashSkillFiles(files), nil
}

func skillMetricsBaseURL() string {
	if direct := strings.TrimSpace(os.Getenv("AIGATE_SKILL_METRICS_URL")); direct != "" {
		return strings.TrimRight(direct, "/")
	}
	registryURL := strings.TrimSpace(os.Getenv("AIGATE_SKILL_REPO_REGISTRY_URL"))
	if registryURL == "" {
		return ""
	}
	parsed, err := url.Parse(registryURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func toolingAnonymousClientIDPath(home string) string {
	return filepath.Join(aigateDataRoot(home), "tooling", "anonymous-client.json")
}

func loadOrCreateToolingAnonymousID(home string) (string, error) {
	path := toolingAnonymousClientIDPath(home)
	if raw, err := os.ReadFile(path); err == nil {
		var payload struct {
			AnonymousID string `json:"anonymous_id"`
		}
		if json.Unmarshal(raw, &payload) == nil && strings.TrimSpace(payload.AnonymousID) != "" {
			return strings.TrimSpace(payload.AnonymousID), nil
		}
	}
	var nonce [16]byte
	if _, err := crand.Read(nonce[:]); err != nil {
		return "", err
	}
	id := fmt.Sprintf("aigate-%x", nonce[:])
	payload := map[string]string{"anonymous_id": id}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := writeAtomic(path, append(raw, '\n'), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

func reportDiscoveredSkillInstall(home string, id string) {
	baseURL := skillMetricsBaseURL()
	if baseURL == "" {
		return
	}
	platform, repoFullName, _, sourcePath, err := parseDiscoveredSkillID(id)
	if err != nil {
		return
	}
	anonymousID, err := loadOrCreateToolingAnonymousID(home)
	if err != nil {
		return
	}
	skillName := filepath.Base(strings.Trim(sourcePath, "/"))
	if skillName == "" || skillName == "." {
		skillName = sourcePath
	}
	payload := map[string]string{
		"anonymous_id": anonymousID,
		"skill_name":   skillName,
		"source_repo":  repoFullName,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/events/install", bytes.NewReader(raw))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ai-gate")
	if token := strings.TrimSpace(os.Getenv("AIGATE_SKILL_METRICS_INGEST_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if platform == "" {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

func discoverSkillsFromRepos(home string, repos []skillRepoRecord, clients []toolingClientState) ([]discoveredSkillRecord, error) {
	installed := discoveredInstalledAppsBySource(home, clients)
	items := make([]discoveredSkillRecord, 0)
	success := 0
	var firstErr error
	for _, repo := range repos {
		repo = normalizeSkillRepoRecord(repo)
		if !repo.Enabled {
			continue
		}
		repoItems, err := discoverSkillsFromRepo(repo, installed)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s/%s@%s: %w", repo.Owner, repo.Name, repo.Branch, err)
			}
			continue
		}
		success++
		items = append(items, repoItems...)
	}
	if success == 0 && firstErr != nil {
		return nil, firstErr
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

func discoveredInstalledAppsBySource(home string, clients []toolingClientState) map[string]discoveredInstallState {
	installed := map[string]discoveredInstallState{}
	for _, record := range scanManagedSkills(home, clients) {
		meta := readSkillMetadata(record.ManagedPath)
		key := discoveredSkillKey(meta.Platform, meta.SourceRepo, meta.Branch, meta.SourcePath)
		if key == "" {
			continue
		}
		apps := map[string]bool{}
		for app, enabled := range record.InstalledApps {
			apps[app] = enabled
		}
		hash := strings.TrimSpace(meta.UpstreamHash)
		if hash == "" {
			hash = hashSkillDirectory(record.ManagedPath)
		}
		installed[key] = discoveredInstallState{
			Apps: apps,
			Hash: hash,
		}
	}
	return installed
}

func discoverSkillsFromRepo(repo skillRepoRecord, installed map[string]discoveredInstallState) ([]discoveredSkillRecord, error) {
	switch repo.Platform {
	case "gitlab":
		return discoverGitLabRepoSkills(repo, installed)
	default:
		return discoverGitHubRepoSkills(repo, installed)
	}
}

func discoverGitHubRepoSkills(repo skillRepoRecord, installed map[string]discoveredInstallState) ([]discoveredSkillRecord, error) {
	files, err := fetchGitHubArchiveFiles(repo, func(filePath string) bool {
		return true
	})
	if err != nil {
		return nil, err
	}
	skillFiles := groupDiscoveredSkillFiles(files)
	paths := make([]string, 0, len(skillFiles))
	for sourcePath := range skillFiles {
		paths = append(paths, sourcePath)
	}
	sort.Strings(paths)
	items := make([]discoveredSkillRecord, 0)
	for _, sourcePath := range paths {
		items = append(items, buildDiscoveredSkillRecord(repo, sourcePath, skillFiles[sourcePath], installed))
	}
	return items, nil
}

func fetchGitHubArchiveFiles(repo skillRepoRecord, match func(string) bool) (map[string]string, error) {
	var archive []byte
	for attempt := 1; attempt <= githubArchiveRetryMaxAttempts; attempt++ {
		req, err := newGitHubArchiveRequest(repo)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 300 {
			archive, err = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				return nil, err
			}
			break
		}
		httpErr := describeGitHubHTTPError("archive", resp)
		_ = resp.Body.Close()
		if attempt < githubArchiveRetryMaxAttempts && isGitHubRateLimitError(httpErr) {
			time.Sleep(time.Duration(attempt) * githubArchiveRetryBaseDelay)
			continue
		}
		return nil, httpErr
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	for _, file := range reader.File {
		relativePath := trimGitHubArchivePath(file.Name)
		if relativePath == "" || file.FileInfo().IsDir() || !match(relativePath) {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(handle)
		closeErr := handle.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		files[relativePath] = string(raw)
	}
	return files, nil
}

func isGitHubRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "限流") || strings.Contains(message, "rate limit")
}

func trimGitHubArchivePath(filePath string) string {
	filePath = strings.Trim(filePath, "/")
	if filePath == "" {
		return ""
	}
	_, relativePath, found := strings.Cut(filePath, "/")
	if !found {
		return ""
	}
	return strings.Trim(relativePath, "/")
}

func discoverGitLabRepoSkills(repo skillRepoRecord, installed map[string]discoveredInstallState) ([]discoveredSkillRecord, error) {
	projectID := url.PathEscape(repo.Owner + "/" + repo.Name)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/projects/%s/repository/tree", gitlabAPIBase(), projectID), nil)
	if err != nil {
		return nil, err
	}
	params := req.URL.Query()
	params.Set("ref", repo.Branch)
	params.Set("recursive", "true")
	params.Set("per_page", "100")
	req.URL.RawQuery = params.Encode()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitlab tree failed: %s", resp.Status)
	}
	var payload []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	skillFiles, err := fetchGitLabSkillDiscoveryFiles(repo, projectID, payload)
	if err != nil {
		return nil, err
	}
	items := make([]discoveredSkillRecord, 0)
	paths := make([]string, 0, len(skillFiles))
	for sourcePath := range skillFiles {
		paths = append(paths, sourcePath)
	}
	sort.Strings(paths)
	for _, sourcePath := range paths {
		items = append(items, buildDiscoveredSkillRecord(repo, sourcePath, skillFiles[sourcePath], installed))
	}
	return items, nil
}

func fetchGitLabFile(repo skillRepoRecord, projectID string, path string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/projects/%s/repository/files/%s/raw", gitlabAPIBase(), projectID, url.PathEscape(path)), nil)
	if err != nil {
		return "", err
	}
	params := req.URL.Query()
	params.Set("ref", repo.Branch)
	req.URL.RawQuery = params.Encode()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("gitlab raw failed: %s", resp.Status)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func fetchGitLabSkillDiscoveryFiles(repo skillRepoRecord, projectID string, payload []struct {
	Path string `json:"path"`
	Type string `json:"type"`
}) (map[string]map[string]string, error) {
	roots := collectDiscoveredSkillRootsFromTree(payload)
	files := map[string]map[string]string{}
	for _, root := range roots {
		files[root] = map[string]string{}
	}
	for _, entry := range payload {
		if entry.Type != "blob" {
			continue
		}
		root, ok := matchDiscoveredSkillRoot(roots, entry.Path)
		if !ok {
			continue
		}
		raw, err := fetchGitLabFile(repo, projectID, entry.Path)
		if err != nil {
			return nil, err
		}
		files[root][strings.TrimPrefix(strings.TrimPrefix(entry.Path, root), "/")] = raw
	}
	for root, entries := range files {
		if _, ok := entries["SKILL.md"]; ok {
			continue
		}
		delete(files, root)
	}
	return files, nil
}

func collectDiscoveredSkillRootsFromTree(payload []struct {
	Path string `json:"path"`
	Type string `json:"type"`
}) []string {
	roots := make([]string, 0)
	for _, entry := range payload {
		if entry.Type != "blob" || !strings.HasSuffix(entry.Path, "/SKILL.md") {
			continue
		}
		root := strings.Trim(strings.TrimSuffix(entry.Path, "/SKILL.md"), "/")
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		if len(roots[i]) == len(roots[j]) {
			return roots[i] < roots[j]
		}
		return len(roots[i]) > len(roots[j])
	})
	return roots
}

func groupDiscoveredSkillFiles(files map[string]string) map[string]map[string]string {
	roots := make([]string, 0)
	for filePath := range files {
		if !strings.HasSuffix(filePath, "/SKILL.md") {
			continue
		}
		root := strings.Trim(strings.TrimSuffix(filePath, "/SKILL.md"), "/")
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		if len(roots[i]) == len(roots[j]) {
			return roots[i] < roots[j]
		}
		return len(roots[i]) > len(roots[j])
	})
	grouped := map[string]map[string]string{}
	for _, root := range roots {
		grouped[root] = map[string]string{}
	}
	for filePath, raw := range files {
		root, ok := matchDiscoveredSkillRoot(roots, filePath)
		if !ok {
			continue
		}
		grouped[root][strings.TrimPrefix(strings.TrimPrefix(filePath, root), "/")] = raw
	}
	for root, entries := range grouped {
		if _, ok := entries["SKILL.md"]; ok {
			continue
		}
		delete(grouped, root)
	}
	return grouped
}

func matchDiscoveredSkillRoot(roots []string, filePath string) (string, bool) {
	for _, root := range roots {
		if pathWithinDiscoveredSkill(root, filePath) {
			return root, true
		}
	}
	return "", false
}

func hashSkillFiles(files map[string]string) string {
	if len(files) == 0 {
		return ""
	}
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	digest := sha256.New()
	for _, key := range keys {
		_, _ = digest.Write([]byte(key))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(files[key]))
		_, _ = digest.Write([]byte{0})
	}
	return fmt.Sprintf("%x", digest.Sum(nil))
}

func hashSkillDirectory(root string) string {
	files := map[string]string{}
	_ = filepath.WalkDir(root, func(current string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if current != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		if rel == ".aigate-skill.json" {
			return nil
		}
		raw, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(raw)
		return nil
	})
	return hashSkillFiles(files)
}

func buildDiscoveredSkillRecord(repo skillRepoRecord, sourcePath string, files map[string]string, installed map[string]discoveredInstallState) discoveredSkillRecord {
	sourcePath = strings.Trim(sourcePath, "/")
	body := files["SKILL.md"]
	name, description := parseDiscoveredSkillBody(body, sourcePath)
	key := discoveredSkillKey(repo.Platform, repo.Owner+"/"+repo.Name, repo.Branch, sourcePath)
	contentHash := hashSkillFiles(files)
	record := discoveredSkillRecord{
		ID:              key,
		Name:            name,
		Description:     description,
		Platform:        repo.Platform,
		RepoOwner:       repo.Owner,
		RepoName:        repo.Name,
		Branch:          repo.Branch,
		RepoURL:         buildRepoURL(repo.Platform, repo.Owner, repo.Name),
		SourcePath:      sourcePath,
		SourceURL:       buildRepoTreeURL(repo.Platform, repo.Owner, repo.Name, repo.Branch, sourcePath),
		ManagedName:     buildDiscoveredManagedName(repo, sourcePath),
		ContentHash:     contentHash,
		InstalledApps:   map[string]bool{},
		UpdateAvailable: false,
	}
	if state, ok := installed[key]; ok {
		record.InstalledHash = state.Hash
		record.UpdateAvailable = state.Hash != "" && contentHash != "" && state.Hash != contentHash
		for app, enabled := range state.Apps {
			record.InstalledApps[app] = enabled
		}
	}
	return record
}

func buildRepoURL(platform string, owner string, name string) string {
	switch normalizeSkillRepoPlatform(platform) {
	case "gitlab":
		return fmt.Sprintf("https://gitlab.com/%s/%s", owner, name)
	default:
		return fmt.Sprintf("https://github.com/%s/%s", owner, name)
	}
}

func buildRepoTreeURL(platform string, owner string, name string, branch string, sourcePath string) string {
	base := buildRepoURL(platform, owner, name)
	sourcePath = strings.Trim(sourcePath, "/")
	switch normalizeSkillRepoPlatform(platform) {
	case "gitlab":
		if sourcePath == "" {
			return fmt.Sprintf("%s/-/tree/%s", base, branch)
		}
		return fmt.Sprintf("%s/-/tree/%s/%s", base, branch, sourcePath)
	default:
		if sourcePath == "" {
			return fmt.Sprintf("%s/tree/%s", base, branch)
		}
		return fmt.Sprintf("%s/tree/%s/%s", base, branch, sourcePath)
	}
}

func buildDiscoveredManagedName(repo skillRepoRecord, sourcePath string) string {
	base := filepath.Base(strings.Trim(sourcePath, "/"))
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = repo.Name
	}
	name := strings.ToLower(repo.Name + "-" + base)
	replacer := strings.NewReplacer("/", "-", "\\", "-", "_", "-", " ", "-", ".", "-")
	name = replacer.Replace(name)
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	return strings.Trim(name, "-")
}

func parseDiscoveredSkillBody(body string, sourcePath string) (string, string) {
	name := ""
	description := ""
	lines := strings.Split(body, "\n")
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
			switch {
			case strings.HasPrefix(trimmed, "name:") && name == "":
				name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "name:")), `"'`)
			case strings.HasPrefix(trimmed, "title:") && name == "":
				name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "title:")), `"'`)
			case strings.HasPrefix(trimmed, "description:") && description == "":
				description = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "description:")), `"'`)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") && name == "" {
			name = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			continue
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") && description == "" {
			description = trimmed
		}
	}
	if name == "" {
		name = filepath.Base(strings.Trim(sourcePath, "/"))
		if name == "." || name == "" {
			name = "Skill"
		}
	}
	return name, description
}

func discoveredSkillKey(platform string, sourceRepo string, branch string, sourcePath string) string {
	if strings.TrimSpace(sourceRepo) == "" {
		return ""
	}
	return strings.ToLower(strings.Join([]string{
		normalizeSkillRepoPlatform(platform),
		strings.TrimSpace(sourceRepo),
		firstNonEmpty(branch, "main"),
		strings.Trim(strings.TrimSpace(sourcePath), "/"),
	}, ":"))
}

func encodeRepoPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for idx, part := range parts {
		parts[idx] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func installDiscoveredSkillCollection(home string, id string, method string, apps []string) (int, error) {
	platform, repoFullName, branch, sourcePath, err := parseDiscoveredSkillID(id)
	if err != nil {
		return 0, err
	}
	owner, name, found := strings.Cut(repoFullName, "/")
	if !found {
		return 0, fmt.Errorf("invalid discovered skill repo: %s", repoFullName)
	}
	repo := normalizeSkillRepoRecord(skillRepoRecord{
		Platform: platform,
		Owner:    owner,
		Name:     name,
		Branch:   branch,
		Enabled:  true,
	})
	managedName := buildDiscoveredManagedName(repo, sourcePath)
	targetDir := filepath.Join(managedSkillsRoot(home), managedName)
	files, err := fetchDiscoveredSkillFiles(repo, sourcePath)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, fmt.Errorf("no files found for discovered skill: %s", id)
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return 0, err
	}
	for relativePath, raw := range files {
		fullPath := filepath.Join(targetDir, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(fullPath, []byte(raw), 0o644); err != nil {
			return 0, err
		}
	}
	if err := writeSkillMetadata(targetDir, skillMetadata{
		Name:         managedName,
		SourceRepo:   repoFullName,
		SourceKind:   "discovered",
		Platform:     repo.Platform,
		Branch:       repo.Branch,
		SourcePath:   sourcePath,
		SourceURL:    buildRepoTreeURL(repo.Platform, repo.Owner, repo.Name, repo.Branch, sourcePath),
		UpstreamHash: hashSkillFiles(files),
	}); err != nil {
		return 0, err
	}
	return applyManagedSkillCollection(home, managedName, method, apps)
}

func parseDiscoveredSkillID(id string) (platform string, repoFullName string, branch string, sourcePath string, err error) {
	parts := strings.SplitN(strings.TrimSpace(id), ":", 4)
	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("invalid discovered skill id: %s", id)
	}
	return normalizeSkillRepoPlatform(parts[0]), parts[1], firstNonEmpty(parts[2], "main"), strings.Trim(parts[3], "/"), nil
}

func fetchDiscoveredSkillFiles(repo skillRepoRecord, sourcePath string) (map[string]string, error) {
	switch repo.Platform {
	case "gitlab":
		return fetchGitLabSkillFiles(repo, sourcePath)
	default:
		return fetchGitHubSkillFiles(repo, sourcePath)
	}
}

func fetchGitHubSkillFiles(repo skillRepoRecord, sourcePath string) (map[string]string, error) {
	prefix := strings.Trim(sourcePath, "/")
	files, err := fetchGitHubArchiveFiles(repo, func(filePath string) bool {
		return pathWithinDiscoveredSkill(prefix, filePath)
	})
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for filePath, raw := range files {
		result[strings.TrimPrefix(strings.TrimPrefix(filePath, prefix), "/")] = raw
	}
	return result, nil
}

func fetchGitLabSkillFiles(repo skillRepoRecord, sourcePath string) (map[string]string, error) {
	projectID := url.PathEscape(repo.Owner + "/" + repo.Name)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/projects/%s/repository/tree", gitlabAPIBase(), projectID), nil)
	if err != nil {
		return nil, err
	}
	params := req.URL.Query()
	params.Set("ref", repo.Branch)
	params.Set("recursive", "true")
	params.Set("per_page", "100")
	req.URL.RawQuery = params.Encode()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitlab tree failed: %s", resp.Status)
	}
	var payload []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	files := map[string]string{}
	prefix := strings.Trim(sourcePath, "/")
	for _, entry := range payload {
		if entry.Type != "blob" || !pathWithinDiscoveredSkill(prefix, entry.Path) {
			continue
		}
		raw, err := fetchGitLabFile(repo, projectID, entry.Path)
		if err != nil {
			return nil, err
		}
		files[strings.TrimPrefix(strings.TrimPrefix(entry.Path, prefix), "/")] = raw
	}
	return files, nil
}

func pathWithinDiscoveredSkill(prefix string, fullPath string) bool {
	prefix = strings.Trim(prefix, "/")
	fullPath = strings.Trim(fullPath, "/")
	if prefix == "" {
		return true
	}
	return fullPath == prefix || strings.HasPrefix(fullPath, prefix+"/")
}
