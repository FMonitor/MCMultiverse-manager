# Command List (MCMM)

## Scope
- `Bungee大厅`：允许申请、审批、全局管理命令。
- `MiniServer`：禁止创建/审批，只保留本世界管理和返回大厅。

## World Request Flow

| 编号 | 指令 | 适用范围 | 权限 | 说明 |
| --- | --- | --- | --- | --- |
| 1 | `/mcmm world request <template_name> <world_alias>` | Bungee大厅 | 玩家 | 创建申请，返回 `request_id`，并通知在线 OP。 |
| 2 | `/mcmm world request list` | Bungee大厅 | 玩家 | 列出可见申请（至少包含 `request_id/actor/template/status`）。 |
| 3 | `/mcmm world request reject <request_id> [reason]` | Bungee大厅 | OP | 拒绝申请。 |
| 4 | `/mcmm world request approve <request_id>` | Bungee大厅 | OP | 批准申请并触发 worker 创建流程。 |
| 5 | `/mcmm world request cancel <request_id>` | Bungee大厅 | OP 或申请人 | 取消未完成申请。 |

返回建议：
- 成功：`[MCMM] OK: <action> request_id=<id>`
- 失败：`[MCMM] FAIL: <reason>`

## World Management

| 编号 | 指令 | 适用范围 | 权限 | 说明 |
| --- | --- | --- | --- | --- |
| 6 | `/mcmm world remove <instance_id>` | Bungee大厅, MiniServer | owner 或 OP | 删除世界，需二次确认。 |
| 7 | `/mcmm world list` | Bungee大厅, MiniServer | 玩家 | 列出当前玩家拥有的实例。 |
| 8 | `/mcmm world set <public\|privacy>` | MiniServer | owner 或 OP | 设置当前世界公开性。 |
| 9 | `/mcmm world info` | MiniServer | 玩家 | 显示当前世界信息（id/alias/owner/created/access/whitelist_count）。 |

二次确认规范（用于 `world remove`）：
- 第一步：`/mcmm world remove <id>`
- 插件提示：`[MCMM] Confirm remove <id> within 30s: /mcmm confirm`
- 第二步：`/mcmm confirm`

## Player Permission

| 编号 | 指令 | 适用范围 | 权限 | 说明 |
| --- | --- | --- | --- | --- |
| 10 | `/mcmm player invite <player> [world_alias]` | Bungee大厅, MiniServer | owner 或 OP | 添加白名单成员。 |
| 11 | `/mcmm player reject <player> [world_alias]` | Bungee大厅, MiniServer | owner 或 OP | 从白名单移除。 |
| 12 | `/mcmm player kick <player> [world_alias]` | Bungee大厅, MiniServer | owner 或 OP | 踢出在线玩家。 |
| 13 | `/mcmm player ban <player> [world_alias]` | Bungee大厅, MiniServer | OP | 封禁玩家（建议全局 ban，不按单世界）。 |

说明：
- 在 MiniServer 中省略 `[world_alias]` 时默认当前世界。
- `public` 世界执行 `invite/reject` 可返回“操作已记录，当前为公开模式”。

## Admin Advanced

| 编号 | 指令 | 适用范围 | 权限 | 说明 |
| --- | --- | --- | --- | --- |
| 14 | `/mcmm instance list` | Bungee大厅 | OP | 列出全部实例与状态。 |
| 15 | `/mcmm instance lockdown <instance_id>` | Bungee大厅 | OP | 锁定实例并踢出非 OP。 |
| 16 | `/mcmm instance create <template_name> <world_alias>` | Bungee大厅 | OP | 直建实例（绕过审批）。 |
| 17 | `/mcmm instance stop <instance_id>` | Bungee大厅 | OP | 停止实例容器并置为 `Off`。 |
| 18 | `/mcmm instance remove <instance_id>` | Bungee大厅 | OP | 归档并移除运行实例。 |

## Backend Mapping

建议统一入口：`POST /v1/cmd/world`，按 `action` 路由。

| action | 对应指令 |
| --- | --- |
| `request_create` | 1 |
| `request_list` | 2 |
| `request_reject` | 3 |
| `request_approve` | 4 |
| `request_cancel` | 5 |
| `world_remove` | 6 |
| `world_list` | 7 |
| `world_set_access` | 8 |
| `world_info` | 9 |
| `member_add` | 10 |
| `member_remove` | 11 |
| `player_kick` | 12 |
| `player_ban` | 13 |
| `instance_list` | 14 |
| `instance_lockdown` | 15 |
| `instance_create` | 16 |
| `instance_stop` | 17 |
| `instance_remove` | 18 |

## Status Machine

`map_instances.status` 只保留：
- `Waiting`
- `Preparing`
- `Starting`
- `On`
- `Stopping`
- `Off`
- `Archived`
