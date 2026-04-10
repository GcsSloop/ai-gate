import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { Modal } from "antd";
import type { ReactNode } from "react";

import { ToolingPage } from "./ToolingPage";
import * as api from "../../lib/api";

vi.mock("../../lib/api", async () => {
  const actual = await vi.importActual<typeof import("../../lib/api")>("../../lib/api");
  return {
    ...actual,
    getToolingState: vi.fn(),
    importToolingSkills: vi.fn(),
    updateToolingSkill: vi.fn(),
    deleteToolingSkill: vi.fn(),
    getToolingDiscoveredSkills: vi.fn(),
    refreshToolingDiscoveredSkills: vi.fn(),
    installToolingDiscoveredSkill: vi.fn(),
    searchToolingRepos: vi.fn(),
    resolveToolingRepo: vi.fn(),
    addToolingRepo: vi.fn(),
    updateToolingRepo: vi.fn(),
    removeToolingRepo: vi.fn(),
    reorderToolingRepos: vi.fn(),
    importToolingMcpServers: vi.fn(),
    applyToolingMcpServer: vi.fn(),
    installToolingMcpServer: vi.fn(),
    deleteToolingMcpServer: vi.fn(),
  };
});

function buildToolingState(): api.ToolingState {
  return {
    skill_sync_method: "symlink",
    clients: [
      {
        app: "codex",
        label: "Codex",
        skills_dir: "/tmp/home/.codex/skills",
        mcp_path: "/tmp/home/.codex/config.toml",
        skills_count: 2,
        mcp_status: "ready",
      },
    ],
    skill_stats: {
      total: 2,
      by_source: {
        codex: 2,
      },
    },
    skill_repos: [
      {
        platform: "github",
        owner: "openai",
        name: "codex-superpowers",
        branch: "main",
        enabled: true,
        skill_count: 12,
        star_count: 13600,
      },
    ],
    installed_skills: [
      {
        name: "superpowers",
        description: "包含 2 个技能",
        directory: "/tmp/aigate/tooling/skills/superpowers",
        source_client: "codex",
        source_kind: "codex",
        managed_path: "/tmp/aigate/tooling/skills/superpowers",
        installed_apps: { codex: true },
      },
      {
        name: "imagegen",
        description: "Generate or edit raster images.",
        directory: "/tmp/aigate/tooling/skills/imagegen",
        source_client: "codex",
        source_kind: "codex",
        managed_path: "/tmp/aigate/tooling/skills/imagegen",
        installed_apps: { codex: false },
      },
    ],
    repo_search_results: [
      {
        owner: "gcssloop",
        name: "codex-router-skills",
        branch: "main",
        url: "https://example.invalid/repo",
        description: "Test repo",
      },
    ],
    discovered_mcp_servers: [
      {
        id: "fetch",
        name: "Fetch Server",
        description: "Fetch MCP",
        source_apps: { codex: true },
        client_status: { codex: "enabled" },
        spec: { type: "stdio", command: "uvx" },
      },
    ],
    mcp_templates: [
      {
        id: "fetch",
        name: "mcp-server-fetch",
        description: "Quick template: mcp-fetch",
        type: "stdio",
        command: "uvx",
        args: ["mcp-server-fetch"],
      },
    ],
    mcp_servers: [
      {
        id: "fetch",
        name: "Fetch Server",
        description: "Fetch MCP",
        enabled_apps: { codex: true },
        client_status: { codex: "enabled" },
        spec: { type: "stdio", command: "node", args: ["/tmp/home/.codex/mcp/fetch/bin/server.js"] },
        delete_allowed: true,
        delete_targets: ["/tmp/home/.codex/mcp/fetch"],
      },
      {
        id: "time",
        name: "Time Server",
        description: "Time MCP",
        template_id: "time",
        enabled_apps: { codex: false },
        client_status: { codex: "disabled" },
        spec: { type: "stdio", command: "npx" },
        delete_allowed: false,
        delete_reason: "该 MCP 由 AI Gate 提供，需在来源侧管理。",
      },
    ],
  };
}

function buildDiscoveredSkillResponse(overrides?: Partial<api.ToolingSkillDiscoveryResponse>): api.ToolingSkillDiscoveryResponse {
  return {
    cached: true,
    fetched_at: "2026-04-08T10:00:00Z",
    total: 2,
    offset: 0,
    limit: 30,
    items: [
      {
        id: "github:openai/codex-skills:main:skills/zulu",
        name: "Zulu Skill",
        description: "Cached zulu summary",
        platform: "github",
        repo_owner: "openai",
        repo_name: "codex-skills",
        branch: "main",
        repo_url: "https://github.com/openai/codex-skills",
        source_path: "skills/zulu",
        source_url: "https://github.com/openai/codex-skills/tree/main/skills/zulu",
        managed_name: "codex-skills-zulu",
        content_hash: "hash-zulu-v1",
        installed_apps: { codex: false },
        update_available: false,
      },
      {
        id: "github:openai/codex-skills:main:skills/alpha",
        name: "Alpha Skill",
        description: "Cached alpha summary",
        platform: "github",
        repo_owner: "openai",
        repo_name: "codex-skills",
        branch: "main",
        repo_url: "https://github.com/openai/codex-skills",
        source_path: "skills/alpha",
        source_url: "https://github.com/openai/codex-skills/tree/main/skills/alpha",
        managed_name: "codex-skills-alpha",
        content_hash: "hash-alpha-v1",
        installed_apps: { codex: false },
        update_available: false,
      },
    ],
    ...overrides,
  };
}

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("ToolingPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getToolingState).mockResolvedValue(buildToolingState());
    vi.mocked(api.importToolingSkills).mockResolvedValue({ imported: 1 });
    vi.mocked(api.updateToolingSkill).mockResolvedValue({ applied: 1, enabled: true, skill_sync_method: "symlink" });
    vi.mocked(api.deleteToolingSkill).mockResolvedValue();
    vi.mocked((api as typeof api & { getToolingDiscoveredSkills: typeof vi.fn }).getToolingDiscoveredSkills).mockResolvedValue(buildDiscoveredSkillResponse());
    vi.mocked((api as typeof api & { refreshToolingDiscoveredSkills: typeof vi.fn }).refreshToolingDiscoveredSkills).mockResolvedValue(
      buildDiscoveredSkillResponse({
        cached: false,
        fetched_at: "2026-04-08T10:01:00Z",
        items: [
          {
            ...buildDiscoveredSkillResponse().items[1],
            description: "Latest alpha summary",
          },
          {
            ...buildDiscoveredSkillResponse().items[0],
            description: "Latest zulu summary",
          },
        ],
      }),
    );
    vi.mocked((api as typeof api & { installToolingDiscoveredSkill: typeof vi.fn }).installToolingDiscoveredSkill).mockResolvedValue({
      applied: 1,
      enabled: true,
      skill_sync_method: "symlink",
    });
    vi.mocked(api.searchToolingRepos).mockResolvedValue({ items: [] });
    vi.mocked((api as typeof api & { resolveToolingRepo: typeof vi.fn }).resolveToolingRepo).mockImplementation(async (input: string) => {
      if (input.includes("gitlab.com")) {
        return {
          platform: "gitlab",
          owner: "gitlab-org",
          name: "codex-superpowers",
          repo_url: "https://gitlab.com/gitlab-org/codex-superpowers",
          branch_options: ["main", "master", "develop"],
          selected_branch: "main",
        };
      }
      return {
        platform: "github",
        owner: "openai",
        name: "codex-superpowers",
        repo_url: "https://github.com/openai/codex-superpowers",
        branch_options: ["main", "release"],
        selected_branch: "main",
      };
    });
    vi.mocked(api.addToolingRepo).mockResolvedValue({
      platform: "github",
      owner: "openai",
      name: "codex-superpowers",
      branch: "main",
      enabled: true,
      skill_count: 12,
    });
    vi.mocked((api as typeof api & { updateToolingRepo: typeof vi.fn }).updateToolingRepo).mockResolvedValue({
      platform: "gitlab",
      owner: "gitlab-org",
      name: "codex-superpowers",
      branch: "develop",
      enabled: true,
      skill_count: 12,
    });
    vi.mocked(api.removeToolingRepo).mockResolvedValue();
    vi.mocked((api as typeof api & { reorderToolingRepos: typeof vi.fn }).reorderToolingRepos).mockImplementation(async (items) => {
      return items.map((item) => ({
        platform: item.platform ?? "github",
        owner: item.owner,
        name: item.name,
        branch: "main",
        enabled: true,
        skill_count: item.name === "codex-superpowers" ? 12 : 0,
      }));
    });
    vi.mocked(api.importToolingMcpServers).mockResolvedValue({ imported: 1 });
    vi.mocked(api.applyToolingMcpServer).mockResolvedValue();
    vi.mocked(api.installToolingMcpServer).mockResolvedValue(buildToolingState().mcp_servers[0]);
    vi.mocked(api.deleteToolingMcpServer).mockResolvedValue();
  });

  it("renders the minimal skills management layout", async () => {
    render(<ToolingPage mode="skills" t={(text) => text} />);

    expect(await screen.findByText("Skill 技能")).toBeInTheDocument();
    expect(screen.getByText("Codex 1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /导入已有/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /发现技能/ })).toBeInTheDocument();
    expect(screen.getByText("superpowers")).toBeInTheDocument();
    expect(screen.getByText("包含 2 个技能")).toBeInTheDocument();
    expect(screen.getByLabelText("Codex 已激活")).toBeInTheDocument();
    expect(screen.getByLabelText("Codex 未激活")).toBeInTheDocument();
    expect(screen.queryByText("Skills 管理")).not.toBeInTheDocument();
    expect(screen.queryByText("管理 Codex 技能集合。")).not.toBeInTheDocument();
    expect(screen.queryByText("来源统计")).not.toBeInTheDocument();
    expect(screen.queryByText("当前安装技能列表")).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText("搜索技能")).not.toBeInTheDocument();
    expect(screen.queryByText("仓库管理")).not.toBeInTheDocument();
  });

  it("imports, toggles, and deletes skill collections", async () => {
    const confirmSpy = vi.spyOn(Modal, "confirm").mockImplementation((config) => {
      void config.onOk?.();
      return {
        destroy: vi.fn(),
        update: vi.fn(),
      } as ReturnType<typeof Modal.confirm>;
    });

    render(<ToolingPage mode="skills" t={(text) => text} />);

    await screen.findByText("Skill 技能");
    fireEvent.click(screen.getByRole("button", { name: /导入已有/ }));
    await waitFor(() => expect(api.importToolingSkills).toHaveBeenCalledWith("codex"));

    fireEvent.click(screen.getByRole("button", { name: "启用 imagegen 到 Codex" }));
    await waitFor(() =>
      expect(api.updateToolingSkill).toHaveBeenCalledWith("imagegen", {
        apps: ["codex"],
        enabled: true,
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: "删除 superpowers" }));
    await waitFor(() => expect(confirmSpy).toHaveBeenCalled());
    await waitFor(() => expect(api.deleteToolingSkill).toHaveBeenCalledWith("superpowers"));
  });

  it("uses the managed collection directory when toggling and deleting skills", async () => {
    const managedCollection: api.ToolingSkillRecord = {
      name: "Superpowers Collection",
      description: "包含 2 个技能",
      directory: "/tmp/aigate/tooling/skills/superpowers",
      source_client: "codex",
      source_kind: "codex",
      managed_path: "/tmp/aigate/tooling/skills/superpowers",
      installed_apps: { codex: true },
    };
    vi.mocked(api.getToolingState)
      .mockResolvedValueOnce({
        ...buildToolingState(),
        installed_skills: [managedCollection],
      })
      .mockResolvedValue({
        ...buildToolingState(),
        installed_skills: [
          {
            ...managedCollection,
            installed_apps: { codex: false },
          },
        ],
      });

    const confirmSpy = vi.spyOn(Modal, "confirm").mockImplementation((config) => {
      void config.onOk?.();
      return {
        destroy: vi.fn(),
        update: vi.fn(),
      } as ReturnType<typeof Modal.confirm>;
    });

    render(<ToolingPage mode="skills" t={(text) => text} />);

    await screen.findByText("Superpowers Collection");
    fireEvent.click(screen.getByRole("button", { name: "停用 Superpowers Collection 于 Codex" }));
    await waitFor(() =>
      expect(api.updateToolingSkill).toHaveBeenCalledWith("superpowers", {
        apps: ["codex"],
        enabled: false,
      }),
    );

    await screen.findByRole("button", { name: "启用 Superpowers Collection 到 Codex" });
    const deleteButton = screen.getByRole("button", { name: "删除 Superpowers Collection" });
    await waitFor(() => expect(deleteButton).toBeEnabled());
    fireEvent.click(deleteButton);
    await waitFor(() => expect(confirmSpy).toHaveBeenCalled());
    await waitFor(() => expect(api.deleteToolingSkill).toHaveBeenCalledWith("superpowers"));
  });

  it("shows full-screen skills discovery with cached list by default and silently refreshes it", async () => {
    const refreshDeferred = createDeferred<api.ToolingSkillDiscoveryResponse>();
    vi.mocked((api as typeof api & { refreshToolingDiscoveredSkills: typeof vi.fn }).refreshToolingDiscoveredSkills).mockImplementationOnce(() => refreshDeferred.promise);

    render(<ToolingPage mode="skills" t={(text) => text} />);

    await screen.findByText("Skill 技能");
    fireEvent.click(screen.getByRole("button", { name: /发现技能/ }));

    const dialog = await screen.findByRole("dialog", { name: "发现技能" });
    expect(vi.mocked(api.getToolingDiscoveredSkills)).toHaveBeenCalledWith({ limit: 30, offset: 0 });
    expect(within(dialog).getByPlaceholderText("搜索发现的技能")).toBeInTheDocument();
    expect(await within(dialog).findByText("最新索引")).toBeInTheDocument();
    const skillTitles = (await within(dialog).findAllByTestId("tooling-discovered-skill-title")).map((node) => node.textContent);
    expect(skillTitles).toEqual(["Zulu Skill", "Alpha Skill"]);

    refreshDeferred.resolve(buildDiscoveredSkillResponse({
      cached: false,
      fetched_at: "2026-04-08T10:05:00Z",
      items: [
        {
          ...buildDiscoveredSkillResponse().items[1],
          description: "Latest alpha summary",
        },
        {
          ...buildDiscoveredSkillResponse().items[0],
          description: "Latest zulu summary",
        },
      ],
    }));

    expect(await within(dialog).findByText("Latest alpha summary")).toBeInTheDocument();
    expect(within(dialog).getByText("已更新")).toBeInTheDocument();
    expect(vi.mocked(api.refreshToolingDiscoveredSkills)).toHaveBeenCalledWith({ limit: 30, offset: 0 });
  });

  it("supports manual refresh, viewing source repo, and installing a discovered skill", async () => {
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);
    vi.mocked((api as typeof api & { getToolingState: typeof vi.fn }).getToolingState)
      .mockResolvedValueOnce(buildToolingState())
      .mockImplementation(() => new Promise(() => {}));
    vi.mocked((api as typeof api & { refreshToolingDiscoveredSkills: typeof vi.fn }).refreshToolingDiscoveredSkills)
      .mockResolvedValueOnce(
        buildDiscoveredSkillResponse({
          cached: false,
          fetched_at: "2026-04-08T10:01:00Z",
          items: [
            {
              ...buildDiscoveredSkillResponse().items[1],
              description: "Latest alpha summary",
            },
            {
              ...buildDiscoveredSkillResponse().items[0],
              description: "Latest zulu summary",
            },
          ],
        }),
      )
      .mockImplementationOnce(() => new Promise(() => {}));

    render(<ToolingPage mode="skills" t={(text) => text} />);

    await screen.findByText("Skill 技能");
    fireEvent.click(screen.getByRole("button", { name: /发现技能/ }));

    const dialog = await screen.findByRole("dialog", { name: "发现技能" });
    fireEvent.click(within(dialog).getByRole("button", { name: "刷新技能索引" }));

    await waitFor(() => expect(vi.mocked(api.refreshToolingDiscoveredSkills)).toHaveBeenCalledWith({ limit: 30, offset: 0 }));

    fireEvent.click(within(dialog).getByRole("button", { name: "查看 Alpha Skill 的仓库页面" }));
    expect(openSpy).toHaveBeenCalledWith("https://github.com/openai/codex-skills/tree/main/skills/alpha", "_blank", "noopener,noreferrer");

    fireEvent.click(within(dialog).getByRole("button", { name: "打开 Alpha Skill 的源目录" }));
    expect(openSpy).toHaveBeenCalledWith("https://github.com/openai/codex-skills/tree/main/skills/alpha", "_blank", "noopener,noreferrer");

    fireEvent.click(within(dialog).getByRole("button", { name: "安装 Alpha Skill" }));
    await waitFor(() =>
      expect(vi.mocked((api as typeof api & { installToolingDiscoveredSkill: typeof vi.fn }).installToolingDiscoveredSkill)).toHaveBeenCalledWith({
        id: "github:openai/codex-skills:main:skills/alpha",
        apps: ["codex"],
      }),
    );
    await waitFor(() =>
      expect(within(dialog).getByRole("button", { name: "已安装 Alpha Skill" })).toBeDisabled(),
    );
  });

  it("distinguishes install states in discovered skill cards", async () => {
    vi.mocked((api as typeof api & { getToolingDiscoveredSkills: typeof vi.fn }).getToolingDiscoveredSkills).mockResolvedValue(
      buildDiscoveredSkillResponse({
        items: [
          {
            ...buildDiscoveredSkillResponse().items[1],
            installed_apps: { codex: true },
            update_available: true,
          },
          {
            ...buildDiscoveredSkillResponse().items[0],
            installed_apps: { codex: true },
            update_available: false,
          },
        ],
      }),
    );
    vi.mocked((api as typeof api & { refreshToolingDiscoveredSkills: typeof vi.fn }).refreshToolingDiscoveredSkills).mockResolvedValue(
      buildDiscoveredSkillResponse({
        cached: false,
        items: [
          {
            ...buildDiscoveredSkillResponse().items[1],
            installed_apps: { codex: true },
            update_available: true,
          },
          {
            ...buildDiscoveredSkillResponse().items[0],
            installed_apps: { codex: true },
            update_available: false,
          },
        ],
      }),
    );

    render(<ToolingPage mode="skills" t={(text) => text} />);

    await screen.findByText("Skill 技能");
    fireEvent.click(screen.getByRole("button", { name: /发现技能/ }));

    const dialog = await screen.findByRole("dialog", { name: "发现技能" });
    expect(within(dialog).getByText("可更新")).toBeInTheDocument();
    expect(within(dialog).getAllByText("已安装").length).toBeGreaterThan(0);
    expect(within(dialog).getByRole("button", { name: "更新 Alpha Skill" })).toBeEnabled();
    expect(within(dialog).getByRole("button", { name: "已安装 Zulu Skill" })).toBeDisabled();
  });

  it("manages repositories with a compact list and edit/delete actions", async () => {
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);
    render(<ToolingPage mode="skills" t={(text) => text} />);

    await screen.findByText("Skill 技能");
    fireEvent.click(screen.getByRole("button", { name: /发现技能/ }));

    const discoverDialog = await screen.findByRole("dialog", { name: "发现技能" });
    fireEvent.click(within(discoverDialog).getByRole("button", { name: "仓库管理" }));

    const repoDialog = (await screen.findAllByRole("dialog")).at(-1)!;
    expect(within(repoDialog).queryByLabelText("仓库平台")).not.toBeInTheDocument();
    expect(within(repoDialog).getByText("openai/codex-superpowers")).toBeInTheDocument();
    expect(within(repoDialog).getByText("12 skills")).toBeInTheDocument();
    expect(within(repoDialog).getByText("13.6K stars")).toBeInTheDocument();
    expect(within(repoDialog).getByText("https://github.com/openai/codex-superpowers")).toBeInTheDocument();
    expect(within(repoDialog).queryByRole("button", { name: "添加仓库" })).not.toBeInTheDocument();

    fireEvent.click(within(repoDialog).getByRole("button", { name: "查看仓库 openai/codex-superpowers" }));
    expect(openSpy).toHaveBeenCalledWith("https://github.com/openai/codex-superpowers", "_blank", "noopener,noreferrer");

    fireEvent.click(within(repoDialog).getByRole("button", { name: "编辑仓库 openai/codex-superpowers" }));

    const editDialog = (await screen.findAllByRole("dialog")).at(-1)!;
    expect(within(editDialog).getByDisplayValue("https://github.com/openai/codex-superpowers")).toBeDisabled();
    fireEvent.mouseDown(within(editDialog).getByLabelText("分支"));
    fireEvent.click((await screen.findAllByText("release")).at(-1)!);
    fireEvent.click(within(editDialog).getByRole("button", { name: "保存仓库" }));

    await waitFor(() =>
      expect(vi.mocked((api as typeof api & { updateToolingRepo: typeof vi.fn }).updateToolingRepo)).toHaveBeenCalledWith(
        "github",
        "openai",
        "codex-superpowers",
        {
          platform: "github",
          owner: "openai",
          name: "codex-superpowers",
          branch: "release",
        },
      ),
    );

    fireEvent.click(within(repoDialog).getByRole("button", { name: "删除仓库 openai/codex-superpowers" }));
    await waitFor(() => expect(api.removeToolingRepo).toHaveBeenCalledWith("github", "openai", "codex-superpowers"));
  });

  it("supports discovery pagination query", async () => {
    vi.mocked(api.getToolingDiscoveredSkills)
      .mockResolvedValueOnce(
        buildDiscoveredSkillResponse({
          total: 35,
          offset: 0,
          limit: 30,
        }),
      )
      .mockResolvedValueOnce(
        buildDiscoveredSkillResponse({
          total: 35,
          offset: 30,
          limit: 30,
          items: [
            {
              ...buildDiscoveredSkillResponse().items[0],
              id: "github:openai/codex-skills:main:skills/page-2",
              name: "Page Two Skill",
            },
          ],
        }),
      );

    vi.mocked(api.refreshToolingDiscoveredSkills).mockResolvedValue(
      buildDiscoveredSkillResponse({
        cached: false,
        total: 35,
        offset: 0,
        limit: 30,
      }),
    );

    render(<ToolingPage mode="skills" t={(text) => text} />);
    await screen.findByText("Skill 技能");
    fireEvent.click(screen.getByRole("button", { name: /发现技能/ }));
    const dialog = await screen.findByRole("dialog", { name: "发现技能" });
    await within(dialog).findByText("Alpha Skill");

    fireEvent.click(within(dialog).getByRole("listitem", { name: "2" }));
    await within(dialog).findByText("Page Two Skill");

    expect(vi.mocked(api.getToolingDiscoveredSkills)).toHaveBeenLastCalledWith({ limit: 30, offset: 30 });
  });

  it("renders the minimal mcp management layout and toggles codex sync", async () => {
    render(<ToolingPage mode="mcp" t={(text) => text} />);

    expect(await screen.findByText("MCP 服务")).toBeInTheDocument();
    expect(screen.getByText("Codex 2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /导入已有/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /发现服务/ })).not.toBeInTheDocument();
    expect(screen.getByText("fetch")).toBeInTheDocument();
    expect(screen.getAllByLabelText("Codex 已激活").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("Codex 未激活")).toBeInTheDocument();
    expect(screen.queryByText("MCP 管理")).not.toBeInTheDocument();
    expect(screen.queryByText("管理 Codex MCP 服务。")).not.toBeInTheDocument();
    expect(screen.queryByText("来源统计")).not.toBeInTheDocument();
    expect(screen.queryByText("当前安装服务列表")).not.toBeInTheDocument();
    expect(screen.queryByText("快速模板")).not.toBeInTheDocument();
    expect(screen.queryByText("Fetch MCP")).not.toBeInTheDocument();
    expect(screen.queryByText("Time MCP")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "停用 fetch 于 Codex" }));

    await waitFor(() =>
      expect(api.applyToolingMcpServer).toHaveBeenCalledWith("fetch", ["codex"], false),
    );
  });

  it("shows binary mcp servers with friendly title and path as subtitle", async () => {
    const binaryPath = "/Applications/Pencil.app/Contents/Resources/app.asar.unpacked/out/mcp-server-darwin-arm64";
    vi.mocked(api.getToolingState).mockResolvedValue({
      ...buildToolingState(),
      mcp_servers: [
        {
          id: "pencil",
          name: binaryPath,
          enabled_apps: { codex: true },
          client_status: { codex: "enabled" },
          spec: { type: "stdio", command: binaryPath },
          delete_allowed: true,
          delete_targets: ["/tmp/home/.codex/mcp/pencil"],
        },
      ],
    });

    render(<ToolingPage mode="mcp" t={(text) => text} />);

    expect(await screen.findByText("pencil")).toBeInTheDocument();
    expect(screen.getByText(binaryPath)).toBeInTheDocument();
  });

  it("confirms mcp deletion with local cleanup checked by default", async () => {
    let cleanupContent: ReactNode;
    const confirmSpy = vi.spyOn(Modal, "confirm").mockImplementation((config) => {
      cleanupContent = config.content;
      void config.onOk?.();
      return {
        destroy: vi.fn(),
        update: vi.fn(),
      } as ReturnType<typeof Modal.confirm>;
    });

    render(<ToolingPage mode="mcp" t={(text) => text} />);

    await screen.findByText("MCP 服务");
    fireEvent.click(screen.getByRole("button", { name: "删除 fetch" }));

    await waitFor(() => expect(confirmSpy).toHaveBeenCalled());
    render(<>{cleanupContent}</>);
    expect(screen.getByText("将同时从 AI Gate 与 Codex 删除 Fetch Server")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "同步清理本地文件" })).toBeChecked();
    expect(screen.getByText("以下本地路径将被删除")).toBeInTheDocument();
    const deleteTarget = screen.getByText("/tmp/home/.codex/mcp/fetch");
    expect(deleteTarget).toBeInTheDocument();
    expect(deleteTarget).toHaveClass("tooling-delete-target-path");
    await waitFor(() => expect(api.deleteToolingMcpServer).toHaveBeenCalledWith("fetch", true));
  });

  it("blocks deleting non-removable mcp servers and explains why", async () => {
    const warningSpy = vi.spyOn(Modal, "warning").mockImplementation(() => ({
      destroy: vi.fn(),
      update: vi.fn(),
    }) as ReturnType<typeof Modal.warning>);

    render(<ToolingPage mode="mcp" t={(text) => text} />);

    await screen.findByText("MCP 服务");
    fireEvent.click(screen.getByRole("button", { name: "删除 time" }));

    await waitFor(() => expect(warningSpy).toHaveBeenCalled());
    expect(api.deleteToolingMcpServer).not.toHaveBeenCalled();
  });

  it("keeps the current content visible while a toggle refresh is pending", async () => {
    const initialState = buildToolingState();
    const refreshDeferred = createDeferred<api.ToolingState>();
    vi.mocked(api.getToolingState)
      .mockResolvedValueOnce(initialState)
      .mockImplementationOnce(() => refreshDeferred.promise);

    render(<ToolingPage mode="skills" t={(text) => text} />);

    await screen.findByText("imagegen");
    fireEvent.click(screen.getByRole("button", { name: "启用 imagegen 到 Codex" }));

    await waitFor(() =>
      expect(api.updateToolingSkill).toHaveBeenCalledWith("imagegen", {
        apps: ["codex"],
        enabled: true,
      }),
    );

    expect(screen.getByText("Skill 技能")).toBeInTheDocument();
    expect(screen.getByText("imagegen")).toBeInTheDocument();

    refreshDeferred.resolve(initialState);
    await screen.findByText("imagegen");
  });
});
