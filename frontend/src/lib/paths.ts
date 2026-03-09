export const WEBUI_BASE = "/ai-router/webui";

function resolveInitialAPIBase(): string {
  if (typeof window !== "undefined") {
    const protocol = window.location.protocol;
    if (protocol === "tauri:" || protocol === "file:") {
      return "http://127.0.0.1:6789/ai-router/api";
    }
  }
  return "/ai-router/api";
}

let apiBase = resolveInitialAPIBase();

export function setAPIBase(value: string): void {
  apiBase = value.replace(/\/$/, "");
}

export function apiPath(path: string): string {
  if (path.startsWith("/")) {
    return `${apiBase}${path}`;
  }
  return `${apiBase}/${path}`;
}
