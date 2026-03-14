# 真实快照清理迁移记录

## 目的

验证当前“旧库清理迁移”逻辑在真实数据库上的实际效果，而不是只看合成数据。

本次验证只在数据库副本上进行，不触碰正在运行的正式库。

## 为什么使用一致性快照

直接对正在写入的 SQLite 文件做 `cp`，容易得到页级不一致的副本。这样的副本有时能读元数据，但扫全表时会报 `database disk image is malformed`。

因此本次使用 SQLite 自带的 `.backup` 生成一致性快照。

## 验证步骤

### 1. 生成真实库一致性快照

源库：

- `~/.aigate/data/aigate.sqlite`

快照副本：

- `/tmp/aigate-real-consistent.sqlite`

使用命令：

```bash
sqlite3 ~/.aigate/data/aigate.sqlite ".backup /tmp/aigate-real-consistent.sqlite"
```

### 2. 在副本上统计真实分布

先统计表级体积，再统计 `messages` 的行数、`item_type` 分布和 `storage_mode` 分布。

### 3. 在副本上执行当前启动迁移

迁移方式不是手工删表，而是走当前代码中的启动逻辑：

- 调用 `bootstrap.NewApp(...)`
- 由 `cleanupLegacyAuditData(...)` 完成清理

迁移后的副本：

- `/tmp/aigate-real-migrated.sqlite`

### 4. 对比迁移前后结果

对比内容包括：

- 文件总大小
- 主要表体积
- 审计表残留行数
- 保留表行数

## 真实库迁移前状态

数据库总大小：

- `3,419,820,032 bytes`

主要表体积：

- `messages`: `3,419,279,360 bytes`
- `conversations`: `274,432 bytes`
- `runs`: `172,032 bytes`
- `account_usage_snapshots`: `40,960 bytes`

主要行数：

- `messages`: `959,210`
- `conversations`: `3,171`
- `runs`: `3,124`
- `account_usage_snapshots`: `277`

结论：

- 体积几乎全部来自 `messages`
- `conversations` 和 `runs` 的空间占用可以忽略

## 真实库字段分布

`messages.item_type` 分布如下：

| item_type | 行数 | 占比 | avg(raw_item_json) | avg(content) |
| --- | ---: | ---: | ---: | ---: |
| `message` | 276,479 | 28.82% | 1628.3 | 514.0 |
| `function_call` | 236,274 | 24.63% | 339.7 | 0.0 |
| `function_call_output` | 236,246 | 24.63% | 4463.3 | 3163.9 |
| `reasoning` | 152,848 | 15.93% | 2274.0 | 0.0 |
| `custom_tool_call_output` | 25,797 | 2.69% | 286.8 | 191.8 |
| `custom_tool_call` | 25,797 | 2.69% | 3235.4 | 0.0 |
| `web_search_call` | 5,761 | 0.60% | 257.7 | 0.0 |

`messages.storage_mode` 分布如下：

- `full`: `916,946`
- `summary`: `42,264`

结论：

- 旧库的大头不是表数量，而是高体积的明细载荷
- 压缩逻辑只覆盖了少量数据，大部分记录仍然保留完整内容

## 每次请求的平均放大量

基于真实快照中的 `runs` 和 `messages` 统计：

- `runs = 3,124`
- `messages = 959,210`
- 平均每个 `run` 对应 `307.05` 条 `messages`
- 仅 `messages` 表平均每个 `run` 占用 `1,094,519.64 bytes`

结论：

- 在旧模型下，请求级审计会产生极高的放大效应
- 这也是数据库快速膨胀到 GB 级的核心原因

## 迁移执行结果

迁移后的副本：

- `/tmp/aigate-real-migrated.sqlite`

迁移后数据库总大小：

- `114,688 bytes`

对比结果：

- 迁移前：`3,419,820,032 bytes`
- 迁移后：`114,688 bytes`
- 缩小倍数：`29,818.5x`
- 体积下降：`99.996646%`

迁移后表体积：

- `account_usage_snapshots`: `40,960 bytes`
- `accounts`: `12,288 bytes`
- `usage_events`: `4,096 bytes`
- `maintenance_state`: `4,096 bytes`

迁移后行数：

- `messages`: `0`
- `conversations`: `0`
- `runs`: `0`
- `usage_events`: `0`
- `maintenance_state`: `1`
- `account_usage_snapshots`: `277`
- `accounts`: `5`

## 如何理解这个结果

这次验证跑的是“旧库清理迁移”，不是“历史审计数据转 usage_events 的回填迁移”。

因此结果符合当前设计：

- 清空旧审计表
- 保留账户、快照、设置等轻量数据
- 写入一条维护标记，避免重复清理
- 不从旧审计记录中生成历史 `usage_events`

所以迁移后 `usage_events` 为 `0` 是正常结果，不是迁移失败。

## 最终结论

真实库验证说明了三件事：

1. 当前数据库体积的核心来源就是 `messages` 审计明细
2. 只要停止保留这类请求级明细，数据库会立刻回到非常轻的量级
3. 当前的清理迁移对真实旧库有效，而且瘦身幅度远大于合成测试

这也说明“移除持续审计持久化，改为轻量 usage 事件”的方向是正确的。
