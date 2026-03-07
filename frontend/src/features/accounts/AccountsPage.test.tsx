import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";

import { AccountsPage } from "./AccountsPage";

describe("AccountsPage", () => {
  it("supports official upload, third-party create, and chat test in a single dashboard", async () => {
    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "mirror-east",
        auth_mode: "api_key",
        base_url: "https://code.ppchat.vip/v1",
        status: "active",
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

    const summaryResponse = () =>
      new Response(
        JSON.stringify({
          total_conversations: 12,
          active_conversations: 4,
          total_runs: 28,
          failover_runs: 3,
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      );

    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(listResponse())
      .mockResolvedValueOnce(summaryResponse())
      .mockResolvedValueOnce(new Response(null, { status: 201 }))
      .mockResolvedValueOnce(listResponse())
      .mockResolvedValueOnce(summaryResponse())
      .mockResolvedValueOnce(new Response(null, { status: 201 }))
      .mockResolvedValueOnce(listResponse())
      .mockResolvedValueOnce(summaryResponse())
      .mockResolvedValueOnce(
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

    vi.stubGlobal("fetch", fetchMock);

    render(<AccountsPage />);

    expect(await screen.findByText("mirror-east")).toBeInTheDocument();
    expect(screen.getByText("本地 Codex 接入说明")).toBeInTheDocument();
    expect(screen.getByText(/base_url = "http:\/\/127\.0\.0\.1:6789\/ai-router\/api"/)).toBeInTheDocument();
    expect(screen.getByText("会话统计")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /添加账户/ }));
    fireEvent.click(await screen.findByText("官方账户"));

    const officialModal = await screen.findByRole("dialog", { name: "添加官方账户" });
    const file = new File(['{"auth_mode":"chatgpt","tokens":{"access_token":"token-upload"}}'], "auth.json", {
      type: "application/json",
    });
    fireEvent.change(within(officialModal).getByLabelText("选择 auth.json"), {
      target: { files: [file] },
    });
    fireEvent.click(within(officialModal).getByRole("button", { name: /导\s*入/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/import-local",
        expect.objectContaining({ method: "POST", body: expect.any(FormData) }),
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
        expect.objectContaining({ method: "POST" }),
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
});
