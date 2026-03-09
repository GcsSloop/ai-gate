# 薄网关模式

## 目标

薄网关模式下，网关只承担四类职责：

- 官方账号认证与刷新
- 请求路由到官方上游
- 官方请求体、响应体、SSE 事件的透明透传
- 审计日志与运行观测

网关不承担本地代理编排、工具闭环、多轮补偿或第三方兼容适配。

## 保留能力

- `POST /ai-router/api/v1/responses`
- `GET /ai-router/api/v1/models`
- 官方账号切换
- 官方账号 token 刷新
- 基础运行记录与监控

## 禁用能力

以下能力依赖网关本地语义，在薄网关模式下必须禁用，而不是本地伪实现：

- 本地生成 `response_id`
- 用本地历史重放替代 `previous_response_id`
- `/responses` 回退到 `/chat/completions`
- 第三方 OpenAI 兼容供应商支持
- 本地拼装 `/v1/responses/{id}`
- 本地拼装 `/v1/responses/{id}/input_items`
- 本地模拟 `/v1/responses/{id}/cancel`
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

## 回滚

设置：

```bash
THIN_GATEWAY_MODE=false
```

然后重启后端，恢复旧兼容路径。
