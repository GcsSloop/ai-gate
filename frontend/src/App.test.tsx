import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

import { App } from "./App";
import { loadDesktopShellContext, refreshDesktopTrayState, subscribeDesktopBackendStateChanged } from "./lib/desktop-shell";

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
  SettingsPage: () => <div>settings-page</div>,
}));

vi.mock("./lib/desktop-shell", () => ({
  loadDesktopShellContext: vi.fn(),
  refreshDesktopTrayState: vi.fn(),
  subscribeDesktopBackendStateChanged: vi.fn(),
}));

describe("App", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
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
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
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
    expect(screen.getByLabelText("Open settings")).toBeInTheDocument();
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
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
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
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
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
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
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
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
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
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
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

  it("opens settings full-screen and supports back navigation", async () => {
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
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
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
    fireEvent.click(screen.getByRole("button", { name: "打开设置" }));
    expect(await screen.findByText("settings-page")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "返回首页" }));
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
                proxy_host: "127.0.0.1",
                proxy_port: 6789,
                auto_failover_enabled: false,
                auto_backup_interval_hours: 24,
                backup_retention_count: 10,
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
