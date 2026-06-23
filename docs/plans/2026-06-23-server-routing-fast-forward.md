# 服务端路由快速转发实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 移除服务端用户账户池逻辑，让服务端网关请求默认使用全部上游账户，并通过全局最优路径加短期粘性缓存提升多并发转发效率。

**Architecture:** 服务端用户仅承担 token 鉴权、登录会话和用量归属，不再拥有独立账户池、排序、激活或锁定状态。网关候选从全局账户池构造，按健康度、优先级和容量评分选出最优路径；选中后写入进程内 sticky selector，在 TTL 内优先复用，只有上游不可达、限流、额度不足或冷却时失效并切换到下一个候选。请求热路径不查询 `server_user_accounts`，服务模式成功/失败切换不依赖 `SetActive` 或 cooldown 同步写库，token 鉴权不做每请求同步写入；gateway/responses 使用内存缓存和异步 usage 写入队列，队列压力过高时优先保障转发而不是回退同步写库。

**Tech Stack:** Go `net/http`、SQLite repository、现有 `routing.Candidate` 评分模型、React + Ant Design。

---

### Task 1: 后端服务用户不再需要账户池

**Files:**
- Modify: `backend/internal/api/gateway_handler_test.go`
- Modify: `backend/internal/api/responses_handler_test.go`
- Modify: `backend/internal/api/server_users_handler_test.go`
- Modify: `backend/internal/api/server_me_handler_test.go`
- Modify: `backend/internal/api/gateway_handler.go`
- Modify: `backend/internal/api/responses_handler.go`
- Modify: `backend/internal/api/server_users_handler.go`
- Modify: `backend/internal/api/server_me_handler.go`
- Modify: `backend/internal/serverusers/repository.go`

**Steps:**
1. 写失败测试：新建服务用户没有账户分配时，合法 token 请求仍能命中全局上游账户。
2. 写失败测试：服务用户管理 API 不再暴露 `/server-users/{id}/accounts`。
3. 写失败测试：普通用户 `/me/accounts` 和 `/me/accounts/{id}/state` 不再可用。
4. 移除 gateway 和 responses handler 中的服务用户账户池过滤。
5. 移除 handler interface 中的账户池方法依赖。
6. 保留 legacy repository 方法和 SQLite 表作为兼容残留，但运行时不调用。

**Verification:**
- `cd backend && go test ./internal/api ./internal/serverusers -run 'ServerUser|Gateway|Responses|Assigned|ServerMe' -count=1`

### Task 2: 全局最优路径短期粘性

**Files:**
- Create: `backend/internal/routing/sticky.go`
- Create: `backend/internal/routing/sticky_test.go`
- Modify: `backend/internal/api/account_routing_state.go`
- Modify: `backend/internal/api/gateway_handler.go`
- Modify: `backend/internal/api/responses_handler.go`

**Steps:**
1. 写失败测试：sticky selector 在 TTL 内把上次成功账户移动到候选首位。
2. 写失败测试：失效 sticky 后会重新选择评分最高候选。
3. 实现并发安全的进程内 selector，按 scope 记录 `account_id/expires_at`。
4. 成功请求后记住命中账户，容量/限流/连接错误时失效该 scope。
5. 服务用户请求只更新进程内 sticky，不在转发热路径同步写入全局 active/cooldown 状态。
6. 在 chat 和 responses 两条转发路径复用相同排序逻辑。

**Verification:**
- `cd backend && go test ./internal/routing ./internal/api -run 'Sticky|Gateway|Responses' -count=1`

### Task 3: 前端移除账户池界面

**Files:**
- Modify: `frontend/src/features/server-users/ServerUsersPage.tsx`
- Modify: `frontend/src/features/server-users/ServerUsersPage.test.tsx`
- Modify: `frontend/src/features/server-users/UserPoolPage.tsx`
- Modify: `frontend/src/features/server-users/UserPoolPage.test.tsx`
- Modify: `frontend/src/lib/api.ts`

**Steps:**
1. 写失败测试：服务用户页面不再显示“账户池/分配账户”。
2. 写失败测试：普通用户页面只显示个人用量，不显示账户池状态编辑表。
3. 删除前端账户池 API 调用和相关类型。
4. 保留服务用户创建、禁用、轮换 token 和用量展示。

**Verification:**
- `npm --prefix frontend test -- ServerUsersPage UserPoolPage`

### Task 4: 收敛验证

**Verification:**
- `cd backend && go test ./internal/api ./internal/routing ./internal/serverusers -count=1`
- `npm --prefix frontend test -- ServerUsersPage UserPoolPage`

No commit is created unless explicitly requested.

### Task 5: 转发热路径异步写库

**Files:**
- Create: `backend/internal/api/async_usage.go`
- Create: `backend/internal/api/async_usage_test.go`
- Modify: `backend/internal/api/usage_events.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`

**Steps:**
1. 写失败测试：慢 `SaveEvent` 不阻塞 `AsyncUsageStore.SaveEvent` 调用方。
2. 实现内存 snapshot cache 和异步写入队列。
3. `persistUsageEvent` 遇到异步 repo 时只入队，不在队列满时回退同步写库。
4. bootstrap 只给 gateway/responses handler 使用异步 usage wrapper，其余 dashboard/monitoring/refresh 继续使用原始 repository。

**Verification:**
- `cd backend && go test ./internal/api ./internal/bootstrap -run 'AsyncUsage|NewApp|Gateway|Responses' -count=1`
