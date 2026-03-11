import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { UpdateCard } from "./UpdateCard";
import type { DesktopUpdateInfo, DesktopUpdateService } from "./updateService";

function createService(update: DesktopUpdateInfo | null): DesktopUpdateService {
  return {
    check: vi.fn().mockResolvedValue({ supported: true, update }),
    downloadAndInstall: vi.fn().mockImplementation(async (_target, onProgress) => {
      onProgress?.({ percent: 25, total: 100, transferred: 25 });
      onProgress?.({ percent: 100, total: 100, transferred: 100 });
    }),
    relaunch: vi.fn().mockResolvedValue(undefined),
  };
}

describe("UpdateCard", () => {
  const t = (value: string) => value;

  it("shows the latest status when no update is available", async () => {
    const service = createService(null);

    render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={service} />);

    fireEvent.click(screen.getByRole("button", { name: "检查更新" }));

    await screen.findByText("已是最新版本");
    expect(service.check).toHaveBeenCalledOnce();
  });

  it("supports check, download, install and relaunch", async () => {
    const service = createService({
      body: "Important fixes",
      currentVersion: "2.3.4",
      date: "2026-03-11T12:00:00Z",
      version: "2.3.5",
    });

    render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={service} />);

    fireEvent.click(screen.getByRole("button", { name: "检查更新" }));

    await screen.findByText("发现新版本 2.3.5");
    expect(screen.getByText("Important fixes")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "下载并安装" }));

    await screen.findByText("下载进度 100%");
    await screen.findByRole("button", { name: "立即重启" });
    expect(service.downloadAndInstall).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByRole("button", { name: "立即重启" }));

    await waitFor(() => {
      expect(service.relaunch).toHaveBeenCalledOnce();
    });
  });

  it("shows errors from failed checks", async () => {
    const service: DesktopUpdateService = {
      check: vi.fn().mockRejectedValue(new Error("boom")),
      downloadAndInstall: vi.fn(),
      relaunch: vi.fn(),
    };

    render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={service} />);

    fireEvent.click(screen.getByRole("button", { name: "检查更新" }));

    await screen.findByText("boom");
  });

  it("shows latest version details even when automatic install is unsupported", async () => {
    const service: DesktopUpdateService = {
      check: vi.fn().mockResolvedValue({
        supported: false,
        update: {
          body: "Read-only fallback",
          currentVersion: "2.3.4",
          date: "2026-03-11T13:30:00Z",
          version: "2.3.6",
        },
      }),
      downloadAndInstall: vi.fn(),
      relaunch: vi.fn(),
    };

    render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={service} />);

    fireEvent.click(screen.getByRole("button", { name: "检查更新" }));

    await screen.findByText("2.3.6");
    expect(screen.getByText("Read-only fallback")).toBeInTheDocument();
    expect(screen.getByText("当前环境不支持自动安装，但已检查到最新版本。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "下载并安装" })).not.toBeInTheDocument();
  });

  it("adds a checking animation hook while the version lookup is in flight", async () => {
    let resolveCheck: ((value: { supported: boolean; update: DesktopUpdateInfo | null }) => void) | undefined;
    const service: DesktopUpdateService = {
      check: vi.fn().mockImplementation(
        () =>
          new Promise((resolve) => {
            resolveCheck = resolve;
          }),
      ),
      downloadAndInstall: vi.fn(),
      relaunch: vi.fn(),
    };

    const { container } = render(<UpdateCard autoCheckOnMount={false} currentVersion="2.3.4" language="zh-CN" t={t} service={service} />);

    fireEvent.click(screen.getByRole("button", { name: "检查更新" }));

    expect(container.querySelector(".update-status-value.is-checking")).not.toBeNull();

    resolveCheck?.({ supported: true, update: null });

    await screen.findByText("已是最新版本");
  });
});
