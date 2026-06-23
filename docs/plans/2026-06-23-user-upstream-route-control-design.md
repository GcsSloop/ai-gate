# 用户上游路由控制设计

## 目标

普通服务用户通过 AI Gate server 版本登录后，可以在“我的网关”页面查看所有上游账号的脱敏基础状态和用量摘要，并能手动切换或锁定自己的上游路由。

## 架构

服务用户路由状态是用户级状态，不复用全局 `accounts.is_active` 或 `accounts.is_locked`，避免一个用户的切换影响其他用户。`server_users` 新增 `preferred_account_id` 和 `route_locked` 两个字段；转发时这些字段随现有 token 鉴权查询进入 request context，不新增热路径 DB 查询。

服务端新增 `/api/me/upstreams` 和 `/api/me/route`。前者返回账号名、供应商、base_url、状态、可用性、用量摘要、当前使用中账号和总数统计，不返回 `credential_ref`、`usage_config_json` 或任何 token。后者只在用户手动切换、锁定、解锁时写库，并同步更新进程内 sticky selector，让后续转发立即生效。

## 路由规则

- 默认：继续使用全局最优路径加用户级 sticky。
- 手动切换：将指定账号写入用户偏好，并为该用户的 chat/responses scope 写入 sticky；后续请求优先命中该账号。
- 锁定：在用户偏好存在时，每次排序都把指定账号放在首位；如果该账号不可尝试，仍允许跳过并由后续候选兜底。
- 失败：限流、容量不足、上游不可达时失效该用户对应 scope 的 sticky，不影响其他用户。

## 前端

`UserPoolPage` 在自助用量下方增加可折叠“上游账号”区域。折叠标题右侧显示“可用 / 总数”；展开后逐个展示账号名、base_url、状态、用量摘要和“当前使用中”标记，并提供“切换”“锁定/解锁”按钮。页面不展示任何上游 token 或配置 JSON。

## 验证

- 后端：`cd backend && go test ./internal/api ./internal/serverusers ./internal/routing -count=1`
- 前端：`npm --prefix frontend test -- UserPoolPage`
- 构建：`npm --prefix frontend run build`
