export const WEBUI_BASE = "/ai-router/webui";
export const API_BASE = "/ai-router/api";

export function apiPath(path: string): string {
  if (path.startsWith("/")) {
    return `${API_BASE}${path}`;
  }
  return `${API_BASE}/${path}`;
}
