# 用户上游路由控制实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 让普通服务用户能查看脱敏上游账号状态，并控制自己的手动切换和锁定路由。

**Architecture:** 用户级路由偏好持久化在 `server_users`，随现有 token 鉴权进入 context，转发热路径不新增 DB 查询。页面 API 返回脱敏账号和用量摘要，手动操作才写库并同步进程内 sticky。

**Tech Stack:** Go `net/http`、SQLite、现有 `routing.StickySelector`、React + Ant Design。

---

### Task 1: 用户路由状态模型

**Files:**
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/store/sqlite/store.go`
- Modify: `backend/internal/serverusers/types.go`
- Modify: `backend/internal/serverusers/repository.go`
- Modify: `backend/internal/serverusers/repository_test.go`

**Steps:**
1. 写失败测试：服务用户可保存 `preferred_account_id` 和 `route_locked`。
2. 增加 SQLite 列和 repository 读写。
3. 回跑 `cd backend && go test ./internal/serverusers -count=1`。

### Task 2: 用户自助上游 API

**Files:**
- Modify: `backend/internal/api/server_me_handler.go`
- Modify: `backend/internal/api/server_me_handler_test.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`

**Steps:**
1. 写失败测试：`GET /me/upstreams` 返回脱敏账号状态、用量、当前账号和统计。
2. 写失败测试：`PUT /me/route` 可切换和锁定指定账号。
3. 实现 handler 和 bootstrap 注入账号、用量、sticky selector。

### Task 3: 转发路由偏好

**Files:**
- Modify: `backend/internal/routing/sticky.go`
- Modify: `backend/internal/routing/sticky_test.go`
- Modify: `backend/internal/api/gateway_handler.go`
- Modify: `backend/internal/api/responses_handler.go`
- Modify: `backend/internal/api/gateway_handler_test.go`
- Modify: `backend/internal/api/responses_handler_test.go`

**Steps:**
1. 写失败测试：用户偏好账号优先于全局评分，且不写全局 active/cooldown。
2. 写失败测试：不同用户 sticky scope 互不影响。
3. 实现用户级 scope、偏好排序和 sticky 查询。

### Task 4: 前端自助页面

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/features/server-users/UserPoolPage.tsx`
- Modify: `frontend/src/features/server-users/UserPoolPage.test.tsx`
- Modify: `frontend/src/styles.css`

**Steps:**
1. 写失败测试：页面展示可用/总数、账号行、当前使用中、切换和锁定按钮。
2. 实现 API wrapper 和 UI。
3. 确认不渲染 token 或 credential 字段。

### Task 5: 收敛验证

**Verification:**
- `cd backend && go test ./... -count=1`
- `npm --prefix frontend test -- UserPoolPage`
- `npm --prefix frontend run build`
- `git diff --check`

No commit is created unless explicitly requested.
