# Lua Usage DSL

本文记录 AI Gate 第三方用量采集脚本的规范。Lua 脚本只负责读取上游用量接口并返回标准化结果，不改变代理协议语义。

## 推荐入口

简单余额接口使用 `simple_usage`：

```lua
simple_usage({
  get = "/v1/usage",
  auth = "bearer",
  remaining = pick("remaining", "quota.remaining", "balance"),
  unit = pick("unit", "quota.unit", default("USD")),
  valid = pick("is_active", "isValid", default(true)),
  display = {
    summary = { label = "余额", value = "$61.96" },
    detail_stats = {
      { label = "余额", value = "$61.96" }
    },
    detail_items = {
      { label = "计费单位", value = "USD" }
    }
  }
})
```

复杂接口使用 `usage_adapter`，在 `limits` 中返回路由判断所需数据，在 `display` 中返回 UI 显示文案。

## 标准返回结构

旧版 `function fetch_usage(ctx)` 仍然兼容。成功返回值可以包含：

```lua
return {
  ok = true,
  source = "remote",
  confidence = "high",
  limits = {
    balance = nil,
    quota_remaining = nil,
    rpm_remaining = nil,
    tpm_remaining = nil,
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
```

`limits` 用于路由、冷却恢复和健康判断。`display` 只用于界面显示：

- `display.summary` 控制账户列表右侧摘要。
- `display.detail_stats` 控制账户详情页上方统计卡片。
- `display.detail_items` 控制账户详情页下方用量条目。

如果脚本不返回 `display`，前端继续使用旧逻辑：官方账号显示 5H/7D 窗口，PPChat 显示日配额，普通第三方账号显示余额或剩余配额。
