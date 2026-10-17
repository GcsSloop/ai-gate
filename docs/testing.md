# Testing

## Backend

```bash
cd backend && go test ./...
```

## Frontend

```bash
npm --prefix frontend test
```

## Account model catalog

The connection test offers the account's own model list instead of a hardcoded
list. The backend serves it from the account's upstream `/models` endpoint:

```bash
curl http://127.0.0.1:<port>/ai-router/api/accounts/<id>/models
```

Official Codex accounts request `/models?client_version=<v>` so the upstream
returns models gated behind newer Codex builds (gpt-5.6 and gpt-6). AI Gate
uses `codexClientVersion` in `backend/internal/providers/codex/adapter.go` as a
released compatibility baseline. When updating `references/openai-codex` to a
release tag, review that baseline too; development `main` uses a workspace
version of `0.0.0`, which must not be advertised as the client version.
Failures answer HTTP 200 with `ok:false` and a `details` string, which lets the
UI fall back to the list cached in the browser and to the built-in suggestions.

## Codex 会话与代理切换

2026-10-17 起，AI Gate 开启代理会将用户级 `config.toml` 的默认 provider
改为 `aigate`，并自动迁移本机持久化的官方 `openai` 会话。Codex 恢复旧会话时，在没有
显式模型相关覆盖参数的情况下会恢复数据库保存的 provider；已加载会话还会
保留运行时配置。因此，迁移摘要只代表持久化记录已切换，正在运行的会话仍需重新加载。

定向验证代理配置读写与 auth 文件保留行为：

```bash
cd backend && go test ./internal/api -run 'TestSettingsHandler(Proxy|UpdatingProxyAddress|EnableWithThirdPartyProvider|EnableNormalizesDuplicateAigateProviderSections|EnableRemovesLegacyAigateDefinitions)' -count=1
```

定位混用时，对照用户级 `model_provider`、`state_5.sqlite` 中会话的
`model_provider` 和 rollout 首行的 `session_meta.model_provider`，只读取元数据，
不输出凭证或聊天正文。重启后也应分别检查新建会话和恢复旧会话；重启本身不等于
迁移。代理开关使用带 manifest 的迁移日志，关闭代理时只恢复本次切换且仍为 `aigate`
的记录；已有历史复制脚本仍用于保留源会话的独立副本。
详细源码证据与验证边界见
[2026-10-17 分析](plans/2026-10-17-codex-proxy-mixed-session-providers.md)。

## Per-account upstream proxy override

The upstream proxy configured on the settings page stays global. Each account can
override it from the account edit dialog (`走代理`):

- stored as `accounts.proxy_mode`: empty inherits the global mode, `direct` never
  proxies, `proxy` always proxies.
- `proxy` reuses the global manual proxy address. When the global mode is
  `direct` it falls back to the system/environment proxy, so one account can still
  be routed through a proxy while the rest stay direct.
- `GET /accounts` reports the effective value as `uses_upstream_proxy`, so an
  inheriting account is never misrepresented in the dialog; saving writes an
  explicit `direct` or `proxy`.
- the override covers account tests, model catalog reads, gateway traffic, and
  usage refreshes, because they all share the upstream transport.

```bash
curl http://127.0.0.1:<port>/ai-router/api/accounts | jq '.[] | {account_name, proxy_mode, uses_upstream_proxy}'
```

## Usage availability signals

The account card usage dot only reads green when a configured usage driver
recently returned something renderable. An account without a matching driver, or
one whose refresh returns no usage figures, shows a neutral dot and keeps the
usage slot blank. Hovering the dot explains which of the two cases applies.

流式 `/responses` 里的 Codex `rate_limits` 允许只携带部分窗口，也可能包含
模型专用的 `limit_id`（例如 `codex_other`）。采集器只接受主 `codex` 窗口中
处于 `0..100` 的 `used_percent`；缺失字段会沿用最近一次完整快照，避免把
临时事件误判成账号额度已用尽。相关回归测试：

```bash
cd backend && go test ./internal/api -run 'TestResponsesUsageCollector' -count=1
```

## Lua usage closed loop

Platform-specific usage adapters are user-managed Lua scripts. Select the Lua
driver, save the script under a shared key, and set the account usage config to
`{"script":"managed:<key>"}`. The backend does not silently provide scripts for
specific upstream domains.

An adapter may return `usage_display.usage_windows` entries with `label`,
`remaining_percent`, `remaining_value`, `total_value`, and `reset_label`. The
account page and server user page render those entries as progress bars. This
protocol is shared by server mode and the desktop client connected to it.

For login-style POST requests that may be rate limited, a script can opt in to
bounded retry with `retry_on_429 = true`, `retry_count`, and `retry_delay_ms`.
Ordinary POST requests are not retried by default. When a Lua script returns a
clear 429/Too Many Requests failure, the Lua driver also retries the complete
script up to three times with backoff. Lua usage configs default to a 15-second
timeout; set `timeout_ms` when an upstream requires a different budget.

For an isolated local check, run the backend on a separate loopback port with a
temporary repository-local database, then call:

```bash
curl -X POST http://127.0.0.1:<port>/ai-router/api/accounts/usage/refresh
curl http://127.0.0.1:<port>/ai-router/api/accounts/usage
```

## Server mode

Start server mode with a repo-local database and development password:

```bash
make server-dev
```

Minimal checks:

1. Open `http://127.0.0.1:6789/ai-gate/webui/`.
2. Log in with `dev-password`, unless `AI_GATE_SERVER_PASSWORD` is set.
3. Create a server user under the service users page and copy the issued token.
4. Verify requests without a token are rejected: `curl -i http://127.0.0.1:6789/ai-gate/v1/models`.
5. Verify requests with a token pass gateway authentication: `curl -i -H "Authorization: Bearer <token>" http://127.0.0.1:6789/ai-gate/v1/models`.

Build the Linux amd64 `.msvc` daemon plugin:

```bash
bash scripts/release/build_ai_gate_msvc.sh --target linux/amd64 --version 0.0.0-dev
bash scripts/test/build_ai_gate_msvc_test.sh
```

## Codex CLI smoke

Start the router backend, then point local Codex CLI to the router:

```toml
model_provider = "router"

[model_providers.router]
name = "router"
base_url = "http://127.0.0.1:6789/ai-router/api"
wire_api = "responses"
requires_openai_auth = true
```

Minimal checks:

1. `curl http://127.0.0.1:6789/ai-router/api/models`
2. Send one non-stream request to `POST /ai-router/api/responses`
3. Send one stream request to `POST /ai-router/api/responses` and verify the stream terminates with an upstream-aligned terminal event
4. Switch between two official `auth.json` accounts and verify requests do not hang or lose terminal output
5. From Codex CLI, run one short prompt and verify the router account list shows a run against the active account
6. Verify stats quality and pagination APIs:
   - `curl "http://127.0.0.1:6789/ai-router/api/dashboard/request-quality?range=24h"`
   - `curl "http://127.0.0.1:6789/ai-router/api/dashboard/recent-events?range=24h&page=1&page_size=20"`

Thin gateway notes:

- Third-party smoke tests are valid only for providers that natively implement `/responses`.
- Do not expect gateway-synthesized response retrieval endpoints to be available.
- Treat upstream `response_id` as authoritative.
