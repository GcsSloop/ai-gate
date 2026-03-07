# codex-router

Codex router prototype workspace.

## Local development

Backend:

```bash
cd backend && go run ./cmd/routerd
```

Frontend:

```bash
npm --prefix frontend dev
```

The frontend dev server proxies `/accounts`, `/policy`, `/monitoring`, `/conversations`, and `/v1` to `http://127.0.0.1:8080`.
