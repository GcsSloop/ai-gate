import { listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";

export const BACKEND_STATE_CHANGED_EVENT = "aigate-backend-state-changed";

function isDesktopShell() {
  if (typeof window === "undefined") {
    return false;
  }
  const protocol = window.location.protocol;
  return protocol === "tauri:" || protocol === "file:";
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
