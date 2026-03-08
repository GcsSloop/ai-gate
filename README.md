# ccc-gateway

`ccc-gateway` is a local gateway prototype focused on multi-account switching.

`ccc` stands for:

- `codex`
- `cursor`
- `claude code`

## Local-only policy

`ccc-gateway` is local-only by design:

- backend listen address is restricted to loopback (`127.0.0.1` / `localhost` / `::1`)
- desktop bundle starts the Go sidecar locally
- this project does not provide cloud/server deployment artifacts

## Local development

Copy env file:

```bash
cp .env.example .env
```

Edit `.env` and replace `CODEX_ROUTER_ENCRYPTION_KEY` with a real random secret before running the backend.

Backend:

```bash
make backend
```

Frontend:

```bash
make frontend
```

Desktop (Tauri shell):

```bash
npm --prefix desktop install
npm --prefix desktop run dev
```

Tests:

```bash
make test
```

Optional low-volume third-party smoke:

```bash
THIRD_PARTY_BASE_URL=https://code.ppchat.vip/v1 \
THIRD_PARTY_API_KEY=sk-... \
make smoke-third-party
```

The frontend dev server proxies `/accounts`, `/policy`, `/monitoring`, `/conversations`, and `/v1` to `http://127.0.0.1:6789`.

## Codex CLI via Gateway

The gateway now exposes:

- `/ai-router/api/responses`
- `/ai-router/api/models`
- `/ai-router/api/chat/completions`
- `/ai-router/api/v1/responses`
- `/ai-router/api/v1/models`
- `/ai-router/api/v1/chat/completions`
- `/ai-router/api/v1/responses/{response_id}`
- `/ai-router/api/v1/responses/{response_id}/input_items`
- `/ai-router/api/v1/responses/{response_id}/cancel`
- `/ai-router/api/v1/responses/input_tokens`
- `/ai-router/api/v1/models/{model_id}`
- `DELETE /ai-router/api/v1/responses/{response_id}`

Recommended local Codex CLI config:

```toml
model_provider = "router"

[model_providers.router]
name = "router"
base_url = "http://127.0.0.1:6789/ai-router/api"
wire_api = "responses"
requires_openai_auth = true
```

Notes:

- Third-party accounts continue to use their configured OpenAI-compatible `base_url + api_key`.
- Uploaded local `auth.json` accounts are treated as official Codex sessions and routed to `https://chatgpt.com/backend-api/codex`.
- Gateway-managed `response_id` values are used to replay conversation history, so official and third-party accounts can share one conversation chain.

## Tauri package for customers (GitLab CI)

Local macOS build:

```bash
npm --prefix frontend ci
npm --prefix desktop install
bash scripts/desktop/build_sidecar_macos.sh
npm --prefix desktop run tauri build -- --target universal-apple-darwin
bash scripts/desktop/notarize_macos.sh
bash scripts/desktop/collect_release_assets.sh
```

`release-assets/` will contain:

- `ccc-gateway-<tag>-macOS.dmg`
- `ccc-gateway-<tag>-macOS.zip`
- `SHA256SUMS`

GitLab CI pipeline (`.gitlab-ci.yml`) uses:

- test stages for backend/frontend
- macOS package stage on tag (`v*`)
- optional codesign + notarize if the following variables are provided:
  - `APPLE_SIGNING_IDENTITY`
  - `APPLE_API_KEY_PATH`
  - `APPLE_API_KEY_ID`
  - `APPLE_API_ISSUER`
