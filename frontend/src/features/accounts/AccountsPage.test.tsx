import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";

import { refreshDesktopTrayState } from "../../lib/desktop-shell";
import { AccountsPage } from "./AccountsPage";

vi.mock("../../lib/desktop-shell", () => ({
  refreshDesktopTrayState: vi.fn(),
}));

describe("AccountsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("supports official upload, third-party create, and chat test in a single dashboard", async () => {
    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "mirror-east",
        auth_mode: "api_key",
        base_url: "https://code.ppchat.vip/v1",
        status: "active",
        is_active: false,
        priority: 2,
        balance: 12.5,
        quota_remaining: 5000,
        rpm_remaining: 90,
        tpm_remaining: 80000,
        health_score: 0.93,
        recent_error_rate: 0.01,
        last_total_tokens: 0,
        last_input_tokens: 0,
        last_output_tokens: 0,
        model_context_window: 0,
        primary_used_percent: 0,
        secondary_used_percent: 0,
      },
    ];

    const listResponse = () =>
      new Response(JSON.stringify(accountList), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts" && (!init?.method || init.method === "GET")) {
        return Promise.resolve(listResponse());
      }
      if (url === "/ai-router/api/accounts/usage" && (!init?.method || init.method === "GET")) {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/accounts/import-current" && init?.method === "POST") {
        return Promise.resolve(new Response(null, { status: 201 }));
      }
      if (url === "/ai-router/api/accounts" && init?.method === "POST") {
        return Promise.resolve(new Response(null, { status: 201 }));
      }
      if (url === "/ai-router/api/accounts/1" && init?.method === "PUT") {
        return Promise.resolve(new Response(null, { status: 200 }));
      }
      if (url === "/ai-router/api/accounts/1/test" && init?.method === "POST") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              ok: true,
              message: "远端连通性测试成功",
              details: "模型 gpt-5.4 已返回响应",
              content: "pong",
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    render(<AccountsPage />);

    expect(await screen.findByText("mirror-east")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "账户列表" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /添加账户/ }));
    fireEvent.click(await screen.findByText("官方账户"));

    const officialModal = await screen.findByRole("dialog", { name: "添加官方账户" });
    fireEvent.change(within(officialModal).getByLabelText("账户名称"), {
      target: { value: "current-codex" },
    });
    fireEvent.click(within(officialModal).getByRole("button", { name: /导\s*入/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/import-current",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ account_name: "current-codex" }),
        }),
      );
    });

    fireEvent.click(screen.getByRole("button", { name: /添加账户/ }));
    fireEvent.click(await screen.findByText("第三方账户"));

    const thirdPartyModal = await screen.findByRole("dialog", { name: "添加第三方账户" });
    fireEvent.change(within(thirdPartyModal).getByLabelText("账户名称"), {
      target: { value: "ppchat-main" },
    });
    fireEvent.change(within(thirdPartyModal).getByLabelText("接口地址"), {
      target: { value: "https://code.ppchat.vip/v1" },
    });
    fireEvent.change(within(thirdPartyModal).getByLabelText("API Key"), {
      target: { value: "sk-test" },
    });
    fireEvent.click(within(thirdPartyModal).getByRole("button", { name: /保\s*存/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({
            provider_type: "openai-compatible",
            account_name: "ppchat-main",
            auth_mode: "api_key",
            base_url: "https://code.ppchat.vip/v1",
            credential_ref: "sk-test",
            supports_responses: true,
          }),
        }),
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "编辑" }));
    const editModal = await screen.findByRole("dialog", { name: "编辑账户" });
    const responsesSwitch = within(editModal).getByLabelText("原生 /responses");
    expect(responsesSwitch).toBeInTheDocument();
    expect(within(editModal).queryByText("回退配置")).not.toBeInTheDocument();
    fireEvent.click(responsesSwitch);
    fireEvent.click(within(editModal).getByRole("button", { name: /保\s*存/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({
            account_name: "mirror-east",
            base_url: "https://code.ppchat.vip/v1",
            supports_responses: true,
          }),
        }),
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "测试" }));
    const testModal = await screen.findByRole("dialog", { name: "对话测试" });
    fireEvent.change(within(testModal).getByLabelText("输入内容"), {
      target: { value: "ping" },
    });
    fireEvent.click(within(testModal).getByRole("button", { name: "发送测试" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1/test",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ model: "gpt-5.4", input: "ping" }),
        }),
      );
    });

    expect(await within(testModal).findByText("远端连通性测试成功")).toBeInTheDocument();
    expect(within(testModal).getByText("pong")).toBeInTheDocument();
  });

  it("highlights active account and allows manual activation", async () => {
    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "account-a",
        auth_mode: "api_key",
        base_url: "https://a.example/v1",
        status: "active",
        is_active: false,
        priority: 2,
        balance: 12.5,
        quota_remaining: 5000,
        rpm_remaining: 90,
        tpm_remaining: 80000,
        health_score: 0.93,
        recent_error_rate: 0.01,
        last_total_tokens: 0,
        last_input_tokens: 0,
        last_output_tokens: 0,
        model_context_window: 0,
        primary_used_percent: 0,
        secondary_used_percent: 0,
      },
      {
        id: 2,
        provider_type: "openai-compatible",
        account_name: "account-b",
        auth_mode: "api_key",
        base_url: "https://b.example/v1",
        status: "active",
        is_active: true,
        priority: 1,
        balance: 12.5,
        quota_remaining: 5000,
        rpm_remaining: 90,
        tpm_remaining: 80000,
        health_score: 0.93,
        recent_error_rate: 0.01,
        last_total_tokens: 0,
        last_input_tokens: 0,
        last_output_tokens: 0,
        model_context_window: 0,
        primary_used_percent: 0,
        secondary_used_percent: 0,
      },
    ];

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts" && (!init?.method || init.method === "GET")) {
        return Promise.resolve(new Response(JSON.stringify(accountList), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/accounts/usage" && (!init?.method || init.method === "GET")) {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/accounts/1" && init?.method === "PUT") {
        return Promise.resolve(new Response(null, { status: 200 }));
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    render(<AccountsPage />);

    expect(await screen.findByText("account-a")).toBeInTheDocument();
    const activeRow = screen.getByText("account-b").closest("tr");
    expect(activeRow).toHaveClass("active-account-row");

    fireEvent.click(screen.getAllByRole("button", { name: "设为激活" })[0]);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ is_active: true }),
        }),
      );
    });
    await waitFor(() => {
      expect(refreshDesktopTrayState).toHaveBeenCalledTimes(1);
    });
  });
});
