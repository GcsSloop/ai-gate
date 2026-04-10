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
  user_hash?: string;
  anonymous_id?: string;
  skill_name: string;
  source_repo?: string;
  client_version?: string;
  installed_at?: string;
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
  repos_total: number;
  repos_enabled: number;
  skills_total: number;
  ranking_skills_total: number;
  total_install_events: number;
  ranking_day: string;
  catalog_fetched_at: string;
};

type UserInstallSummary = {
  user_hash: string;
  installs: number;
  last_seen: string;
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
};

type ScanWorking = {
  started_at: string;
  repos: TrackedRepoItem[];
  repo_skill_counts: Record<string, number>;
  repo_stars: Record<string, number>;
  repo_errors: Record<string, string>;
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
const SCAN_WORKING_TTL_SECONDS = 2 * 24 * 60 * 60;
const SCAN_DEFAULT_BATCH_SIZE = 12;
const SKILLS_SOURCE_PAGES = [
  "https://skills.sh/",
  "https://skills.sh/trending",
  "https://skills.sh/hot",
  "https://skills.sh/official",
];

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
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
        await assertBearer(request, env.INGEST_BEARER_TOKEN);
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
        const stats = await getOverviewStats(env, windowDays, rankingDay);
        return json(stats);
      }
      if (request.method === "GET" && url.pathname === "/admin/api/users") {
        await assertAdmin(request, env);
        const limit = normalizeUsersLimit(url.searchParams.get("limit"));
        const offset = normalizeUsersOffset(url.searchParams.get("offset"));
        const items = await listUsers(env, limit, offset);
        return json({ items, limit, offset });
      }
      if (request.method === "GET" && url.pathname.startsWith("/admin/api/users/") && url.pathname.endsWith("/skills")) {
        await assertAdmin(request, env);
        const userHash = decodeURIComponent(url.pathname.slice("/admin/api/users/".length, -"/skills".length));
        if (!userHash.trim()) {
          throw httpError(400, "invalid_user_hash");
        }
        const items = await listSkillsForUser(env, userHash.trim());
        return json({ user_hash: userHash.trim(), items });
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
        const catalog = await getSkillCatalog(env, force);
        const sliced = sliceCatalog(catalog, offset, limit);
        return json(sliced);
      }
      if (request.method === "GET" && url.pathname === "/admin/api/scan/status") {
        await assertAdmin(request, env);
        const status = await getScanStatus(env);
        return json(status);
      }
      if (request.method === "POST" && url.pathname === "/admin/api/scan/start") {
        await assertAdmin(request, env);
        const status = await startCatalogScan(env);
        return json(status);
      }
      if (request.method === "POST" && url.pathname === "/admin/api/scan/step") {
        await assertAdmin(request, env);
        const batchSize = normalizeScanBatchSize(url.searchParams.get("batch"));
        const status = await stepCatalogScan(env, batchSize);
        return json(status);
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
  const clientVersion = trim(payload.client_version ?? "", 60);
  if (!skillName) {
    throw httpError(400, "invalid_payload");
  }

  let anonymousId = trim(payload.anonymous_id ?? "", 120);
  let userHash = trim(payload.user_hash ?? "", 120);
  if (!userHash) {
    if (!anonymousId) {
      anonymousId = `anon_${crypto.randomUUID()}`;
    }
    userHash = await sha256Hex(anonymousId);
  }

  const eventDay = normalizeDay(payload.installed_at) ?? utcDay();
  const eventKey = await sha256Hex(`${eventDay}:${userHash}:${skillName}:${sourceRepo}`);
  const now = new Date().toISOString();
  const result = await env.DB
    .prepare(
      `INSERT OR IGNORE INTO install_events (
        event_key, event_day, user_hash, skill_name, source_repo, client_version, created_at
      ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)`,
    )
    .bind(eventKey, eventDay, userHash, skillName, sourceRepo, clientVersion, now)
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

async function getOverviewStats(env: Env, windowDays: number, rankingDay: string): Promise<OverviewStats> {
  const userStats = await getUserStats(env, windowDays);
  const repos = await listTrackedRepos(env);
  const catalog = await getSkillCatalog(env, false);
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
  const reposEnabled = repos.reduce((acc, repo) => acc + (repo.enabled ? 1 : 0), 0);
  return {
    ...userStats,
    repos_total: repos.length,
    repos_enabled: reposEnabled,
    skills_total: catalog.items.length,
    ranking_skills_total: Number(rankingTotalRes?.ranking_skills_total ?? 0),
    total_install_events: Number(installsRes?.total_install_events ?? 0),
    ranking_day: rankingDay,
    catalog_fetched_at: catalog.fetched_at,
  };
}

async function listUsers(env: Env, limit: number, offset: number): Promise<UserInstallSummary[]> {
  const query = await env.DB
    .prepare(
      `SELECT user_hash, COUNT(1) AS installs, MAX(created_at) AS last_seen
       FROM install_events
       GROUP BY user_hash
       ORDER BY last_seen DESC
       LIMIT ?1 OFFSET ?2`,
    )
    .bind(limit, offset)
    .all<UserInstallSummary>();
  return query.results ?? [];
}

async function listSkillsForUser(env: Env, userHash: string): Promise<UserSkillInstall[]> {
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
  return query.results ?? [];
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

async function upsertTrackedRepo(env: Env, raw: Partial<TrackedRepoItem>): Promise<TrackedRepoItem> {
  const item = normalizeTrackedRepo(raw, 0);
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
  const catalog = await buildCatalogFromSkillsSource(env, await listTrackedRepos(env), undefined, undefined, undefined);
  await env.SKILL_METRICS_CACHE.put(SKILL_CATALOG_CACHE_KEY, JSON.stringify(catalog), { expirationTtl: SKILL_CATALOG_TTL_SECONDS });
  return catalog;
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

async function startCatalogScan(env: Env): Promise<ScanStatus> {
  const repos = (await listTrackedRepos(env)).filter((item) => item.enabled);
  const status: ScanStatus = {
    running: true,
    started_at: new Date().toISOString(),
    total_repos: repos.length,
    processed_repos: 0,
    success_repos: 0,
    failed_repos: 0,
    current_index: 0,
    message: "running",
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

async function stepCatalogScan(env: Env, batchSize: number): Promise<ScanStatus> {
  const status = await getScanStatus(env);
  if (!status.running) return status;
  const workingRaw = await env.SKILL_METRICS_CACHE.get(SCAN_WORKING_CACHE_KEY);
  if (!workingRaw) {
    const stopped: ScanStatus = { ...status, running: false, finished_at: new Date().toISOString(), message: "working_state_missing" };
    await env.SKILL_METRICS_CACHE.put(SCAN_STATUS_CACHE_KEY, JSON.stringify(stopped), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
    return stopped;
  }
  const working = JSON.parse(workingRaw) as ScanWorking;
  const startIndex = status.current_index;
  const endIndex = Math.min(startIndex + batchSize, working.repos.length);

  for (let i = startIndex; i < endIndex; i += 1) {
    const repo = working.repos[i];
    const repoName = `${repo.owner}/${repo.name}`;
    status.current_repo = repoName;
    try {
      const stars = await fetchRepoStars(repo);
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
  } else {
    status.message = "running";
  }

  await env.SKILL_METRICS_CACHE.put(SCAN_STATUS_CACHE_KEY, JSON.stringify(status), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
  await env.SKILL_METRICS_CACHE.put(SCAN_WORKING_CACHE_KEY, JSON.stringify(working), { expirationTtl: SCAN_WORKING_TTL_SECONDS });
  return status;
}

async function buildCatalogFromSkillsSource(
  _env: Env,
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
  for (const entry of entries) {
    const repoInfo = parseSourceRepo(entry.source);
    if (!repoInfo) continue;
    const key = repoKey(repoInfo.platform, repoInfo.owner, repoInfo.name);
    const tracked = trackedMap.get(key);
    if (!tracked) continue;
    const itemId = `${key}:${entry.skill_id}`;
    if (dedupe.has(itemId)) continue;
    dedupe.add(itemId);
    items.push({
      id: itemId,
      name: entry.name || entry.skill_id,
      platform: repoInfo.platform,
      repo_owner: repoInfo.owner,
      repo_name: repoInfo.name,
      branch: tracked.branch || "main",
      repo_url: repoHomeURL(repoInfo.platform, repoInfo.owner, repoInfo.name),
      source_path: `${entry.skill_id}/SKILL.md`,
      source_url: repoHomeURL(repoInfo.platform, repoInfo.owner, repoInfo.name),
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

function parseSourceRepo(source: string): { platform: "github" | "gitlab"; owner: string; name: string } | null {
  const value = source.trim().replace(/^https?:\/\/(www\.)?github\.com\//i, "").replace(/^https?:\/\/(www\.)?gitlab\.com\//i, "");
  const parts = value.split("/").filter(Boolean);
  if (parts.length < 2) return null;
  const owner = parts[0];
  const name = parts[1];
  if (!owner || !name) return null;
  return { platform: "github", owner, name };
}

async function fetchSkillsSourceEntries(): Promise<SkillsSourceEntry[]> {
  const dedupe = new Set<string>();
  const entries: SkillsSourceEntry[] = [];
  const pattern = /source\\":\\"([^"\\]+)\\",\\"skillId\\":\\"([^"\\]+)\\",\\"name\\":\\"([^"\\]*)/g;
  for (const page of SKILLS_SOURCE_PAGES) {
    const resp = await fetch(page, { headers: { "User-Agent": "aigate-skill-metrics" } });
    if (!resp.ok) continue;
    const html = await resp.text();
    pattern.lastIndex = 0;
    let match: RegExpExecArray | null = null;
    while ((match = pattern.exec(html)) !== null) {
      const source = match[1]?.trim() ?? "";
      const skillId = match[2]?.trim() ?? "";
      const name = (match[3] ?? "").replace(/\\u[\da-fA-F]{4}/g, "").trim() || skillId;
      const repoInfo = parseSourceRepo(source);
      if (!repoInfo || !skillId) continue;
      const key = `${repoKey(repoInfo.platform, repoInfo.owner, repoInfo.name)}:${skillId.toLowerCase()}`;
      if (dedupe.has(key)) continue;
      dedupe.add(key);
      entries.push({
        source: `${repoInfo.owner}/${repoInfo.name}`,
        skill_id: skillId,
        name,
      });
    }
  }
  return entries;
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

async function fetchRepoStars(repo: TrackedRepoItem): Promise<number> {
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

function normalizeTrackedRepo(raw: Partial<TrackedRepoItem>, fallbackOrder: number): TrackedRepoItem {
  const platform = normalizePlatform(raw.platform ?? "github");
  const owner = trim(raw.owner ?? "", 120);
  const name = trim(raw.name ?? "", 120);
  const branch = trim(raw.branch ?? "main", 120) || "main";
  const enabled = raw.enabled !== false;
  const sortOrder = Number.isFinite(raw.sort_order) ? Math.max(0, Math.floor(raw.sort_order as number)) : fallbackOrder;
  if (!platform || !owner || !name) {
    throw httpError(400, "invalid_repo_item");
  }
  return { platform, owner, name, branch, enabled, sort_order: sortOrder };
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
      .progress-wrap { margin-top:10px; }
      .progress-track { width:100%; height:10px; border-radius:999px; background:#e5e7eb; overflow:hidden; }
      .progress-bar { height:100%; width:0%; background:linear-gradient(90deg,#0f766e,#14b8a6); transition:width .25s ease; }
      .progress-meta { margin-top:8px; display:flex; gap:12px; flex-wrap:wrap; color:var(--muted); font-size:12px; }
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
            <div class="stat"><span class="muted">仓库总数</span><b id="repoCount">-</b></div>
            <div class="stat"><span class="muted">启用仓库数</span><b id="repoEnabledCount">-</b></div>
            <div class="stat"><span class="muted">已收录技能数</span><b id="skillCount">-</b></div>
            <div class="stat"><span class="muted">排行榜技能数（当日）</span><b id="rankingCount">-</b></div>
            <div class="stat"><span class="muted">安装事件总数</span><b id="installEventCount">-</b></div>
          </div>
          <div class="row" style="margin-top:12px">
            <button id="startScanBtn" class="primary">手动触发扫描</button>
            <button id="stepScanBtn">继续扫描一步</button>
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
        </div>

        <div class="panel tab-panel" id="tab-repos">
          <h2>仓库追踪管理</h2>
          <div class="row" style="margin-bottom:10px">
            <select id="repoPlatform"><option value="github">github</option><option value="gitlab">gitlab</option></select>
            <input id="repoOwner" placeholder="owner" />
            <input id="repoName" placeholder="repo" />
            <input id="repoBranch" placeholder="branch" value="main" />
            <input id="repoOrder" placeholder="order" type="number" value="0" style="width:100px" />
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
            <input id="catalogOffset" type="number" value="0" style="width:120px" />
            <input id="catalogLimit" type="number" value="300" style="width:120px" />
            <button id="loadCatalogPageBtn">加载分页</button>
            <span class="muted" id="catalogPageInfo"></span>
          </div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Skill</th><th>来源仓库</th><th>分支</th><th>路径</th><th>链接</th></tr></thead>
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
            <button id="refreshUsersBtn">刷新用户列表</button>
            <span class="muted">点击用户查看安装明细</span>
          </div>
          <div class="row" style="align-items:flex-start">
            <div style="flex:1; min-width:360px">
              <div class="table-wrap">
                <table>
                  <thead><tr><th>用户哈希</th><th>安装总数</th><th>最近安装</th></tr></thead>
                  <tbody id="userRows"></tbody>
                </table>
              </div>
            </div>
            <div style="flex:1; min-width:360px">
              <div class="mono muted" id="userSkillTitle">选择左侧用户</div>
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
    <script>
      const state = { authenticated: ${initialAuthenticated ? "true" : "false"} };
      const byId = (id) => document.getElementById(id);

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
        const query = new URLSearchParams({ window });
        const s = await request("/admin/api/stats/overview?" + query.toString());
        byId("totalUsers").textContent = s.total_users || 0;
        byId("activeUsers").textContent = s.active_users || 0;
        byId("activeUsersLabel").textContent = "活跃用户（" + (s.active_window_label || "1天") + "）";
        byId("repoCount").textContent = s.repos_total || 0;
        byId("repoEnabledCount").textContent = s.repos_enabled || 0;
        byId("skillCount").textContent = s.skills_total || 0;
        byId("rankingCount").textContent = s.ranking_skills_total || 0;
        byId("installEventCount").textContent = s.total_install_events || 0;
        byId("overviewHint").textContent = "目录缓存: " + (s.catalog_fetched_at || "-") + " | 排行榜日期: " + (s.ranking_day || "-");
      }

      async function loadCatalog(forceRefresh, customOffset, customLimit) {
        const offset = typeof customOffset === "number" ? customOffset : Number(byId("catalogOffset").value || 0);
        const limit = typeof customLimit === "number" ? customLimit : Number(byId("catalogLimit").value || 300);
        const query = new URLSearchParams();
        if (forceRefresh) query.set("refresh", "1");
        query.set("offset", String(offset));
        query.set("limit", String(limit));
        const data = await request("/admin/api/skills/final?" + query.toString());
        byId("catalogFetchedAt").textContent = "技能目录缓存时间: " + (data.fetched_at || "-");
        byId("catalogPageInfo").textContent = "总计 " + (data.total_items || 0) + "，当前 offset=" + (data.offset || 0) + "，limit=" + (data.limit || 0);

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
          const tr = document.createElement("tr");
          tr.innerHTML = "<td>" + item.name + "</td>" +
            "<td class='mono'>" + item.repo_owner + "/" + item.repo_name + "</td>" +
            "<td>" + item.branch + "</td>" +
            "<td class='mono'>" + item.source_path + "</td>" +
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
      }

      async function startScan() {
        await request("/admin/api/scan/start", { method: "POST" });
        await driveScanUntilComplete();
      }

      async function stepScan() {
        await request("/admin/api/scan/step", { method: "POST" });
        await updateScanStatus();
      }

      async function driveScanUntilComplete() {
        for (let i = 0; i < 200; i += 1) {
          const status = await request("/admin/api/scan/step", { method: "POST" });
          await updateScanStatus();
          if (!status.running) break;
          await new Promise((resolve) => setTimeout(resolve, 300));
        }
        await loadCatalog(false);
      }

      async function saveRepo() {
        const payload = {
          platform: byId("repoPlatform").value,
          owner: byId("repoOwner").value.trim(),
          name: byId("repoName").value.trim(),
          branch: byId("repoBranch").value.trim() || "main",
          sort_order: Number(byId("repoOrder").value || 0),
          enabled: true
        };
        await request("/admin/api/tracked-repos", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(payload) });
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
        const data = await request("/admin/api/users?limit=100");
        const tbody = byId("userRows");
        tbody.innerHTML = "";
        for (const item of data.items || []) {
          const user = item.user_hash;
          const tr = document.createElement("tr");
          tr.innerHTML = "<td class='mono'><a href='#' data-user='" + user + "'>" + user + "</a></td>" +
            "<td>" + item.installs + "</td>" +
            "<td class='mono'>" + (item.last_seen || "") + "</td>";
          tbody.appendChild(tr);
        }
        tbody.querySelectorAll("a[data-user]").forEach((a) => {
          a.addEventListener("click", async (e) => {
            e.preventDefault();
            await loadUserSkills(a.getAttribute("data-user"));
          });
        });
      }

      async function loadUserSkills(user) {
        byId("userSkillTitle").textContent = user;
        const data = await request("/admin/api/users/" + encodeURIComponent(user) + "/skills");
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

      async function loadAll() {
        if (!state.authenticated) return;
        await Promise.all([loadStats(), loadCatalog(false), loadRanking(), loadUsers(), updateScanStatus()]);
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
      byId("refreshOverviewBtn").addEventListener("click", loadStats);
      byId("activeWindow").addEventListener("change", loadStats);
      byId("startScanBtn").addEventListener("click", startScan);
      byId("stepScanBtn").addEventListener("click", stepScan);

      loadSession().then(loadAll);
    </script>
  </body>
</html>`;
}
