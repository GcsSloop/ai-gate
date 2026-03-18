import { invoke } from "@tauri-apps/api/core";
import { relaunch as tauriRelaunch } from "@tauri-apps/plugin-process";

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

export type DesktopUpdateStatus =
  | "idle"
  | "checking"
  | "up-to-date"
  | "available"
  | "downloading"
  | "ready"
  | "unsupported"
  | "cancelled"
  | "error";

export type DesktopUpdateState = {
  status: DesktopUpdateStatus;
  update: DesktopUpdateInfo | null;
  progress: DownloadProgress | null;
  error: string | null;
};

export type DesktopUpdateAdapter = {
  isSupported: () => boolean;
  getState: () => Promise<DesktopUpdateState>;
  check: () => Promise<DesktopUpdateState>;
  startDownload: (version: string) => Promise<DesktopUpdateState>;
  cancelDownload: () => Promise<DesktopUpdateState>;
  relaunch: () => Promise<void>;
};

export type DesktopUpdateService = {
  getState: () => Promise<DesktopUpdateState>;
  check: (currentVersion?: string) => Promise<{ supported: boolean; update: DesktopUpdateInfo | null }>;
  downloadAndInstall: (update: DesktopUpdateInfo) => Promise<void>;
  cancelDownload: () => Promise<void>;
  relaunch: () => Promise<void>;
};

export function emptyUpdateState(status: DesktopUpdateStatus = "idle"): DesktopUpdateState {
  return {
    status,
    update: null,
    progress: null,
    error: null,
  };
}

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

function createDefaultAdapter(): DesktopUpdateAdapter {
  return {
    isSupported: isDesktopShell,
    getState: () => invoke<DesktopUpdateState>("get_update_state"),
    check: () => invoke<DesktopUpdateState>("check_for_app_update"),
    startDownload: (version) => invoke<DesktopUpdateState>("start_update_download", { version }),
    cancelDownload: () => invoke<DesktopUpdateState>("cancel_update_download"),
    relaunch: () => tauriRelaunch(),
  };
}

export function createDesktopUpdateService(adapter: DesktopUpdateAdapter = createDefaultAdapter()): DesktopUpdateService {
  return {
    async getState() {
      if (!adapter.isSupported()) {
        return {
          ...emptyUpdateState("unsupported"),
          error: null,
        };
      }
      return adapter.getState();
    },
    async check(currentVersion) {
      if (!adapter.isSupported()) {
        void currentVersion;
        return { supported: false, update: null };
      }
      const state = await adapter.check();
      return {
        supported: state.status !== "unsupported",
        update: state.update,
      };
    },
    async downloadAndInstall(update) {
      if (!adapter.isSupported()) {
        throw new Error("Automatic updates are only available in the desktop app.");
      }
      await adapter.startDownload(update.version);
    },
    async cancelDownload() {
      if (!adapter.isSupported()) {
        return;
      }
      await adapter.cancelDownload();
    },
    relaunch: () => adapter.relaunch(),
  };
}
