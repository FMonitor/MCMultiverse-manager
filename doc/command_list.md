# Command List (MCMM)

## Scope
- `Bungee大厅`：申请流、模板查看、实例全局查看。
- `MiniServer`：世界管理（访问模式、成员、删除）。

## Request Commands (`/mcmm req ...`)

| 指令 | 权限 | 说明 |
| --- | --- | --- |
| `/mcmm req create <world_alias> [template_id\|template_name]` | 玩家 | 创建世界申请。模板可选；不填时走空世界流程。最终别名会写成 `<player>_<world_alias>`。 |
| `/mcmm req list` | 玩家 | 普通玩家看自己的请求，OP 看 pending 请求。显示短号 `#<id>`。 |
| `/mcmm req approve <request_no\|request_id>` | OP | 审批通过。 |
| `/mcmm req reject <request_no\|request_id> [reason]` | OP | 审批拒绝。 |
| `/mcmm req cancel <request_no\|request_id> [reason]` | 申请人/OP | 取消请求。 |

说明：
- `request_no` 是 `user_requests.id`（自增短号，推荐日常使用）。
- `request_id` 是 UUID（幂等键，内部保留）。

## World Commands (`/mcmm world ...`)

| 指令 | 权限 | 说明 |
| --- | --- | --- |
| `/mcmm world list` | 玩家 | 列出自己可加入的世界（owner/member/public）。 |
| `/mcmm world <instance_id\|alias>` | 玩家 | 加入世界（短 id 或别名都可）。 |
| `/mcmm world info [instance_id\|alias]` | 玩家 | 查看世界信息。 |
| `/mcmm world on <instance_id\|alias>` | owner/OP | 启动世界容器。 |
| `/mcmm world off <instance_id\|alias>` | owner/OP | 关闭世界容器。 |
| `/mcmm world set <public\|privacy>` | owner/OP | 设置访问模式。 |
| `/mcmm world remove <instance_id\|alias>` | owner/OP | 删除（归档）世界，需二次确认。 |
| `/mcmm world <world_alias> add user <user>` | owner/OP | 添加成员。 |
| `/mcmm world <world_alias> remove user <user>` | owner/OP | 移除成员。 |
| `/mcmm player invite <player_name> <instance_id\|alias>` | owner/OP | 邀请玩家（写入 `instance_members`，支持离线玩家；需玩家已存在于数据库）。 |
| `/mcmm player reject <player_name> <instance_id\|alias>` | owner/OP | 取消邀请（从 `instance_members` 删除）。 |

二次确认：
- 第一步：`/mcmm world remove <id_or_alias>`
- 第二步：30 秒内 `/mcmm confirm`

## Other Commands

| 指令 | 权限 | 说明 |
| --- | --- | --- |
| `/mcmm template list` | 玩家 | 列模板（含 `#id:tag (version)`）。 |
| `/mcmm instance list` | OP | 列出所有实例。 |
| `/mcmm instance create <world_alias> [template_id\|template_name]` | OP | 直接创建实例（绕过申请）。 |
| `/mcmm instance on <instance_id\|alias>` | OP | 启动任意实例容器。 |
| `/mcmm instance off <instance_id\|alias>` | OP | 关闭任意实例容器。 |
| `/mcmm instance stop <instance_id\|alias>` | OP | 兼容别名，等同于 `instance off`。 |
| `/mcmm instance remove <instance_id\|alias>` | OP | 归档并下线实例。 |
| `/mcmm instance lockdown <instance_id\|alias>` | OP | 锁定实例（仅 OP 可加入）。 |
| `/mcmm instance unlock <instance_id\|alias>` | OP | 解除锁定（恢复为 `privacy`）。 |
| `/mcmm confirm` | 玩家 | 确认删除。 |
| `/mcmm help` | 玩家 | 显示帮助。 |

## Backend Action Mapping

| action | 指令 |
| --- | --- |
| `request_create` | `req create` |
| `request_list` | `req list` |
| `request_approve` | `req approve` |
| `request_reject` | `req reject` |
| `request_cancel` | `req cancel` |
| `world_list` | `world list` |
| `world_info` | `world info` |
| `world_on` | `world on` |
| `world_off` | `world off` |
| `world_set_access` | `world set` |
| `world_remove` | `world remove` |
| `member_add` | `world <alias> add user` |
| `member_remove` | `world <alias> remove user` |
| `player_invite` | `player invite` |
| `player_reject` | `player reject` |
| `template_list` | `template list` |
| `instance_list` | `instance list` |
| `instance_create` | `instance create` |
| `instance_on` | `instance on` |
| `instance_off` | `instance off` |
| `instance_stop` | `instance stop` |
| `instance_remove` | `instance remove` |
| `instance_lockdown` | `instance lockdown` |
| `instance_unlock` | `instance unlock` |
