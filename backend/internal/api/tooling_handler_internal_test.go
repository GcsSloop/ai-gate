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
	t.Setenv("AIGATE_SKILL_METRICS_URL", server.URL)

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
	t.Setenv("AIGATE_SKILL_METRICS_URL", server.URL)

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
}

func TestReportToolingClientActiveIsThrottledWithinHalfDay(t *testing.T) {
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
	t.Setenv("AIGATE_SKILL_METRICS_URL", server.URL)

	reportToolingClientActive(home)
	reportToolingClientActive(home)

	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("events/install hits = %d, want 1 within active throttle window", hits)
	}
}

func TestReportDiscoveredSkillInstallQueuesOnNetworkFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AIGATE_SKILL_METRICS_URL", "http://127.0.0.1:1")

	id := discoveredSkillKey("github", "openai/skills", "main", "collections/demo-skill")
	reportDiscoveredSkillInstall(home, id)

	queue := loadPendingSkillMetricsQueue(home)
	if len(queue.Items) != 1 {
		t.Fatalf("pending queue items = %d, want 1", len(queue.Items))
	}
	if queue.Items[0].Kind != "install" {
		t.Fatalf("pending queue kind = %q, want install", queue.Items[0].Kind)
	}
	if queue.Items[0].RetryCount != 0 {
		t.Fatalf("pending queue retry_count = %d, want 0", queue.Items[0].RetryCount)
	}
}

func TestFlushPendingSkillMetricsDropsItemAfterMaxRetry(t *testing.T) {
	home := t.TempDir()
	queue := pendingSkillMetricsQueue{
		Items: []pendingSkillMetricsEvent{
			{
				Kind:       "install",
				Payload:    map[string]string{"anonymous_id": "aigate-test", "skill_name": "demo", "source_repo": "openai/skills"},
				RetryCount: skillMetricsMaxRetry - 1,
			},
		},
	}
	if err := savePendingSkillMetricsQueue(home, queue); err != nil {
		t.Fatalf("seed pending queue: %v", err)
	}

	flushPendingSkillMetricsEvents(home, "http://127.0.0.1:1", time.Now().UTC())

	after := loadPendingSkillMetricsQueue(home)
	if len(after.Items) != 0 {
		t.Fatalf("pending queue items = %d, want 0 after max retry", len(after.Items))
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

func TestParseDiscoveredSkillIDSupportsLegacyCentralFormat(t *testing.T) {
	platform, repo, branch, sourcePath, err := parseDiscoveredSkillID("github:openai/skills:collections/demo")
	if err != nil {
		t.Fatalf("parseDiscoveredSkillID legacy format err = %v", err)
	}
	if platform != "github" || repo != "openai/skills" || branch != "main" || sourcePath != "collections/demo" {
		t.Fatalf("legacy parse result = (%q,%q,%q,%q), want (github,openai/skills,main,collections/demo)", platform, repo, branch, sourcePath)
	}
}
