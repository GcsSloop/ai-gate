import { createExecutionContext, waitOnExecutionContext } from "cloudflare:test";
import { describe, expect, it } from "vitest";
import worker from "../src";

const noopEnv = {
  DB: {} as D1Database,
  SKILL_METRICS_CACHE: {} as KVNamespace,
};

describe("skill metrics worker", () => {
	it("GET /health requires admin auth", async () => {
		const request = new Request("https://example.com/health");
		const ctx = createExecutionContext();
		const response = await worker.fetch(request, noopEnv, ctx);
		await waitOnExecutionContext(ctx);

		expect(response.status).toBe(401);
		const payload = (await response.json()) as { error: string };
		expect(payload.error).toBe("unauthorized");
	});

	it("POST /tracked-repos rejects unauthorized access", async () => {
		const request = new Request("https://example.com/tracked-repos", {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: JSON.stringify({
				platform: "github",
				owner: "openai",
				name: "skills",
				branch: "main",
			}),
		});
		const ctx = createExecutionContext();
		const response = await worker.fetch(request, noopEnv, ctx);
		await waitOnExecutionContext(ctx);

		expect(response.status).toBe(401);
		const payload = (await response.json()) as { error: string };
		expect(payload.error).toBe("unauthorized");
	});

	it("DELETE /tracked-repos rejects invalid query", async () => {
		const request = new Request(
			"https://example.com/tracked-repos?platform=github&owner=openai",
			{
				method: "DELETE",
				headers: { authorization: "Bearer test" },
			},
		);
		const ctx = createExecutionContext();
		const response = await worker.fetch(
			request,
			{ ...noopEnv, TRACKED_REPOS_ADMIN_TOKEN: "test" },
			ctx,
		);
		await waitOnExecutionContext(ctx);

		expect(response.status).toBe(400);
		const payload = (await response.json()) as { error: string };
		expect(payload.error).toBe("invalid_query");
	});

	it("GET /tracked-repos requires admin auth", async () => {
		const request = new Request("https://example.com/tracked-repos");
		const ctx = createExecutionContext();
		const response = await worker.fetch(request, noopEnv, ctx);
		await waitOnExecutionContext(ctx);
		expect(response.status).toBe(401);
		const payload = (await response.json()) as { error: string };
		expect(payload.error).toBe("unauthorized");
	});

	it("POST /events/install is public even when bearer token is configured", async () => {
		const mockDb = {
			prepare: () => ({
				bind: () => ({
					run: async () => ({ meta: { changes: 1 } }),
				}),
			}),
		} as unknown as D1Database;
		const mockKv = {
			delete: async () => null,
		} as unknown as KVNamespace;
		const request = new Request("https://example.com/events/install", {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: JSON.stringify({ skill_name: "demo-skill" }),
		});
		const ctx = createExecutionContext();
		const response = await worker.fetch(request, { DB: mockDb, SKILL_METRICS_CACHE: mockKv, INGEST_BEARER_TOKEN: "ingest-token" }, ctx);
		await waitOnExecutionContext(ctx);
		expect(response.status).toBe(201);
	});

	it("POST /events/install auto generates anonymous_id when missing", async () => {
		const mockDb = {
			prepare: () => ({
				bind: () => ({
					run: async () => ({ meta: { changes: 1 } }),
				}),
			}),
		} as unknown as D1Database;
		const mockKv = {
			delete: async () => null,
		} as unknown as KVNamespace;
		const request = new Request("https://example.com/events/install", {
			method: "POST",
			headers: { "content-type": "application/json", authorization: "Bearer ingest-token" },
			body: JSON.stringify({ skill_name: "demo-skill" }),
		});
		const ctx = createExecutionContext();
		const response = await worker.fetch(
			request,
			{ DB: mockDb, SKILL_METRICS_CACHE: mockKv, INGEST_BEARER_TOKEN: "ingest-token" },
			ctx,
		);
		await waitOnExecutionContext(ctx);
		expect(response.status).toBe(201);
		const payload = (await response.json()) as { anonymous_id?: string; user_hash?: string };
		expect(payload.anonymous_id?.startsWith("anon_")).toBe(true);
		expect((payload.user_hash ?? "").length).toBe(64);
	});

	it("POST /events/install allows anonymous ingest when bearer is not configured", async () => {
		const mockDb = {
			prepare: () => ({
				bind: () => ({
					run: async () => ({ meta: { changes: 1 } }),
				}),
			}),
		} as unknown as D1Database;
		const mockKv = {
			delete: async () => null,
		} as unknown as KVNamespace;
		const request = new Request("https://example.com/events/install", {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: JSON.stringify({ anonymous_id: "anon_test_1", skill_name: "__client_active__", source_repo: "__aigate_client__" }),
		});
		const ctx = createExecutionContext();
		const response = await worker.fetch(request, { DB: mockDb, SKILL_METRICS_CACHE: mockKv }, ctx);
		await waitOnExecutionContext(ctx);
		expect(response.status).toBe(201);
		const payload = (await response.json()) as { inserted: boolean; user_hash?: string };
		expect(payload.inserted).toBe(true);
		expect((payload.user_hash ?? "").length).toBe(64);
	});

	it("GET /rankings/skills returns public shape without install stats", async () => {
		const mockDb = {
			prepare: () => ({
				bind: () => ({
					all: async () => ({
						results: [
							{
								skill_name: "alpha-skill",
								source_repo: "openai/skills",
								installs: 123,
								unique_users: 45,
							},
						],
					}),
				}),
			}),
		} as unknown as D1Database;
		const mockKv = {
			get: async () => null,
			put: async () => null,
		} as unknown as KVNamespace;
		const request = new Request("https://example.com/rankings/skills?limit=50");
		const ctx = createExecutionContext();
		const response = await worker.fetch(request, { DB: mockDb, SKILL_METRICS_CACHE: mockKv }, ctx);
		await waitOnExecutionContext(ctx);
		expect(response.status).toBe(200);
		const payload = (await response.json()) as { items: Array<Record<string, unknown>> };
		expect(payload.items).toEqual([
			{
				skill_name: "alpha-skill",
				source_repo: "openai/skills",
			},
		]);
	});

	it("GET /skills/search filters by query and returns indexed_total", async () => {
		const mockKv = {
			get: async (key: string) => {
				if (key === "catalog:skills:v1") {
					return JSON.stringify({
						fetched_at: "2026-04-11T00:00:00Z",
						repos: [],
						items: [
							{
								id: "github:openai/skills:alpha",
								name: "Alpha Skill",
								platform: "github",
								repo_owner: "openai",
								repo_name: "skills",
								branch: "main",
								repo_url: "https://github.com/openai/skills",
								source_path: "alpha",
								source_url: "https://github.com/openai/skills/tree/main/alpha",
								managed_name: "skills-alpha",
							},
							{
								id: "github:openai/skills:beta",
								name: "Beta Skill",
								platform: "github",
								repo_owner: "openai",
								repo_name: "skills",
								branch: "main",
								repo_url: "https://github.com/openai/skills",
								source_path: "beta",
								source_url: "https://github.com/openai/skills/tree/main/beta",
								managed_name: "skills-beta",
							},
						],
					});
				}
				return null;
			},
			put: async () => null,
		} as unknown as KVNamespace;
		const mockDb = {
			prepare: () => ({
				bind: () => ({
					all: async () => ({ results: [] }),
					first: async () => ({ total_users: 0, active_users_1d: 0 }),
				}),
			}),
		} as unknown as D1Database;
		const request = new Request("https://example.com/skills/search?q=alpha&limit=50&offset=0");
		const ctx = createExecutionContext();
		const response = await worker.fetch(request, { DB: mockDb, SKILL_METRICS_CACHE: mockKv }, ctx);
		await waitOnExecutionContext(ctx);
		expect(response.status).toBe(200);
		const payload = (await response.json()) as { indexed_total: number; total: number; items: Array<{ name: string }> };
		expect(payload.indexed_total).toBe(2);
		expect(payload.total).toBe(1);
		expect(payload.items.map((item) => item.name)).toEqual(["Alpha Skill"]);
	});

	it("GET /admin/api/stats/overview returns aggregate overview with window switch", async () => {
		const mockDb = {
			prepare: (sql: string) => ({
				bind: () => ({
					first: async () => {
						if (sql.includes("COUNT(DISTINCT user_hash) AS total_users")) return { total_users: 12 };
						if (sql.includes("COUNT(DISTINCT user_hash) AS active_users")) return { active_users: 5 };
						if (sql.includes("COUNT(DISTINCT skill_name) AS ranking_skills_total")) return { ranking_skills_total: 9 };
						if (sql.includes("COUNT(1) AS total_install_events")) return { total_install_events: 28 };
						return {};
					},
					all: async () => {
						if (sql.includes("FROM tracked_repos")) {
							return {
								results: [
									{ platform: "github", owner: "openai", name: "skills", branch: "main", enabled: 1, sort_order: 0, updated_at: "2026-04-11T00:00:00Z" },
									{ platform: "github", owner: "anthropics", name: "skills", branch: "main", enabled: 0, sort_order: 1, updated_at: "2026-04-11T00:00:00Z" },
								],
							};
						}
						return { results: [] };
					},
				}),
				first: async () => {
					if (sql.includes("COUNT(DISTINCT user_hash) AS total_users")) return { total_users: 12 };
					if (sql.includes("COUNT(1) AS total_install_events")) return { total_install_events: 28 };
					return {};
				},
				all: async () => {
					if (sql.includes("FROM tracked_repos")) {
						return {
							results: [
								{ platform: "github", owner: "openai", name: "skills", branch: "main", enabled: 1, sort_order: 0, updated_at: "2026-04-11T00:00:00Z" },
								{ platform: "github", owner: "anthropics", name: "skills", branch: "main", enabled: 0, sort_order: 1, updated_at: "2026-04-11T00:00:00Z" },
							],
						};
					}
					return { results: [] };
				},
			}),
		} as unknown as D1Database;
		const mockKv = {
			get: async (key: string) => {
				if (key === "catalog:skills:v1") {
					return JSON.stringify({
						fetched_at: "2026-04-11T00:00:00Z",
						repos: [],
						items: [
							{ id: "a", name: "A", platform: "github", repo_owner: "o", repo_name: "r", branch: "main", repo_url: "", source_path: "", source_url: "" },
							{ id: "b", name: "B", platform: "github", repo_owner: "o", repo_name: "r", branch: "main", repo_url: "", source_path: "", source_url: "" },
							{ id: "c", name: "C", platform: "github", repo_owner: "o", repo_name: "r", branch: "main", repo_url: "", source_path: "", source_url: "" },
						],
					});
				}
				return null;
			},
			put: async () => null,
		} as unknown as KVNamespace;
		const request = new Request("https://example.com/admin/api/stats/overview?window=7d", {
			headers: { authorization: "Bearer admin-token" },
		});
		const ctx = createExecutionContext();
		const response = await worker.fetch(
			request,
			{ DB: mockDb, SKILL_METRICS_CACHE: mockKv, TRACKED_REPOS_ADMIN_TOKEN: "admin-token" },
			ctx,
		);
		await waitOnExecutionContext(ctx);
		expect(response.status).toBe(200);
		const payload = (await response.json()) as {
			total_users: number;
			active_users: number;
			active_window_days: number;
			repos_total: number;
			repos_enabled: number;
			skills_total: number;
			ranking_skills_total: number;
			total_install_events: number;
		};
		expect(payload.total_users).toBe(12);
		expect(payload.active_users).toBe(5);
		expect(payload.active_window_days).toBe(7);
		expect(payload.repos_total).toBe(2);
		expect(payload.repos_enabled).toBe(1);
		expect(payload.skills_total).toBe(3);
		expect(payload.ranking_skills_total).toBe(9);
		expect(payload.total_install_events).toBe(28);
	});
});
