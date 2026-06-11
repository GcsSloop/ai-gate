# AI Gate Server Mode Msvc Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an isolated AI Gate server mode with embedded Web UI, authenticated control plane, token-gated multi-user gateway usage, server-oriented defaults, Linux amd64 `.msvc` packaging, and `make server-dev`.

**Architecture:** Keep the current desktop/client behavior as the default path. Add explicit server mode through config/bootstrap options, then layer management authentication and gateway token identity only when server mode is enabled. Embed the frontend build into the Go binary and package the server binary plus `daemon.json` as an `.msvc` daemon plugin with `data/` declared as persisted data.

**Tech Stack:** Go `net/http`, Go `embed`, SQLite migrations/repositories, Vite React + Ant Design, shell release harness, Makefile.

---

### PDCA 1: Server Mode Configuration And Defaults

**Plan:** Introduce explicit server mode without changing normal client defaults.

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `backend/cmd/routerd/main.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`
- Test: `backend/internal/config/config_test.go`
- Test: `backend/internal/bootstrap/bootstrap_test.go`

**Do:**
1. Add `Mode`, `ServerMode`, `GatewayPrefix`, `StaticWebRoot`, and `SkipCodexConfig` style config fields as needed.
2. Make server mode opt-in via `AI_GATE_MODE=server` and optional CLI `--server`.
3. In server mode, default listen address to `0.0.0.0:6789`, database to `./data/aigate.sqlite`, prefix to `/ai-gate`, and proxy enabled at startup.
4. Keep normal mode defaults unchanged.

**Check:**
- Run `cd backend && go test ./internal/config ./internal/bootstrap -run 'ServerMode|Default|RootRedirect|Proxy' -count=1`.
- Expected: server defaults are verified while existing local defaults still pass.

**Act/Commit:**
- Commit with `feat: 增加服务端模式启动默认值`.

### PDCA 2: Embedded Web UI And Prefix Routing

**Plan:** Serve the frontend from the Go service in server mode, while preserving the current Vite and desktop paths.

**Files:**
- Create: `backend/internal/webui/embed.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`
- Modify: `frontend/vite.config.ts`
- Test: `backend/internal/bootstrap/bootstrap_test.go`
- Test: `frontend/src/indexHtml.test.ts`

**Do:**
1. Add a small embedded static file server that serves built `frontend/dist` when embedded assets exist.
2. Mount `/ai-gate/webui/` in server mode and keep `/ai-router/webui/` for current client mode.
3. Redirect `/` to the active prefix Web UI.
4. Add Vite server build mode with base `/ai-gate/webui/` while leaving desktop base `./` unchanged.

**Check:**
- Run `npm --prefix frontend run build -- --mode server`.
- Run `cd backend && go test ./internal/bootstrap -run 'WebUI|RootRedirect|Prefix' -count=1`.

**Act/Commit:**
- Commit with `feat: 内嵌服务端网页入口`.

### PDCA 3: Control Plane Password Authentication

**Plan:** Require authorization before accessing server mode control pages and management APIs.

**Files:**
- Create: `backend/internal/serverauth/session.go`
- Create: `backend/internal/serverauth/session_test.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`
- Modify: `backend/internal/config/config.go`
- Test: `backend/internal/bootstrap/bootstrap_test.go`

**Do:**
1. Add `AI_GATE_SERVER_PASSWORD` support, requiring it in server mode unless a dev default is explicitly set by `make server-dev`.
2. Add `/ai-gate/auth/login`, `/ai-gate/auth/logout`, and `/ai-gate/auth/session`.
3. Protect `/ai-gate/webui/` and `/ai-gate/api/*` with a secure enough HTTP-only cookie session.
4. Leave gateway token routes independent from control-plane password sessions.

**Check:**
- Run `cd backend && go test ./internal/serverauth ./internal/bootstrap ./internal/config -run 'Auth|Password|ServerMode' -count=1`.

**Act/Commit:**
- Commit with `feat: 增加服务端控制台访问认证`.

### PDCA 4: Server Users, Token Issuing, And Per-User Usage

**Plan:** Add managed server users whose bearer tokens authorize gateway traffic and own usage metrics.

**Files:**
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Create: `backend/internal/serverusers/types.go`
- Create: `backend/internal/serverusers/repository.go`
- Create: `backend/internal/serverusers/repository_test.go`
- Create: `backend/internal/api/server_users_handler.go`
- Create: `backend/internal/api/server_users_handler_test.go`
- Modify: `backend/internal/usage/types.go`
- Modify: `backend/internal/usage/repository.go`
- Modify: `backend/internal/usage/repository_test.go`
- Modify: `backend/internal/api/dashboard_handler.go`

**Do:**
1. Add `server_users` table with token hashes and status.
2. Add nullable `server_user_id` to `usage_events` and rollup-safe aggregation paths.
3. Add management API for list/create/disable/rotate-token and per-user usage summary.
4. Never store raw issued tokens after creation/rotation responses.

**Check:**
- Run `cd backend && go test ./internal/serverusers ./internal/usage ./internal/api -run 'ServerUser|Token|Usage' -count=1`.

**Act/Commit:**
- Commit with `feat: 增加服务端用户令牌管理`.

### PDCA 5: Gateway Token Gate And User-Aware Load Balancing

**Plan:** Require valid user tokens for server gateway requests and spread users across upstream accounts.

**Files:**
- Modify: `backend/internal/api/gateway_handler.go`
- Modify: `backend/internal/api/responses_handler.go`
- Modify: `backend/internal/api/usage_capture.go`
- Modify: `backend/internal/routing/scoring.go`
- Test: `backend/internal/api/gateway_handler_test.go`
- Test: `backend/internal/api/responses_handler_test.go`
- Test: `backend/internal/routing/scoring_test.go`

**Do:**
1. Add middleware for `/ai-gate/v1/*` that validates bearer token or `x-ai-gate-token`.
2. Attach server user identity to request context.
3. Save gateway usage events with `server_user_id`.
4. Rotate ordered candidates by stable hash of user ID before existing routing feasibility checks.
5. Return `401` for missing/invalid tokens before any upstream attempt.

**Check:**
- Run `cd backend && go test ./internal/api ./internal/routing -run 'ServerUser|GatewayToken|LoadBalance|Responses' -count=1`.

**Act/Commit:**
- Commit with `feat: 增加服务端网关令牌校验和分流`.

### PDCA 6: Frontend Login And User Management

**Plan:** Add server-only control UI for login and user token administration without disturbing current client UI.

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/App.tsx`
- Create: `frontend/src/features/server-users/ServerUsersPage.tsx`
- Create: `frontend/src/features/server-users/ServerUsersPage.test.tsx`
- Modify: `frontend/src/lib/i18n.ts`

**Do:**
1. Detect server mode through a small backend session/settings endpoint.
2. Show login screen when `/ai-gate/auth/session` is unauthenticated.
3. Add a server-only navigation item for user management.
4. Support create user, rotate token, disable user, copy token once, and display usage totals.
5. Keep desktop/client pages and proxy switch behavior unchanged in normal mode.

**Check:**
- Run `npm --prefix frontend test -- ServerUsersPage App`.
- Run `npm --prefix frontend run build -- --mode server`.
- If the dev server is needed, open the page with Browser and verify login/user management layout at desktop and mobile widths.

**Act/Commit:**
- Commit with `feat: 增加服务端用户管理界面`.

### PDCA 7: Msvc Packaging And Server Dev Harness

**Plan:** Produce a Linux amd64 `.msvc` daemon plugin and add a local server-mode developer command.

**Files:**
- Modify: `Makefile`
- Create: `scripts/release/build_ai_gate_msvc.sh`
- Create: `scripts/test/build_ai_gate_msvc_test.sh`
- Modify: `README.md` or `docs/testing.md`

**Do:**
1. Add `make server-dev` with server mode env, repo-local data, dev password, and no Codex config mutation.
2. Add release script that builds frontend server assets and Go Linux amd64 binary.
3. Generate root `daemon.json` with `ai-gate` service naming, gateway prefix `ai-gate`, Linux amd64 target, B/S Web UI metadata, and `datas` containing `data`.
4. Package staged files as `.msvc`.

**Check:**
- Run `bash scripts/release/build_ai_gate_msvc.sh --help`.
- Run `bash scripts/release/build_ai_gate_msvc.sh --target linux/amd64 --version 0.0.0-dev`.
- Run `unzip -l dist/msvc/ai-gate_0.0.0-dev_linux_amd64.msvc | grep daemon.json`.
- Run `bash scripts/test/build_ai_gate_msvc_test.sh`.

**Act/Commit:**
- Commit with `feat: 增加 ai-gate msvc 打包脚本`.

### PDCA 8: End-To-End Closure

**Plan:** Run the narrow and broad harnesses needed to prove the feature is closed.

**Files:**
- Modify docs only if final verification exposes missing instructions.

**Do:**
1. Run backend package tests covering changed areas.
2. Run frontend tests and server build.
3. Run msvc packaging harness.
4. Start `make server-dev` and manually probe auth, web UI, and gateway token rejection/acceptance paths.

**Check:**
- Run `cd backend && go test ./...`.
- Run `npm --prefix frontend test`.
- Run `npm --prefix frontend run build -- --mode server`.
- Run `bash scripts/test/build_ai_gate_msvc_test.sh`.
- Run curl checks against `make server-dev`.

**Act/Commit:**
- Commit any final docs/test adjustments with `chore: 完成服务端模式闭环验证`.
