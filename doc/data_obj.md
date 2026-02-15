# Data Objects (PostgreSQL)

本文档用于对齐当前“插件命令驱动”方案，并作为 `deploy/init.sql` 的目标结构。

## 1. `users`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 用户主键。 |
| `mc_uuid` | `UUID` | `NOT NULL UNIQUE` | Minecraft UUID。 |
| `mc_name` | `TEXT` | `NOT NULL UNIQUE` | 玩家名（按当前唯一名处理）。 |
| `server_role` | `TEXT` | `NOT NULL DEFAULT 'user'` | 服务器级角色（`user/admin`）。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |

## 2. `map_templates`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 模板主键。 |
| `tag` | `TEXT` | `NOT NULL UNIQUE` | 模板标识（命令里 `<template name>`）。 |
| `display_name` | `TEXT` | `NOT NULL` | 展示名。 |
| `game_version` | `TEXT` | `NOT NULL` | MC 版本（如 `1.16.5`）。 |
| `blob_path` | `TEXT` | `NOT NULL` | 模板路径。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |

## 3. `server_images`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `TEXT` | PK | 运行时镜像标识（如 `runtime-java17`）。 |
| `name` | `TEXT` | `NOT NULL` | 展示名。 |
| `game_version` | `TEXT` | `NOT NULL` | 对应 MC 版本。 |

## 4. `map_instances`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 实例主键。 |
| `alias` | `TEXT` | `NOT NULL UNIQUE` | 世界别名（玩家看到的世界名）。 |
| `owner_id` | `BIGINT` | `NOT NULL FK -> users(id)` | 所有者。 |
| `template_id` | `BIGINT` | 可空 FK -> map_templates(id) | 来源模板。 |
| `source_type` | `TEXT` | `NOT NULL` | 来源（`template/upload/empty`）。 |
| `game_version` | `TEXT` | `NOT NULL` | 目标 MC 版本。 |
| `access_mode` | `TEXT` | `NOT NULL DEFAULT 'privacy'` | 访问模式（`privacy/public`）。 |
| `status` | `TEXT` | `NOT NULL` | 状态机状态。 |
| `health_status` | `TEXT` | `NOT NULL DEFAULT 'unknown'` | 健康状态（`unknown/healthy/start_failed/unreachable`）。 |
| `last_error_msg` | `TEXT` | 可空 | 最近一次失败原因。 |
| `last_health_at` | `TIMESTAMPTZ` | 可空 | 最近一次健康结果写入时间。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 最近更新时间。 |
| `last_active_at` | `TIMESTAMPTZ` | 可空 | 最近活跃时间。 |
| `archived_at` | `TIMESTAMPTZ` | 可空 | 归档时间。 |

状态机固定为 7 个：
- `Waiting`
- `Preparing`
- `Starting`
- `On`
- `Stopping`
- `Off`
- `Archived`

健康状态：
- `unknown`：尚未做过有效健康判定。
- `healthy`：容器启动并完成 ServerTap 初始化。
- `start_failed`：容器或启动流程失败。
- `unreachable`：容器已尝试启动，但 ServerTap 不可达/超时。

## 5. `instance_members`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 成员关系主键。 |
| `instance_id` | `BIGINT` | `NOT NULL FK -> map_instances(id)` | 实例 ID。 |
| `user_id` | `BIGINT` | `NOT NULL FK -> users(id)` | 用户 ID。 |
| `role` | `TEXT` | `NOT NULL` | 成员角色（`owner/member`）。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |

补充：
- `UNIQUE(instance_id, user_id)`。
- `public` 世界可允许非白名单进入，但白名单仍保留（用于切换回 `privacy`）。

## 6. `user_requests`

`user_requests` 统一承载“申请、审批、取消、幂等”。

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 记录主键。 |
| `request_id` | `UUID` | `NOT NULL UNIQUE` | 对外请求号（命令回显给玩家）。 |
| `request_type` | `TEXT` | `NOT NULL` | 请求类型（`world_create/world_remove/member_add/member_remove` 等）。 |
| `actor_user_id` | `BIGINT` | `NOT NULL FK -> users(id)` | 发起用户。 |
| `target_instance_id` | `BIGINT` | 可空 FK -> map_instances(id) | 目标实例。 |
| `template_id` | `BIGINT` | 可空 FK -> map_templates(id) | 申请创建时的模板。 |
| `requested_alias` | `TEXT` | 可空 | 申请创建时请求的世界名。 |
| `status` | `TEXT` | `NOT NULL` | `pending/approved/rejected/canceled/processing/succeeded/failed`。 |
| `reviewed_by_user_id` | `BIGINT` | 可空 FK -> users(id) | 审批人。 |
| `review_note` | `TEXT` | 可空 | 拒绝/取消原因。 |
| `response_payload` | `JSONB` | `NOT NULL DEFAULT '{}'` | 返回快照（例如实例 id）。 |
| `error_code` | `TEXT` | 可空 | 错误码。 |
| `error_msg` | `TEXT` | 可空 | 错误信息。 |
| `expires_at` | `TIMESTAMPTZ` | 可空 | 申请超时（可选）。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 更新时间。 |

说明：
- `id` 是内部主键。
- `request_id` 是对外可见请求号。

## 7. Go Mapping

对应文件：`internal/pgsql/sqlmodel_i.go`

- `User` -> `users`
- `MapTemplate` -> `map_templates`
- `ServerImage` -> `server_images`
- `MapInstance` -> `map_instances`
- `InstanceMember` -> `instance_members`
- `UserRequest` -> `user_requests`
