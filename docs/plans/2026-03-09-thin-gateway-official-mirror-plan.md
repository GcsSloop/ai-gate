# 官方接口薄网关实施计划

> 历史方案归档：该计划基于“薄网关可切换模式 + 官方上游限定”的旧假设，已被当前“薄网关单一路径 + 第三方仅限原生 `/responses`”实现取代。保留此文档仅作演进记录，不应再作为当前实施依据。

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**目标：** 将当前网关收敛为“真正的薄网关层”，仅负责官方接口的认证管理、路由转发、协议透传和可观测性，不承担本地会话语义、工具编排、多轮补偿和第三方兼容逻辑。

**架构：** 网关对外暴露与官方一致的接口形态，对内只调用官方上游并透传其请求体、响应体、SSE 事件和错误模型。所有协议主语义以上游为准；本地数据库只保存审计与观测数据，不参与 `response_id`、`previous_response_id`、对话恢复、工具闭环等核心行为。

**技术栈：** Go、`net/http`、SSE、SQLite、现有后端路由与测试框架。

---

## 边界约束

**必须保留：**
- 官方接口路径、方法、状态码、错误结构、SSE 生命周期
- 官方账号认证与刷新
- 请求日志、审计日志、基础运行指标

**必须删除或禁用：**
- 本地生成 `response_id` 并作为协议主语义
- 用本地消息重放替代 `previous_response_id`
- `/responses` 到 `/chat/completions` 的兼容回退
- 网关内工具执行、多轮编排、MCP 调度
- 第三方 OpenAI 兼容供应商支持（在薄网关模式下）

**本计划的明确非目标：**
- 不在网关中实现 openai-codex 的 turn loop
- 不在网关中实现 function/tool call 闭环
- 不在网关中保留“跨账号共享一条对话链”的自定义能力
- 不承诺本地检索接口继续提供自造响应内容

### 任务 1：新增“薄网关模式”配置并冻结运行边界

**文件：**
- 修改：`backend/internal/config/config.go`
- 修改：`backend/internal/config/config_test.go`
- 修改：`.env.example`

**步骤 1：先写失败测试**

新增配置测试，要求：
- `THIN_GATEWAY_MODE=true` 可解析
- 默认值对新部署为 `true`
- 允许通过环境变量关闭，用于紧急回滚

**步骤 2：运行测试确认失败**

运行：`cd backend && go test ./internal/config -run ThinGateway -count=1`

预期：FAIL，因为配置项尚不存在。

**步骤 3：补最小实现**

新增配置项：
- `ThinGatewayMode bool`
- 回滚开关：`THIN_GATEWAY_MODE=false`

**步骤 4：运行测试确认通过**

运行：`cd backend && go test ./internal/config -run ThinGateway -count=1`

预期：PASS。

**步骤 5：提交**

```bash
git add backend/internal/config/config.go backend/internal/config/config_test.go .env.example
git commit -m "feat: add thin gateway mode config"
```

### 任务 2：先定义薄网关模式下对外接口清单

**文件：**
- 修改：`README.md`
- 新增：`docs/thin-gateway-mode.md`
- 修改：`docs/testing.md`

**步骤 1：列出允许保留的接口**

明确仅保留这些官方镜像接口：
- `POST /v1/responses`
- `GET /v1/models`
- 其他仅在官方有稳定语义且可透明代理时保留

同时明确这些接口的处理策略：
- 若官方上游可透传：保留
- 若当前网关只能依赖本地语义实现：在薄网关模式禁用

**步骤 2：文档中明确禁用项**

写清楚以下能力在薄网关模式不可用：
- 本地 response 检索拼装
- 本地 input_items 重建
- 本地 cancel/delete 模拟
- 第三方兼容模式

**步骤 3：自检**

运行：`rg -n "薄网关|thin gateway|response_id|previous_response_id|fallback" README.md docs -S`

预期：文档描述一致，不再混入“本地语义补偿”表述。

**步骤 4：提交**

```bash
git add README.md docs/thin-gateway-mode.md docs/testing.md
git commit -m "docs: define thin gateway mode boundary"
```

### 任务 3：将 `responses` 主路径改造成透明代理

**文件：**
- 修改：`backend/internal/api/responses_handler.go`
- 修改：`backend/internal/api/responses_handler_test.go`

**步骤 1：先写失败测试**

新增测试验证：
- 网关不再生成本地 `resp_*`
- 上游返回的 `id` 原样返回
- 上游 SSE 事件中的 `response` 语义不被本地替换

**步骤 2：运行测试确认失败**

运行：`cd backend && go test ./internal/api -run 'TestResponsesHandler(ThinModePassthroughID|ThinModePassthroughSSE)' -count=1`

预期：FAIL，因为当前实现会构造本地 ID 和本地事件。

**步骤 3：补最小实现**

在 `ThinGatewayMode=true` 时：
- 不调用 `newRouterResponseID*`
- 不根据本地 conversation 改写 response body
- 不拼装本地输出项替换上游结果
- 仅负责转发、流式复制、错误映射、日志记录

**步骤 4：运行测试确认通过**

运行：`cd backend && go test ./internal/api -run 'TestResponsesHandler(ThinModePassthroughID|ThinModePassthroughSSE)' -count=1`

预期：PASS。

**步骤 5：提交**

```bash
git add backend/internal/api/responses_handler.go backend/internal/api/responses_handler_test.go
git commit -m "refactor: make responses path transparent in thin gateway mode"
```

### 任务 4：移除 `previous_response_id` 的本地语义替代

**文件：**
- 修改：`backend/internal/api/responses_handler.go`
- 修改：`backend/internal/gateway/openai/responses.go`
- 修改：`backend/internal/api/responses_handler_test.go`

**步骤 1：先写失败测试**

新增测试要求：
- 请求中的 `previous_response_id` 原样透传给官方上游
- 薄网关模式下不通过本地 `conversationIDFromResponseID` 驱动协议流程

**步骤 2：运行测试确认失败**

运行：`cd backend && go test ./internal/api -run 'TestResponsesHandler(ThinModeForwardsPreviousResponseID|ThinModeNoLocalReplay)' -count=1`

预期：FAIL，因为当前逻辑优先依赖本地 replay。

**步骤 3：补最小实现**

在薄网关模式下：
- 不调用本地 replay 路径构造协议输入
- `previous_response_id` 仅作为上游请求字段
- 本地数据库只记录观测数据，不参与协议重建

**步骤 4：运行测试确认通过**

运行：`cd backend && go test ./internal/api -run 'TestResponsesHandler(ThinModeForwardsPreviousResponseID|ThinModeNoLocalReplay)' -count=1`

预期：PASS。

**步骤 5：提交**

```bash
git add backend/internal/api/responses_handler.go backend/internal/gateway/openai/responses.go backend/internal/api/responses_handler_test.go
git commit -m "fix: forward previous_response_id without local replay in thin mode"
```

### 任务 5：在薄网关模式下禁用厚网关接口

**文件：**
- 修改：`backend/internal/api/responses_handler.go`
- 修改：`backend/internal/api/responses_compact_test.go`
- 修改：`backend/internal/api/responses_handler_test.go`

**步骤 1：先写失败测试**

新增测试明确薄网关模式下以下接口行为：
- `/v1/responses/compact` 返回禁用或与官方一致的不可用行为
- `/v1/responses/{id}`
- `/v1/responses/{id}/input_items`
- `/v1/responses/{id}/cancel`
- `DELETE /v1/responses/{id}`

如果当前无法透明透传上游实现，则应返回明确不可用状态，而不是本地伪实现。

**步骤 2：运行测试确认失败**

运行：`cd backend && go test ./internal/api -run 'TestResponsesHandler(ThinModeDisablesSyntheticEndpoints|ThinModeDisablesCompact)' -count=1`

预期：FAIL，因为当前仍有本地实现。

**步骤 3：补最小实现**

在薄网关模式下：
- 禁用依赖本地状态拼装的接口
- 返回固定、可解释、可测试的错误语义

**步骤 4：运行测试确认通过**

运行：`cd backend && go test ./internal/api -run 'TestResponsesHandler(ThinModeDisablesSyntheticEndpoints|ThinModeDisablesCompact)' -count=1`

预期：PASS。

**步骤 5：提交**

```bash
git add backend/internal/api/responses_handler.go backend/internal/api/responses_compact_test.go backend/internal/api/responses_handler_test.go
git commit -m "feat: disable synthetic responses endpoints in thin gateway mode"
```

### 任务 6：彻底关闭第三方兼容回退

**文件：**
- 修改：`backend/internal/api/responses_handler.go`
- 修改：`backend/internal/api/responses_compat_test.go`
- 修改：`backend/internal/api/accounts_handler.go`

**步骤 1：先写失败测试**

新增测试要求：
- 薄网关模式下不允许 `/responses -> /chat/completions` 回退
- `AllowChatFallback` 对薄网关模式无效
- 使用第三方账号时返回明确不支持错误

**步骤 2：运行测试确认失败**

运行：`cd backend && go test ./internal/api -run 'TestResponsesHandler(ThinModeNoChatFallback|ThinModeRejectsThirdPartyAccounts)' -count=1`

预期：FAIL，因为当前兼容路径仍然存在。

**步骤 3：补最小实现**

在薄网关模式下：
- 仅允许官方账号
- 删除或跳过 `tryExecuteCompatibleResponses*`
- 对第三方账号返回固定错误

**步骤 4：运行测试确认通过**

运行：`cd backend && go test ./internal/api -run 'TestResponsesHandler(ThinModeNoChatFallback|ThinModeRejectsThirdPartyAccounts)' -count=1`

预期：PASS。

**步骤 5：提交**

```bash
git add backend/internal/api/responses_handler.go backend/internal/api/responses_compat_test.go backend/internal/api/accounts_handler.go
git commit -m "feat: restrict thin gateway mode to official upstream only"
```

### 任务 7：收敛本地数据库职责为“审计与观测”

**文件：**
- 修改：`backend/internal/conversations/repository.go`
- 修改：`backend/internal/usage/repository.go`
- 修改：`backend/internal/api/responses_handler.go`
- 修改：`backend/internal/api/conversations_handler.go`

**步骤 1：先写失败测试**

新增测试要求：
- 请求与运行记录仍可保存
- 本地记录不再被用来驱动协议主流程
- 观测页面仍能看到运行概况，但不再伪造官方 response 内容

**步骤 2：运行测试确认失败**

运行：`cd backend && go test ./internal/api -run 'Test(ThinModeAuditStillWorks|ThinModeStorageNotUsedForProtocol)' -count=1`

预期：FAIL，因为当前存储和协议语义耦合。

**步骤 3：补最小实现**

调整职责：
- conversations/runs/messages 仅用于审计
- 不再由这些表反推 `response_id` 语义
- 监控与观测界面仅展示本地记录，不冒充官方检索接口

**步骤 4：运行测试确认通过**

运行：`cd backend && go test ./internal/api -run 'Test(ThinModeAuditStillWorks|ThinModeStorageNotUsedForProtocol)' -count=1`

预期：PASS。

**步骤 5：提交**

```bash
git add backend/internal/conversations/repository.go backend/internal/usage/repository.go backend/internal/api/responses_handler.go backend/internal/api/conversations_handler.go
git commit -m "refactor: limit local storage to audit and observability in thin mode"
```

### 任务 8：强化账号切换与认证刷新，只保证“稳定透传”

**文件：**
- 修改：`backend/internal/api/official_auth.go`
- 修改：`backend/internal/api/responses_handler.go`
- 修改：`backend/internal/api/official_refresh_test.go`
- 修改：`backend/internal/api/responses_handler_test.go`

**步骤 1：先写失败测试**

新增测试要求：
- 切换官方账号后，新请求不会复用旧认证上下文
- refresh 失败时返回一个明确终态，不会无返回
- 流式请求中断时也必须有终态错误

**步骤 2：运行测试确认失败**

运行：`cd backend && go test ./internal/api -run 'Test(ThinModeAccountSwitchIsolated|ThinModeRefreshFailureTerminal)' -count=1`

预期：FAIL，因为当前仍有本地拼装路径和旧上下文风险。

**步骤 3：补最小实现**

只做这三件事：
- 请求前认证刷新
- 账号切换时重置上游连接上下文
- 所有失败路径都输出明确终态

不在这里增加任何编排逻辑。

**步骤 4：运行测试确认通过**

运行：`cd backend && go test ./internal/api -run 'Test(ThinModeAccountSwitchIsolated|ThinModeRefreshFailureTerminal)' -count=1`

预期：PASS。

**步骤 5：提交**

```bash
git add backend/internal/api/official_auth.go backend/internal/api/responses_handler.go backend/internal/api/official_refresh_test.go backend/internal/api/responses_handler_test.go
git commit -m "fix: isolate account auth context in thin gateway mode"
```

### 任务 9：建立“薄网关 contract tests”

**文件：**
- 新增：`backend/internal/api/contracts/thin_gateway_contract_test.go`
- 新增：`backend/internal/api/contracts/testdata/*.http`
- 修改：`backend/internal/api/responses_handler_test.go`

**步骤 1：先写失败测试**

contract tests 只验证三类事情：
- 请求字段是否原样透传
- 响应状态码/错误体/SSE 终态是否与上游一致
- 网关没有注入额外协议语义

**步骤 2：运行测试确认失败**

运行：`cd backend && go test ./internal/api/contracts -run ThinGateway -count=1`

预期：FAIL，直到代理行为收敛。

**步骤 3：补最小实现**

建立 fixture 驱动断言：
- 比较 header/body/event shape
- 允许时间戳、trace id 等非确定性字段豁免
- 禁止出现本地 `resp_*`、本地 synthetic output

**步骤 4：运行测试确认通过**

运行：`cd backend && go test ./internal/api/contracts -run ThinGateway -count=1`

预期：PASS。

**步骤 5：提交**

```bash
git add backend/internal/api/contracts backend/internal/api/responses_handler_test.go
git commit -m "test: add thin gateway passthrough contract suite"
```

### 任务 10：最终验证与发布前检查

**文件：**
- 无需修改，除非验证时发现问题。

**步骤 1：运行 API 测试**

运行：`cd backend && go test ./internal/api/... -count=1`

预期：PASS。

**步骤 2：运行全量后端测试**

运行：`cd backend && go test ./... -count=1`

预期：PASS。

**步骤 3：执行后端 smoke**

运行：`bash scripts/ci/run_backend_smoke.sh`

预期：PASS。

**步骤 4：人工验收清单**

- `POST /v1/responses` 为透明代理
- `previous_response_id` 不再本地替代
- 不存在 chat fallback
- 切账号时没有无返回
- 本地数据库不再承担协议语义

**步骤 5：若验证发现问题，再补最后修复提交**

```bash
git add <修复文件>
git commit -m "chore: finalize thin gateway verification fixes"
```

## 上线策略

1. 在预发环境启用 `THIN_GATEWAY_MODE=true`
2. 跑 contract tests、smoke tests、账号切换稳定性测试
3. 观察 24 小时错误率与无返回问题
4. 再切生产

## 回滚策略

1. 设置 `THIN_GATEWAY_MODE=false`
2. 重启后端
3. 执行 smoke test 确认旧兼容路径恢复

## 执行注意事项

- 当前工作树已有未提交改动，实施前应先确认是否需要单独 worktree
- 每一步只能做最小改动，避免在“薄网关模式”里再次引入本地语义
- 若发现某个官方接口当前无法透明实现，应优先禁用，而不是补本地伪实现
