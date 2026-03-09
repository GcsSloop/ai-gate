# Thin Gateway Third-Party Responses Implementation Plan

> Historical note: this plan records the transition from official-only filtering to native third-party `/responses` support. The current code already implements this behavior; do not treat the step list below as pending work.

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 让薄网关模式支持“原生 `/responses`”第三方供应商，且不引入任何协议兼容补偿。

**Architecture:** 账号模型新增显式能力位 `supports_responses`。薄网关模式下的候选筛选从“只选官方账号”改为“只选 `supports_responses=true` 的账号”；官方和第三方都走透明代理。对不支持 `/responses` 的激活账号直接失败，并记录明确日志。

**Tech Stack:** Go、SQLite、现有 `/responses` handler、现有 accounts API、前端账户页。

---

### Task 1: 增加账号能力位 `supports_responses`

**Files:**
- Modify: `backend/internal/accounts/types.go`
- Modify: `backend/internal/accounts/repository.go`
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/api/accounts_handler.go`
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/features/accounts/AccountsPage.tsx`
- Test: `backend/internal/accounts/repository_test.go`

**Step 1: Write the failing test**
- 新增仓储测试，要求 `supports_responses` 可持久化并正确读回。

**Step 2: Run test to verify it fails**
- Run: `cd backend && go test ./internal/accounts -run SupportsResponses -count=1`

**Step 3: Write minimal implementation**
- 在账号模型、SQLite 映射、API DTO、前端类型中增加 `supports_responses`。
- 官方账号默认 `true`；第三方账号默认 `false`。

**Step 4: Run test to verify it passes**
- Run: `cd backend && go test ./internal/accounts -run SupportsResponses -count=1`

**Step 5: Commit**
- `git commit -m "feat: add supports_responses account capability"`

### Task 2: 薄网关候选筛选改为按 `supports_responses`

**Files:**
- Modify: `backend/internal/api/responses_handler.go`
- Test: `backend/internal/api/responses_compat_test.go`

**Step 1: Write the failing test**
- 新增测试：
  - 激活的第三方账号 `supports_responses=true` 时，薄网关命中它。
  - 激活账号 `supports_responses=false` 时，直接失败，不切到别的账号。

**Step 2: Run test to verify it fails**
- Run: `cd backend && go test ./internal/api -run 'TestResponsesHandlerThinMode(UsesActiveThirdPartyResponsesAccount|FailsWhenActiveAccountDoesNotSupportResponses)' -count=1`

**Step 3: Write minimal implementation**
- 候选选择逻辑优先检查 active account。
- active account 存在但 `supports_responses=false` 时，返回明确错误。
- 没有 active account 时，只在 `supports_responses=true` 账号中按评分选择。

**Step 4: Run test to verify it passes**
- Run: `cd backend && go test ./internal/api -run 'TestResponsesHandlerThinMode(UsesActiveThirdPartyResponsesAccount|FailsWhenActiveAccountDoesNotSupportResponses)' -count=1`

**Step 5: Commit**
- `git commit -m "feat: select thin gateway accounts by responses capability"`

### Task 3: 第三方账号直接透明代理 `/responses`

**Files:**
- Modify: `backend/internal/api/responses_handler.go`
- Modify: `backend/internal/providers/openai/adapter.go` if needed
- Test: `backend/internal/api/responses_handler_test.go`
- Test: `backend/internal/api/contracts/thin_gateway_contract_test.go`

**Step 1: Write the failing test**
- 新增测试：
  - 第三方账号在薄网关模式下请求 `base_url + /responses`
  - 请求/响应/SSE 原样透传
  - 不走 `/chat/completions`

**Step 2: Run test to verify it fails**
- Run: `cd backend && go test ./internal/api -run 'TestResponsesHandlerThinModeThirdPartyResponsesPassthrough' -count=1`

**Step 3: Write minimal implementation**
- 对官方账号继续走 codex adapter。
- 对第三方 `supports_responses=true` 账号，直接构造 `/responses` 请求并透传原始 body。

**Step 4: Run test to verify it passes**
- Run: `cd backend && go test ./internal/api -run 'TestResponsesHandlerThinModeThirdPartyResponsesPassthrough' -count=1`

**Step 5: Commit**
- `git commit -m "feat: proxy third-party responses in thin gateway mode"`

### Task 4: 补充筛选与失败日志

**Files:**
- Modify: `backend/internal/api/request_logging.go`
- Modify: `backend/internal/api/responses_handler.go`
- Test: `backend/internal/api/request_logging_test.go`

**Step 1: Write the failing test**
- 新增测试：
  - 激活账号被跳过时，日志明确包含 `reason`
  - 激活账号不支持 `/responses` 时，日志写明 `supports_responses=false`

**Step 2: Run test to verify it fails**
- Run: `cd backend && go test ./internal/api -run 'TestResponsesHandlerLogsThinGatewayCandidateSkipReason' -count=1`

**Step 3: Write minimal implementation**
- 增加 `candidate selected/skip/fail` 日志。

**Step 4: Run test to verify it passes**
- Run: `cd backend && go test ./internal/api -run 'TestResponsesHandlerLogsThinGatewayCandidateSkipReason' -count=1`

**Step 5: Commit**
- `git commit -m "feat: add thin gateway candidate selection logs"`

### Task 5: 最终验证

**Files:**
- No code changes unless verification fails.

**Step 1: Run targeted backend tests**
- Run: `cd backend && go test ./internal/api ./internal/accounts -count=1`

**Step 2: Run full backend tests**
- Run: `cd backend && go test ./... -count=1`

**Step 3: Run frontend tests**
- Run: `npm --prefix frontend test`

**Step 4: Commit any final doc/test adjustments**
- `git commit -m "test: verify thin gateway third-party responses support"` if needed
