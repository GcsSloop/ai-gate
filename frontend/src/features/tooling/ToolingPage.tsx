import {
  CheckCircleOutlined,
  CloudDownloadOutlined,
  DeleteOutlined,
  PlusOutlined,
  SearchOutlined,
} from "@ant-design/icons";
import { Button, Card, Checkbox, Empty, Input, Modal, Space, Spin, Typography, message } from "antd";
import { useEffect, useMemo, useState } from "react";

import {
  addToolingRepo,
  applyToolingMcpServer,
  deleteToolingMcpServer,
  deleteToolingSkill,
  getToolingState,
  importToolingMcpServers,
  importToolingSkills,
  removeToolingRepo,
  searchToolingRepos,
  updateToolingSkill,
  type ToolingMcpServer,
  type ToolingRepoSearchResult,
  type ToolingSkillRecord,
  type ToolingState,
} from "../../lib/api";
import sourceOpenAIIcon from "../../assets/providers/openai.png";

type ToolingMode = "skills" | "mcp";

type ToolingPageProps = {
  mode: ToolingMode;
  t: (text: string) => string;
};

type RepoSearchRow = ToolingRepoSearchResult & {
  key: string;
};

type ManagedClientDescriptor = {
  id: string;
  label: string;
  icon: string;
};

type ManagedClientStatus = ManagedClientDescriptor & {
  enabled: boolean;
};

const managedClients: ManagedClientDescriptor[] = [
  {
    id: "codex",
    label: "Codex",
    icon: sourceOpenAIIcon,
  },
];

export function ToolingPage({ mode, t }: ToolingPageProps) {
  const [messageApi, contextHolder] = message.useMessage();
  const [loading, setLoading] = useState(true);
  const [state, setState] = useState<ToolingState | null>(null);
  const [skillDiscoverOpen, setSkillDiscoverOpen] = useState(false);
  const [repoQuery, setRepoQuery] = useState("");
  const [repoSearchLoading, setRepoSearchLoading] = useState(false);
  const [repoSearchResults, setRepoSearchResults] = useState<RepoSearchRow[]>([]);
  const [importBusy, setImportBusy] = useState(false);
  const [repoBusyKey, setRepoBusyKey] = useState<string | null>(null);
  const [skillBusyName, setSkillBusyName] = useState<string | null>(null);
  const [mcpBusyId, setMcpBusyId] = useState<string | null>(null);

  async function reload(options?: { background?: boolean }) {
    const background = options?.background ?? false;
    if (!background) {
      setLoading(true);
    }
    try {
      const next = await getToolingState();
      setState(next);
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("加载技能与 MCP 管理失败"));
    } finally {
      if (!background) {
        setLoading(false);
      }
    }
  }

  useEffect(() => {
    void reload();
  }, []);

  const skillCount = state?.skill_stats.by_source.codex ?? 0;
  const mcpCount = state?.mcp_servers.length ?? 0;
  const repoRows = repoSearchResults.length > 0
    ? repoSearchResults
    : (state?.repo_search_results ?? []).map((item) => ({ ...item, key: `${item.owner}/${item.name}` }));

  const skills = useMemo(() => state?.installed_skills ?? [], [state?.installed_skills]);
  const mcpServers = useMemo(() => state?.mcp_servers ?? [], [state?.mcp_servers]);

  async function handleImportSkills() {
    setImportBusy(true);
    try {
      await importToolingSkills("codex");
      void messageApi.success(t("导入完成"));
      await reload({ background: true });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("导入失败"));
    } finally {
      setImportBusy(false);
    }
  }

  async function handleSkillToggle(skill: ToolingSkillRecord) {
    const enabled = !skill.installed_apps?.codex;
    const collectionName = skillCollectionName(skill);
    setSkillBusyName(skill.name);
    try {
      await updateToolingSkill(collectionName, { apps: ["codex"], enabled });
      void messageApi.success(enabled ? t("已启用到 Codex") : t("已从 Codex 停用"));
      await reload({ background: true });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("技能同步失败"));
    } finally {
      setSkillBusyName(null);
    }
  }

  function confirmDeleteSkill(skill: ToolingSkillRecord) {
    Modal.confirm({
      title: t("确认删除技能"),
      content: t(`将同时从 AI Gate 与 Codex 删除 ${skill.name}`),
      okText: t("删除"),
      cancelText: t("取消"),
      okButtonProps: { danger: true },
      onOk: async () => {
        const collectionName = skillCollectionName(skill);
        setSkillBusyName(skill.name);
        try {
          await deleteToolingSkill(collectionName);
          void messageApi.success(t("删除成功"));
          await reload({ background: true });
        } catch (error) {
          void messageApi.error(error instanceof Error ? error.message : t("删除失败"));
        } finally {
          setSkillBusyName(null);
        }
      },
    });
  }

  async function handleImportMcp() {
    setImportBusy(true);
    try {
      await importToolingMcpServers("codex");
      void messageApi.success(t("导入完成"));
      await reload({ background: true });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("导入失败"));
    } finally {
      setImportBusy(false);
    }
  }

  async function handleMcpToggle(server: ToolingMcpServer) {
    const enabled = !server.enabled_apps?.codex;
    setMcpBusyId(server.id);
    try {
      await applyToolingMcpServer(server.id, ["codex"], enabled);
      void messageApi.success(enabled ? t("已启用到 Codex") : t("已从 Codex 停用"));
      await reload({ background: true });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("MCP 同步失败"));
    } finally {
      setMcpBusyId(null);
    }
  }

  function confirmDeleteMcp(server: ToolingMcpServer) {
    if (server.delete_allowed === false) {
      Modal.warning({
        title: t("无法删除 MCP"),
        content: server.delete_reason || t("该 MCP 不能在这里删除。"),
        okText: t("知道了"),
        centered: true,
      });
      return;
    }
    let cleanupLocalFiles = true;
    Modal.confirm({
      title: t("确认删除 MCP"),
      content: (
        <DeleteMcpConfirmContent
          server={server}
          t={t}
          onCleanupChange={(next) => {
            cleanupLocalFiles = next;
          }}
        />
      ),
      okText: t("删除"),
      cancelText: t("取消"),
      centered: true,
      okButtonProps: { danger: true },
      onOk: async () => {
        setMcpBusyId(server.id);
        try {
          await deleteToolingMcpServer(server.id, cleanupLocalFiles);
          void messageApi.success(t("删除成功"));
          await reload({ background: true });
        } catch (error) {
          void messageApi.error(error instanceof Error ? error.message : t("删除失败"));
        } finally {
          setMcpBusyId(null);
        }
      },
    });
  }

  async function handleRepoSearch() {
    const query = repoQuery.trim();
    if (!query) {
      setRepoSearchResults([]);
      return;
    }
    setRepoSearchLoading(true);
    try {
      const result = await searchToolingRepos(query);
      setRepoSearchResults(result.items.map((item) => ({ ...item, key: `${item.owner}/${item.name}` })));
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("搜索失败"));
    } finally {
      setRepoSearchLoading(false);
    }
  }

  async function handleRepoAdd(owner: string, name: string, branch = "main") {
    const key = `${owner}/${name}`;
    setRepoBusyKey(key);
    try {
      await addToolingRepo(owner, name, branch);
      void messageApi.success(t("仓库已添加"));
      await reload({ background: true });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("添加失败"));
    } finally {
      setRepoBusyKey(null);
    }
  }

  async function handleRepoRemove(owner: string, name: string) {
    const key = `${owner}/${name}`;
    setRepoBusyKey(key);
    try {
      await removeToolingRepo(owner, name);
      void messageApi.success(t("仓库已删除"));
      await reload({ background: true });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("删除失败"));
    } finally {
      setRepoBusyKey(null);
    }
  }

  if (loading || !state) {
    return (
      <div className="dashboard-page tooling-page tooling-loading">
        {contextHolder}
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div className="dashboard-page tooling-page tooling-minimal-page">
      {contextHolder}
      <Card className="tooling-card tooling-minimal-shell">
        <div className="tooling-minimal-toolbar">
          <div className="tooling-minimal-titlebar">
            <div className="tooling-inline-title">
              {mode === "skills" ? t("Skill 技能") : t("MCP 服务")}
            </div>
            <div className="tooling-inline-count">{`Codex ${mode === "skills" ? skillCount : mcpCount}`}</div>
          </div>
          <Space className="tooling-minimal-actions" wrap size={10}>
            <Button
              type="text"
              className="tooling-action-button"
              icon={<CloudDownloadOutlined />}
              loading={importBusy}
              onClick={() => void (mode === "skills" ? handleImportSkills() : handleImportMcp())}
            >
              {t("导入已有")}
            </Button>
            {mode === "skills" ? (
              <Button
                type="text"
                className="tooling-action-button"
                icon={<SearchOutlined />}
                onClick={() => setSkillDiscoverOpen(true)}
              >
                {t("发现技能")}
              </Button>
            ) : null}
          </Space>
        </div>

        <div className="tooling-managed-list">
          {mode === "skills" ? (
            skills.length > 0 ? (
              skills.map((skill) => (
                <ManagedCard
                  key={skill.managed_path}
                  title={skill.name}
                  description={skill.description || skill.directory}
                  statuses={buildManagedStatuses(skill.installed_apps)}
                  busy={skillBusyName === skill.name}
                  toggleLabel={skill.installed_apps?.codex ? t(`停用 ${skill.name} 于 Codex`) : t(`启用 ${skill.name} 到 Codex`)}
                  deleteLabel={t(`删除 ${skill.name}`)}
                  onToggle={() => void handleSkillToggle(skill)}
                  onDelete={() => confirmDeleteSkill(skill)}
                />
              ))
            ) : (
              <Empty description={t("暂无技能")} />
            )
          ) : mcpServers.length > 0 ? (
            mcpServers.map((server) => {
              const display = describeMcpCard(server);
              return (
                <ManagedCard
                  key={server.id}
                  title={display.title}
                  description={display.subtitle}
                  titleVariant="account"
                  descriptionVariant="account"
                  statuses={buildManagedStatuses(server.enabled_apps)}
                  busy={mcpBusyId === server.id}
                  toggleLabel={server.enabled_apps?.codex ? t(`停用 ${display.title} 于 Codex`) : t(`启用 ${display.title} 到 Codex`)}
                  deleteLabel={t(`删除 ${display.title}`)}
                  onToggle={() => void handleMcpToggle(server)}
                  onDelete={() => confirmDeleteMcp(server)}
                />
              );
            })
          ) : (
            <Empty description={t("暂无 MCP")} />
          )}
        </div>
      </Card>

      <Modal
        open={skillDiscoverOpen}
        title={t("发现技能")}
        onCancel={() => setSkillDiscoverOpen(false)}
        footer={null}
        centered
        destroyOnHidden
      >
        <Space orientation="vertical" size={16} style={{ width: "100%" }}>
          <div>
            <div className="tooling-modal-section-title">{t("仓库搜索")}</div>
            <Space.Compact className="tooling-search-bar">
              <Input value={repoQuery} onChange={(event) => setRepoQuery(event.target.value)} placeholder={t("输入仓库关键词")} />
              <Button icon={<SearchOutlined />} loading={repoSearchLoading} onClick={() => void handleRepoSearch()}>
                {t("搜索")}
              </Button>
            </Space.Compact>
            <div className="tooling-discovery-list">
              {repoRows.length > 0 ? repoRows.map((repo) => (
                <DiscoveryRow
                  key={repo.key}
                  title={`${repo.owner}/${repo.name}`}
                  description={repo.description || repo.branch}
                  actionLabel={t("添加")}
                  actionBusy={repoBusyKey === `${repo.owner}/${repo.name}`}
                  onAction={() => void handleRepoAdd(repo.owner, repo.name, repo.branch)}
                />
              )) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t("暂无搜索结果")} />}
            </div>
          </div>

          <div>
            <div className="tooling-modal-section-title">{t("仓库管理")}</div>
            <div className="tooling-discovery-list">
              {state.skill_repos.length > 0 ? state.skill_repos.map((repo) => (
                <DiscoveryRow
                  key={`${repo.owner}/${repo.name}`}
                  title={`${repo.owner}/${repo.name}`}
                  description={`${repo.branch} · ${repo.skill_count} skills`}
                  actionLabel={t("移除")}
                  danger
                  actionBusy={repoBusyKey === `${repo.owner}/${repo.name}`}
                  onAction={() => void handleRepoRemove(repo.owner, repo.name)}
                />
              )) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t("暂无已管理仓库")} />}
            </div>
          </div>
        </Space>
      </Modal>
    </div>
  );
}

function DeleteMcpConfirmContent({
  server,
  t,
  onCleanupChange,
}: {
  server: ToolingMcpServer;
  t: (text: string) => string;
  onCleanupChange: (next: boolean) => void;
}) {
  const [cleanupLocalFiles, setCleanupLocalFiles] = useState(true);
  const deleteTargets = server.delete_targets ?? [];

  useEffect(() => {
    onCleanupChange(cleanupLocalFiles);
  }, [cleanupLocalFiles, onCleanupChange]);

  return (
    <Space orientation="vertical" size={10} className="tooling-delete-confirm-content">
      <div>{t(`将同时从 AI Gate 与 Codex 删除 ${server.name}`)}</div>
      <Checkbox
        defaultChecked
        checked={cleanupLocalFiles}
        onChange={(event) => {
          setCleanupLocalFiles(event.target.checked);
        }}
      >
        {t("同步清理本地文件")}
      </Checkbox>
      <Typography.Text type="secondary">
        {t("仅清理 Codex 直接托管的当前 MCP 目录，默认开启。")}
      </Typography.Text>
      {cleanupLocalFiles && deleteTargets.length > 0 ? (
        <div className="tooling-delete-targets" role="list" aria-label={t("待删除路径列表")}>
          <div className="tooling-delete-targets-title">{t("以下本地路径将被删除")}</div>
          {deleteTargets.map((target) => (
            <div key={target} role="listitem" className="tooling-delete-target-path">
              {target}
            </div>
          ))}
        </div>
      ) : null}
    </Space>
  );
}

function skillCollectionName(skill: ToolingSkillRecord): string {
  const trimmed = skill.managed_path.trim();
  const parts = trimmed.split(/[\\/]/).filter(Boolean);
  return parts.at(-1) ?? skill.name;
}

function buildManagedStatuses(enabledApps?: Record<string, boolean>): ManagedClientStatus[] {
  return managedClients.map((client) => ({
    ...client,
    enabled: Boolean(enabledApps?.[client.id]),
  }));
}

function describeMcpCard(server: ToolingMcpServer): { title: string; subtitle?: string } {
  const displayPath = getMcpDisplayPath(server);
  if (!displayPath) {
    return { title: normalizeMcpTitle(server.name) };
  }
  const friendlyTitle = normalizeMcpTitle(displayPath);
  return {
    title: friendlyTitle,
    subtitle: displayPath,
  };
}

function getMcpDisplayPath(server: ToolingMcpServer): string | undefined {
  if (looksLikeLocalPath(server.name)) {
    return server.name.trim();
  }
  const command = server.spec?.command;
  if (typeof command === "string" && looksLikeLocalPath(command)) {
    return command.trim();
  }
  return undefined;
}

function looksLikeLocalPath(value?: string): boolean {
  if (!value) {
    return false;
  }
  const trimmed = value.trim();
  return (
    trimmed.startsWith("/") ||
    trimmed.startsWith("~/") ||
    trimmed.startsWith("./") ||
    trimmed.startsWith("../") ||
    trimmed.startsWith("\\\\") ||
    /^[A-Za-z]:[\\/]/.test(trimmed)
  );
}

function normalizeMcpTitle(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    return value;
  }

  const fromPath = normalizeMcpToken(pathLikeBaseName(trimmed));
  if (fromPath) {
    return fromPath;
  }

  const fromBundle = inferAppBundleName(trimmed);
  if (fromBundle) {
    return fromBundle;
  }

  const fromRaw = normalizeMcpToken(trimmed);
  if (fromRaw) {
    return fromRaw;
  }

  return trimmed;
}

function pathLikeBaseName(value: string): string {
  return value.split(/[\\/]/).filter(Boolean).at(-1) ?? value;
}

function inferAppBundleName(value: string): string | undefined {
  const segment = value
    .split(/[\\/]/)
    .find((part) => part.toLowerCase().endsWith(".app"));
  if (!segment) {
    return undefined;
  }
  const appName = segment.slice(0, -4).trim();
  return appName ? appName.toLowerCase() : undefined;
}

function normalizeMcpToken(value: string): string | undefined {
  const scopedBase = value.trim().split("/").filter(Boolean).at(-1) ?? value.trim();
  let normalized = scopedBase.toLowerCase();
  normalized = normalized.replace(/\.(exe|cmd|bat|sh)$/i, "");
  normalized = normalized.replace(/[-_](darwin|linux|windows)([-_](amd64|arm64|x64|x86_64|aarch64))?$/i, "");
  normalized = normalized.replace(/^@[^/]+\//, "");
  normalized = normalized.replace(/^(?:mcp[-_]?server[-_]?|server[-_]?|mcp[-_]?)+/i, "");
  normalized = normalized.replace(/[-_]?server$/i, "");
  normalized = normalized.replace(/\s+server$/i, "");
  normalized = normalized.replace(/^[-_.\s]+|[-_.\s]+$/g, "");

  if (!normalized || normalized === "mcp") {
    return undefined;
  }
  return normalized;
}

function ManagedCard({
  title,
  description,
  titleVariant = "default",
  descriptionVariant = "default",
  statuses,
  busy,
  toggleLabel,
  deleteLabel,
  onToggle,
  onDelete,
}: {
  title: string;
  description?: string;
  titleVariant?: "default" | "account";
  descriptionVariant?: "default" | "account";
  statuses: ManagedClientStatus[];
  busy?: boolean;
  toggleLabel: string;
  deleteLabel: string;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const primaryStatus = statuses[0];
  const isPrimaryEnabled = primaryStatus?.enabled ?? false;
  return (
    <div className="tooling-item-card">
      <div className="tooling-item-main">
        <div className={`tooling-item-title ${titleVariant === "account" ? "is-account" : ""}`}>{title}</div>
        {description ? (
          <div className={`tooling-item-description ${descriptionVariant === "account" ? "is-account" : "is-default"}`}>
            {description}
          </div>
        ) : null}
      </div>
      <div className="tooling-item-aside">
        <div className="tooling-item-statuses" aria-label="安装目标状态">
          {statuses.map((status) => (
            <div
              key={status.id}
              className={`tooling-status-pill ${status.enabled ? "is-enabled" : "is-disabled"}`}
              aria-label={`${status.label} ${status.enabled ? "已激活" : "未激活"}`}
              title={`${status.label} ${status.enabled ? "已激活" : "未激活"}`}
            >
              <img src={status.icon} alt="" aria-hidden="true" className="tooling-provider-icon tooling-provider-icon-small" />
              <span>{status.label}</span>
            </div>
          ))}
        </div>
        <div className="tooling-item-hover-actions">
          <button
            type="button"
            className={`tooling-item-primary-action ${isPrimaryEnabled ? "is-danger" : "is-primary"}`}
            aria-label={toggleLabel}
            onClick={onToggle}
            disabled={busy}
          >
            <CheckCircleOutlined className="tooling-item-primary-action-icon" />
            {isPrimaryEnabled ? "停用" : "启用"}
          </button>
          <button
            type="button"
            className="tooling-item-icon-action tooling-item-icon-action-danger"
            aria-label={deleteLabel}
            onClick={onDelete}
            disabled={busy}
          >
            <DeleteOutlined />
          </button>
        </div>
      </div>
    </div>
  );
}

function DiscoveryRow({
  title,
  description,
  actionLabel,
  actionBusy,
  danger = false,
  onAction,
}: {
  title: string;
  description: string;
  actionLabel: string;
  actionBusy?: boolean;
  danger?: boolean;
  onAction: () => void;
}) {
  return (
    <div className="tooling-repo-row">
      <div className="tooling-repo-main">
        <div className="tooling-repo-title">{title}</div>
        <div className="stats-subtitle">{description}</div>
      </div>
      <Button size="small" danger={danger} loading={actionBusy} icon={danger ? <DeleteOutlined /> : <PlusOutlined />} onClick={onAction}>
        {actionLabel}
      </Button>
    </div>
  );
}
