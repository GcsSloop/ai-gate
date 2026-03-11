import { describe, expect, it, vi } from "vitest";

import {
  createDesktopUpdateService,
  mapDownloadProgress,
  type DesktopUpdateAdapter,
  type DesktopUpdateCheckResult,
} from "./updateService";

function buildAdapter(result: DesktopUpdateCheckResult | null): DesktopUpdateAdapter {
  return {
    isSupported: () => true,
    check: vi.fn().mockResolvedValue(result),
    relaunch: vi.fn().mockResolvedValue(undefined),
  };
}

describe("updateService", () => {
  it("returns unsupported result with null update when manifest lookup fails", async () => {
    const fetchMock = vi.fn().mockRejectedValue(new Error("network unavailable"));
    const service = createDesktopUpdateService({
        isSupported: () => false,
        check: vi.fn(),
        relaunch: vi.fn(),
      },
      fetchMock as typeof fetch,
    );

    await expect(service.check()).resolves.toEqual({ supported: false, update: null });
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it("falls back to latest manifest lookup when desktop updater is unsupported", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        version: "2.3.6",
        notes: "Read-only fallback",
        pub_date: "2026-03-11T13:30:00Z",
      }),
    });
    const service = createDesktopUpdateService(
      {
        isSupported: () => false,
        check: vi.fn(),
        relaunch: vi.fn(),
      },
      fetchMock as typeof fetch,
    );

    await expect(service.check("2.3.4")).resolves.toEqual({
      supported: false,
      update: {
        body: "Read-only fallback",
        currentVersion: "2.3.4",
        date: "2026-03-11T13:30:00Z",
        version: "2.3.6",
      },
    });
  });

  it("returns update details when a newer release is available", async () => {
    const downloadAndInstall = vi.fn().mockImplementation(async (handler?: (event: unknown) => void) => {
      handler?.({ event: "Started", data: { contentLength: 100 } });
      handler?.({ event: "Progress", data: { chunkLength: 40 } });
      handler?.({ event: "Progress", data: { chunkLength: 60 } });
      handler?.({ event: "Finished" });
    });
    const adapter = buildAdapter({
      body: "Bug fixes",
      currentVersion: "2.3.4",
      date: "2026-03-11T12:00:00Z",
      downloadAndInstall,
      version: "2.3.5",
    });
    const service = createDesktopUpdateService(adapter);
    const checkResult = await service.check();

    expect(checkResult.supported).toBe(true);
    expect(checkResult.update).toMatchObject({
      body: "Bug fixes",
      currentVersion: "2.3.4",
      version: "2.3.5",
    });

    const progress = vi.fn();
    await service.downloadAndInstall(checkResult.update!, progress);

    expect(downloadAndInstall).toHaveBeenCalledOnce();
    expect(progress.mock.calls).toEqual([[{ percent: 0, transferred: 0, total: 100 }], [{ percent: 40, transferred: 40, total: 100 }], [{ percent: 100, transferred: 100, total: 100 }], [{ percent: 100, transferred: 100, total: 100 }]]);
  });

  it("propagates relaunch through the adapter", async () => {
    const adapter = buildAdapter(null);
    const service = createDesktopUpdateService(adapter);

    await service.relaunch();

    expect(adapter.relaunch).toHaveBeenCalledOnce();
  });

  it("maps download progress events into deterministic UI state", () => {
    expect(mapDownloadProgress(undefined)).toEqual({ percent: 0, total: 0, transferred: 0 });
    expect(mapDownloadProgress({ contentLength: 200 })).toEqual({ percent: 0, total: 200, transferred: 0 });
    expect(mapDownloadProgress({ chunkLength: 50 }, { total: 200, transferred: 25, percent: 12.5 })).toEqual({
      percent: 37.5,
      total: 200,
      transferred: 75,
    });
  });
});
