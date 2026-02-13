# Data Objects (PostgreSQL)

本文档与 `deploy/init.sql` 一致。

## 1. `users`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 用户主键。 |
| `mc_uuid` | `UUID` | `NOT NULL UNIQUE` | Minecraft UUID。 |
| `mc_name` | `TEXT` | `NOT NULL` | 玩家名。 |
| `server_role` | `TEXT` | `NOT NULL DEFAULT 'user'` | 服务器级角色（`user/admin`）。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |

## 2. `map_templates`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 模板主键。 |
| `tag` | `TEXT` | `NOT NULL UNIQUE` | 模板标识。 |
| `display_name` | `TEXT` | `NOT NULL` | 展示名。 |
| `version` | `TEXT` | `NOT NULL` | 模板版本。 |
| `game_version` | `TEXT` | `NOT NULL` | MC 版本。 |
| `size_bytes` | `BIGINT` | `NOT NULL CHECK(size_bytes >= 0)` | 包大小。 |
| `sha256_hash` | `TEXT` | `NOT NULL` | 内容哈希。 |
| `blob_path` | `TEXT` | `NOT NULL` | 存储路径。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |

## 3. `server_images`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `TEXT` | PK | 服务端唯一标识（如 `s1`）。 |
| `name` | `TEXT` | `NOT NULL` | 展示名。 |
| `game_version` | `TEXT` | `NOT NULL` | 服务端 Minecraft 版本。 |

## 4. `map_instances`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 实例主键。 |
| `owner_id` | `BIGINT` | `NOT NULL FK -> users(id)` | 所有者。 |
| `template_id` | `BIGINT` | 可空 FK -> map_templates(id) | 来源模板。 |
| `source_type` | `TEXT` | `NOT NULL` | 来源（`template/upload`）。 |
| `game_version` | `TEXT` | `NOT NULL DEFAULT 'unknown'` | 当前实例目标 MC 版本。 |
| `status` | `TEXT` | `NOT NULL` | 状态机状态（可含 `archived`）。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 最近更新时间。 |
| `last_active_at` | `TIMESTAMPTZ` | 可空 | 最近活跃时间。 |
| `archived_at` | `TIMESTAMPTZ` | 可空 | 归档时间。 |

## 5. `instance_members`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 成员关系主键。 |
| `instance_id` | `BIGINT` | `NOT NULL FK -> map_instances(id)` | 实例 ID。 |
| `user_id` | `BIGINT` | `NOT NULL FK -> users(id)` | 用户 ID。 |
| `role` | `TEXT` | `NOT NULL` | 成员角色（`owner/member`）。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |

补充：`UNIQUE(instance_id, user_id)`。

## 6. `user_requests` (Idempotency)

`user_requests` 是幂等请求表，名字保持简短。

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 记录主键。 |
| `request_id` | `UUID` | `NOT NULL UNIQUE` | 幂等键。 |
| `request_type` | `TEXT` | `NOT NULL` | 请求类型（`create_instance/delete_instance/archive_instance/restore_instance`）。 |
| `actor_user_id` | `BIGINT` | 可空 FK -> users(id) | 发起用户。 |
| `target_instance_id` | `BIGINT` | 可空 FK -> map_instances(id) | 目标实例。 |
| `status` | `TEXT` | `NOT NULL` | 处理状态（`accepted/processing/succeeded/failed`）。 |
| `response_payload` | `JSONB` | `NOT NULL DEFAULT '{}'` | 成功或失败响应快照。 |
| `error_code` | `TEXT` | 可空 | 错误码。 |
| `error_msg` | `TEXT` | 可空 | 错误信息。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 更新时间。 |

补充：`id` 与 `request_id` 不重复语义。
- `id`：数据库内部主键（自增，便于 join/排序）。
- `request_id`：业务幂等键（客户端传入或后端生成，要求唯一）。

## 7. Go Mapping

对应文件：`internal/pgsql/sqlmodel_i.go`

- `User` -> `users`
- `MapTemplate` -> `map_templates`
- `ServerImage` -> `server_images`
- `MapInstance` -> `map_instances`
- `InstanceMember` -> `instance_members`
- `UserRequest` -> `user_requests`
