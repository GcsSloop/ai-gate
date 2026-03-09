import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

import { App } from "./App";
import { refreshDesktopTrayState, subscribeDesktopBackendStateChanged } from "./lib/desktop-shell";

vi.mock("./features/accounts/AccountsPage", () => ({
  AccountsPage: ({ syncToken }: { syncToken?: number }) => <div>accounts-sync:{syncToken ?? 0}</div>,
}));

vi.mock("./lib/desktop-shell", () => ({
  refreshDesktopTrayState: vi.fn(),
  subscribeDesktopBackendStateChanged: vi.fn(),
}));

describe("App", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders single-page dashboard shell", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url === "/ai-router/api/settings/proxy/status") {
          return Promise.resolve(
            new Response(JSON.stringify({ enabled: false }), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
          );
        }
        if (url === "/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    expect(await screen.findByText("accounts-sync:0")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "设置" })).toBeInTheDocument();
    expect(screen.getByText("开启代理")).toBeInTheDocument();
    expect(screen.getByText("AI Gate")).toBeInTheDocument();
  });

  it("refreshes tray state after toggling proxy from the page", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url === "/ai-router/api/settings/proxy/status" && (!init?.method || init.method === "GET")) {
          return Promise.resolve(
            new Response(JSON.stringify({ enabled: false }), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
          );
        }
        if (url === "/ai-router/api/settings/proxy/enable" && init?.method === "POST") {
          return Promise.resolve(
            new Response(JSON.stringify({ enabled: true }), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
          );
        }
        if (url === "/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockResolvedValue(() => {});

    render(<App />);

    const toggle = await screen.findByRole("switch");
    fireEvent.click(toggle);

    await waitFor(() => {
      expect(refreshDesktopTrayState).toHaveBeenCalledTimes(1);
    });
  });

  it("refreshes page state when the desktop shell reports backend changes", async () => {
    let proxyEnabled = false;
    let backendStateChanged: (() => void) | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url === "/ai-router/api/settings/proxy/status" && (!init?.method || init.method === "GET")) {
          return Promise.resolve(
            new Response(JSON.stringify({ enabled: proxyEnabled }), {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
          );
        }
        if (url === "/ai-router/api/accounts") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        if (url === "/ai-router/api/accounts/usage") {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { "Content-Type": "application/json" } }));
        }
        return Promise.resolve(new Response(null, { status: 404 }));
      }),
    );
    vi.mocked(subscribeDesktopBackendStateChanged).mockImplementation(async (handler: () => void) => {
      backendStateChanged = handler;
      return () => {};
    });

    render(<App />);

    expect(await screen.findByText("accounts-sync:0")).toBeInTheDocument();
    expect(screen.getByRole("switch")).toHaveAttribute("aria-checked", "false");

    proxyEnabled = true;
    await act(async () => {
      backendStateChanged?.();
    });

    await waitFor(() => {
      expect(screen.getByText("accounts-sync:1")).toBeInTheDocument();
      expect(screen.getByRole("switch")).toHaveAttribute("aria-checked", "true");
    });
  });
});
