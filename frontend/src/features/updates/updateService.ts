import { relaunch as tauriRelaunch } from "@tauri-apps/plugin-process";
import { check as tauriCheck } from "@tauri-apps/plugin-updater";

const GITHUB_LATEST_MANIFEST_URL = "https://github.com/GcsSloop/ai-gate/releases/latest/download/latest.json";

export type DesktopUpdateInfo = {
  body?: string | null;
  currentVersion: string;
  date?: string | null;
  version: string;
};

export type DownloadProgress = {
  percent: number;
  total: number;
  transferred: number;
};

type DownloadEvent =
  | { event: "Started"; data: { contentLength?: number } }
  | { event: "Progress"; data: { chunkLength?: number } }
  | { event: "Finished" };

type TauriUpdateResult = DesktopUpdateInfo & {
  downloadAndInstall: (handler?: (event: DownloadEvent) => void) => Promise<void>;
};

export type DesktopUpdateCheckResult = TauriUpdateResult | null;

export type DesktopUpdateAdapter = {
  isSupported: () => boolean;
  check: () => Promise<DesktopUpdateCheckResult>;
  relaunch: () => Promise<void>;
};

export type DesktopUpdateService = {
  check: (currentVersion?: string) => Promise<{ supported: boolean; update: DesktopUpdateInfo | null }>;
  downloadAndInstall: (update: DesktopUpdateInfo, onProgress?: (progress: DownloadProgress) => void) => Promise<void>;
  relaunch: () => Promise<void>;
};

type LatestManifest = {
  notes?: string;
  pub_date?: string;
  version?: string;
};

function isDesktopShell() {
  if (typeof window === "undefined") {
    return false;
  }
  const shellWindow = window as Window & {
    __TAURI__?: unknown;
    __TAURI_INTERNALS__?: unknown;
  };
  if (shellWindow.__TAURI__ || shellWindow.__TAURI_INTERNALS__) {
    return true;
  }
  const protocol = window.location.protocol;
  return protocol === "tauri:" || protocol === "file:";
}

export function mapDownloadProgress(
  event?: { contentLength?: number; chunkLength?: number },
  previous: DownloadProgress = { percent: 0, total: 0, transferred: 0 },
): DownloadProgress {
  if (!event) {
    return previous;
  }
  if (typeof event.contentLength === "number") {
    return {
      percent: 0,
      total: event.contentLength,
      transferred: 0,
    };
  }
  const transferred = previous.transferred + (event.chunkLength ?? 0);
  const total = previous.total;
  const percent = total > 0 ? Math.min(100, (transferred / total) * 100) : previous.percent;
  return {
    percent,
    total,
    transferred,
  };
}

function createDefaultAdapter(): DesktopUpdateAdapter {
  return {
    isSupported: isDesktopShell,
    check: () => tauriCheck() as Promise<DesktopUpdateCheckResult>,
    relaunch: () => tauriRelaunch(),
  };
}

async function fetchLatestManifest(fetchImpl: typeof fetch, currentVersion?: string): Promise<DesktopUpdateInfo | null> {
  const response = await fetchImpl(GITHUB_LATEST_MANIFEST_URL, {
    headers: {
      Accept: "application/json",
    },
  });
  if (!response.ok) {
    throw new Error(`Failed to fetch latest release metadata (${response.status})`);
  }
  const payload = (await response.json()) as LatestManifest;
  if (!payload.version) {
    return null;
  }
  return {
    body: payload.notes ?? null,
    currentVersion: currentVersion ?? "",
    date: payload.pub_date ?? null,
    version: payload.version,
  };
}

export function createDesktopUpdateService(
  adapter: DesktopUpdateAdapter = createDefaultAdapter(),
  fetchImpl: typeof fetch = ((...args) => {
    if (typeof globalThis.fetch !== "function") {
      throw new Error("Fetch API is unavailable");
    }
    return globalThis.fetch(...args);
  }) as typeof fetch,
): DesktopUpdateService {
  let currentUpdate: TauriUpdateResult | null = null;

  return {
    async check(currentVersion) {
      if (!adapter.isSupported()) {
        currentUpdate = null;
        try {
          return { supported: false, update: await fetchLatestManifest(fetchImpl, currentVersion) };
        } catch {
          return { supported: false, update: null };
        }
      }
      currentUpdate = await adapter.check();
      if (!currentUpdate) {
        return { supported: true, update: null };
      }
      const { body, currentVersion: latestCurrentVersion, date, version } = currentUpdate;
      return {
        supported: true,
        update: {
          body,
          currentVersion: latestCurrentVersion,
          date,
          version,
        },
      };
    },
    async downloadAndInstall(update, onProgress) {
      if (!currentUpdate || currentUpdate.version !== update.version) {
        throw new Error("Update is no longer available. Check again and retry.");
      }
      let progress: DownloadProgress = { percent: 0, total: 0, transferred: 0 };
      await currentUpdate.downloadAndInstall((event) => {
        if (event.event === "Started") {
          progress = mapDownloadProgress({ contentLength: event.data.contentLength }, progress);
        } else if (event.event === "Progress") {
          progress = mapDownloadProgress({ chunkLength: event.data.chunkLength }, progress);
        } else {
          progress = { ...progress, percent: 100 };
        }
        onProgress?.(progress);
      });
    },
    relaunch: () => adapter.relaunch(),
  };
}
