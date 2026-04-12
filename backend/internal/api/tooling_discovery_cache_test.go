package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSkillDiscoveryCacheRefreshDue(t *testing.T) {
	now := timeNowUTC()

	if due := skillDiscoveryCacheRefreshDue(skillDiscoveryCache{
		FetchedAt:         now.Format(time.RFC3339),
		NextAutoRefreshAt: now.Add(2 * time.Hour).Format(time.RFC3339),
	}); due {
		t.Fatal("expected cache not due when next auto refresh is in the future")
	}

	if due := skillDiscoveryCacheRefreshDue(skillDiscoveryCache{
		FetchedAt:         now.Format(time.RFC3339),
		NextAutoRefreshAt: now.Add(-1 * time.Minute).Format(time.RFC3339),
	}); !due {
		t.Fatal("expected cache due when next auto refresh is in the past")
	}

	if due := skillDiscoveryCacheRefreshDue(skillDiscoveryCache{
		FetchedAt: now.Add(-25 * time.Hour).Format(time.RFC3339),
	}); !due {
		t.Fatal("expected cache due when fetched_at is older than 24h")
	}
}

func TestToolingHandlerListDiscoveredSkillsReadsCacheOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cache := skillDiscoveryCache{
		FetchedAt:         timeNowUTC().Add(-1 * time.Hour).Format(time.RFC3339),
		NextAutoRefreshAt: timeNowUTC().Add(2 * time.Hour).Format(time.RFC3339),
		Items: []discoveredSkillRecord{
			{
				ID:         "github:openai/skills:main:skills/alpha",
				Name:       "Alpha Skill",
				Platform:   "github",
				RepoOwner:  "openai",
				RepoName:   "skills",
				Branch:     "main",
				SourcePath: "skills/alpha",
			},
			{
				ID:         "github:openai/skills:main:skills/zulu",
				Name:       "Zulu Skill",
				Platform:   "github",
				RepoOwner:  "openai",
				RepoName:   "skills",
				Branch:     "main",
				SourcePath: "skills/zulu",
			},
		},
	}
	if err := os.MkdirAll(filepath.Dir(skillDiscoveryCachePath(home)), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	if err := saveSkillDiscoveryCache(home, cache); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	handler := NewToolingHandler()
	req := httptest.NewRequest(http.MethodGet, "/tooling/skills/discover?limit=1&offset=0&q=alpha", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /tooling/skills/discover status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload toolingSkillDiscoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Cached {
		t.Fatal("expected cached=true for local discovery cache response")
	}
	if payload.Updating {
		t.Fatal("expected updating=false when cache is fresh")
	}
	if payload.IndexedTotal != 2 {
		t.Fatalf("indexed_total = %d, want 2", payload.IndexedTotal)
	}
	if payload.Total != 1 {
		t.Fatalf("total = %d, want 1 for filtered query", payload.Total)
	}
	if len(payload.Items) != 1 || payload.Items[0].Name != "Alpha Skill" {
		t.Fatalf("unexpected paged items: %#v", payload.Items)
	}
}
