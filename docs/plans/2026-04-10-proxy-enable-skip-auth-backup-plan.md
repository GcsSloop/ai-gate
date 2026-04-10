# Proxy Enable Missing Auth Backup Plan (2026-04-10)

## 背景

在 macOS 客户端开启代理时，系统会尝试备份 `~/.codex/auth.json`。当该文件不存在时，备份流程返回错误并阻塞代理开启，导致用户在网络切换或首次环境下无法正常启用代理。

## 根因

代理开启流程将 `auth.json` 视为必需备份文件，缺少“可选文件”语义。相关 API 在列出和恢复备份文件时也将 `auth.json` 缺失视作错误，形成联动失败。

## 目标

1. `proxy_enable` 在 `auth.json` 缺失时继续执行，不阻塞主流程。
2. 备份文件查询与恢复接口对缺失 `auth.json` 保持容错。
3. 保留已有备份机制与安全边界，不引入额外权限扩大。

## 变更范围

- `backend/internal/api/settings_handler.go`
  - 代理开启时将 `auth.json` 备份改为可选逻辑。
  - 备份文件列表与恢复流程跳过不存在的 `auth.json`，避免返回硬错误。
- `backend/internal/api/settings_handler_test.go`
  - 增加/更新回归测试，覆盖缺失 `auth.json` 时代理开启成功的场景。

## 测试策略

1. 目标测试：
   - `go test ./internal/api -run TestSettingsHandlerProxy -count=1`
2. 用例重点：
   - 缺失 `~/.codex/auth.json` 时，`/settings/proxy/enable` 返回成功。
   - 备份查询与恢复路径不因缺失 `auth.json` 失败。

## 风险与回滚

- 风险：如果未来某些流程强依赖 `auth.json`，跳过备份可能导致恢复后状态不完整。
- 缓解：仅在文件不存在时跳过，文件存在时仍按原逻辑备份/恢复；日志保留可观测性。
- 回滚：回滚到前一提交即可恢复严格依赖行为。

## 发布说明

该修复属于代理稳定性补丁，建议作为 `v1.2.11` 发布，附带说明“缺失 `auth.json` 不再阻塞代理开启”。
