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

	it("POST /events/install requires bearer token", async () => {
		const request = new Request("https://example.com/events/install", {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: JSON.stringify({ skill_name: "demo-skill" }),
		});
		const ctx = createExecutionContext();
		const response = await worker.fetch(request, { ...noopEnv, INGEST_BEARER_TOKEN: "ingest-token" }, ctx);
		await waitOnExecutionContext(ctx);
		expect(response.status).toBe(401);
		const payload = (await response.json()) as { error: string };
		expect(payload.error).toBe("missing_bearer_token");
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
});
