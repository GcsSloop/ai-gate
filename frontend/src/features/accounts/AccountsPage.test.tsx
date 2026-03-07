import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { AccountsPage } from "./AccountsPage";

describe("AccountsPage", () => {
  it("lists accounts and submits a third-party account", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify([
            {
              id: 1,
              provider_type: "openai-compatible",
              account_name: "mirror-east",
              auth_mode: "api_key",
              base_url: "https://code.ppchat.vip/v1",
              status: "active",
            },
          ]),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      )
      .mockResolvedValueOnce(new Response(null, { status: 201 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ authorization_url: "https://auth.example.test", state: "abc" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(null, {
          status: 201,
        }),
      );

    vi.stubGlobal("fetch", fetchMock);

    render(<AccountsPage />);

    expect(await screen.findByText("mirror-east")).toBeInTheDocument();
    expect(screen.getByLabelText("Base URL for mirror-east")).toHaveValue("https://code.ppchat.vip/v1");

    fireEvent.change(screen.getByLabelText("Account name"), {
      target: { value: "ppchat-main" },
    });
    fireEvent.change(screen.getByLabelText("Base URL"), {
      target: { value: "https://code.ppchat.vip/v1" },
    });
    fireEvent.change(screen.getByLabelText("API key"), {
      target: { value: "sk-test" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save third-party account" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenNthCalledWith(
        2,
        "/accounts",
        expect.objectContaining({
          method: "POST",
        }),
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "Connect official account" }));
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/accounts/auth/authorize",
      expect.objectContaining({ method: "POST" }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Import local Codex credentials" }));
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      "/accounts/import-local",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
