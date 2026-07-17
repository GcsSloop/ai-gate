import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { UserPoolPage } from "./UserPoolPage";
import { getServerMe, getServerUpstreams, updateServerRoute, updateServerUpstreamLock } from "../../lib/api";

vi.mock("../../lib/api", async () => {
  const actual = await vi.importActual<typeof import("../../lib/api")>("../../lib/api");
  return {
    ...actual,
    getServerMe: vi.fn(),
    getServerUpstreams: vi.fn(),
    updateServerRoute: vi.fn(),
    updateServerUpstreamLock: vi.fn(),
  };
});

describe("UserPoolPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(getServerMe).mockResolvedValue({
      user: { id: 1, name: "alice", username: "alice", role: "user", status: "active", created_at: "2026-06-15T00:00:00Z" },
      request_count: 3,
      total_tokens: 120,
    });
    vi.mocked(getServerUpstreams).mockResolvedValue({
      total_accounts: 2,
      available_accounts: 1,
      current_account_id: 1,
      route_locked: false,
      accounts: [
        {
          id: 1,
          provider_type: "openai-compatible",
          account_name: "账号 A",
          source_icon: "openai",
          auth_mode: "api_key",
          base_url: "https://upstream-a.example/v1",
          status: "active",
          available: true,
          current: true,
          preferred: false,
          account_locked: false,
          supports_responses: true,
          balance: 10,
          quota_remaining: 1000,
          rpm_remaining: 20,
          tpm_remaining: 3000,
          health_score: 0.9,
          recent_error_rate: 0,
          last_total_tokens: 120,
          last_input_tokens: 40,
          last_output_tokens: 80,
          model_context_window: 0,
          primary_used_percent: 0,
          secondary_used_percent: 0,
          usage_display: {
            usage_windows: [
              { label: "周额度", remaining_percent: 98.8, remaining_value: "$296.39", total_value: "$300.00" },
              { label: "总额度", remaining_percent: 92.1, remaining_value: "$8291.59", total_value: "$9000.00" },
            ],
          },
        },
        {
          id: 2,
          provider_type: "openai-compatible",
          account_name: "账号 B",
          source_icon: "ppchat",
          auth_mode: "api_key",
          base_url: "https://upstream-b.example/v1",
          status: "disabled",
          available: false,
          current: false,
          preferred: false,
          account_locked: false,
          supports_responses: true,
          balance: 0,
          quota_remaining: 0,
          rpm_remaining: 0,
          tpm_remaining: 0,
          health_score: 0,
          recent_error_rate: 0,
          last_total_tokens: 0,
          last_input_tokens: 0,
          last_output_tokens: 0,
          model_context_window: 0,
          primary_used_percent: 0,
          secondary_used_percent: 0,
        },
      ],
    });
    vi.mocked(updateServerRoute).mockResolvedValue({
      preferred_account_id: 1,
      route_locked: true,
    });
    vi.mocked(updateServerUpstreamLock).mockResolvedValue({
      id: 1,
      account_locked: true,
    });
  });

  it("shows own usage and sanitized upstream route controls", async () => {
    const { container } = render(<UserPoolPage t={(value) => value} />);

    expect(await screen.findByText(/请求数 3/)).toBeInTheDocument();
    expect(await screen.findByText("120")).toBeInTheDocument();
    expect(await screen.findByText("可用 1 / 2")).toBeInTheDocument();
    expect(container.querySelector(".user-upstreams-receipt")).toBeInTheDocument();
    expect(await screen.findByText("账号 A")).toBeInTheDocument();
    expect(screen.getByText("当前使用中")).toBeInTheDocument();
    expect(screen.getByText("https://upstream-a.example/v1")).toBeInTheDocument();
    expect(screen.getByText("$296.39 / $300.00")).toBeInTheDocument();
    expect(screen.getByText("$8291.59 / $9000.00")).toBeInTheDocument();
    expect(screen.getByRole("progressbar", { name: "账号 A-周额度" })).toHaveAttribute("aria-valuenow", "98.8");
    expect(screen.getByRole("progressbar", { name: "账号 A-总额度" })).toHaveAttribute("aria-valuenow", "92.1");
    expect(screen.queryByText(/sk-/)).not.toBeInTheDocument();
    expect(screen.queryByText("credential_ref")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "锁定-账号 A" }));
    await waitFor(() => expect(updateServerUpstreamLock).toHaveBeenCalledWith(1, true));
    expect(updateServerRoute).not.toHaveBeenCalledWith({ account_id: 1, locked: true });
    await waitFor(() => expect(getServerUpstreams).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.getByRole("button", { name: "切换-账号 A" })).not.toBeDisabled());

    fireEvent.click(screen.getByRole("button", { name: "切换-账号 A" }));
    await waitFor(() => expect(updateServerRoute).toHaveBeenCalledWith({ account_id: 1, locked: false }));
  });

  it("shows account lock state and keeps manual switch available", async () => {
    vi.mocked(getServerUpstreams).mockResolvedValue({
      total_accounts: 1,
      available_accounts: 0,
      current_account_id: 1,
      route_locked: false,
      accounts: [
        {
          id: 1,
          provider_type: "openai-compatible",
          account_name: "账号 A",
          source_icon: "openai",
          auth_mode: "api_key",
          base_url: "https://upstream-a.example/v1",
          status: "active",
          available: false,
          current: true,
          preferred: false,
          account_locked: true,
          supports_responses: true,
          balance: 10,
          quota_remaining: 1000,
          rpm_remaining: 20,
          tpm_remaining: 3000,
          health_score: 0.9,
          recent_error_rate: 0,
          last_total_tokens: 120,
          last_input_tokens: 40,
          last_output_tokens: 80,
          model_context_window: 0,
          primary_used_percent: 0,
          secondary_used_percent: 0,
        },
      ],
    });

    render(<UserPoolPage t={(value) => value} />);

    expect(await screen.findByText("已锁定")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "切换-账号 A" })).not.toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "解锁-账号 A" }));
    await waitFor(() => expect(updateServerUpstreamLock).toHaveBeenCalledWith(1, false));
  });
});
