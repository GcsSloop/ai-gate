# AI Gate

English | [简体中文](docs/README.zh-CN.md)

<p align="center">
  <img src="assets/aigate_1024_1024.png" alt="AI Gate icon" width="128" height="128">
</p>

<p align="center">
  <strong>A local-first Codex gateway for account switching, thin routing, and desktop control.</strong>
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white">
  <img alt="React" src="https://img.shields.io/badge/React-19-20232A?logo=react&logoColor=61DAFB">
  <img alt="Tauri" src="https://img.shields.io/badge/Tauri-2.x-24C8DB?logo=tauri&logoColor=white">
  <img alt="Mode" src="https://img.shields.io/badge/Mode-Local--Only-black">
  <img alt="API" src="https://img.shields.io/badge/API-Responses-4B32C3">
</p>

AI Gate is a local gateway and desktop shell for Codex-style workflows. It focuses on a narrow, explicit contract:

- switch between official and compatible accounts locally
- route requests to native upstream `/responses` APIs
- preserve upstream response semantics instead of re-implementing them
- provide lightweight local observability and desktop controls

This repository is intentionally **not** a cloud deployment stack and **not** a protocol emulation layer.

## Why This Project Exists

Codex users often need a stable local entry point for:

- switching between multiple authenticated accounts
- routing requests without changing client behavior
- observing usage and health locally
- packaging the experience into a desktop app for non-terminal workflows

AI Gate solves that by staying thin. It does not synthesize response state, fake retrieval endpoints, or reconstruct multi-turn semantics locally.

## Core Principles

- **Local only**: backend binds to loopback only and the desktop bundle starts the sidecar locally.
- **Thin gateway**: upstream owns `response_id`, `previous_response_id`, status codes, and SSE lifecycle.
- **No protocol cosplay**: unsupported semantics are removed instead of being faked.
- **Operational clarity**: account switching, audit data, and monitoring stay visible locally.

## Architecture

```mermaid
flowchart LR
    A["Codex CLI / Desktop Client"] --> B["AI Gate Router<br/>Go backend"]
    C["AI Gate Desktop<br/>Tauri shell"] --> B
    B --> D["Official Codex upstream<br/>chatgpt.com/backend-api/codex"]
    B --> E["Compatible upstreams<br/>native /responses only"]
    B --> F["Local SQLite<br/>audit + monitoring"]
```

## What It Does

- Routes `POST /responses` and `GET /models` through a local gateway endpoint.
- Supports official account auth flows and token refresh.
- Supports third-party providers only when they natively implement `/responses`.
- Exposes a React frontend and Tauri desktop shell for local control.
- Stores local audit and monitoring data for observability.

## What It Explicitly Does Not Do

- Fall back from `/responses` to `/chat/completions`
- Generate local `response_id`
- Rebuild `previous_response_id` chains from local history
- Emulate response retrieval endpoints
- Act as a public remote gateway or hosted SaaS deployment target

For the precise boundary, see [docs/thin-gateway-mode.md](docs/thin-gateway-mode.md).

## Quick Start

### 1. Prepare environment

```bash
cp .env.example .env
```

Edit `.env` and replace `CODEX_ROUTER_ENCRYPTION_KEY` with a real random secret before starting the backend.

Current local defaults:

```env
CODEX_ROUTER_LISTEN_ADDR=127.0.0.1:6789
CODEX_ROUTER_DATABASE_PATH=data/codex-router.sqlite
CODEX_ROUTER_SCHEDULER_INTERVAL=5m
CODEX_ROUTER_ENCRYPTION_KEY=change-this-to-a-random-32-plus-char-secret
```

### 2. Start backend

```bash
make backend
```

### 3. Start frontend

```bash
make frontend
```

The frontend dev server proxies the local API surface to `http://127.0.0.1:6789`.

### 4. Start desktop shell

```bash
npm --prefix desktop install
npm --prefix desktop run dev
```

## Use With Codex CLI

AI Gate is intended to sit behind a standard Codex client configuration while preserving upstream Responses semantics.

Recommended local config:

```toml
model_provider = "router"

[model_providers.router]
name = "router"
base_url = "http://127.0.0.1:6789/ai-router/api"
wire_api = "responses"
requires_openai_auth = true
```

Gateway contract:

- `POST /ai-router/api/v1/responses`
- `GET /ai-router/api/v1/models`

Important notes:

- Official accounts are routed to `https://chatgpt.com/backend-api/codex`.
- Third-party accounts must already support `/responses`.
- Upstream `response_id` is authoritative.
- AI Gate does not fake retrieval or continuation semantics that require local response reconstruction.

## Local Development

### Backend

```bash
make backend
```

### Frontend

```bash
make frontend
```

### Tests

```bash
make test
```

That runs:

- `cd backend && go test ./...`
- `npm --prefix frontend run test`

### Optional third-party smoke

```bash
THIRD_PARTY_BASE_URL=https://code.ppchat.vip/v1 \
THIRD_PARTY_API_KEY=sk-... \
make smoke-third-party
```

Use this only for providers that natively implement `/responses`.

## Desktop Packaging

Local macOS package flow:

```bash
npm --prefix frontend ci
npm --prefix desktop install
bash scripts/desktop/build_sidecar_macos.sh
npm --prefix desktop run tauri build -- --target universal-apple-darwin
bash scripts/desktop/notarize_macos.sh
bash scripts/desktop/collect_release_assets.sh
```

Artifacts are collected into `release-assets/`:

- `aigate-<tag>-macOS.dmg`
- `aigate-<tag>-macOS.zip`
- `SHA256SUMS`

GitLab CI supports macOS packaging on tags and can optionally sign/notarize when the required Apple credentials are present.

## Repository Layout

```text
backend/              Go router backend
frontend/             React + Vite web UI
desktop/              Tauri desktop shell
docs/                 operational notes and design docs
scripts/              packaging, migration, and smoke scripts
references/           upstream source references used for reverse engineering
```

## Documentation

- [docs/thin-gateway-mode.md](docs/thin-gateway-mode.md) - exact protocol boundary for the thin gateway
- [docs/testing.md](docs/testing.md) - backend, frontend, and Codex CLI verification flow

## Local-Only Policy

AI Gate is local-only by design:

- backend listen address is restricted to loopback (`127.0.0.1` / `localhost` / `::1`)
- desktop bundle starts the Go sidecar locally
- this repository does not ship cloud/server deployment artifacts

If you need a hosted gateway, that is a different product shape and should be designed explicitly rather than inferred from this codebase.
