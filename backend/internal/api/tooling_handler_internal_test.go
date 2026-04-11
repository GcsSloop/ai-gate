package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReportToolingClientActiveFallsBackWhenAnonymousFileCannotPersist(t *testing.T) {
	home := t.TempDir()
	blockToolingDirWithFile(t, home)

	var (
		mu      sync.Mutex
		payload map[string]string
		hits    int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/events/install" {
			http.NotFound(w, r)
			return
		}
		defer r.Body.Close()
		raw := map[string]string{}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		mu.Lock()
		hits++
		payload = raw
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"inserted":true}`))
	}))
	t.Cleanup(server.Close)
	setDefaultToolingSkillMetricsBaseURLForTest(t, server.URL)

	reportToolingClientActive(home)

	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("events/install hits = %d, want 1", hits)
	}
	if got := strings.TrimSpace(payload["anonymous_id"]); got == "" {
		t.Fatalf("anonymous_id empty, payload = %#v", payload)
	}
	if payload["skill_name"] != skillMetricsHeartbeatName {
		t.Fatalf("skill_name = %q, want %q", payload["skill_name"], skillMetricsHeartbeatName)
	}
	if payload["source_repo"] != skillMetricsHeartbeatSource {
		t.Fatalf("source_repo = %q, want %q", payload["source_repo"], skillMetricsHeartbeatSource)
	}
	if len(payload) != 3 {
		t.Fatalf("payload fields = %v, want only anonymous_id/skill_name/source_repo", payload)
	}
}

func TestReportDiscoveredSkillInstallFallsBackWhenAnonymousFileCannotPersist(t *testing.T) {
	home := t.TempDir()
	blockToolingDirWithFile(t, home)

	var (
		mu      sync.Mutex
		payload map[string]string
		hits    int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/events/install" {
			http.NotFound(w, r)
			return
		}
		defer r.Body.Close()
		raw := map[string]string{}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		mu.Lock()
		hits++
		payload = raw
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"inserted":true}`))
	}))
	t.Cleanup(server.Close)
	setDefaultToolingSkillMetricsBaseURLForTest(t, server.URL)

	id := discoveredSkillKey("github", "openai/skills", "main", "collections/demo-skill")
	reportDiscoveredSkillInstall(home, id)

	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("events/install hits = %d, want 1", hits)
	}
	if got := strings.TrimSpace(payload["anonymous_id"]); got == "" {
		t.Fatalf("anonymous_id empty, payload = %#v", payload)
	}
	if payload["skill_name"] != "demo-skill" {
		t.Fatalf("skill_name = %q, want demo-skill", payload["skill_name"])
	}
	if payload["source_repo"] != "openai/skills" {
		t.Fatalf("source_repo = %q, want openai/skills", payload["source_repo"])
	}
	if len(payload) != 3 {
		t.Fatalf("payload fields = %v, want only anonymous_id/skill_name/source_repo", payload)
	}
}

func TestReportToolingClientActiveIsThrottledWithinOneHour(t *testing.T) {
	home := t.TempDir()

	var (
		mu   sync.Mutex
		hits int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/events/install" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"inserted":true}`))
	}))
	t.Cleanup(server.Close)
	setToolingSkillMetricsBaseURL(t, home, server.URL)

	reportToolingClientActive(home)
	reportToolingClientActive(home)

	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("events/install hits = %d, want 1 within active throttle window", hits)
	}
}

func TestReportToolingClientActiveReportsAgainAfterOneHour(t *testing.T) {
	home := t.TempDir()
	now := time.Now().UTC()
	if err := saveSkillMetricsState(home, skillMetricsReportState{
		LastActiveAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	var (
		mu   sync.Mutex
		hits int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/events/install" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"inserted":true}`))
	}))
	t.Cleanup(server.Close)
	setToolingSkillMetricsBaseURL(t, home, server.URL)

	reportToolingClientActive(home)

	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("events/install hits = %d, want 1 for overdue active report", hits)
	}
}

func TestReportDiscoveredSkillInstallDoesNotQueueOnNetworkFailure(t *testing.T) {
	home := t.TempDir()
	setToolingSkillMetricsBaseURL(t, home, "http://127.0.0.1:1")

	id := discoveredSkillKey("github", "openai/skills", "main", "collections/demo-skill")
	reportDiscoveredSkillInstall(home, id)

	pendingPath := filepath.Join(aigateDataRoot(home), "tooling", "skill-metrics-pending.json")
	if _, err := os.Stat(pendingPath); !os.IsNotExist(err) {
		t.Fatalf("pending queue file should not exist, stat err = %v", err)
	}
}

func blockToolingDirWithFile(t *testing.T, home string) {
	t.Helper()
	toolingPath := filepath.Join(aigateDataRoot(home), "tooling")
	if err := os.MkdirAll(filepath.Dir(toolingPath), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(toolingPath, []byte("blocked"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
}

func setToolingSkillMetricsBaseURL(t *testing.T, home string, baseURL string) {
	t.Helper()
	cfg := loadToolingConfig(home)
	cfg.SkillMetricsBaseURL = baseURL
	cfg.SkillRepoRegistryURL = defaultToolingRepoRegistryURL(baseURL)
	if err := saveToolingConfig(home, cfg); err != nil {
		t.Fatalf("save tooling config: %v", err)
	}
}

func setDefaultToolingSkillMetricsBaseURLForTest(t *testing.T, baseURL string) {
	t.Helper()
	prev := defaultToolingSkillMetricsBaseURL
	defaultToolingSkillMetricsBaseURL = baseURL
	t.Cleanup(func() {
		defaultToolingSkillMetricsBaseURL = prev
	})
}

func TestNormalizeDiscoveredSourcePathRemovesSkillMarkdownSuffix(t *testing.T) {
	got := normalizeDiscoveredSourcePath("collections/demo/SKILL.md")
	if got != "collections/demo" {
		t.Fatalf("normalizeDiscoveredSourcePath = %q, want collections/demo", got)
	}
}

func TestDiscoveredSkillKeyNormalizesSourcePath(t *testing.T) {
	key := discoveredSkillKey("github", "openai/skills", "main", "collections/demo/SKILL.md")
	if key != "github:openai/skills:main:collections/demo" {
		t.Fatalf("discoveredSkillKey = %q, want normalized skill dir key", key)
	}
}

func TestLoadConfigAutoFillsSkillMetricsURLs(t *testing.T) {
	home := t.TempDir()
	path := toolingConfigPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir tooling dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"skill_sync_method":"copy","skill_repos":[]}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h := NewToolingHandler()
	cfg := h.loadConfig(home)
	if cfg.SkillMetricsBaseURL == "" {
		t.Fatalf("SkillMetricsBaseURL should be auto-filled")
	}
	if cfg.SkillRepoRegistryURL == "" {
		t.Fatalf("SkillRepoRegistryURL should be auto-filled")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(raw), `"skill_metrics_base_url"`) {
		t.Fatalf("expected persisted skill_metrics_base_url in config: %s", string(raw))
	}
}

func TestSkillMetricsBaseURLUsesConfigNotEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AIGATE_SKILL_METRICS_URL", "https://should-not-be-used.example.com")
	cfg := loadToolingConfig(home)
	cfg.SkillMetricsBaseURL = "https://configured.example.com"
	cfg.SkillRepoRegistryURL = defaultToolingRepoRegistryURL(cfg.SkillMetricsBaseURL)
	if err := saveToolingConfig(home, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	got := skillMetricsBaseURL(home)
	if got != "https://configured.example.com" {
		t.Fatalf("skillMetricsBaseURL = %q, want https://configured.example.com", got)
	}
}

func TestParseDiscoveredSkillIDSupportsLegacyCentralFormat(t *testing.T) {
	platform, repo, branch, sourcePath, err := parseDiscoveredSkillID("github:openai/skills:collections/demo")
	if err != nil {
		t.Fatalf("parseDiscoveredSkillID legacy format err = %v", err)
	}
	if platform != "github" || repo != "openai/skills" || branch != "main" || sourcePath != "collections/demo" {
		t.Fatalf("legacy parse result = (%q,%q,%q,%q), want (github,openai/skills,main,collections/demo)", platform, repo, branch, sourcePath)
	}
}
