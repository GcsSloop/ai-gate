# Testing

## Backend

```bash
cd backend && go test ./...
```

## Frontend

```bash
npm --prefix frontend test
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

Thin gateway notes:

- Third-party smoke tests are valid only for providers that natively implement `/responses`.
- Do not expect gateway-synthesized response retrieval endpoints to be available.
- Treat upstream `response_id` as authoritative.
