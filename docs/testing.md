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

2026-10-19 修复 Windows 开启代理时的数据库路径问题。带盘符的路径必须转换为
`file:///C:/Users/.../.codex/state_5.sqlite`；`file://C:/Users/...` 会将盘符误作 URI
主机名。`modernc.org/sqlite v1.39.0` 在此打开失败场景会返回误导性的
`SQL logic error: out of memory (1)`，不能据此判断系统内存不足。
数据库连接在读取会话前主动验证，继续保留 `mode=rw`，避免意外创建 Codex 数据库。

2026-10-19 起，Codex 创建后未写入任何内容的空 rollout 文件会被跳过，不再让整批
迁移以 `read Codex session header: EOF` 失败；首行 JSON 完整但缺少结尾换行时仍按
有效 header 迁移，只有内容损坏的 header 才会中止并回滚。

定向回归覆盖盘符、中文、空格、URI 特殊字符、不创建缺失数据库，以及代理迁移和恢复：

```bash
cd backend && go test ./internal/api -run '^(TestCodexStateDatabase|TestCodexSessionProvider|TestSettingsHandlerProxyAutomaticallySwitchesAndRestoresSessionProviders|TestSettingsHandlerProxyMigrationFailureRestoresConfig)' -count=1
```

Dev CI 的 `Windows Codex Session Migration` job 在 Windows runner 上执行同一组测试，
验证原生盘符路径下的数据库读写及代理开关。UNC 路径仅验证 URI 编码，不代表已验证
网络共享上的 SQLite/WAL 可用性。

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

## 官方账户额度保护

2026-10-19 新增账号级 `quota_protection_enabled`，官方账户创建、授权、导入及旧数据库迁移后默认开启。
编辑弹窗在“走代理”后显示“额度保护”，第三方账户不显示，路由也不会对第三方应用此阈值。
`PUT /accounts/<id>` 可以传 `{"quota_protection_enabled":false}` 手动关闭；未传字段的更新保留原值。
复制、分享、导入及数据库备份保留设置，旧分享缺少字段时官方账户默认开启。

任一官方窗口的 `used_percent >= 97` 时，AI Gate 在转发前跳过该账户并尝试其他可用账户，
即使全局自动故障转移关闭也触发这次保护切换。覆盖流式及非流式 `/responses`、
`/chat/completions` 和 Responses 子路径；成功后沿用活动账户或服务端用户粘性路由的切换机制。
保护不写入持久冷却状态，窗口重置或关闭保护后即可恢复候选资格。
没有替代账户时返回 HTTP 429、`error.code=quota_protected`，流式请求也会立即结束。

判断使用上游报告的最后一次额度快照。未知额度允许请求；已经到达重置时间的旧窗口不再拦截。
保护在新请求开始前执行，已经转发的请求和并发中的请求仍可能继续消耗额度，因此不能保证实际剩余恰好为 3%。

```bash
cd backend && go test ./internal/routing ./internal/api -run 'TestQuotaProtection' -count=1
cd backend && go test ./internal/accounts ./internal/store/sqlite -run 'TestSQLiteRepositoryCreateAndListAccounts|TestOpenEnablesQuotaProtectionForExistingAccounts' -count=1
npm --prefix frontend test -- src/features/accounts/AccountsPage.test.tsx -t 'quota protection|proxy_mode='
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
