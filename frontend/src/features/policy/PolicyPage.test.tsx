import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { PolicyPage } from "./PolicyPage";

describe("PolicyPage", () => {
  it("loads and saves routing policy", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            name: "default",
            candidate_order: ["account-a", "account-b"],
            minimum_balance_threshold: 5,
            minimum_quota_threshold: 1000,
            token_budget_factor: 1.3,
            model_pool_rules: { "gpt-5.2-codex": ["primary"] },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      )
      .mockResolvedValueOnce(new Response(null, { status: 200 }));

    vi.stubGlobal("fetch", fetchMock);

    render(<PolicyPage />);

    expect(await screen.findByDisplayValue("account-a,account-b")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Token budget factor"), { target: { value: "1.5" } });
    fireEvent.click(screen.getByRole("button", { name: "Save policy" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenNthCalledWith(
        2,
        "/policy/default",
        expect.objectContaining({ method: "PUT" }),
      );
    });
  });
});
