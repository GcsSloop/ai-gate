# 统计延迟卡片秒数展示设计

## 目标

统计页红框中的平均延迟、P95 延迟、P99 延迟和最大 / 最小延迟卡片统一以秒为单位展示，并固定保留 1 位小数。最近记录列表中的延迟展示保持现状，不纳入本次改动。

## 方案

复用 `frontend/src/features/stats/StatsPage.tsx` 现有的 `formatLatency` helper：将后端返回的毫秒值除以 1000，再使用 `Intl.NumberFormat` 配置 `minimumFractionDigits: 1` 和 `maximumFractionDigits: 1`，最后追加 `s` 单位。四张质量卡片继续通过同一个 helper 渲染，确保单值和最大 / 最小组合一致。

## 测试与发布

在 `StatsPage.test.tsx` 中补充卡片展示断言，覆盖毫秒到秒的换算、固定 1 位小数和最大 / 最小组合，同时确认最近记录仍显示毫秒。发布前运行前端测试、生产构建、发布脚本 harness，以及 `release_no_apple_signing_test.sh`，确认 GitHub Actions CI 没有 Apple 账号签名、公证或证书导入逻辑。Apple 签名辅助脚本仅供本地 macOS 打包使用，不属于本次 CI 变更范围。
