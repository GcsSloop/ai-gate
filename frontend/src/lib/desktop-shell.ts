import { listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";

import type { AppSettings } from "./api";

export const BACKEND_STATE_CHANGED_EVENT = "aigate-backend-state-changed";

export type DesktopShellContext = {
  backend_addr: string;
  backend_api_base: string;
  launch_at_login: boolean;
  silent_start: boolean;
  close_to_tray: boolean;
};

export type AppMetadata = {
  name: string;
  version: string;
  description: string;
  author: string;
};

export type DesktopRecentLog = {
  timestamp_ms: number;
  level: string;
  category: string;
  message: string;
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

export async function loadDesktopShellContext(): Promise<DesktopShellContext | null> {
  if (!isDesktopShell()) {
    return null;
  }
  return invoke<DesktopShellContext>("get_desktop_shell_context");
}

export async function applyDesktopAppSettings(settings: AppSettings): Promise<DesktopShellContext | null> {
  if (!isDesktopShell()) {
    return null;
  }
  return invoke<DesktopShellContext>("apply_app_settings", { payload: settings });
}

export async function getAppMetadata(): Promise<AppMetadata> {
  if (!isDesktopShell()) {
    return {
      name: "AI Gate",
      version: "0.1.0",
      description: "AI Gate 是一个本地桌面代理与账号编排工具，用于统一管理路由、故障转移与数据备份。",
      author: "GcsSloop",
    };
  }
  return invoke<AppMetadata>("get_app_metadata");
}

export async function getRecentDesktopLogs(limit = 50): Promise<DesktopRecentLog[]> {
  if (!isDesktopShell()) {
    return [];
  }
  return invoke<DesktopRecentLog[]>("get_recent_desktop_logs", { limit });
}

export async function refreshDesktopTrayState(): Promise<void> {
  if (!isDesktopShell()) {
    return;
  }
  await invoke("refresh_tray_state");
}

export async function subscribeDesktopBackendStateChanged(handler: () => void): Promise<() => void> {
  if (!isDesktopShell()) {
    return () => {};
  }
  return listen(BACKEND_STATE_CHANGED_EVENT, () => {
    handler();
  });
}
