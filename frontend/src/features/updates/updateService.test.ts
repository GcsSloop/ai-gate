import { describe, expect, it, vi } from "vitest";

import {
  createDesktopUpdateService,
  emptyUpdateState,
  type DesktopUpdateAdapter,
  type DesktopUpdateState,
} from "./updateService";

function buildAdapter(state: DesktopUpdateState): DesktopUpdateAdapter {
  return {
    isSupported: () => true,
    getState: vi.fn().mockResolvedValue(state),
    check: vi.fn().mockResolvedValue(state),
    startDownload: vi.fn().mockResolvedValue(state),
    cancelDownload: vi.fn().mockResolvedValue({ ...state, status: "cancelled" }),
    relaunch: vi.fn().mockResolvedValue(undefined),
  };
}

describe("updateService", () => {
  it("returns unsupported result without requesting desktop state in web mode", async () => {
    const adapter: DesktopUpdateAdapter = {
      isSupported: () => false,
      getState: vi.fn(),
      check: vi.fn(),
      startDownload: vi.fn(),
      cancelDownload: vi.fn(),
      relaunch: vi.fn(),
    };
    const service = createDesktopUpdateService(adapter);

    await expect(service.check()).resolves.toEqual({ supported: false, update: null });
    await expect(service.getState()).resolves.toEqual({ status: "unsupported", update: null, progress: null, error: null });
    expect(adapter.check).not.toHaveBeenCalled();
    expect(adapter.getState).not.toHaveBeenCalled();
  });

  it("hydrates an existing backend-owned downloading state", async () => {
    const adapter = buildAdapter({
      status: "downloading",
      update: {
        body: "Bug fixes",
        currentVersion: "2.3.4",
        date: "2026-03-11T12:00:00Z",
        version: "2.3.5",
      },
      progress: { percent: 19, total: 100, transferred: 19 },
      error: null,
    });
    const service = createDesktopUpdateService(adapter);

    await expect(service.getState()).resolves.toEqual({
      status: "downloading",
      update: {
        body: "Bug fixes",
        currentVersion: "2.3.4",
        date: "2026-03-11T12:00:00Z",
        version: "2.3.5",
      },
      progress: { percent: 19, total: 100, transferred: 19 },
      error: null,
    });
  });

  it("returns update details when a newer release is available", async () => {
    const adapter = buildAdapter({
      status: "available",
      update: {
        body: "Bug fixes",
        currentVersion: "2.3.4",
        date: "2026-03-11T12:00:00Z",
        version: "2.3.5",
      },
      progress: null,
      error: null,
    });
    const service = createDesktopUpdateService(adapter);

    await expect(service.check()).resolves.toEqual({
      supported: true,
      update: {
        body: "Bug fixes",
        currentVersion: "2.3.4",
        date: "2026-03-11T12:00:00Z",
        version: "2.3.5",
      },
    });
  });

  it("starts a host-managed download by version and supports cancellation", async () => {
    const adapter = buildAdapter({
      status: "available",
      update: {
        currentVersion: "2.3.4",
        version: "2.3.5",
      },
      progress: null,
      error: null,
    });
    const service = createDesktopUpdateService(adapter);

    await service.downloadAndInstall({ currentVersion: "2.3.4", version: "2.3.5" });
    await service.cancelDownload();

    expect(adapter.startDownload).toHaveBeenCalledWith("2.3.5");
    expect(adapter.cancelDownload).toHaveBeenCalledOnce();
  });

  it("propagates relaunch through the adapter", async () => {
    const adapter = buildAdapter(emptyUpdateState());
    const service = createDesktopUpdateService(adapter);

    await service.relaunch();

    expect(adapter.relaunch).toHaveBeenCalledOnce();
  });
});
