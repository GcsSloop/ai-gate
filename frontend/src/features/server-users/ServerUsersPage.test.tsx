import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { ServerUsersPage } from "./ServerUsersPage";
import { createServerUser, deleteServerUser, listServerUsers } from "../../lib/api";

vi.mock("../../lib/api", async () => {
  const actual = await vi.importActual<typeof import("../../lib/api")>("../../lib/api");
  return {
    ...actual,
    listServerUsers: vi.fn(),
    createServerUser: vi.fn(),
    rotateServerUserToken: vi.fn(),
    disableServerUser: vi.fn(),
    deleteServerUser: vi.fn(),
  };
});

describe("ServerUsersPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.defineProperty(window, "location", {
      value: new URL("http://127.0.0.1:6791/ai-gate/webui/"),
      writable: true,
    });
    Object.assign(navigator, {
      clipboard: {
        writeText: vi.fn().mockResolvedValue(undefined),
      },
    });
    vi.mocked(listServerUsers).mockResolvedValue([]);
    vi.mocked(createServerUser).mockResolvedValue({
      user: { id: 1, name: "alice", status: "active", created_at: "2026-06-12T00:00:00Z" },
      token: "agt_once",
    });
    vi.mocked(deleteServerUser).mockResolvedValue(undefined);
  });

  it("creates a server user and shows the issued token once", async () => {
    render(<ServerUsersPage t={(value) => value} />);
    await waitFor(() => expect(listServerUsers).toHaveBeenCalled());

    fireEvent.change(screen.getByPlaceholderText("用户名称"), { target: { value: "alice" } });
    fireEvent.click(screen.getByRole("button", { name: /新建用户/ }));

    await waitFor(() => expect(createServerUser).toHaveBeenCalledWith("alice"));
    expect(await screen.findByDisplayValue("agt_once")).toBeInTheDocument();
  });

  it("copies an AI Gate shared account payload for the issued token", async () => {
    render(<ServerUsersPage t={(value) => value} />);
    await waitFor(() => expect(listServerUsers).toHaveBeenCalled());

    fireEvent.change(screen.getByPlaceholderText("用户名称"), { target: { value: "alice" } });
    fireEvent.click(screen.getByRole("button", { name: /新建用户/ }));
    expect(await screen.findByDisplayValue("agt_once")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "复制 AI Gate 导入配置" }));

    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledTimes(1));
    const copied = vi.mocked(navigator.clipboard.writeText).mock.calls[0][0];
    const payload = JSON.parse(copied);
    expect(payload.kind).toBe("aigate-account-share");
    expect(payload.schema_version).toBe(1);
    expect(payload.account).toMatchObject({
      provider_type: "openai-compatible",
      account_name: "alice",
      auth_mode: "api_key",
      base_url: "http://127.0.0.1:6791/ai-gate/v1",
      credential_ref: "agt_once",
      account_driver: "builtin_api_key",
      usage_config_json: "",
      supports_responses: true,
    });
  });

  it("does not render account pool assignment controls", async () => {
    vi.mocked(listServerUsers).mockResolvedValue([
      { id: 1, name: "alice", username: "alice", role: "user", status: "active", created_at: "2026-06-12T00:00:00Z" },
    ]);

    render(<ServerUsersPage t={(value) => value} />);
    expect(await screen.findByText("alice")).toBeInTheDocument();
    expect(screen.queryByText("账户池")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "分配账户" })).not.toBeInTheDocument();
  });

  it("deletes a server user after confirmation", async () => {
    vi.mocked(listServerUsers)
      .mockResolvedValueOnce([
        { id: 9, name: "delete-me", username: "delete-me", role: "user", status: "disabled", created_at: "2026-06-12T00:00:00Z" },
      ])
      .mockResolvedValueOnce([]);

    render(<ServerUsersPage t={(value) => value} />);
    expect(await screen.findByText("delete-me")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "删除用户-delete-me" }));
    fireEvent.click(await screen.findByRole("button", { name: "确认删除" }));

    await waitFor(() => expect(deleteServerUser).toHaveBeenCalledWith(9));
    await waitFor(() => expect(listServerUsers).toHaveBeenCalledTimes(2));
  });

  it("clears the one-time token panel when deleting the issued user", async () => {
    vi.mocked(listServerUsers)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([
        { id: 1, name: "alice", username: "alice", role: "user", status: "active", created_at: "2026-06-12T00:00:00Z" },
      ])
      .mockResolvedValueOnce([]);

    render(<ServerUsersPage t={(value) => value} />);
    await waitFor(() => expect(listServerUsers).toHaveBeenCalled());

    fireEvent.change(screen.getByPlaceholderText("用户名称"), { target: { value: "alice" } });
    fireEvent.click(screen.getByRole("button", { name: /新建用户/ }));
    expect(await screen.findByDisplayValue("agt_once")).toBeInTheDocument();
    expect(await screen.findByText("alice")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "删除用户-alice" }));
    fireEvent.click(await screen.findByRole("button", { name: "确认删除" }));

    await waitFor(() => expect(deleteServerUser).toHaveBeenCalledWith(1));
    await waitFor(() => expect(screen.queryByDisplayValue("agt_once")).not.toBeInTheDocument());
  });
});
