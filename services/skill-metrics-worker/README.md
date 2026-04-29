# AI Gate Skill Metrics Worker

Cloudflare Worker service for lightweight skill install telemetry and ranking.

## Features

- `POST /events/install`: ingest install events (idempotent by day + user + skill + repo, public endpoint).
- auto-generate `anonymous_id` when install payload omits `anonymous_id`.
- `GET /rankings/skills`: public daily ranking order only (no install counts / user counts).
- `GET /skills/final`: public final skill list aggregated from tracked repos.
- `GET /admin/api/rankings/skills`: full ranking details for admin (auth required).
- `GET /admin`: minimalist web admin panel (auth required).
- `GET /tracked-repos`: fetch centrally managed skill repositories list (auth required).
- `POST|PUT|DELETE /tracked-repos`: manage tracked repos (auth required).
- `GET /admin/api/stats/users`: total users and recent 1-day active users (auth required).
- `GET /admin/api/users`: user list with install counts (auth required).
- `GET /admin/api/users/:hash/skills`: per-user installed skills (auth required).
- `GET /admin/api/skills/final`: tracked-repo skill counts + final skill list (auth required, supports `?refresh=1`).
- `POST /admin/api/scan/start|step`, `GET /admin/api/scan/status`: manual scan and progress for repo-level scanning.
- `scheduled()` cron trigger: rebuilds daily ranking and writes KV snapshots.

## Resource Bindings

Configured in [`wrangler.jsonc`](./wrangler.jsonc):

- `DB`: D1 database (`aigate-skill-metrics-db`)
- `SKILL_METRICS_CACHE`: KV namespace
- cron: `23 3 * * *` (UTC)

## Local Development

```bash
cd services/skill-metrics-worker
npm run dev
```

Optional ingest bearer token (kept for compatibility, not required by current API behavior):

```bash
cp .dev.vars.example .dev.vars
# edit .dev.vars (optional)
```

Admin token for repository management:

```bash
npx wrangler secret put TRACKED_REPOS_ADMIN_TOKEN --config wrangler.jsonc
```

Admin panel password/session secrets:

```bash
npx wrangler secret put ADMIN_UI_PASSWORD --config wrangler.jsonc
npx wrangler secret put ADMIN_SESSION_SECRET --config wrangler.jsonc
```

## Deploy

```bash
cd services/skill-metrics-worker
npx wrangler deploy
```

## Tracked Repos Bootstrap

Seed payload file:

- [`config/tracked-repos.seed.json`](./config/tracked-repos.seed.json)

Bootstrap command:

```bash
cd services/skill-metrics-worker
TRACKED_REPOS_ADMIN_TOKEN='<admin token>' \
BASE_URL='https://skills.ai-gate.work' \
./scripts/tracked-repos.sh replace ./config/tracked-repos.seed.json
```

Import and dedupe repositories from skills.sh/cc-ai source pages:

```bash
cd services/skill-metrics-worker
TRACKED_REPOS_ADMIN_TOKEN='<admin token>' \
BASE_URL='https://skills.ai-gate.work' \
./scripts/import-ccai-repos.sh
```

## Initialize Schema

```bash
cd services/skill-metrics-worker
npx wrangler d1 execute aigate-skill-metrics-db --remote --file=./sql/schema.sql
```

## Endpoints

### POST `/events/install`

Request body:

```json
{
  "anonymous_id": "aigate-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "skill_name": "skill-name",
  "source_repo": "owner/repo"
}
```

### GET `/rankings/skills?day=2026-04-10&limit=50` (public)

Response:

```json
{
  "day": "2026-04-10",
  "items": [
    {
      "skill_name": "frontend-design",
      "source_repo": "anthropics/skills"
    }
  ],
  "cached": true
}
```

This endpoint is public and only exposes ranking order.

### GET `/admin/api/rankings/skills?day=2026-04-10&limit=50` (auth required)

Response includes full metrics:

```json
{
  "day": "2026-04-10",
  "items": [
    {
      "skill_name": "frontend-design",
      "source_repo": "anthropics/skills",
      "installs": 42,
      "unique_users": 31
    }
  ],
  "cached": true
}
```

### GET `/skills/final` (public)

Response:

```json
{
  "fetched_at": "2026-04-10T13:30:00.000Z",
  "repos": [
    {
      "platform": "github",
      "owner": "openai",
      "name": "skills",
      "branch": "main",
      "enabled": true,
      "sort_order": 0,
      "skill_count": 42
    }
  ],
  "items": [
    {
      "id": "github:openai/skills:main:foo/SKILL.md",
      "name": "foo",
      "platform": "github",
      "repo_owner": "openai",
      "repo_name": "skills",
      "branch": "main",
      "repo_url": "https://github.com/openai/skills",
      "source_path": "foo/SKILL.md",
      "source_url": "https://github.com/openai/skills/blob/main/foo/SKILL.md"
    }
  ]
}
```

Pagination query params:

- `limit` (default `300`, max `2000`)
- `offset` (default `0`)

### GET `/admin`

Open:

- `https://skills.ai-gate.work/admin`

Login with `ADMIN_UI_PASSWORD`.

### GET `/tracked-repos` (auth required)

Response:

```json
{
  "items": [
    {
      "platform": "github",
      "owner": "openai",
      "name": "skills",
      "branch": "main",
      "enabled": true,
      "sort_order": 0
    }
  ]
}
```

### POST `/tracked-repos` (auth required)

```json
{
  "platform": "github",
  "owner": "openai",
  "name": "skills",
  "branch": "main",
  "enabled": true,
  "sort_order": 0
}
```

### AI Gate Backend Integration

Set these env vars in AI Gate backend runtime:

- `AIGATE_SKILL_REPO_REGISTRY_URL` (example: `https://skills.ai-gate.work/tracked-repos`)
- `AIGATE_SKILL_REPO_REGISTRY_TOKEN` (required if `/tracked-repos` is protected)

When set, skill discovery refresh will pull centralized tracked repos and merge missing entries into local scanning list.
