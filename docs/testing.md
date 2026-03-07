# Testing

## Backend

```bash
cd backend && go test ./...
```

## Frontend

```bash
npm --prefix frontend test
```

## Optional third-party smoke

This is intentionally low volume. It creates one OpenAI-compatible third-party account in the router and sends one tiny completion request.

```bash
ROUTER_URL=http://127.0.0.1:6789 \
THIRD_PARTY_BASE_URL=https://code.ppchat.vip/v1 \
THIRD_PARTY_API_KEY=sk-... \
bash scripts/test/third_party_smoke.sh
```

Do not put third-party account credentials in `.env`. Manage them in the Accounts page during normal use, and only pass them inline for one-off smoke runs. Rotate any test key after use.

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
3. Verify `GET /ai-router/api/v1/responses/{response_id}`, `GET /ai-router/api/v1/responses/{response_id}/input_items`, and `POST /ai-router/api/v1/responses/{response_id}/cancel`
4. Verify `POST /ai-router/api/v1/responses/input_tokens`
5. From Codex CLI, run one short prompt and verify the router account list shows runs against either a third-party account or an uploaded local `auth.json` account
