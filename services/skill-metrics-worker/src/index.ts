type Env = {
  DB: D1Database;
  SKILL_METRICS_CACHE: KVNamespace;
  INGEST_BEARER_TOKEN?: string;
  TRACKED_REPOS_ADMIN_TOKEN?: string;
  ADMIN_UI_PASSWORD?: string;
  ADMIN_SESSION_SECRET?: string;
  GITHUB_TOKEN?: string;
  GITLAB_TOKEN?: string;
};

type InstallEventPayload = {
  anonymous_id?: string;
  skill_name: string;
  source_repo?: string;
  client_version?: string;
  client_platform?: string;
  client_arch?: string;
  client_app?: string;
};

type SkillRankItem = {
  skill_name: string;
  source_repo: string;
  installs: number;
  unique_users: number;
};

type PublicSkillRankItem = {
  skill_name: string;
  source_repo: string;
};

type TrackedRepoItem = {
  platform: "github" | "gitlab";
  owner: string;
  name: string;
  branch: string;
  enabled: boolean;
  sort_order: number;
  updated_at?: string;
};

type UserStats = {
  total_users: number;
  active_users: number;
  active_users_1d?: number;
  active_window_days: number;
  active_window_label: string;
};

type OverviewStats = UserStats & {
  active_users_7d: number;
  active_users_30d: number;
  new_users_window: number;
  repos_total: number;
  repos_enabled: number;
  skills_total: number;
  ranking_skills_total: number;
  total_install_events: number;
  ranking_day: string;
  catalog_fetched_at: string;
  catalog_version: string;
  scan_success_rate: number;
  scan_avg_duration_ms: number;
  curve_days: number;
  user_curve: UserCurvePoint[];
};

type UserCurvePoint = {
  day: string;
  total_users: number;
  daily_active_users: number;
};

type UserInstallSummary = {
  user_hash: string;
  anonymous_id: string;
  anonymous_id_prefix: string;
  client_version: string;
  client_platform: string;
  client_arch: string;
  client_app: string;
  total_events: number;
  installs: number;
  last_seen: string;
  first_seen: string;
};

type UserProfile = {
  user_hash: string;
  anonymous_id: string;
  anonymous_id_prefix: string;
  first_seen: string;
  last_seen: string;
  total_events: number;
  installs: number;
  active_days: number;
  last_client_version: string;
  last_client_platform: string;
  last_client_arch: string;
  last_client_app: string;
};

type UserSkillInstall = {
  skill_name: string;
  source_repo: string;
  installs: number;
  last_installed_at: string;
};

type CatalogRepoSummary = TrackedRepoItem & {
  skill_count: number;
  star_count: number;
  error?: string;
};

type CatalogSkillItem = {
  id: string;
  name: string;
  platform: "github" | "gitlab";
  repo_owner: string;
  repo_name: string;
  branch: string;
  repo_url: string;
  source_path: string;
  source_url: string;
  managed_name: string;
  skills_sh_url?: string;
  audits_summary?: {
    match_confidence: number;
    providers: Array<{
      provider: "agent_trust_hub" | "socket" | "snyk";
      label: string;
      status: "pass" | "warn" | "fail" | "info";
      url: string;
    }>;
  };
};

type SkillCatalog = {
  fetched_at: string;
  repos: CatalogRepoSummary[];
  items: CatalogSkillItem[];
};

type SkillsSourceEntry = {
  source: string;
  skill_id: string;
  name: string;
};

type ScanStatus = {
  running: boolean;
  started_at: string;
  finished_at?: string;
  last_scan_at?: string;
  total_repos: number;
  processed_repos: number;
  success_repos: number;
  failed_repos: number;
  current_index: number;
  current_repo?: string;
  message?: string;
  batch_size?: number;
};

type ScanWorking = {
  started_at: string;
  repos: TrackedRepoItem[];
  repo_skill_counts: Record<string, number>;
  repo_stars: Record<string, number>;
  repo_errors: Record<string, string>;
};

type ScanFailureItem = {
  repo_key: string;
  repo: string;
  error: string;
};

type ScanHistory = {
  runs: Array<{
    started_at: string;
    finished_at?: string;
    total_repos: number;
    success_repos: number;
    failed_repos: number;
    duration_ms: number;
  }>;
};

type SkillsShSyncTaskState = {
  name: "repos" | "audits";
  next_run_at: string;
  retry_count: number;
  cursor: number;
  etag?: string;
  last_modified?: string;
  last_error?: string;
  last_success_at?: string;
  total_items?: number;
};

type SkillsShSyncState = {
  repos: SkillsShSyncTaskState;
  audits: SkillsShSyncTaskState;
  robots_allowed?: boolean;
  robots_checked_at?: string;
  robots_error?: string;
  updated_at: string;
};

type SkillsShAuditEntry = {
  source: string;
  skill_id: string;
  name: string;
  slug: string;
  providers: Array<{
    provider: "agent_trust_hub" | "socket" | "snyk";
    label: string;
    status: "pass" | "warn" | "fail" | "info";
    url: string;
  }>;
};

type SkillsShAuditApiSkill = {
  source?: string;
  skillId?: string;
  name?: string;
  agentTrustHub?: {
    result?: {
      overall_risk_level?: string;
    };
  } | null;
  socket?: {
    result?: {
      alertCount?: number;
    };
  } | null;
  snyk?: {
    result?: {
      overall_risk_level?: string;
    };
  } | null;
};

type SkillsShListApiSkill = {
  source?: string;
  skillId?: string;
  name?: string;
};

const CORS_HEADERS = {
  "access-control-allow-origin": "*",
  "access-control-allow-methods": "GET,POST,PUT,DELETE,OPTIONS",
  "access-control-allow-headers": "Content-Type,Authorization",
};

const DEFAULT_LIMIT = 50;
const MAX_LIMIT = 200;
const SESSION_COOKIE_NAME = "aigate_admin_session";
const SESSION_TTL_SECONDS = 7 * 24 * 60 * 60;
const SKILL_CATALOG_CACHE_KEY = "catalog:skills:v1";
const SKILL_CATALOG_TTL_SECONDS = 12 * 60 * 60;
const SCAN_STATUS_CACHE_KEY = "scan:skills:status:v1";
const SCAN_WORKING_CACHE_KEY = "scan:skills:working:v1";
const SCAN_HISTORY_CACHE_KEY = "scan:skills:history:v1";
const SCAN_WORKING_TTL_SECONDS = 2 * 24 * 60 * 60;
const SCAN_DEFAULT_BATCH_SIZE = 12;
const SKILLS_SH_AUDITS_URL = "https://skills.sh/audits";
const SKILLS_SH_SKILLS_ALL_TIME_API = "https://skills.sh/api/skills/all-time";
const SKILLS_SOURCE_PAGES = [
  "https://skills.sh/",
  "https://skills.sh/trending",
  "https://skills.sh/hot",
  "https://skills.sh/official",
  SKILLS_SH_AUDITS_URL,
];
const SKILLS_SH_ROBOTS_URL = "https://skills.sh/robots.txt";
const EXTERNAL_STATE_CACHE_KEY = "external:skills-sh:state:v1";
const EXTERNAL_RUN_LOCK_CACHE_KEY = "external:skills-sh:run:lock:v1";
const EXTERNAL_RUN_STATUS_CACHE_KEY = "external:skills-sh:run:status:v1";
const EXTERNAL_SKILL_AUDIT_CACHE_TTL_SECONDS = 14 * 24 * 60 * 60;
const EXTERNAL_SYNC_RETRY_BACKOFF_MS = [5 * 60 * 1000, 30 * 60 * 1000, 2 * 60 * 60 * 1000];
const EXTERNAL_SYNC_INTERVAL_MS = 24 * 60 * 60 * 1000;
const EXTERNAL_SYNC_JITTER_MAX_MS = 90 * 60 * 1000;
const EXTERNAL_SYNC_PAGE_DELAY_MIN_MS = 200;
const EXTERNAL_SYNC_PAGE_DELAY_MAX_MS = 800;
const SKILLS_SH_SKILLS_MAX_PAGES = 300;
const SKILLS_SH_AUDITS_MAX_PAGES = 300;
const EXTERNAL_FETCH_TIMEOUT_MS = 20_000;
const EXTERNAL_FETCH_RETRIES = 3;

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: CORS_HEADERS });
    }
    const url = new URL(request.url);
    try {
      if (request.method === "GET" && url.pathname === "/health") {
        await assertAdmin(request, env);
        return json({ ok: true, service: "aigate-skill-metrics" });
      }
      if (request.method === "GET" && url.pathname === "/admin") {
        const authenticated = await isAdminAuthorized(request, env);
        return html(renderAdminPage(authenticated));
      }
      if (request.method === "GET" && url.pathname === "/admin/session") {
        const authenticated = await isAdminAuthorized(request, env);
        return json({ authenticated });
      }
      if (request.method === "POST" && url.pathname === "/admin/login") {
        return await loginAdmin(request, env);
      }
      if (request.method === "POST" && url.pathname === "/admin/logout") {
        return logoutAdmin();
      }
      if (request.method === "POST" && url.pathname === "/events/install") {
        const payload = (await request.json()) as InstallEventPayload;
        const result = await ingestInstallEvent(env, payload);
        return json(result, 201);
      }
      if (request.method === "GET" && url.pathname === "/rankings/skills") {
        const day = normalizeDay(url.searchParams.get("day")) ?? utcDay();
        const limit = normalizeLimit(url.searchParams.get("limit"));
        const data = await getSkillRanking(env, day, limit);
        return json({ day, items: toPublicSkillRanking(data.items), cached: data.cached });
      }
      if (request.method === "GET" && url.pathname === "/skills/final") {
        const force = url.searchParams.get("refresh") === "1";
        const limit = normalizeCatalogLimit(url.searchParams.get("limit"));
        const offset = normalizeCatalogOffset(url.searchParams.get("offset"));
        const catalog = await getSkillCatalog(env, force);
        const sliced = sliceCatalog(catalog, offset, limit);
        return json(sliced);
      }
      if (request.method === "GET" && url.pathname === "/skills/search") {
        const limit = normalizeCatalogLimit(url.searchParams.get("limit"));
        const offset = normalizeCatalogOffset(url.searchParams.get("offset"));
        const query = (url.searchParams.get("q") ?? "").trim();
        const day = normalizeDay(url.searchParams.get("day")) ?? utcDay();
        const catalog = await getSkillCatalog(env, false);
        const ranking = await getSkillRanking(env, day, 10);
        const { items, total, indexedTotal } = filterAndSliceCatalog(catalog, query, offset, limit, ranking.items.map((item) => item.skill_name));
        return json({
          cached: true,
          fetched_at: catalog.fetched_at,
          indexed_total: indexedTotal,
          total,
          offset,
          limit,
          query,
          items,
        });
      }
      if (request.method === "POST" && url.pathname === "/skills/external-sync/start") {
        const result = await triggerExternalSyncBackground(env, ctx);
        return json(result, result.started ? 202 : 200);
      }
      if (request.method === "GET" && url.pathname === "/skills/external-sync/status") {
        const status = await getExternalSyncRunStatus(env);
        return json(status ?? { running: false });
      }
      if (request.method === "GET" && url.pathname === "/tracked-repos") {
        await assertAdmin(request, env);
        const items = await listTrackedRepos(env);
        return json({ items });
      }
      if (request.method === "POST" && url.pathname === "/tracked-repos") {
        await assertAdmin(request, env);
        const payload = (await request.json()) as Partial<TrackedRepoItem>;
        const item = await upsertTrackedRepo(env, payload);
        return json(item, 201);
      }
      if (request.method === "PUT" && url.pathname === "/tracked-repos") {
        await assertAdmin(request, env);
        const payload = (await request.json()) as { items?: Partial<TrackedRepoItem>[] };
        if (!Array.isArray(payload.items)) {
          throw httpError(400, "invalid_payload");
        }
        const items = await replaceTrackedRepos(env, payload.items);
        return json({ items });
      }
      if (request.method === "DELETE" && url.pathname === "/tracked-repos") {
        await assertAdmin(request, env);
        const platform = normalizePlatform(url.searchParams.get("platform"));
        const owner = trim(url.searchParams.get("owner") ?? "", 120);
        const name = trim(url.searchParams.get("name") ?? "", 120);
        if (!platform || !owner || !name) {
          throw httpError(400, "invalid_query");
        }
        await deleteTrackedRepo(env, platform, owner, name);
        return json({ removed: true });
      }
      if (request.method === "GET" && url.pathname === "/admin/api/stats/users") {
        await assertAdmin(request, env);
        const stats = await getUserStats(env, normalizeActiveWindowDays(url.searchParams.get("window")));
        return json(stats);
      }
      if (request.method === "GET" && url.pathname === "/admin/api/stats/overview") {
        await assertAdmin(request, env);
        const windowDays = normalizeActiveWindowDays(url.searchParams.get("window"));
        const rankingDay = normalizeDay(url.searchParams.get("day")) ?? utcDay();
        const curveDays = normalizeCurveDays(url.searchParams.get("curve_days"));
        const stats = await getOverviewStats(env, windowDays, rankingDay, curveDays);
        return json(stats);
      }
      if (request.method === "GET" && url.pathname === "/admin/api/users") {
        await assertAdmin(request, env);
        const limit = normalizeUsersLimit(url.searchParams.get("limit"));
        const offset = normalizeUsersOffset(url.searchParams.get("offset"));
        const query = trim(url.searchParams.get("q") ?? "", 120);
        const sort = normalizeUserSort(url.searchParams.get("sort"));
        const items = await listUsers(env, { limit, offset, query, sort });
        const total = await countUsers(env, query);
        return json({ items, total, limit, offset, query, sort });
      }
      if (request.method === "GET" && url.pathname.startsWith("/admin/api/users/") && !url.pathname.endsWith("/skills")) {
        await assertAdmin(request, env);
        const userHash = decodeURIComponent(url.pathname.slice("/admin/api/users/".length));
        if (!userHash.trim()) {
          throw httpError(400, "invalid_user_hash");
        }
        const profile = await getUserProfile(env, userHash.trim());
        if (!profile) {
          throw httpError(404, "user_not_found");
        }
        return json(profile);
      }
      if (request.method === "GET" && url.pathname.startsWith("/admin/api/users/") && url.pathname.endsWith("/skills")) {
        await assertAdmin(request, env);
        const userHash = decodeURIComponent(url.pathname.slice("/admin/api/users/".length, -"/skills".length));
        if (!userHash.trim()) {
          throw httpError(400, "invalid_user_hash");
        }
        const limit = normalizeUsersLimit(url.searchParams.get("limit"));
        const offset = normalizeUsersOffset(url.searchParams.get("offset"));
        const query = trim(url.searchParams.get("q") ?? "", 120);
        const items = await listSkillsForUser(env, userHash.trim(), { limit, offset, query });
        return json({ user_hash: userHash.trim(), items, limit, offset, query });
      }
      if (request.method === "GET" && url.pathname === "/admin/api/rankings/skills") {
        await assertAdmin(request, env);
        const day = normalizeDay(url.searchParams.get("day")) ?? utcDay();
        const limit = normalizeLimit(url.searchParams.get("limit"));
        const data = await getSkillRanking(env, day, limit);
        return json({ day, items: data.items, cached: data.cached });
      }
      if (request.method === "GET" && url.pathname === "/admin/api/skills/final") {
        await assertAdmin(request, env);
        const force = url.searchParams.get("refresh") === "1";
        const limit = normalizeCatalogLimit(url.searchParams.get("limit"));
        const offset = normalizeCatalogOffset(url.searchParams.get("offset"));
        const query = trim(url.searchParams.get("q") ?? "", 160);
        const catalog = await getSkillCatalog(env, force);
        if (query) {
          const filtered = filterAndSliceCatalog(catalog, query, offset, limit, []);
          return json({
            fetched_at: catalog.fetched_at,
            repos: catalog.repos,
            items: filtered.items,
            total_items: filtered.total,
            indexed_total: filtered.indexedTotal,
            offset,
            limit,
            query,
          });
        }
        return json(sliceCatalog(catalog, offset, limit));
      }
      if (request.method === "GET" && url.pathname === "/admin/api/scan/status") {
        await assertAdmin(request, env);
        const status = await getScanStatus(env);
        return json({ ...status, failures: await listScanFailures(env) });
      }
      if (request.method === "POST" && url.pathname === "/admin/api/scan/start") {
        await assertAdmin(request, env);
        const status = await startCatalogScan(env);
        return json(status);
      }
      if (request.method === "POST" && url.pathname === "/admin/api/scan/stop") {
        await assertAdmin(request, env);
        const status = await stopCatalogScan(env);
        return json(status);
      }
      if (request.method === "POST" && url.pathname === "/admin/api/scan/step") {
        await assertAdmin(request, env);
        const batchSize = normalizeScanBatchSize(url.searchParams.get("batch"));
        const status = await stepCatalogScan(env, batchSize);
        return json(status);
      }
      if (request.method === "POST" && url.pathname === "/admin/api/scan/retry-failures") {
        await assertAdmin(request, env);
        const status = await retryFailedCatalogScan(env);
        return json(status);
      }
      if (request.method === "POST" && url.pathname === "/admin/api/external-sync/run") {
        await assertAdmin(request, env);
        const result = await runExternalSyncNow(env);
        return json(result);
      }
      if (request.method === "GET" && url.pathname === "/admin/api/tracked-repos") {
        await assertAdmin(request, env);
        const items = await listTrackedRepos(env);
        return json({ items });
      }
      if (request.method === "POST" && url.pathname === "/admin/api/tracked-repos") {
        await assertAdmin(request, env);
        const payload = (await request.json()) as Partial<TrackedRepoItem>;
        const item = await upsertTrackedRepo(env, payload);
        return json(item, 201);
      }
      if (request.method === "PUT" && url.pathname === "/admin/api/tracked-repos") {
        await assertAdmin(request, env);
        const payload = (await request.json()) as { items?: Partial<TrackedRepoItem>[] };
        if (!Array.isArray(payload.items)) {
          throw httpError(400, "invalid_payload");
        }
        const items = await replaceTrackedRepos(env, payload.items);
        return json({ items });
      }
      if (request.method === "DELETE" && url.pathname.startsWith("/admin/api/tracked-repos/")) {
        await assertAdmin(request, env);
        const key = decodeURIComponent(url.pathname.slice("/admin/api/tracked-repos/".length));
        const [platformRaw, repoRaw] = key.split(":", 2);
        const platform = normalizePlatform(platformRaw);
        const [owner = "", name = ""] = repoRaw?.split("/", 2) ?? [];
        if (!platform || !owner || !name) {
          throw httpError(400, "invalid_repo_key");
        }
        await deleteTrackedRepo(env, platform, owner, name);
        return json({ removed: true });
      }
      return json({ error: "not_found" }, 404);
    } catch (err) {
      const message = err instanceof Error ? err.message : "unknown_error";
      const code = (err as { status?: number })?.status ?? 500;
      return json({ error: message }, code);
    }
  },

  async scheduled(_controller: ScheduledController, env: Env): Promise<void> {
    const day = utcDay();
    await rebuildDailyRanking(env, day, 200);
    try {
      await runSkillsShEnhancementSync(env);
      await getSkillCatalog(env, true);
    } catch {
      // keep scheduled task resilient; ranking rebuild should not fail on catalog refresh
    }
  },
};

async function ingestInstallEvent(
  env: Env,
  payload: InstallEventPayload,
): Promise<{ inserted: boolean; event_day: string; event_key: string; user_hash: string; anonymous_id?: string }> {
  const skillName = trim(payload.skill_name ?? "", 120);
  const sourceRepo = trim(payload.source_repo ?? "", 240);
  const clientVersion = trim(payload.client_version ?? "", 80);
  const clientPlatform = trim(payload.client_platform ?? "", 40);
  const clientArch = trim(payload.client_arch ?? "", 40);
  const clientApp = trim(payload.client_app ?? "", 80);
  if (!skillName) {
    throw httpError(400, "invalid_payload");
  }

  let anonymousId = trim(payload.anonymous_id ?? "", 120);
  if (!anonymousId) {
    anonymousId = `anon_${crypto.randomUUID()}`;
  }
  const userHash = await sha256Hex(anonymousId);

  const eventDay = utcDay();
  const eventKey = await sha256Hex(`${eventDay}:${userHash}:${skillName}:${sourceRepo}`);
  const now = new Date().toISOString();
  const result = await env.DB
    .prepare(
      `INSERT OR IGNORE INTO install_events (
        event_key, event_day, user_hash, anonymous_id, skill_name, source_repo, client_version, client_platform, client_arch, client_app, created_at
      ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11)`,
    )
    .bind(eventKey, eventDay, userHash, anonymousId, skillName, sourceRepo, clientVersion, clientPlatform, clientArch, clientApp, now)
    .run();

  await env.SKILL_METRICS_CACHE.delete(cacheKey(eventDay, DEFAULT_LIMIT));
  await env.SKILL_METRICS_CACHE.delete(cacheKey(eventDay, MAX_LIMIT));

  const response: { inserted: boolean; event_day: string; event_key: string; user_hash: string; anonymous_id?: string } = {
    inserted: Number(result.meta.changes ?? 0) > 0,
    event_day: eventDay,
    event_key: eventKey,
    user_hash: userHash,
  };
  if (anonymousId) {
    response.anonymous_id = anonymousId;
  }
  return response;
}

async function getSkillRanking(env: Env, day: string, limit: number): Promise<{ items: SkillRankItem[]; cached: boolean }> {
  const cacheEnabled = limit === DEFAULT_LIMIT || limit === MAX_LIMIT;
  if (!cacheEnabled) {
    const rows = await selectDailyRanking(env, day, limit);
    return { items: rows, cached: false };
  }
  const key = cacheKey(day, limit);
  const cachedRaw = await env.SKILL_METRICS_CACHE.get(key);
  if (cachedRaw) {
    return { items: JSON.parse(cachedRaw) as SkillRankItem[], cached: true };
  }
  const rows = await selectDailyRanking(env, day, limit);
  const ttl = rows.length > 0 ? 24 * 60 * 60 : 60;
  await env.SKILL_METRICS_CACHE.put(key, JSON.stringify(rows), { expirationTtl: ttl });
  return { items: rows, cached: false };
}

function toPublicSkillRanking(items: SkillRankItem[]): PublicSkillRankItem[] {
  return items.map((item) => ({
    skill_name: item.skill_name,
    source_repo: item.source_repo,
  }));
}

async function rebuildDailyRanking(env: Env, day: string, maxRows: number): Promise<void> {
  const rows = await selectDailyRanking(env, day, maxRows);
  await env.DB.prepare("DELETE FROM daily_skill_rank WHERE event_day = ?1").bind(day).run();
  for (const row of rows) {
    await env.DB
      .prepare(
        `INSERT INTO daily_skill_rank (event_day, skill_name, source_repo, installs, unique_users, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6)`,
      )
      .bind(day, row.skill_name, row.source_repo, row.installs, row.unique_users, new Date().toISOString())
      .run();
  }
  await env.SKILL_METRICS_CACHE.put(cacheKey(day, DEFAULT_LIMIT), JSON.stringify(rows.slice(0, DEFAULT_LIMIT)), { expirationTtl: 24 * 60 * 60 });
  await env.SKILL_METRICS_CACHE.put(cacheKey(day, MAX_LIMIT), JSON.stringify(rows.slice(0, MAX_LIMIT)), { expirationTtl: 24 * 60 * 60 });
}

async function selectDailyRanking(env: Env, day: string, limit: number): Promise<SkillRankItem[]> {
  const query = await env.DB
    .prepare(
      `SELECT
         skill_name,
         COALESCE(source_repo, '') AS source_repo,
         COUNT(1) AS installs,
         COUNT(DISTINCT user_hash) AS unique_users
       FROM install_events
       WHERE event_day = ?1
         AND skill_name != '__client_active__'
       GROUP BY skill_name, source_repo
       ORDER BY installs DESC, unique_users DESC, skill_name ASC
       LIMIT ?2`,
    )
    .bind(day, limit)
    .all<SkillRankItem>();
  return query.results ?? [];
}

async function getUserStats(env: Env, windowDays: number): Promise<UserStats> {
  const normalizedWindowDays = normalizeActiveWindowDays(String(windowDays));
  const totalRes = await env.DB
    .prepare("SELECT COUNT(DISTINCT user_hash) AS total_users FROM install_events")
    .first<{ total_users: number }>();
  const since = new Date(Date.now() - normalizedWindowDays * 24 * 60 * 60 * 1000).toISOString();
  const activeRes = await env.DB
    .prepare("SELECT COUNT(DISTINCT user_hash) AS active_users FROM install_events WHERE created_at >= ?1")
    .bind(since)
    .first<{ active_users: number }>();
  return {
    total_users: Number(totalRes?.total_users ?? 0),
    active_users: Number(activeRes?.active_users ?? 0),
    active_users_1d: normalizedWindowDays === 1 ? Number(activeRes?.active_users ?? 0) : undefined,
    active_window_days: normalizedWindowDays,
    active_window_label: activeWindowLabel(normalizedWindowDays),
  };
}

async function getOverviewStats(env: Env, windowDays: number, rankingDay: string, curveDays: number): Promise<OverviewStats> {
  const userStats = await getUserStats(env, windowDays);
  const active7d = await getUserStats(env, 7);
  const active30d = await getUserStats(env, 30);
  const userCurve = await getUserCurve(env, curveDays);
  const repos = await listTrackedRepos(env);
  const catalog = await getSkillCatalog(env, false);
  const history = await getScanHistory(env);
  const durationRows = history.runs.filter((item) => item.duration_ms > 0);
  const avgDuration = durationRows.length
    ? Math.round(durationRows.reduce((acc, item) => acc + item.duration_ms, 0) / durationRows.length)
    : 0;
  const totalRuns = history.runs.length;
  const successRuns = history.runs.filter((item) => item.failed_repos === 0).length;
  const rankingTotalRes = await env.DB
    .prepare(
      `SELECT COUNT(DISTINCT skill_name) AS ranking_skills_total
       FROM install_events
       WHERE event_day = ?1
         AND skill_name != '__client_active__'`,
    )
    .bind(rankingDay)
    .first<{ ranking_skills_total: number }>();
  const installsRes = await env.DB
    .prepare("SELECT COUNT(1) AS total_install_events FROM install_events WHERE skill_name != '__client_active__'")
    .first<{ total_install_events: number }>();
  const newUsersSince = new Date(Date.now() - userStats.active_window_days * 24 * 60 * 60 * 1000).toISOString();
  const newUsersRes = await env.DB
    .prepare(
      `SELECT COUNT(1) AS new_users_window
       FROM (
         SELECT user_hash, MIN(created_at) AS first_seen
         FROM install_events
         GROUP BY user_hash
       ) t
       WHERE first_seen >= ?1`,
    )
    .bind(newUsersSince)
    .first<{ new_users_window: number }>();
  const reposEnabled = repos.reduce((acc, repo) => acc + (repo.enabled ? 1 : 0), 0);
  const catalogVersion = await sha256Hex(JSON.stringify([catalog.fetched_at, catalog.items.length, catalog.repos.length]));
  return {
    ...userStats,
    active_users_7d: active7d.active_users,
    active_users_30d: active30d.active_users,
    new_users_window: Number(newUsersRes?.new_users_window ?? 0),
    repos_total: repos.length,
    repos_enabled: reposEnabled,
    skills_total: catalog.items.length,
    ranking_skills_total: Number(rankingTotalRes?.ranking_skills_total ?? 0),
    total_install_events: Number(installsRes?.total_install_events ?? 0),
    ranking_day: rankingDay,
    catalog_fetched_at: catalog.fetched_at,
    catalog_version: catalogVersion.slice(0, 12),
    scan_success_rate: totalRuns > 0 ? Number(((successRuns / totalRuns) * 100).toFixed(2)) : 0,
    scan_avg_duration_ms: avgDuration,
    curve_days: curveDays,
    user_curve: userCurve,
  };
}

async function getUserCurve(env: Env, curveDays: number): Promise<UserCurvePoint[]> {
  const days = normalizeCurveDays(String(curveDays));
  const today = utcDay();
  const startDay = utcDayOffset(-(days - 1));
  const startAt = `${startDay}T00:00:00.000Z`;

  const dailyActiveRows = await env.DB
    .prepare(
      `SELECT
         substr(created_at, 1, 10) AS day,
         COUNT(DISTINCT user_hash) AS daily_active_users
       FROM install_events
       WHERE created_at >= ?1
       GROUP BY day
       ORDER BY day ASC`,
    )
    .bind(startAt)
    .all<{ day: string; daily_active_users: number }>();
  const dailyActiveMap = new Map<string, number>();
  for (const row of dailyActiveRows.results ?? []) {
    const day = normalizeDay(row.day);
    if (!day) continue;
    dailyActiveMap.set(day, Number(row.daily_active_users ?? 0));
  }

  const baselineRow = await env.DB
    .prepare(
      `SELECT COUNT(1) AS baseline_total
       FROM (
         SELECT user_hash, MIN(created_at) AS first_seen
         FROM install_events
         GROUP BY user_hash
       ) u
       WHERE first_seen < ?1`,
    )
    .bind(startAt)
    .first<{ baseline_total: number }>();
  let runningTotal = Number(baselineRow?.baseline_total ?? 0);

  const newUserRows = await env.DB
    .prepare(
      `SELECT
         substr(first_seen, 1, 10) AS day,
         COUNT(1) AS new_users
       FROM (
         SELECT user_hash, MIN(created_at) AS first_seen
         FROM install_events
         GROUP BY user_hash
       ) u
       WHERE first_seen >= ?1
       GROUP BY day
       ORDER BY day ASC`,
    )
    .bind(startAt)
    .all<{ day: string; new_users: number }>();
  const newUsersMap = new Map<string, number>();
  for (const row of newUserRows.results ?? []) {
    const day = normalizeDay(row.day);
    if (!day) continue;
    newUsersMap.set(day, Number(row.new_users ?? 0));
  }

  const points: UserCurvePoint[] = [];
  for (let i = 0; i < days; i += 1) {
    const day = utcDayOffset(-(days - 1 - i));
    runningTotal += Number(newUsersMap.get(day) ?? 0);
    points.push({
      day,
      total_users: runningTotal,
      daily_active_users: Number(dailyActiveMap.get(day) ?? 0),
    });
  }
  if (points.length === 0) {
    points.push({ day: today, total_users: 0, daily_active_users: 0 });
  }
  return points;
}

async function listUsers(
  env: Env,
  options: { limit: number; offset: number; query: string; sort: "last_seen_desc" | "installs_desc" },
): Promise<UserInstallSummary[]> {
  const where = options.query ? "WHERE user_hash LIKE ?3 OR anonymous_id LIKE ?3" : "";
  const orderBy = options.sort === "installs_desc" ? "installs DESC, last_seen DESC" : "last_seen DESC, installs DESC";
  const stmt = env.DB.prepare(`
    WITH user_base AS (
      SELECT
        user_hash,
        COUNT(1) AS total_events,
        SUM(CASE WHEN skill_name != '__client_active__' THEN 1 ELSE 0 END) AS installs,
        MIN(created_at) AS first_seen,
        MAX(created_at) AS last_seen
      FROM install_events
      ${where}
      GROUP BY user_hash
    )
    SELECT
      b.user_hash,
      COALESCE((SELECT ie.anonymous_id FROM install_events ie WHERE ie.user_hash = b.user_hash ORDER BY ie.created_at DESC LIMIT 1), '') AS anonymous_id,
      COALESCE((SELECT ie.client_version FROM install_events ie WHERE ie.user_hash = b.user_hash ORDER BY ie.created_at DESC LIMIT 1), '') AS client_version,
      COALESCE((SELECT ie.client_platform FROM install_events ie WHERE ie.user_hash = b.user_hash ORDER BY ie.created_at DESC LIMIT 1), '') AS client_platform,
      COALESCE((SELECT ie.client_arch FROM install_events ie WHERE ie.user_hash = b.user_hash ORDER BY ie.created_at DESC LIMIT 1), '') AS client_arch,
      COALESCE((SELECT ie.client_app FROM install_events ie WHERE ie.user_hash = b.user_hash ORDER BY ie.created_at DESC LIMIT 1), '') AS client_app,
      b.total_events,
      b.installs,
      b.first_seen,
      b.last_seen
    FROM user_base b
    ORDER BY ${orderBy}
    LIMIT ?1 OFFSET ?2`);
  const query = options.query
    ? await stmt.bind(options.limit, options.offset, `%${options.query}%`).all<UserInstallSummary>()
    : await stmt.bind(options.limit, options.offset).all<UserInstallSummary>();
  return (query.results ?? []).map((item) => ({
    ...item,
    anonymous_id_prefix: summarizeIdentifier(item.anonymous_id),
  }));
}

async function countUsers(env: Env, query: string): Promise<number> {
  if (query) {
    const row = await env.DB
      .prepare("SELECT COUNT(DISTINCT user_hash) AS total FROM install_events WHERE user_hash LIKE ?1 OR anonymous_id LIKE ?1")
      .bind(`%${query}%`)
      .first<{ total: number }>();
    return Number(row?.total ?? 0);
  }
  const row = await env.DB.prepare("SELECT COUNT(DISTINCT user_hash) AS total FROM install_events").first<{ total: number }>();
  return Number(row?.total ?? 0);
}

async function getUserProfile(env: Env, userHash: string): Promise<UserProfile | null> {
  const row = await env.DB
    .prepare(
      `SELECT
         user_hash,
         MIN(created_at) AS first_seen,
         MAX(created_at) AS last_seen,
         COUNT(1) AS total_events,
         SUM(CASE WHEN skill_name != '__client_active__' THEN 1 ELSE 0 END) AS installs,
         COUNT(DISTINCT event_day) AS active_days,
         COALESCE((SELECT ie.anonymous_id FROM install_events ie WHERE ie.user_hash = install_events.user_hash ORDER BY ie.created_at DESC LIMIT 1), '') AS anonymous_id,
         COALESCE((SELECT ie.client_version FROM install_events ie WHERE ie.user_hash = install_events.user_hash ORDER BY ie.created_at DESC LIMIT 1), '') AS last_client_version,
         COALESCE((SELECT ie.client_platform FROM install_events ie WHERE ie.user_hash = install_events.user_hash ORDER BY ie.created_at DESC LIMIT 1), '') AS last_client_platform,
         COALESCE((SELECT ie.client_arch FROM install_events ie WHERE ie.user_hash = install_events.user_hash ORDER BY ie.created_at DESC LIMIT 1), '') AS last_client_arch,
         COALESCE((SELECT ie.client_app FROM install_events ie WHERE ie.user_hash = install_events.user_hash ORDER BY ie.created_at DESC LIMIT 1), '') AS last_client_app
       FROM install_events
       WHERE user_hash = ?1
       GROUP BY user_hash`,
    )
    .bind(userHash)
    .first<UserProfile>();
  if (!row) {
    return null;
  }
  return {
    ...row,
    anonymous_id_prefix: summarizeIdentifier(row.anonymous_id),
  };
}

function summarizeIdentifier(value: string): string {
  const normalized = (value || "").trim();
  if (!normalized) {
    return "-";
  }
  if (normalized.length <= 12) {
    return normalized;
  }
  return `${normalized.slice(0, 8)}...${normalized.slice(-4)}`;
}

async function listSkillsForUser(
  env: Env,
  userHash: string,
  options: { limit: number; offset: number; query: string },
): Promise<UserSkillInstall[]> {
  if (options.query) {
    const query = await env.DB
      .prepare(
        `SELECT skill_name, COALESCE(source_repo, '') AS source_repo, COUNT(1) AS installs, MAX(created_at) AS last_installed_at
         FROM install_events
         WHERE user_hash = ?1
           AND skill_name != '__client_active__'
           AND skill_name LIKE ?2
         GROUP BY skill_name, source_repo
         ORDER BY last_installed_at DESC, installs DESC, skill_name ASC`,
      )
      .bind(userHash, `%${options.query}%`)
      .all<UserSkillInstall>();
    return (query.results ?? []).slice(options.offset, options.offset + options.limit);
  }
  const query = await env.DB
    .prepare(
      `SELECT skill_name, COALESCE(source_repo, '') AS source_repo, COUNT(1) AS installs, MAX(created_at) AS last_installed_at
       FROM install_events
       WHERE user_hash = ?1
         AND skill_name != '__client_active__'
       GROUP BY skill_name, source_repo
       ORDER BY last_installed_at DESC, installs DESC, skill_name ASC`,
    )
    .bind(userHash)
    .all<UserSkillInstall>();
  return (query.results ?? []).slice(options.offset, options.offset + options.limit);
}

async function listTrackedRepos(env: Env): Promise<TrackedRepoItem[]> {
  const result = await env.DB
    .prepare(
      `SELECT platform, owner, name, branch, enabled, sort_order, updated_at
       FROM tracked_repos
       ORDER BY sort_order ASC, owner ASC, name ASC`,
    )
    .all<{
      platform: string;
      owner: string;
      name: string;
      branch: string;
      enabled: number;
      sort_order: number;
      updated_at: string;
    }>();
  return (result.results ?? []).map((row) => ({
    platform: normalizePlatform(row.platform) ?? "github",
    owner: row.owner,
    name: row.name,
    branch: row.branch || "main",
    enabled: row.enabled !== 0,
    sort_order: Number(row.sort_order) || 0,
    updated_at: row.updated_at,
  }));
}

async function upsertTrackedRepo(env: Env, raw: Partial<TrackedRepoItem> & Record<string, unknown>): Promise<TrackedRepoItem> {
  const fallbackOrder = Number.isFinite(raw.sort_order) ? Math.max(0, Math.floor(raw.sort_order as number)) : await nextTrackedRepoSortOrder(env);
  const item = normalizeTrackedRepo(raw, fallbackOrder);
  if (!hasExplicitBranch(raw)) {
    item.branch = await resolveDefaultBranch(env, item);
  }
  const now = new Date().toISOString();
  await env.DB
    .prepare(
      `INSERT INTO tracked_repos (repo_key, platform, owner, name, branch, enabled, sort_order, updated_at)
       VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
       ON CONFLICT(repo_key) DO UPDATE SET
         branch = excluded.branch,
         enabled = excluded.enabled,
         sort_order = excluded.sort_order,
         updated_at = excluded.updated_at`,
    )
    .bind(repoKey(item.platform, item.owner, item.name), item.platform, item.owner, item.name, item.branch, item.enabled ? 1 : 0, item.sort_order, now)
    .run();
  return { ...item, updated_at: now };
}

async function nextTrackedRepoSortOrder(env: Env): Promise<number> {
  const row = await env.DB.prepare("SELECT COALESCE(MAX(sort_order), -1) AS max_sort FROM tracked_repos").first<{ max_sort?: number }>();
  const maxSort = Number(row?.max_sort ?? -1);
  if (!Number.isFinite(maxSort)) return 0;
  return Math.max(0, Math.floor(maxSort) + 1);
}

function hasExplicitBranch(raw: Partial<TrackedRepoItem> & Record<string, unknown>): boolean {
  if (trim(raw.branch ?? "", 120)) return true;
  const candidates = [
    typeof raw.repo === "string" ? raw.repo : "",
    typeof raw.repo_url === "string" ? raw.repo_url : "",
    typeof raw.url === "string" ? raw.url : "",
    typeof raw.full_name === "string" ? raw.full_name : "",
  ];
  return candidates.some((candidate) => {
    const parsed = parseRepoReference(candidate);
    return !!parsed?.branch;
  });
}

async function resolveDefaultBranch(env: Env, item: TrackedRepoItem): Promise<string> {
  if (item.platform === "gitlab") {
    const branch = await fetchGitLabDefaultBranch(item.owner, item.name, env.GITLAB_TOKEN);
    return branch || item.branch || "main";
  }
  const branch = await fetchGitHubDefaultBranch(item.owner, item.name, env.GITHUB_TOKEN);
  return branch || item.branch || "main";
}

async function replaceTrackedRepos(env: Env, raws: Partial<TrackedRepoItem>[]): Promise<TrackedRepoItem[]> {
  const normalized = raws.map((raw, idx) => normalizeTrackedRepo(raw, idx));
  const now = new Date().toISOString();
  await env.DB.prepare("DELETE FROM tracked_repos").run();
  for (const item of normalized) {
    await env.DB
      .prepare(
        `INSERT INTO tracked_repos (repo_key, platform, owner, name, branch, enabled, sort_order, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)`,
      )
      .bind(repoKey(item.platform, item.owner, item.name), item.platform, item.owner, item.name, item.branch, item.enabled ? 1 : 0, item.sort_order, now)
      .run();
  }
  return normalized.map((item) => ({ ...item, updated_at: now }));
}

async function deleteTrackedRepo(env: Env, platform: "github" | "gitlab", owner: string, name: string): Promise<void> {
  await env.DB.prepare("DELETE FROM tracked_repos WHERE repo_key = ?1").bind(repoKey(platform, owner, name)).run();
}

async function getSkillCatalog(env: Env, forceRefresh: boolean): Promise<SkillCatalog> {
  if (!forceRefresh) {
    const raw = await env.SKILL_METRICS_CACHE.get(SKILL_CATALOG_CACHE_KEY);
    if (raw) {
      return JSON.parse(raw) as SkillCatalog;
    }
  }
  if (forceRefresh) {
    try {
      await runSkillsShEnhancementSync(env, { force: true });
    } catch {
      // keep catalog rebuild resilient if external enhancement sync fails
    }
  }
  const repos = await listTrackedRepos(env);
  const repoStars = await readCachedRepoStars(env);
  const catalog = await buildCatalogFromSkillsSource(env, repos, undefined, repoStars, undefined);
  await env.SKILL_METRICS_CACHE.put(SKILL_CATALOG_CACHE_KEY, JSON.stringify(catalog), { expirationTtl: SKILL_CATALOG_TTL_SECONDS });
  return catalog;
}

async function readCachedRepoStars(env: Env): Promise<Record<string, number>> {
  const raw = await env.SKILL_METRICS_CACHE.get(SKILL_CATALOG_CACHE_KEY);
  if (!raw) {
    return {};
  }
  try {
    const catalog = JSON.parse(raw) as SkillCatalog;
    const stars: Record<string, number> = {};
    for (const repo of catalog.repos ?? []) {
      const key = repoKey(repo.platform, repo.owner, repo.name);
      stars[key] = Number(repo.star_count ?? 0);
    }
    return stars;
  } catch {
    return {};
  }
}

async function getScanStatus(env: Env): Promise<ScanStatus> {
  const raw = await env.SKILL_METRICS_CACHE.get(SCAN_STATUS_CACHE_KEY);
  if (!raw) {
    return {
      running: false,
      started_at: "",
      total_repos: 0,
      processed_repos: 0,
      success_repos: 0,
      failed_repos: 0,
      current_index: 0,
      message: "not_started",
    };
  }
  return JSON.parse(raw) as ScanStatus;
}

async function getScanWorking(env: Env): Promise<ScanWorking | null> {
  const raw = await env.SKILL_METRICS_CACHE.get(SCAN_WORKING_CACHE_KEY);
  if (!raw) return null;
  return JSON.parse(raw) as ScanWorking;
}

async function listScanFailures(env: Env): Promise<ScanFailureItem[]> {
  const working = await getScanWorking(env);
  if (working) {
    const failures = Object.entries(working.repo_errors).map(([key, error]) => ({
      repo_key: key,
      repo: key.split(":")[1] ?? key,
      error,
    }));
    failures.sort((a, b) => a.repo.localeCompare(b.repo, "en", { sensitivity: "base" }));
    return failures;
  }
  const catalogRaw = await env.SKILL_METRICS_CACHE.get(SKILL_CATALOG_CACHE_KEY);
  if (!catalogRaw) return [];
  const catalog = JSON.parse(catalogRaw) as SkillCatalog;
  const failures = (catalog.repos ?? [])
    .filter((item) => item.error)
    .map((item) => ({
      repo_key: repoKey(item.platform, item.owner, item.name),
      repo: `${item.owner}/${item.name}`,
      error: item.error ?? "",
    }));
  failures.sort((a, b) => a.repo.localeCompare(b.repo, "en", { sensitivity: "base" }));
  return failures;
}

async function getScanHistory(env: Env): Promise<ScanHistory> {
  const raw = await env.SKILL_METRICS_CACHE.get(SCAN_HISTORY_CACHE_KEY);
  if (!raw) return { runs: [] };
  try {
    const parsed = JSON.parse(raw) as ScanHistory;
    if (!Array.isArray(parsed.runs)) return { runs: [] };
    return { runs: parsed.runs.slice(0, 20) };
  } catch {
    return { runs: [] };
  }
}

async function appendScanHistory(env: Env, status: ScanStatus): Promise<void> {
  if (!status.started_at) return;
  const finishedAt = status.finished_at || new Date().toISOString();
  const durationMs = Math.max(0, Date.parse(finishedAt) - Date.parse(status.started_at));
  const history = await getScanHistory(env);
  history.runs.unshift({
    started_at: status.started_at,
    finished_at: status.finished_at,
    total_repos: status.total_repos,
    success_repos: status.success_repos,
    failed_repos: status.failed_repos,
    duration_ms: Number.isFinite(durationMs) ? durationMs : 0,
  });
  history.runs = history.runs.slice(0, 20);
  await env.SKILL_METRICS_CACHE.put(SCAN_HISTORY_CACHE_KEY, JSON.stringify(history), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
}

async function startCatalogScan(env: Env): Promise<ScanStatus> {
  const repos = (await listTrackedRepos(env)).filter((item) => item.enabled);
  const now = new Date().toISOString();
  const status: ScanStatus = {
    running: true,
    started_at: now,
    total_repos: repos.length,
    processed_repos: 0,
    success_repos: 0,
    failed_repos: 0,
    current_index: 0,
    message: "running",
    batch_size: SCAN_DEFAULT_BATCH_SIZE,
  };
  const sourceCounts = await fetchSkillCountsBySource();
  const working: ScanWorking = {
    started_at: status.started_at,
    repos,
    repo_skill_counts: sourceCounts,
    repo_stars: {},
    repo_errors: {},
  };
  await env.SKILL_METRICS_CACHE.put(SCAN_STATUS_CACHE_KEY, JSON.stringify(status), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
  await env.SKILL_METRICS_CACHE.put(SCAN_WORKING_CACHE_KEY, JSON.stringify(working), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
  return status;
}

async function stopCatalogScan(env: Env): Promise<ScanStatus> {
  const status = await getScanStatus(env);
  if (!status.running) return status;
  const stopped: ScanStatus = {
    ...status,
    running: false,
    finished_at: new Date().toISOString(),
    message: "stopped_by_admin",
  };
  await env.SKILL_METRICS_CACHE.put(SCAN_STATUS_CACHE_KEY, JSON.stringify(stopped), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
  await appendScanHistory(env, stopped);
  return stopped;
}

async function stepCatalogScan(env: Env, batchSize: number): Promise<ScanStatus> {
  const status = await getScanStatus(env);
  if (!status.running) return status;
  status.batch_size = batchSize;
  const working = await getScanWorking(env);
  if (!working) {
    const stopped: ScanStatus = { ...status, running: false, finished_at: new Date().toISOString(), message: "working_state_missing" };
    await env.SKILL_METRICS_CACHE.put(SCAN_STATUS_CACHE_KEY, JSON.stringify(stopped), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
    await appendScanHistory(env, stopped);
    return stopped;
  }
  const startIndex = status.current_index;
  const endIndex = Math.min(startIndex + batchSize, working.repos.length);

  for (let i = startIndex; i < endIndex; i += 1) {
    const repo = working.repos[i];
    const repoName = `${repo.owner}/${repo.name}`;
    status.current_repo = repoName;
    try {
      const stars = await fetchRepoStars(env, repo);
      working.repo_stars[repoKey(repo.platform, repo.owner, repo.name)] = stars;
      status.success_repos += 1;
    } catch (err) {
      status.failed_repos += 1;
      const message = err instanceof Error ? err.message : "star_fetch_failed";
      working.repo_errors[repoKey(repo.platform, repo.owner, repo.name)] = message;
    }
    status.processed_repos = i + 1;
    status.current_index = i + 1;
  }

  if (status.current_index >= working.repos.length) {
    const catalog = await buildCatalogFromSkillsSource(env, working.repos, working.repo_skill_counts, working.repo_stars, working.repo_errors);
    await env.SKILL_METRICS_CACHE.put(SKILL_CATALOG_CACHE_KEY, JSON.stringify(catalog), { expirationTtl: SKILL_CATALOG_TTL_SECONDS });
    status.running = false;
    status.finished_at = new Date().toISOString();
    status.last_scan_at = status.finished_at;
    status.message = "completed";
    await appendScanHistory(env, status);
  } else {
    status.message = "running";
  }

  await env.SKILL_METRICS_CACHE.put(SCAN_STATUS_CACHE_KEY, JSON.stringify(status), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
  await env.SKILL_METRICS_CACHE.put(SCAN_WORKING_CACHE_KEY, JSON.stringify(working), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
  return status;
}

async function retryFailedCatalogScan(env: Env): Promise<ScanStatus> {
  const failures = await listScanFailures(env);
  if (!failures.length) {
    return {
      running: false,
      started_at: "",
      total_repos: 0,
      processed_repos: 0,
      success_repos: 0,
      failed_repos: 0,
      current_index: 0,
      message: "no_failures",
    };
  }
  const allRepos = await listTrackedRepos(env);
  const failedKeys = new Set(failures.map((item) => item.repo_key));
  const repos = allRepos.filter((item) => item.enabled && failedKeys.has(repoKey(item.platform, item.owner, item.name)));
  const status: ScanStatus = {
    running: true,
    started_at: new Date().toISOString(),
    total_repos: repos.length,
    processed_repos: 0,
    success_repos: 0,
    failed_repos: 0,
    current_index: 0,
    message: "retry_failures",
    batch_size: SCAN_DEFAULT_BATCH_SIZE,
  };
  const sourceCounts = await fetchSkillCountsBySource();
  const working: ScanWorking = {
    started_at: status.started_at,
    repos,
    repo_skill_counts: sourceCounts,
    repo_stars: {},
    repo_errors: {},
  };
  await env.SKILL_METRICS_CACHE.put(SCAN_STATUS_CACHE_KEY, JSON.stringify(status), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
  await env.SKILL_METRICS_CACHE.put(SCAN_WORKING_CACHE_KEY, JSON.stringify(working), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
  return status;
}

async function runExternalSyncNow(
  env: Env,
  onProgress?: (stage: string) => Promise<void> | void,
): Promise<{
  ok: true;
  finished_at: string;
  tracked_repos: number;
  skills_external_map: number;
  audit_skills: number;
  audit_rows: number;
  catalog_fetched_at?: string;
  indexed_total?: number;
}> {
  await onProgress?.("prepare_state");
  let state = await loadSkillsShSyncState(env);
  state = await ensureSkillsShRobotsState(env, state, true);
  if (state.robots_allowed === false) {
    throw new Error("skills_sh_robots_disallow");
  }
  await onProgress?.("sync_audits");
  state = await runSkillsShAuditSyncTask(env, state, true);
  await onProgress?.("sync_repos");
  state = await runSkillsShRepoSyncTask(env, state, true);
  await onProgress?.("save_state");
  state.updated_at = new Date().toISOString();
  await saveSkillsShSyncState(env, state);
  await onProgress?.("refresh_catalog");
  const catalog = await getSkillCatalog(env, true);
  const tracked = await env.DB.prepare("SELECT COUNT(*) AS total FROM tracked_repos").first<{ total?: number }>();
  const mapped = await env.DB.prepare("SELECT COUNT(*) AS total FROM skills_external_map").first<{ total?: number }>();
  const auditSkills = await env.DB.prepare("SELECT COUNT(DISTINCT skill_key) AS total FROM skills_audit_cache").first<{ total?: number }>();
  const auditRows = await env.DB.prepare("SELECT COUNT(*) AS total FROM skills_audit_cache").first<{ total?: number }>();
  return {
    ok: true,
    finished_at: new Date().toISOString(),
    tracked_repos: Number(tracked?.total ?? 0),
    skills_external_map: Number(mapped?.total ?? 0),
    audit_skills: Number(auditSkills?.total ?? 0),
    audit_rows: Number(auditRows?.total ?? 0),
    catalog_fetched_at: catalog.fetched_at,
    indexed_total: Array.isArray(catalog.items) ? catalog.items.length : undefined,
  };
}

type ExternalSyncRunStatus = {
  running: boolean;
  started_at?: string;
  finished_at?: string;
  stage?: string;
  heartbeat_at?: string;
  error?: string;
  tracked_repos?: number;
  skills_external_map?: number;
  audit_skills?: number;
  audit_rows?: number;
  catalog_fetched_at?: string;
  indexed_total?: number;
};

async function getExternalSyncRunStatus(env: Env): Promise<ExternalSyncRunStatus | null> {
  const raw = await env.SKILL_METRICS_CACHE.get(EXTERNAL_RUN_STATUS_CACHE_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as ExternalSyncRunStatus;
  } catch {
    return null;
  }
}

async function triggerExternalSyncBackground(
  env: Env,
  ctx: ExecutionContext,
): Promise<{ started: boolean; message: string; status?: ExternalSyncRunStatus }> {
  const lock = await env.SKILL_METRICS_CACHE.get(EXTERNAL_RUN_LOCK_CACHE_KEY);
  if (lock) {
    const status = await getExternalSyncRunStatus(env);
    return {
      started: false,
      message: "sync_already_running",
      status: status ?? { running: true },
    };
  }

  const startedAt = new Date().toISOString();
  await env.SKILL_METRICS_CACHE.put(EXTERNAL_RUN_LOCK_CACHE_KEY, startedAt, { expirationTtl: 20 * 60 });
  await env.SKILL_METRICS_CACHE.put(
    EXTERNAL_RUN_STATUS_CACHE_KEY,
    JSON.stringify({
      running: true,
      started_at: startedAt,
      stage: "queued",
      heartbeat_at: startedAt,
    } satisfies ExternalSyncRunStatus),
    { expirationTtl: 24 * 60 * 60 },
  );

  ctx.waitUntil((async () => {
    const updateStatus = async (patch: Partial<ExternalSyncRunStatus>) => {
      const current = (await getExternalSyncRunStatus(env)) ?? { running: true, started_at: startedAt };
      const next = {
        ...current,
        ...patch,
      };
      await env.SKILL_METRICS_CACHE.put(EXTERNAL_RUN_STATUS_CACHE_KEY, JSON.stringify(next), { expirationTtl: 3 * 24 * 60 * 60 });
    };
    try {
      await updateStatus({ running: true, stage: "running", heartbeat_at: new Date().toISOString() });
      const result = await runExternalSyncNow(env, async (stage) => {
        await updateStatus({ running: true, stage, heartbeat_at: new Date().toISOString() });
      });
      await env.SKILL_METRICS_CACHE.put(
        EXTERNAL_RUN_STATUS_CACHE_KEY,
        JSON.stringify({
          running: false,
          started_at: startedAt,
          finished_at: result.finished_at,
          stage: "done",
          heartbeat_at: new Date().toISOString(),
          tracked_repos: result.tracked_repos,
          skills_external_map: result.skills_external_map,
          audit_skills: result.audit_skills,
          audit_rows: result.audit_rows,
          catalog_fetched_at: result.catalog_fetched_at,
          indexed_total: result.indexed_total,
        } satisfies ExternalSyncRunStatus),
        { expirationTtl: 3 * 24 * 60 * 60 },
      );
    } catch (err) {
      await env.SKILL_METRICS_CACHE.put(
        EXTERNAL_RUN_STATUS_CACHE_KEY,
        JSON.stringify({
          running: false,
          started_at: startedAt,
          finished_at: new Date().toISOString(),
          stage: "failed",
          heartbeat_at: new Date().toISOString(),
          error: err instanceof Error ? err.message : "external_sync_failed",
        } satisfies ExternalSyncRunStatus),
        { expirationTtl: 3 * 24 * 60 * 60 },
      );
    } finally {
      await env.SKILL_METRICS_CACHE.delete(EXTERNAL_RUN_LOCK_CACHE_KEY);
    }
  })());

  return {
    started: true,
    message: "sync_started",
    status: { running: true, started_at: startedAt },
  };
}

async function buildCatalogFromSkillsSource(
  env: Env,
  repos: TrackedRepoItem[],
  sourceCounts?: Record<string, number>,
  repoStars?: Record<string, number>,
  repoErrors?: Record<string, string>,
): Promise<SkillCatalog> {
  const entries = await fetchSkillsSourceEntries();
  const trackedRepos = repos.filter((item) => item.enabled);
  const trackedMap = new Map<string, TrackedRepoItem>();
  for (const repo of trackedRepos) {
    trackedMap.set(repoKey(repo.platform, repo.owner, repo.name), repo);
  }

  const counts = sourceCounts ?? countSkillsBySource(entries);
  const reposSummary: CatalogRepoSummary[] = trackedRepos.map((repo) => {
    const key = repoKey(repo.platform, repo.owner, repo.name);
    return {
      ...repo,
      skill_count: Number(counts[key] ?? 0),
      star_count: Number(repoStars?.[key] ?? 0),
      error: repoErrors?.[key],
    };
  });

  const dedupe = new Set<string>();
  const items: CatalogSkillItem[] = [];
  const repoRootsCache = new Map<string, string[]>();
  const repoRootSkillCache = new Map<string, boolean>();
  const auditOverlay = await loadAuditOverlay(env);
  for (const entry of entries) {
    const repoInfo = parseSourceRepo(entry.source);
    if (!repoInfo) continue;
    const key = repoKey(repoInfo.platform, repoInfo.owner, repoInfo.name);
    const tracked = trackedMap.get(key);
    if (!tracked) continue;
    const itemId = `${key}:${entry.skill_id}`;
    if (dedupe.has(itemId)) continue;
    dedupe.add(itemId);
    const branch = tracked.branch || "main";
    const roots = await getRepoSkillRoots(env, repoInfo.platform, repoInfo.owner, repoInfo.name, branch, repoRootsCache);
    const hasRootSkill = await getRepoHasRootSkill(env, repoInfo.platform, repoInfo.owner, repoInfo.name, branch, repoRootSkillCache);
    const sourcePath = resolvePreferredCatalogSourcePath(entry.skill_id, roots, hasRootSkill, repoInfo.owner, repoInfo.name);
    const skillKey = discoveredSkillId(repoInfo.platform, repoInfo.owner, repoInfo.name, branch, sourcePath);
    const sourceRepoKey = `${repoInfo.owner.toLowerCase()}/${repoInfo.name.toLowerCase()}`;
    const fallbackPathKey = `${sourceRepoKey}::${normalizePathKey(sourcePath)}`;
    const fallbackNameKey = `${sourceRepoKey}::${normalizeName(entry.name || entry.skill_id)}`;
    const overlay = auditOverlay.bySkillKey.get(skillKey)
      ?? auditOverlay.byRepoAndPath.get(fallbackPathKey)
      ?? auditOverlay.byRepoAndName.get(fallbackNameKey);
    items.push({
      id: discoveredSkillId(repoInfo.platform, repoInfo.owner, repoInfo.name, branch, sourcePath),
      name: entry.name || entry.skill_id,
      platform: repoInfo.platform,
      repo_owner: repoInfo.owner,
      repo_name: repoInfo.name,
      branch,
      repo_url: repoHomeURL(repoInfo.platform, repoInfo.owner, repoInfo.name),
      source_path: sourcePath,
      source_url: repoTreeURL(repoInfo.platform, repoInfo.owner, repoInfo.name, branch, sourcePath),
      managed_name: buildDiscoveredManagedName(repoInfo.owner, repoInfo.name, sourcePath),
      skills_sh_url: overlay?.skills_sh_url,
      audits_summary: overlay?.audits_summary,
    });
  }

  items.sort((a, b) => {
    const byName = a.name.localeCompare(b.name, "en", { sensitivity: "base" });
    if (byName !== 0) return byName;
    return a.id.localeCompare(b.id, "en", { sensitivity: "base" });
  });

  return {
    fetched_at: new Date().toISOString(),
    repos: reposSummary,
    items,
  };
}

function resolvePreferredCatalogSourcePath(
  skillId: string,
  roots: string[],
  hasRootSkill: boolean,
  repoOwner: string,
  repoName: string,
): string {
  if (hasRootSkill || roots.some((root) => root === ".")) {
    return ".";
  }
  const resolved = resolveCatalogSourcePath(skillId, roots, repoOwner, repoName);
  if (resolved === "." || roots.length > 0) {
    return resolved;
  }
  const normalizedSkill = normalizeCatalogSourcePath(skillId).toLowerCase();
  const normalizedRepo = repoName.trim().toLowerCase();
  if (normalizedSkill && normalizedRepo && normalizedSkill === normalizedRepo) {
    return ".";
  }
  return resolved;
}

function parseSourceRepo(source: string): { platform: "github" | "gitlab"; owner: string; name: string } | null {
  const trimmed = source.trim();
  const platform = /^https?:\/\/(www\.)?gitlab\.com\//i.test(trimmed) ? "gitlab" : "github";
  const value = trimmed.replace(/^https?:\/\/(www\.)?github\.com\//i, "").replace(/^https?:\/\/(www\.)?gitlab\.com\//i, "");
  const parts = value.split("/").filter(Boolean);
  if (parts.length < 2) return null;
  const owner = parts[0];
  const name = parts[1];
  if (!owner || !name) return null;
  return { platform, owner, name };
}

function decodePathSegment(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function isLikelySkillSlug(value: string): boolean {
  if (!value) return false;
  if (value.startsWith("_")) return false;
  const lowered = value.toLowerCase();
  if (lowered === "docs" || lowered === "official" || lowered === "trending" || lowered === "hot" || lowered === "audits") {
    return false;
  }
  return true;
}

function extractSkillsSourceEntriesFromHtml(html: string): SkillsSourceEntry[] {
  const dedupe = new Set<string>();
  const entries: SkillsSourceEntry[] = [];
  const pushEntry = (source: string, skillId: string, name?: string): void => {
    const repoInfo = parseSourceRepo(source);
    if (!repoInfo) return;
    const normalizedSkillId = decodePathSegment(skillId.trim());
    if (!isLikelySkillSlug(normalizedSkillId)) return;
    const key = `${repoKey(repoInfo.platform, repoInfo.owner, repoInfo.name)}:${normalizedSkillId.toLowerCase()}`;
    if (dedupe.has(key)) return;
    dedupe.add(key);
    entries.push({
      source: `${repoInfo.owner}/${repoInfo.name}`,
      skill_id: normalizedSkillId,
      name: (name ?? normalizedSkillId).trim() || normalizedSkillId,
    });
  };

  const legacyPattern = /source\\":\\"([^"\\]+)\\",\\"skillId\\":\\"([^"\\]+)\\",\\"name\\":\\"([^"\\]*)/g;
  let legacyMatch: RegExpExecArray | null = null;
  while ((legacyMatch = legacyPattern.exec(html)) !== null) {
    const source = legacyMatch[1]?.trim() ?? "";
    const skillId = legacyMatch[2]?.trim() ?? "";
    const name = (legacyMatch[3] ?? "").replace(/\\u[\da-fA-F]{4}/g, "").trim() || skillId;
    if (!source || !skillId) continue;
    pushEntry(source, skillId, name);
  }

  const hrefPattern = /href="\/([^\/"?#<]+)\/([^\/"?#<]+)\/([^\/"?#<]+)"/g;
  let hrefMatch: RegExpExecArray | null = null;
  while ((hrefMatch = hrefPattern.exec(html)) !== null) {
    const owner = decodePathSegment((hrefMatch[1] ?? "").trim());
    const repo = decodePathSegment((hrefMatch[2] ?? "").trim());
    const slug = decodePathSegment((hrefMatch[3] ?? "").trim());
    if (!owner || !repo || !slug) continue;
    if (owner.startsWith("_") || repo.startsWith("_")) continue;
    pushEntry(`${owner}/${repo}`, slug, slug);
  }

  return entries;
}

async function fetchSkillsShSkillsApiPage(
  page: number,
  fastMode = false,
): Promise<{ skills: SkillsShListApiSkill[]; total: number; hasMore: boolean }> {
  const normalizedPage = Math.max(0, Number.isFinite(page) ? Math.floor(page) : 0);
  const endpoint = `${SKILLS_SH_SKILLS_ALL_TIME_API}/${normalizedPage}`;
  const headers: Record<string, string> = {
    "User-Agent": "aigate-skill-metrics",
    Accept: "application/json",
  };
  if (!fastMode) {
    await sleep(randomInt(EXTERNAL_SYNC_PAGE_DELAY_MIN_MS, EXTERNAL_SYNC_PAGE_DELAY_MAX_MS));
  }
  const resp = await fetchWithRetry(endpoint, { headers }, EXTERNAL_FETCH_RETRIES, EXTERNAL_FETCH_TIMEOUT_MS);
  if (!resp.ok) {
    throw new Error(`skills_all_time_api_http_${resp.status}`);
  }
  const payload = (await resp.json()) as {
    skills?: SkillsShListApiSkill[];
    total?: number;
    hasMore?: boolean;
  };
  return {
    skills: Array.isArray(payload.skills) ? payload.skills : [],
    total: Number(payload.total ?? 0),
    hasMore: Boolean(payload.hasMore),
  };
}

async function fetchSkillsSourceEntriesFromApiAllTime(fastMode = false): Promise<SkillsSourceEntry[]> {
  const dedupe = new Set<string>();
  const entries: SkillsSourceEntry[] = [];
  let page = 0;
  while (page < SKILLS_SH_SKILLS_MAX_PAGES) {
    const response = await fetchSkillsShSkillsApiPage(page, fastMode);
    const pageSkills = Array.isArray(response.skills) ? response.skills : [];
    for (const item of pageSkills) {
      const source = String(item.source ?? "").trim();
      const skillId = String(item.skillId ?? "").trim();
      if (!source || !skillId) continue;
      const repoInfo = parseSourceRepo(source);
      if (!repoInfo) continue;
      const normalizedSkillId = decodePathSegment(skillId);
      if (!isLikelySkillSlug(normalizedSkillId)) continue;
      const key = `${repoKey(repoInfo.platform, repoInfo.owner, repoInfo.name)}:${normalizedSkillId.toLowerCase()}`;
      if (dedupe.has(key)) continue;
      dedupe.add(key);
      entries.push({
        source: `${repoInfo.owner}/${repoInfo.name}`,
        skill_id: normalizedSkillId,
        name: String(item.name ?? normalizedSkillId).trim() || normalizedSkillId,
      });
    }
    if (!response.hasMore || pageSkills.length === 0) {
      break;
    }
    page += 1;
  }
  return entries;
}

async function fetchSkillsSourceEntries(): Promise<SkillsSourceEntry[]> {
  try {
    const apiEntries = await fetchSkillsSourceEntriesFromApiAllTime(false);
    if (apiEntries.length > 0) {
      return apiEntries;
    }
  } catch {
    // Fall back to HTML pages when skills.sh API is temporarily unavailable.
  }
  const dedupe = new Set<string>();
  const entries: SkillsSourceEntry[] = [];
  for (const page of SKILLS_SOURCE_PAGES) {
    const resp = await fetch(page, { headers: { "User-Agent": "aigate-skill-metrics" } });
    if (!resp.ok) continue;
    const html = await resp.text();
    const parsed = extractSkillsSourceEntriesFromHtml(html);
    for (const entry of parsed) {
      const repoInfo = parseSourceRepo(entry.source);
      if (!repoInfo || !entry.skill_id) continue;
      const key = `${repoKey(repoInfo.platform, repoInfo.owner, repoInfo.name)}:${entry.skill_id.toLowerCase()}`;
      if (dedupe.has(key)) continue;
      dedupe.add(key);
      entries.push(entry);
    }
  }
  return entries;
}

async function runSkillsShEnhancementSync(env: Env, options?: { force?: boolean }): Promise<void> {
  let state = await loadSkillsShSyncState(env);
  const fastMode = Boolean(options?.force);
  state = await ensureSkillsShRobotsState(env, state, Boolean(options?.force));
  if (state.robots_allowed === false) {
    await saveSkillsShSyncState(env, state);
    return;
  }
  const now = Date.now();
  if (fastMode) {
    if (options?.force || Date.parse(state.audits.next_run_at) <= now) {
      state = await runSkillsShAuditSyncTask(env, state, fastMode);
    }
    if (options?.force || Date.parse(state.repos.next_run_at) <= now) {
      state = await runSkillsShRepoSyncTask(env, state, fastMode);
    }
  } else {
    if (options?.force || Date.parse(state.repos.next_run_at) <= now) {
      state = await runSkillsShRepoSyncTask(env, state, fastMode);
    }
    if (options?.force || Date.parse(state.audits.next_run_at) <= now) {
      state = await runSkillsShAuditSyncTask(env, state, fastMode);
    }
  }
  state.updated_at = new Date().toISOString();
  await saveSkillsShSyncState(env, state);
}

function newSkillsShTaskState(name: "repos" | "audits"): SkillsShSyncTaskState {
  return {
    name,
    next_run_at: new Date(Date.now() + randomInt(0, EXTERNAL_SYNC_JITTER_MAX_MS)).toISOString(),
    retry_count: 0,
    cursor: 0,
  };
}

async function loadSkillsShSyncState(env: Env): Promise<SkillsShSyncState> {
  const raw = await env.SKILL_METRICS_CACHE.get(EXTERNAL_STATE_CACHE_KEY);
  if (!raw) {
    const now = new Date().toISOString();
    return {
      repos: newSkillsShTaskState("repos"),
      audits: newSkillsShTaskState("audits"),
      updated_at: now,
    };
  }
  try {
    const parsed = JSON.parse(raw) as SkillsShSyncState;
    return {
      repos: parsed.repos ?? newSkillsShTaskState("repos"),
      audits: parsed.audits ?? newSkillsShTaskState("audits"),
      robots_allowed: parsed.robots_allowed,
      robots_checked_at: parsed.robots_checked_at,
      robots_error: parsed.robots_error,
      updated_at: parsed.updated_at || new Date().toISOString(),
    };
  } catch {
    const now = new Date().toISOString();
    return {
      repos: newSkillsShTaskState("repos"),
      audits: newSkillsShTaskState("audits"),
      updated_at: now,
    };
  }
}

async function saveSkillsShSyncState(env: Env, state: SkillsShSyncState): Promise<void> {
  await env.SKILL_METRICS_CACHE.put(EXTERNAL_STATE_CACHE_KEY, JSON.stringify(state), {
    expirationTtl: EXTERNAL_SKILL_AUDIT_CACHE_TTL_SECONDS,
  });
}

async function ensureSkillsShRobotsState(env: Env, state: SkillsShSyncState, force = false): Promise<SkillsShSyncState> {
  const checkedAt = state.robots_checked_at ? Date.parse(state.robots_checked_at) : 0;
  if (!force && checkedAt > 0 && Date.now() < checkedAt + EXTERNAL_SYNC_INTERVAL_MS) {
    return state;
  }
  const robots = await fetchSkillsShRobots();
  state.robots_allowed = robots.allowed;
  state.robots_checked_at = new Date().toISOString();
  state.robots_error = robots.error || "";
  return state;
}

async function fetchSkillsShRobots(): Promise<{ allowed: boolean; error?: string }> {
  try {
    const resp = await fetch(SKILLS_SH_ROBOTS_URL, { headers: { "User-Agent": "aigate-skill-metrics" } });
    if (!resp.ok) {
      if (resp.status === 404) {
        // skills.sh currently does not expose robots.txt; fail-open for weak dependency mode.
        return { allowed: true, error: "robots_missing" };
      }
      return { allowed: true, error: `robots_http_${resp.status}` };
    }
    const raw = await resp.text();
    const rules = parseRobotsRules(raw, "aigate-skill-metrics");
    const allowed = isRobotsPathAllowed(rules, "/") && isRobotsPathAllowed(rules, "/audits");
    return { allowed, error: allowed ? "" : "robots_disallow" };
  } catch (err) {
    const message = err instanceof Error ? err.message : "robots_fetch_failed";
    return { allowed: true, error: message };
  }
}

function parseRobotsRules(raw: string, userAgent: string): Map<string, string[]> {
  const lines = raw.split(/\r?\n/).map((line) => line.replace(/\s+#.*$/, "").trim()).filter(Boolean);
  const rules = new Map<string, string[]>();
  let currentAgents: string[] = [];
  for (const line of lines) {
    const [keyRaw, ...rest] = line.split(":");
    const key = keyRaw.trim().toLowerCase();
    const value = rest.join(":").trim();
    if (key === "user-agent") {
      currentAgents = [value.toLowerCase()];
      if (!rules.has(currentAgents[0])) {
        rules.set(currentAgents[0], []);
      }
      continue;
    }
    if (key !== "allow" && key !== "disallow") {
      continue;
    }
    for (const agent of currentAgents) {
      const current = rules.get(agent) ?? [];
      current.push(`${key}:${value}`);
      rules.set(agent, current);
    }
  }
  if (!rules.has(userAgent.toLowerCase()) && !rules.has("*")) {
    return new Map([["*", []]]);
  }
  return rules;
}

function isRobotsPathAllowed(rules: Map<string, string[]>, path: string): boolean {
  const selected = rules.get("aigate-skill-metrics") ?? rules.get("*") ?? [];
  let blocked = false;
  let allowLen = -1;
  let blockLen = -1;
  for (const rule of selected) {
    const [kind, valueRaw] = rule.split(":", 2);
    const value = valueRaw ?? "";
    if (!value) {
      continue;
    }
    if (!path.startsWith(value)) {
      continue;
    }
    if (kind === "allow" && value.length > allowLen) {
      allowLen = value.length;
    }
    if (kind === "disallow" && value.length > blockLen) {
      blockLen = value.length;
      blocked = true;
    }
  }
  if (!blocked) return true;
  return allowLen >= blockLen;
}

async function runSkillsShRepoSyncTask(env: Env, state: SkillsShSyncState, fastMode = false): Promise<SkillsShSyncState> {
  try {
    const sourceData = await fetchSkillsSourceEntriesForSync(state.repos, fastMode);
    const entries = sourceData.entries;
    const repos = dedupeReposFromEntries(entries);
    await upsertMissingTrackedRepos(env, repos);
    state.repos.cursor = 0;
    state.repos.total_items = repos.length;
    state.repos.retry_count = 0;
    state.repos.last_error = "";
    state.repos.last_success_at = new Date().toISOString();
    state.repos.etag = sourceData.etag || state.repos.etag;
    state.repos.last_modified = sourceData.lastModified || state.repos.last_modified;
    state.repos.next_run_at = new Date(Date.now() + EXTERNAL_SYNC_INTERVAL_MS + randomInt(0, EXTERNAL_SYNC_JITTER_MAX_MS)).toISOString();
  } catch (err) {
    state.repos = applySyncRetryState(state.repos, err);
  }
  return state;
}

async function fetchSkillsSourceEntriesForSync(
  task: SkillsShSyncTaskState,
  fastMode = false,
): Promise<{ entries: SkillsSourceEntry[]; etag?: string; lastModified?: string; notModified: boolean }> {
  const apiEntries = await fetchSkillsSourceEntriesFromApiAllTime(fastMode);
  if (apiEntries.length > 0) {
    return { entries: apiEntries, notModified: false };
  }
  const fallbackEntries = await fetchSkillsSourceEntries();
  return {
    entries: fallbackEntries,
    etag: task.etag,
    lastModified: task.last_modified,
    notModified: false,
  };
}

async function runSkillsShAuditSyncTask(env: Env, state: SkillsShSyncState, fastMode = false): Promise<SkillsShSyncState> {
  try {
    let page = 0;
    let totalItems = state.audits.total_items ?? 0;
    while (page < SKILLS_SH_AUDITS_MAX_PAGES) {
      const response = await fetchSkillsShAuditsApiPage(page, fastMode);
      const entries = extractSkillsShAuditEntriesFromApi(response.skills);
      if (entries.length > 0) {
        await upsertSkillsShAuditEntries(env, entries);
        await upsertSkillsShExternalMappings(env, entries);
      }
      totalItems = Number(response.total ?? totalItems);
      if (!response.hasMore || entries.length === 0) {
        break;
      }
      page += 1;
      if (!fastMode) {
        await sleep(randomInt(EXTERNAL_SYNC_PAGE_DELAY_MIN_MS, EXTERNAL_SYNC_PAGE_DELAY_MAX_MS));
      }
    }
    state.audits.cursor = 0;
    state.audits.total_items = totalItems;
    state.audits.retry_count = 0;
    state.audits.last_error = "";
    state.audits.last_success_at = new Date().toISOString();
    state.audits.next_run_at = new Date(Date.now() + EXTERNAL_SYNC_INTERVAL_MS + randomInt(0, EXTERNAL_SYNC_JITTER_MAX_MS)).toISOString();
  } catch (err) {
    state.audits = applySyncRetryState(state.audits, err);
  }
  return state;
}

function applySyncRetryState(task: SkillsShSyncTaskState, err: unknown): SkillsShSyncTaskState {
  const retryCount = Math.min(task.retry_count + 1, EXTERNAL_SYNC_RETRY_BACKOFF_MS.length);
  const backoff = EXTERNAL_SYNC_RETRY_BACKOFF_MS[Math.max(0, retryCount - 1)] ?? EXTERNAL_SYNC_RETRY_BACKOFF_MS[EXTERNAL_SYNC_RETRY_BACKOFF_MS.length - 1];
  const message = err instanceof Error ? err.message : "sync_failed";
  if (retryCount >= EXTERNAL_SYNC_RETRY_BACKOFF_MS.length) {
    return {
      ...task,
      retry_count: 0,
      last_error: message,
      next_run_at: new Date(Date.now() + EXTERNAL_SYNC_INTERVAL_MS + randomInt(0, EXTERNAL_SYNC_JITTER_MAX_MS)).toISOString(),
    };
  }
  return {
    ...task,
    retry_count: retryCount,
    last_error: message,
    next_run_at: new Date(Date.now() + backoff).toISOString(),
  };
}

async function fetchSkillsShAuditsApiPage(
  page: number,
  fastMode = false,
): Promise<{ skills: SkillsShAuditApiSkill[]; total: number; hasMore: boolean }> {
  const normalizedPage = Math.max(0, Number.isFinite(page) ? Math.floor(page) : 0);
  const endpoint = `https://skills.sh/api/audits/${normalizedPage}`;
  const headers: Record<string, string> = {
    "User-Agent": "aigate-skill-metrics",
    Accept: "application/json",
  };
  if (!fastMode) {
    await sleep(randomInt(EXTERNAL_SYNC_PAGE_DELAY_MIN_MS, EXTERNAL_SYNC_PAGE_DELAY_MAX_MS));
  }
  const resp = await fetchWithRetry(endpoint, { headers }, EXTERNAL_FETCH_RETRIES, EXTERNAL_FETCH_TIMEOUT_MS);
  if (!resp.ok) {
    throw new Error(`audits_api_http_${resp.status}`);
  }
  const payload = (await resp.json()) as {
    skills?: SkillsShAuditApiSkill[];
    total?: number;
    hasMore?: boolean;
  };
  return {
    skills: Array.isArray(payload.skills) ? payload.skills : [],
    total: Number(payload.total ?? 0),
    hasMore: Boolean(payload.hasMore),
  };
}

function dedupeReposFromEntries(entries: SkillsSourceEntry[]): TrackedRepoItem[] {
  const map = new Map<string, TrackedRepoItem>();
  for (const entry of entries) {
    const repoInfo = parseSourceRepo(entry.source);
    if (!repoInfo) continue;
    const key = repoKey(repoInfo.platform, repoInfo.owner, repoInfo.name);
    if (map.has(key)) continue;
    map.set(key, {
      platform: repoInfo.platform,
      owner: repoInfo.owner,
      name: repoInfo.name,
      branch: "main",
      enabled: true,
      sort_order: 0,
    });
  }
  return [...map.values()];
}

async function upsertMissingTrackedRepos(env: Env, repos: TrackedRepoItem[]): Promise<void> {
  for (let i = 0; i < repos.length; i += 1) {
    const repo = repos[i];
    const key = repoKey(repo.platform, repo.owner, repo.name);
    const exists = await env.DB.prepare("SELECT repo_key FROM tracked_repos WHERE repo_key = ?1").bind(key).first<{ repo_key: string }>();
    if (exists?.repo_key) {
      continue;
    }
    const sortOrder = await nextTrackedRepoSortOrder(env);
    const now = new Date().toISOString();
    await env.DB
      .prepare(
        `INSERT INTO tracked_repos (repo_key, platform, owner, name, branch, enabled, sort_order, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)`,
      )
      .bind(key, repo.platform, repo.owner, repo.name, "main", 1, sortOrder, now)
      .run();
  }
}

function extractSkillsShAuditEntries(html: string): SkillsShAuditEntry[] {
  const rows = new Map<string, SkillsShAuditEntry>();
  const rowPattern = /<a[^>]+href="\/([^\/"?#<]+)\/([^\/"?#<]+)\/([^\/"?#<]+)"[^>]*>([\s\S]*?)<\/a>/g;
  let match: RegExpExecArray | null = null;
  while ((match = rowPattern.exec(html)) !== null) {
    const owner = decodePathSegment((match[1] ?? "").trim());
    const repo = decodePathSegment((match[2] ?? "").trim());
    const skillId = decodePathSegment((match[3] ?? "").trim());
    if (!owner || !repo || !skillId) continue;
    if (owner.startsWith("_") || repo.startsWith("_") || !isLikelySkillSlug(skillId)) continue;
    const source = `${owner}/${repo}`;
    const key = `${source.toLowerCase()}::${skillId.toLowerCase()}`;
    const rowHtml = match[0] ?? "";
    const skillUrlPath = `/${source}/${skillId}`;
    const spanTexts = [...rowHtml.matchAll(/<span[^>]*>([^<]+)<\/span>/g)].map((item) =>
      (item[1] ?? "").replace(/\s+/g, " ").trim().toLowerCase(),
    );
    const athStatus = inferAthStatus(spanTexts);
    const socketStatus = inferSocketStatus(spanTexts);
    const snykStatus = inferSnykStatus(spanTexts);
    const providers: SkillsShAuditEntry["providers"] = [
      { provider: "agent_trust_hub", label: "Agent Trust Hub", status: athStatus, url: `https://skills.sh${skillUrlPath}/security/agent-trust-hub` },
      { provider: "socket", label: "Socket", status: socketStatus, url: `https://skills.sh${skillUrlPath}/security/socket` },
      { provider: "snyk", label: "Snyk", status: snykStatus, url: `https://skills.sh${skillUrlPath}/security/snyk` },
    ];
    const title = rowHtml.match(/<h3[^>]*>([^<]+)<\/h3>/i)?.[1]?.trim() || skillId;
    rows.set(key, {
      source,
      skill_id: skillId,
      name: title,
      slug: skillId,
      providers,
    });
  }
  return [...rows.values()];
}

function extractSkillsShAuditEntriesFromApi(items: SkillsShAuditApiSkill[]): SkillsShAuditEntry[] {
  const rows = new Map<string, SkillsShAuditEntry>();
  for (const item of items) {
    const source = (item.source ?? "").trim();
    const skillId = (item.skillId ?? "").trim();
    if (!source || !skillId) continue;
    const repoInfo = parseSourceRepo(source);
    if (!repoInfo) continue;
    const normalizedSource = `${repoInfo.owner}/${repoInfo.name}`;
    const normalizedSkillId = decodePathSegment(skillId);
    const key = `${normalizedSource.toLowerCase()}::${normalizedSkillId.toLowerCase()}`;
    const skillUrlPath = `/${normalizedSource}/${normalizedSkillId}`;
    const providers: SkillsShAuditEntry["providers"] = [
      {
        provider: "agent_trust_hub",
        label: "Agent Trust Hub",
        status: riskLevelToStatus(item.agentTrustHub?.result?.overall_risk_level),
        url: `https://skills.sh${skillUrlPath}/security/agent-trust-hub`,
      },
      {
        provider: "socket",
        label: "Socket",
        status: socketAlertCountToStatus(item.socket?.result?.alertCount),
        url: `https://skills.sh${skillUrlPath}/security/socket`,
      },
      {
        provider: "snyk",
        label: "Snyk",
        status: riskLevelToStatus(item.snyk?.result?.overall_risk_level),
        url: `https://skills.sh${skillUrlPath}/security/snyk`,
      },
    ];
    rows.set(key, {
      source: normalizedSource,
      skill_id: normalizedSkillId,
      name: (item.name ?? normalizedSkillId).trim() || normalizedSkillId,
      slug: normalizedSkillId,
      providers,
    });
  }
  return [...rows.values()];
}

function riskLevelToStatus(value?: string): "pass" | "warn" | "fail" | "info" {
  const level = (value ?? "").trim().toLowerCase();
  if (!level) return "info";
  if (level === "safe" || level === "low") return "pass";
  if (level === "medium" || level === "med" || level === "warning" || level === "warn") return "warn";
  if (level === "high" || level === "critical" || level === "danger") return "fail";
  return "info";
}

function socketAlertCountToStatus(value?: number): "pass" | "warn" | "fail" | "info" {
  if (typeof value !== "number" || Number.isNaN(value)) return "info";
  return value > 0 ? "warn" : "pass";
}

function inferAthStatus(tokens: string[]): "pass" | "warn" | "fail" | "info" {
  for (const token of tokens) {
    if (token.includes("safe")) return "pass";
    if (token.includes("danger") || token.includes("unsafe")) return "fail";
    if (token.includes("warn")) return "warn";
  }
  return "info";
}

function inferSocketStatus(tokens: string[]): "pass" | "warn" | "fail" | "info" {
  for (const token of tokens) {
    const alertsMatch = token.match(/(\d+)\s*alerts?/);
    if (alertsMatch) {
      const alerts = Number(alertsMatch[1] ?? "0");
      return alerts > 0 ? "warn" : "pass";
    }
  }
  return "info";
}

function inferSnykStatus(tokens: string[]): "pass" | "warn" | "fail" | "info" {
  for (const token of tokens) {
    if (!token.includes("risk")) continue;
    if (token.includes("low")) return "pass";
    if (token.includes("med") || token.includes("medium")) return "warn";
    if (token.includes("high") || token.includes("critical")) return "fail";
    return "info";
  }
  return "info";
}

async function upsertSkillsShAuditEntries(env: Env, entries: SkillsShAuditEntry[]): Promise<void> {
  const now = new Date().toISOString();
  const expiresAt = new Date(Date.now() + EXTERNAL_SKILL_AUDIT_CACHE_TTL_SECONDS * 1000).toISOString();
  for (const entry of entries) {
    for (const provider of entry.providers) {
      const skillKey = `${entry.source.toLowerCase()}:${entry.skill_id.toLowerCase()}`;
      await env.DB
        .prepare(
          `INSERT INTO skills_audit_cache (
            skill_key, source_repo, source_skill_id, provider, status, label, source_url, fetched_at, expires_at
          ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
          ON CONFLICT(skill_key, provider) DO UPDATE SET
            source_repo=excluded.source_repo,
            source_skill_id=excluded.source_skill_id,
            status=excluded.status,
            label=excluded.label,
            source_url=excluded.source_url,
            fetched_at=excluded.fetched_at,
            expires_at=excluded.expires_at`,
        )
        .bind(
          skillKey,
          entry.source.toLowerCase(),
          entry.skill_id.toLowerCase(),
          provider.provider,
          provider.status,
          provider.label,
          provider.url,
          now,
          expiresAt,
        )
        .run();
    }
  }
}

async function upsertSkillsShExternalMappings(env: Env, entries: SkillsShAuditEntry[]): Promise<void> {
  const now = new Date().toISOString();
  for (const entry of entries) {
    const repoInfo = parseSourceRepo(entry.source);
    if (!repoInfo) continue;
    const canonicalPath = normalizeCatalogSourcePath(entry.skill_id);
    const exactSkillKey = discoveredSkillId(repoInfo.platform, repoInfo.owner, repoInfo.name, "main", canonicalPath);
    await env.DB
      .prepare(
        `INSERT INTO skills_external_map (
          skill_key, source_repo, source_path, source_name_normalized, skills_sh_slug, skills_sh_url, match_confidence, matched_at
        ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
        ON CONFLICT(skill_key) DO UPDATE SET
          source_repo=excluded.source_repo,
          source_path=excluded.source_path,
          source_name_normalized=excluded.source_name_normalized,
          skills_sh_slug=excluded.skills_sh_slug,
          skills_sh_url=excluded.skills_sh_url,
          match_confidence=excluded.match_confidence,
          matched_at=excluded.matched_at`,
      )
      .bind(
        exactSkillKey,
        `${repoInfo.owner.toLowerCase()}/${repoInfo.name.toLowerCase()}`,
        canonicalPath.toLowerCase(),
        normalizeName(entry.name),
        entry.slug.toLowerCase(),
        `https://skills.sh/${repoInfo.owner}/${repoInfo.name}/${entry.slug}`,
        1.0,
        now,
      )
      .run();
  }
}

function normalizeName(input: string): string {
  return input.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

function normalizePathKey(input: string): string {
  const normalized = normalizeCatalogSourcePath(input || "").trim().toLowerCase();
  if (!normalized || normalized === ".") {
    return ".";
  }
  const withoutPrefix = normalized.replace(/^\.?\/*/, "").replace(/^skills\//, "");
  return withoutPrefix || ".";
}

async function loadAuditOverlay(env: Env): Promise<{
  bySkillKey: Map<string, { skills_sh_url: string; audits_summary: CatalogSkillItem["audits_summary"] }>;
  byRepoAndPath: Map<string, { skills_sh_url: string; audits_summary: CatalogSkillItem["audits_summary"] }>;
  byRepoAndName: Map<string, { skills_sh_url: string; audits_summary: CatalogSkillItem["audits_summary"] }>;
}> {
  const now = new Date().toISOString();
  const rows = await env.DB
    .prepare(
      `SELECT
         m.skill_key AS skill_key,
         m.source_repo AS source_repo,
         m.source_path AS source_path,
         m.source_name_normalized AS source_name_normalized,
         m.skills_sh_url AS skills_sh_url,
         m.match_confidence AS match_confidence,
         c.provider AS provider,
         c.status AS status,
         c.label AS label,
         c.source_url AS source_url
       FROM skills_external_map m
       LEFT JOIN skills_audit_cache c
         ON c.skill_key = (LOWER(m.source_repo) || ':' || LOWER(m.source_path))
        AND c.expires_at >= ?1`,
    )
    .bind(now)
    .all<{
      skill_key: string;
      source_repo: string;
      source_path: string;
      source_name_normalized: string;
      skills_sh_url: string;
      match_confidence: number;
      provider: "agent_trust_hub" | "socket" | "snyk" | null;
      status: "pass" | "warn" | "fail" | "info" | null;
      label: string | null;
      source_url: string | null;
    }>();
  const bySkillKey = new Map<string, { skills_sh_url: string; audits_summary: CatalogSkillItem["audits_summary"] }>();
  const byRepoAndPath = new Map<string, { skills_sh_url: string; audits_summary: CatalogSkillItem["audits_summary"] }>();
  const byRepoAndName = new Map<string, { skills_sh_url: string; audits_summary: CatalogSkillItem["audits_summary"] }>();
  const repoPathCounts = new Map<string, number>();
  const repoNameCounts = new Map<string, number>();
  for (const row of rows.results ?? []) {
    const key = (row.skill_key ?? "").trim();
    const confidence = Number(row.match_confidence ?? 0);
    if (!key || confidence < 0.7) {
      continue;
    }
    const repoPathKey = `${(row.source_repo ?? "").trim().toLowerCase()}::${normalizePathKey(row.source_path ?? "")}`;
    const repoNameKey = `${(row.source_repo ?? "").trim().toLowerCase()}::${(row.source_name_normalized ?? "").trim().toLowerCase()}`;
    repoPathCounts.set(repoPathKey, (repoPathCounts.get(repoPathKey) ?? 0) + 1);
    repoNameCounts.set(repoNameKey, (repoNameCounts.get(repoNameKey) ?? 0) + 1);
    if (!bySkillKey.has(key)) {
      bySkillKey.set(key, {
        skills_sh_url: row.skills_sh_url || "",
        audits_summary: {
          match_confidence: confidence,
          providers: [],
        },
      });
    }
    if (repoPathKey !== "::" && !byRepoAndPath.has(repoPathKey)) {
      byRepoAndPath.set(repoPathKey, {
        skills_sh_url: row.skills_sh_url || "",
        audits_summary: {
          match_confidence: confidence,
          providers: [],
        },
      });
    }
    if (repoNameKey !== "::" && !byRepoAndName.has(repoNameKey)) {
      byRepoAndName.set(repoNameKey, {
        skills_sh_url: row.skills_sh_url || "",
        audits_summary: {
          match_confidence: confidence,
          providers: [],
        },
      });
    }
    if (row.provider && row.status && row.source_url) {
      const providerItem = {
        provider: row.provider,
        status: row.status,
        label: row.label || row.provider,
        url: row.source_url,
      };
      bySkillKey.get(key)?.audits_summary?.providers.push(providerItem);
      byRepoAndPath.get(repoPathKey)?.audits_summary?.providers.push(providerItem);
      byRepoAndName.get(repoNameKey)?.audits_summary?.providers.push(providerItem);
    }
  }
  for (const [key, count] of repoPathCounts.entries()) {
    if (count > 1) {
      byRepoAndPath.delete(key);
    }
  }
  for (const [key, count] of repoNameCounts.entries()) {
    if (count > 1) {
      byRepoAndName.delete(key);
    }
  }
  return { bySkillKey, byRepoAndPath, byRepoAndName };
}

async function sleep(ms: number): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

async function fetchWithRetry(
  url: string,
  init: RequestInit,
  retries: number,
  timeoutMs: number,
): Promise<Response> {
  let lastError: unknown;
  for (let attempt = 1; attempt <= Math.max(1, retries); attempt += 1) {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), timeoutMs);
    try {
      const response = await fetch(url, { ...init, signal: controller.signal });
      clearTimeout(timeoutId);
      return response;
    } catch (err) {
      clearTimeout(timeoutId);
      lastError = err;
      if (attempt < retries) {
        await sleep(randomInt(400, 1200));
      }
    }
  }
  throw lastError instanceof Error ? lastError : new Error("external_fetch_failed");
}

function randomInt(min: number, max: number): number {
  const low = Math.min(min, max);
  const high = Math.max(min, max);
  return Math.floor(Math.random() * (high - low + 1)) + low;
}

function countSkillsBySource(entries: SkillsSourceEntry[]): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const entry of entries) {
    const repoInfo = parseSourceRepo(entry.source);
    if (!repoInfo) continue;
    const key = repoKey(repoInfo.platform, repoInfo.owner, repoInfo.name);
    counts[key] = (counts[key] ?? 0) + 1;
  }
  return counts;
}

async function fetchSkillCountsBySource(): Promise<Record<string, number>> {
  const entries = await fetchSkillsSourceEntries();
  return countSkillsBySource(entries);
}

async function fetchRepoStars(env: Env, repo: TrackedRepoItem): Promise<number> {
  const apiStars = repo.platform === "gitlab"
    ? await fetchGitLabRepoStars(repo, env.GITLAB_TOKEN)
    : await fetchGitHubRepoStars(repo, env.GITHUB_TOKEN);
  if (apiStars >= 0) {
    return apiStars;
  }
  return fetchRepoStarsViaBadge(repo);
}

async function fetchRepoStarsViaBadge(repo: TrackedRepoItem): Promise<number> {
  const badgeType = repo.platform === "gitlab" ? "gitlab" : "github";
  const endpoint = `https://img.shields.io/${badgeType}/stars/${encodeURIComponent(repo.owner)}/${encodeURIComponent(repo.name)}.json`;
  const resp = await fetch(endpoint, { headers: { "User-Agent": "aigate-skill-metrics" } });
  if (!resp.ok) {
    throw httpError(resp.status, `star_badge_failed:${repo.owner}/${repo.name}:${resp.status}`);
  }
  const payload = (await resp.json()) as { value?: string; message?: string };
  const raw = (payload.value ?? payload.message ?? "0").trim().toLowerCase();
  return parseCompactNumber(raw);
}

async function fetchGitHubRepoStars(repo: TrackedRepoItem, token?: string): Promise<number> {
  const endpoint = `https://api.github.com/repos/${encodeURIComponent(repo.owner)}/${encodeURIComponent(repo.name)}`;
  const headersWithAuth: Record<string, string> = {
    "User-Agent": "aigate-skill-metrics",
    Accept: "application/vnd.github+json",
  };
  const auth = (token ?? "").trim();
  if (auth) {
    headersWithAuth.Authorization = `Bearer ${auth}`;
  }
  let resp = await fetch(endpoint, { headers: headersWithAuth });
  if (!resp.ok && auth && (resp.status === 401 || resp.status === 403 || resp.status === 404)) {
    const headersNoAuth = {
      "User-Agent": "aigate-skill-metrics",
      Accept: "application/vnd.github+json",
    };
    resp = await fetch(endpoint, { headers: headersNoAuth });
  }
  if (!resp.ok) {
    return -1;
  }
  const payload = (await resp.json()) as { stargazers_count?: number };
  const count = Number(payload.stargazers_count ?? -1);
  return Number.isFinite(count) && count >= 0 ? count : -1;
}

async function fetchGitLabRepoStars(repo: TrackedRepoItem, token?: string): Promise<number> {
  const project = encodeURIComponent(`${repo.owner}/${repo.name}`);
  const endpoint = `https://gitlab.com/api/v4/projects/${project}`;
  const headers: Record<string, string> = { "User-Agent": "aigate-skill-metrics" };
  const auth = (token ?? "").trim();
  if (auth) {
    headers["PRIVATE-TOKEN"] = auth;
  }
  const resp = await fetch(endpoint, { headers });
  if (!resp.ok) {
    return -1;
  }
  const payload = (await resp.json()) as { star_count?: number };
  const count = Number(payload.star_count ?? -1);
  return Number.isFinite(count) && count >= 0 ? count : -1;
}

function parseCompactNumber(raw: string): number {
  if (!raw) return 0;
  const cleaned = raw.replace(/,/g, "").trim().toLowerCase();
  const unit = cleaned.slice(-1);
  const base = unit === "k" || unit === "m" || unit === "b" ? cleaned.slice(0, -1) : cleaned;
  const parsed = Number(base);
  if (!Number.isFinite(parsed)) return 0;
  if (unit === "k") return Math.round(parsed * 1_000);
  if (unit === "m") return Math.round(parsed * 1_000_000);
  if (unit === "b") return Math.round(parsed * 1_000_000_000);
  return Math.round(parsed);
}

function repoHomeURL(platform: "github" | "gitlab", owner: string, name: string): string {
  if (platform === "gitlab") {
    return `https://gitlab.com/${owner}/${name}`;
  }
  return `https://github.com/${owner}/${name}`;
}

function repoTreeURL(platform: "github" | "gitlab", owner: string, name: string, branch: string, sourcePath: string): string {
  const root = repoHomeURL(platform, owner, name);
  const normalizedPath = sourcePath.replace(/^\/+|\/+$/g, "");
  if (!normalizedPath || normalizedPath === ".") {
    return platform === "gitlab"
      ? `${root}/-/tree/${encodeURIComponent(branch)}`
      : `${root}/tree/${encodeURIComponent(branch)}`;
  }
  if (platform === "gitlab") {
    return `${root}/-/tree/${encodeURIComponent(branch)}/${normalizedPath}`;
  }
  return `${root}/tree/${encodeURIComponent(branch)}/${normalizedPath}`;
}

function normalizeCatalogSourcePath(value: string): string {
  return value.trim().replace(/^\/+|\/+$/g, "").replace(/\\/g, "/").replace(/\/SKILL\.md$/i, "");
}

function resolveCatalogSourcePath(skillId: string, roots: string[], repoOwner: string, repoName: string): string {
  const normalized = normalizeCatalogSourcePath(skillId);
  if (!normalized) return normalized;
  if (roots.some((root) => root === ".")) {
    return ".";
  }
  if (normalized.includes("/")) return normalized;
  if (!roots.length) return resolveCatalogSourcePathByRepoHeuristic(normalized, repoOwner, repoName);

  const exact = roots.find((root) => root.toLowerCase() === normalized.toLowerCase());
  if (exact) return exact;

  const candidates = roots.filter((root) => {
    const base = root.split("/").filter(Boolean).at(-1) ?? "";
    return base.toLowerCase() === normalized.toLowerCase();
  });
  if (!candidates.length) {
    return resolveCatalogSourcePathByRepoHeuristic(normalized, repoOwner, repoName);
  }
  if (candidates.length === 1) return candidates[0];

  candidates.sort((a, b) => {
    const aPreferred = a.toLowerCase().endsWith(`/skills/${normalized.toLowerCase()}`);
    const bPreferred = b.toLowerCase().endsWith(`/skills/${normalized.toLowerCase()}`);
    if (aPreferred !== bPreferred) return aPreferred ? -1 : 1;
    const aDepth = a.split("/").length;
    const bDepth = b.split("/").length;
    if (aDepth !== bDepth) return aDepth - bDepth;
    return a.localeCompare(b, "en", { sensitivity: "base" });
  });
  return candidates[0];
}

function resolveCatalogSourcePathByRepoHeuristic(normalized: string, repoOwner: string, repoName: string): string {
  const owner = repoOwner.trim().toLowerCase();
  const repo = repoName.trim().toLowerCase();
  if (repo === "claude-code") {
    return `plugins/${normalized}/skills/${normalized}`;
  }
  if (owner === "github" && repo === "awesome-copilot") {
    return `skills/${normalized}`;
  }
  return normalized;
}

async function getRepoSkillRoots(
  env: Env,
  platform: "github" | "gitlab",
  owner: string,
  name: string,
  branch: string,
  cache: Map<string, string[]>,
): Promise<string[]> {
  const key = `${platform}:${owner.toLowerCase()}/${name.toLowerCase()}@${branch}`;
  const cached = cache.get(key);
  if (cached) return cached;
  try {
    const roots = platform === "gitlab"
      ? await fetchGitLabSkillRoots(owner, name, branch, env.GITLAB_TOKEN)
      : await fetchGitHubSkillRoots(owner, name, branch, env.GITHUB_TOKEN);
    cache.set(key, roots);
    return roots;
  } catch {
    cache.set(key, []);
    return [];
  }
}

async function getRepoHasRootSkill(
  env: Env,
  platform: "github" | "gitlab",
  owner: string,
  name: string,
  branch: string,
  cache: Map<string, boolean>,
): Promise<boolean> {
  const key = `${platform}:${owner.toLowerCase()}/${name.toLowerCase()}@${branch}`;
  const cached = cache.get(key);
  if (typeof cached === "boolean") return cached;
  const exists = platform === "gitlab"
    ? await fetchGitLabRootSkillExists(owner, name, branch, env.GITLAB_TOKEN)
    : await fetchGitHubRootSkillExists(owner, name, branch, env.GITHUB_TOKEN);
  cache.set(key, exists);
  return exists;
}

async function fetchGitHubSkillRoots(owner: string, name: string, branch: string, token?: string): Promise<string[]> {
  const endpoint = `https://api.github.com/repos/${encodeURIComponent(owner)}/${encodeURIComponent(name)}/git/trees/${encodeURIComponent(branch)}?recursive=1`;
  const headersWithAuth: Record<string, string> = {
    "User-Agent": "aigate-skill-metrics",
    Accept: "application/vnd.github+json",
  };
  const auth = (token ?? "").trim();
  if (auth) {
    headersWithAuth.Authorization = `Bearer ${auth}`;
  }
  let resp = await fetch(endpoint, { headers: headersWithAuth });
  if (!resp.ok && auth && (resp.status === 401 || resp.status === 403 || resp.status === 404)) {
    const headersNoAuth = {
      "User-Agent": "aigate-skill-metrics",
      Accept: "application/vnd.github+json",
    };
    resp = await fetch(endpoint, { headers: headersNoAuth });
  }
  if (!resp.ok) {
    return (await fetchGitHubRootSkillExists(owner, name, branch, token)) ? ["."] : [];
  }
  const payload = (await resp.json()) as { tree?: Array<{ path?: string; type?: string }> };
  const tree = Array.isArray(payload.tree) ? payload.tree : [];
  let hasRootSkill = false;
  const roots = new Set<string>();
  for (const item of tree) {
    const path = (item.path ?? "").trim();
    if (item.type !== "blob") continue;
    const lower = path.toLowerCase();
    if (lower === "skill.md") {
      hasRootSkill = true;
      continue;
    }
    if (!lower.endsWith("/skill.md")) continue;
    const root = path.slice(0, -"/SKILL.md".length).replace(/\/+$/g, "");
    if (root) roots.add(root);
  }
  if (hasRootSkill) return ["."];
  if (roots.size > 0) return [...roots];
  return (await fetchGitHubRootSkillExists(owner, name, branch, token)) ? ["."] : [];
}

async function fetchGitLabSkillRoots(owner: string, name: string, branch: string, token?: string): Promise<string[]> {
  const project = encodeURIComponent(`${owner}/${name}`);
  const endpoint = `https://gitlab.com/api/v4/projects/${project}/repository/tree?ref=${encodeURIComponent(branch)}&recursive=true&per_page=100`;
  const headers: Record<string, string> = { "User-Agent": "aigate-skill-metrics" };
  const auth = (token ?? "").trim();
  if (auth) {
    headers["PRIVATE-TOKEN"] = auth;
  }
  const resp = await fetch(endpoint, { headers });
  if (!resp.ok) {
    return (await fetchGitLabRootSkillExists(owner, name, branch, token)) ? ["."] : [];
  }
  const payload = (await resp.json()) as Array<{ path?: string; type?: string }>;
  let hasRootSkill = false;
  const roots = new Set<string>();
  for (const item of payload) {
    const path = (item.path ?? "").trim();
    if (item.type !== "blob") continue;
    const lower = path.toLowerCase();
    if (lower === "skill.md") {
      hasRootSkill = true;
      continue;
    }
    if (!lower.endsWith("/skill.md")) continue;
    const root = path.slice(0, -"/SKILL.md".length).replace(/\/+$/g, "");
    if (root) roots.add(root);
  }
  if (hasRootSkill) return ["."];
  if (roots.size > 0) return [...roots];
  return (await fetchGitLabRootSkillExists(owner, name, branch, token)) ? ["."] : [];
}

async function fetchGitHubRootSkillExists(owner: string, name: string, branch: string, token?: string): Promise<boolean> {
  const endpoint = `https://raw.githubusercontent.com/${encodeURIComponent(owner)}/${encodeURIComponent(name)}/${encodeURIComponent(branch)}/SKILL.md`;
  const headers: Record<string, string> = { "User-Agent": "aigate-skill-metrics" };
  try {
    const resp = await fetch(endpoint, { method: "HEAD", headers });
    if (resp.ok) return true;
    if (resp.status === 405) {
      const getResp = await fetch(endpoint, { method: "GET", headers });
      return getResp.ok;
    }
    return false;
  } catch {
    return false;
  }
}

async function fetchGitLabRootSkillExists(owner: string, name: string, branch: string, token?: string): Promise<boolean> {
  const endpoint = `https://gitlab.com/${encodeURIComponent(owner)}/${encodeURIComponent(name)}/-/raw/${encodeURIComponent(branch)}/SKILL.md`;
  const headers: Record<string, string> = { "User-Agent": "aigate-skill-metrics" };
  const auth = (token ?? "").trim();
  if (auth) {
    headers["PRIVATE-TOKEN"] = auth;
  }
  try {
    const resp = await fetch(endpoint, { method: "HEAD", headers });
    if (resp.ok) return true;
    if (resp.status === 405) {
      const getResp = await fetch(endpoint, { method: "GET", headers });
      return getResp.ok;
    }
    return false;
  } catch {
    return false;
  }
}

function discoveredSkillId(platform: "github" | "gitlab", owner: string, name: string, branch: string, sourcePath: string): string {
  return `${platform}:${owner}/${name}:${branch}:${sourcePath}`;
}

function buildDiscoveredManagedName(owner: string, name: string, sourcePath: string): string {
  const source = sourcePath.replace(/^\/+|\/+$/g, "").replace(/\\/g, "/");
  const last = source.split("/").filter(Boolean).at(-1) ?? "skill";
  return `${name}-${last}`.toLowerCase().replace(/[^a-z0-9._-]+/g, "-");
}

function sliceCatalog(catalog: SkillCatalog, offset: number, limit: number): SkillCatalog & { total_items: number; offset: number; limit: number } {
  return {
    ...catalog,
    items: catalog.items.slice(offset, offset + limit),
    total_items: catalog.items.length,
    offset,
    limit,
  };
}

function filterAndSliceCatalog(
  catalog: SkillCatalog,
  query: string,
  offset: number,
  limit: number,
  recommended: string[],
): { indexedTotal: number; total: number; items: CatalogSkillItem[] } {
  const indexedTotal = catalog.items.length;
  const q = query.trim().toLowerCase();
  const source = q
    ? catalog.items.filter((item) =>
      [item.name, item.repo_owner, item.repo_name, item.source_path].join(" ").toLowerCase().includes(q),
    )
    : catalog.items.slice();
  const recommendedOrder = new Map<string, number>();
  for (let i = 0; i < recommended.length; i += 1) {
    const name = recommended[i]?.trim().toLowerCase();
    if (!name || recommendedOrder.has(name)) continue;
    recommendedOrder.set(name, i);
  }
  source.sort((a, b) => {
    const leftKey = a.name.trim().toLowerCase();
    const rightKey = b.name.trim().toLowerCase();
    const leftRank = recommendedOrder.get(leftKey);
    const rightRank = recommendedOrder.get(rightKey);
    if (leftRank !== undefined || rightRank !== undefined) {
      if (leftRank === undefined) return 1;
      if (rightRank === undefined) return -1;
      if (leftRank !== rightRank) return leftRank - rightRank;
    }
    const byName = a.name.localeCompare(b.name, "en", { sensitivity: "base" });
    if (byName !== 0) return byName;
    return a.id.localeCompare(b.id, "en", { sensitivity: "base" });
  });
  return {
    indexedTotal,
    total: source.length,
    items: source.slice(offset, offset + limit),
  };
}

function normalizeTrackedRepo(raw: Partial<TrackedRepoItem> & Record<string, unknown>, fallbackOrder: number): TrackedRepoItem {
  let platform = normalizePlatform(raw.platform ?? "github");
  let owner = trim(raw.owner ?? "", 120);
  let name = trim(raw.name ?? "", 120);
  let branch = trim(raw.branch ?? "main", 120) || "main";
  const enabled = raw.enabled !== false;
  const sortOrder = Number.isFinite(raw.sort_order) ? Math.max(0, Math.floor(raw.sort_order as number)) : fallbackOrder;

  const candidates = [
    typeof raw.repo === "string" ? raw.repo : "",
    typeof raw.repo_url === "string" ? raw.repo_url : "",
    typeof raw.url === "string" ? raw.url : "",
    typeof raw.full_name === "string" ? raw.full_name : "",
    name ? "" : owner,
  ];
  for (const candidate of candidates) {
    const parsed = parseRepoReference(candidate);
    if (!parsed) continue;
    if (!owner) owner = trim(parsed.owner, 120);
    if (!name) name = trim(parsed.name, 120);
    if ((branch === "main" || !branch) && parsed.branch) {
      branch = trim(parsed.branch, 120) || branch;
    }
    if (!platform && parsed.platform) {
      platform = parsed.platform;
    }
    if (owner && name) {
      break;
    }
  }

  platform = platform ?? "github";
  if (!platform || !owner || !name) {
    throw httpError(400, "invalid_repo_item");
  }
  return { platform, owner, name, branch, enabled, sort_order: sortOrder };
}

function parseRepoReference(input: string): { platform?: "github" | "gitlab"; owner: string; name: string; branch?: string } | null {
  const value = input.trim();
  if (!value) return null;

  if (value.startsWith("http://") || value.startsWith("https://")) {
    try {
      const parsed = new URL(value);
      const host = parsed.host.toLowerCase();
      const segments = parsed.pathname.split("/").filter(Boolean).map((part) => decodeURIComponent(part));
      if (host.includes("github.com")) {
        if (segments.length < 2) return null;
        const owner = segments[0] ?? "";
        const name = normalizeRepoName(segments[1] ?? "");
        let branch = "";
        if (segments[2] === "tree" && segments.length >= 4) {
          branch = normalizeRepoBranch(segments.slice(3).join("/"));
        }
        return { platform: "github", owner, name, branch: branch || undefined };
      }
      if (host.includes("gitlab.com")) {
        const marker = segments.indexOf("-");
        const repoParts = marker >= 0 ? segments.slice(0, marker) : segments;
        if (repoParts.length < 2) return null;
        const owner = repoParts.slice(0, -1).join("/");
        const name = normalizeRepoName(repoParts[repoParts.length - 1] ?? "");
        let branch = "";
        if (marker >= 0 && segments[marker + 1] === "tree" && segments.length > marker + 2) {
          branch = normalizeRepoBranch(segments.slice(marker + 2).join("/"));
        }
        return { platform: "gitlab", owner, name, branch: branch || undefined };
      }
      return null;
    } catch {
      return null;
    }
  }

  const shorthand = value.replace(/^(github|gitlab):/i, "");
  const parts = shorthand.split("/").filter(Boolean);
  if (parts.length < 2) return null;
  return {
    platform: /^gitlab:/i.test(value) ? "gitlab" : /^github:/i.test(value) ? "github" : undefined,
    owner: parts.slice(0, -1).join("/"),
    name: normalizeRepoName(parts[parts.length - 1] ?? ""),
  };
}

function normalizeRepoName(name: string): string {
  return name.trim().replace(/\.git$/i, "");
}

function normalizeRepoBranch(branch: string): string {
  return decodeURIComponent(branch).trim().replace(/^\/+|\/+$/g, "");
}

async function fetchGitHubDefaultBranch(owner: string, name: string, token?: string): Promise<string> {
  const endpoint = `https://api.github.com/repos/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`;
  const headers: Record<string, string> = {
    "User-Agent": "aigate-skill-metrics",
    Accept: "application/vnd.github+json",
  };
  const auth = (token ?? "").trim();
  if (auth) {
    headers.Authorization = `Bearer ${auth}`;
  }
  try {
    const resp = await fetch(endpoint, { headers });
    if (!resp.ok) return "";
    const payload = (await resp.json()) as { default_branch?: string };
    return normalizeRepoBranch(payload.default_branch ?? "");
  } catch {
    return "";
  }
}

async function fetchGitLabDefaultBranch(owner: string, name: string, token?: string): Promise<string> {
  const project = encodeURIComponent(`${owner}/${name}`);
  const endpoint = `https://gitlab.com/api/v4/projects/${project}`;
  const headers: Record<string, string> = { "User-Agent": "aigate-skill-metrics" };
  const auth = (token ?? "").trim();
  if (auth) {
    headers["PRIVATE-TOKEN"] = auth;
  }
  try {
    const resp = await fetch(endpoint, { headers });
    if (!resp.ok) return "";
    const payload = (await resp.json()) as { default_branch?: string };
    return normalizeRepoBranch(payload.default_branch ?? "");
  } catch {
    return "";
  }
}

function repoKey(platform: "github" | "gitlab", owner: string, name: string): string {
  return `${platform}:${owner.toLowerCase()}/${name.toLowerCase()}`;
}

function normalizePlatform(raw?: string | null): "github" | "gitlab" | null {
  switch ((raw ?? "").toLowerCase()) {
    case "gitlab":
      return "gitlab";
    case "github":
      return "github";
    default:
      return null;
  }
}

function normalizeDay(input?: string | null): string | null {
  if (!input) return null;
  const value = input.trim();
  if (!value) return null;
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return value;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return null;
  return parsed.toISOString().slice(0, 10);
}

function normalizeActiveWindowDays(input?: string | null): number {
  const value = (input ?? "").trim().toLowerCase();
  if (!value) return 1;
  if (value === "1d" || value === "day" || value === "daily") return 1;
  if (value === "7d" || value === "1w" || value === "week" || value === "weekly") return 7;
  if (value === "30d" || value === "1m" || value === "month" || value === "monthly") return 30;
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return 1;
  const days = Math.floor(parsed);
  if (days <= 1) return 1;
  if (days <= 7) return 7;
  return 30;
}

function normalizeCurveDays(input?: string | null): number {
  const value = (input ?? "").trim().toLowerCase();
  if (value === "14d" || value === "14") return 14;
  if (value === "30d" || value === "30" || value === "") return 30;
  if (value === "60d" || value === "60") return 60;
  if (value === "90d" || value === "90") return 90;
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return 30;
  const days = Math.floor(parsed);
  if (days <= 14) return 14;
  if (days <= 30) return 30;
  if (days <= 60) return 60;
  return 90;
}

function activeWindowLabel(days: number): string {
  if (days <= 1) return "1天";
  if (days <= 7) return "1周";
  return "1月";
}

function normalizeLimit(input?: string | null): number {
  const parsed = Number(input ?? DEFAULT_LIMIT);
  if (!Number.isFinite(parsed) || parsed <= 0) return DEFAULT_LIMIT;
  return Math.min(Math.floor(parsed), MAX_LIMIT);
}

function normalizeUsersLimit(input?: string | null): number {
  const parsed = Number(input ?? 100);
  if (!Number.isFinite(parsed) || parsed <= 0) return 100;
  return Math.min(Math.floor(parsed), 500);
}

function normalizeUsersOffset(input?: string | null): number {
  const parsed = Number(input ?? 0);
  if (!Number.isFinite(parsed) || parsed <= 0) return 0;
  return Math.floor(parsed);
}

function normalizeUserSort(input?: string | null): "last_seen_desc" | "installs_desc" {
  const value = (input ?? "").trim().toLowerCase();
  if (value === "installs_desc") return "installs_desc";
  return "last_seen_desc";
}

function normalizeCatalogLimit(input?: string | null): number {
  const parsed = Number(input ?? 300);
  if (!Number.isFinite(parsed) || parsed <= 0) return 300;
  return Math.min(Math.floor(parsed), 2000);
}

function normalizeCatalogOffset(input?: string | null): number {
  const parsed = Number(input ?? 0);
  if (!Number.isFinite(parsed) || parsed <= 0) return 0;
  return Math.floor(parsed);
}

function normalizeScanBatchSize(input?: string | null): number {
  const parsed = Number(input ?? SCAN_DEFAULT_BATCH_SIZE);
  if (!Number.isFinite(parsed) || parsed <= 0) return SCAN_DEFAULT_BATCH_SIZE;
  return Math.min(Math.floor(parsed), 50);
}

function utcDay(): string {
  return new Date().toISOString().slice(0, 10);
}

function utcDayOffset(offsetDays: number): string {
  const now = new Date();
  const utc = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() + offsetDays);
  return new Date(utc).toISOString().slice(0, 10);
}

function trim(value: string, max: number): string {
  return value.trim().slice(0, max);
}

function cacheKey(day: string, limit: number): string {
  return `ranking:skills:${day}:${limit}`;
}

function json(payload: unknown, status = 200, extraHeaders?: Record<string, string>): Response {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "content-type": "application/json; charset=utf-8", ...CORS_HEADERS, ...(extraHeaders ?? {}) },
  });
}

function html(content: string): Response {
  return new Response(content, { status: 200, headers: { "content-type": "text/html; charset=utf-8" } });
}

function httpError(status: number, message: string): Error & { status: number } {
  const err = new Error(message) as Error & { status: number };
  err.status = status;
  return err;
}

async function sha256Hex(input: string): Promise<string> {
  const raw = new TextEncoder().encode(input);
  const digest = await crypto.subtle.digest("SHA-256", raw);
  return [...new Uint8Array(digest)].map((n) => n.toString(16).padStart(2, "0")).join("");
}

async function hmacHex(secret: string, input: string): Promise<string> {
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const sig = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(input));
  return [...new Uint8Array(sig)].map((n) => n.toString(16).padStart(2, "0")).join("");
}

function getCookie(request: Request, name: string): string {
  const header = request.headers.get("cookie") ?? "";
  const chunks = header.split(";").map((item) => item.trim());
  for (const chunk of chunks) {
    const [k, ...rest] = chunk.split("=");
    if (k === name) {
      return decodeURIComponent(rest.join("="));
    }
  }
  return "";
}

function toBase64Url(input: string): string {
  const bytes = new TextEncoder().encode(input);
  let binary = "";
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function fromBase64Url(input: string): string {
  const normalized = input.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized + "=".repeat((4 - (normalized.length % 4)) % 4);
  const binary = atob(padded);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
  return new TextDecoder().decode(bytes);
}

function timingSafeEq(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i += 1) {
    diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  }
  return diff === 0;
}

async function issueSessionToken(env: Env): Promise<string> {
  const secret = trim(env.ADMIN_SESSION_SECRET ?? "", 256);
  if (!secret) {
    throw httpError(503, "session_secret_not_configured");
  }
  const payload = JSON.stringify({ exp: Math.floor(Date.now() / 1000) + SESSION_TTL_SECONDS });
  const payloadB64 = toBase64Url(payload);
  const sig = await hmacHex(secret, payloadB64);
  return `${payloadB64}.${sig}`;
}

async function verifySessionToken(token: string, env: Env): Promise<boolean> {
  const secret = trim(env.ADMIN_SESSION_SECRET ?? "", 256);
  if (!secret) return false;
  const [payloadB64 = "", sig = ""] = token.split(".", 2);
  if (!payloadB64 || !sig) return false;
  const expectedSig = await hmacHex(secret, payloadB64);
  if (!timingSafeEq(sig, expectedSig)) return false;
  let parsed: { exp?: number } = {};
  try {
    parsed = JSON.parse(fromBase64Url(payloadB64)) as { exp?: number };
  } catch {
    return false;
  }
  const exp = Number(parsed.exp ?? 0);
  return Number.isFinite(exp) && exp > Math.floor(Date.now() / 1000);
}

async function isAdminAuthorized(request: Request, env: Env): Promise<boolean> {
  const auth = request.headers.get("authorization")?.trim() ?? "";
  const adminToken = trim(env.TRACKED_REPOS_ADMIN_TOKEN ?? "", 256);
  if (adminToken && auth.toLowerCase().startsWith("bearer ")) {
    const current = auth.slice(7).trim();
    if (current && timingSafeEq(current, adminToken)) {
      return true;
    }
  }
  const cookieToken = getCookie(request, SESSION_COOKIE_NAME);
  if (!cookieToken) return false;
  return verifySessionToken(cookieToken, env);
}

async function assertAdmin(request: Request, env: Env): Promise<void> {
  if (!(await isAdminAuthorized(request, env))) {
    throw httpError(401, "unauthorized");
  }
}

async function loginAdmin(request: Request, env: Env): Promise<Response> {
  const passwordExpected = trim(env.ADMIN_UI_PASSWORD ?? "", 256);
  if (!passwordExpected) {
    throw httpError(503, "admin_ui_password_not_configured");
  }
  const contentType = (request.headers.get("content-type") ?? "").toLowerCase();
  let password = "";
  if (contentType.includes("application/json")) {
    const payload = (await request.json()) as { password?: string };
    password = trim(payload.password ?? "", 256);
  } else {
    const form = await request.formData();
    password = trim(String(form.get("password") ?? ""), 256);
  }
  if (!password || !timingSafeEq(password, passwordExpected)) {
    throw httpError(401, "invalid_password");
  }
  const token = await issueSessionToken(env);
  return json(
    { ok: true },
    200,
    {
      "set-cookie": `${SESSION_COOKIE_NAME}=${encodeURIComponent(token)}; Path=/; HttpOnly; Secure; SameSite=Lax; Max-Age=${SESSION_TTL_SECONDS}`,
    },
  );
}

function logoutAdmin(): Response {
  return json(
    { ok: true },
    200,
    {
      "set-cookie": `${SESSION_COOKIE_NAME}=; Path=/; HttpOnly; Secure; SameSite=Lax; Max-Age=0`,
    },
  );
}

async function assertBearer(request: Request, token?: string, options?: { required?: boolean }): Promise<void> {
  if (!token) {
    if (options?.required) {
      throw httpError(503, "bearer_token_not_configured");
    }
    return;
  }
  const auth = request.headers.get("authorization")?.trim() ?? "";
  if (!auth.toLowerCase().startsWith("bearer ")) {
    throw httpError(401, "missing_bearer_token");
  }
  const current = auth.slice(7).trim();
  if (!current || current !== token) {
    throw httpError(401, "invalid_bearer_token");
  }
}

function renderAdminPage(initialAuthenticated: boolean): string {
  return `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>AI Gate Skill Admin</title>
    <style>
      :root { color-scheme: light; --bg:#f5f7fb; --panel:#fff; --fg:#111827; --muted:#6b7280; --line:#e5e7eb; --accent:#0f766e; --danger:#b91c1c; }
      * { box-sizing: border-box; }
      body { margin:0; font-family: ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background:var(--bg); color:var(--fg); }
      .wrap { max-width: 1200px; margin: 0 auto; padding: 24px; }
      .panel { background:var(--panel); border:1px solid var(--line); border-radius: 12px; padding:16px; margin-bottom:16px; }
      h1,h2 { margin:0 0 12px; font-weight:600; }
      h1 { font-size: 20px; }
      h2 { font-size: 16px; }
      .row { display:flex; gap:12px; flex-wrap:wrap; align-items:center; }
      input,select,button { height:36px; border:1px solid var(--line); border-radius:8px; padding:0 10px; background:#fff; }
      button { cursor:pointer; background:#f8fafc; }
      button.primary { background:var(--accent); color:#fff; border-color:var(--accent); }
      button.danger { background:#fff; color:var(--danger); border-color:#fecaca; }
      table { width:100%; border-collapse: collapse; font-size:13px; }
      th,td { border-bottom:1px solid var(--line); text-align:left; padding:8px 6px; vertical-align: top; }
      th { color:var(--muted); font-weight:600; }
      .muted { color:var(--muted); font-size:12px; }
      .hidden { display:none !important; }
      .stats { display:grid; grid-template-columns: repeat(auto-fit,minmax(180px,1fr)); gap:12px; }
      .stat { border:1px solid var(--line); border-radius:10px; padding:10px 12px; background:#fff; }
      .stat b { font-size:20px; display:block; margin-top:6px; }
      .right { margin-left:auto; }
      .mono { font-family: ui-monospace, Menlo, Monaco, Consolas, monospace; font-size:12px; }
      .tag { display:inline-block; border:1px solid var(--line); border-radius:999px; padding:2px 8px; font-size:12px; color:var(--muted); }
      .tabs { display:flex; gap:8px; flex-wrap:wrap; margin-bottom:12px; }
      .tab { border:1px solid var(--line); background:#fff; color:#374151; border-radius:999px; padding:6px 12px; cursor:pointer; }
      .tab.active { background:#111827; color:#fff; border-color:#111827; }
      .tab-panel { display:none; }
      .tab-panel.active { display:block; }
      .table-wrap { max-height: 420px; overflow:auto; border:1px solid var(--line); border-radius:10px; }
      .list-wrap { max-height: 520px; overflow:auto; border:1px solid var(--line); border-radius:10px; padding:8px; background:#fff; }
      .user-card { border:1px solid var(--line); border-radius:10px; padding:10px 12px; margin-bottom:8px; background:#fff; }
      .user-card:last-child { margin-bottom:0; }
      .user-card .title { display:flex; align-items:center; justify-content:space-between; gap:12px; margin-bottom:6px; }
      .user-card .title b { font-size:13px; }
      .kv { display:grid; grid-template-columns: 130px 1fr; gap:6px 10px; font-size:12px; }
      .kv .k { color:var(--muted); }
      .detail-panel { border:1px solid var(--line); border-radius:10px; padding:12px; background:#fff; }
      .progress-wrap { margin-top:10px; }
      .progress-track { width:100%; height:10px; border-radius:999px; background:#e5e7eb; overflow:hidden; }
      .progress-bar { height:100%; width:0%; background:linear-gradient(90deg,#0f766e,#14b8a6); transition:width .25s ease; }
      .progress-meta { margin-top:8px; display:flex; gap:12px; flex-wrap:wrap; color:var(--muted); font-size:12px; }
      .audit-pill { display:inline-flex; align-items:center; border-radius:999px; border:1px solid var(--line); padding:1px 8px; font-size:11px; margin-right:4px; margin-bottom:4px; }
      .audit-pill.is-pass { color:#15803d; border-color:#86efac; background:#f0fdf4; }
      .audit-pill.is-warn { color:#b45309; border-color:#fcd34d; background:#fffbeb; }
      .audit-pill.is-fail { color:#b91c1c; border-color:#fca5a5; background:#fef2f2; }
      .audit-pill.is-info { color:#0369a1; border-color:#bae6fd; background:#f0f9ff; }
      .audit-pill.is-unknown { color:#6b7280; border-color:#d1d5db; background:#f3f4f6; }
      .curve-wrap { margin-top:12px; border:1px solid var(--line); border-radius:10px; padding:12px; background:#fff; }
      .curve-head { display:flex; gap:12px; align-items:center; justify-content:space-between; margin-bottom:8px; flex-wrap:wrap; }
      .curve-legend { display:flex; gap:12px; align-items:center; flex-wrap:wrap; }
      .curve-legend .item { display:inline-flex; align-items:center; gap:6px; font-size:12px; color:var(--muted); }
      .curve-legend .dot { width:10px; height:10px; border-radius:999px; display:inline-block; }
      .curve-legend .dot.total { background:#0f766e; }
      .curve-legend .dot.daily { background:#2563eb; }
      .curve-chart { width:100%; height:220px; display:block; background:linear-gradient(to top, #fafafa, #fff); border:1px solid var(--line); border-radius:8px; }
    </style>
  </head>
  <body>
    <div class="wrap">
      <div class="panel">
        <div class="row">
          <h1>Skill 管理后台</h1>
          <span class="tag">极简版</span>
          <button id="logoutBtn" class="right hidden">退出登录</button>
        </div>
        <div id="loginHint" class="muted">受鉴权保护，仅已登录管理员可访问数据。</div>
      </div>

      <div id="loginPanel" class="panel ${initialAuthenticated ? "hidden" : ""}">
        <h2>登录</h2>
        <div class="row">
          <input id="password" type="password" placeholder="输入管理员密码" style="min-width:280px" />
          <button id="loginBtn" class="primary">登录</button>
          <span id="loginMsg" class="muted"></span>
        </div>
      </div>

      <div id="app" class="${initialAuthenticated ? "" : "hidden"}">
        <div class="panel">
          <div class="tabs">
            <button class="tab active" data-tab="overview">概览</button>
            <button class="tab" data-tab="repos">仓库</button>
            <button class="tab" data-tab="catalog">Skills 列表</button>
            <button class="tab" data-tab="ranking">排行榜</button>
            <button class="tab" data-tab="users">用户</button>
          </div>
        </div>

        <div class="panel tab-panel active" id="tab-overview">
          <h2>总览统计</h2>
          <div class="row" style="margin-bottom:12px">
            <span class="muted">活跃周期</span>
            <select id="activeWindow">
              <option value="1d">1天</option>
              <option value="7d">1周</option>
              <option value="30d">1月</option>
            </select>
            <button id="refreshOverviewBtn">刷新总览</button>
            <span class="muted" id="overviewHint">-</span>
          </div>
          <div class="stats">
            <div class="stat"><span class="muted">累计匿名用户</span><b id="totalUsers">-</b></div>
            <div class="stat"><span class="muted" id="activeUsersLabel">活跃用户（1天）</span><b id="activeUsers">-</b></div>
            <div class="stat"><span class="muted">活跃用户（7天）</span><b id="activeUsers7d">-</b></div>
            <div class="stat"><span class="muted">活跃用户（30天）</span><b id="activeUsers30d">-</b></div>
            <div class="stat"><span class="muted" id="newUsersLabel">窗口新增用户</span><b id="newUsersWindow">-</b></div>
            <div class="stat"><span class="muted">仓库总数</span><b id="repoCount">-</b></div>
            <div class="stat"><span class="muted">启用仓库数</span><b id="repoEnabledCount">-</b></div>
            <div class="stat"><span class="muted">已收录技能数</span><b id="skillCount">-</b></div>
            <div class="stat"><span class="muted">排行榜技能数（当日）</span><b id="rankingCount">-</b></div>
            <div class="stat"><span class="muted">安装事件总数</span><b id="installEventCount">-</b></div>
            <div class="stat"><span class="muted">扫描成功率</span><b id="scanSuccessRate">-</b></div>
            <div class="stat"><span class="muted">扫描平均耗时</span><b id="scanAvgDuration">-</b></div>
            <div class="stat"><span class="muted">目录版本</span><b class="mono" id="catalogVersion">-</b></div>
          </div>
          <div class="curve-wrap">
            <div class="curve-head">
              <div class="curve-legend">
                <span class="item"><span class="dot total"></span>累计用户数</span>
                <span class="item"><span class="dot daily"></span>日活用户数</span>
              </div>
              <div class="row">
                <span class="muted">曲线周期</span>
                <select id="userCurveDays">
                  <option value="14">14天</option>
                  <option value="30" selected>30天</option>
                  <option value="60">60天</option>
                  <option value="90">90天</option>
                </select>
                <span class="muted" id="userCurveHint">-</span>
              </div>
            </div>
            <div id="userCurveChart" class="curve-chart" role="img" aria-label="用户趋势曲线"></div>
          </div>
          <div class="row" style="margin-top:12px">
            <button id="startScanBtn" class="primary">手动触发扫描</button>
            <button id="stopScanBtn">停止扫描</button>
            <button id="retryFailedScanBtn">失败重试</button>
            <button id="stepScanBtn">继续扫描一步</button>
            <button id="runExternalSyncBtn">立即执行外部同步</button>
            <select id="scanBatchSize">
              <option value="6">批次 6</option>
              <option value="12" selected>批次 12</option>
              <option value="24">批次 24</option>
            </select>
            <span class="muted" id="scanProgress">未开始</span>
          </div>
          <div class="progress-wrap">
            <div class="progress-track">
              <div class="progress-bar" id="scanProgressBar"></div>
            </div>
            <div class="progress-meta">
              <span id="scanProgressPct">0%</span>
              <span id="scanProgressCount">0/0</span>
              <span id="scanProgressSuccess">成功 0</span>
              <span id="scanProgressFailed">失败 0</span>
            </div>
          </div>
          <div class="row" style="margin-top:12px"><span class="muted">扫描失败明细</span></div>
          <div class="table-wrap" style="max-height:220px">
            <table>
              <thead><tr><th>仓库</th><th>错误</th></tr></thead>
              <tbody id="scanFailureRows"></tbody>
            </table>
          </div>
        </div>

        <div class="panel tab-panel" id="tab-repos">
          <h2>仓库追踪管理</h2>
          <div class="row" style="margin-bottom:10px">
            <input id="repoInput" placeholder="仓库 URL 或 owner/repo（例如 https://github.com/iOfficeAI/OfficeCLI.git）" style="min-width:520px;max-width:100%" />
            <button id="addRepoBtn" class="primary">新增 / 更新</button>
            <button id="refreshReposBtn">刷新仓库</button>
          </div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>仓库</th><th>分支</th><th>顺序</th><th>Skills 数</th><th>Star</th><th>启用</th><th>更新时间</th><th>操作</th></tr></thead>
              <tbody id="repoRows"></tbody>
            </table>
          </div>
          <div class="muted" id="catalogFetchedAt" style="margin-top:8px">技能目录缓存时间: -</div>
        </div>

        <div class="panel tab-panel" id="tab-catalog">
          <h2>最终 Skills 列表（提供给客户端）</h2>
          <div class="row" style="margin-bottom:10px">
            <button id="refreshCatalogBtn">刷新并重建目录</button>
            <span class="muted">按技能名称排序，含来源仓库和原始链接</span>
          </div>
          <div class="row" style="margin-bottom:10px">
            <input id="catalogQuery" placeholder="按名称/仓库/路径搜索（回车触发）" style="min-width:360px;max-width:100%" />
            <input id="catalogOffset" type="number" value="0" style="width:120px" />
            <input id="catalogLimit" type="number" value="300" style="width:120px" />
            <button id="loadCatalogPageBtn">加载分页</button>
            <span class="muted" id="catalogPageInfo"></span>
          </div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Skill</th><th>来源仓库</th><th>分支</th><th>路径</th><th>Audits</th><th>链接</th></tr></thead>
              <tbody id="catalogRows"></tbody>
            </table>
          </div>
        </div>

        <div class="panel tab-panel" id="tab-ranking">
          <h2>技能排行榜</h2>
          <div class="row" style="margin-bottom:10px">
            <input id="rankDay" placeholder="YYYY-MM-DD (可空)" />
            <input id="rankLimit" type="number" value="50" style="width:100px" />
            <button id="refreshRankBtn">刷新</button>
          </div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Skill</th><th>来源</th><th>安装次数</th><th>用户数</th></tr></thead>
              <tbody id="rankRows"></tbody>
            </table>
          </div>
        </div>

        <div class="panel tab-panel" id="tab-users">
          <h2>用户安装技能</h2>
          <div class="row" style="margin-bottom:10px">
            <input id="userQuery" placeholder="按匿名ID/哈希搜索（前缀）" style="min-width:260px;max-width:100%" />
            <select id="userSort">
              <option value="last_seen_desc">按最近活跃</option>
              <option value="installs_desc">按安装数</option>
            </select>
            <input id="userOffset" type="number" value="0" style="width:120px" />
            <input id="userLimit" type="number" value="100" style="width:120px" />
            <button id="refreshUsersBtn">刷新用户列表</button>
            <span class="muted" id="usersPageInfo">默认列表，点击查看详情</span>
          </div>
          <div class="row" style="align-items:flex-start">
            <div style="flex:1; min-width:360px">
              <div class="list-wrap" id="userList"></div>
            </div>
            <div style="flex:1; min-width:360px">
              <div class="detail-panel">
                <div class="mono muted" id="userSkillTitle">选择左侧用户</div>
                <div class="kv" id="userDetailKv" style="margin-top:8px">
                  <div class="k">匿名ID</div><div>-</div>
                  <div class="k">用户哈希</div><div>-</div>
                  <div class="k">最近活跃</div><div>-</div>
                  <div class="k">首次活跃</div><div>-</div>
                  <div class="k">安装技能数</div><div>-</div>
                  <div class="k">总事件数</div><div>-</div>
                  <div class="k">活跃天数</div><div>-</div>
                  <div class="k">客户端</div><div>-</div>
                  <div class="k">系统</div><div>-</div>
                </div>
              </div>
              <div class="row" style="margin:8px 0">
                <input id="userSkillQuery" placeholder="筛选 skill 名称" style="min-width:220px;max-width:100%" />
                <input id="userSkillOffset" type="number" value="0" style="width:100px" />
                <input id="userSkillLimit" type="number" value="100" style="width:100px" />
                <button id="refreshUserSkillBtn">刷新明细</button>
              </div>
              <div class="table-wrap">
                <table>
                  <thead><tr><th>Skill</th><th>来源</th><th>次数</th><th>最近时间</th></tr></thead>
                  <tbody id="userSkillRows"></tbody>
                </table>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
    <script src="https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js"></script>
    <script>
      const state = { authenticated: ${initialAuthenticated ? "true" : "false"}, selectedUser: "" };
      const byId = (id) => document.getElementById(id);
      let userCurveChart = null;

      async function request(path, options) {
        const res = await fetch(path, { credentials: "include", ...options });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(data.error || ("http_" + res.status));
        return data;
      }

      function toggleAuthUI() {
        byId("loginPanel").classList.toggle("hidden", state.authenticated);
        byId("app").classList.toggle("hidden", !state.authenticated);
        byId("logoutBtn").classList.toggle("hidden", !state.authenticated);
      }

      function setupTabs() {
        document.querySelectorAll(".tab").forEach((tabBtn) => {
          tabBtn.addEventListener("click", () => {
            const tab = tabBtn.getAttribute("data-tab");
            document.querySelectorAll(".tab").forEach((el) => el.classList.toggle("active", el === tabBtn));
            document.querySelectorAll(".tab-panel").forEach((panel) => panel.classList.toggle("active", panel.id === ("tab-" + tab)));
          });
        });
      }

      async function loadSession() {
        try {
          const data = await request("/admin/session");
          state.authenticated = !!data.authenticated;
          toggleAuthUI();
        } catch {}
      }

      async function login() {
        const password = byId("password").value.trim();
        if (!password) return;
        byId("loginMsg").textContent = "登录中...";
        try {
          await request("/admin/login", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ password }) });
          state.authenticated = true;
          toggleAuthUI();
          byId("loginMsg").textContent = "登录成功";
          await loadAll();
        } catch (e) {
          byId("loginMsg").textContent = "登录失败: " + e.message;
        }
      }

      async function logout() {
        await request("/admin/logout", { method: "POST" });
        state.authenticated = false;
        toggleAuthUI();
      }

      async function loadStats() {
        const window = byId("activeWindow").value || "1d";
        const curveDays = byId("userCurveDays").value || "30";
        const query = new URLSearchParams({ window, curve_days: curveDays });
        const s = await request("/admin/api/stats/overview?" + query.toString());
        byId("totalUsers").textContent = s.total_users || 0;
        byId("activeUsers").textContent = s.active_users || 0;
        byId("activeUsers7d").textContent = s.active_users_7d || 0;
        byId("activeUsers30d").textContent = s.active_users_30d || 0;
        byId("newUsersWindow").textContent = s.new_users_window || 0;
        byId("activeUsersLabel").textContent = "活跃用户（" + (s.active_window_label || "1天") + "）";
        byId("newUsersLabel").textContent = (s.active_window_label || "1天") + "内新增用户";
        byId("repoCount").textContent = s.repos_total || 0;
        byId("repoEnabledCount").textContent = s.repos_enabled || 0;
        byId("skillCount").textContent = s.skills_total || 0;
        byId("rankingCount").textContent = s.ranking_skills_total || 0;
        byId("installEventCount").textContent = s.total_install_events || 0;
        byId("scanSuccessRate").textContent = (s.scan_success_rate || 0) + "%";
        byId("scanAvgDuration").textContent = formatDuration(s.scan_avg_duration_ms || 0);
        byId("catalogVersion").textContent = s.catalog_version || "-";
        byId("overviewHint").textContent = "目录缓存: " + (s.catalog_fetched_at || "-") + " | 排行榜日期: " + (s.ranking_day || "-");
        byId("userCurveHint").textContent = "周期 " + (s.curve_days || curveDays) + " 天";
        renderUserCurve(Array.isArray(s.user_curve) ? s.user_curve : []);
      }

      async function loadCatalog(forceRefresh, customOffset, customLimit) {
        const offset = typeof customOffset === "number" ? customOffset : Number(byId("catalogOffset").value || 0);
        const limit = typeof customLimit === "number" ? customLimit : Number(byId("catalogLimit").value || 300);
        const q = (byId("catalogQuery").value || "").trim();
        const query = new URLSearchParams();
        if (forceRefresh) query.set("refresh", "1");
        query.set("offset", String(offset));
        query.set("limit", String(limit));
        if (q) query.set("q", q);
        const data = await request("/admin/api/skills/final?" + query.toString());
        byId("catalogFetchedAt").textContent = "技能目录缓存时间: " + (data.fetched_at || "-");
        const indexed = typeof data.indexed_total === "number" ? "，索引总量 " + data.indexed_total : "";
        byId("catalogPageInfo").textContent = "总计 " + (data.total_items || 0) + indexed + "，当前 offset=" + (data.offset || 0) + "，limit=" + (data.limit || 0);

        const tbody = byId("repoRows");
        tbody.innerHTML = "";
        for (const item of data.repos || []) {
          const key = encodeURIComponent(item.platform + ":" + item.owner + "/" + item.name);
          const tr = document.createElement("tr");
          tr.innerHTML = "<td><b>" + item.owner + "/" + item.name + "</b><div class='muted'>" + item.platform + "</div></td>" +
            "<td>" + item.branch + "</td>" +
            "<td>" + item.sort_order + "</td>" +
            "<td>" + (item.skill_count || 0) + (item.error ? "<div class='muted'>error: " + item.error + "</div>" : "") + "</td>" +
            "<td>" + ((item.star_count || 0).toLocaleString()) + "</td>" +
            "<td>" + (item.enabled ? "yes" : "no") + "</td>" +
            "<td class='mono'>" + (item.updated_at || "") + "</td>" +
            "<td><button data-key='" + key + "' class='danger'>删除</button></td>";
          tbody.appendChild(tr);
        }
        tbody.querySelectorAll("button[data-key]").forEach((btn) => {
          btn.addEventListener("click", async () => {
            await request("/admin/api/tracked-repos/" + btn.getAttribute("data-key"), { method: "DELETE" });
            await loadCatalog(true);
          });
        });

        const skillTbody = byId("catalogRows");
        skillTbody.innerHTML = "";
        for (const item of data.items || []) {
          const providers = Array.isArray(item?.audits_summary?.providers) ? item.audits_summary.providers : [];
          const auditsHtml = providers.length > 0
            ? providers.map((provider) => {
                const label = provider?.label || provider?.provider || "Audit";
                const status = provider?.status || "info";
                const url = provider?.url ? String(provider.url) : "";
                if (url) {
                  return "<a class='audit-pill is-" + status + "' href='" + url + "' target='_blank' rel='noreferrer'>" + label + "</a>";
                }
                return "<span class='audit-pill is-" + status + "'>" + label + "</span>";
              }).join("")
            : "<span class='audit-pill is-unknown'>未知</span>";
          const tr = document.createElement("tr");
          tr.innerHTML = "<td>" + item.name + "</td>" +
            "<td class='mono'>" + item.repo_owner + "/" + item.repo_name + "</td>" +
            "<td>" + item.branch + "</td>" +
            "<td class='mono'>" + item.source_path + "</td>" +
            "<td>" + auditsHtml + "</td>" +
            "<td><a href='" + item.source_url + "' target='_blank' rel='noreferrer'>查看</a></td>";
          skillTbody.appendChild(tr);
        }
      }

      async function updateScanStatus() {
        const data = await request("/admin/api/scan/status");
        if (!data.running && !data.started_at) {
          byId("scanProgress").textContent = "未开始";
          byId("scanProgressBar").style.width = "0%";
          byId("scanProgressPct").textContent = "0%";
          byId("scanProgressCount").textContent = "0/0";
          byId("scanProgressSuccess").textContent = "成功 0";
          byId("scanProgressFailed").textContent = "失败 0";
          byId("scanFailureRows").innerHTML = "";
          return;
        }
        const total = Number(data.total_repos || 0);
        const processed = Number(data.processed_repos || 0);
        const percent = total > 0 ? Math.min(100, Math.round((processed / total) * 100)) : 0;
        const current = data.current_repo ? ("，当前: " + data.current_repo) : "";
        const done = data.running ? "扫描中" : "已完成";
        byId("scanProgress").textContent = done + " | 最近开始: " + (data.started_at || "-") +
          " | 总仓库: " + total +
          " | 成功: " + (data.success_repos || 0) +
          " | 失败: " + (data.failed_repos || 0) +
          " | 进度: " + processed + "/" + total + current;
        byId("scanProgressBar").style.width = percent + "%";
        byId("scanProgressPct").textContent = percent + "%";
        byId("scanProgressCount").textContent = processed + "/" + total;
        byId("scanProgressSuccess").textContent = "成功 " + (data.success_repos || 0);
        byId("scanProgressFailed").textContent = "失败 " + (data.failed_repos || 0);
        const failureRows = byId("scanFailureRows");
        failureRows.innerHTML = "";
        for (const item of data.failures || []) {
          const tr = document.createElement("tr");
          tr.innerHTML = "<td class='mono'>" + item.repo + "</td><td class='mono'>" + item.error + "</td>";
          failureRows.appendChild(tr);
        }
      }

      async function startScan() {
        await request("/admin/api/scan/start", { method: "POST" });
        await driveScanUntilComplete();
      }

      async function stopScan() {
        await request("/admin/api/scan/stop", { method: "POST" });
        await updateScanStatus();
      }

      async function retryFailedScan() {
        await request("/admin/api/scan/retry-failures", { method: "POST" });
        await driveScanUntilComplete();
      }

      async function stepScan() {
        const batch = Number(byId("scanBatchSize").value || 12);
        await request("/admin/api/scan/step?batch=" + encodeURIComponent(String(batch)), { method: "POST" });
        await updateScanStatus();
      }

      async function runExternalSync() {
        const btn = byId("runExternalSyncBtn");
        const oldText = btn.textContent;
        btn.disabled = true;
        btn.textContent = "同步中...";
        byId("overviewHint").textContent = "外部同步执行中，请稍候...";
        try {
          const start = await request("/skills/external-sync/start", { method: "POST" });
          if (!start.started && start.status && start.status.running) {
            byId("overviewHint").textContent = "已有外部同步任务在执行，正在等待结果...";
          }
          let finished = null;
          for (let i = 0; i < 240; i += 1) {
            const status = await request("/skills/external-sync/status");
            if (!status.running) {
              finished = status;
              break;
            }
            await new Promise((resolve) => setTimeout(resolve, 1000));
          }
          await Promise.all([loadStats(), loadCatalog(false), updateScanStatus()]);
          if (finished && !finished.error) {
            byId("overviewHint").textContent =
              "外部同步完成: repos=" + (finished.tracked_repos || 0) +
              "，mapped=" + (finished.skills_external_map || 0) +
              "，audit_skills=" + (finished.audit_skills || 0) +
              "，audit_rows=" + (finished.audit_rows || 0) +
              "，完成时间=" + (finished.finished_at || "-");
          } else if (finished && finished.error) {
            byId("overviewHint").textContent = "外部同步失败: " + finished.error;
          } else {
            byId("overviewHint").textContent = "外部同步仍在进行中，可稍后查看状态。";
          }
        } catch (e) {
          byId("overviewHint").textContent = "外部同步失败: " + e.message;
        } finally {
          btn.disabled = false;
          btn.textContent = oldText;
        }
      }

      async function driveScanUntilComplete() {
        const batch = Number(byId("scanBatchSize").value || 12);
        for (let i = 0; i < 200; i += 1) {
          const status = await request("/admin/api/scan/step?batch=" + encodeURIComponent(String(batch)), { method: "POST" });
          await updateScanStatus();
          if (!status.running) break;
          await new Promise((resolve) => setTimeout(resolve, 300));
        }
        await loadCatalog(false);
      }

      async function saveRepo() {
        const repoInput = byId("repoInput").value.trim();
        if (!repoInput) return;
        const payload = {
          repo_url: repoInput,
          enabled: true
        };
        await request("/admin/api/tracked-repos", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(payload) });
        byId("repoInput").value = "";
        await loadCatalog(true);
      }

      async function loadRanking() {
        const day = byId("rankDay").value.trim();
        const limit = Number(byId("rankLimit").value || 50);
        const query = new URLSearchParams();
        if (day) query.set("day", day);
        query.set("limit", String(limit));
        const data = await request("/admin/api/rankings/skills?" + query.toString());
        const tbody = byId("rankRows");
        tbody.innerHTML = "";
        for (const item of data.items || []) {
          const tr = document.createElement("tr");
          tr.innerHTML = "<td>" + item.skill_name + "</td>" +
            "<td class='mono'>" + (item.source_repo || "-") + "</td>" +
            "<td>" + item.installs + "</td>" +
            "<td>" + item.unique_users + "</td>";
          tbody.appendChild(tr);
        }
      }

      async function loadUsers() {
        const query = new URLSearchParams();
        query.set("limit", String(Number(byId("userLimit").value || 100)));
        query.set("offset", String(Number(byId("userOffset").value || 0)));
        const userQuery = (byId("userQuery").value || "").trim();
        const userSort = (byId("userSort").value || "last_seen_desc").trim();
        if (userQuery) query.set("q", userQuery);
        if (userSort) query.set("sort", userSort);
        const data = await request("/admin/api/users?" + query.toString());
        const list = byId("userList");
        list.innerHTML = "";
        for (const item of data.items || []) {
          const user = item.user_hash;
          const card = document.createElement("div");
          card.className = "user-card";
          card.innerHTML =
            "<div class='title'>" +
              "<b class='mono'>" + (item.anonymous_id_prefix || "-") + "</b>" +
              "<button data-user='" + user + "'>查看详情</button>" +
            "</div>" +
            "<div class='muted mono'>hash: " + summarizeId(user) + "</div>" +
            "<div class='muted'>最近活跃: " + (item.last_seen || "-") + "</div>" +
            "<div class='muted'>安装技能数: " + Number(item.installs || 0) + " | 总事件: " + Number(item.total_events || 0) + "</div>" +
            "<div class='muted'>客户端: " + (item.client_app || "-") + (item.client_version ? (" " + item.client_version) : "") + "</div>" +
            "<div class='muted'>系统: " + [item.client_platform || "-", item.client_arch || "-"].join(" / ") + "</div>";
          list.appendChild(card);
        }
        byId("usersPageInfo").textContent = "共 " + (data.total || 0) + " 用户，当前 offset=" + (data.offset || 0) + "，limit=" + (data.limit || 0);
        list.querySelectorAll("button[data-user]").forEach((btn) => {
          btn.addEventListener("click", async () => {
            state.selectedUser = btn.getAttribute("data-user") || "";
            await loadUserDetail(state.selectedUser);
            await loadUserSkills(state.selectedUser);
          });
        });
      }

      async function loadUserDetail(user) {
        if (!user) return;
        const data = await request("/admin/api/users/" + encodeURIComponent(user));
        byId("userSkillTitle").textContent = "用户详情";
        const details = [
          ["匿名ID", data.anonymous_id || "-"],
          ["用户哈希", data.user_hash || "-"],
          ["最近活跃", data.last_seen || "-"],
          ["首次活跃", data.first_seen || "-"],
          ["安装技能数", Number(data.installs || 0)],
          ["总事件数", Number(data.total_events || 0)],
          ["活跃天数", Number(data.active_days || 0)],
          ["客户端", (data.last_client_app || "-") + (data.last_client_version ? (" " + data.last_client_version) : "")],
          ["系统", [data.last_client_platform || "-", data.last_client_arch || "-"].join(" / ")],
        ];
        byId("userDetailKv").innerHTML = details.map((item) =>
          "<div class='k'>" + item[0] + "</div><div class='mono'>" + item[1] + "</div>"
        ).join("");
      }

      async function loadUserSkills(user) {
        if (!user) return;
        if (!byId("userSkillTitle").textContent || byId("userSkillTitle").textContent === "选择左侧用户") {
          byId("userSkillTitle").textContent = user;
        }
        const query = new URLSearchParams();
        query.set("limit", String(Number(byId("userSkillLimit").value || 100)));
        query.set("offset", String(Number(byId("userSkillOffset").value || 0)));
        const skillQuery = (byId("userSkillQuery").value || "").trim();
        if (skillQuery) query.set("q", skillQuery);
        const data = await request("/admin/api/users/" + encodeURIComponent(user) + "/skills?" + query.toString());
        const tbody = byId("userSkillRows");
        tbody.innerHTML = "";
        for (const item of data.items || []) {
          const tr = document.createElement("tr");
          tr.innerHTML = "<td>" + item.skill_name + "</td>" +
            "<td class='mono'>" + (item.source_repo || "-") + "</td>" +
            "<td>" + item.installs + "</td>" +
            "<td class='mono'>" + (item.last_installed_at || "") + "</td>";
          tbody.appendChild(tr);
        }
      }

      function summarizeId(value) {
        const text = String(value || "").trim();
        if (!text) return "-";
        if (text.length <= 14) return text;
        return text.slice(0, 10) + "..." + text.slice(-4);
      }

      async function loadAll() {
        if (!state.authenticated) return;
        await Promise.all([loadStats(), loadCatalog(false), loadRanking(), loadUsers(), updateScanStatus()]);
      }

      function formatDuration(ms) {
        const value = Number(ms || 0);
        if (!Number.isFinite(value) || value <= 0) return "-";
        if (value < 1000) return value + "ms";
        if (value < 60000) return (value / 1000).toFixed(1) + "s";
        return Math.round(value / 60000) + "m";
      }

      function renderUserCurve(points) {
        const chartEl = byId("userCurveChart");
        if (!chartEl) return;
        const list = Array.isArray(points) ? points : [];
        if (!window.echarts) {
          chartEl.innerHTML = "<div style='padding:12px;color:#6b7280;font-size:12px'>ECharts 加载失败，无法渲染曲线</div>";
          return;
        }
        if (!userCurveChart) {
          userCurveChart = window.echarts.init(chartEl, null, { renderer: "canvas" });
          userCurveChart.on("click", function (params) {
            if (!params || typeof params.dataIndex !== "number") return;
            const item = list[params.dataIndex];
            if (!item) return;
            byId("userCurveHint").textContent =
              item.day + " | 累计用户 " + Number(item.total_users || 0) + " | 日活用户 " + Number(item.daily_active_users || 0);
          });
        }
        if (list.length === 0) {
          userCurveChart.clear();
          userCurveChart.setOption({
            title: {
              text: "暂无用户趋势数据",
              left: 12,
              top: 12,
              textStyle: { color: "#6b7280", fontSize: 12, fontWeight: 400 },
            },
            xAxis: { show: false, type: "category", data: [] },
            yAxis: { show: false, type: "value" },
            series: [],
          });
          return;
        }
        const xData = list.map((item) => item.day);
        const totalSeries = list.map((item) => Number(item.total_users || 0));
        const activeSeries = list.map((item) => Number(item.daily_active_users || 0));
        const yMax = Math.max(1, ...totalSeries, ...activeSeries);
        userCurveChart.setOption({
          animation: true,
          grid: { left: 48, right: 20, top: 18, bottom: 32 },
          tooltip: {
            trigger: "axis",
            backgroundColor: "rgba(17,24,39,0.92)",
            borderWidth: 0,
            textStyle: { color: "#fff", fontSize: 12 },
            formatter: function (params) {
              if (!Array.isArray(params) || params.length === 0) return "";
              const idx = params[0].dataIndex;
              const item = list[idx];
              if (!item) return "";
              return [
                "<div>" + item.day + "</div>",
                "<div>累计用户: " + Number(item.total_users || 0) + "</div>",
                "<div>日活用户: " + Number(item.daily_active_users || 0) + "</div>",
              ].join("");
            },
          },
          xAxis: {
            type: "category",
            data: xData,
            boundaryGap: false,
            axisLabel: { color: "#6b7280", fontSize: 11 },
            axisLine: { lineStyle: { color: "#d1d5db" } },
          },
          yAxis: {
            type: "value",
            min: 0,
            max: yMax,
            axisLabel: { color: "#6b7280", fontSize: 11 },
            splitLine: { lineStyle: { color: "#eef2f7" } },
          },
          series: [
            {
              name: "累计用户",
              type: "line",
              smooth: true,
              showSymbol: true,
              symbolSize: 6,
              itemStyle: { color: "#0f766e" },
              lineStyle: { width: 2.5, color: "#0f766e" },
              data: totalSeries,
            },
            {
              name: "日活用户",
              type: "line",
              smooth: true,
              showSymbol: true,
              symbolSize: 6,
              itemStyle: { color: "#2563eb" },
              lineStyle: { width: 2.2, color: "#2563eb" },
              data: activeSeries,
            },
          ],
        }, { notMerge: true });
      }

      setupTabs();
      byId("loginBtn").addEventListener("click", login);
      byId("password").addEventListener("keydown", (e) => { if (e.key === "Enter") login(); });
      byId("logoutBtn").addEventListener("click", logout);
      byId("addRepoBtn").addEventListener("click", saveRepo);
      byId("refreshReposBtn").addEventListener("click", () => loadCatalog(false));
      byId("refreshCatalogBtn").addEventListener("click", () => loadCatalog(true));
      byId("loadCatalogPageBtn").addEventListener("click", () => loadCatalog(false));
      byId("refreshRankBtn").addEventListener("click", loadRanking);
      byId("refreshUsersBtn").addEventListener("click", loadUsers);
      byId("refreshUserSkillBtn").addEventListener("click", () => loadUserSkills(state.selectedUser));
      byId("refreshOverviewBtn").addEventListener("click", loadStats);
      byId("activeWindow").addEventListener("change", loadStats);
      byId("userCurveDays").addEventListener("change", loadStats);
      byId("startScanBtn").addEventListener("click", startScan);
      byId("stopScanBtn").addEventListener("click", stopScan);
      byId("retryFailedScanBtn").addEventListener("click", retryFailedScan);
      byId("stepScanBtn").addEventListener("click", stepScan);
      byId("runExternalSyncBtn").addEventListener("click", runExternalSync);
      byId("catalogQuery").addEventListener("keydown", (e) => { if (e.key === "Enter") loadCatalog(false, 0); });
      byId("userQuery").addEventListener("keydown", (e) => { if (e.key === "Enter") { byId("userOffset").value = "0"; loadUsers(); } });
      byId("userSkillQuery").addEventListener("keydown", (e) => { if (e.key === "Enter") { byId("userSkillOffset").value = "0"; loadUserSkills(state.selectedUser); } });
      window.addEventListener("resize", () => { if (userCurveChart) userCurveChart.resize(); });

      loadSession().then(loadAll);
    </script>
  </body>
</html>`;
}
