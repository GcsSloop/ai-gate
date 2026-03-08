export const WEBUI_BASE = "/ai-router/webui";

function resolveAPIBase(): string {
  if (typeof window !== "undefined") {
    const protocol = window.location.protocol;
    if (protocol === "tauri:" || protocol === "file:") {
      return "http://127.0.0.1:6789/ai-router/api";
    }
  }
  return "/ai-router/api";
}

export const API_BASE = resolveAPIBase();

export function apiPath(path: string): string {
  if (path.startsWith("/")) {
    return `${API_BASE}${path}`;
  }
  return `${API_BASE}/${path}`;
}
