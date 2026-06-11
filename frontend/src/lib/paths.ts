export const WEBUI_BASE = "/ai-router/webui";

function resolveInitialAPIBase(): string {
  if (typeof window !== "undefined") {
    if (window.location.pathname.startsWith("/ai-gate/")) {
      return "/ai-gate/api";
    }
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

export function authPath(path: string): string {
  const normalized = path.startsWith("/") ? path : `/${path}`;
  if (typeof window !== "undefined" && window.location.pathname.startsWith("/ai-gate/")) {
    return `/ai-gate/auth${normalized}`;
  }
  return `/ai-router/auth${normalized}`;
}

export function isServerWebUI(): boolean {
  return typeof window !== "undefined" && window.location.pathname.startsWith("/ai-gate/");
}
