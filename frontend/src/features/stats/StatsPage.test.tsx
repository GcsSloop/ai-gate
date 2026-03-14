import { render, screen, waitFor } from "@testing-library/react";

import { StatsPage } from "./StatsPage";

vi.mock("../../lib/api", () => ({
  listAccounts: vi.fn(),
  getDashboardSummary: vi.fn(),
  getDashboardTrends: vi.fn(),
  getDashboardRecentEvents: vi.fn(),
}));

import { getDashboardRecentEvents, getDashboardSummary, getDashboardTrends, listAccounts } from "../../lib/api";

describe("StatsPage", () => {
  const t = (value: string) => value;

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listAccounts).mockResolvedValue([{ id: 1, account_name: "Alpha" }] as never);
    vi.mocked(getDashboardSummary).mockResolvedValue({
      request_count: 12,
      success_count: 10,
      failure_count: 2,
      input_tokens: 12000,
      output_tokens: 4000,
      total_tokens: 16000,
      estimated_cost: 1.23,
      balance_delta: -4.5,
      quota_delta: -8000,
    } as never);
    vi.mocked(getDashboardTrends).mockResolvedValue([
      {
        bucket: "2026-03-15T09:00:00Z",
        request_count: 2,
        input_tokens: 300,
        output_tokens: 30,
        total_tokens: 330,
        estimated_cost: 0.3,
        balance_delta: 0,
        quota_delta: 0,
      },
    ] as never);
    vi.mocked(getDashboardRecentEvents).mockResolvedValue([
      {
        id: 1,
        account_id: 1,
        provider_type: "openai-compatible",
        request_kind: "responses",
        model: "gpt-5.2",
        status: "completed",
        input_tokens: 1200,
        output_tokens: 300,
        total_tokens: 1500,
        estimated_cost: 0.42,
        latency_ms: 321,
        created_at: "2026-03-15T10:05:00Z",
      },
    ] as never);
  });

  it("renders summary, trends, and recent event rows", async () => {
    render(<StatsPage language="zh-CN" t={t} />);

    expect(await screen.findByText("Token 与费用统计")).toBeInTheDocument();
    expect(screen.getByText("请求数")).toBeInTheDocument();
    expect(screen.getByText("预估费用")).toBeInTheDocument();
    expect(screen.getByText("状态分布")).toBeInTheDocument();
    expect(screen.getByText("最近记录")).toBeInTheDocument();
    expect(screen.getByText("gpt-5.2")).toBeInTheDocument();

    await waitFor(() => {
      expect(getDashboardSummary).toHaveBeenCalledWith(24, undefined, "");
      expect(getDashboardTrends).toHaveBeenCalledWith(24, undefined, "");
      expect(getDashboardRecentEvents).toHaveBeenCalledWith(24, undefined, "", 20);
    });
  });
});
