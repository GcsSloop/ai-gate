import {
  BarChartOutlined,
  CheckCircleOutlined,
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  ExportOutlined,
  HolderOutlined,
  InfoCircleOutlined,
  PlusOutlined,
} from "@ant-design/icons";
import {
  App as AntApp,
  Avatar,
  Button,
  Card,
  Descriptions,
  Dropdown,
  Empty,
  Form,
  Input,
  Modal,
  Select,
  Skeleton,
  Statistic,
  Tag,
  Tooltip,
  Typography,
  message,
} from "antd";
import {
  DndContext,
  DragOverlay,
  MouseSensor,
  PointerSensor,
  closestCenter,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type CSSProperties,
  type FocusEvent as ReactFocusEvent,
  type MouseEvent as ReactMouseEvent,
  type ReactNode,
} from "react";

import {
  createAccount,
  deleteAccount,
  duplicateAccount,
  fetchPPChatTokenLogs,
  getLuaUsageScript,
  importCurrentCodexAuth,
  importSharedAccount,
  listLuaUsageScripts,
  listAccountUsage,
  listAccounts,
  refreshAccountUsage,
  saveLuaUsageScript,
  shareAccount,
  testAccount,
  testLuaUsage,
  updateAccount,
  type AccountRecord,
  type PPChatTokenLogsPayload,
  type AccountTestResult,
} from "../../lib/api";
import { writeClipboardText } from "../../lib/clipboard";
import { refreshDesktopTrayState } from "../../lib/desktop-shell";
import type { AppLanguage, Translator } from "../../lib/i18n";
import sourceClaudeCodeIcon from "../../assets/providers/claude-code.png";
import sourceOpenAIIcon from "../../assets/providers/openai.png";
import sourcePPChatIcon from "../../assets/providers/ppchat.png";

const { Title, Text } = Typography;

const defaultBaseURL = "https://code.ppchat.vip/v1";
type AddModalMode = "official" | "third_party" | "shared_import" | null;

const statusColorMap: Record<string, string> = {
  active: "green",
  cooldown: "gold",
  degraded: "orange",
  invalid: "red",
  disabled: "default",
};

const statusTextMap: Record<string, string> = {
  active: "可用",
  cooldown: "冷却中",
  degraded: "降级",
  invalid: "失效",
  disabled: "已停用",
};

const routingCooldownTextMap: Record<string, string> = {
  official_remaining_below_3pct: "路由冷却",
  usage_limited: "路由冷却",
  capacity_failed: "路由冷却",
  rate_limited: "路由冷却",
  routing_cooldown: "路由冷却",
};

const authModeTextMap: Record<string, string> = {
  api_key: "API Key",
  oauth: "官方授权",
  codex_local_import: "本地导入",
};

type SourceIcon = "openai" | "claude_code" | "ppchat";

const sourceIconMap: Record<SourceIcon, { label: string; icon: string }> = {
  openai: { label: "OpenAI", icon: sourceOpenAIIcon },
  claude_code: { label: "Claude Code", icon: sourceClaudeCodeIcon },
  ppchat: { label: "PPChat", icon: sourcePPChatIcon },
};

function normalizeSourceIcon(raw?: string): SourceIcon {
  if (raw === "ppchat") {
    return "ppchat";
  }
  return raw === "claude_code" ? "claude_code" : "openai";
}

function inferSourceIconByBaseURL(baseURL: string): SourceIcon {
  if (/ppchat\.vip/i.test(baseURL)) {
    return "ppchat";
  }
  return "openai";
}

function parseUsageConfig(raw?: string): Record<string, unknown> {
  if (!raw || raw.trim() === "") {
    return {};
  }
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    if (!parsed || Array.isArray(parsed)) {
      return {};
    }
    return parsed;
  } catch {
    return {};
  }
}

function stringifyUsageConfig(value: Record<string, unknown>): string {
  return JSON.stringify(value);
}

function getManagedLuaScriptKey(raw?: string): string {
  const parsed = parseUsageConfig(raw);
  const script = typeof parsed.script === "string" ? String(parsed.script) : "";
  return script.startsWith("managed:") ? script.slice("managed:".length) : "";
}

function inferLuaScriptKeyFromBaseURL(baseURL: string): string {
  try {
    const parsed = new URL(baseURL);
    return parsed.hostname
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9.-]+/g, "-")
      .replace(/^-+|-+$/g, "");
  } catch {
    return "";
  }
}

function stringifyLuaConfigDraft(raw: string, scriptKey: string): string {
  const parsed = raw.trim() ? (JSON.parse(raw) as Record<string, unknown>) : {};
  if (scriptKey.trim() !== "") {
    parsed.script = `managed:${scriptKey.trim()}`;
  }
  return stringifyUsageConfig(parsed);
}

function buildLuaSkillPrompt(
  account: AccountRecord,
  scriptKey: string,
  usageConfigJSON: string,
): string {
  const accountContext = {
    account_id: account.id,
    account_name: account.account_name,
    provider_type: account.provider_type,
    auth_mode: account.auth_mode,
    base_url: account.base_url,
    source_icon: normalizeSourceIcon(account.source_icon),
    usage_driver: "lua",
    script_key: scriptKey,
    usage_config_json: usageConfigJSON,
  };

  return `# AI Gate Lua Usage Adapter Skill

你正在为 AI Gate 调试第三方 API 的 usage 采集脚本。目标是直接配置并验证账户 ${account.account_name}（ID ${account.id}） 的 Lua usage 驱动，不要改成内置 driver，也不要降级成手工统计。

## 账户上下文
${JSON.stringify(accountContext, null, 2)}

## 你的任务
1. 基于当前账户上下文，为该账户生成或修正 Lua usage 脚本。
2. 只处理 usage 采集，不要改请求代理逻辑。
3. 优先复用共享脚本键 \`${scriptKey || "请先填写共享脚本标识"}\`。
4. 若第三方 usage 接口需要额外字段，请直接写入 \`usage_config_json\`。

## Lua 入口规范
- 必须实现：\`function fetch_usage(ctx)\`
- 成功返回：
\`\`\`lua
return {
  ok = true,
  source = "remote",
  confidence = "high",
  limits = {
    balance = nil,
    quota_remaining = nil,
    rpm_remaining = nil,
    tpm_remaining = nil,
    daily_remaining = nil,
    monthly_remaining = nil,
    primary_used_percent = nil,
    secondary_used_percent = nil,
    primary_resets_at = nil,
    secondary_resets_at = nil
  },
  meta = {},
  payload = {}
}
\`\`\`
- 失败返回：
\`\`\`lua
return {
  ok = false,
  error = {
    kind = "auth",
    message = "session expired"
  }
}
\`\`\`

## ctx 可用字段
- \`ctx.account.id\`
- \`ctx.account.account_name\`
- \`ctx.account.provider_type\`
- \`ctx.account.auth_mode\`
- \`ctx.account.base_url\`
- \`ctx.credential.access_token\`
- \`ctx.credential.api_key\`
- \`ctx.credential.refresh_token\`
- \`ctx.credential.session\`
- \`ctx.credential.headers\`
- \`ctx.credential.metadata\`
- \`ctx.config\`：来自 \`usage_config_json\`

## host API
- \`ctx.host.http_get({ url = "...", headers = { Authorization = "Bearer xxx" } })\`
- \`ctx.host.json_decode(raw)\`
- \`ctx.host.json_encode(value)\`
- \`ctx.host.sleep_ms(ms)\`

## 配置方式
- 所有配置和验证都走 HTTP API，服务基地址固定为：\`http://127.0.0.1:6789\`
- 共享脚本列表：
  - \`GET http://127.0.0.1:6789/ai-router/api/accounts/usage-scripts\`
- 读取共享脚本：
  - \`GET http://127.0.0.1:6789/ai-router/api/accounts/usage-scripts/${scriptKey || "<script_key>"}\`
- 保存共享脚本：
  - \`PUT http://127.0.0.1:6789/ai-router/api/accounts/usage-scripts/${scriptKey || "<script_key>"}\`
  - body: \`{ "content": "<lua source>" }\`
- 账户引用方式：
  - \`usage_driver = "lua"\`
  - \`usage_config_json\` 中写 \`"script":"managed:${scriptKey || "<script_key>"}"\`
- 更新账户接口：
  - \`PUT http://127.0.0.1:6789/ai-router/api/accounts/${account.id}\`

## 调试步骤
1. 先读取当前共享脚本：
   - \`GET http://127.0.0.1:6789/ai-router/api/accounts/usage-scripts/${scriptKey || "<script_key>"}\`
2. 修改脚本后，不要先假设成功，必须调用 Lua 测试接口：
   - \`POST http://127.0.0.1:6789/ai-router/api/accounts/${account.id}/usage-lua-test\`
   - body:
\`\`\`json
{
  "usage_config_json": ${JSON.stringify(usageConfigJSON)},
  "script_content": "<当前 Lua 源码>"
}
\`\`\`
3. 只有当返回 \`ok=true\`，并且 \`content\` 中出现标准化字段（如 \`quota_remaining\` / \`balance\` / \`primary_used_percent\`），才算成功。
4. 如果失败，优先检查：
   - 鉴权头是否正确
   - 上游 usage 接口 URL 是否正确
   - JSON 解析是否匹配真实响应
   - 返回结构是否符合 AI Gate Lua schema
5. 测试通过后，再保存共享脚本并更新账户配置。

## 成功判定
- Lua 测试接口返回 \`ok=true\`
- \`content\` 为格式化后的标准化 usage JSON
- 更新账户接口返回成功
- 后续再调用 \`GET http://127.0.0.1:6789/ai-router/api/accounts/usage\` 时可以看到该账户 usage 已可正常刷新

## 执行要求
- 直接在当前仓库中完成修改
- 优先做最小改动
- 不要输出伪代码
- 配置、保存、验证都优先使用 HTTP API，不要依赖数据库直改或手工文件操作
`;
}

function sameAccountOrder(
  left: AccountRecord[],
  right: AccountRecord[],
): boolean {
  return (
    left.length === right.length &&
    left.every((item, index) => item.id === right[index]?.id)
  );
}

function sortAccountsByPriority(items: AccountRecord[]): AccountRecord[] {
  return [...items].sort((left, right) => {
    if (left.priority === right.priority) {
      return left.id - right.id;
    }
    return right.priority - left.priority;
  });
}

function mergeDisplayUsage(
  previous: AccountRecord,
  next: AccountRecord,
): AccountRecord {
  return {
    ...next,
    balance: previous.balance,
    quota_remaining: previous.quota_remaining,
    rpm_remaining: previous.rpm_remaining,
    tpm_remaining: previous.tpm_remaining,
    health_score: previous.health_score,
    recent_error_rate: previous.recent_error_rate,
    last_total_tokens: previous.last_total_tokens,
    last_input_tokens: previous.last_input_tokens,
    last_output_tokens: previous.last_output_tokens,
    model_context_window: previous.model_context_window,
    primary_used_percent: previous.primary_used_percent,
    secondary_used_percent: previous.secondary_used_percent,
    primary_resets_at: previous.primary_resets_at,
    secondary_resets_at: previous.secondary_resets_at,
    checked_at: previous.checked_at,
    stale: previous.stale,
    last_error: previous.last_error,
    ppchat_today_used_quota: previous.ppchat_today_used_quota,
    ppchat_today_added_quota: previous.ppchat_today_added_quota,
    ppchat_today_remaining_quota: previous.ppchat_today_remaining_quota,
  };
}

function formatResetTime(value: string | undefined, language: AppLanguage) {
  if (!value) {
    return "--";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "--";
  }
  const now = new Date();
  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate();
  if (sameDay) {
    return date.toLocaleTimeString(language, {
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    });
  }
  return `${date.toLocaleDateString(language, {
    month: "numeric",
    day: "numeric",
  })} ${date.toLocaleTimeString(language, {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  })}`;
}

function formatTomorrowMidnight(language: AppLanguage, now = new Date()) {
  const nextMidnight = new Date(now);
  nextMidnight.setHours(24, 0, 0, 0);
  return nextMidnight.toLocaleTimeString(language, {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

function formatCheckedAt(value: string | undefined, language: AppLanguage) {
  if (!value) {
    return language === "en-US" ? "No usage data yet" : "尚无 usage 数据";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return language === "en-US" ? "Unknown refresh time" : "刷新时间未知";
  }
  return date.toLocaleString(language, {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

function getUsageHealthState(
  record: Pick<AccountRecord, "checked_at" | "stale">,
): "ok" | "warning" | "danger" | "unknown" {
  if (!record.checked_at) {
    return "unknown";
  }
  if (record.stale) {
    return "danger";
  }
  const checkedAt = new Date(record.checked_at);
  if (Number.isNaN(checkedAt.getTime())) {
    return "unknown";
  }
  const ageMinutes = (Date.now() - checkedAt.getTime()) / 60000;
  if (ageMinutes <= 10) {
    return "ok";
  }
  if (ageMinutes <= 30) {
    return "warning";
  }
  return "danger";
}

function renderUsageHealthTooltip(
  record: Pick<AccountRecord, "checked_at" | "stale" | "last_error">,
  language: AppLanguage,
) {
  const checkedAt = formatCheckedAt(record.checked_at, language);
  const errorMessage = maskSensitiveErrorText(record.last_error || "");
  if (!record.checked_at) {
    return <span>{checkedAt}</span>;
  }
  return (
    <div className="account-usage-health-tooltip">
      <div>{checkedAt}</div>
      {record.stale ? (
        <div>{errorMessage || (language === "en-US" ? "Refresh failed" : "刷新失败")}</div>
      ) : (
        <div>{language === "en-US" ? "Refresh healthy" : "刷新正常"}</div>
      )}
    </div>
  );
}

function maskSensitiveErrorText(value: string): string {
  if (!value) {
    return "";
  }
  return value
    .replace(
      /\b(token(?:_key)?=)([^&\s"]+)/gi,
      (_match, prefix: string, secret: string) => `${prefix}${maskSecretValue(secret)}`,
    )
    .replace(/\bsk-[A-Za-z0-9_-]{8,}\b/g, (secret) => maskSecretValue(secret));
}

function maskSecretValue(secret: string): string {
  if (/^sk-/i.test(secret)) {
    return "sk-***";
  }
  return "***";
}

function formatInteger(language: AppLanguage, value: number): string {
  return new Intl.NumberFormat(language, { maximumFractionDigits: 0 }).format(
    value,
  );
}

function formatDeletedAccountMessage(
  language: AppLanguage,
  accountName: string,
): string {
  return language === "en-US"
    ? `Deleted account ${accountName}`
    : `已删除账户 ${accountName}`;
}

function getRoutingCooldownSeconds(record: AccountRecord): number | undefined {
  return (
    record.routing_cooldown_remaining_seconds ?? record.cooldown_remaining_seconds
  );
}

function getRoutingCooldownLabel(record: AccountRecord): string {
  return routingCooldownTextMap[record.routing_cooldown_reason ?? ""] ?? "路由冷却";
}

function formatActiveAccountMessage(
  language: AppLanguage,
  accountName: string,
): string {
  return language === "en-US"
    ? `Current account switched to ${accountName}`
    : `已切换当前使用中账户为 ${accountName}`;
}

function formatDeleteAccountTitle(
  language: AppLanguage,
  accountName: string,
): string {
  return language === "en-US"
    ? `Delete account "${accountName}"?`
    : `确认删除账户「${accountName}」吗？`;
}

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) {
    return 0;
  }
  return Math.min(Math.max(value, 0), 100);
}

function getRemainingTone(value: number): "ok" | "warning" | "danger" {
  if (value < 10) {
    return "danger";
  }
  if (value < 30) {
    return "warning";
  }
  return "ok";
}

function isOfficialAccount(record: AccountRecord): boolean {
  return (
    record.auth_mode === "oauth" || record.auth_mode === "codex_local_import"
  );
}

function isPPChatAccount(record: AccountRecord): boolean {
  return (
    normalizeSourceIcon(record.source_icon) === "ppchat" ||
    /ppchat\.vip/i.test(record.base_url)
  );
}

function buildGenericUsageWindows(
  record: AccountRecord,
  language: AppLanguage,
) {
  const windows: Array<{
    label: string;
    remainingPercent: number;
    resetLabel: string;
  }> = [];

  if (
    record.primary_used_percent > 0 ||
    Boolean(record.primary_resets_at)
  ) {
    windows.push({
      label: "P1",
      remainingPercent: clampPercent(100 - record.primary_used_percent),
      resetLabel: formatResetTime(record.primary_resets_at, language),
    });
  }

  if (
    record.secondary_used_percent > 0 ||
    Boolean(record.secondary_resets_at)
  ) {
    windows.push({
      label: "P2",
      remainingPercent: clampPercent(100 - record.secondary_used_percent),
      resetLabel: formatResetTime(record.secondary_resets_at, language),
    });
  }

  return windows;
}

type AccountsPageProps = {
  language?: AppLanguage;
  t?: Translator;
  syncToken?: number;
  addModalMode?: AddModalMode;
  onAddModalModeConsumed?: () => void;
  showAddButton?: boolean;
};

type AccountCardRenderOptions = {
  className?: string;
  actionsDisabled?: boolean;
  cardRef?: (node: HTMLDivElement | null) => void;
  handleAttributes?: Record<string, unknown>;
  handleListeners?: Record<string, unknown>;
  setHandleRef?: (node: HTMLButtonElement | null) => void;
  style?: CSSProperties;
};

type SortableAccountCardProps = {
  id: number;
  record: AccountRecord;
  renderCard: (
    record: AccountRecord,
    options?: AccountCardRenderOptions,
  ) => ReactNode;
};

function SortableAccountCard({
  id,
  record,
  renderCard,
}: SortableAccountCardProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id });

  const style: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return renderCard(record, {
    cardRef: setNodeRef,
    className: isDragging ? "account-card-item-placeholder" : undefined,
    handleAttributes: attributes as Record<string, unknown>,
    handleListeners: listeners as Record<string, unknown>,
    setHandleRef: setActivatorNodeRef,
    style,
  });
}

export function AccountsPage({
  language = "zh-CN",
  t = (value) => value,
  syncToken = 0,
  addModalMode: externalAddModalMode,
  onAddModalModeConsumed,
  showAddButton = true,
}: AccountsPageProps) {
  const [messageApi, contextHolder] = message.useMessage();
  const { modal } = AntApp.useApp();
  const [accounts, setAccounts] = useState<AccountRecord[]>([]);
  const [internalAddModalMode, setInternalAddModalMode] =
    useState<AddModalMode>(null);
  const [editingAccount, setEditingAccount] = useState<AccountRecord | null>(
    null,
  );
  const [detailAccount, setDetailAccount] = useState<AccountRecord | null>(
    null,
  );
  const [sharedImportError, setSharedImportError] = useState<string>("");
  const [testResult, setTestResult] = useState<AccountTestResult | null>(null);
  const [luaTestResult, setLuaTestResult] = useState<AccountTestResult | null>(
    null,
  );
  const [luaScriptKey, setLuaScriptKey] = useState("");
  const [luaScriptContent, setLuaScriptContent] = useState("");
  const [luaConfigDraft, setLuaConfigDraft] = useState("");
  const [availableLuaScripts, setAvailableLuaScripts] = useState<string[]>([]);
  const [showAdvancedLuaConfig, setShowAdvancedLuaConfig] = useState(false);
  const [luaScriptLoading, setLuaScriptLoading] = useState(false);
  const [luaTesting, setLuaTesting] = useState(false);
  const [detailLogsLoading, setDetailLogsLoading] = useState(false);
  const [detailLogs, setDetailLogs] = useState<
    PPChatTokenLogsPayload["data"] | null
  >(null);
  const [draggingAccountID, setDraggingAccountID] = useState<number | null>(
    null,
  );
  const [visibleActionAccountID, setVisibleActionAccountID] = useState<
    number | null
  >(null);

  const [thirdPartyForm] = Form.useForm();
  const [officialForm] = Form.useForm();
  const [sharedImportForm] = Form.useForm();
  const [editForm] = Form.useForm();
  const [testForm] = Form.useForm();
  const editUsageDriver = Form.useWatch("usage_driver", editForm);
  const editBaseURL = Form.useWatch("base_url", editForm);
  const accountsRef = useRef<AccountRecord[]>([]);
  const dragSnapshotRef = useRef<AccountRecord[] | null>(null);
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 4 },
    }),
    useSensor(MouseSensor, {
      activationConstraint: { distance: 4 },
    }),
  );

  function setAccountsState(
    next: AccountRecord[] | ((items: AccountRecord[]) => AccountRecord[]),
    options?: { preserveOrder?: boolean },
  ) {
    setAccounts((items) => {
      const resolved = typeof next === "function" ? next(items) : next;
      const ordered = options?.preserveOrder
        ? resolved
        : sortAccountsByPriority(resolved);
      accountsRef.current = ordered;
      return ordered;
    });
  }

  useEffect(() => {
    void refreshAll();
  }, [syncToken]);

  useEffect(() => {
    if (!externalAddModalMode) {
      return;
    }
    setInternalAddModalMode(externalAddModalMode);
    onAddModalModeConsumed?.();
  }, [externalAddModalMode, onAddModalModeConsumed]);

  useEffect(() => {
    if (!detailAccount) {
      setDetailLogs(null);
      setDetailLogsLoading(false);
      return;
    }
    if (normalizeSourceIcon(detailAccount.source_icon) !== "ppchat") {
      setDetailLogs(null);
      setDetailLogsLoading(false);
      return;
    }
    let cancelled = false;
    setDetailLogsLoading(true);
    void fetchPPChatTokenLogs(detailAccount.id)
      .then((payload) => {
        if (cancelled) {
          return;
        }
        setDetailLogs(payload.data);
      })
      .catch(() => {
        if (!cancelled) {
          setDetailLogs(null);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setDetailLogsLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [detailAccount]);

  useEffect(() => {
    if (!editingAccount) {
      setLuaScriptKey("");
      setLuaScriptContent("");
      setLuaConfigDraft("");
      setAvailableLuaScripts([]);
      setShowAdvancedLuaConfig(false);
      setLuaScriptLoading(false);
      setLuaTestResult(null);
      return;
    }

    const config = parseUsageConfig(editingAccount.usage_config_json);
    const explicitScriptKey = getManagedLuaScriptKey(
      editingAccount.usage_config_json,
    );
    const inferredScriptKey = inferLuaScriptKeyFromBaseURL(
      editingAccount.base_url,
    );
    const nextScriptKey = explicitScriptKey || inferredScriptKey;
    setLuaScriptKey(nextScriptKey);
    setLuaConfigDraft(JSON.stringify(config, null, 2));
    setLuaScriptContent("");
    setLuaTestResult(null);
    setShowAdvancedLuaConfig(false);

    let cancelled = false;
    void listLuaUsageScripts()
      .then((response) => {
        if (!cancelled) {
          setAvailableLuaScripts(response.items);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setAvailableLuaScripts([]);
        }
      });

    if (!nextScriptKey) {
      setLuaScriptLoading(false);
      return () => {
        cancelled = true;
      };
    }

    setLuaScriptLoading(true);
    void getLuaUsageScript(nextScriptKey)
      .then((record) => {
        if (!cancelled) {
          setLuaScriptContent(record.content);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setLuaScriptContent("");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLuaScriptLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [editingAccount]);

  useEffect(() => {
    if (!editingAccount || editUsageDriver !== "lua") {
      return;
    }
    if (luaScriptKey.trim() !== "") {
      return;
    }
    const inferred = inferLuaScriptKeyFromBaseURL(
      editBaseURL || editingAccount.base_url,
    );
    if (inferred) {
      setLuaScriptKey(inferred);
    }
  }, [editBaseURL, editUsageDriver, editingAccount, luaScriptKey]);

  async function refreshAll() {
    const previousByID = new Map(
      accountsRef.current.map((item) => [item.id, item]),
    );
    const accountItems = sortAccountsByPriority(
      (await listAccounts()).map((item) => {
        const previous = previousByID.get(item.id);
        return previous ? mergeDisplayUsage(previous, item) : item;
      }),
    );
    accountsRef.current = accountItems;
    setAccounts(accountItems);
    void refreshUsage();
  }

  async function refreshUsage() {
    try {
      const usageItems = await listAccountUsage();
      const usageByAccount = new Map(
        usageItems.map((item) => [item.account_id, item]),
      );
      setAccountsState(
        (items) =>
          items.map((item) => {
            const usage = usageByAccount.get(item.id);
            if (!usage) {
              return item;
            }
            return {
              ...item,
              ...usage,
            };
          }),
        { preserveOrder: true },
      );
    } catch {
      // Keep base account list responsive even when usage endpoint is unavailable.
    }
  }

  async function handleCreateThirdParty(values: {
    account_name: string;
    base_url: string;
    credential_ref: string;
  }) {
    await createAccount({
      provider_type: "openai-compatible",
      account_name: values.account_name,
      source_icon: inferSourceIconByBaseURL(values.base_url || ""),
      auth_mode: "api_key",
      base_url: values.base_url,
      credential_ref: values.credential_ref,
      supports_responses: true,
    });
    try {
      await refreshAccountUsage();
    } catch {
      // Keep account creation successful even if immediate usage refresh fails.
    }
    setInternalAddModalMode(null);
    thirdPartyForm.resetFields();
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success(t("第三方账户已添加"));
  }

  async function handleCreateOfficial(values: { account_name: string }) {
    await importCurrentCodexAuth(values.account_name || "local-codex");
    try {
      await refreshAccountUsage();
    } catch {
      // Keep import successful even if immediate usage refresh fails.
    }
    officialForm.resetFields();
    setInternalAddModalMode(null);
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success(t("官方账户已导入"));
  }

  async function handleImportShared(values: { payload: string }) {
    setSharedImportError("");
    try {
      await importSharedAccount(values.payload);
      try {
        await refreshAccountUsage();
      } catch {
        // Keep import successful even if immediate usage refresh fails.
      }
      sharedImportForm.resetFields();
      setInternalAddModalMode(null);
      await refreshAll();
      void refreshDesktopTrayState();
      void messageApi.success(t("导入成功"));
    } catch (error) {
      setSharedImportError(
        error instanceof Error ? t(error.message) : t("分享内容无效"),
      );
    }
  }

  function openEditModal(account: AccountRecord) {
    setEditingAccount(account);
    setTestResult(null);
    setLuaTestResult(null);
    editForm.setFieldsValue({
      account_name: account.account_name,
      source_icon: normalizeSourceIcon(account.source_icon),
      base_url: account.base_url,
      credential_ref: "",
      usage_driver: account.usage_driver === "lua" ? "lua" : "",
    });
    testForm.setFieldsValue({
      model: getDefaultTestModel(account),
      input: "ping",
    });
  }

  async function handleEdit(values: {
    account_name: string;
    source_icon: SourceIcon;
    base_url: string;
    credential_ref?: string;
    usage_driver?: string;
  }) {
    if (!editingAccount) {
      return;
    }
    let usageDriver: string | undefined;
    let usageConfigJSON: string | undefined;
    if (values.usage_driver === "lua") {
      if (luaScriptKey.trim() === "") {
        throw new Error(t("请填写共享脚本标识"));
      }
      if (luaScriptContent.trim() === "") {
        throw new Error(t("请填写 Lua 脚本"));
      }
      usageConfigJSON = stringifyLuaConfigDraft(
        luaConfigDraft || "{}",
        luaScriptKey,
      );
      await saveLuaUsageScript(luaScriptKey, luaScriptContent);
      usageDriver = "lua";
    } else {
      usageDriver = "";
      usageConfigJSON = "";
    }
    await updateAccount(editingAccount.id, {
      account_name: values.account_name,
      source_icon: normalizeSourceIcon(values.source_icon),
      base_url: values.base_url,
      credential_ref: values.credential_ref || undefined,
      usage_driver: usageDriver,
      usage_config_json: usageConfigJSON,
      supports_responses: true,
    });
    setEditingAccount(null);
    editForm.resetFields();
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success(t("账户已更新"));
  }

  async function handleDelete(account: AccountRecord) {
    await deleteAccount(account.id);
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success(
      formatDeletedAccountMessage(language, account.account_name),
    );
  }

  async function handleTest(values: { model: string; input: string }) {
    if (!editingAccount) {
      return;
    }
    const result = await testAccount(editingAccount.id, values);
    setTestResult(result);
  }

  async function handleCopyLuaSkill() {
    if (!editingAccount) {
      return;
    }
    const usageConfigJSON = (() => {
      try {
        return stringifyLuaConfigDraft(luaConfigDraft || "{}", luaScriptKey);
      } catch {
        return editingAccount.usage_config_json || "{}";
      }
    })();
    await writeClipboardText(
      buildLuaSkillPrompt(editingAccount, luaScriptKey.trim(), usageConfigJSON),
    );
    void messageApi.success(t("已复制 AI Skill"));
  }

  async function handleLuaTest() {
    if (!editingAccount) {
      return;
    }
    if ((editUsageDriver || "") !== "lua") {
      setLuaTestResult({
        ok: false,
        message: t("请先将用量驱动切换为 Lua"),
      });
      return;
    }
    let usageConfigJSON = "";
    try {
      usageConfigJSON = stringifyLuaConfigDraft(
        luaConfigDraft || "{}",
        luaScriptKey,
      );
    } catch (error) {
      setLuaTestResult({
        ok: false,
        message: t("Lua 配置无效"),
        details:
          error instanceof Error
            ? error.message
            : t("usage_config_json 不是合法 JSON"),
      });
      return;
    }
    setLuaTesting(true);
    try {
      const result = await testLuaUsage(editingAccount.id, {
        usage_config_json: usageConfigJSON,
        script_content: luaScriptContent,
      });
      setLuaTestResult(result);
    } finally {
      setLuaTesting(false);
    }
  }

  async function loadLuaScriptByKey(key: string) {
    const trimmed = key.trim();
    setLuaScriptKey(trimmed);
    if (trimmed === "") {
      setLuaScriptContent("");
      return;
    }
    setLuaScriptLoading(true);
    try {
      const record = await getLuaUsageScript(trimmed);
      setLuaScriptContent(record.content);
    } catch {
      setLuaScriptContent("");
    } finally {
      setLuaScriptLoading(false);
    }
  }

  async function handleSelectLuaScript(nextKey: string) {
    await loadLuaScriptByKey(nextKey);
  }

  async function handleCreateLuaScript() {
    const fallbackBaseURL =
      String(editForm.getFieldValue("base_url") || "") ||
      editingAccount?.base_url ||
      "";
    const nextKey = inferLuaScriptKeyFromBaseURL(fallbackBaseURL);
    if (!nextKey) {
      void messageApi.warning(t("当前接口地址无法自动生成脚本标识"));
      return;
    }
    await loadLuaScriptByKey(nextKey);
    void messageApi.success(t("已切换到当前供应商脚本"));
  }

  async function handleSetActive(account: AccountRecord) {
    if (account.is_active) {
      return;
    }
    const previous = [...accounts];
    setAccountsState((items) =>
      items.map((item) => ({
        ...item,
        is_active: item.id === account.id,
      })),
    );
    try {
      await updateAccount(account.id, { is_active: true });
      void refreshDesktopTrayState();
      void messageApi.success(
        formatActiveAccountMessage(language, account.account_name),
      );
    } catch (error) {
      setAccountsState(previous);
      void messageApi.error(
        error instanceof Error ? t(error.message) : t("切换激活账户失败"),
      );
    }
  }

  async function handleCopyAccount(record: AccountRecord) {
    try {
      await duplicateAccount(record.id);
      await refreshAll();
      void refreshDesktopTrayState();
      void messageApi.success(t("已复制账户"));
    } catch (error) {
      void messageApi.warning(
        error instanceof Error ? t(error.message) : t("复制失败，请稍后重试"),
      );
    }
  }

  async function handleShareAccount(record: AccountRecord) {
    await modal.confirm({
      title: t("分享账户"),
      content: t("即将复制可导入的账户信息，请注意保管，不要泄露。"),
      okText: t("确认分享"),
      cancelText: t("取消"),
      centered: true,
      onOk: async () => {
        const response = await shareAccount(record.id);
        await writeClipboardText(response.payload);
        void messageApi.success(t("已复制账户分享信息"));
      },
    });
  }

  function showActionTray(accountID: number) {
    setVisibleActionAccountID(accountID);
  }

  function hideActionTray(accountID: number) {
    setVisibleActionAccountID((current) =>
      current === accountID ? null : current,
    );
  }

  function handleCardFocus(accountID: number) {
    showActionTray(accountID);
  }

  function handleCardBlur(
    accountID: number,
    event: ReactFocusEvent<HTMLDivElement>,
  ) {
    const nextTarget = event.relatedTarget;
    if (nextTarget instanceof Node && event.currentTarget.contains(nextTarget)) {
      return;
    }
    hideActionTray(accountID);
  }

  function handleCardMouseEnter(accountID: number) {
    showActionTray(accountID);
  }

  function handleCardMouseLeave(
    accountID: number,
    event: ReactMouseEvent<HTMLDivElement>,
  ) {
    if (
      typeof document !== "undefined" &&
      event.currentTarget.contains(document.activeElement)
    ) {
      return;
    }
    hideActionTray(accountID);
  }

  function handleDragStart(event: DragStartEvent) {
    const activeID = Number(event.active.id);
    dragSnapshotRef.current = accountsRef.current;
    setDraggingAccountID(activeID);
  }

  function handleDragOver(event: DragOverEvent) {
    const overID = event.over ? Number(event.over.id) : null;
    if (overID === null) {
      return;
    }

    setAccountsState(
      (items) => {
        const activeIndex = items.findIndex(
          (item) => item.id === Number(event.active.id),
        );
        const overIndex = items.findIndex((item) => item.id === overID);
        if (activeIndex < 0 || overIndex < 0 || activeIndex === overIndex) {
          return items;
        }
        return arrayMove(items, activeIndex, overIndex);
      },
      { preserveOrder: true },
    );
  }

  async function finishDragSort() {
    const snapshot = dragSnapshotRef.current;
    const current = accountsRef.current;
    dragSnapshotRef.current = null;
    setDraggingAccountID(null);

    if (!snapshot || sameAccountOrder(snapshot, current)) {
      return;
    }

    try {
      await persistAccountPriority(current);
    } catch {
      setAccountsState(snapshot);
      void messageApi.warning(
        t("排序已更新到界面，但保存顺序失败，请稍后重试"),
      );
    }
  }

  async function handleDragEnd(event: DragEndEvent) {
    if (!event.over) {
      if (dragSnapshotRef.current) {
        setAccountsState(dragSnapshotRef.current);
      }
      dragSnapshotRef.current = null;
      setDraggingAccountID(null);
      return;
    }
    await finishDragSort();
  }

  function handleDragCancel() {
    if (dragSnapshotRef.current) {
      setAccountsState(dragSnapshotRef.current);
    }
    dragSnapshotRef.current = null;
    setDraggingAccountID(null);
  }

  async function persistAccountPriority(items: AccountRecord[]) {
    for (let index = 0; index < items.length; index += 1) {
      const item = items[index];
      const priority = items.length - index;
      let attempt = 0;
      let saved = false;
      while (attempt < 3 && !saved) {
        try {
          await updateAccount(item.id, { priority });
          saved = true;
        } catch (error) {
          attempt += 1;
          if (attempt >= 3) {
            throw error;
          }
          await sleep(120 * attempt);
        }
      }
    }
  }

  const detailLogMaxTokens = useMemo(() => {
    if (!detailLogs?.logs?.length) {
      return 0;
    }
    return Math.max(
      ...detailLogs.logs.map(
        (log) => log.prompt_tokens + log.completion_tokens,
      ),
      1,
    );
  }, [detailLogs]);

  const ppchatQuotaSummary = useMemo(() => {
    const info = detailLogs?.token_info;
    if (!info) {
      return null;
    }
    const added = Math.max(info.today_added_quota ?? 0, 0);
    const used = Math.max(info.today_used_quota ?? 0, 0);
    const remain = info.remain_quota_display ?? 0;
    const remainingVisible = Math.max(remain, 0);
    const total = Math.max(added, used + remainingVisible, 1);
    const overflow = remain < 0 ? Math.abs(remain) : Math.max(used - total, 0);

    return {
      added,
      used,
      remain,
      total,
      overflow,
      usageCount: info.today_usage_count ?? 0,
      progressPercent: Math.min((used / total) * 100, 100),
    };
  }, [detailLogs]);

  const draggingAccount =
    draggingAccountID === null
      ? null
      : (accounts.find((item) => item.id === draggingAccountID) ??
        dragSnapshotRef.current?.find(
          (item) => item.id === draggingAccountID,
        ) ??
        null);

  function renderAccountCard(
    record: AccountRecord,
    options: AccountCardRenderOptions = {},
  ) {
    const actionsVisible = visibleActionAccountID === record.id;
    const sourceIcon = sourceIconMap[normalizeSourceIcon(record.source_icon)];
    const usageHealthState = getUsageHealthState(record);
    const usageWindows = isOfficialAccount(record)
      ? [
          {
            label: "5H",
            remainingPercent: clampPercent(100 - record.primary_used_percent),
            resetLabel: formatResetTime(record.primary_resets_at, language),
          },
          {
            label: "7D",
            remainingPercent: clampPercent(100 - record.secondary_used_percent),
            resetLabel: formatResetTime(record.secondary_resets_at, language),
          },
        ]
      : isPPChatAccount(record) && (record.ppchat_today_added_quota ?? 0) > 0
        ? [
          {
            label: "1D",
            remainingPercent: clampPercent(
                ((record.ppchat_today_remaining_quota ?? 0) /
                  Math.max(record.ppchat_today_added_quota ?? 0, 1)) *
                  100,
              ),
              resetLabel: formatTomorrowMidnight(language),
            },
          ]
        : buildGenericUsageWindows(record, language);

    return (
      <div
        ref={options.cardRef}
        className={`account-card-item ${record.is_active ? "active-account-card" : ""} ${options.className ?? ""}`.trim()}
        style={options.style}
        data-actions-visible={actionsVisible ? "true" : "false"}
        onFocus={options.actionsDisabled ? undefined : () => handleCardFocus(record.id)}
        onBlur={
          options.actionsDisabled
            ? undefined
            : (event) => handleCardBlur(record.id, event)
        }
        onMouseEnter={
          options.actionsDisabled ? undefined : () => handleCardMouseEnter(record.id)
        }
        onMouseLeave={
          options.actionsDisabled
            ? undefined
            : (event) => handleCardMouseLeave(record.id, event)
        }
      >
        <Card variant="borderless" className="account-card-surface">
          <div className="account-card-shell">
            <button
              type="button"
              ref={options.setHandleRef}
              className="account-drag-handle"
              aria-label={`${t("拖拽排序")}-${record.account_name}`}
              {...(options.handleAttributes as
                | ButtonHTMLAttributes<HTMLButtonElement>
                | undefined)}
              {...(options.handleListeners as
                | ButtonHTMLAttributes<HTMLButtonElement>
                | undefined)}
            >
              <HolderOutlined />
            </button>
            <div className="account-main">
              <Avatar
                src={sourceIcon.icon}
                size={36}
                shape="square"
                className="account-source-icon"
              />
              <div className="account-main-text">
                <div className="account-title-row">
                  <Text strong>{record.account_name}</Text>
                  <Tag color={statusColorMap[record.status] ?? "default"}>
                    {t(statusTextMap[record.status] ?? record.status)}
                  </Tag>
                  {getRoutingCooldownSeconds(record) ? (
                    <Tag color="gold">{t(getRoutingCooldownLabel(record))}</Tag>
                  ) : null}
                  {record.is_active ? (
                    <Tag color="green">{t("当前使用中")}</Tag>
                  ) : null}
                </div>
                <Text type="secondary" className="account-base-url">
                  <Tooltip
                    mouseEnterDelay={0.15}
                    placement="topLeft"
                    getPopupContainer={() => document.body}
                    styles={{
                      body: {
                        maxWidth: 320,
                      },
                    }}
                    title={renderUsageHealthTooltip(record, language)}
                  >
                    <span
                      className={`account-usage-health-dot is-${usageHealthState}`}
                      aria-label={`${record.account_name}-usage-health`}
                    />
                  </Tooltip>
                  {record.base_url || t("OpenAI 官方")}
                </Text>
              </div>
            </div>
            <div className="account-side-slot">
              <div
                className={`account-usage-mini ${usageWindows.length === 0 ? "account-usage-mini-empty" : ""} ${usageWindows.length === 1 ? "account-usage-mini-single" : ""}`.trim()}
              >
                {usageWindows.map((item) => (
                  <div className="account-usage-mini-row" key={item.label}>
                    <div className="account-usage-mini-head">
                      <span className="account-usage-mini-label">
                        {item.label}
                      </span>
                      <span className="account-usage-mini-reset">
                        {item.resetLabel || ""}
                      </span>
                      <span
                        className={`account-usage-mini-value is-${getRemainingTone(item.remainingPercent)}`}
                      >{`${Math.round(item.remainingPercent)}%`}</span>
                    </div>
                    <div className="account-usage-mini-meter">
                      <div
                        className="account-usage-mini-track"
                        aria-label={`${record.account_name}-${item.label}`}
                      >
                        <div
                          className={`account-usage-mini-fill is-${getRemainingTone(item.remainingPercent)}`}
                          style={{ width: `${item.remainingPercent}%` }}
                        />
                      </div>
                    </div>
                  </div>
                ))}
              </div>
              <div className="account-actions">
                <Button
                  type="primary"
                  className="account-enable-button"
                  aria-label={`${t("设为激活")}-${record.account_name}`}
                  icon={<CheckCircleOutlined />}
                  disabled={record.is_active || options.actionsDisabled}
                  onClick={() => void handleSetActive(record)}
                >
                  {t("启用")}
                </Button>
                <Button
                  type="text"
                  className="account-action-button"
                  aria-label={`${t("编辑")}-${record.account_name}`}
                  icon={<EditOutlined />}
                  disabled={options.actionsDisabled}
                  onClick={() => openEditModal(record)}
                />
                <Button
                  type="text"
                  className="account-action-button"
                  aria-label={`${t("复制")}-${record.account_name}`}
                  icon={<CopyOutlined />}
                  disabled={options.actionsDisabled}
                  onClick={() => void handleCopyAccount(record)}
                />
                <Button
                  type="text"
                  className="account-action-button"
                  aria-label={`${t("分享")}-${record.account_name}`}
                  icon={<ExportOutlined />}
                  disabled={options.actionsDisabled}
                  onClick={() => void handleShareAccount(record)}
                />
                <Button
                  type="text"
                  className="account-action-button"
                  aria-label={`${t("详情")}-${record.account_name}`}
                  icon={<InfoCircleOutlined />}
                  disabled={options.actionsDisabled}
                  onClick={() => setDetailAccount(record)}
                />
                <Button
                  type="text"
                  danger
                  className="account-action-button"
                  aria-label={`${t("删除")}-${record.account_name}`}
                  icon={<DeleteOutlined />}
                  disabled={options.actionsDisabled}
                  onClick={() =>
                    void modal.confirm({
                      title: formatDeleteAccountTitle(
                        language,
                        record.account_name,
                      ),
                      okText: t("删除"),
                      cancelText: t("取消"),
                      centered: true,
                      okButtonProps: { danger: true },
                      onOk: () => handleDelete(record),
                    })
                  }
                />
              </div>
            </div>
          </div>
        </Card>
      </div>
    );
  }

  return (
    <div className="dashboard-page">
      {contextHolder}
      {showAddButton ? (
        <div className="dashboard-header">
          <div>
            <Title level={2} style={{ marginBottom: 8 }}>
              {t("账户列表")}
            </Title>
            <Text type="secondary">
              {t("主表仅展示核心状态，详细信息请通过详情查看。")}
            </Text>
          </div>
          <Dropdown
            menu={{
              items: [
                { key: "official", label: t("官方账户") },
                { key: "third_party", label: t("第三方账户") },
                { key: "shared_import", label: t("导入账户") },
              ],
              onClick: ({ key }) => {
                setSharedImportError("");
                setInternalAddModalMode(key as AddModalMode);
              },
            }}
            trigger={["click"]}
          >
            <Button type="primary" icon={<PlusOutlined />}>
              {t("添加账户")}
            </Button>
          </Dropdown>
        </div>
      ) : null}

      {accounts.length === 0 ? (
        <Card className="accounts-card" variant="borderless">
          <div className="accounts-empty">
            <Empty description={t("暂无账户")} />
          </div>
        </Card>
      ) : (
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragStart={handleDragStart}
          onDragOver={handleDragOver}
          onDragEnd={(event) => void handleDragEnd(event)}
          onDragCancel={handleDragCancel}
        >
          <SortableContext
            items={accounts.map((record) => record.id)}
            strategy={verticalListSortingStrategy}
          >
            <div className="account-cards">
              {accounts.map((record) => (
                <SortableAccountCard
                  key={record.id}
                  id={record.id}
                  record={record}
                  renderCard={renderAccountCard}
                />
              ))}
            </div>
          </SortableContext>
          <DragOverlay>
            {draggingAccount ? (
              <div className="account-drag-overlay">
                {renderAccountCard(draggingAccount, { actionsDisabled: true })}
              </div>
            ) : null}
          </DragOverlay>
        </DndContext>
      )}

      <Modal
        open={!!detailAccount}
        title={t("账户详情")}
        onCancel={() => setDetailAccount(null)}
        footer={null}
        destroyOnHidden
        centered
        width={880}
      >
        {detailAccount ? (
          normalizeSourceIcon(detailAccount.source_icon) === "ppchat" ? (
            <div className="account-detail-layout">
              {detailLogsLoading ? (
                <Card
                  variant="borderless"
                  className="account-detail-chart-card"
                >
                  <Skeleton active paragraph={{ rows: 8 }} />
                </Card>
              ) : (
                <div className="ppchat-metrics-grid">
                  <Card variant="borderless" className="ppchat-progress-card">
                    <div className="ppchat-stat-title">{t("今日配额进度")}</div>
                    <div className="ppchat-progress-head">
                      <span
                        className={`ppchat-progress-primary ${ppchatQuotaSummary?.overflow ? "is-danger" : ""}`}
                      >
                        {ppchatQuotaSummary
                          ? `${ppchatQuotaSummary.overflow ? t("超出") : t("剩余")} ${formatInteger(language, ppchatQuotaSummary.overflow || ppchatQuotaSummary.remain)}`
                          : "--"}
                      </span>
                      <span
                        className={`ppchat-progress-secondary ${ppchatQuotaSummary?.overflow ? "is-danger" : ""}`}
                      >
                        {ppchatQuotaSummary
                          ? `${t("已用")} ${formatInteger(language, ppchatQuotaSummary.used)} / ${t("新增")} ${formatInteger(language, ppchatQuotaSummary.added)}`
                          : "--"}
                      </span>
                    </div>
                    <div
                      className="ppchat-progress-bar-shell"
                      aria-label={t("今日配额进度")}
                    >
                      <div
                        className={`ppchat-progress-bar ${ppchatQuotaSummary?.overflow ? "is-danger" : ""}`}
                        style={{
                          width: `${ppchatQuotaSummary?.progressPercent ?? 0}%`,
                        }}
                      />
                    </div>
                  </Card>
                  <Card
                    variant="borderless"
                    className="ppchat-metric-card ppchat-metric-card-compact"
                  >
                    <Statistic
                      title={
                        <span className="ppchat-stat-title">
                          {t("当天增加配额")}
                        </span>
                      }
                      value={
                        ppchatQuotaSummary
                          ? formatInteger(language, ppchatQuotaSummary.added)
                          : "--"
                      }
                      valueRender={(node) => (
                        <span className="ppchat-stat-value">{node}</span>
                      )}
                    />
                  </Card>
                  <Card
                    variant="borderless"
                    className="ppchat-metric-card ppchat-metric-card-compact"
                  >
                    <Statistic
                      title={
                        <span className="ppchat-stat-title">
                          {t("今日已用次数")}
                        </span>
                      }
                      value={
                        ppchatQuotaSummary
                          ? formatInteger(
                              language,
                              ppchatQuotaSummary.usageCount,
                            )
                          : "--"
                      }
                      valueRender={(node) => (
                        <span className="ppchat-stat-value">{node}</span>
                      )}
                    />
                  </Card>
                </div>
              )}
              <Card
                variant="borderless"
                className="account-detail-chart-card"
                title={t("PPChat Token 日志")}
                extra={<BarChartOutlined />}
              >
                {detailLogsLoading ? (
                  <Skeleton active paragraph={{ rows: 5 }} />
                ) : detailLogs?.logs?.length ? (
                  <div className="token-log-list">
                    {detailLogs.logs.slice(0, 8).map((log, index) => {
                      const total = log.prompt_tokens + log.completion_tokens;
                      const width =
                        detailLogMaxTokens > 0
                          ? Math.max((total / detailLogMaxTokens) * 100, 6)
                          : 0;
                      return (
                        <div
                          className="token-log-row"
                          key={`${log.created_at}-${index}`}
                        >
                          <div className="token-log-meta">
                            <span>{log.model_name}</span>
                            <span>{log.created_time}</span>
                          </div>
                          <div className="token-log-bar-bg">
                            <div
                              className="token-log-bar"
                              style={{ width: `${width}%` }}
                            />
                          </div>
                          <div className="token-log-values">
                            <span>Prompt {log.prompt_tokens}</span>
                            <span>Completion {log.completion_tokens}</span>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <Empty
                    description={t("暂无 PPChat 日志数据")}
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                  />
                )}
              </Card>
            </div>
          ) : (
            <div className="account-detail-layout">
              <div className="account-detail-stats">
                <Card variant="borderless">
                  <Statistic
                    title={t("额度余额")}
                    value={Math.round(detailAccount.quota_remaining)}
                  />
                </Card>
                <Card variant="borderless">
                  <Statistic
                    title={t("健康分")}
                    value={detailAccount.health_score.toFixed(2)}
                  />
                </Card>
                <Card variant="borderless">
                  <Statistic
                    title={t("最近 Token")}
                    value={Math.round(detailAccount.last_total_tokens)}
                  />
                </Card>
                <Card variant="borderless">
                  <Statistic
                    title={t("错误率")}
                    value={(detailAccount.recent_error_rate * 100).toFixed(1)}
                    suffix="%"
                  />
                </Card>
              </div>
              <Card variant="borderless" className="account-detail-meta">
                <Descriptions column={2} size="small">
                  <Descriptions.Item label={t("账户")}>
                    {detailAccount.account_name}
                  </Descriptions.Item>
                  <Descriptions.Item label={t("来源")}>
                    {
                      sourceIconMap[
                        normalizeSourceIcon(detailAccount.source_icon)
                      ].label
                    }
                  </Descriptions.Item>
                  <Descriptions.Item label={t("状态")}>
                    {t(
                      statusTextMap[detailAccount.status] ??
                        detailAccount.status,
                    )}
                  </Descriptions.Item>
                  <Descriptions.Item label={t("路由状态")}>
                    {getRoutingCooldownSeconds(detailAccount)
                      ? `${t(getRoutingCooldownLabel(detailAccount))} · ${Math.max(
                          Math.floor((getRoutingCooldownSeconds(detailAccount) ?? 0) / 60),
                          0,
                        )}m`
                      : t("正常")}
                  </Descriptions.Item>
                  <Descriptions.Item label={t("认证方式")}>
                    {t(
                      authModeTextMap[detailAccount.auth_mode] ??
                        detailAccount.auth_mode,
                    )}
                  </Descriptions.Item>
                  <Descriptions.Item label={t("接口地址")} span={2}>
                    {detailAccount.base_url || t("OpenAI 官方")}
                  </Descriptions.Item>
                  <Descriptions.Item label={t("5 小时剩余")}>
                    {(100 - detailAccount.primary_used_percent).toFixed(0)}% ·{" "}
                    {formatResetTime(detailAccount.primary_resets_at, language)}
                  </Descriptions.Item>
                  <Descriptions.Item label={t("1 周剩余")}>
                    {(100 - detailAccount.secondary_used_percent).toFixed(0)}% ·{" "}
                    {formatResetTime(detailAccount.secondary_resets_at, language)}
                  </Descriptions.Item>
                </Descriptions>
              </Card>
            </div>
          )
        ) : null}
      </Modal>

      <Modal
        open={internalAddModalMode === "third_party"}
        title={t("添加第三方账户")}
        onCancel={() => setInternalAddModalMode(null)}
        footer={null}
        destroyOnHidden
        centered
      >
        <Form
          form={thirdPartyForm}
          layout="vertical"
          initialValues={{ base_url: defaultBaseURL }}
          onFinish={(values) => void handleCreateThirdParty(values)}
        >
          <Form.Item
            label={t("账户名称")}
            name="account_name"
            rules={[{ required: true, message: t("请输入账户名称") }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            label={t("接口地址")}
            name="base_url"
            rules={[{ required: true, message: t("请输入接口地址") }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            label="API Key"
            name="credential_ref"
            rules={[{ required: true, message: t("请输入 API Key") }]}
          >
            <Input.Password />
          </Form.Item>
          <div className="modal-footer">
            <Button onClick={() => setInternalAddModalMode(null)}>
              {t("取消")}
            </Button>
            <Button type="primary" htmlType="submit">
              {t("保存")}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        open={internalAddModalMode === "official"}
        title={t("添加官方账户")}
        onCancel={() => setInternalAddModalMode(null)}
        footer={null}
        destroyOnHidden
        centered
      >
        <Form
          form={officialForm}
          layout="vertical"
          onFinish={(values) => void handleCreateOfficial(values)}
        >
          <Form.Item
            label={t("账户名称")}
            name="account_name"
            initialValue="local-codex"
          >
            <Input />
          </Form.Item>
          <Text type="secondary">
            {language === "en-US" ? (
              <>
                The app reads <code>~/.codex/auth.json</code> directly from the
                current user directory.
              </>
            ) : (
              <>
                将直接读取当前用户目录下的 <code>~/.codex/auth.json</code>。
              </>
            )}
          </Text>
          <div className="modal-footer">
            <Button onClick={() => setInternalAddModalMode(null)}>
              {t("取消")}
            </Button>
            <Button type="primary" htmlType="submit">
              {t("导入")}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        open={internalAddModalMode === "shared_import"}
        title={t("导入账户")}
        onCancel={() => {
          setSharedImportError("");
          setInternalAddModalMode(null);
        }}
        footer={null}
        destroyOnHidden
        centered
      >
        <Form
          form={sharedImportForm}
          layout="vertical"
          onFinish={(values) => void handleImportShared(values)}
        >
          <Form.Item
            label={t("粘贴分享内容")}
            name="payload"
            rules={[{ required: true, message: t("请粘贴分享内容") }]}
          >
            <Input.TextArea rows={8} spellCheck={false} />
          </Form.Item>
          {sharedImportError ? (
            <Text type="danger">{sharedImportError}</Text>
          ) : null}
          <div className="modal-footer">
            <Button
              onClick={() => {
                setSharedImportError("");
                setInternalAddModalMode(null);
              }}
            >
              {t("取消")}
            </Button>
            <Button type="primary" htmlType="submit">
              {t("校验并导入")}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        open={!!editingAccount}
        title={t("编辑账户")}
        onCancel={() => setEditingAccount(null)}
        footer={null}
        destroyOnHidden
        centered
      >
        <Form
          form={editForm}
          layout="vertical"
          onFinish={(values) =>
            void handleEdit(values).catch((error) => {
              void messageApi.error(
                error instanceof Error ? error.message : t("账户更新失败"),
              );
            })
          }
        >
          <Form.Item
            label={t("账户名称")}
            name="account_name"
            rules={[{ required: true, message: t("请输入账户名称") }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            label={t("来源图标")}
            name="source_icon"
            rules={[{ required: true, message: t("请选择来源图标") }]}
          >
            <Select
              options={(Object.keys(sourceIconMap) as SourceIcon[]).map(
                (key) => ({
                  value: key,
                  label: (
                    <span className="source-option">
                      <Avatar
                        src={sourceIconMap[key].icon}
                        size={16}
                        shape="square"
                      />
                      <span>{sourceIconMap[key].label}</span>
                    </span>
                  ),
                }),
              )}
            />
          </Form.Item>
          <Form.Item
            label={t("接口地址")}
            name="base_url"
            rules={[{ required: true, message: t("请输入接口地址") }]}
          >
            <Input />
          </Form.Item>
          <Form.Item label={t("API Key / Token")} name="credential_ref">
            <Input.Password placeholder={t("留空表示不修改")} />
          </Form.Item>
          <Form.Item label={t("用量驱动")} name="usage_driver">
            <Select
              options={[
                { value: "", label: t("自动 / 内置") },
                { value: "lua", label: "Lua" },
              ]}
            />
          </Form.Item>
          {editUsageDriver === "lua" ? (
            <div className="lua-usage-panel">
              <div className="lua-usage-panel-header">
                <Text strong>{t("Lua Usage 配置")}</Text>
                <div className="lua-usage-panel-actions">
                  <Button
                    type="text"
                    onClick={() => setShowAdvancedLuaConfig((value) => !value)}
                  >
                    {showAdvancedLuaConfig ? t("隐藏高级配置") : t("高级配置")}
                  </Button>
                  <Button
                    type="text"
                    onClick={() =>
                      void handleCopyLuaSkill().catch((error) => {
                        void messageApi.error(
                          error instanceof Error
                            ? error.message
                            : t("复制 AI Skill 失败"),
                        );
                      })
                    }
                  >
                    {t("复制 AI Skill")}
                  </Button>
                </div>
              </div>
              <Form.Item label={t("共享脚本")} htmlFor="lua-script-key">
                <div className="lua-script-select-row">
                  <Select
                    id="lua-script-key"
                    value={luaScriptKey || undefined}
                    placeholder={t("按当前供应商自动匹配脚本")}
                    options={availableLuaScripts.map((item) => ({
                      value: item,
                      label: item,
                    }))}
                    onChange={(value) =>
                      void handleSelectLuaScript(String(value || ""))
                    }
                    showSearch
                    optionFilterProp="label"
                    allowClear
                    className="lua-script-select"
                  />
                  <Button onClick={() => void handleCreateLuaScript()}>
                    {t("新增脚本")}
                  </Button>
                </div>
                {luaScriptKey ? (
                  <Text type="secondary">
                    {t("当前脚本标识")}: {luaScriptKey}
                  </Text>
                ) : (
                  <Text type="secondary">
                    {t("将按接口地址主机名自动匹配共享脚本")}
                  </Text>
                )}
              </Form.Item>
              <Form.Item label={t("Lua 脚本")} htmlFor="lua-script-content">
                <Input.TextArea
                  id="lua-script-content"
                  rows={10}
                  value={luaScriptContent}
                  onChange={(event) => setLuaScriptContent(event.target.value)}
                  spellCheck={false}
                  placeholder="function fetch_usage(ctx) ... end"
                />
              </Form.Item>
              {showAdvancedLuaConfig ? (
                <Form.Item
                  label={t("Usage 配置 JSON")}
                  htmlFor="lua-config-draft"
                >
                  <Input.TextArea
                    id="lua-config-draft"
                    rows={5}
                    value={luaConfigDraft}
                    onChange={(event) => setLuaConfigDraft(event.target.value)}
                    spellCheck={false}
                  />
                </Form.Item>
              ) : null}
              {luaScriptLoading ? (
                <Text type="secondary">{t("正在加载共享脚本...")}</Text>
              ) : null}
              <div className="modal-footer">
                <Button
                  onClick={() => void handleLuaTest()}
                  loading={luaTesting}
                >
                  {t("测试 Lua 脚本")}
                </Button>
              </div>
            </div>
          ) : null}
          <div className="modal-footer">
            <Button onClick={() => setEditingAccount(null)}>{t("取消")}</Button>
            <Button type="primary" htmlType="submit">
              {t("保存")}
            </Button>
          </div>
        </Form>
        <div className="edit-test-panel">
          <Text strong>{t("连接测试")}</Text>
          <Form
            form={testForm}
            layout="vertical"
            initialValues={{ model: "gpt-5.4", input: "ping" }}
            onFinish={(values) => void handleTest(values)}
          >
            <Form.Item
              label={t("模型")}
              name="model"
              rules={[{ required: true, message: t("请选择模型") }]}
            >
              <Select
                options={[
                  { value: "gpt-5.4", label: "gpt-5.4" },
                  { value: "gpt-5.1-codex-max", label: "gpt-5.1-codex-max" },
                  { value: "gpt-5.2-codex", label: "gpt-5.2-codex" },
                  { value: "gpt-5", label: "gpt-5" },
                  { value: "gpt-4.1", label: "gpt-4.1" },
                ]}
              />
            </Form.Item>
            <Form.Item
              label={t("输入内容")}
              name="input"
              rules={[{ required: true, message: t("请输入测试内容") }]}
            >
              <Input.TextArea rows={3} />
            </Form.Item>
            <div className="modal-footer">
              <Button htmlType="submit">{t("测试")}</Button>
            </div>
          </Form>
        </div>
        {testResult ? (
          <div className="test-result-panel">
            <Tag color={testResult.ok ? "green" : "red"}>
              {testResult.message}
            </Tag>
            {testResult.details ? (
              <Text type="secondary">{testResult.details}</Text>
            ) : null}
            {testResult.content ? (
              <pre className="test-output">{testResult.content}</pre>
            ) : null}
          </div>
        ) : null}
        {luaTestResult ? (
          <div className="test-result-panel">
            <Tag color={luaTestResult.ok ? "green" : "red"}>
              {luaTestResult.message}
            </Tag>
            {luaTestResult.details ? (
              <Text type="secondary">{luaTestResult.details}</Text>
            ) : null}
            {luaTestResult.content ? (
              <pre className="test-output">{luaTestResult.content}</pre>
            ) : null}
          </div>
        ) : null}
      </Modal>
    </div>
  );
}

function getDefaultTestModel(account: AccountRecord): string {
  if (
    account.auth_mode === "codex_local_import" ||
    account.provider_type === "openai-official"
  ) {
    return "gpt-5.4";
  }
  return "gpt-5.4";
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms);
  });
}
