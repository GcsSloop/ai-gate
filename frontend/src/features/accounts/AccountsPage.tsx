import {
  ApiOutlined,
  CheckCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  ExportOutlined,
  HolderOutlined,
  InfoCircleOutlined,
  LockOutlined,
  UnlockOutlined,
  PlusOutlined,
} from "@ant-design/icons";
import {
  AutoComplete,
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
  Statistic,
  Switch,
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
  completeOfficialAuthDevice,
  deleteAccount,
  getAccountUpstreams,
  startOfficialAuth,
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
  updateAccountUpstreamLock,
  updateAccountUpstreamRoute,
  type AccountRecord,
  type AccountTestResult,
  type OfficialAuthSession,
  type ServerUpstreamAccount,
  type ServerUpstreams,
} from "../../lib/api";
import { writeClipboardText } from "../../lib/clipboard";
import { openExternalUrl, refreshDesktopTrayState } from "../../lib/desktop-shell";
import type { AppLanguage, Translator } from "../../lib/i18n";
import { isServerWebUI } from "../../lib/paths";
import sourceAIGateIcon from "../../assets/aigate_1024_1024.png";
import sourceClaudeCodeIcon from "../../assets/providers/claude-code.png";
import sourceOpenAIIcon from "../../assets/providers/openai.png";
import sourcePPChatIcon from "../../assets/providers/ppchat.png";

const { Title, Text } = Typography;

const defaultBaseURL = "https://code.ppchat.vip/v1";
type AddModalMode = "official" | "third_party" | "shared_import" | null;
type OfficialEntryMode = "oauth" | "local_import";

const statusTextMap: Record<string, string> = {
  active: "可用",
  cooldown: "冷却中",
  degraded: "降级",
  invalid: "失效",
  disabled: "已停用",
};

const routingCooldownTextMap: Record<string, string> = {
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

type SourceIcon = "openai" | "claude_code" | "ppchat" | "aigate";

const sourceIconMap: Record<SourceIcon, { label: string; icon: string }> = {
  openai: { label: "OpenAI", icon: sourceOpenAIIcon },
  claude_code: { label: "Claude Code", icon: sourceClaudeCodeIcon },
  ppchat: { label: "PPChat", icon: sourcePPChatIcon },
  aigate: { label: "AI Gate", icon: sourceAIGateIcon },
};

const selectableSourceIcons: SourceIcon[] = ["openai", "claude_code", "ppchat"];

const TEST_MODEL_SUGGESTIONS = [
  "gpt-5.5",
  "gpt-5.5-pro",
  "gpt-5.4",
  "gpt-5.3-codex",
  "gpt-5.2",
  "gpt-5.2-codex",
  "gpt-5.1",
  "gpt-5.1-codex-max",
  "gpt-5",
  "gpt-4.1",
];

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

function getKnownLuaDSLTemplate(account: Pick<AccountRecord, "base_url" | "source_icon">): string {
  const baseURL = account.base_url.trim().toLowerCase();
  if (/nodeseek\.in/i.test(baseURL)) {
    return `simple_usage({
  get = "/v1/usage",
  auth = "bearer",
  remaining = pick("remaining", "quota.remaining", "balance"),
  unit = pick("unit", "quota.unit", default("USD")),
  valid = pick("is_active", "isValid", default(true)),
  display = {
    summary = {
      label = "余额",
      value = function(payload)
        local remaining = payload.remaining or payload.balance
        if remaining == nil and type(payload.quota) == "table" then
          remaining = payload.quota.remaining
        end
        if type(remaining) == "number" then
          return "$" .. string.format("%.2f", remaining)
        end
        return "--"
      end
    },
    detail_stats = {
      { label = "余额", value = function(payload)
        local remaining = payload.remaining or payload.balance
        if remaining == nil and type(payload.quota) == "table" then
          remaining = payload.quota.remaining
        end
        if type(remaining) == "number" then
          return "$" .. string.format("%.2f", remaining)
        end
        return "--"
      end },
      { label = "状态", value = "可用" }
    },
    detail_items = {
      { label = "计费单位", value = pick("unit", "quota.unit", default("USD")) }
    }
  }
})
`;
  }
  if (normalizeSourceIcon(account.source_icon) === "ppchat" || /ppchat\.vip/i.test(baseURL)) {
    return `usage_adapter({
  get = "https://his.ppchat.vip/api/token-logs?page=1&page_size=1&token_key={{api_key}}",

  limits = {
    quota_remaining = pick("data.token_info.remain_quota_display")
  },

  meta = {
    today_used_quota = pick("data.token_info.today_used_quota"),
    today_added_quota = pick("data.token_info.today_added_quota"),
    unit = "quota"
  },

  display = {
    summary = {
      label = "余额",
      value = function(payload)
        local token = payload.data and payload.data.token_info or {}
        if type(token.remain_quota_display) == "number" then
          return string.format("%.0f", token.remain_quota_display)
        end
        return "--"
      end
    },
    detail_stats = {
      { label = "剩余配额", value = function(payload)
        local token = payload.data and payload.data.token_info or {}
        if type(token.remain_quota_display) == "number" then
          return string.format("%.0f", token.remain_quota_display)
        end
        return "--"
      end },
      { label = "当天已用", value = function(payload)
        local token = payload.data and payload.data.token_info or {}
        if type(token.today_used_quota) == "number" then
          return string.format("%.0f", token.today_used_quota)
        end
        return "--"
      end }
    },
    detail_items = {
      { label = "当天增加配额", value = function(payload)
        local token = payload.data and payload.data.token_info or {}
        if type(token.today_added_quota) == "number" then
          return string.format("%.0f", token.today_added_quota)
        end
        return "--"
      end }
    }
  }
})
`;
  }
  return `simple_usage({
  get = "/v1/usage",
  auth = "bearer",
  remaining = pick("remaining", "quota.remaining", "balance"),
  unit = pick("unit", "quota.unit", default("USD")),
  valid = pick("is_active", "isValid", default(true)),
  display = {
    summary = { label = "余额", value = function(payload)
      local remaining = payload.remaining or payload.balance
      if remaining == nil and type(payload.quota) == "table" then
        remaining = payload.quota.remaining
      end
      if type(remaining) == "number" then
        return "$" .. string.format("%.2f", remaining)
      end
      return "--"
    end }
  }
})
`;
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

## Lua DSL 规范（优先使用）
- 简单余额接口优先使用 \`simple_usage({...})\`：
\`\`\`lua
simple_usage({
  get = "/v1/usage",
  auth = "bearer",
  remaining = pick("remaining", "quota.remaining", "balance"),
  unit = pick("unit", "quota.unit", default("USD")),
  valid = pick("is_active", "isValid", default(true)),
  display = {
    summary = { label = "余额", value = function(payload)
      local remaining = payload.remaining or payload.balance
      if remaining == nil and type(payload.quota) == "table" then
        remaining = payload.quota.remaining
      end
      if type(remaining) == "number" then
        return "$" .. string.format("%.2f", remaining)
      end
      return "--"
    end },
    detail_stats = {
      { label = "余额", value = function(payload)
        local remaining = payload.remaining or payload.balance
        if remaining == nil and type(payload.quota) == "table" then
          remaining = payload.quota.remaining
        end
        if type(remaining) == "number" then
          return "$" .. string.format("%.2f", remaining)
        end
        return "--"
      end }
    },
    detail_items = {
      { label = "计费单位", value = pick("unit", "quota.unit", default("USD")) }
    }
  }
})
\`\`\`
- 需要自定义映射时使用 \`usage_adapter({...})\`：
\`\`\`lua
usage_adapter({
  request = {
    url = "{{base_url}}/v1/usage",
    method = "GET",
    headers = { Authorization = "Bearer {{api_key}}" }
  },
  extract = {
    remaining = pick("remaining", "quota.remaining", "balance"),
    unit = pick("unit", "quota.unit", default("USD"))
  },
  result = function(v)
    return {
      ok = true,
      source = "remote",
      confidence = "high",
      limits = { balance = v.remaining },
      meta = { unit = v.unit },
      display = {
        summary = { label = "余额", value = "$" .. string.format("%.2f", v.remaining) },
        detail_stats = {
          { label = "余额", value = "$" .. string.format("%.2f", v.remaining) }
        },
        detail_items = {
          { label = "计费单位", value = v.unit }
        }
      }
    }
  end
})
\`\`\`
- \`pick("a.b", "c", default(x))\` 会按顺序读取 JSON 字段路径。
- \`display.summary\` 控制账户列表右侧摘要；\`display.detail_stats\` 控制详情页上方卡片；\`display.detail_items\` 控制详情页用量条目。它们只影响显示，不影响路由判断。
- \`get = "/path"\` 会自动拼接 \`ctx.account.base_url\`；也可以直接写完整 URL。
- \`auth = "bearer"\` 会自动使用当前账户 API key/access token。

## 兼容旧入口
- 旧脚本仍然兼容：\`function fetch_usage(ctx)\`
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
  display = {
    summary = { label = "余额", value = "$61.96" },
    detail_stats = {
      { label = "余额", value = "$61.96" }
    },
    detail_items = {
      { label = "计费单位", value = "USD" }
    }
  },
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
    usage_display: previous.usage_display,
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
  record: Pick<
    AccountRecord,
    "checked_at" | "stale" | "usage_driver" | "usage_config_json"
  >,
): "ok" | "warning" | "danger" | "unknown" {
  const usageDriver = (record.usage_driver || "").trim().toLowerCase();
  const usageConfig = parseUsageConfig(record.usage_config_json);
  const luaScript =
    typeof usageConfig.script === "string" ? String(usageConfig.script) : "";
  const hasBuiltinUsageDriver = usageDriver.startsWith("builtin_");
  const hasLuaUsageDriver =
    usageDriver === "lua" && luaScript.trim() !== "";

  if (!hasBuiltinUsageDriver && !hasLuaUsageDriver) {
    return "unknown";
  }
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

function formatUsageAmount(language: AppLanguage, value: number): string {
  return new Intl.NumberFormat(language, {
    maximumFractionDigits: 2,
  }).format(value);
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

function formatAccountLockMessage(
  language: AppLanguage,
  accountName: string,
  locked: boolean,
): string {
  if (language === "en-US") {
    return locked
      ? `Locked account ${accountName}`
      : `Unlocked account ${accountName}`;
  }
  return locked ? `已锁定账户 ${accountName}` : `已解除锁定账户 ${accountName}`;
}

function renderAccountLockTooltip(language: AppLanguage) {
  return language === "en-US"
    ? "Locked account: automatic routing will not select it, but you can still enable it manually."
    : "账户已锁定：自动路由不会选择该账户，但仍可手动启用。";
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

function isPPChatUpstream(account: ServerUpstreamAccount): boolean {
  return (
    normalizeSourceIcon(account.source_icon) === "ppchat" ||
    /ppchat\.vip/i.test(account.base_url)
  );
}

function isRemoteUpstreamStatusSelectable(account: ServerUpstreamAccount): boolean {
  return account.status !== "disabled" && account.status !== "invalid";
}

function canManuallySelectRemoteUpstream(account: ServerUpstreamAccount): boolean {
  return isRemoteUpstreamStatusSelectable(account);
}

function isAIGateAccount(record: AccountRecord): boolean {
  const raw = (record.base_url || "").trim();
  if (raw === "") {
    return false;
  }
  try {
    const parsed = new URL(raw);
    return parsed.pathname.split("/").filter(Boolean).includes("ai-gate");
  } catch {
    return /(^|\/)ai-gate(\/|$)/i.test(raw);
  }
}

function getDisplaySourceIcon(record: AccountRecord): SourceIcon {
  return isAIGateAccount(record) ? "aigate" : normalizeSourceIcon(record.source_icon);
}

function isInteractiveTarget(
  target: EventTarget | null,
  root?: Element | null,
): boolean {
  if (!(target instanceof Element)) {
    return false;
  }
  const interactive = target.closest(
    "button,a,input,textarea,select,[role='button'],[data-no-row-toggle='true']",
  );
  return Boolean(interactive && interactive !== root);
}

function formatRemoteUpstreamUsage(
  account: ServerUpstreamAccount,
  language: AppLanguage,
): string {
  const display = account.usage_display?.summary;
  if (display?.label || display?.value) {
    return `${display.label ?? ""} ${display.value ?? ""}`.trim();
  }
  if (account.balance > 0) {
    return `${language === "en-US" ? "Balance" : "余额"} ${formatUsageAmount(language, account.balance)}`;
  }
  if (account.quota_remaining > 0) {
    return `${language === "en-US" ? "Quota" : "额度"} ${formatUsageAmount(language, account.quota_remaining)}`;
  }
  return language === "en-US" ? "Usage unknown" : "用量未知";
}

function buildRemoteUpstreamUsageWindow(
  account: ServerUpstreamAccount,
  language: AppLanguage,
) {
  const addedQuota = account.ppchat_today_added_quota ?? 0;
  if (isPPChatUpstream(account) && addedQuota > 0) {
    return {
      label: "1D",
      remainingPercent: clampPercent(
        ((account.ppchat_today_remaining_quota ?? 0) /
          Math.max(addedQuota, 1)) *
          100,
      ),
      resetLabel: formatTomorrowMidnight(language),
    };
  }
  return null;
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

function buildUsageAmount(record: AccountRecord, language: AppLanguage) {
  const summary = record.usage_display?.summary;
  if (summary?.label || summary?.value) {
    return {
      label: summary.label || (language === "en-US" ? "Usage" : "用量"),
      value: summary.value || "--",
    };
  }
  if (record.balance > 0) {
    return {
      label: language === "en-US" ? "Balance" : "余额",
      value: formatUsageAmount(language, record.balance),
    };
  }
  if (record.quota_remaining > 0) {
    return {
      label: language === "en-US" ? "Remaining quota" : "剩余配额",
      value: formatUsageAmount(language, record.quota_remaining),
    };
  }
  return null;
}

function buildDetailStats(record: AccountRecord, language: AppLanguage) {
  const customStats = record.usage_display?.detail_stats?.filter(
    (item) => item.label || item.value,
  );
  if (customStats?.length) {
    return customStats.map((item) => ({
      title: item.label || (language === "en-US" ? "Usage" : "用量"),
      value: item.value || "--",
    }));
  }
  return [
    {
      title: language === "en-US" ? "Quota balance" : "额度余额",
      value: formatInteger(
        language,
        Math.round(
          isPPChatAccount(record) && (record.ppchat_today_added_quota ?? 0) > 0
            ? (record.ppchat_today_remaining_quota ?? 0)
            : record.quota_remaining,
        ),
      ),
    },
    {
      title: language === "en-US" ? "Health score" : "健康分",
      value: record.health_score.toFixed(2),
    },
    {
      title: language === "en-US" ? "Recent tokens" : "最近 Token",
      value: formatInteger(language, Math.round(record.last_total_tokens)),
    },
    {
      title: language === "en-US" ? "Error rate" : "错误率",
      value: `${(record.recent_error_rate * 100).toFixed(1)}%`,
    },
  ];
}

function buildDetailUsageItems(record: AccountRecord, language: AppLanguage) {
  const customItems = record.usage_display?.detail_items?.filter(
    (item) => item.label || item.value,
  );
  if (customItems?.length) {
    return customItems.map((item) => ({
      label: item.label || (language === "en-US" ? "Usage" : "用量"),
      value: item.value || "--",
    }));
  }
  if (isPPChatAccount(record)) {
    return [
      {
        label: language === "en-US" ? "Remaining quota" : "剩余配额",
        value: `${formatInteger(language, record.ppchat_today_remaining_quota ?? 0)} · ${formatTomorrowMidnight(language)}`,
      },
      {
        label: language === "en-US" ? "Used today" : "当天已用配额",
        value: `${formatInteger(language, record.ppchat_today_used_quota ?? 0)} / ${language === "en-US" ? "Added today" : "当天增加配额"} ${formatInteger(language, record.ppchat_today_added_quota ?? 0)}`,
      },
    ];
  }
  return [
    {
      label: language === "en-US" ? "5-hour remaining" : "5 小时剩余",
      value: `${(100 - record.primary_used_percent).toFixed(0)}% · ${formatResetTime(record.primary_resets_at, language)}`,
    },
    {
      label: language === "en-US" ? "1-week remaining" : "1 周剩余",
      value: `${(100 - record.secondary_used_percent).toFixed(0)}% · ${formatResetTime(record.secondary_resets_at, language)}`,
    },
  ];
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

type RemoteUpstreamsState = {
  loading: boolean;
  data?: ServerUpstreams;
  error?: string;
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
  const [officialEntryMode, setOfficialEntryMode] =
    useState<OfficialEntryMode>("oauth");
  const [officialOAuthLaunching, setOfficialOAuthLaunching] = useState(false);
  const [officialImporting, setOfficialImporting] = useState(false);
  const [officialOAuthSession, setOfficialOAuthSession] =
    useState<OfficialAuthSession | null>(null);
  const officialOAuthPollingTimerRef = useRef<number | null>(null);
  const officialOAuthPollingInFlightRef = useRef(false);
  const [editingAccount, setEditingAccount] = useState<AccountRecord | null>(
    null,
  );
  const [testingAccount, setTestingAccount] = useState<AccountRecord | null>(
    null,
  );
  const [detailAccount, setDetailAccount] = useState<AccountRecord | null>(
    null,
  );
  const [sharedImportError, setSharedImportError] = useState<string>("");
  const [testResult, setTestResult] = useState<AccountTestResult | null>(null);
  const [testLoading, setTestLoading] = useState(false);
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
  const [draggingAccountID, setDraggingAccountID] = useState<number | null>(
    null,
  );
  const [visibleActionAccountID, setVisibleActionAccountID] = useState<
    number | null
  >(null);
  const [remoteUpstreamsByAccountID, setRemoteUpstreamsByAccountID] = useState<
    Record<number, RemoteUpstreamsState>
  >({});
  const [expandedRemoteAccountID, setExpandedRemoteAccountID] = useState<
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
  const serverWebUI = isServerWebUI();
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
    if (internalAddModalMode !== "official") {
      setOfficialEntryMode("oauth");
      setOfficialOAuthLaunching(false);
    }
  }, [internalAddModalMode]);

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
          setLuaScriptContent(getKnownLuaDSLTemplate(editingAccount));
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
    void refreshRemoteUpstreams(accountItems);
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

  async function refreshRemoteUpstreams(accountItems: AccountRecord[]) {
    const aiGateAccounts = accountItems.filter(isAIGateAccount);
    const aiGateIDs = new Set(aiGateAccounts.map((item) => item.id));
    setRemoteUpstreamsByAccountID((previous) => {
      const next: Record<number, RemoteUpstreamsState> = {};
      for (const account of aiGateAccounts) {
        next[account.id] = {
          loading: true,
          data: previous[account.id]?.data,
          error: undefined,
        };
      }
      return next;
    });
    setExpandedRemoteAccountID((current) =>
      current !== null && aiGateIDs.has(current) ? current : null,
    );
    await Promise.all(
      aiGateAccounts.map(async (account) => {
        try {
          const data = await getAccountUpstreams(account.id);
          setRemoteUpstreamsByAccountID((previous) => {
            if (!accountsRef.current.some((item) => item.id === account.id && isAIGateAccount(item))) {
              return previous;
            }
            return {
              ...previous,
              [account.id]: { loading: false, data },
            };
          });
        } catch (error) {
          const message =
            error instanceof Error ? error.message : t("无法读取上游状态");
          setRemoteUpstreamsByAccountID((previous) => {
            if (!accountsRef.current.some((item) => item.id === account.id && isAIGateAccount(item))) {
              return previous;
            }
            return {
              ...previous,
              [account.id]: { loading: false, error: message },
            };
          });
          setExpandedRemoteAccountID((current) =>
            current === account.id ? null : current,
          );
        }
      }),
    );
  }

  async function handleCreateThirdParty(values: {
    account_name: string;
    base_url: string;
    credential_ref: string;
    skip_tls_verify?: boolean;
  }) {
    await createAccount({
      provider_type: "openai-compatible",
      account_name: values.account_name,
      source_icon: inferSourceIconByBaseURL(values.base_url || ""),
      auth_mode: "api_key",
      base_url: values.base_url,
      credential_ref: values.credential_ref,
      supports_responses: true,
      skip_tls_verify: values.skip_tls_verify ?? false,
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

  async function handleCreateOfficial(values: { account_name?: string }) {
    setOfficialImporting(true);
    try {
      if (
        officialEntryMode === "oauth" &&
        (officialOAuthSession?.device_code || "").trim() !== "" &&
        (officialOAuthSession?.user_code || "").trim() !== ""
      ) {
        await completeOfficialAuthDevice({
          device_code: officialOAuthSession?.device_code || "",
          user_code: officialOAuthSession?.user_code || "",
        }, { allowPending: false });
      } else {
        await importCurrentCodexAuth(values.account_name || "local-codex");
      }
      await finishOfficialImportFlow(t("官方账户已导入"));
    } catch (error) {
      const message = error instanceof Error ? error.message : t("导入失败");
      void messageApi.error(message);
    } finally {
      setOfficialImporting(false);
    }
  }

  async function handleStartOfficialOAuth() {
    if (officialOAuthLaunching) {
      return;
    }
    setOfficialOAuthLaunching(true);
    try {
      let targetURL = "https://auth.openai.com/codex/device";
      const maxRetries = 3;
      let session: OfficialAuthSession | null = null;
      let lastError: unknown = null;
      for (let attempt = 1; attempt <= maxRetries; attempt += 1) {
        try {
          session = await startOfficialAuth();
          break;
        } catch (error) {
          lastError = error;
          if (attempt < maxRetries) {
            await new Promise<void>((resolve) => {
              window.setTimeout(resolve, 300);
            });
          }
        }
      }
      if (!session) {
        throw lastError instanceof Error ? lastError : new Error(t("启动 OAuth 失败"));
      }
      setOfficialOAuthSession(session);
      const candidate = (session.authorization_url || "").trim();
      if (/^https?:\/\//i.test(candidate)) {
        targetURL = candidate;
      }
      const verificationURI = (session.verification_uri || "").trim();
      if (/^https?:\/\//i.test(verificationURI)) {
        targetURL = verificationURI;
      }
      await openExternalUrl(targetURL);
      void messageApi.success(t("已打开 ChatGPT 登录页"));
      startOfficialOAuthAutoPoll(session);
    } catch (error) {
      setOfficialOAuthSession(null);
      const friendlyMessage = t("网络波动，暂时无法获取设备码。已自动重试 3 次，请稍后重试。");
      void messageApi.error(friendlyMessage);
    } finally {
      setOfficialOAuthLaunching(false);
    }
  }

  function sessionOrNull(session: OfficialAuthSession | null): OfficialAuthSession | null {
    if (
      session &&
      (session.device_code || "").trim() !== "" &&
      (session.user_code || "").trim() !== ""
    ) {
      return session;
    }
    return null;
  }

  function stopOfficialOAuthAutoPoll() {
    if (officialOAuthPollingTimerRef.current !== null) {
      window.clearInterval(officialOAuthPollingTimerRef.current);
      officialOAuthPollingTimerRef.current = null;
    }
    officialOAuthPollingInFlightRef.current = false;
  }

  function closeOfficialModal() {
    stopOfficialOAuthAutoPoll();
    setInternalAddModalMode(null);
    setOfficialOAuthSession(null);
  }

  async function finishOfficialImportFlow(successMessage: string) {
    stopOfficialOAuthAutoPoll();
    try {
      await refreshAccountUsage();
    } catch {
      // Keep import successful even if immediate usage refresh fails.
    }
    officialForm.resetFields();
    setOfficialOAuthSession(null);
    setInternalAddModalMode(null);
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success(successMessage);
  }

  function startOfficialOAuthAutoPoll(session: OfficialAuthSession | null) {
    stopOfficialOAuthAutoPoll();
    const readySession = sessionOrNull(session);
    if (!readySession) {
      return;
    }
    const runOnce = async () => {
      if (officialOAuthPollingInFlightRef.current) {
        return;
      }
      officialOAuthPollingInFlightRef.current = true;
      try {
        const result = await completeOfficialAuthDevice(
          {
            device_code: readySession.device_code || "",
            user_code: readySession.user_code || "",
          },
          { allowPending: true },
        );
        if (result === "completed") {
          await finishOfficialImportFlow(t("检测到 OAuth 登录完成，已自动导入"));
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : "";
        if (/expired/i.test(message) || /gone/i.test(message)) {
          stopOfficialOAuthAutoPoll();
          void messageApi.error(t("OAuth 设备码已过期，请重新获取"));
        }
      } finally {
        officialOAuthPollingInFlightRef.current = false;
      }
    };

    officialOAuthPollingTimerRef.current = window.setInterval(() => {
      void runOnce();
    }, 2000);
    void runOnce();
  }

  useEffect(() => {
    if (internalAddModalMode !== "official" || officialEntryMode !== "oauth") {
      stopOfficialOAuthAutoPoll();
    }
  }, [internalAddModalMode, officialEntryMode]);

  useEffect(() => {
    return () => {
      stopOfficialOAuthAutoPoll();
    };
  }, []);

  async function handleCopyOfficialUserCode() {
    const code = (officialOAuthSession?.user_code || "").trim();
    if (code === "") {
      return;
    }
    try {
      await writeClipboardText(code);
      void messageApi.success(t("设备码已复制"));
    } catch {
      void messageApi.error(t("复制失败，请检查系统剪贴板权限"));
    }
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
    setLuaTestResult(null);
    editForm.setFieldsValue({
      account_name: account.account_name,
      source_icon: normalizeSourceIcon(account.source_icon),
      base_url: account.base_url,
      credential_ref: "",
      usage_driver: account.usage_driver === "lua" ? "lua" : "",
      skip_tls_verify: account.skip_tls_verify ?? false,
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
    skip_tls_verify?: boolean;
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
      skip_tls_verify: values.skip_tls_verify ?? false,
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

  async function runConnectionTest(
    account: AccountRecord,
    values: { model: string; input: string },
  ) {
    setTestLoading(true);
    try {
      const result = await testAccount(account.id, values);
      setTestResult(result);
    } finally {
      setTestLoading(false);
    }
  }

  function openTestModal(account: AccountRecord) {
    setTestingAccount(account);
    setTestResult(null);
    setTestLoading(false);
    const payload = {
      model: getDefaultTestModel(account),
      input: "ping",
    };
    testForm.setFieldsValue(payload);
  }

  async function handleTest(values: { model: string; input: string }) {
    if (!testingAccount) {
      return;
    }
    await runConnectionTest(testingAccount, values);
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

  async function handleToggleAccountLock(account: AccountRecord) {
    const nextLocked = !account.is_locked;
    const previous = [...accounts];
    setAccountsState(
      (items) =>
        items.map((item) =>
          item.id === account.id ? { ...item, is_locked: nextLocked } : item,
        ),
      { preserveOrder: true },
    );
    void messageApi.success(
      formatAccountLockMessage(language, account.account_name, nextLocked),
    );
    try {
      await updateAccount(account.id, { is_locked: nextLocked });
    } catch (error) {
      setAccountsState(previous, { preserveOrder: true });
      void messageApi.error(
        error instanceof Error ? t(error.message) : t("更新账户锁定状态失败"),
      );
    }
  }

  async function handleUpdateRemoteUpstreamRoute(
    account: AccountRecord,
    upstream: ServerUpstreamAccount,
    locked: boolean,
  ) {
    if (!canManuallySelectRemoteUpstream(upstream)) {
      return;
    }
    try {
      const response = await updateAccountUpstreamRoute(account.id, {
        account_id: upstream.id,
        locked,
      });
      const preferredAccountID = response.preferred_account_id ?? upstream.id;
      setRemoteUpstreamsByAccountID((previous) => {
        const state = previous[account.id];
        if (!state?.data) {
          return previous;
        }
        return {
          ...previous,
          [account.id]: {
            ...state,
            data: {
              ...state.data,
              current_account_id: preferredAccountID,
              preferred_account_id: preferredAccountID,
              route_locked: response.route_locked,
              accounts: state.data.accounts.map((item) => ({
                ...item,
                current: item.id === preferredAccountID,
                preferred: item.id === preferredAccountID,
              })),
            },
          },
        };
      });
      void messageApi.success(
        language === "en-US"
          ? locked
            ? `Locked upstream ${upstream.account_name}`
            : `Enabled upstream ${upstream.account_name}`
          : locked
            ? `已锁定上游账户 ${upstream.account_name}`
            : `已启用上游账户 ${upstream.account_name}`,
      );
    } catch (error) {
      void messageApi.error(
        error instanceof Error ? t(error.message) : t("更新上游路由失败"),
      );
    }
  }

  async function handleUpdateRemoteUpstreamLock(
    account: AccountRecord,
    upstream: ServerUpstreamAccount,
    locked: boolean,
  ) {
    try {
      const response = await updateAccountUpstreamLock(account.id, upstream.id, locked);
      setRemoteUpstreamsByAccountID((previous) => {
        const state = previous[account.id];
        if (!state?.data) {
          return previous;
        }
        return {
          ...previous,
          [account.id]: {
            ...state,
            data: {
              ...state.data,
              available_accounts: state.data.accounts.reduce((count, item) => {
                if (item.id === upstream.id) {
                  return count + (locked ? 0 : isRemoteUpstreamStatusSelectable(item) ? 1 : 0);
                }
                return count + (item.available ? 1 : 0);
              }, 0),
              accounts: state.data.accounts.map((item) =>
                item.id === upstream.id
                  ? {
                      ...item,
                      account_locked: response.account_locked,
                      available: !response.account_locked && isRemoteUpstreamStatusSelectable(item),
                    }
                  : item,
              ),
            },
          },
        };
      });
      void messageApi.success(
        language === "en-US"
          ? locked
            ? `Locked upstream account ${upstream.account_name}`
            : `Unlocked upstream account ${upstream.account_name}`
          : locked
            ? `已锁定上游账户 ${upstream.account_name}`
            : `已解除锁定上游账户 ${upstream.account_name}`,
      );
    } catch (error) {
      void messageApi.error(
        error instanceof Error ? t(error.message) : t("更新上游锁定状态失败"),
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
    const sourceIcon = sourceIconMap[getDisplaySourceIcon(record)];
    const usageHealthState = getUsageHealthState(record);
    const aiGateAccount = isAIGateAccount(record);
    const remoteState = remoteUpstreamsByAccountID[record.id];
    const remoteUpstreams = remoteState?.data;
    const remoteExpanded = expandedRemoteAccountID === record.id && Boolean(remoteUpstreams);
    const usageWindows = record.usage_display?.summary
      ? []
      : isOfficialAccount(record)
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
    const usageAmount =
      usageWindows.length === 0 ? buildUsageAmount(record, language) : null;
    const canToggleRemote = aiGateAccount && Boolean(remoteUpstreams) && !options.actionsDisabled;
    const toggleRemote = () => {
      if (!canToggleRemote) {
        return;
      }
      setExpandedRemoteAccountID((current) => (current === record.id ? null : record.id));
    };

    return (
      <div
        ref={options.cardRef}
        className={`account-card-item ${record.is_active && !serverWebUI ? "active-account-card" : ""} ${aiGateAccount ? "has-remote-upstreams" : ""} ${remoteExpanded ? "is-remote-expanded" : ""} ${options.className ?? ""}`.trim()}
        style={options.style}
        data-actions-visible={actionsVisible ? "true" : "false"}
        role={canToggleRemote ? "button" : undefined}
        tabIndex={canToggleRemote ? 0 : undefined}
        aria-expanded={canToggleRemote ? remoteExpanded : undefined}
        aria-label={canToggleRemote ? `${remoteExpanded ? t("收起上游") : t("展开上游")}-${record.account_name}` : undefined}
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
        onClick={
          canToggleRemote
            ? (event) => {
                if (!isInteractiveTarget(event.target, event.currentTarget)) {
                  toggleRemote();
                }
              }
            : undefined
        }
        onKeyDown={
          canToggleRemote
            ? (event) => {
                if (isInteractiveTarget(event.target, event.currentTarget)) {
                  return;
                }
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  toggleRemote();
                }
              }
            : undefined
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
                alt={sourceIcon.label}
                size={36}
                shape="square"
                className="account-source-icon"
              />
              <div className="account-main-text">
                <div className="account-title-row">
                  <Text strong>{record.account_name}</Text>
                  {record.is_locked ? (
                    <Tooltip
                      title={renderAccountLockTooltip(language)}
                      placement="top"
                      getPopupContainer={() => document.body}
                    >
                      <LockOutlined
                        className="account-lock-status"
                        aria-label={`${record.account_name}-locked`}
                      />
                    </Tooltip>
                  ) : null}
                  {record.is_active && !serverWebUI ? (
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
              {aiGateAccount ? (
                <Tooltip
                  title={remoteState?.error ? t("无法读取上游状态") : t("点击展开上游详情")}
                  placement="top"
                  getPopupContainer={() => document.body}
                >
                  <div className={`account-upstream-mini ${remoteState?.loading ? "is-loading" : ""}`.trim()}>
                    <span className="account-upstream-mini-label">{t("可用/全部")}</span>
                    <span className="account-upstream-mini-value">
                      {remoteUpstreams ? `${remoteUpstreams.available_accounts}/${remoteUpstreams.total_accounts}` : "--"}
                    </span>
                  </div>
                </Tooltip>
              ) : (
                <div
                  className={`account-usage-mini ${usageWindows.length === 0 ? "account-usage-mini-empty" : ""} ${usageWindows.length === 1 ? "account-usage-mini-single" : ""}`.trim()}
                >
                  {usageAmount ? (
                    <div className="account-usage-amount">
                      <span className="account-usage-amount-label">
                        {usageAmount.label}
                      </span>
                      <span className="account-usage-amount-value">
                        {usageAmount.value}
                      </span>
                    </div>
                  ) : null}
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
              )}
              <div className="account-actions">
                {!serverWebUI ? (
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
                ) : null}
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
                  aria-label={`${t("测试")}-${record.account_name}`}
                  icon={<ApiOutlined />}
                  disabled={options.actionsDisabled}
                  onClick={() => openTestModal(record)}
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
                  aria-label={`${record.is_locked ? t("解除锁定") : t("锁定")}-${record.account_name}`}
                  icon={record.is_locked ? <UnlockOutlined /> : <LockOutlined />}
                  disabled={options.actionsDisabled}
                  onClick={() => void handleToggleAccountLock(record)}
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
        {remoteExpanded && remoteUpstreams ? (
          <div className="account-remote-upstreams-panel" data-no-row-toggle="true">
            <div className="account-remote-upstreams-list">
              {remoteUpstreams.accounts.map((account) => {
                const remoteUsageWindow = buildRemoteUpstreamUsageWindow(
                  account,
                  language,
                );
                return (
                  <div
                    className={account.current ? "account-remote-upstream-row is-current" : "account-remote-upstream-row"}
                    key={account.id}
                  >
                    <div className="account-remote-upstream-main">
                      <div className="account-remote-upstream-title">
                        <Text strong>{account.account_name}</Text>
                        {account.current ? <Tag color="green">{t("当前使用中")}</Tag> : null}
                        {account.account_locked ? <Tag color="blue">{t("已锁定")}</Tag> : null}
                        {!account.available && !account.account_locked ? <Tag>{t("不可用")}</Tag> : null}
                      </div>
                      <Text type="secondary" className="account-remote-upstream-url">
                        {account.base_url || t("OpenAI 官方")}
                      </Text>
                    </div>
                    <div className="account-remote-upstream-meta">
                      {remoteUsageWindow ? (
                        <div className="account-remote-upstream-usage-mini">
                          <div className="account-usage-mini-head">
                            <span className="account-usage-mini-label">
                              {remoteUsageWindow.label}
                            </span>
                            <span className="account-usage-mini-reset">
                              {remoteUsageWindow.resetLabel}
                            </span>
                            <span
                              className={`account-usage-mini-value is-${getRemainingTone(remoteUsageWindow.remainingPercent)}`}
                            >{`${Math.round(remoteUsageWindow.remainingPercent)}%`}</span>
                          </div>
                          <div className="account-usage-mini-meter">
                            <div className="account-usage-mini-track">
                              <div
                                className={`account-usage-mini-fill is-${getRemainingTone(remoteUsageWindow.remainingPercent)}`}
                                style={{ width: `${remoteUsageWindow.remainingPercent}%` }}
                              />
                            </div>
                          </div>
                        </div>
                      ) : (
                        <span>{formatRemoteUpstreamUsage(account, language)}</span>
                  )}
                      <span>{t(statusTextMap[account.status] ?? account.status)}</span>
                    </div>
                    <div className="account-remote-upstream-actions">
                      <Button
                        type="text"
                        className="account-action-button"
                        aria-label={`${t("启用")}-${account.account_name}`}
                        icon={<CheckCircleOutlined />}
                        disabled={!canManuallySelectRemoteUpstream(account) || (account.current && !remoteUpstreams.route_locked)}
                        onClick={() => void handleUpdateRemoteUpstreamRoute(record, account, false)}
                      />
                      <Button
                        type="text"
                        className="account-action-button"
                        aria-label={`${account.account_locked ? t("解除锁定") : t("锁定")}-${account.account_name}`}
                        icon={account.account_locked ? <UnlockOutlined /> : <LockOutlined />}
                        disabled={!isRemoteUpstreamStatusSelectable(account)}
                        onClick={() => void handleUpdateRemoteUpstreamLock(record, account, !account.account_locked)}
                      />
                    </div>
                  </div>
                );
              })}
              {remoteUpstreams.accounts.length === 0 ? (
                <Text type="secondary">{t("暂无上游账号")}</Text>
              ) : null}
            </div>
          </div>
        ) : null}
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
          <div className="account-detail-layout">
              <div className="account-detail-stats">
                {buildDetailStats(detailAccount, language).map((item) => (
                  <Card variant="borderless" key={item.title}>
                    <Statistic title={t(item.title)} value={item.value} />
                  </Card>
                ))}
              </div>
              <Card variant="borderless" className="account-detail-meta">
                <Descriptions column={2} size="small">
                  <Descriptions.Item label={t("账户")}>
                    {detailAccount.account_name}
                  </Descriptions.Item>
                  <Descriptions.Item label={t("来源")}>
                    {
                      sourceIconMap[
                        getDisplaySourceIcon(detailAccount)
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
                  <Descriptions.Item label={t("接口地址")}>
                    {detailAccount.base_url || t("OpenAI 官方")}
                  </Descriptions.Item>
                  {buildDetailUsageItems(detailAccount, language).map((item) => (
                    <Descriptions.Item label={t(item.label)} key={item.label}>
                      {item.value}
                    </Descriptions.Item>
                  ))}
                </Descriptions>
              </Card>
            </div>
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
          initialValues={{ base_url: defaultBaseURL, skip_tls_verify: false }}
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
          <Form.Item
            label={t("跳过 TLS 证书校验")}
            name="skip_tls_verify"
            valuePropName="checked"
            extra={t("仅在该账户上游使用自签名或不合规证书时开启。")}
          >
            <Switch aria-label={t("跳过 TLS 证书校验")} />
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
        onCancel={closeOfficialModal}
        footer={null}
        destroyOnHidden
        centered
        className="official-account-modal"
      >
        <Form
          form={officialForm}
          layout="vertical"
          className="official-account-form"
          onFinish={(values) => void handleCreateOfficial(values)}
        >
          <div className="official-account-header">
            <div
              className="menu-pill-group official-account-tabs"
              role="tablist"
              aria-label={t("账户导入方式")}
            >
              <button
                type="button"
                role="tab"
                aria-selected={officialEntryMode === "oauth"}
                className={officialEntryMode === "oauth" ? "menu-pill-button is-active" : "menu-pill-button"}
                onClick={() => setOfficialEntryMode("oauth")}
              >
                {t("OAuth 登录")}
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={officialEntryMode === "local_import"}
                className={officialEntryMode === "local_import" ? "menu-pill-button is-active" : "menu-pill-button"}
                onClick={() => setOfficialEntryMode("local_import")}
              >
                {t("导入本地")}
              </button>
            </div>
          </div>
          <div className="official-account-content">
            {officialEntryMode === "oauth" ? (
              <div className="official-account-panel">
                {!officialOAuthLaunching && !officialOAuthSession?.user_code ? (
                  <Button onClick={() => void handleStartOfficialOAuth()}>
                    {t("使用 ChatGPT 登录")}
                  </Button>
                ) : null}
                {officialOAuthSession?.user_code ? (
                  <div className="official-account-device-code">
                    <Text type="secondary">{t("设备码")}</Text>
                    <span className="official-account-device-code-plain">
                      {officialOAuthSession.user_code}
                    </span>
                    <div className="official-account-device-code-slots">
                      {officialOAuthSession.user_code.split("").map((char, index) =>
                        char === "-" ? (
                          <span
                            key={`dash-${index}`}
                            className="official-account-device-code-separator"
                          >
                            -
                          </span>
                        ) : (
                          <span
                            key={`slot-${index}`}
                            className="official-account-device-code-slot"
                          >
                            {char}
                          </span>
                        ),
                      )}
                    </div>
                    <Button
                      block
                      onClick={() => void handleCopyOfficialUserCode()}
                    >
                      {t("复制设备码")}
                    </Button>
                  </div>
                ) : null}
              </div>
            ) : (
              <div className="official-account-panel">
                <Form.Item
                  label={t("账户名称")}
                  name="account_name"
                  initialValue="local-codex"
                >
                  <Input />
                </Form.Item>
                <Button
                  type="primary"
                  htmlType="submit"
                  loading={officialImporting}
                >
                  {t("导入")}
                </Button>
              </div>
            )}
          </div>
          <div className="official-account-footer modal-footer">
            <Button onClick={closeOfficialModal}>
              {t("取消")}
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
              options={selectableSourceIcons.map(
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
          <Form.Item
            label={t("跳过 TLS 证书校验")}
            name="skip_tls_verify"
            valuePropName="checked"
            extra={t("仅在该账户上游使用自签名或不合规证书时开启。")}
          >
            <Switch aria-label={t("跳过 TLS 证书校验")} />
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

      <Modal
        open={!!testingAccount}
        title={t("连接测试")}
        onCancel={() => {
          setTestingAccount(null);
          setTestLoading(false);
        }}
        footer={null}
        destroyOnHidden
        centered
      >
        <Form
          form={testForm}
          layout="vertical"
          onFinish={(values) => void handleTest(values)}
        >
          <Form.Item
            label={t("模型")}
            name="model"
            rules={[{ required: true, message: t("请选择模型") }]}
          >
            <AutoComplete
              options={TEST_MODEL_SUGGESTIONS.map((value) => ({ value }))}
              filterOption={(inputValue, option) =>
                (option?.value ?? "")
                  .toString()
                  .toLowerCase()
                  .includes(inputValue.toLowerCase())
              }
              placeholder="gpt-5.4"
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
            <Button htmlType="submit" loading={testLoading}>
              {t("测试")}
            </Button>
          </div>
        </Form>
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
