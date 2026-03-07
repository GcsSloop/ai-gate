import { render, screen } from "@testing-library/react";

import { ConversationsPage } from "./ConversationsPage";

describe("ConversationsPage", () => {
  it("renders conversation and run-chain details", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify([{ id: 2, client_id: "client-2", state: "active" }]),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify([
            { id: 10, account_id: 1, status: "capacity_failed", stream_offset: 100 },
            { id: 11, account_id: 2, status: "completed", stream_offset: 100 },
          ]),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );

    vi.stubGlobal("fetch", fetchMock);

    render(<ConversationsPage />);

    expect(await screen.findByText("client-2")).toBeInTheDocument();
    expect(await screen.findByText("capacity_failed")).toBeInTheDocument();
    expect(screen.getByText("completed")).toBeInTheDocument();
  });
});
