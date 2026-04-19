import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { App as AntApp, ConfigProvider } from "antd";

import {
  refreshDesktopTrayState,
  isDesktopShell,
  openExternalUrl,
  writeDesktopClipboardText,
} from "../../lib/desktop-shell";
import { AccountsPage } from "./AccountsPage";

vi.mock("../../lib/desktop-shell", () => ({
  refreshDesktopTrayState: vi.fn(),
  openExternalUrl: vi.fn(),
  writeDesktopClipboardText: vi.fn(),
  isDesktopShell: vi.fn(() => false),
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
    vi.mocked(isDesktopShell).mockReturnValue(false);
  });

  it("supports official oauth branch and keeps local import as a separate branch", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/auth/authorize" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              authorization_url: "https://auth.openai.com/codex/device",
              state: "state-1",
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      if (
        url === "/ai-router/api/accounts/import-current" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(new Response(null, { status: 201 }));
      }
      if (
        url === "/ai-router/api/accounts/usage/refresh" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderAccountsPage();

    fireEvent.click(await screen.findByRole("button", { name: /添加账户/ }));
    fireEvent.click(await screen.findByText("官方账户"));

    const officialModal = await screen.findByRole("dialog", {
      name: "添加官方账户",
    });

    fireEvent.click(within(officialModal).getByRole("button", { name: "OAuth 登录" }));
    fireEvent.click(
      within(officialModal).getByRole("button", { name: "打开 OAuth 登录页" }),
    );
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/auth/authorize",
        expect.objectContaining({ method: "POST" }),
      );
      expect(openExternalUrl).toHaveBeenCalledWith(
        "https://auth.openai.com/codex/device",
      );
    });

    fireEvent.click(
      within(officialModal).getByRole("button", {
        name: /导\s*入已授权账号/,
      }),
    );
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/import-current",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ account_name: "local-codex" }),
        }),
      );
    });

    fireEvent.click(await screen.findByRole("button", { name: /添加账户/ }));
    fireEvent.click(await screen.findByText("官方账户"));
    const reopenedModal = await screen.findByRole("dialog", {
      name: "添加官方账户",
    });
    fireEvent.click(
      within(reopenedModal).getByRole("button", { name: "导入本地" }),
    );
    expect(
      within(reopenedModal).getByRole("button", { name: /导\s*入$/ }),
    ).toBeInTheDocument();
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
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(listResponse());
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/import-current" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(new Response(null, { status: 201 }));
      }
      if (
        url === "/ai-router/api/accounts/usage/refresh" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(new Response(null, { status: 204 }));
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
      if (
        url.startsWith("/ai-router/api/accounts/1/ppchat-token-logs") &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              success: true,
              data: {
                logs: [],
                pagination: {
                  page: 1,
                  page_size: 10,
                  total: 0,
                  total_pages: 0,
                },
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
    expect(
      screen.getByRole("heading", { name: "账户列表" }),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /添加账户/ }));
    fireEvent.click(await screen.findByText("官方账户"));

    const officialModal = await screen.findByRole("dialog", {
      name: "添加官方账户",
    });
    expect(officialModal.closest(".ant-modal-wrap")).toHaveClass(
      "ant-modal-centered",
    );
    fireEvent.change(within(officialModal).getByLabelText("账户名称"), {
      target: { value: "current-codex" },
    });
    fireEvent.click(
      within(officialModal).getByRole("button", {
        name: /导\s*入已授权账号/,
      }),
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/import-current",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ account_name: "current-codex" }),
        }),
      );
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/usage/refresh",
        expect.objectContaining({ method: "POST" }),
      );
    });

    fireEvent.click(screen.getByRole("button", { name: /添加账户/ }));
    fireEvent.click(await screen.findByText("第三方账户"));

    const thirdPartyModal = await screen.findByRole("dialog", {
      name: "添加第三方账户",
    });
    expect(
      within(thirdPartyModal).queryByLabelText("原生 /responses"),
    ).not.toBeInTheDocument();
    fireEvent.change(within(thirdPartyModal).getByLabelText("账户名称"), {
      target: { value: "ppchat-main" },
    });
    fireEvent.change(within(thirdPartyModal).getByLabelText("接口地址"), {
      target: { value: "https://code.ppchat.vip/v1" },
    });
    fireEvent.change(within(thirdPartyModal).getByLabelText("API Key"), {
      target: { value: "sk-test" },
    });
    fireEvent.click(
      within(thirdPartyModal).getByRole("button", { name: /保\s*存/ }),
    );

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
    expect(
      within(editModal).queryByLabelText("原生 /responses"),
    ).not.toBeInTheDocument();
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
            usage_driver: "",
            usage_config_json: "",
            supports_responses: true,
          }),
        }),
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "详情-mirror-east" }));
    const detailModal = await screen.findByRole("dialog", { name: "账户详情" });
    expect(
      await within(detailModal).findByText("今日配额进度"),
    ).toBeInTheDocument();
    expect(within(detailModal).getByText("当天增加配额")).toBeInTheDocument();
    expect(within(detailModal).getByText("今日已用次数")).toBeInTheDocument();
    expect(within(detailModal).getByText("剩余 13,931")).toBeInTheDocument();
    expect(
      within(detailModal).getByText("已用 1,068 / 新增 14,999"),
    ).toBeInTheDocument();
    expect(
      within(detailModal).queryByText("TOKEN 名称"),
    ).not.toBeInTheDocument();
    expect(within(detailModal).queryByText("套餐类型")).not.toBeInTheDocument();
    expect(within(detailModal).queryByText("到期时间")).not.toBeInTheDocument();
    expect(
      within(detailModal).queryByText("今日 OPUS 使用次数"),
    ).not.toBeInTheDocument();
    expect(
      within(detailModal).queryByText("今日大TOKEN请求数"),
    ).not.toBeInTheDocument();
    expect(
      await within(detailModal).findByText("PPChat Token 日志"),
    ).toBeInTheDocument();
    fireEvent.click(within(detailModal).getByRole("button", { name: "Close" }));

    fireEvent.click(screen.getByRole("button", { name: "编辑-mirror-east" }));
    const editTestModal = await screen.findByRole("dialog", {
      name: "编辑账户",
    });
    fireEvent.change(within(editTestModal).getByLabelText("输入内容"), {
      target: { value: "ping" },
    });
    fireEvent.click(
      within(editTestModal).getByRole("button", { name: /测\s*试/ }),
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1/test",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ model: "gpt-5.4", input: "ping" }),
        }),
      );
    });

    expect(
      await within(editTestModal).findByText("远端连通性测试成功"),
    ).toBeInTheDocument();
    expect(within(editTestModal).getByText("pong")).toBeInTheDocument();
  });

  it("confirms before sharing and only copies after explicit approval", async () => {
    const clipboardWriteText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(window.navigator, "clipboard", {
      value: { writeText: clipboardWriteText },
      configurable: true,
    });

    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "mirror-east",
        source_icon: "ppchat",
        auth_mode: "api_key",
        base_url: "https://code.ppchat.vip/v1",
        account_driver: "builtin_api_key",
        usage_driver: "lua",
        usage_config_json: '{"script":"adapters/vendor.lua"}',
        status: "active",
        is_active: false,
        priority: 2,
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
    ];

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/1/share" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(
          new Response(
            JSON.stringify({ payload: '{"kind":"aigate-account-share"}' }),
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

    renderAccountsPage();

    expect(await screen.findByText("mirror-east")).toBeInTheDocument();
    fetchMock.mockClear();

    fireEvent.click(screen.getByRole("button", { name: "分享-mirror-east" }));
    const confirmModal = await screen.findByRole("dialog", {
      name: "分享账户",
    });
    fireEvent.click(
      within(confirmModal).getByRole("button", { name: /取\s*消/ }),
    );

    await waitFor(() => {
      expect(clipboardWriteText).not.toHaveBeenCalled();
      expect(fetchMock).not.toHaveBeenCalled();
    });

    fireEvent.click(screen.getByRole("button", { name: "分享-mirror-east" }));
    const approvedModal = await screen.findByRole("dialog", {
      name: "分享账户",
    });
    fireEvent.click(
      within(approvedModal).getByRole("button", { name: "确认分享" }),
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1/share",
        expect.objectContaining({ method: "POST" }),
      );
      expect(clipboardWriteText).toHaveBeenCalledWith(
        '{"kind":"aigate-account-share"}',
      );
    });
  });

  it("routes shared account copy through desktop shell clipboard in desktop mode", async () => {
    vi.mocked(isDesktopShell).mockReturnValue(true);
    const desktopClipboardWrite = vi.mocked(writeDesktopClipboardText);
    desktopClipboardWrite.mockResolvedValue(undefined);
    const clipboardWriteText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(window.navigator, "clipboard", {
      value: { writeText: clipboardWriteText },
      configurable: true,
    });
    const shellWindow = window as Window & {
      __TAURI__?: unknown;
      __TAURI_INTERNALS__?: unknown;
    };
    shellWindow.__TAURI__ = {};

    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "mirror-east",
        source_icon: "ppchat",
        auth_mode: "api_key",
        base_url: "https://code.ppchat.vip/v1",
        account_driver: "builtin_api_key",
        usage_driver: "lua",
        usage_config_json: '{"script":"adapters/vendor.lua"}',
        status: "active",
        is_active: false,
        priority: 2,
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
    ];

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/1/share" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(
          new Response(
            JSON.stringify({ payload: '{"kind":"aigate-account-share"}' }),
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

    renderAccountsPage();

    expect(await screen.findByText("mirror-east")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "分享-mirror-east" }));
    const confirmModal = await screen.findByRole("dialog", {
      name: "分享账户",
    });
    fireEvent.click(
      within(confirmModal).getByRole("button", { name: "确认分享" }),
    );

    await waitFor(() => {
      expect(desktopClipboardWrite).toHaveBeenCalledWith(
        '{"kind":"aigate-account-share"}',
      );
      expect(clipboardWriteText).not.toHaveBeenCalled();
    });

    delete shellWindow.__TAURI__;
  });

  it("routes lua skill copy through desktop shell clipboard in desktop mode", async () => {
    vi.mocked(isDesktopShell).mockReturnValue(true);
    const desktopClipboardWrite = vi.mocked(writeDesktopClipboardText);
    desktopClipboardWrite.mockResolvedValue(undefined);
    const clipboardWriteText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(window.navigator, "clipboard", {
      value: { writeText: clipboardWriteText },
      configurable: true,
    });
    const shellWindow = window as Window & {
      __TAURI__?: unknown;
      __TAURI_INTERNALS__?: unknown;
    };
    shellWindow.__TAURI__ = {};

    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "mirror-east",
        source_icon: "ppchat",
        auth_mode: "api_key",
        base_url: "https://code.ppchat.vip/v1",
        account_driver: "builtin_api_key",
        usage_driver: "lua",
        usage_config_json: '{"script":"adapters/vendor.lua"}',
        status: "active",
        is_active: false,
        priority: 2,
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
    ];

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage-scripts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify({ items: ["adapters/vendor.lua"] }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage-scripts/adapters%2Fvendor.lua" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              key: "adapters/vendor.lua",
              content: "return {}",
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

    renderAccountsPage();

    expect(await screen.findByText("mirror-east")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "编辑-mirror-east" }));
    const editModal = await screen.findByRole("dialog", { name: "编辑账户" });
    fireEvent.click(
      await within(editModal).findByRole("button", { name: "复制 AI Skill" }),
    );

    await waitFor(() => {
      expect(desktopClipboardWrite).toHaveBeenCalledTimes(1);
      expect(clipboardWriteText).not.toHaveBeenCalled();
    });

    delete shellWindow.__TAURI__;
  });

  it("falls back to document copy when navigator clipboard is unavailable", async () => {
    Object.defineProperty(window.navigator, "clipboard", {
      value: undefined,
      configurable: true,
    });
    const originalExecCommand = document.execCommand;
    Object.defineProperty(document, "execCommand", {
      value: vi.fn(() => true),
      configurable: true,
    });
    const execCommand = document.execCommand as unknown as ReturnType<
      typeof vi.fn
    >;

    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "mirror-east",
        source_icon: "ppchat",
        auth_mode: "api_key",
        base_url: "https://code.ppchat.vip/v1",
        account_driver: "builtin_api_key",
        usage_driver: "lua",
        usage_config_json: '{"script":"adapters/vendor.lua"}',
        status: "active",
        is_active: false,
        priority: 2,
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
    ];

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/1/share" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(
          new Response(
            JSON.stringify({ payload: '{"kind":"aigate-account-share"}' }),
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

    renderAccountsPage();

    expect(await screen.findByText("mirror-east")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "分享-mirror-east" }));
    const confirmModal = await screen.findByRole("dialog", {
      name: "分享账户",
    });
    fireEvent.click(
      within(confirmModal).getByRole("button", { name: "确认分享" }),
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1/share",
        expect.objectContaining({ method: "POST" }),
      );
      expect(execCommand).toHaveBeenCalledWith("copy");
    });

    Object.defineProperty(document, "execCommand", {
      value: originalExecCommand,
      configurable: true,
    });
  });

  it("keeps only one account action tray visible at a time", async () => {
    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "mirror-east",
        source_icon: "openai",
        auth_mode: "api_key",
        base_url: "https://one.example/v1",
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
        id: 2,
        provider_type: "openai-compatible",
        account_name: "mirror-west",
        source_icon: "claude_code",
        auth_mode: "api_key",
        base_url: "https://two.example/v1",
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
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    renderAccountsPage();

    expect(await screen.findByText("mirror-east")).toBeInTheDocument();
    const firstCard = screen
      .getByText("mirror-east")
      .closest(".account-card-item") as HTMLElement;
    const secondCard = screen
      .getByText("mirror-west")
      .closest(".account-card-item") as HTMLElement;

    fireEvent.focus(screen.getByRole("button", { name: "编辑-mirror-east" }));
    expect(firstCard.dataset.actionsVisible).toBe("true");
    expect(secondCard.dataset.actionsVisible).toBe("false");

    fireEvent.mouseEnter(secondCard);
    expect(firstCard.dataset.actionsVisible).toBe("false");
    expect(secondCard.dataset.actionsVisible).toBe("true");

    fireEvent.mouseLeave(secondCard);
    expect(secondCard.dataset.actionsVisible).toBe("false");
  });

  it("supports editing lua usage config, copying AI skill context, and testing lua output", async () => {
    const clipboardWriteText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(window.navigator, "clipboard", {
      value: { writeText: clipboardWriteText },
      configurable: true,
    });

    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "vendor-lua",
        source_icon: "openai",
        auth_mode: "api_key",
        base_url: "https://mirror.example.test/v1",
        account_driver: "builtin_api_key",
        usage_driver: "lua",
        usage_config_json:
          '{"script":"managed:vendor_shared","endpoint":"https://usage.example.test/v1/usage"}',
        status: "active",
        is_active: false,
        priority: 1,
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
    ];

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage-scripts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify({ items: ["vendor_shared"] }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage-scripts/vendor_shared" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              key: "vendor_shared",
              content:
                "function fetch_usage(ctx)\n  return { ok = true, source = 'remote', confidence = 'high', limits = { quota_remaining = 123 } }\nend\n",
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage-scripts/vendor_shared" &&
        init?.method === "PUT"
      ) {
        return Promise.resolve(
          new Response(JSON.stringify({ key: "vendor_shared" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/1/usage-lua-test" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              ok: true,
              message: "Lua usage 测试成功",
              details: "脚本已返回标准化 usage 结果",
              content:
                '{\n  "quota_remaining": 4321,\n  "meta": {\n    "account_name": "vendor-lua"\n  }\n}',
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      if (url === "/ai-router/api/accounts/1" && init?.method === "PUT") {
        return Promise.resolve(new Response(null, { status: 200 }));
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    renderAccountsPage();

    expect(await screen.findByText("vendor-lua")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "编辑-vendor-lua" }));

    const editModal = await screen.findByRole("dialog", { name: "编辑账户" });
    expect(
      await within(editModal).findByText("Lua Usage 配置"),
    ).toBeInTheDocument();
    expect(
      within(editModal).queryByLabelText("Usage 配置 JSON"),
    ).not.toBeInTheDocument();
    expect(
      within(editModal).getByText("当前脚本标识: vendor_shared"),
    ).toBeInTheDocument();
    expect(
      (within(editModal).getByLabelText("Lua 脚本") as HTMLTextAreaElement)
        .value,
    ).toContain("fetch_usage");

    fireEvent.click(
      await within(editModal).findByRole("button", { name: "复制 AI Skill" }),
    );
    await waitFor(() => {
      expect(clipboardWriteText).toHaveBeenCalledTimes(1);
    });
    expect(clipboardWriteText.mock.calls[0][0]).toContain("账户上下文");
    expect(clipboardWriteText.mock.calls[0][0]).toContain(
      '"account_name": "vendor-lua"',
    );
    expect(clipboardWriteText.mock.calls[0][0]).toContain(
      '"script_key": "vendor_shared"',
    );
    expect(clipboardWriteText.mock.calls[0][0]).toContain(
      "http://127.0.0.1:6789",
    );

    fireEvent.click(
      within(editModal).getByRole("button", { name: "高级配置" }),
    );
    expect(
      await within(editModal).findByLabelText("Usage 配置 JSON"),
    ).toBeInTheDocument();

    fireEvent.click(
      within(editModal).getByRole("button", { name: "测试 Lua 脚本" }),
    );
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1/usage-lua-test",
        expect.objectContaining({
          method: "POST",
        }),
      );
    });

    expect(
      await within(editModal).findByText("Lua usage 测试成功"),
    ).toBeInTheDocument();
    expect(
      within(editModal)
        .getAllByText(/quota_remaining/)
        .some((node) => node.classList.contains("test-output")),
    ).toBe(true);

    fireEvent.click(within(editModal).getByRole("button", { name: /保\s*存/ }));
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/usage-scripts/vendor_shared",
        expect.objectContaining({
          method: "PUT",
        }),
      );
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/1",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({
            account_name: "vendor-lua",
            source_icon: "openai",
            base_url: "https://mirror.example.test/v1",
            usage_driver: "lua",
            usage_config_json:
              '{"script":"managed:vendor_shared","endpoint":"https://usage.example.test/v1/usage"}',
            supports_responses: true,
          }),
        }),
      );
    });
  });

  it("imports shared payload and keeps the modal open on validation failure", async () => {
    const validPayload = '{"kind":"aigate-account-share","schema_version":1}';

    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "mirror-east",
        source_icon: "ppchat",
        auth_mode: "api_key",
        base_url: "https://code.ppchat.vip/v1",
        account_driver: "builtin_api_key",
        usage_driver: "lua",
        usage_config_json: '{"script":"adapters/vendor.lua"}',
        status: "active",
        is_active: false,
        priority: 2,
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
    ];

    let importAttempt = 0;
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/import-shared" &&
        init?.method === "POST"
      ) {
        importAttempt += 1;
        if (importAttempt === 1) {
          return Promise.resolve(new Response("分享内容无效", { status: 400 }));
        }
        return Promise.resolve(new Response(null, { status: 201 }));
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    renderAccountsPage();

    expect(await screen.findByText("mirror-east")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /添加账户/ }));
    fireEvent.click(await screen.findByText("导入账户"));

    const importModal = await screen.findByRole("dialog", { name: "导入账户" });
    fireEvent.change(within(importModal).getByLabelText("粘贴分享内容"), {
      target: { value: validPayload },
    });
    fireEvent.click(
      within(importModal).getByRole("button", { name: "校验并导入" }),
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/import-shared",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ payload: validPayload }),
        }),
      );
    });
    expect(
      await within(importModal).findByText("分享内容无效"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("dialog", { name: "导入账户" }),
    ).toBeInTheDocument();

    fireEvent.click(
      within(importModal).getByRole("button", { name: "校验并导入" }),
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/import-shared",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ payload: validPayload }),
        }),
      );
    });

    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "导入账户" }),
      ).not.toBeInTheDocument();
    });
  });

  it("centers the delete confirmation modal vertically", async () => {
    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "mirror-east",
        source_icon: "openai",
        auth_mode: "api_key",
        base_url: "https://api.example/v1",
        status: "active",
        is_active: false,
        priority: 2,
        balance: 0,
        quota_remaining: 0,
        rpm_remaining: 0,
        tpm_remaining: 0,
        health_score: 100,
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
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    renderAccountsPage();

    expect(await screen.findByText("mirror-east")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "删除-mirror-east" }));
    await waitFor(() => {
      const confirmWrap = document
        .querySelector(".ant-modal-confirm")
        ?.closest(".ant-modal-wrap");
      expect(confirmWrap).not.toBeNull();
      expect(confirmWrap).toHaveClass("ant-modal-centered");
    });
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
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (url === "/ai-router/api/accounts/1" && init?.method === "PUT") {
        return Promise.resolve(new Response(null, { status: 200 }));
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    renderAccountsPage();

    expect(await screen.findByText("account-a")).toBeInTheDocument();
    const activeRow = screen
      .getByText("account-b")
      .closest(".account-card-item");
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

  it("renders cards in descending priority order from the homepage list", async () => {
    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "low-priority",
        source_icon: "openai",
        auth_mode: "api_key",
        base_url: "https://low.example/v1",
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
      {
        id: 2,
        provider_type: "openai-compatible",
        account_name: "high-priority",
        source_icon: "claude_code",
        auth_mode: "api_key",
        base_url: "https://high.example/v1",
        status: "active",
        is_active: true,
        priority: 9,
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
        account_name: "mid-priority",
        source_icon: "ppchat",
        auth_mode: "api_key",
        base_url: "https://mid.example/v1",
        status: "active",
        is_active: false,
        priority: 5,
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
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    const { container } = renderAccountsPage();

    expect(await screen.findByText("high-priority")).toBeInTheDocument();

    const order = Array.from(
      container.querySelectorAll(".account-cards .account-card-item strong"),
    ).map((node) => node.textContent);
    expect(order).toEqual(["high-priority", "mid-priority", "low-priority"]);
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
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        const payload = listCallCount === 0 ? initialList : duplicatedList;
        listCallCount += 1;
        return Promise.resolve(
          new Response(JSON.stringify(payload), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/1/duplicate" &&
        init?.method === "POST"
      ) {
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
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
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
    expect(
      document.querySelector(
        '[aria-label="official-main-5H"] .account-usage-mini-fill',
      ),
    ).toHaveClass("is-warning");
    expect(
      document.querySelector(
        '[aria-label="official-main-7D"] .account-usage-mini-fill',
      ),
    ).toHaveClass("is-danger");
  });

  it("renders fallback usage windows for lua-driven third-party accounts", async () => {
    const accountList = [
      {
        id: 35,
        provider_type: "openai-compatible",
        account_name: "lua-main",
        source_icon: "openai",
        auth_mode: "api_key",
        base_url: "https://w.ciykj.cn",
        status: "active",
        is_active: false,
        priority: 1,
        account_driver: "builtin_api_key",
        usage_driver: "lua",
        usage_config_json: '{"script":"managed:w.ciykj.cn"}',
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
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                account_id: 35,
                balance: 0,
                quota_remaining: 29994,
                rpm_remaining: 0,
                tpm_remaining: 0,
                health_score: 1,
                recent_error_rate: 0,
                last_total_tokens: 0,
                last_input_tokens: 0,
                last_output_tokens: 0,
                model_context_window: 0,
                primary_used_percent: 23,
                secondary_used_percent: 67,
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

    expect(await screen.findByText("lua-main")).toBeInTheDocument();
    expect(await screen.findByText("P1")).toBeInTheDocument();
    expect(screen.getByText("P2")).toBeInTheDocument();
    expect(screen.getByText("77%")).toBeInTheDocument();
    expect(screen.getByText("33%")).toBeInTheDocument();
  });

  it("keeps previous usage visible while a refresh is pending", async () => {
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

    let usageCallCount = 0;
    let resolveSecondUsage: ((value: Response) => void) | null = null;

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        usageCallCount += 1;
        if (usageCallCount === 1) {
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
        return new Promise<Response>((resolve) => {
          resolveSecondUsage = resolve;
        });
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    const { rerender } = render(
      <ConfigProvider>
        <AntApp>
          <AccountsPage syncToken={0} />
        </AntApp>
      </ConfigProvider>,
    );

    expect(await screen.findByText("official-main")).toBeInTheDocument();
    expect(await screen.findByText("25%")).toBeInTheDocument();
    expect(screen.getByText("5%")).toBeInTheDocument();

    rerender(
      <ConfigProvider>
        <AntApp>
          <AccountsPage syncToken={1} />
        </AntApp>
      </ConfigProvider>,
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(4);
    });

    expect(screen.getByText("25%")).toBeInTheDocument();
    expect(screen.getByText("5%")).toBeInTheDocument();
    expect(screen.queryByText("100%")).not.toBeInTheDocument();

    resolveSecondUsage?.(
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
            primary_used_percent: 20,
            secondary_used_percent: 10,
          },
        ]),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    expect(await screen.findByText("80%")).toBeInTheDocument();
    expect(screen.getByText("90%")).toBeInTheDocument();
  });

  it("formats official reset windows with explicit time for both 5H and 7D", async () => {
    const now = new Date();
    const primaryReset = new Date(now.getTime() + 60 * 60 * 1000);
    const secondaryReset = new Date(now.getTime() + 2 * 60 * 60 * 1000);

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
        primary_used_percent: 23,
        secondary_used_percent: 67,
      },
    ];

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
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
                primary_used_percent: 23,
                secondary_used_percent: 67,
                primary_resets_at: primaryReset.toISOString(),
                secondary_resets_at: secondaryReset.toISOString(),
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
    const primaryResetTime = primaryReset.toLocaleTimeString("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    });
    const primaryResetDateTime = `${primaryReset.toLocaleDateString("zh-CN", {
      month: "numeric",
      day: "numeric",
    })} ${primaryResetTime}`;
    expect(
      screen.getByText(
        (_content, element) =>
          element?.textContent === primaryResetTime ||
          element?.textContent === primaryResetDateTime,
      ),
    ).toBeInTheDocument();
    const secondaryResetTime = secondaryReset.toLocaleTimeString("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    });
    const secondaryResetDateTime = `${secondaryReset.toLocaleDateString("zh-CN", {
      month: "numeric",
      day: "numeric",
    })} ${secondaryResetTime}`;
    expect(
      screen.getByText(
        (_content, element) =>
          element?.textContent === secondaryResetTime ||
          element?.textContent === secondaryResetDateTime,
      ),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "详情-official-main" }));

    const detailModal = await screen.findByRole("dialog", {
      name: "账户详情",
    });
    const detailPrimaryValue = `77% · ${primaryResetTime}`;
    const detailPrimaryDateTimeValue = `77% · ${primaryResetDateTime}`;
    expect(
      within(detailModal).getByText(
        (_content, element) =>
          element?.textContent === detailPrimaryValue ||
          element?.textContent === detailPrimaryDateTimeValue,
      ),
    ).toBeInTheDocument();
    const detailSecondaryValue = `33% · ${secondaryResetTime}`;
    const detailSecondaryDateTimeValue = `33% · ${secondaryResetDateTime}`;
    expect(
      within(detailModal).getByText(
        (_content, element) =>
          element?.textContent === detailSecondaryValue ||
          element?.textContent === detailSecondaryDateTimeValue,
      ),
    ).toBeInTheDocument();
  });


  it("shows usage health details from the status dot tooltip", async () => {
    const checkedAt = new Date("2026-03-19T08:35:00.000Z");
    const accountList = [
      {
        id: 1,
        provider_type: "codex",
        account_name: "official-main",
        source_icon: "openai",
        auth_mode: "codex_local_import",
        base_url: "https://chatgpt.com/backend-api/codex",
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
        primary_used_percent: 20,
        secondary_used_percent: 30,
      },
    ];

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
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
                primary_used_percent: 20,
                secondary_used_percent: 30,
                checked_at: checkedAt.toISOString(),
                stale: true,
                last_error:
                  'do_usage_request: GET "https://his.ppchat.vip/api/token-logs?page=1&page_size=1&token_key=sk-abcdefghijklmnopqrstuvwxyz123456": context deadline exceeded',
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
    fireEvent.mouseEnter(screen.getByLabelText("official-main-usage-health"));

    expect(
      await screen.findByText((content) => content.includes("token_key=sk-***")),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/token_key=sk-abcdefghijklmnopqrstuvwxyz123456/),
    ).not.toBeInTheDocument();
  });

  it("keeps usage status dot gray when usage driver is not configured", async () => {
    const checkedAt = new Date("2026-03-19T08:35:00.000Z");
    const accountList = [
      {
        id: 1,
        provider_type: "openai-compatible",
        account_name: "no-usage-driver",
        source_icon: "openai",
        auth_mode: "api_key",
        base_url: "https://example.com/v1",
        account_driver: "builtin_api_key",
        usage_driver: "",
        usage_config_json: "",
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
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
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
                primary_used_percent: 0,
                secondary_used_percent: 0,
                checked_at: checkedAt.toISOString(),
                stale: true,
                last_error: "usage refresh failed",
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

    expect(await screen.findByText("no-usage-driver")).toBeInTheDocument();
    const dot = screen.getByLabelText("no-usage-driver-usage-health");
    expect(dot).toHaveClass("is-unknown");
    expect(dot).not.toHaveClass("is-danger");
  });

  it("renders ppchat daily remaining usage meter on the card", async () => {
    const nextMidnight = new Date();
    nextMidnight.setHours(24, 0, 0, 0);
    const expectedReset = nextMidnight.toLocaleTimeString("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    });

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
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
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
                primary_used_percent: 0,
                secondary_used_percent: 0,
                ppchat_today_used_quota: 1068,
                ppchat_today_remaining_quota: 13931,
                ppchat_today_added_quota: 14999,
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

    expect(await screen.findByText("ppchat-main")).toBeInTheDocument();
    expect(await screen.findByText("1D")).toBeInTheDocument();
    expect(screen.getByText(expectedReset)).toBeInTheDocument();
    expect(screen.getByText("93%")).toBeInTheDocument();
    expect(document.querySelector(".account-usage-mini")).toHaveClass(
      "account-usage-mini-single",
    );
    expect(
      fetchMock.mock.calls.some(([url]) =>
        String(url).includes("/ai-router/api/accounts/1/ppchat-token-logs"),
      ),
    ).toBe(false);
  });

  it("refreshes usage after shared account import so imported accounts do not stay at default progress", async () => {
    const accountList = [
      {
        id: 1,
        provider_type: "openai-official",
        account_name: "shared-codex",
        source_icon: "openai",
        auth_mode: "codex_local_import",
        base_url: "https://chatgpt.com/backend-api/codex",
        status: "active",
        is_active: false,
        priority: 1,
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
    ];

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/import-shared" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(new Response(null, { status: 201 }));
      }
      if (
        url === "/ai-router/api/accounts/usage/refresh" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    renderAccountsPage();
    expect(await screen.findByText("shared-codex")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /添加账户/ }));
    fireEvent.click(await screen.findByText("导入账户"));

    const importModal = await screen.findByRole("dialog", { name: "导入账户" });
    fireEvent.change(within(importModal).getByLabelText("粘贴分享内容"), {
      target: { value: '{"kind":"aigate-account-share","schema_version":1}' },
    });
    fireEvent.click(
      within(importModal).getByRole("button", { name: "校验并导入" }),
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/accounts/usage/refresh",
        expect.objectContaining({ method: "POST" }),
      );
    });
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
      if (
        url === "/ai-router/api/accounts" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify(accountList), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        url === "/ai-router/api/accounts/usage" &&
        (!init?.method || init.method === "GET")
      ) {
        return Promise.resolve(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (
        /^\/ai-router\/api\/accounts\/\d+$/.test(url) &&
        init?.method === "PUT"
      ) {
        return Promise.resolve(new Response(null, { status: 200 }));
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });

    vi.stubGlobal("fetch", fetchMock);

    const { container } = renderAccountsPage();

    expect(await screen.findByText("account-a")).toBeInTheDocument();

    const cards = Array.from(
      container.querySelectorAll(".account-card-item"),
    ) as HTMLElement[];
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
    fireEvent.mouseMove(document.body, {
      buttons: 1,
      clientX: 24,
      clientY: 56,
    });
    fireEvent.mouseMove(document.body, {
      buttons: 1,
      clientX: 24,
      clientY: 175,
    });

    await waitFor(() => {
      expect(
        container.querySelector(".account-card-item-placeholder"),
      ).toBeTruthy();
      expect(document.body.querySelector(".account-drag-overlay")).toBeTruthy();
    });

    await waitFor(() => {
      const liveOrder = Array.from(
        container.querySelectorAll(".account-cards .account-card-item strong"),
      ).map((node) => node.textContent);
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
