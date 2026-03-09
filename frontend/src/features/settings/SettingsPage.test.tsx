import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { SettingsPage } from "./SettingsPage";

describe("SettingsPage", () => {
  it("supports creating backup and restoring backup", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/ai-router/api/settings/codex/backups" && (!init?.method || init.method === "GET")) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              { backup_id: "20260309-090000.000", created_at: "2026-03-09T01:00:00Z" },
              { backup_id: "20260308-220000.000", created_at: "2026-03-08T14:00:00Z" },
            ]),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      if (url === "/ai-router/api/settings/codex/backup" && init?.method === "POST") {
        return Promise.resolve(
          new Response(JSON.stringify({ backup_id: "20260309-100000.000" }), {
            status: 201,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (url === "/ai-router/api/settings/codex/restore" && init?.method === "POST") {
        return Promise.resolve(
          new Response(JSON.stringify({ ok: "true", pre_restore_id: "20260309-100001.000-pre-restore" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      if (url === "/ai-router/api/settings/codex/backups/20260309-090000.000/files" && init?.method === "GET") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              backup_id: "20260309-090000.000",
              files: {
                "config.toml": "model_provider = \"router\"",
                "auth.json": "{\"tokens\":{\"access_token\":\"token\"}}",
                "manifest.json": "{\"created_at\":\"2026-03-09T01:00:00Z\"}",
              },
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        );
      }
      return Promise.resolve(new Response(null, { status: 404 }));
    });
    vi.stubGlobal("fetch", fetchMock);

    render(<SettingsPage />);

    expect(await screen.findByText("Codex 备份与恢复")).toBeInTheDocument();
    expect(await screen.findByText("20260309-090000.000")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "一键备份" }));
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/ai-router/api/settings/codex/backup", expect.objectContaining({ method: "POST" }));
    });

    fireEvent.click(screen.getAllByRole("button", { name: /恢\s*复/ })[0]);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/ai-router/api/settings/codex/restore",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ backup_id: "20260309-090000.000" }),
        }),
      );
    });

    fireEvent.click(screen.getAllByRole("button", { name: /查\s*看/ })[0]);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith("/ai-router/api/settings/codex/backups/20260309-090000.000/files");
    });
  });
});
