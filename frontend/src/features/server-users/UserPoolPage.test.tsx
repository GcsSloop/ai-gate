import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { UserPoolPage } from "./UserPoolPage";
import { getServerMe, listMyServerAccounts, updateMyServerAccountState } from "../../lib/api";

vi.mock("../../lib/api", async () => {
  const actual = await vi.importActual<typeof import("../../lib/api")>("../../lib/api");
  return {
    ...actual,
    getServerMe: vi.fn(),
    listMyServerAccounts: vi.fn(),
    updateMyServerAccountState: vi.fn(),
  };
});

describe("UserPoolPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(getServerMe).mockResolvedValue({
      user: { id: 1, name: "alice", username: "alice", role: "user", status: "active", created_at: "2026-06-15T00:00:00Z" },
      request_count: 3,
      total_tokens: 120,
    });
    vi.mocked(listMyServerAccounts).mockResolvedValue([
      {
        user_id: 1,
        account_id: 10,
        account_name: "pool-a",
        provider_type: "openai-compatible",
        status: "active",
        position: 0,
        is_active: false,
        is_locked: false,
        supports_responses: true,
      },
    ]);
    vi.mocked(updateMyServerAccountState).mockResolvedValue(undefined);
  });

  it("shows own usage and updates assigned account state", async () => {
    render(<UserPoolPage t={(value) => value} />);

    expect(await screen.findByText("pool-a")).toBeInTheDocument();
    expect(screen.getByText(/请求数 3/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("switch", { name: "激活 pool-a" }));

    await waitFor(() => expect(updateMyServerAccountState).toHaveBeenCalledWith(10, { position: 0, is_active: true, is_locked: false }));
  });
});
