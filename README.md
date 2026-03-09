# AI Gate

`AI Gate` is a local gateway prototype focused on multi-account switching.

## Local-only policy

`AI Gate` is local-only by design:

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

Thin gateway mode is intended to mirror the official Responses API surface while staying out of orchestration and conversation semantics.

Current thin-gateway contract:

- `/ai-router/api/v1/responses`
- `/ai-router/api/v1/models`

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

- The router now runs in thin-gateway mode only.
- Official `auth.json` accounts are routed to `https://chatgpt.com/backend-api/codex`.
- Third-party accounts are supported only when they natively implement `/responses`; the router does not fall back to `/chat/completions`.
- `response_id` and `previous_response_id` semantics are owned by the upstream service, not reconstructed locally.
- Endpoints that require gateway-synthesized response state are removed instead of being faked locally.
- See [`docs/thin-gateway-mode.md`](docs/thin-gateway-mode.md) for the exact boundary.

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

- `aigate-<tag>-macOS.dmg`
- `aigate-<tag>-macOS.zip`
- `SHA256SUMS`

GitLab CI pipeline (`.gitlab-ci.yml`) uses:

- test stages for backend/frontend
- macOS package stage on tag (`v*`)
- optional codesign + notarize if the following variables are provided:
  - `APPLE_SIGNING_IDENTITY`
  - `APPLE_API_KEY_PATH`
  - `APPLE_API_KEY_ID`
  - `APPLE_API_ISSUER`
