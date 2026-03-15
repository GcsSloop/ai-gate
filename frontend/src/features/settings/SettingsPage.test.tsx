import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { Modal } from "antd";

import { SettingsPage } from "./SettingsPage";
import { applyDesktopAppSettings, getAppMetadata, getRecentDesktopLogs } from "../../lib/desktop-shell";
import { setAPIBase } from "../../lib/paths";

vi.mock("../../lib/desktop-shell", () => ({
  applyDesktopAppSettings: vi.fn(),
  getAppMetadata: vi.fn(),
  getRecentDesktopLogs: vi.fn(),
}));

const baseSettings = {
  launch_at_login: false,
  silent_start: false,
  close_to_tray: true,
  show_proxy_switch_on_home: true,
  show_home_update_indicator: true,
  status_refresh_interval_seconds: 60,
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
};

describe("SettingsPage", () => {
  const identity = (value: string) => value;

  async function openBackupMenu(label = "备份操作 20260309-101500.000") {
    const trigger = await screen.findByRole("button", { name: label });
    await act(async () => {
      fireEvent.click(trigger);
    });
    await waitFor(() => {
      expect(screen.getByRole("button", { name: label })).toHaveAttribute("aria-expanded", "true");
    });
    return screen.getByRole("button", { name: label });
  }

  beforeEach(() => {
    vi.clearAllMocks();
    setAPIBase("/ai-router/api");
  });

  it("renders new settings sections and saves desktop-bound app settings", async () => {
    const onSettingsChanged = vi.fn();
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts") {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              { id: 1, account_name: "Alpha", provider_type: "codex", auth_mode: "oauth", base_url: "", status: "active", priority: 1, is_active: true, balance: 0, quota_remaining: 0, rpm_remaining: 0, tpm_remaining: 0, health_score: 100, recent_error_rate: 0, last_total_tokens: 0, last_input_tokens: 0, last_output_tokens: 0, model_context_window: 0, primary_used_percent: 0, secondary_used_percent: 0 },
              { id: 2, account_name: "Beta", provider_type: "codex", auth_mode: "oauth", base_url: "", status: "active", priority: 2, is_active: false, balance: 0, quota_remaining: 0, rpm_remaining: 0, tpm_remaining: 0, health_score: 100, recent_error_rate: 0, last_total_tokens: 0, last_input_tokens: 0, last_output_tokens: 0, model_context_window: 0, primary_used_percent: 0, secondary_used_percent: 0 },
            ]),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      if (url === "/ai-router/api/settings/failover-queue") {
        return Promise.resolve(
          new Response(JSON.stringify([2, 1]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (url === "/ai-router/api/settings/database/backups") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/app" && init?.method === "PUT") {
        return Promise.resolve(
          new Response(String(init.body), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(getAppMetadata).mockResolvedValue({
      name: "AI Gate",
      version: "0.1.0",
      description: "桌面代理与路由控制台",
      author: "GcsSloop",
    });
    vi.mocked(getRecentDesktopLogs).mockResolvedValue([]);
    vi.mocked(applyDesktopAppSettings).mockResolvedValue({
      backend_addr: "127.0.0.1:6789",
      backend_api_base: "http://127.0.0.1:6789/ai-router/api",
      launch_at_login: true,
      silent_start: false,
      close_to_tray: true,
    });

    render(<SettingsPage initialSettings={baseSettings} language="zh-CN" t={identity} proxyEnabled={true} onSettingsChanged={onSettingsChanged} />);

    expect(await screen.findByRole("tab", { name: "通用" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "代理" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "高级" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "关于" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("switch", { name: "开机自启" }));
    fireEvent.click(screen.getByRole("button", { name: "保存设置" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/settings/app",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ ...baseSettings, launch_at_login: true }),
        }),
      );
    });
    expect(applyDesktopAppSettings).toHaveBeenCalledWith({ ...baseSettings, launch_at_login: true });
    expect(onSettingsChanged).toHaveBeenCalledWith({ ...baseSettings, launch_at_login: true });
  });

  it("supports database backup actions and about metadata", async () => {
    const confirmSpy = vi.spyOn(Modal, "confirm").mockImplementation((config) => {
      void config.onOk?.();
      return {
        destroy: vi.fn(),
        update: vi.fn(),
      } as ReturnType<typeof Modal.confirm>;
    });
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/failover-queue") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/database/backups" && (!init?.method || init.method === "GET")) {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ backup_id: "20260309-101500.000", created_at: "2026-03-09T10:15:00Z", size_bytes: 4096 }]),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      if (url === "/ai-router/api/settings/database/backup" && init?.method === "POST") {
        return Promise.resolve(
          new Response(
            JSON.stringify({ backup_id: "20260309-111500.000", created_at: "2026-03-09T11:15:00Z", size_bytes: 4096 }),
            { status: 201, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      if (url === "/ai-router/api/settings/database/restore" && init?.method === "POST") {
        return Promise.resolve(
          new Response(JSON.stringify({ restored_from: "20260309-101500.000" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (url === "/ai-router/api/settings/database/backups/20260309-101500.000" && init?.method === "DELETE") {
        return Promise.resolve(
          new Response(JSON.stringify({ deleted_backup_id: "20260309-101500.000" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(getAppMetadata).mockResolvedValue({
      name: "AI Gate",
      version: "0.1.0",
      description: "桌面代理与路由控制台",
      author: "GcsSloop",
    });
    vi.mocked(getRecentDesktopLogs).mockResolvedValue([]);
    vi.mocked(applyDesktopAppSettings).mockResolvedValue({
      backend_addr: "127.0.0.1:6789",
      backend_api_base: "http://127.0.0.1:6789/ai-router/api",
      launch_at_login: false,
      silent_start: false,
      close_to_tray: true,
    });

    render(<SettingsPage initialSettings={baseSettings} language="zh-CN" t={identity} proxyEnabled={false} onSettingsChanged={vi.fn()} />);

    fireEvent.click(await screen.findByRole("tab", { name: "高级" }));
    const toolbar = screen.getByTestId("settings-tab-toolbar");
    expect(within(toolbar).getByRole("tab", { name: "高级" })).toBeInTheDocument();
    expect(within(toolbar).getByRole("button", { name: "保存设置" })).toBeInTheDocument();
    expect(screen.getByText("备份与恢复").closest(".settings-card")).toHaveClass("settings-card-overflow-visible");

    expect(await screen.findByRole("button", { name: "备份操作 20260309-101500.000" })).toBeInTheDocument();
    expect(screen.queryByText("审计存储")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "立即优化" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "立即备份" }));
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/ai-router/api/settings/database/backup", expect.objectContaining({ method: "POST" }));
    });
    await waitFor(() => {
      expect(
        fetchMock.mock.calls.filter(([input, init]) => String(input) === "/ai-router/api/settings/database/backups" && (!init?.method || init.method === "GET"))
          .length,
      ).toBeGreaterThanOrEqual(2);
    });

    await openBackupMenu();
    fireEvent.click(await screen.findByText("恢复此备份"));
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/settings/database/restore",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ backup_id: "20260309-101500.000" }),
        }),
      );
    });

    await act(async () => {
      await openBackupMenu();
    });
    await act(async () => {
      fireEvent.click(await screen.findByText("删除此备份"));
    });
    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalled();
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/settings/database/backups/20260309-101500.000",
        expect.objectContaining({ method: "DELETE" }),
      );
    });

    fireEvent.click(screen.getByRole("tab", { name: "关于" }));
    expect(await screen.findByText("GcsSloop")).toBeInTheDocument();
    expect(screen.getByText("桌面代理与路由控制台")).toBeInTheDocument();
    expect(screen.getByText("GitHub")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "GcsSloop/ai-gate" })).toHaveAttribute("href", "https://github.com/GcsSloop/ai-gate");
    expect(screen.getByText("应用更新")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "检查更新" })).toBeInTheDocument();

    confirmSpy.mockRestore();
  });

  it("auto-saves language changes immediately", async () => {
    const onSettingsChanged = vi.fn();
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/failover-queue") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/database/backups") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/app" && init?.method === "PUT") {
        return Promise.resolve(
          new Response(String(init.body), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(getAppMetadata).mockResolvedValue({
      name: "AI Gate",
      version: "0.1.0",
      description: "桌面代理与路由控制台",
      author: "GcsSloop",
    });
    vi.mocked(getRecentDesktopLogs).mockResolvedValue([]);
    vi.mocked(applyDesktopAppSettings).mockResolvedValue(null);

    render(<SettingsPage initialSettings={baseSettings} language="zh-CN" t={identity} proxyEnabled={false} onSettingsChanged={onSettingsChanged} />);

    fireEvent.click(await screen.findByRole("radio", { name: "English" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/settings/app",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ ...baseSettings, language: "en-US" }),
        }),
      );
    });

    expect(onSettingsChanged).toHaveBeenCalledWith({ ...baseSettings, language: "en-US" });
  });

  it("places interface preferences before window behavior in general settings", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/failover-queue") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/database/backups") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(getAppMetadata).mockResolvedValue({
      name: "AI Gate",
      version: "0.1.0",
      description: "桌面代理与路由控制台",
      author: "GcsSloop",
    });
    vi.mocked(getRecentDesktopLogs).mockResolvedValue([]);

    render(<SettingsPage initialSettings={baseSettings} language="zh-CN" t={identity} proxyEnabled={false} onSettingsChanged={vi.fn()} />);

    const preferencesHeading = await screen.findByText("界面偏好");
    const windowHeading = screen.getByText("窗口行为");
    expect(
      preferencesHeading.compareDocumentPosition(windowHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("saves the home update indicator preference", async () => {
    const onSettingsChanged = vi.fn();
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/failover-queue") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/database/backups") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/app" && init?.method === "PUT") {
        return Promise.resolve(
          new Response(String(init.body), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(getAppMetadata).mockResolvedValue({
      name: "AI Gate",
      version: "0.1.0",
      description: "桌面代理与路由控制台",
      author: "GcsSloop",
    });
    vi.mocked(getRecentDesktopLogs).mockResolvedValue([]);
    vi.mocked(applyDesktopAppSettings).mockResolvedValue(null);

    render(<SettingsPage initialSettings={baseSettings} language="zh-CN" t={identity} proxyEnabled={false} onSettingsChanged={onSettingsChanged} />);

    fireEvent.click(await screen.findByRole("switch", { name: "首页更新提示" }));
    fireEvent.click(screen.getByRole("button", { name: "保存设置" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/settings/app",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ ...baseSettings, show_home_update_indicator: false }),
        }),
      );
    });
    expect(onSettingsChanged).toHaveBeenCalledWith({ ...baseSettings, show_home_update_indicator: false });
  });

  it("auto-saves theme changes immediately", async () => {
    const onSettingsChanged = vi.fn();
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/failover-queue") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/database/backups") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/app" && init?.method === "PUT") {
        return Promise.resolve(
          new Response(String(init.body), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(getAppMetadata).mockResolvedValue({
      name: "AI Gate",
      version: "0.1.0",
      description: "桌面代理与路由控制台",
      author: "GcsSloop",
    });
    vi.mocked(getRecentDesktopLogs).mockResolvedValue([]);
    vi.mocked(applyDesktopAppSettings).mockResolvedValue(null);

    render(<SettingsPage initialSettings={baseSettings} language="zh-CN" t={identity} proxyEnabled={false} onSettingsChanged={onSettingsChanged} />);

    fireEvent.click(await screen.findByRole("radio", { name: "深色模式" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/settings/app",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ ...baseSettings, theme_mode: "dark" }),
        }),
      );
    });

    expect(onSettingsChanged).toHaveBeenCalledWith({ ...baseSettings, theme_mode: "dark" });
  });

  it("saves the configurable status refresh interval within the allowed range", async () => {
    const onSettingsChanged = vi.fn();
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/failover-queue") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/database/backups") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/app" && init?.method === "PUT") {
        return Promise.resolve(
          new Response(String(init.body), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(getAppMetadata).mockResolvedValue({
      name: "AI Gate",
      version: "0.1.0",
      description: "桌面代理与路由控制台",
      author: "GcsSloop",
    });
    vi.mocked(getRecentDesktopLogs).mockResolvedValue([]);
    vi.mocked(applyDesktopAppSettings).mockResolvedValue(null);

    render(<SettingsPage initialSettings={baseSettings} language="zh-CN" t={identity} proxyEnabled={false} onSettingsChanged={onSettingsChanged} />);

    fireEvent.change(await screen.findByLabelText("状态刷新间隔（秒）"), { target: { value: "5" } });
    fireEvent.click(screen.getByRole("button", { name: "保存设置" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/settings/app",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ ...baseSettings, status_refresh_interval_seconds: 5 }),
        }),
      );
    });

    expect(onSettingsChanged).toHaveBeenCalledWith({ ...baseSettings, status_refresh_interval_seconds: 5 });
  });

  it("renders recent desktop logs inside a bounded scroll panel", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/failover-queue") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/database/backups") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(getAppMetadata).mockResolvedValue({
      name: "AI Gate",
      version: "0.1.0",
      description: "桌面代理与路由控制台",
      author: "GcsSloop",
    });
    vi.mocked(getRecentDesktopLogs).mockResolvedValue([
      { timestamp_ms: 1, level: "info", category: "sidecar", message: "spawn success" },
      { timestamp_ms: 2, level: "warn", category: "recovery", message: "restart triggered" },
    ]);
    vi.mocked(applyDesktopAppSettings).mockResolvedValue(null);

    render(<SettingsPage initialSettings={baseSettings} language="zh-CN" t={identity} proxyEnabled={false} onSettingsChanged={vi.fn()} />);

    fireEvent.click(await screen.findByRole("tab", { name: "高级" }));
    expect(await screen.findByText("最近日志")).toBeInTheDocument();
    expect(screen.getByText("spawn success")).toBeInTheDocument();
    expect(screen.getByText("restart triggered")).toBeInTheDocument();
    expect(screen.getByTestId("settings-recent-logs")).toBeInTheDocument();
  });

  it("renders backup actions in a compact menu and keeps backup settings spaced from the list", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/failover-queue") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/database/backups") {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ backup_id: "20260309-101500.000", created_at: "2026-03-09T10:15:00Z", size_bytes: 4096 }]),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(getAppMetadata).mockResolvedValue({
      name: "AI Gate",
      version: "0.1.0",
      description: "桌面代理与路由控制台",
      author: "GcsSloop",
    });
    vi.mocked(getRecentDesktopLogs).mockResolvedValue([]);

    render(<SettingsPage initialSettings={baseSettings} language="zh-CN" t={identity} proxyEnabled={false} onSettingsChanged={vi.fn()} />);

    fireEvent.click(await screen.findByRole("tab", { name: "高级" }));

    const actionsButton = await screen.findByRole("button", { name: "备份操作 20260309-101500.000" });
    expect(actionsButton).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "恢复此备份" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "删除此备份" })).not.toBeInTheDocument();
    expect(screen.getByTestId("backup-settings-grid")).toBeInTheDocument();
    expect(screen.getByTestId("backup-list")).toBeInTheDocument();

    await openBackupMenu();
    expect(await screen.findByText("恢复此备份")).toBeInTheDocument();
    expect(screen.getByText("删除此备份")).toBeInTheDocument();
  });

  it("closes the backup action menu when clicking outside", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/accounts") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/failover-queue") {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      if (url === "/ai-router/api/settings/database/backups") {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ backup_id: "20260309-101500.000", created_at: "2026-03-09T10:15:00Z", size_bytes: 4096 }]),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(getAppMetadata).mockResolvedValue({
      name: "AI Gate",
      version: "0.1.0",
      description: "桌面代理与路由控制台",
      author: "GcsSloop",
    });
    vi.mocked(getRecentDesktopLogs).mockResolvedValue([]);

    render(<SettingsPage initialSettings={baseSettings} language="zh-CN" t={identity} proxyEnabled={false} onSettingsChanged={vi.fn()} />);

    fireEvent.click(await screen.findByRole("tab", { name: "高级" }));
    await openBackupMenu();
    expect(await screen.findByText("恢复此备份")).toBeInTheDocument();

    fireEvent.mouseDown(document.body);

    await waitFor(() => {
      expect(screen.queryByText("恢复此备份")).not.toBeInTheDocument();
      expect(screen.queryByText("删除此备份")).not.toBeInTheDocument();
    });
  });
});
