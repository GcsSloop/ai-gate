# 服务端用户账户池 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在服务端模式中增加普通用户登录、用户级上游账户池分配、用户独立排序/激活/锁定以及按用户隔离的用量和候选路由。

**Architecture:** 管理员仍使用现有访问密码登录控制台并维护全局上游账号；普通用户使用 `username + token` 登录，仅访问 `/me/*` 自助接口。新增 `server_user_accounts` 作为用户到全局账号的分配和用户级路由状态表，网关在服务端 token 认证后只从该用户分配池构造候选，不修改全局账号顺序、激活和锁定状态。

**Tech Stack:** Go `net/http` + SQLite repository + existing routing candidates, React + Vite + Ant Design, Go tests and Vitest.

---

## PDCA 0: 计划闭环

**Plan:** 固化需求、架构和后续验证边界。

**Do:**
- Create: `docs/plans/2026-06-15-server-user-account-pools.md`

**Check:**
- Run: `git diff -- docs/plans/2026-06-15-server-user-account-pools.md`
- Expected: 计划覆盖普通用户权限、管理员分配、网关候选过滤、前端限制和验证命令。

**Act:**
- Commit: `docs: 增加服务端用户账户池计划`

## PDCA 1: 服务端用户模型和账户池仓库

**Plan:** 先把数据模型做实，普通用户默认没有分配账号，分配表保存用户级 `position/is_active/is_locked`。

**Files:**
- Modify: `backend/internal/store/sqlite/migrations.go`
- Modify: `backend/internal/serverusers/types.go`
- Modify: `backend/internal/serverusers/repository.go`
- Test: `backend/internal/serverusers/repository_test.go`

**Do:**
1. 写失败测试：
   - 新建用户含 `username`、`role=user`、`status=active`。
   - `AuthenticateLogin(username, token)` 成功，错误用户名或错误 token 失败。
   - 新用户 `ListAssignedAccounts(userID)` 返回空。
   - `SetAccountAssignments(userID, accountIDs)` 后返回脱敏账号池视图。
   - `UpdateUserAccountState(userID, accountID, position, is_active, is_locked)` 只改该用户分配表。
2. Run red:
   - `cd backend && go test ./internal/serverusers -run 'AccountPool|Login|Assignment' -count=1`
3. 最小实现：
   - `server_users` 增加 `username` 唯一列和 `role` 列，兼容旧 `name`。
   - 新增 `server_user_accounts(user_id, account_id, position, is_active, is_locked, created_at, updated_at)`。
   - 仓库方法暴露账号池分配、用户级状态更新和用户登录。
4. Run green:
   - `cd backend && go test ./internal/serverusers -count=1`

**Check:** 确认新建普通用户没有默认账号池，分配表没有写入全局 `accounts` 的 `priority/is_active/is_locked`。

**Act:** Commit: `feat: 增加服务端用户账户池仓库`

## PDCA 2: 管理员用户管理 API

**Plan:** 管理员 API 继续由现有 password session 保护，新增账号池分配接口和用户明细，默认新建用户不分配账号。

**Files:**
- Modify: `backend/internal/api/server_users_handler.go`
- Test: `backend/internal/api/server_users_handler_test.go`

**Do:**
1. 写失败测试：
   - `GET /server-users` 返回每个用户的已分配账号数量。
   - `GET /server-users/{id}/accounts` 返回全局账号列表和该用户分配状态。
   - `PUT /server-users/{id}/accounts` 可指定账号 ID 列表，空列表代表无账号池。
   - 普通用户自助字段不能通过管理员分配接口之外的账户配置 API 暴露。
2. Run red:
   - `cd backend && go test ./internal/api -run 'ServerUsers.*Account|ServerUsersHandler' -count=1`
3. 最小实现 handler 路由、payload 校验、错误码。
4. Run green:
   - `cd backend && go test ./internal/api -run 'ServerUsers.*Account|ServerUsersHandler' -count=1`

**Check:** 管理员能配置分配范围，但不签发默认账号池。

**Act:** Commit: `feat: 增加服务端用户账户池管理接口`

## PDCA 3: 普通用户登录和自助 API

**Plan:** 普通用户通过 `username + token` 换取独立 session，只能访问 `/me/*`，不能访问 `/accounts`、`/settings`、`/server-users` 等管理员接口。

**Files:**
- Create: `backend/internal/serverauth/user_session.go`
- Modify: `backend/internal/bootstrap/bootstrap.go`
- Create/Modify: `backend/internal/api/server_me_handler.go`
- Test: `backend/internal/serverauth/session_test.go`
- Test: `backend/internal/api/server_me_handler_test.go`
- Test: `backend/internal/bootstrap/bootstrap_test.go` if existing harness fits

**Do:**
1. 写失败测试：
   - `POST /auth/user-login` 使用用户名和 token 成功，返回 `role=user`。
   - `GET /auth/session` 能区分 admin/user session。
   - 普通用户 session 请求 `/api/accounts` 得到 `401/403`。
   - `GET /api/me` 返回自己的用户信息和用量聚合。
   - `GET /api/me/accounts` 只返回被分配账号的脱敏池信息。
   - `PUT /api/me/accounts/order` 和 `PUT /api/me/accounts/{accountID}/state` 只能更新自己的 `position/is_active/is_locked`。
2. Run red:
   - `cd backend && go test ./internal/serverauth ./internal/api ./internal/bootstrap -run 'UserLogin|Me|ServerModePermission' -count=1`
3. 最小实现：
   - session cookie 记录角色和用户 ID。
   - 管理员 session 才能通过现有 `/api/*` 管理接口。
   - `/api/me/*` 接受普通用户 session，管理员可选访问但不作为普通用户视图。
4. Run green:
   - `cd backend && go test ./internal/serverauth ./internal/api ./internal/bootstrap -run 'UserLogin|Me|ServerModePermission' -count=1`

**Check:** 普通用户没有配置、创建、删除、导入上游账号的 API 权限。

**Act:** Commit: `feat: 增加服务端普通用户登录和自助接口`

## PDCA 4: 网关按用户账户池路由

**Plan:** 服务端 token 请求绑定到用户后，候选只来自该用户分配池，并应用用户级顺序、激活、锁定；全局健康、冷却和 capacity 逻辑保留。

**Files:**
- Modify: `backend/internal/api/server_gateway_auth.go`
- Modify: `backend/internal/api/gateway_handler.go`
- Modify: `backend/internal/api/responses_handler.go`
- Modify: `backend/internal/api/usage_events.go`
- Test: `backend/internal/api/server_gateway_auth_test.go`
- Test: `backend/internal/api/gateway_handler_test.go`
- Test: `backend/internal/api/responses_handler_test.go`

**Do:**
1. 写失败测试：
   - 无分配账号的合法 token 请求返回 `403 no upstream accounts assigned to this user`。
   - 用户 A 只会请求 A 分配池中的上游，用户 B 可共享同一上游但拥有独立 active/locked/order。
   - 用户级锁定跳过该账号，不影响另一个用户。
   - 使用事件仍记录 `server_user_id`。
   - `/responses` 和 `/chat/completions` 行为一致。
2. Run red:
   - `cd backend && go test ./internal/api -run 'Gateway.*Assigned|Responses.*Assigned|ServerGateway' -count=1`
3. 最小实现：
   - handler 注入 `serverusers.Repository` 或窄接口。
   - 构建候选前查询用户分配池并 overlay 用户级状态到 `accounts.Account` 的 `Priority/IsActive/IsLocked`。
   - 无池返回 403，池存在但候选都失败仍走现有 502。
4. Run green:
   - `cd backend && go test ./internal/api -run 'Gateway.*Assigned|Responses.*Assigned|ServerGateway' -count=1`

**Check:** 网关仍保持薄转发语义，不合成上游协议行为。

**Act:** Commit: `feat: 按服务端用户账户池路由`

## PDCA 5: 管理员前端账户池分配

**Plan:** 在现有“服务用户”页面给管理员增加账号池分配入口，不改变普通客户端账号页。

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/features/server-users/ServerUsersPage.tsx`
- Test: `frontend/src/features/server-users/ServerUsersPage.test.tsx`

**Do:**
1. 写失败测试：
   - 表格显示已分配账号数量。
   - 点击分配打开账号选择弹窗。
   - 默认新用户无选中账号。
   - 保存调用 `PUT /server-users/{id}/accounts`。
2. Run red:
   - `npm --prefix frontend test -- ServerUsersPage`
3. 最小实现类型、API wrapper 和 UI。
4. Run green:
   - `npm --prefix frontend test -- ServerUsersPage`

**Check:** 页面没有把普通用户可编辑的顺序/激活状态误写到全局账号。

**Act:** Commit: `feat: 增加服务用户账户池分配界面`

## PDCA 6: 普通用户受限前端

**Plan:** 服务端 WebUI 登录页支持管理员密码登录和普通用户登录；普通用户进入后只看到自己的用量和被分配账号池，可调整顺序、激活和锁定，不能看到配置型页面和新增账号按钮。

**Files:**
- Modify: `frontend/src/lib/api.ts`
- Modify: `frontend/src/App.tsx`
- Create: `frontend/src/features/server-users/UserPoolPage.tsx`
- Test: `frontend/src/App.test.tsx` if existing
- Test: `frontend/src/features/server-users/UserPoolPage.test.tsx`

**Do:**
1. 写失败测试：
   - 登录页切换到普通用户登录时提交用户名和 token。
   - 普通用户 session 只显示受限导航。
   - 普通用户页面显示自己的 usage summary 和账号池。
   - 调整顺序、激活、锁定调用 `/me/accounts/*`，不调用 `/accounts/*`。
2. Run red:
   - `npm --prefix frontend test -- App UserPoolPage`
3. 最小实现 API、登录状态、受限导航和账号池列表。
4. Run green:
   - `npm --prefix frontend test -- App UserPoolPage`

**Check:** 普通用户看不到账户配置、删除、导入、设置、服务用户管理。

**Act:** Commit: `feat: 增加服务端普通用户控制台`

## PDCA 7: 闭环验证、浏览器确认和打包

**Plan:** 使用后端、前端、server-dev、浏览器和 msvc 打包 harness 验证整体功能闭环。

**Files:**
- Modify docs only if behavior or commands changed.

**Do:**
1. Run backend full:
   - `cd backend && go test ./...`
2. Run frontend:
   - `npm --prefix frontend test`
   - `npm --prefix frontend run build -- --mode server`
3. Run packaging test:
   - `bash scripts/test/build_ai_gate_msvc_test.sh`
4. Start server-dev:
   - `make server-dev`
5. Browser check:
   - Open `http://127.0.0.1:6791/ai-gate/webui/`
   - Admin login with configured password.
   - Create user, confirm no default account pool.
   - Assign accounts to user.
   - Logout, ordinary user login with username/token.
   - Confirm only self usage and account pool page is visible.
   - Toggle user-level active/lock/order and confirm admin/global account page is unchanged.
6. Optional curl gateway smoke with a stub or existing configured upstream:
   - Valid token and no pool returns 403.
   - Valid token with pool reaches only assigned candidate.
   - Invalid token returns 401.

**Check:** 功能闭环并且当前客户端/桌面模式仍能构建和测试。

**Act:** Commit: `test: 验证服务端用户账户池闭环` or docs/test adjustment commit if needed.
