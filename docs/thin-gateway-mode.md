# 薄网关模式

## 目标

当前版本只保留薄网关职责，网关只承担四类职责：

- 账号认证与刷新
- 请求路由到原生 `/responses` 上游
- 请求体、响应体、SSE 事件的透明透传
- 审计日志与运行观测

网关不承担本地代理编排、工具闭环、多轮补偿或协议兼容补偿。

## 保留能力

- `POST /ai-router/api/v1/responses`
- `GET /ai-router/api/v1/models`
- 官方账号切换
- 账号 token 刷新
- 原生支持 `/responses` 的第三方账号
- 基础运行记录与监控

## 禁用能力

以下能力依赖网关本地语义，已经被删除，而不是保留为本地伪实现：

- 本地生成 `response_id`
- 用本地历史重放替代 `previous_response_id`
- `/responses` 回退到 `/chat/completions`
- 本地拼装 `/v1/responses/{id}`
- 本地拼装 `/v1/responses/{id}/input_items`
- 本地模拟 `/v1/responses/{id}/cancel`
- 本地实现 `/v1/responses/input_tokens`
- 本地实现 `/v1/responses/compact`

## 协议原则

- 上游 `response_id` 是唯一可信语义来源。
- `previous_response_id` 原样转发给上游。
- 上游错误结构和状态码优先，网关只做最小映射。
- SSE 生命周期以上游为准，网关不注入额外代理语义。

## 存储原则

本地数据库只保留：

- 请求与运行审计
- 账号使用观测
- 监控页面所需摘要信息

本地数据库不再承担：

- 响应协议检索索引
- 对话恢复语义
- `response_id` 推导
- 多轮工具执行状态

## 账号要求

- 官方账号默认视为支持 `/responses`
- 第三方账号创建时默认声明支持 `/responses`
- 如果激活账号不支持 `/responses`，网关直接返回明确错误，不做回退
