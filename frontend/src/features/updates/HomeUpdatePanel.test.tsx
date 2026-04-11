import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { HomeUpdatePanel } from "./HomeUpdatePanel";
import type { DesktopUpdateInfo, DesktopUpdateService } from "./updateService";

function buildUpdate(): DesktopUpdateInfo {
  return {
    body: "notes",
    currentVersion: "1.3.3",
    date: "2026-04-11T11:51:06Z",
    version: "1.3.4",
  };
}

describe("HomeUpdatePanel", () => {
  const t = (value: string) => value;

  it("allows retry install and check update in error state", async () => {
    const update = buildUpdate();
    const service: DesktopUpdateService = {
      getState: vi.fn().mockResolvedValue({
        status: "error",
        update,
        progress: null,
        error: "install update failed: transient lock",
      }),
      check: vi.fn().mockResolvedValue({ supported: true, update }),
      downloadAndInstall: vi.fn().mockResolvedValue(undefined),
      cancelDownload: vi.fn().mockResolvedValue(undefined),
      relaunch: vi.fn().mockResolvedValue(undefined),
    };

    render(<HomeUpdatePanel currentVersion="1.3.3" initialUpdate={update} language="zh-CN" t={t} service={service} />);

    await screen.findByText("install update failed: transient lock");
    fireEvent.click(screen.getByRole("button", { name: "重试安装" }));
    await waitFor(() => expect(service.downloadAndInstall).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByRole("button", { name: "检查更新" }));
    await waitFor(() => expect(service.check).toHaveBeenCalledTimes(1));
  });

  it("extracts non-Error invoke failures and shows detail", async () => {
    const update = buildUpdate();
    const service: DesktopUpdateService = {
      getState: vi.fn().mockResolvedValue({
        status: "available",
        update,
        progress: null,
        error: null,
      }),
      check: vi.fn().mockResolvedValue({ supported: true, update }),
      downloadAndInstall: vi.fn().mockRejectedValue({ message: "install update failed: dmg busy" }),
      cancelDownload: vi.fn().mockResolvedValue(undefined),
      relaunch: vi.fn().mockResolvedValue(undefined),
    };

    render(<HomeUpdatePanel currentVersion="1.3.3" initialUpdate={update} language="zh-CN" t={t} service={service} />);
    await screen.findByText("发现新版本 1.3.4");

    fireEvent.click(screen.getByRole("button", { name: "下载并安装" }));
    await screen.findByText("install update failed: dmg busy");
  });
});
