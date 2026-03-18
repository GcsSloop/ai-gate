import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { UpdateCard } from "./UpdateCard";
import type { DesktopUpdateInfo, DesktopUpdateService, DesktopUpdateState } from "./updateService";

function createService(state: DesktopUpdateState): DesktopUpdateService {
  const currentState = structuredClone(state) as DesktopUpdateState;
  const snapshot = () => structuredClone(currentState) as DesktopUpdateState;
  return {
    getState: vi.fn().mockImplementation(async () => snapshot()),
    check: vi.fn().mockImplementation(async () => ({ supported: currentState.status !== "unsupported", update: currentState.update ? structuredClone(currentState.update) : null })),
    downloadAndInstall: vi.fn().mockImplementation(async () => {
      currentState.status = "downloading";
      currentState.progress = { percent: 25, total: 100, transferred: 25 };
    }),
    cancelDownload: vi.fn().mockImplementation(async () => {
      currentState.status = "cancelled";
      currentState.progress = null;
    }),
    relaunch: vi.fn().mockResolvedValue(undefined),
  };
}

function availableUpdate(): DesktopUpdateInfo {
  return {
    body: "Important fixes",
    currentVersion: "2.3.4",
    date: "2026-03-11T12:00:00Z",
    version: "2.3.5",
  };
}

describe("UpdateCard", () => {
  const t = (value: string) => value;

  it("shows the latest status when no update is available", async () => {
    const service = createService({
      status: "up-to-date",
      update: null,
      progress: null,
      error: null,
    });

    render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={service} />);

    fireEvent.click(screen.getByRole("button", { name: "检查更新" }));

    await screen.findByText("已是最新版本");
    expect(service.check).toHaveBeenCalledOnce();
  });

  it("hydrates and renders a backend-owned download on first paint", async () => {
    const service = createService({
      status: "downloading",
      update: availableUpdate(),
      progress: { percent: 19, total: 100, transferred: 19 },
      error: null,
    });

    render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={service} />);

    await screen.findByText("下载进度 19%");
    expect(screen.getByRole("button", { name: "终止下载" })).toBeInTheDocument();
  });

  it("supports host-managed download, cancellation and relaunch", async () => {
    const state: DesktopUpdateState = {
      status: "available",
      update: availableUpdate(),
      progress: null,
      error: null,
    };
    const service = createService(state);

    render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={service} />);

    await screen.findByText("发现新版本 2.3.5");
    fireEvent.click(screen.getByRole("button", { name: "下载并安装" }));

    await screen.findByText("下载进度 25%");
    expect(service.downloadAndInstall).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByRole("button", { name: "终止下载" }));

    await screen.findByText("下载已取消");
    expect(service.cancelDownload).toHaveBeenCalledOnce();

    const readyService = createService({
      status: "ready",
      update: availableUpdate(),
      progress: { percent: 100, total: 100, transferred: 100 },
      error: null,
    });
    render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={readyService} />);

    await screen.findByRole("button", { name: "立即重启" });
    fireEvent.click(screen.getByRole("button", { name: "立即重启" }));

    await waitFor(() => {
      expect(readyService.relaunch).toHaveBeenCalledOnce();
    });
  });

  it("shows errors from failed checks", async () => {
    const service: DesktopUpdateService = {
      getState: vi.fn().mockResolvedValue({ status: "idle", update: null, progress: null, error: null }),
      check: vi.fn().mockRejectedValue(new Error("boom")),
      downloadAndInstall: vi.fn(),
      cancelDownload: vi.fn(),
      relaunch: vi.fn(),
    };

    render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={service} />);

    fireEvent.click(screen.getByRole("button", { name: "检查更新" }));

    await screen.findByText("boom");
  });

  it("shows latest version details even when automatic install is unsupported", async () => {
    const service: DesktopUpdateService = {
      getState: vi.fn().mockResolvedValue({ status: "unsupported", update: availableUpdate(), progress: null, error: null }),
      check: vi.fn().mockResolvedValue({
        supported: false,
        update: availableUpdate(),
      }),
      downloadAndInstall: vi.fn(),
      cancelDownload: vi.fn(),
      relaunch: vi.fn(),
    };

    render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={service} />);

    fireEvent.click(screen.getByRole("button", { name: "检查更新" }));

    await screen.findByText("2.3.5");
    expect(screen.getByText("Important fixes")).toBeInTheDocument();
    expect(screen.getByText("当前环境不支持自动安装，但已检查到最新版本。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "下载并安装" })).not.toBeInTheDocument();
  });

  it("adds a checking animation hook while the version lookup is in flight", async () => {
    let resolveCheck: ((value: { supported: boolean; update: DesktopUpdateInfo | null }) => void) | undefined;
    const service: DesktopUpdateService = {
      getState: vi.fn().mockResolvedValue({ status: "idle", update: null, progress: null, error: null }),
      check: vi.fn().mockImplementation(
        () =>
          new Promise((resolve) => {
            resolveCheck = resolve;
          }),
      ),
      downloadAndInstall: vi.fn(),
      cancelDownload: vi.fn(),
      relaunch: vi.fn(),
    };

    const { container } = render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={service} />);

    fireEvent.click(screen.getByRole("button", { name: "检查更新" }));

    expect(container.querySelector(".update-status-value.is-checking")).not.toBeNull();

    resolveCheck?.({ supported: true, update: null });

    await waitFor(() => {
      expect(service.getState).toHaveBeenCalled();
    });
  });
});
