import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { SettingsPage } from "./SettingsPage";
import { applyDesktopAppSettings, getAppMetadata } from "../../lib/desktop-shell";
import { setAPIBase } from "../../lib/paths";

vi.mock("../../lib/desktop-shell", () => ({
  applyDesktopAppSettings: vi.fn(),
  getAppMetadata: vi.fn(),
}));

const baseSettings = {
  launch_at_login: false,
  silent_start: false,
  close_to_tray: true,
  show_proxy_switch_on_home: true,
  proxy_host: "127.0.0.1",
  proxy_port: 6789,
  auto_failover_enabled: false,
  auto_backup_interval_hours: 24,
  backup_retention_count: 10,
};

describe("SettingsPage", () => {
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
    vi.mocked(applyDesktopAppSettings).mockResolvedValue({
      backend_addr: "127.0.0.1:6789",
      backend_api_base: "http://127.0.0.1:6789/ai-router/api",
      launch_at_login: true,
      silent_start: false,
      close_to_tray: true,
    });

    render(<SettingsPage initialSettings={baseSettings} proxyEnabled={true} onSettingsChanged={onSettingsChanged} />);

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
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(getAppMetadata).mockResolvedValue({
      name: "AI Gate",
      version: "0.1.0",
      description: "桌面代理与路由控制台",
      author: "GcsSloop",
    });
    vi.mocked(applyDesktopAppSettings).mockResolvedValue({
      backend_addr: "127.0.0.1:6789",
      backend_api_base: "http://127.0.0.1:6789/ai-router/api",
      launch_at_login: false,
      silent_start: false,
      close_to_tray: true,
    });

    render(<SettingsPage initialSettings={baseSettings} proxyEnabled={false} onSettingsChanged={vi.fn()} />);

    fireEvent.click(await screen.findByRole("tab", { name: "高级" }));
    expect(await screen.findByRole("button", { name: "恢复此备份" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "立即备份" }));
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/ai-router/api/settings/database/backup", expect.objectContaining({ method: "POST" }));
    });

    fireEvent.click(screen.getByRole("button", { name: "恢复此备份" }));
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/settings/database/restore",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ backup_id: "20260309-101500.000" }),
        }),
      );
    });

    fireEvent.click(screen.getByRole("tab", { name: "关于" }));
    expect(await screen.findByText("GcsSloop")).toBeInTheDocument();
    expect(screen.getByText("桌面代理与路由控制台")).toBeInTheDocument();
  });
});
