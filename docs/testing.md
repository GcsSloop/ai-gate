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
ROUTER_URL=http://127.0.0.1:8080 \
THIRD_PARTY_BASE_URL=https://code.ppchat.vip/v1 \
THIRD_PARTY_API_KEY=sk-... \
bash scripts/test/third_party_smoke.sh
```

Do not commit real keys. Rotate any test key after use.
