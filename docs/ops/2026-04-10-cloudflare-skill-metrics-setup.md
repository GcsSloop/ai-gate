# Cloudflare Skill Metrics Setup Log (2026-04-10)

## Scope

Implemented and deployed a lightweight telemetry service using Cloudflare Workers + D1 + KV for skill installation metrics and ranking.

Repository path:

- `/Users/gcssloop/WorkSpace/AIGC/codex-router/services/skill-metrics-worker`

## Created Cloud Resources

### D1

- Name: `aigate-skill-metrics-db`
- Database ID: `84bcb438-4628-4b3f-8bbc-c1b0a55a53b6`
- Region: `APAC`

Creation command:

```bash
npx wrangler d1 create aigate-skill-metrics-db
```

### KV

- Binding: `SKILL_METRICS_CACHE`
- Namespace ID: `d15a84c8260f4d80bee42ebe976302c7`

Creation command:

```bash
npx wrangler kv namespace create SKILL_METRICS_CACHE
```

## Worker Configuration

Configured in:

- `/Users/gcssloop/WorkSpace/AIGC/codex-router/services/skill-metrics-worker/wrangler.jsonc`

Key settings:

- Worker name: `aigate-skill-metrics`
- D1 binding: `DB`
- KV binding: `SKILL_METRICS_CACHE`
- Cron: `23 3 * * *` (UTC)

## Database Schema Applied

Schema file:

- `/Users/gcssloop/WorkSpace/AIGC/codex-router/services/skill-metrics-worker/sql/schema.sql`

Execution command:

```bash
npx wrangler d1 execute aigate-skill-metrics-db --remote --file=services/skill-metrics-worker/sql/schema.sql --config services/skill-metrics-worker/wrangler.jsonc
```

Result:

- `Processed 5 queries`
- `Executed 5 queries`

## Deployment

Deploy command:

```bash
npx wrangler deploy --config services/skill-metrics-worker/wrangler.jsonc
```

Current endpoint:

- `https://aigate-skill-metrics.gcssloop.workers.dev`

Current version ID (latest in this setup): `3e082831-03fa-4419-b01b-558e79bbdc24`

After central tracked-repos rollout update:

- Current version ID: `3f63d98b-70ab-4cf0-8f17-fa76133f2673`

## Smoke Test Commands

Health check:

```bash
curl -sS https://aigate-skill-metrics.gcssloop.workers.dev/health
```

Ingest sample event:

```bash
curl -sS -X POST https://aigate-skill-metrics.gcssloop.workers.dev/events/install \
  -H 'Content-Type: application/json' \
  -d '{"user_hash":"u_demo_003","skill_name":"frontend-design","source_repo":"anthropics/skills","client_version":"1.2.12"}'
```

Read ranking:

```bash
curl -sS 'https://aigate-skill-metrics.gcssloop.workers.dev/rankings/skills?day=2026-04-10&limit=50'
```

## Security Notes

- No secret values are committed into the repository.
- Optional ingest token should be stored as Worker secret, not in git:

```bash
npx wrangler secret put INGEST_BEARER_TOKEN --config services/skill-metrics-worker/wrangler.jsonc
```

- Local development secret template is provided at:
  - `/Users/gcssloop/WorkSpace/AIGC/codex-router/services/skill-metrics-worker/.dev.vars.example`

## Central Tracked Repositories (Completed)

Tracked repository APIs are now available:

- `GET /tracked-repos`
- `POST /tracked-repos` (admin)
- `PUT /tracked-repos` (admin, replace all)
- `DELETE /tracked-repos` (admin)

Admin secret configured in Cloudflare Worker:

- Secret key: `TRACKED_REPOS_ADMIN_TOKEN`
- Secret value is not committed to repository.

Seed list file:

- `/Users/gcssloop/WorkSpace/AIGC/codex-router/services/skill-metrics-worker/config/tracked-repos.seed.json`

Apply seed list command:

```bash
TOKEN='<admin token>'
BASE_URL='https://aigate-skill-metrics.gcssloop.workers.dev' \
TRACKED_REPOS_ADMIN_TOKEN="$TOKEN" \
/Users/gcssloop/WorkSpace/AIGC/codex-router/services/skill-metrics-worker/scripts/tracked-repos.sh \
replace \
/Users/gcssloop/WorkSpace/AIGC/codex-router/services/skill-metrics-worker/config/tracked-repos.seed.json
```

Verification snapshot:

- `GET /tracked-repos` item count: `29`
- Current top 5 (by configured `sort_order`):
  - `obra/superpowers`
  - `anthropics/skills`
  - `shadcn/ui`
  - `browser-use/browser-use`
  - `nextlevelbuilder/ui-ux-pro-max-skill`

## AI Gate Backend Integration

Set environment variables for backend process:

```bash
export AIGATE_SKILL_REPO_REGISTRY_URL='https://aigate-skill-metrics.gcssloop.workers.dev/tracked-repos'
# Optional only when GET /tracked-repos is protected by auth:
export AIGATE_SKILL_REPO_REGISTRY_TOKEN=''
```

Behavior after configuration:

- On every skill discovery refresh, backend loads central tracked repos.
- Missing repos are merged into local repo config automatically.
- Repo order from central list respects `sort_order`.

## Admin Console & Auth Upgrade (2026-04-10)

Implemented auth-gated admin console with minimal UI:

- `/admin` (web console)
- `/admin/api/stats/users`
- `/admin/api/users`
- `/admin/api/users/:hash/skills`
- `/admin/api/tracked-repos` (CRUD)
- `/admin/api/rankings/skills`
- `/admin/api/skills/final` (仓库 skills 数量 + 最终 skills 列表，支持 `refresh=1`)
- `/admin/api/scan/start`
- `/admin/api/scan/step`
- `/admin/api/scan/status`

Public-only endpoint:

- `/rankings/skills`
- `/skills/final`

### 2026-04-10 仓库扩容与扫描验证

执行动作：

1. 基于 `https://cc-ai.cn/skills-cn/frontend/index.html` 的上游数据源（skills.sh 页面数据）抓取并去重仓库；
2. 导入 tracked repos；
3. 手动触发扫描并轮询进度。

本次结果：

- tracked repos 总数：`233`（>= 148）
- 扫描状态：`completed`
- 扫描总仓库：`233`
- 成功：`233`
- 失败：`0`
- 最近扫描完成时间：`2026-04-10T13:44:14.110Z`

接口校验：

- `/admin/api/skills/final?limit=5` 返回：
  - `repos=233`
  - `total_items=1221`
  - 包含每仓库 `star_count` 与 `skill_count`
- `/skills/final?limit=3`（公开）返回成功

Auth-required endpoints:

- `/tracked-repos` and all admin APIs.

New worker secrets required:

- `TRACKED_REPOS_ADMIN_TOKEN` (Bearer auth for API/script use)
- `ADMIN_UI_PASSWORD` (web login password)
- `ADMIN_SESSION_SECRET` (HMAC signing key for session cookie)

Install payload update:

- `POST /events/install` now accepts optional `anonymous_id`.
- If both `user_hash` and `anonymous_id` are missing, service auto-generates `anonymous_id` and returns it in response.
