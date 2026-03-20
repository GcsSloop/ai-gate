import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

const mockedUpdateService = vi.hoisted(() => ({
  getState: vi.fn(),
  check: vi.fn(),
  downloadAndInstall: vi.fn(),
  cancelDownload: vi.fn(),
  relaunch: vi.fn(),
}));

vi.mock("./features/updates/updateService", () => ({
  createDesktopUpdateService: vi.fn(() => mockedUpdateService),
}));

import { App } from "./App";
import {
  loadDesktopShellContext,
  refreshDesktopTrayState,
  subscribeDesktopBackendStateChanged,
} from "./lib/desktop-shell";

vi.mock("./features/accounts/AccountsPage", () => ({
  AccountsPage: ({
    syncToken,
    addModalMode,
    showAddButton,
  }: {
    syncToken?: number;
    addModalMode?: string | null;
    showAddButton?: boolean;
  }) => (
    <div>
      accounts-sync:{syncToken ?? 0};add-mode:{addModalMode ?? "none"};show-add:{String(showAddButton)}
    </div>
  ),
}));

vi.mock("./features/settings/SettingsPage", () => ({
  SettingsPage: ({ initialTab }: { initialTab?: string }) => <div>settings-page:{initialTab ?? "general"}</div>,
}));

vi.mock("./features/stats/StatsPage", () => ({
  StatsPage: () => <div>stats-page</div>,
}));

vi.mock("./lib/desktop-shell", () => ({
  loadDesktopShellContext: vi.fn(),
  refreshDesktopTrayState: vi.fn(),
  subscribeDesktopBackendStateChanged: vi.fn(),
}));

class MockEventSource {
  static instances: MockEventSource[] = [];

  url: string;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  close = vi.fn();

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  emit(data: string) {
    this.onmessage?.(new MessageEvent("message", { data }));
  }
}

describe("App", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    MockEventSource.instances = [];
    mockedUpdateService.getState.mockResolvedValue({ status: "idle", update: null, progress: null, error: null });
    mockedUpdateService.check.mockResolvedValue({ supported: true, update: null });
    mockedUpdateService.downloadAndInstall.mockResolvedValue(undefined);
    mockedUpdateService.cancelDownload.mockResolvedValue(undefined);
    mockedUpdateService.relaunch.mockResolvedValue(undefined);
    vi.stubGlobal("EventSource", MockEventSource as unknown as typeof EventSource);
    vi.mocked(loadDesktopShellContext).mockResolvedValue({
      backend_addr: "127.0.0.1:6789",
      backend_api_base: "http://127.0.0.1:6789/ai-router/api",
      launch_at_login: false,
      silent_start: false,
      close_to_tray: true,
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("refreshes accounts immediately when backend account state events arrive", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: false,
                status_refresh_interval_seconds: 3600,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: true,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.instances[0]?.url).toBe("http://127.0.0.1:6789/ai-router/api/dashboard/state-events");

    await act(async () => {
      MockEventSource.instances[0]?.emit("accounts-routing-changed");
    });

    await waitFor(() => {
      expect(screen.getByText(/accounts-sync:1/)).toBeInTheDocument();
    });
  });

  it("shows a themed home update indicator when a new version is available", async () => {
    mockedUpdateService.check.mockResolvedValue({
      supported: true,
      update: {
        body: "Bug fixes",
        currentVersion: "1.0.0",
        date: "2026-03-13T12:00:00Z",
        version: "1.0.1",
      },
    });
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    const updateButton = await screen.findByLabelText("打开更新");
    expect(updateButton).toBeInTheDocument();
    expect(updateButton.querySelector(".top-home-update-dot")).not.toBeNull();
  });

  it("offers shared account import from the global add menu", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: false,
                status_refresh_interval_seconds: 3600,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: true,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText("添加账户"));
    fireEvent.click(await screen.findByText("导入账户"));

    await waitFor(() => {
      expect(screen.getByText(/add-mode:shared_import/)).toBeInTheDocument();
    });
  });

  it("checks for home updates every hour when the indicator is enabled", async () => {
    vi.useFakeTimers();
    mockedUpdateService.check.mockResolvedValue({ supported: true, update: null });
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);
    await act(async () => {
      await Promise.resolve();
    });

    expect(screen.getByText(/accounts-sync:0/)).toBeInTheDocument();
    expect(mockedUpdateService.check).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(60 * 60 * 1_000);
      await Promise.resolve();
    });

    expect(mockedUpdateService.check).toHaveBeenCalledTimes(2);
  });

  it("switches to the stats page from the top navigation", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "统计" }));
    expect(screen.getByText("stats-page")).toBeInTheDocument();
  });

  it("renders accounts, stats, and settings as shared text tabs", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByRole("tab", { name: "账户" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "统计" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "设置" })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "演示" })).not.toBeInTheDocument();
    expect(screen.getByRole("tablist", { name: "主导航" })).toBeInTheDocument();
    expect(document.querySelector(".top-menu-title")).toBeNull();

    fireEvent.click(screen.getByRole("tab", { name: "设置" }));
    expect(await screen.findByText("settings-page:general")).toBeInTheDocument();
  });

  it("does not start home update checks when the setting is disabled", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: false,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    expect(screen.queryByLabelText("打开更新")).not.toBeInTheDocument();
    expect(mockedUpdateService.check).not.toHaveBeenCalled();
  });

  it("refreshes proxy status and account sync token with the configured cadence", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
        return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              launch_at_login: false,
              silent_start: false,
              close_to_tray: true,
              show_proxy_switch_on_home: true,
              show_home_update_indicator: false,
              status_refresh_interval_seconds: 5,
              proxy_host: "127.0.0.1",
              proxy_port: 6789,
              auto_failover_enabled: false,
              auto_backup_interval_hours: 24,
              backup_retention_count: 10,
              audit_limit_message: 200,
              audit_limit_function_call: 100,
              audit_limit_function_call_output: 100,
              audit_limit_reasoning: 40,
              audit_limit_custom_tool_call: 100,
              audit_limit_custom_tool_call_output: 100,
              language: "zh-CN",
              theme_mode: "system",
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    await act(async () => {
      await Promise.resolve();
    });

    expect(screen.getByText(/accounts-sync:0/)).toBeInTheDocument();
    expect(vi.mocked(refreshDesktopTrayState)).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await Promise.resolve();
    });

    expect(screen.getByText(/accounts-sync:1/)).toBeInTheDocument();
    expect(vi.mocked(refreshDesktopTrayState)).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenCalledWith("http://127.0.0.1:6789/ai-router/api/settings/proxy/status");
  });

  it("opens an update modal when the home update indicator is clicked", async () => {
    mockedUpdateService.check.mockResolvedValue({
      supported: true,
      update: {
        body: "Bug fixes",
        currentVersion: "1.0.0",
        date: "2026-03-13T12:00:00Z",
        version: "1.0.1",
      },
    });
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    fireEvent.click(await screen.findByLabelText("打开更新"));

    expect(await screen.findByRole("dialog", { name: "应用更新" })).toBeInTheDocument();
    expect(screen.queryByText("settings-page:about")).not.toBeInTheDocument();
    expect(screen.getAllByText("应用更新")).toHaveLength(1);
    expect(screen.queryByText("从 GitHub Release 检查、下载并安装最新桌面版本。")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "检查更新" })).not.toBeInTheDocument();
    expect(screen.getByText("当前版本")).toBeInTheDocument();
    expect(screen.getByText("1.0.0")).toBeInTheDocument();
    expect(screen.getByText("目标版本")).toBeInTheDocument();
    expect(screen.getByText("1.0.1")).toBeInTheDocument();
    expect(screen.getByText("发布时间")).toBeInTheDocument();
    expect(screen.getByText("Bug fixes")).toBeInTheDocument();
  });

  it("checks once more when the user switches back to the accounts page", async () => {
    mockedUpdateService.check.mockResolvedValue({ supported: true, update: null });
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    expect(mockedUpdateService.check).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("tab", { name: "设置" }));
    expect(await screen.findByText("settings-page:general")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "账户" }));
    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();

    await waitFor(() => {
      expect(mockedUpdateService.check).toHaveBeenCalledTimes(2);
    });
  });

  it("hides the home proxy switch when app settings disable it", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: false,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    expect(screen.getByText("AI Gate")).toBeInTheDocument();
    expect(screen.queryByText("开启代理")).not.toBeInTheDocument();
  });

  it("renders top-level copy in English when the saved language is en-US", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "en-US",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    expect(document.documentElement.lang).toBe("en-US");
    expect(screen.getByRole("tablist", { name: "Primary navigation" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Account" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Settings" })).toBeInTheDocument();
    expect(screen.getByText("Proxy")).toBeInTheDocument();
    expect(screen.getByLabelText("Add account")).toBeInTheDocument();
  });

  it("applies dark theme immediately when the saved mode is dark", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "dark",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    expect(document.querySelector('[data-theme-mode="dark"]')).toBeInTheDocument();
    expect(document.body.dataset.themeMode).toBe("dark");
    expect(document.body.dataset.themePreference).toBe("dark");
  });

  it("refreshes tray state once after app bootstrap", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    await waitFor(() => {
      expect(refreshDesktopTrayState).toHaveBeenCalledTimes(1);
    });
  });

  it("refreshes tray state after toggling proxy from the page", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status" && (!init?.method || init.method === "GET")) {
          return Promise.resolve(
            new Response(JSON.stringify({ enabled: false }), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app" && (!init?.method || init.method === "GET")) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/enable" && init?.method === "POST") {
          return Promise.resolve(
            new Response(JSON.stringify({ enabled: true }), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    const toggle = await screen.findByRole("switch");
    fireEvent.click(toggle);

    await waitFor(() => {
      expect(refreshDesktopTrayState).toHaveBeenCalledTimes(1);
    });
  });

  it("retries bootstrapping app settings until the backend becomes ready", async () => {
    let appSettingsAttempts = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(
            new Response(JSON.stringify({ enabled: false }), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          appSettingsAttempts += 1;
          if (appSettingsAttempts === 1) {
            return Promise.resolve(new Response(null, { status: 503 }));
          }
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(screen.getByText("正在载入设置中心…")).toBeInTheDocument();

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    expect(appSettingsAttempts).toBe(2);
  });

  it("refreshes page state when the desktop shell reports backend changes", async () => {
    let proxyEnabled = false;
    let backendStateChanged: (() => void) | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status" && (!init?.method || init.method === "GET")) {
          return Promise.resolve(
            new Response(JSON.stringify({ enabled: proxyEnabled }), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app" && (!init?.method || init.method === "GET")) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockImplementation(async (handler: () => void) => {
      backendStateChanged = handler;
      return () => {};
    });

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    expect(screen.getByRole("switch")).toHaveAttribute("aria-checked", "false");

    proxyEnabled = true;
    await act(async () => {
      backendStateChanged?.();
    });

    await waitFor(() => {
      expect(screen.getByText(/accounts-sync:1/)).toBeInTheDocument();
      expect(screen.getByRole("switch")).toHaveAttribute("aria-checked", "true");
    });
  });

  it("triggers an immediate usage refresh when the browser comes back online", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: false,
                status_refresh_interval_seconds: 3600,
                usage_request_timeout_seconds: 15,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: true,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();

    await act(async () => {
      window.dispatchEvent(new Event("online"));
    });

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        "http://127.0.0.1:6789/ai-router/api/accounts/usage/refresh",
        expect.objectContaining({ method: "POST" }),
      );
      expect(screen.getByText(/accounts-sync:1/)).toBeInTheDocument();
    });
  });

  it("triggers an immediate usage refresh when the page becomes visible after a long hidden period", async () => {
    let currentTime = new Date("2026-03-20T09:00:00Z").getTime();
    vi.spyOn(Date, "now").mockImplementation(() => currentTime);
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: false,
                status_refresh_interval_seconds: 3600,
                usage_request_timeout_seconds: 15,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: true,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();

    await act(async () => {
      Object.defineProperty(document, "visibilityState", {
        configurable: true,
        value: "hidden",
      });
      document.dispatchEvent(new Event("visibilitychange"));
      currentTime += 16_000;
      Object.defineProperty(document, "visibilityState", {
        configurable: true,
        value: "visible",
      });
      document.dispatchEvent(new Event("visibilitychange"));
    });

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith(
        "http://127.0.0.1:6789/ai-router/api/accounts/usage/refresh",
        expect.objectContaining({ method: "POST" }),
      );
      expect(screen.getByText(/accounts-sync:1/)).toBeInTheDocument();
    });
  });

  it("switches between settings and accounts with the shared header tabs", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(new Response(JSON.stringify({ enabled: false }), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "设置" }));
    expect(await screen.findByText("settings-page:general")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "账户" }));
    expect(await screen.findByText(/accounts-sync:0/)).toBeInTheDocument();
  });

  it("uses global add button and does not render backup label", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/proxy/status") {
          return Promise.resolve(
            new Response(JSON.stringify({ enabled: true, last_backup_id: "backup-123" }), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/settings/app") {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                launch_at_login: false,
                silent_start: false,
                close_to_tray: true,
                show_proxy_switch_on_home: true,
                show_home_update_indicator: true,
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
                audit_limit_message: 200,
                audit_limit_function_call: 100,
                audit_limit_function_call_output: 100,
                audit_limit_reasoning: 40,
                audit_limit_custom_tool_call: 100,
                audit_limit_custom_tool_call_output: 100,
                language: "zh-CN",
                theme_mode: "system",
              }),
              { status: 200, headers: { "Content-Type": "application/json" } },
            ),
          );
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "http://127.0.0.1:6789/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);
    expect(await screen.findByText(/show-add:false/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "添加账户" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "官方账户" }));
    expect(await screen.findByText(/add-mode:official/)).toBeInTheDocument();
    expect(screen.queryByText(/备份:/)).not.toBeInTheDocument();
  });
});
