# Testing

## Backend

```bash
cd backend && go test ./...
```

## Frontend

```bash
npm --prefix frontend test
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
Ordinary POST requests are not retried by default.

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
