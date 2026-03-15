import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { App as AntApp, ConfigProvider } from "antd";

import { refreshDesktopTrayState } from "../../lib/desktop-shell";
import { AccountsPage } from "./AccountsPage";

vi.mock("../../lib/desktop-shell", () => ({
  refreshDesktopTrayState: vi.fn(),
}));

function renderAccountsPage() {
  return render(
    <ConfigProvider>
      <AntApp>
        <AccountsPage />
      </AntApp>
    </ConfigProvider>,
  );
}

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
        source_icon: "ppchat",
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
      if (url.startsWith("/ai-router/api/accounts/1/ppchat-token-logs") && (!init?.method || init.method === "GET")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              success: true,
              data: {
                logs: [],
                pagination: { page: 1, page_size: 10, total: 0, total_pages: 0 },
                token_info: {
                  name: "edwardtoday-xmax",
                  today_usage_count: 172,
                  today_used_quota: 1068,
                  remain_quota_display: 13931,
                  today_added_quota: 14999,
                  today_opus_usage: 0,
                  today_big_token_requests: 0,
                  expired_time_formatted: "2026-04-23 08:18:32",
                },
              },
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    renderAccountsPage();

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
    expect(within(thirdPartyModal).queryByLabelText("原生 /responses")).not.toBeInTheDocument();
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
            source_icon: "ppchat",
            auth_mode: "api_key",
            base_url: "https://code.ppchat.vip/v1",
            credential_ref: "sk-test",
            supports_responses: true,
          }),
        }),
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "编辑-mirror-east" }));
    const editModal = await screen.findByRole("dialog", { name: "编辑账户" });
    expect(within(editModal).queryByLabelText("原生 /responses")).not.toBeInTheDocument();
    expect(within(editModal).queryByText("回退配置")).not.toBeInTheDocument();
    fireEvent.click(within(editModal).getByRole("button", { name: /保\s*存/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({
            account_name: "mirror-east",
            source_icon: "ppchat",
            base_url: "https://code.ppchat.vip/v1",
            supports_responses: true,
          }),
        }),
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "详情-mirror-east" }));
    const detailModal = await screen.findByRole("dialog", { name: "账户详情" });
    expect(await within(detailModal).findByText("今日配额进度")).toBeInTheDocument();
    expect(within(detailModal).getByText("当天增加配额")).toBeInTheDocument();
    expect(within(detailModal).getByText("今日已用次数")).toBeInTheDocument();
    expect(within(detailModal).getByText("剩余 13,931")).toBeInTheDocument();
    expect(within(detailModal).getByText("已用 1,068 / 新增 14,999")).toBeInTheDocument();
    expect(within(detailModal).queryByText("TOKEN 名称")).not.toBeInTheDocument();
    expect(within(detailModal).queryByText("套餐类型")).not.toBeInTheDocument();
    expect(within(detailModal).queryByText("到期时间")).not.toBeInTheDocument();
    expect(within(detailModal).queryByText("今日 OPUS 使用次数")).not.toBeInTheDocument();
    expect(within(detailModal).queryByText("今日大TOKEN请求数")).not.toBeInTheDocument();
    expect(await within(detailModal).findByText("PPChat Token 日志")).toBeInTheDocument();
    fireEvent.click(within(detailModal).getByRole("button", { name: "Close" }));

    fireEvent.click(screen.getByRole("button", { name: "编辑-mirror-east" }));
    const editTestModal = await screen.findByRole("dialog", { name: "编辑账户" });
    fireEvent.change(within(editTestModal).getByLabelText("输入内容"), {
      target: { value: "ping" },
    });
    fireEvent.click(within(editTestModal).getByRole("button", { name: /测\s*试/ }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1/test",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ model: "gpt-5.4", input: "ping" }),
        }),
      );
    });

    expect(await within(editTestModal).findByText("远端连通性测试成功")).toBeInTheDocument();
    expect(within(editTestModal).getByText("pong")).toBeInTheDocument();
  });

  it("highlights active account and allows manual activation", async () => {
    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "account-a",
        source_icon: "openai",
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
        source_icon: "claude_code",
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

    renderAccountsPage();

    expect(await screen.findByText("account-a")).toBeInTheDocument();
    const activeRow = screen.getByText("account-b").closest(".account-card-item");
    expect(activeRow).toHaveClass("active-account-card");

    fireEvent.click(screen.getByRole("button", { name: "设为激活-account-a" }));
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

  it("duplicates an account with the copy action", async () => {
    const initialList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "mirror-east",
        source_icon: "ppchat",
        auth_mode: "api_key",
        base_url: "https://code.ppchat.vip/v1",
        status: "active",
        is_active: true,
        priority: 2,
        balance: 0,
        quota_remaining: 0,
        rpm_remaining: 0,
        tpm_remaining: 0,
        health_score: 1,
        recent_error_rate: 0,
        last_total_tokens: 0,
        last_input_tokens: 0,
        last_output_tokens: 0,
        model_context_window: 0,
        primary_used_percent: 0,
        secondary_used_percent: 0,
      },
    ];
    const duplicatedList = [
      initialList[0],
      {
        ...initialList[0],
        id: 2,
        account_name: "mirror-east 1",
        is_active: false,
      },
    ];
    let listCallCount = 0;

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts" && (!init?.method || init.method === "GET")) {
        const payload = listCallCount === 0 ? initialList : duplicatedList;
        listCallCount += 1;
        return Promise.resolve(new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/accounts/usage" && (!init?.method || init.method === "GET")) {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/accounts/1/duplicate" && init?.method === "POST") {
        return Promise.resolve(new Response(null, { status: 201 }));
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    renderAccountsPage();

    expect(await screen.findByText("mirror-east")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "复制-mirror-east" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1/duplicate",
        expect.objectContaining({
          method: "POST",
        }),
      );
    });

    expect(await screen.findByText("mirror-east 1")).toBeInTheDocument();
    expect(refreshDesktopTrayState).toHaveBeenCalledTimes(1);
  });

  it("renders dual official remaining meters with warning thresholds", async () => {
    const accountList = [
      {
        id: 1,
        provider_type: "codex",
        account_name: "official-main",
        source_icon: "openai",
        auth_mode: "codex_local_import",
        base_url: "",
        status: "active",
        is_active: true,
        priority: 1,
        balance: 0,
        quota_remaining: 0,
        rpm_remaining: 0,
        tpm_remaining: 0,
        health_score: 1,
        recent_error_rate: 0,
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
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                account_id: 1,
                balance: 0,
                quota_remaining: 0,
                rpm_remaining: 0,
                tpm_remaining: 0,
                health_score: 1,
                recent_error_rate: 0,
                last_total_tokens: 0,
                last_input_tokens: 0,
                last_output_tokens: 0,
                model_context_window: 0,
                primary_used_percent: 75,
                secondary_used_percent: 95,
              },
            ]),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    renderAccountsPage();

    expect(await screen.findByText("official-main")).toBeInTheDocument();
    expect(await screen.findByText("5H")).toBeInTheDocument();
    expect(screen.getByText("7D")).toBeInTheDocument();
    expect(screen.getByText("25%")).toBeInTheDocument();
    expect(screen.getByText("5%")).toBeInTheDocument();
    expect(document.querySelector('[aria-label="official-main-5H"] .account-usage-mini-fill')).toHaveClass("is-warning");
    expect(document.querySelector('[aria-label="official-main-7D"] .account-usage-mini-fill')).toHaveClass("is-danger");
  });

  it("renders ppchat daily remaining usage meter on the card", async () => {
    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "ppchat-main",
        source_icon: "ppchat",
        auth_mode: "api_key",
        base_url: "https://code.ppchat.vip/v1",
        status: "active",
        is_active: false,
        priority: 1,
        balance: 0,
        quota_remaining: 0,
        rpm_remaining: 0,
        tpm_remaining: 0,
        health_score: 1,
        recent_error_rate: 0,
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
      if (url.startsWith("/ai-router/api/accounts/1/ppchat-token-logs") && (!init?.method || init.method === "GET")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              success: true,
              data: {
                logs: [],
                pagination: { page: 1, page_size: 10, total: 0, total_pages: 0 },
                token_info: {
                  name: "ppchat-main",
                  today_usage_count: 172,
                  today_used_quota: 1068,
                  remain_quota_display: 13931,
                  today_added_quota: 14999,
                  expired_time_formatted: "2026-04-23 08:18:32",
                },
              },
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    renderAccountsPage();

    expect(await screen.findByText("ppchat-main")).toBeInTheDocument();
    expect(await screen.findByText("1D")).toBeInTheDocument();
    expect(screen.getByText("93%")).toBeInTheDocument();
  });

  it("reorders account cards during pointer drag before release", async () => {
    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "account-a",
        source_icon: "openai",
        auth_mode: "api_key",
        base_url: "https://a.example/v1",
        status: "active",
        is_active: false,
        priority: 3,
        balance: 0,
        quota_remaining: 0,
        rpm_remaining: 0,
        tpm_remaining: 0,
        health_score: 1,
        recent_error_rate: 0,
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
        source_icon: "claude_code",
        auth_mode: "api_key",
        base_url: "https://b.example/v1",
        status: "active",
        is_active: false,
        priority: 2,
        balance: 0,
        quota_remaining: 0,
        rpm_remaining: 0,
        tpm_remaining: 0,
        health_score: 1,
        recent_error_rate: 0,
        last_total_tokens: 0,
        last_input_tokens: 0,
        last_output_tokens: 0,
        model_context_window: 0,
        primary_used_percent: 0,
        secondary_used_percent: 0,
      },
      {
        id: 3,
        provider_type: "openai-compatible",
        account_name: "account-c",
        source_icon: "ppchat",
        auth_mode: "api_key",
        base_url: "https://c.example/v1",
        status: "active",
        is_active: true,
        priority: 1,
        balance: 0,
        quota_remaining: 0,
        rpm_remaining: 0,
        tpm_remaining: 0,
        health_score: 1,
        recent_error_rate: 0,
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
      if (/^\/ai-router\/api\/accounts\/\d+$/.test(url) && init?.method === "PUT") {
        return Promise.resolve(new Response(null, { status: 200 }));
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    const { container } = renderAccountsPage();

    expect(await screen.findByText("account-a")).toBeInTheDocument();

    const cards = Array.from(container.querySelectorAll(".account-card-item")) as HTMLElement[];
    cards.forEach((card, index) => {
      Object.defineProperty(card, "getBoundingClientRect", {
        configurable: true,
        value: () => ({
          x: 0,
          y: index * 100,
          top: index * 100,
          bottom: index * 100 + 100,
          left: 0,
          right: 600,
          width: 600,
          height: 100,
          toJSON: () => ({}),
        }),
      });
    });

    const handles = screen.getAllByLabelText(/拖拽排序-/);
    fireEvent.mouseDown(handles[0], { button: 0, clientX: 24, clientY: 40 });
    fireEvent.mouseMove(document.body, { buttons: 1, clientX: 24, clientY: 56 });
    fireEvent.mouseMove(document.body, { buttons: 1, clientX: 24, clientY: 175 });

    await waitFor(() => {
      expect(container.querySelector(".account-card-item-placeholder")).toBeTruthy();
      expect(document.body.querySelector(".account-drag-overlay")).toBeTruthy();
    });

    await waitFor(() => {
      const liveOrder = Array.from(container.querySelectorAll(".account-cards .account-card-item strong")).map((node) => node.textContent);
      expect(liveOrder).toEqual(["account-b", "account-a", "account-c"]);
    });
    expect(fetchMock).not.toHaveBeenCalledWith(
      "/ai-router/api/accounts/2",
      expect.objectContaining({ method: "PUT" }),
    );

    fireEvent.mouseUp(document.body);

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/2",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ priority: 3 }),
        }),
      );
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ priority: 2 }),
        }),
      );
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/3",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ priority: 1 }),
        }),
      );
    });
  });
});
