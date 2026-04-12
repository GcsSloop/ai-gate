package api

import (
	"encoding/base64"
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
	enableSkillMetricsReportingForTest(t)
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
	if payload["client_app"] != "aigate-desktop" {
		t.Fatalf("client_app = %q, want aigate-desktop", payload["client_app"])
	}
	if strings.TrimSpace(payload["client_platform"]) == "" {
		t.Fatalf("client_platform empty, payload = %#v", payload)
	}
	if strings.TrimSpace(payload["client_arch"]) == "" {
		t.Fatalf("client_arch empty, payload = %#v", payload)
	}
}

func TestReportDiscoveredSkillInstallFallsBackWhenAnonymousFileCannotPersist(t *testing.T) {
	enableSkillMetricsReportingForTest(t)
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
	if payload["client_app"] != "aigate-desktop" {
		t.Fatalf("client_app = %q, want aigate-desktop", payload["client_app"])
	}
	if strings.TrimSpace(payload["client_platform"]) == "" {
		t.Fatalf("client_platform empty, payload = %#v", payload)
	}
	if strings.TrimSpace(payload["client_arch"]) == "" {
		t.Fatalf("client_arch empty, payload = %#v", payload)
	}
}

func TestReportToolingClientActiveIsThrottledWithinOneHour(t *testing.T) {
	enableSkillMetricsReportingForTest(t)
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
	enableSkillMetricsReportingForTest(t)
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
	enableSkillMetricsReportingForTest(t)
	home := t.TempDir()
	setToolingSkillMetricsBaseURL(t, home, "http://127.0.0.1:1")

	id := discoveredSkillKey("github", "openai/skills", "main", "collections/demo-skill")
	reportDiscoveredSkillInstall(home, id)

	pendingPath := filepath.Join(aigateDataRoot(home), "tooling", "skill-metrics-pending.json")
	if _, err := os.Stat(pendingPath); !os.IsNotExist(err) {
		t.Fatalf("pending queue file should not exist, stat err = %v", err)
	}
}

func TestLoadOrCreateToolingAnonymousIDMigratesLegacyPath(t *testing.T) {
	enableSkillMetricsReportingForTest(t)
	home := t.TempDir()
	legacyPath := filepath.Join(home, ".aigate", "tooling", "anonymous-client.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"anonymous_id":"aigate-legacy-fixed-id"}`), 0o600); err != nil {
		t.Fatalf("write legacy id: %v", err)
	}

	got, err := loadOrCreateToolingAnonymousID(home)
	if err != nil {
		t.Fatalf("loadOrCreateToolingAnonymousID err = %v", err)
	}
	if got != "aigate-legacy-fixed-id" {
		t.Fatalf("anonymous id = %q, want aigate-legacy-fixed-id", got)
	}
	currentPath := toolingAnonymousClientIDPath(home)
	raw, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("read migrated current id: %v", err)
	}
	if !strings.Contains(string(raw), "aigate-legacy-fixed-id") {
		t.Fatalf("migrated file = %q, want contains legacy id", string(raw))
	}
}

func TestLoadOrCreateToolingAnonymousIDStableWhenPersistFails(t *testing.T) {
	enableSkillMetricsReportingForTest(t)
	home := t.TempDir()
	blockToolingDirWithFile(t, home)

	first, err := loadOrCreateToolingAnonymousID(home)
	if err != nil {
		t.Fatalf("first loadOrCreateToolingAnonymousID err = %v", err)
	}
	second, err := loadOrCreateToolingAnonymousID(home)
	if err != nil {
		t.Fatalf("second loadOrCreateToolingAnonymousID err = %v", err)
	}
	if strings.TrimSpace(first) == "" || strings.TrimSpace(second) == "" {
		t.Fatalf("anonymous ids should not be empty: first=%q second=%q", first, second)
	}
	if first != second {
		t.Fatalf("anonymous ids should be stable when persist fails: first=%q second=%q", first, second)
	}
}

func TestReportToolingClientActiveDisabledByDefaultInGoTestProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AIGATE_ENABLE_TEST_SKILL_METRICS_REPORTING", "")

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
	if hits != 0 {
		t.Fatalf("events/install hits = %d, want 0 when reporting is disabled in test process", hits)
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

func enableSkillMetricsReportingForTest(t *testing.T) {
	t.Helper()
	t.Setenv("AIGATE_ENABLE_TEST_SKILL_METRICS_REPORTING", "1")
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

func TestNormalizeDiscoveredSourcePathSupportsRepoRootSkill(t *testing.T) {
	got := normalizeDiscoveredSourcePath("SKILL.md")
	if got != "." {
		t.Fatalf("normalizeDiscoveredSourcePath(SKILL.md) = %q, want .", got)
	}
}

func TestDiscoveredSkillKeyNormalizesSourcePath(t *testing.T) {
	key := discoveredSkillKey("github", "openai/skills", "main", "collections/demo/SKILL.md")
	if key != "github:openai/skills:main:collections/demo" {
		t.Fatalf("discoveredSkillKey = %q, want normalized skill dir key", key)
	}
}

func TestBuildRepoTreeURLSupportsRepoRootSkill(t *testing.T) {
	got := buildRepoTreeURL("github", "op7418", "Humanizer-zh", "main", ".")
	want := "https://github.com/op7418/Humanizer-zh/tree/main"
	if got != want {
		t.Fatalf("buildRepoTreeURL(.) = %q, want %q", got, want)
	}
}

func TestPathWithinDiscoveredSkillRootDotMatchesAll(t *testing.T) {
	if !pathWithinDiscoveredSkill(".", "nested/a.txt") {
		t.Fatalf("pathWithinDiscoveredSkill('.', 'nested/a.txt') = false, want true")
	}
}

func TestCollectDiscoveredSkillRootsFromTreePrefersRepoRoot(t *testing.T) {
	roots := collectDiscoveredSkillRootsFromTree([]struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}{
		{Path: "SKILL.md", Type: "blob"},
		{Path: "humanizer-zh/SKILL.md", Type: "blob"},
	})
	if len(roots) != 1 || roots[0] != "." {
		t.Fatalf("roots = %#v, want ['.']", roots)
	}
}

func TestGroupDiscoveredSkillFilesPrefersRepoRoot(t *testing.T) {
	grouped := groupDiscoveredSkillFiles(map[string]string{
		"SKILL.md":               "root",
		"README.md":              "readme",
		"humanizer-zh/SKILL.md":  "child",
		"humanizer-zh/guide.txt": "guide",
	})
	if len(grouped) != 1 {
		t.Fatalf("group count = %d, want 1", len(grouped))
	}
	rootFiles, ok := grouped["."]
	if !ok {
		t.Fatalf("missing root group in %#v", grouped)
	}
	if _, exists := rootFiles["SKILL.md"]; !exists {
		t.Fatalf("root group should include SKILL.md: %#v", rootFiles)
	}
	if _, exists := rootFiles["humanizer-zh/SKILL.md"]; !exists {
		t.Fatalf("root group should include nested files when root skill exists: %#v", rootFiles)
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

func TestInstallDiscoveredSkillCollectionFallbacksToRepoRootSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillBody := "---\nname: Humanizer ZH\n---\n# Humanizer"
	readmeBody := "repo readme"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/op7418/humanizer-zh/git/trees/main"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"tree": [
					{"path":"SKILL.md","type":"blob","sha":"sha-skill"},
					{"path":"README.md","type":"blob","sha":"sha-readme"}
				]
			}`))
			return
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/git/blobs/sha-skill"):
			raw := base64.StdEncoding.EncodeToString([]byte(skillBody))
			_, _ = w.Write([]byte(`{"encoding":"base64","content":"` + raw + `"}`))
			return
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/git/blobs/sha-readme"):
			raw := base64.StdEncoding.EncodeToString([]byte(readmeBody))
			_, _ = w.Write([]byte(`{"encoding":"base64","content":"` + raw + `"}`))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()
	t.Setenv("AIGATE_GITHUB_API_BASE", server.URL)

	applied, err := installDiscoveredSkillCollection(home, "github:op7418/humanizer-zh:main:humanizer-zh", "copy", []string{"codex"})
	if err != nil {
		t.Fatalf("installDiscoveredSkillCollection err = %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
	managedDir := filepath.Join(managedSkillsRoot(home), "humanizer-zh-humanizer-zh")
	meta := readSkillMetadata(managedDir)
	if meta.SourcePath != "." {
		t.Fatalf("meta.SourcePath = %q, want .", meta.SourcePath)
	}
	if !strings.HasSuffix(meta.SourceURL, "/tree/main") {
		t.Fatalf("meta.SourceURL = %q, want .../tree/main", meta.SourceURL)
	}
	if _, err := os.Stat(filepath.Join(managedDir, "SKILL.md")); err != nil {
		t.Fatalf("installed SKILL.md missing: %v", err)
	}
}
