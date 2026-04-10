package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/api"
)

func TestToolingHandlerStateImportAndApplySkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seedSkill(t, filepath.Join(home, ".codex", "skills", "alpha-skill"), "Alpha skill")

	handler := api.NewToolingHandler()

	state := doToolingRequest(t, handler, http.MethodGet, "/tooling/state", nil, nil, http.StatusOK)
	var payload map[string]any
	if err := json.Unmarshal(state, &payload); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if got := payload["skill_sync_method"]; got != "symlink" {
		t.Fatalf("skill_sync_method = %v, want symlink", got)
	}

	imported := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/import", bytes.NewBufferString(`{"source":"codex"}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(imported), `"imported":1`) {
		t.Fatalf("import response = %s, want imported 1", string(imported))
	}

	applyBody := bytes.NewBufferString(`{"apps":["codex"],"method":"copy"}`)
	applied := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/apply", applyBody, map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(applied), `"skill_sync_method":"copy"`) {
		t.Fatalf("apply response = %s, want copy method", string(applied))
	}

	copiedSkill := filepath.Join(home, ".codex", "skills", "alpha-skill", "SKILL.md")
	if _, err := os.Stat(copiedSkill); err != nil {
		t.Fatalf("expected synced skill at %s: %v", copiedSkill, err)
	}
}

func TestToolingHandlerListsManagedSkillsEvenWhenSyncedBySymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seedSkill(t, filepath.Join(home, ".codex", "skills", "alpha-skill"), "Alpha skill")
	handler := api.NewToolingHandler()

	doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/import", bytes.NewBufferString(`{"source":"codex"}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/apply", bytes.NewBufferString(`{"apps":["codex"]}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)

	state := doToolingRequest(t, handler, http.MethodGet, "/tooling/state", nil, nil, http.StatusOK)
	var payload map[string]any
	if err := json.Unmarshal(state, &payload); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	skills := payload["installed_skills"].([]any)
	if len(skills) != 1 {
		t.Fatalf("installed_skills = %d, want 1", len(skills))
	}
	record := skills[0].(map[string]any)
	if managedPath, ok := record["managed_path"].(string); !ok || !strings.Contains(managedPath, filepath.Join("tooling", "skills")) {
		t.Fatalf("managed_path = %v, want managed tooling path", record["managed_path"])
	}
	if installedApps, ok := record["installed_apps"].(map[string]any); !ok || installedApps["codex"] != true {
		t.Fatalf("installed_apps = %v, want codex true", record["installed_apps"])
	}
}

func TestToolingHandlerImportsNamespacedCodexSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seedSkill(t, filepath.Join(home, ".codex", "skills", "superpowers", "brainstorming"), "Brainstorming skill")
	seedSkill(t, filepath.Join(home, ".codex", "skills", "superpowers", "systematic-debugging"), "Systematic debugging skill")
	handler := api.NewToolingHandler()

	imported := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/import", bytes.NewBufferString(`{"source":"codex"}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(imported), `"imported":1`) {
		t.Fatalf("import response = %s, want imported 1", string(imported))
	}

	applied := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/apply", bytes.NewBufferString(`{"apps":["codex"]}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(applied), `"applied":1`) {
		t.Fatalf("apply response = %s, want applied 1", string(applied))
	}

	importedSkill := filepath.Join(home, ".aigate", "data", "tooling", "skills", "superpowers", "brainstorming", "SKILL.md")
	if _, err := os.Stat(importedSkill); err != nil {
		t.Fatalf("expected imported managed skill at %s: %v", importedSkill, err)
	}

	syncedSkill := filepath.Join(home, ".codex", "skills", "superpowers", "brainstorming", "SKILL.md")
	if _, err := os.Stat(syncedSkill); err != nil {
		t.Fatalf("expected synced codex skill at %s: %v", syncedSkill, err)
	}

	state := doToolingRequest(t, handler, http.MethodGet, "/tooling/state", nil, nil, http.StatusOK)
	var payload map[string]any
	if err := json.Unmarshal(state, &payload); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	skills := payload["installed_skills"].([]any)
	if len(skills) != 1 {
		t.Fatalf("installed_skills = %d, want 1", len(skills))
	}
	record := skills[0].(map[string]any)
	if record["name"] != "superpowers" {
		t.Fatalf("skill name = %v, want superpowers", record["name"])
	}
	if record["managed_path"] != filepath.Join(home, ".aigate", "data", "tooling", "skills", "superpowers") {
		t.Fatalf("managed_path = %v, want top-level superpowers dir", record["managed_path"])
	}

	clients := payload["clients"].([]any)
	if len(clients) != 1 {
		t.Fatalf("clients = %d, want 1", len(clients))
	}
	client := clients[0].(map[string]any)
	if client["skills_count"] != float64(1) {
		t.Fatalf("skills_count = %v, want 1 top-level collection", client["skills_count"])
	}
}

func TestToolingHandlerIgnoresDotSystemSkillCollectionsOnImport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seedSkill(t, filepath.Join(home, ".codex", "skills", ".system", "openai-docs"), "OpenAI docs skill")
	seedSkill(t, filepath.Join(home, ".codex", "skills", "superpowers", "brainstorming"), "Brainstorming skill")
	handler := api.NewToolingHandler()

	imported := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/import", bytes.NewBufferString(`{"source":"codex"}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(imported), `"imported":1`) {
		t.Fatalf("import response = %s, want imported 1", string(imported))
	}

	if _, err := os.Stat(filepath.Join(home, ".aigate", "data", "tooling", "skills", ".system")); !os.IsNotExist(err) {
		t.Fatalf("expected .system not imported, stat err = %v", err)
	}

	state := doToolingRequest(t, handler, http.MethodGet, "/tooling/state", nil, nil, http.StatusOK)
	var payload map[string]any
	if err := json.Unmarshal(state, &payload); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	skills := payload["installed_skills"].([]any)
	if len(skills) != 1 {
		t.Fatalf("installed_skills = %d, want 1", len(skills))
	}
	record := skills[0].(map[string]any)
	if record["name"] != "superpowers" {
		t.Fatalf("skill name = %v, want superpowers", record["name"])
	}
}

func TestToolingHandlerImportSkipsExistingManagedSkillCollection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	managedDir := filepath.Join(home, ".aigate", "data", "tooling", "skills", "Humanizer-zh")
	seedSkill(t, managedDir, "Managed humanizer skill")
	seedSkill(t, filepath.Join(home, ".codex", "skills", "Humanizer-zh"), "Codex humanizer skill")
	handler := api.NewToolingHandler()

	imported := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/import", bytes.NewBufferString(`{"source":"codex"}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(imported), `"imported":0`) {
		t.Fatalf("import response = %s, want imported 0", string(imported))
	}

	raw, err := os.ReadFile(filepath.Join(managedDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile managed skill: %v", err)
	}
	if string(raw) != "Managed humanizer skill" {
		t.Fatalf("managed skill body = %q, want preserved managed content", string(raw))
	}
}

func TestToolingHandlerImportSkipsCodexSymlinkedManagedSkillCollection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	managedDir := filepath.Join(home, ".aigate", "data", "tooling", "skills", "Humanizer-zh")
	seedSkill(t, managedDir, "Managed humanizer skill")
	codexSkillsRoot := filepath.Join(home, ".codex", "skills")
	if err := os.MkdirAll(codexSkillsRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll codex skills root: %v", err)
	}
	if err := os.Symlink(managedDir, filepath.Join(codexSkillsRoot, "Humanizer-zh")); err != nil {
		t.Fatalf("Symlink managed skill into codex: %v", err)
	}
	handler := api.NewToolingHandler()

	imported := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/import", bytes.NewBufferString(`{"source":"codex"}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(imported), `"imported":0`) {
		t.Fatalf("import response = %s, want imported 0", string(imported))
	}

	raw, err := os.ReadFile(filepath.Join(managedDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile managed skill: %v", err)
	}
	if string(raw) != "Managed humanizer skill" {
		t.Fatalf("managed skill body = %q, want preserved managed content", string(raw))
	}
}

func TestToolingHandlerImportCopiesExternalSymlinkedSkillCollection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	externalDir := filepath.Join(home, "external-skills", "Humanizer-zh")
	seedSkill(t, externalDir, "External humanizer skill")
	if err := os.WriteFile(filepath.Join(externalDir, "notes.txt"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatalf("WriteFile extra file: %v", err)
	}
	codexSkillsRoot := filepath.Join(home, ".codex", "skills")
	if err := os.MkdirAll(codexSkillsRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll codex skills root: %v", err)
	}
	if err := os.Symlink(externalDir, filepath.Join(codexSkillsRoot, "Humanizer-zh")); err != nil {
		t.Fatalf("Symlink external skill into codex: %v", err)
	}
	handler := api.NewToolingHandler()

	imported := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/import", bytes.NewBufferString(`{"source":"codex"}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(imported), `"imported":1`) {
		t.Fatalf("import response = %s, want imported 1", string(imported))
	}

	managedDir := filepath.Join(home, ".aigate", "data", "tooling", "skills", "Humanizer-zh")
	info, err := os.Lstat(managedDir)
	if err != nil {
		t.Fatalf("Lstat managed dir: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("managed dir should be copied directory, got symlink")
	}
	raw, err := os.ReadFile(filepath.Join(managedDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile managed skill: %v", err)
	}
	if string(raw) != "External humanizer skill" {
		t.Fatalf("managed skill body = %q, want external content", string(raw))
	}
	extra, err := os.ReadFile(filepath.Join(managedDir, "notes.txt"))
	if err != nil {
		t.Fatalf("ReadFile copied extra file: %v", err)
	}
	if string(extra) != "keep me\n" {
		t.Fatalf("copied extra file = %q, want preserved extra file", string(extra))
	}
}

func TestToolingHandlerStateSkipsHiddenManagedSkillCollections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seedSkill(t, filepath.Join(home, ".aigate", "data", "tooling", "skills", ".system", "openai-docs"), "OpenAI docs skill")
	seedSkill(t, filepath.Join(home, ".aigate", "data", "tooling", "skills", "superpowers", "brainstorming"), "Brainstorming skill")
	handler := api.NewToolingHandler()

	state := doToolingRequest(t, handler, http.MethodGet, "/tooling/state", nil, nil, http.StatusOK)
	var payload map[string]any
	if err := json.Unmarshal(state, &payload); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	skills := payload["installed_skills"].([]any)
	if len(skills) != 1 {
		t.Fatalf("installed_skills = %d, want 1", len(skills))
	}
	record := skills[0].(map[string]any)
	if record["name"] != "superpowers" {
		t.Fatalf("skill name = %v, want superpowers", record["name"])
	}
}

func TestToolingHandlerCanToggleSingleSkillCollection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seedSkill(t, filepath.Join(home, ".codex", "skills", "superpowers", "brainstorming"), "Brainstorming skill")
	handler := api.NewToolingHandler()

	doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/import", bytes.NewBufferString(`{"source":"codex"}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)

	disableResp := doToolingRequest(t, handler, http.MethodPut, "/tooling/skills/superpowers", bytes.NewBufferString(`{"apps":["codex"],"enabled":false}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(disableResp), `"applied":0`) {
		t.Fatalf("disable response = %s, want applied 0", string(disableResp))
	}

	codexSkillDir := filepath.Join(home, ".codex", "skills", "superpowers")
	if _, err := os.Stat(codexSkillDir); !os.IsNotExist(err) {
		t.Fatalf("expected codex skill collection removed, stat err = %v", err)
	}
	managedSkillDir := filepath.Join(home, ".aigate", "data", "tooling", "skills", "superpowers")
	if _, err := os.Stat(managedSkillDir); err != nil {
		t.Fatalf("expected managed skill collection kept at %s: %v", managedSkillDir, err)
	}

	enableResp := doToolingRequest(t, handler, http.MethodPut, "/tooling/skills/superpowers", bytes.NewBufferString(`{"apps":["codex"],"enabled":true}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(enableResp), `"applied":1`) {
		t.Fatalf("enable response = %s, want applied 1", string(enableResp))
	}
	if _, err := os.Stat(codexSkillDir); err != nil {
		t.Fatalf("expected codex skill collection synced at %s: %v", codexSkillDir, err)
	}
}

func TestToolingHandlerDeletesSingleSkillCollectionEverywhere(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seedSkill(t, filepath.Join(home, ".codex", "skills", "superpowers", "brainstorming"), "Brainstorming skill")
	handler := api.NewToolingHandler()

	doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/import", bytes.NewBufferString(`{"source":"codex"}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	doToolingRequest(t, handler, http.MethodPut, "/tooling/skills/superpowers", bytes.NewBufferString(`{"apps":["codex"],"enabled":true}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)

	deleted := doToolingRequest(t, handler, http.MethodDelete, "/tooling/skills/superpowers", nil, nil, http.StatusOK)
	if !strings.Contains(string(deleted), `"deleted":true`) {
		t.Fatalf("delete response = %s, want deleted true", string(deleted))
	}

	managedSkillDir := filepath.Join(home, ".aigate", "data", "tooling", "skills", "superpowers")
	if _, err := os.Stat(managedSkillDir); !os.IsNotExist(err) {
		t.Fatalf("expected managed skill collection removed, stat err = %v", err)
	}
	codexSkillDir := filepath.Join(home, ".codex", "skills", "superpowers")
	if _, err := os.Stat(codexSkillDir); !os.IsNotExist(err) {
		t.Fatalf("expected codex skill collection removed, stat err = %v", err)
	}

	state := doToolingRequest(t, handler, http.MethodGet, "/tooling/state", nil, nil, http.StatusOK)
	var payload map[string]any
	if err := json.Unmarshal(state, &payload); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	skills := payload["installed_skills"].([]any)
	if len(skills) != 0 {
		t.Fatalf("installed_skills = %d, want 0", len(skills))
	}
}

func TestToolingHandlerResolvesGitHubRepoAndPrefersMainBranch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	githubAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/openai/codex-superpowers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"default_branch":"release"}`))
		case "/repos/openai/codex-superpowers/branches":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name":"release"},{"name":"main"},{"name":"develop"}]`))
		default:
			t.Fatalf("unexpected github path: %s", r.URL.Path)
		}
	}))
	defer githubAPI.Close()
	t.Setenv("AIGATE_GITHUB_API_BASE", githubAPI.URL)

	handler := api.NewToolingHandler()

	resp := doToolingRequest(
		t,
		handler,
		http.MethodPost,
		"/tooling/skills/repos/resolve",
		bytes.NewBufferString(`{"input":"https://github.com/openai/codex-superpowers/tree/release"}`),
		map[string]string{"Content-Type": "application/json"},
		http.StatusOK,
	)

	var payload map[string]any
	if err := json.Unmarshal(resp, &payload); err != nil {
		t.Fatalf("unmarshal resolve response: %v", err)
	}
	if got := payload["platform"]; got != "github" {
		t.Fatalf("platform = %v, want github", got)
	}
	if got := payload["owner"]; got != "openai" {
		t.Fatalf("owner = %v, want openai", got)
	}
	if got := payload["name"]; got != "codex-superpowers" {
		t.Fatalf("name = %v, want codex-superpowers", got)
	}
	if got := payload["repo_url"]; got != "https://github.com/openai/codex-superpowers" {
		t.Fatalf("repo_url = %v, want github repo url", got)
	}
	if got := payload["selected_branch"]; got != "main" {
		t.Fatalf("selected_branch = %v, want main", got)
	}
	options := payload["branch_options"].([]any)
	if len(options) != 3 {
		t.Fatalf("branch_options = %v, want 3 branches", payload["branch_options"])
	}
}

func TestToolingHandlerResolvesGitLabRepoAndFallsBackToDefaultBranch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	gitlabAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RequestURI(), "/projects/gitlab-org%2Fcodex-superpowers/repository/branches") {
			t.Fatalf("unexpected gitlab request: %s", r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"release","default":true},{"name":"feature-demo","default":false}]`))
	}))
	defer gitlabAPI.Close()
	t.Setenv("AIGATE_GITLAB_API_BASE", gitlabAPI.URL)

	handler := api.NewToolingHandler()

	resp := doToolingRequest(
		t,
		handler,
		http.MethodPost,
		"/tooling/skills/repos/resolve",
		bytes.NewBufferString(`{"input":"gitlab.com/gitlab-org/codex-superpowers.git"}`),
		map[string]string{"Content-Type": "application/json"},
		http.StatusOK,
	)

	var payload map[string]any
	if err := json.Unmarshal(resp, &payload); err != nil {
		t.Fatalf("unmarshal resolve response: %v", err)
	}
	if got := payload["platform"]; got != "gitlab" {
		t.Fatalf("platform = %v, want gitlab", got)
	}
	if got := payload["repo_url"]; got != "https://gitlab.com/gitlab-org/codex-superpowers" {
		t.Fatalf("repo_url = %v, want gitlab repo url", got)
	}
	if got := payload["selected_branch"]; got != "release" {
		t.Fatalf("selected_branch = %v, want release", got)
	}
}

func TestToolingHandlerStateIncludesDefaultSkillRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	handler := api.NewToolingHandler()
	state := doToolingRequest(t, handler, http.MethodGet, "/tooling/state", nil, nil, http.StatusOK)

	var payload map[string]any
	if err := json.Unmarshal(state, &payload); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	repos, ok := payload["skill_repos"].([]any)
	if !ok || len(repos) < 20 {
		t.Fatalf("skill_repos = %v, want default repos", payload["skill_repos"])
	}
	expected := []struct {
		owner  string
		name   string
		branch string
	}{
		{owner: "obra", name: "superpowers", branch: "main"},
		{owner: "anthropics", name: "skills", branch: "main"},
		{owner: "shadcn", name: "ui", branch: "main"},
	}
	for idx, item := range expected {
		repo := repos[idx].(map[string]any)
		if repo["platform"] != "github" || repo["owner"] != item.owner || repo["name"] != item.name || repo["branch"] != item.branch {
			t.Fatalf("default repo[%d] = %v, want github/%s/%s@%s", idx, repo, item.owner, item.name, item.branch)
		}
	}
}

func TestToolingHandlerInstallAndApplyMcpServer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	handler := api.NewToolingHandler()

	body := bytes.NewBufferString(`{
		"id":"fetch",
		"template_id":"fetch",
		"name":"Fetch Server",
		"description":"Fetch MCP"
	}`)
	installed := doToolingRequest(t, handler, http.MethodPost, "/tooling/mcp/install", body, map[string]string{"Content-Type": "application/json"}, http.StatusCreated)
	if !strings.Contains(string(installed), `"id":"fetch"`) {
		t.Fatalf("install response = %s, want id fetch", string(installed))
	}

	path := filepath.Join(home, ".codex", "config.toml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected mcp config at %s: %v", path, err)
	}

	codexConfig, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("ReadFile codex config: %v", err)
	}
	if !strings.Contains(string(codexConfig), "fetch") || !strings.Contains(string(codexConfig), "mcp_servers") {
		t.Fatalf("codex config = %s, want fetch server entry", string(codexConfig))
	}

	applied := doToolingRequest(t, handler, http.MethodPost, "/tooling/mcp/apply", bytes.NewBufferString(`{"id":"fetch","apps":["codex"]}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(applied), `"applied":true`) {
		t.Fatalf("apply response = %s, want applied true", string(applied))
	}
}

func TestToolingHandlerCanToggleMcpServerForCodex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	handler := api.NewToolingHandler()
	doToolingRequest(t, handler, http.MethodPost, "/tooling/mcp/install", bytes.NewBufferString(`{
		"id":"fetch",
		"template_id":"fetch",
		"name":"Fetch Server",
		"description":"Fetch MCP"
	}`), map[string]string{"Content-Type": "application/json"}, http.StatusCreated)

	disabled := doToolingRequest(t, handler, http.MethodPost, "/tooling/mcp/apply", bytes.NewBufferString(`{"id":"fetch","apps":["codex"],"enabled":false}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(disabled), `"applied":false`) {
		t.Fatalf("disable response = %s, want applied false", string(disabled))
	}

	codexConfig, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("ReadFile codex config: %v", err)
	}
	if strings.Contains(string(codexConfig), "[mcp_servers.fetch]") {
		t.Fatalf("codex config = %s, want fetch removed after disable", string(codexConfig))
	}

	enabled := doToolingRequest(t, handler, http.MethodPost, "/tooling/mcp/apply", bytes.NewBufferString(`{"id":"fetch","apps":["codex"],"enabled":true}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(enabled), `"applied":true`) {
		t.Fatalf("enable response = %s, want applied true", string(enabled))
	}
	codexConfig, err = os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("ReadFile codex config: %v", err)
	}
	if !strings.Contains(string(codexConfig), "[mcp_servers.fetch]") {
		t.Fatalf("codex config = %s, want fetch restored after enable", string(codexConfig))
	}
}

func TestToolingHandlerStateAutoImportsExistingCodexMcpServers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatalf("MkdirAll codex: %v", err)
	}
	managedScript := filepath.Join(home, ".codex", "mcp", "fetch", "bin", "server.js")
	if err := os.MkdirAll(filepath.Dir(managedScript), 0o755); err != nil {
		t.Fatalf("MkdirAll managed script dir: %v", err)
	}
	if err := os.WriteFile(managedScript, []byte("console.log('fetch')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile managed script: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte(`[mcp_servers.fetch]
type = "stdio"
command = "node"
args = ["`+managedScript+`"]
`), 0o600); err != nil {
		t.Fatalf("WriteFile codex config: %v", err)
	}
	handler := api.NewToolingHandler()

	state := doToolingRequest(t, handler, http.MethodGet, "/tooling/state", nil, nil, http.StatusOK)
	var payload map[string]any
	if err := json.Unmarshal(state, &payload); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	discovered := payload["discovered_mcp_servers"].([]any)
	if len(discovered) != 1 {
		t.Fatalf("discovered servers = %d, want 1", len(discovered))
	}

	servers := payload["mcp_servers"].([]any)
	if len(servers) != 1 {
		t.Fatalf("mcp_servers = %d, want 1", len(servers))
	}
	server := servers[0].(map[string]any)
	if server["id"] != "fetch" {
		t.Fatalf("server id = %v, want fetch", server["id"])
	}
	enabledApps, ok := server["enabled_apps"].(map[string]any)
	if !ok || enabledApps["codex"] != true {
		t.Fatalf("enabled_apps = %v, want codex true", server["enabled_apps"])
	}
	if deleteAllowed, ok := server["delete_allowed"].(bool); !ok || !deleteAllowed {
		t.Fatalf("delete_allowed = %v, want true", server["delete_allowed"])
	}
	deleteTargets, ok := server["delete_targets"].([]any)
	if !ok || len(deleteTargets) != 1 || deleteTargets[0] != filepath.Join(home, ".codex", "mcp", "fetch") {
		t.Fatalf("delete_targets = %v, want codex managed root", server["delete_targets"])
	}
	clients := payload["clients"].([]any)
	if len(clients) != 1 {
		t.Fatalf("clients = %d, want 1", len(clients))
	}
}

func TestToolingHandlerDeleteMcpServerKeepsCodexManagedFilesByDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	handler := api.NewToolingHandler()
	artifactRoot := filepath.Join(home, ".codex", "mcp", "fetch")
	artifactFile := filepath.Join(artifactRoot, "bin", "server.js")
	if err := os.MkdirAll(filepath.Dir(artifactFile), 0o755); err != nil {
		t.Fatalf("MkdirAll artifact dir: %v", err)
	}
	if err := os.WriteFile(artifactFile, []byte("console.log('fetch')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile artifact: %v", err)
	}
	writeToolingConfig(t, home, map[string]any{
		"skill_sync_method": "symlink",
		"skill_repos":       []any{},
		"mcp_servers": []map[string]any{
			{
				"id":           "fetch",
				"name":         "Fetch Server",
				"enabled_apps": map[string]bool{"codex": true},
				"spec": map[string]any{
					"type":    "stdio",
					"command": "node",
					"args":    []string{artifactFile},
				},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatalf("MkdirAll codex: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte(`[mcp_servers.fetch]
type = "stdio"
command = "node"
args = ["`+artifactFile+`"]
`), 0o600); err != nil {
		t.Fatalf("WriteFile codex config: %v", err)
	}

	deleted := doToolingRequest(t, handler, http.MethodDelete, "/tooling/mcp/servers/fetch", nil, nil, http.StatusOK)
	if !strings.Contains(string(deleted), `"deleted":true`) {
		t.Fatalf("delete response = %s, want deleted true", string(deleted))
	}

	if _, err := os.Stat(artifactRoot); err != nil {
		t.Fatalf("expected local artifact kept by default, stat err = %v", err)
	}
	codexConfig, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("ReadFile codex config: %v", err)
	}
	if strings.Contains(string(codexConfig), "[mcp_servers.fetch]") {
		t.Fatalf("codex config = %s, want fetch removed", string(codexConfig))
	}
	cfg := readToolingConfig(t, home)
	if len(cfg["mcp_servers"].([]any)) != 0 {
		t.Fatalf("tooling config mcp_servers = %v, want empty", cfg["mcp_servers"])
	}
}

func TestToolingHandlerDeleteMcpServerCleansCodexManagedFilesWhenRequested(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	handler := api.NewToolingHandler()
	artifactRoot := filepath.Join(home, ".codex", "mcp", "fetch")
	artifactFile := filepath.Join(artifactRoot, "bin", "server.js")
	unrelatedRoot := filepath.Join(home, ".codex", "mcp", "time")
	unrelatedFile := filepath.Join(unrelatedRoot, "bin", "server.js")
	if err := os.MkdirAll(filepath.Dir(artifactFile), 0o755); err != nil {
		t.Fatalf("MkdirAll fetch artifact dir: %v", err)
	}
	if err := os.WriteFile(artifactFile, []byte("console.log('fetch')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile fetch artifact: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(unrelatedFile), 0o755); err != nil {
		t.Fatalf("MkdirAll unrelated artifact dir: %v", err)
	}
	if err := os.WriteFile(unrelatedFile, []byte("console.log('time')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile unrelated artifact: %v", err)
	}
	writeToolingConfig(t, home, map[string]any{
		"skill_sync_method": "symlink",
		"skill_repos":       []any{},
		"mcp_servers": []map[string]any{
			{
				"id":           "fetch",
				"name":         "Fetch Server",
				"enabled_apps": map[string]bool{"codex": true},
				"spec": map[string]any{
					"type":    "stdio",
					"command": "node",
					"args":    []string{artifactFile},
				},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatalf("MkdirAll codex: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte(`[mcp_servers.fetch]
type = "stdio"
command = "node"
args = ["`+artifactFile+`"]
`), 0o600); err != nil {
		t.Fatalf("WriteFile codex config: %v", err)
	}

	deleted := doToolingRequest(t, handler, http.MethodDelete, "/tooling/mcp/servers/fetch?cleanup_local_files=1", nil, nil, http.StatusOK)
	if !strings.Contains(string(deleted), `"deleted":true`) {
		t.Fatalf("delete response = %s, want deleted true", string(deleted))
	}

	if _, err := os.Stat(artifactRoot); !os.IsNotExist(err) {
		t.Fatalf("expected managed fetch artifact removed, stat err = %v", err)
	}
	if _, err := os.Stat(unrelatedRoot); err != nil {
		t.Fatalf("expected unrelated artifact kept, stat err = %v", err)
	}
}

func TestToolingHandlerDeleteMcpServerDoesNotPromoteSharedCodexParentDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	handler := api.NewToolingHandler()
	sharedRoot := filepath.Join(home, ".codex", "mcp", "servers")
	binFile := filepath.Join(sharedRoot, "node_modules", ".bin", "mcp-server-github")
	otherFile := filepath.Join(sharedRoot, "node_modules", ".bin", "mcp-server-time")
	if err := os.MkdirAll(filepath.Dir(binFile), 0o755); err != nil {
		t.Fatalf("MkdirAll shared bin dir: %v", err)
	}
	if err := os.WriteFile(binFile, []byte("github\n"), 0o644); err != nil {
		t.Fatalf("WriteFile github bin: %v", err)
	}
	if err := os.WriteFile(otherFile, []byte("time\n"), 0o644); err != nil {
		t.Fatalf("WriteFile other bin: %v", err)
	}
	writeToolingConfig(t, home, map[string]any{
		"skill_sync_method": "symlink",
		"skill_repos":       []any{},
		"mcp_servers": []map[string]any{
			{
				"id":           "github",
				"name":         "mcp-server-github",
				"enabled_apps": map[string]bool{"codex": true},
				"spec": map[string]any{
					"type":    "stdio",
					"command": binFile,
				},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatalf("MkdirAll codex: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte(`[mcp_servers.github]
type = "stdio"
command = "`+binFile+`"
`), 0o600); err != nil {
		t.Fatalf("WriteFile codex config: %v", err)
	}

	state := doToolingRequest(t, handler, http.MethodGet, "/tooling/state", nil, nil, http.StatusOK)
	var payload map[string]any
	if err := json.Unmarshal(state, &payload); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	servers := payload["mcp_servers"].([]any)
	server := servers[0].(map[string]any)
	deleteTargets, ok := server["delete_targets"].([]any)
	if !ok || len(deleteTargets) != 1 || deleteTargets[0] != binFile {
		t.Fatalf("delete_targets = %v, want only executable file path", server["delete_targets"])
	}

	deleted := doToolingRequest(t, handler, http.MethodDelete, "/tooling/mcp/servers/github?cleanup_local_files=1", nil, nil, http.StatusOK)
	if !strings.Contains(string(deleted), `"deleted":true`) {
		t.Fatalf("delete response = %s, want deleted true", string(deleted))
	}
	if _, err := os.Stat(binFile); !os.IsNotExist(err) {
		t.Fatalf("expected github executable removed, stat err = %v", err)
	}
	if _, err := os.Stat(sharedRoot); err != nil {
		t.Fatalf("expected shared root kept, stat err = %v", err)
	}
	if _, err := os.Stat(otherFile); err != nil {
		t.Fatalf("expected sibling executable kept, stat err = %v", err)
	}
}

func TestToolingHandlerDeleteMcpServerRejectsManualServers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	handler := api.NewToolingHandler()
	writeToolingConfig(t, home, map[string]any{
		"skill_sync_method": "symlink",
		"skill_repos":       []any{},
		"mcp_servers": []map[string]any{
			{
				"id":           "fetch",
				"name":         "Fetch Server",
				"enabled_apps": map[string]bool{"codex": true},
				"spec": map[string]any{
					"type":    "stdio",
					"command": "uvx",
					"args":    []string{"mcp-server-fetch"},
				},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatalf("MkdirAll codex: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte(`[mcp_servers.fetch]
type = "stdio"
command = "uvx"
args = ["mcp-server-fetch"]
`), 0o600); err != nil {
		t.Fatalf("WriteFile codex config: %v", err)
	}

	doToolingRequest(t, handler, http.MethodDelete, "/tooling/mcp/servers/fetch?cleanup_local_files=true", nil, nil, http.StatusBadRequest)

	cfg := readToolingConfig(t, home)
	if len(cfg["mcp_servers"].([]any)) != 1 {
		t.Fatalf("tooling config mcp_servers = %v, want server preserved after rejection", cfg["mcp_servers"])
	}
	codexConfig, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("ReadFile codex config: %v", err)
	}
	if !strings.Contains(string(codexConfig), "[mcp_servers.fetch]") {
		t.Fatalf("codex config = %s, want fetch preserved after rejection", string(codexConfig))
	}
}

func TestToolingHandlerDeleteMcpServerRejectsAppProvidedServers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	handler := api.NewToolingHandler()
	writeToolingConfig(t, home, map[string]any{
		"skill_sync_method": "symlink",
		"skill_repos":       []any{},
		"mcp_servers": []map[string]any{
			{
				"id":           "fetch",
				"name":         "Fetch Server",
				"template_id":  "fetch",
				"enabled_apps": map[string]bool{"codex": true},
				"spec":         map[string]any{"type": "stdio", "command": "uvx"},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatalf("MkdirAll codex: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte(`[mcp_servers.fetch]
type = "stdio"
command = "uvx"
`), 0o600); err != nil {
		t.Fatalf("WriteFile codex config: %v", err)
	}

	doToolingRequest(t, handler, http.MethodDelete, "/tooling/mcp/servers/fetch", nil, nil, http.StatusBadRequest)

	cfg := readToolingConfig(t, home)
	if len(cfg["mcp_servers"].([]any)) != 1 {
		t.Fatalf("tooling config mcp_servers = %v, want app-provided server preserved", cfg["mcp_servers"])
	}
}

func TestToolingHandlerDiscoverSkillsUsesCacheAndRefreshesLatest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	defaultArchive := makeGitHubArchiveZip(t, "skills-main", map[string]string{
		"README.md": "# OpenAI Skills\n",
	})
	anthropicsArchive := makeGitHubArchiveZip(t, "skills-main", map[string]string{
		"README.md": "# Anthropic Skills\n",
	})
	composioArchive := makeGitHubArchiveZip(t, "awesome-claude-skills-master", map[string]string{
		"README.md": "# Awesome Claude Skills\n",
	})
	archive := makeGitHubArchiveZip(t, "codex-skills-main", map[string]string{
		"skills/zulu/SKILL.md":  "# Zulu Skill\nA trailing skill.\n\nUNIQUE-ZULU-BODY\n",
		"skills/alpha/SKILL.md": "---\ndescription: Alpha summary\n---\n# Alpha Skill\nDetailed alpha body that must not be cached.\n",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/openai/skills/archive/refs/heads/main.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(defaultArchive)
		case r.Method == http.MethodGet && r.URL.Path == "/anthropics/skills/archive/refs/heads/main.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(anthropicsArchive)
		case r.Method == http.MethodGet && r.URL.Path == "/ComposioHQ/awesome-claude-skills/archive/refs/heads/master.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(composioArchive)
		case r.Method == http.MethodGet && r.URL.Path == "/openai/codex-skills/archive/refs/heads/main.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(archive)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/archive/refs/heads/") && strings.HasSuffix(r.URL.Path, ".zip"):
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(defaultArchive)
		case strings.Contains(r.URL.Path, "/git/trees/"), strings.Contains(r.URL.Path, "/contents/"):
			http.Error(w, "legacy github api path should not be used", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("AIGATE_GITHUB_ARCHIVE_BASE", server.URL)

	writeToolingConfig(t, home, map[string]any{
		"skill_sync_method": "symlink",
		"skill_repos": []map[string]any{
			{
				"platform": "github",
				"owner":    "openai",
				"name":     "codex-skills",
				"branch":   "main",
				"enabled":  true,
			},
		},
		"mcp_servers": []any{},
	})
	writeSkillDiscoveryCache(t, home, map[string]any{
		"fetched_at": "2026-04-08T10:00:00Z",
		"items": []map[string]any{
			{
				"id":               "github:openai/codex-skills:skills/cached",
				"name":             "Cached Skill",
				"description":      "Cached summary",
				"platform":         "github",
				"repo_owner":       "openai",
				"repo_name":        "codex-skills",
				"branch":           "main",
				"repo_url":         "https://github.com/openai/codex-skills",
				"source_path":      "skills/cached",
				"source_url":       "https://github.com/openai/codex-skills/tree/main/skills/cached",
				"managed_name":     "cached-skill",
				"content_hash":     "cached-hash",
				"installed_apps":   map[string]bool{"codex": false},
				"update_available": false,
			},
		},
	})

	handler := api.NewToolingHandler()

	cached := doToolingRequest(t, handler, http.MethodGet, "/tooling/skills/discover", nil, nil, http.StatusOK)
	var cachedPayload map[string]any
	if err := json.Unmarshal(cached, &cachedPayload); err != nil {
		t.Fatalf("unmarshal cached discover payload: %v", err)
	}
	if cachedPayload["cached"] != true {
		t.Fatalf("cached flag = %v, want true", cachedPayload["cached"])
	}
	items := cachedPayload["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["name"] != "Cached Skill" {
		t.Fatalf("cached items = %v, want cached skill only", cachedPayload["items"])
	}

	refreshed := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/discover/refresh", bytes.NewBufferString(`{}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	var refreshedPayload map[string]any
	if err := json.Unmarshal(refreshed, &refreshedPayload); err != nil {
		t.Fatalf("unmarshal refreshed discover payload: %v", err)
	}
	if refreshedPayload["cached"] != false {
		t.Fatalf("cached flag after refresh = %v, want false", refreshedPayload["cached"])
	}
	refreshedItems := refreshedPayload["items"].([]any)
	if len(refreshedItems) != 2 {
		t.Fatalf("refreshed items = %d, want 2", len(refreshedItems))
	}
	if refreshedItems[0].(map[string]any)["name"] != "Alpha Skill" || refreshedItems[1].(map[string]any)["name"] != "Zulu Skill" {
		t.Fatalf("refreshed item order = %v, want alpha then zulu", refreshedPayload["items"])
	}
	if strings.TrimSpace(refreshedItems[0].(map[string]any)["content_hash"].(string)) == "" {
		t.Fatalf("refreshed item content_hash = %v, want non-empty hash", refreshedItems[0].(map[string]any)["content_hash"])
	}

	cacheRaw := readSkillDiscoveryCacheRaw(t, home)
	if !strings.Contains(cacheRaw, "Alpha Skill") {
		t.Fatalf("cache raw = %s, want discovered names cached", cacheRaw)
	}
	if strings.Contains(cacheRaw, "Detailed alpha body that must not be cached.") || strings.Contains(cacheRaw, "UNIQUE-ZULU-BODY") {
		t.Fatalf("cache raw = %s, want index-only cache without full body content", cacheRaw)
	}

	cfg := readToolingConfig(t, home)
	repos := cfg["skill_repos"].([]any)
	var target map[string]any
	for _, raw := range repos {
		repo := raw.(map[string]any)
		if repo["owner"] == "openai" && repo["name"] == "codex-skills" {
			target = repo
			break
		}
	}
	if target == nil {
		t.Fatalf("skill_repos = %v, want openai/codex-skills", cfg["skill_repos"])
	}
	if target["skill_count"] != float64(2) {
		t.Fatalf("skill_count = %v, want 2", target["skill_count"])
	}
}

func TestToolingHandlerDiscoverSkillsMarksInstalledUpdatesByHash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	defaultArchive := makeGitHubArchiveZip(t, "skills-main", map[string]string{
		"README.md": "# OpenAI Skills\n",
	})
	anthropicsArchive := makeGitHubArchiveZip(t, "skills-main", map[string]string{
		"README.md": "# Anthropic Skills\n",
	})
	composioArchive := makeGitHubArchiveZip(t, "awesome-claude-skills-master", map[string]string{
		"README.md": "# Awesome Claude Skills\n",
	})
	archive := makeGitHubArchiveZip(t, "codex-skills-main", map[string]string{
		"skills/alpha/SKILL.md":          "# Alpha Skill\nNew upstream summary.\n",
		"skills/alpha/assets/config.txt": "v2\n",
		"skills/zulu/SKILL.md":           "# Zulu Skill\nStable upstream summary.\n",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/openai/skills/archive/refs/heads/main.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(defaultArchive)
		case r.Method == http.MethodGet && r.URL.Path == "/anthropics/skills/archive/refs/heads/main.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(anthropicsArchive)
		case r.Method == http.MethodGet && r.URL.Path == "/ComposioHQ/awesome-claude-skills/archive/refs/heads/master.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(composioArchive)
		case r.Method == http.MethodGet && r.URL.Path == "/openai/codex-skills/archive/refs/heads/main.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(archive)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/archive/refs/heads/") && strings.HasSuffix(r.URL.Path, ".zip"):
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(defaultArchive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("AIGATE_GITHUB_ARCHIVE_BASE", server.URL)

	writeToolingConfig(t, home, map[string]any{
		"skill_sync_method": "symlink",
		"skill_repos": []map[string]any{
			{
				"platform": "github",
				"owner":    "openai",
				"name":     "codex-skills",
				"branch":   "main",
				"enabled":  true,
			},
		},
		"mcp_servers": []any{},
	})

	managedRoot := filepath.Join(home, ".aigate", "data", "tooling", "skills", "codex-skills-alpha")
	if err := os.MkdirAll(filepath.Join(managedRoot, "assets"), 0o755); err != nil {
		t.Fatalf("MkdirAll managed assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(managedRoot, "SKILL.md"), []byte("# Alpha Skill\nOld local summary.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile managed SKILL: %v", err)
	}
	if err := os.WriteFile(filepath.Join(managedRoot, "assets", "config.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile managed asset: %v", err)
	}
	if err := writeSkillMetadataForTest(managedRoot, map[string]any{
		"name":        "codex-skills-alpha",
		"source_repo": "openai/codex-skills",
		"source_kind": "discovered",
		"platform":    "github",
		"branch":      "main",
		"source_path": "skills/alpha",
		"source_url":  "https://github.com/openai/codex-skills/tree/main/skills/alpha",
	}); err != nil {
		t.Fatalf("writeSkillMetadata: %v", err)
	}
	codexRoot := filepath.Join(home, ".codex", "skills", "codex-skills-alpha")
	if err := copyDirForTest(managedRoot, codexRoot); err != nil {
		t.Fatalf("copyDir codex skill: %v", err)
	}

	handler := api.NewToolingHandler()
	refreshed := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/discover/refresh", bytes.NewBufferString(`{}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)

	var payload map[string]any
	if err := json.Unmarshal(refreshed, &payload); err != nil {
		t.Fatalf("unmarshal refreshed discover payload: %v", err)
	}
	items := payload["items"].([]any)
	alpha := items[0].(map[string]any)
	if alpha["name"] != "Alpha Skill" {
		t.Fatalf("first item = %v, want Alpha Skill", alpha["name"])
	}
	if alpha["installed_apps"].(map[string]any)["codex"] != true {
		t.Fatalf("installed_apps = %v, want codex true", alpha["installed_apps"])
	}
	if alpha["update_available"] != true {
		t.Fatalf("update_available = %v, want true", alpha["update_available"])
	}
	if alpha["installed_hash"] == alpha["content_hash"] {
		t.Fatalf("installed_hash = %v, content_hash = %v, want differing hashes", alpha["installed_hash"], alpha["content_hash"])
	}
}

func TestToolingHandlerSkillRepoCRUDSupportsPlatformAwareRecords(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	handler := api.NewToolingHandler()

	created := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/repos", bytes.NewBufferString(`{
		"platform":"github",
		"owner":"openai",
		"name":"codex-skills",
		"branch":"main"
	}`), map[string]string{"Content-Type": "application/json"}, http.StatusCreated)
	if !strings.Contains(string(created), `"platform":"github"`) {
		t.Fatalf("create response = %s, want github platform", string(created))
	}

	listed := doToolingRequest(t, handler, http.MethodGet, "/tooling/skills/repos", nil, nil, http.StatusOK)
	if !strings.Contains(string(listed), `"name":"codex-skills"`) {
		t.Fatalf("list response = %s, want created repo", string(listed))
	}

	updated := doToolingRequest(t, handler, http.MethodPut, "/tooling/skills/repos/github/openai/codex-skills", bytes.NewBufferString(`{
		"platform":"gitlab",
		"owner":"gitlab-org",
		"name":"codex-skills",
		"branch":"develop"
	}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(updated), `"platform":"gitlab"`) || !strings.Contains(string(updated), `"branch":"develop"`) {
		t.Fatalf("update response = %s, want gitlab/develop", string(updated))
	}

	listed = doToolingRequest(t, handler, http.MethodGet, "/tooling/skills/repos", nil, nil, http.StatusOK)
	if !strings.Contains(string(listed), `"platform":"gitlab"`) || !strings.Contains(string(listed), `"name":"skills"`) {
		t.Fatalf("list response after update = %s, want updated gitlab repo plus default skills repo", string(listed))
	}
	if strings.Contains(string(listed), `"name":"codex-skills","branch":"main"`) {
		t.Fatalf("list response after update = %s, want original github codex-skills record removed", string(listed))
	}

	removed := doToolingRequest(t, handler, http.MethodDelete, "/tooling/skills/repos/gitlab/gitlab-org/codex-skills", nil, nil, http.StatusOK)
	if !strings.Contains(string(removed), `"removed":true`) {
		t.Fatalf("delete response = %s, want removed true", string(removed))
	}

	listed = doToolingRequest(t, handler, http.MethodGet, "/tooling/skills/repos", nil, nil, http.StatusOK)
	if !strings.Contains(string(listed), `"owner":"obra","name":"superpowers"`) ||
		!strings.Contains(string(listed), `"owner":"anthropics","name":"skills"`) ||
		!strings.Contains(string(listed), `"owner":"shadcn","name":"ui"`) ||
		strings.Contains(string(listed), `"name":"codex-skills"`) {
		t.Fatalf("list response after delete = %s, want only default repos", string(listed))
	}
}

func TestToolingHandlerReorderSkillReposPersistsOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	handler := api.NewToolingHandler()

	doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/repos", bytes.NewBufferString(`{
		"platform":"github",
		"owner":"z-org",
		"name":"z-repo",
		"branch":"main"
	}`), map[string]string{"Content-Type": "application/json"}, http.StatusCreated)

	doToolingRequest(t, handler, http.MethodPut, "/tooling/skills/repos/order", bytes.NewBufferString(`{
		"items":[
			{"platform":"github","owner":"z-org","name":"z-repo"},
			{"platform":"github","owner":"obra","name":"superpowers"},
			{"platform":"github","owner":"anthropics","name":"skills"},
			{"platform":"github","owner":"shadcn","name":"ui"}
		]
	}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)

	listed := doToolingRequest(t, handler, http.MethodGet, "/tooling/skills/repos", nil, nil, http.StatusOK)
	var repos []map[string]any
	if err := json.Unmarshal(listed, &repos); err != nil {
		t.Fatalf("unmarshal repos: %v", err)
	}
	if len(repos) < 4 {
		t.Fatalf("repos = %v, want at least 4", repos)
	}
	first := repos[0]
	if first["owner"] != "z-org" || first["name"] != "z-repo" {
		t.Fatalf("first repo = %v, want z-org/z-repo", first)
	}
}

func TestToolingHandlerCanInstallDiscoveredSkillIntoManagedAndCodexDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	archive := makeGitHubArchiveZip(t, "codex-skills-main", map[string]string{
		"skills/alpha/SKILL.md":           "# Alpha Skill\nInstallable summary.\n",
		"skills/alpha/assets/config.json": "{\"ok\":true}\n",
		"skills/alpha/assets/readme.txt":  "hello\n",
		"skills/ignore-other/SKILL.md":    "# Ignore Other\nnot selected\n",
		"skills/ignore-other/extra.txt":   "ignore\n",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/openai/codex-skills/archive/refs/heads/main.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(archive)
		case strings.Contains(r.URL.Path, "/git/trees/"), strings.Contains(r.URL.Path, "/contents/"):
			http.Error(w, "legacy github api path should not be used", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("AIGATE_GITHUB_ARCHIVE_BASE", server.URL)

	handler := api.NewToolingHandler()

	installed := doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/discover/install", bytes.NewBufferString(`{
		"id":"github:openai/codex-skills:main:skills/alpha",
		"apps":["codex"]
	}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)
	if !strings.Contains(string(installed), `"applied":1`) {
		t.Fatalf("install response = %s, want applied 1", string(installed))
	}

	managedRoot := filepath.Join(home, ".aigate", "data", "tooling", "skills", "codex-skills-alpha")
	if _, err := os.Stat(filepath.Join(managedRoot, "SKILL.md")); err != nil {
		t.Fatalf("expected managed skill file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "assets", "config.json")); err != nil {
		t.Fatalf("expected managed nested file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "assets", "readme.txt")); err != nil {
		t.Fatalf("expected managed text asset: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".aigate", "data", "tooling", "skills", "codex-skills-ignore-other")); !os.IsNotExist(err) {
		t.Fatalf("expected unrelated skill not installed, stat err = %v", err)
	}
	metaRaw, err := os.ReadFile(filepath.Join(managedRoot, ".aigate-skill.json"))
	if err != nil {
		t.Fatalf("ReadFile metadata: %v", err)
	}
	var metaPayload map[string]any
	if err := json.Unmarshal(metaRaw, &metaPayload); err != nil {
		t.Fatalf("Unmarshal metadata: %v", err)
	}
	if metaPayload["platform"] != "github" || metaPayload["source_path"] != "skills/alpha" {
		t.Fatalf("metadata = %s, want discovery source metadata", string(metaRaw))
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "codex-skills-alpha", "SKILL.md")); err != nil {
		t.Fatalf("expected codex synced skill file: %v", err)
	}
}

func TestToolingHandlerInstallDiscoveredSkillUpdatesExistingManagedFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	archive := makeGitHubArchiveZip(t, "codex-skills-main", map[string]string{
		"skills/alpha/SKILL.md":          "# Alpha Skill\nUpdated summary.\n",
		"skills/alpha/assets/config.txt": "v2\n",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/openai/codex-skills/archive/refs/heads/main.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("AIGATE_GITHUB_ARCHIVE_BASE", server.URL)

	managedRoot := filepath.Join(home, ".aigate", "data", "tooling", "skills", "codex-skills-alpha")
	if err := os.MkdirAll(filepath.Join(managedRoot, "assets"), 0o755); err != nil {
		t.Fatalf("MkdirAll managed assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(managedRoot, "SKILL.md"), []byte("# Alpha Skill\nOld summary.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile managed SKILL: %v", err)
	}
	if err := os.WriteFile(filepath.Join(managedRoot, "stale.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile stale file: %v", err)
	}
	if err := writeSkillMetadataForTest(managedRoot, map[string]any{
		"name":        "codex-skills-alpha",
		"source_repo": "openai/codex-skills",
		"source_kind": "discovered",
		"platform":    "github",
		"branch":      "main",
		"source_path": "skills/alpha",
		"source_url":  "https://github.com/openai/codex-skills/tree/main/skills/alpha",
	}); err != nil {
		t.Fatalf("writeSkillMetadata: %v", err)
	}

	handler := api.NewToolingHandler()
	doToolingRequest(t, handler, http.MethodPost, "/tooling/skills/discover/install", bytes.NewBufferString(`{
		"id":"github:openai/codex-skills:main:skills/alpha",
		"apps":["codex"]
	}`), map[string]string{"Content-Type": "application/json"}, http.StatusOK)

	raw, err := os.ReadFile(filepath.Join(managedRoot, "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile updated SKILL: %v", err)
	}
	if !strings.Contains(string(raw), "Updated summary.") {
		t.Fatalf("updated SKILL = %s, want updated summary", string(raw))
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale file should be removed on update, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "codex-skills-alpha", "assets", "config.txt")); err != nil {
		t.Fatalf("expected synced updated asset in codex dir: %v", err)
	}
}

func makeGitHubArchiveZip(t *testing.T, root string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for relativePath, raw := range files {
		entry, err := zw.Create(root + "/" + strings.TrimLeft(relativePath, "/"))
		if err != nil {
			t.Fatalf("Create zip entry %s: %v", relativePath, err)
		}
		if _, err := entry.Write([]byte(raw)); err != nil {
			t.Fatalf("Write zip entry %s: %v", relativePath, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close zip writer: %v", err)
	}
	return buf.Bytes()
}

func doToolingRequest(t *testing.T, handler http.Handler, method, path string, body *bytes.Buffer, headers map[string]string, wantStatus int) []byte {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body.Bytes())
	}
	req := httptest.NewRequest(method, path, reader)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func seedSkill(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile skill: %v", err)
	}
}

func copyDirForTest(source string, target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(current string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, current)
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
		raw, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, raw, 0o644)
	})
}

func writeSkillMetadataForTest(dir string, payload map[string]any) error {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".aigate-skill.json"), append(raw, '\n'), 0o600)
}

func writeToolingConfig(t *testing.T, home string, payload map[string]any) {
	t.Helper()
	path := filepath.Join(home, ".aigate", "data", "tooling", "tooling.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll tooling config dir: %v", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal tooling config: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("WriteFile tooling config: %v", err)
	}
}

func readToolingConfig(t *testing.T, home string) map[string]any {
	t.Helper()
	path := filepath.Join(home, ".aigate", "data", "tooling", "tooling.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile tooling config: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal tooling config: %v", err)
	}
	return payload
}

func writeSkillDiscoveryCache(t *testing.T, home string, payload map[string]any) {
	t.Helper()
	path := filepath.Join(home, ".aigate", "data", "tooling", "skill-discovery-cache.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll discovery cache dir: %v", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal discovery cache: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("WriteFile discovery cache: %v", err)
	}
}

func readSkillDiscoveryCacheRaw(t *testing.T, home string) string {
	t.Helper()
	path := filepath.Join(home, ".aigate", "data", "tooling", "skill-discovery-cache.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile discovery cache: %v", err)
	}
	return string(raw)
}
