# Data Objects (PostgreSQL)

本文档与 `deploy/init.sql` 一致。

## 1. `schema_migrations`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `version` | `TEXT` | PK | 迁移版本号。 |
| `applied_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 应用时间。 |

## 2. `users`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 用户主键。 |
| `mc_uuid` | `UUID` | `NOT NULL UNIQUE` | Minecraft UUID。 |
| `mc_name` | `TEXT` | `NOT NULL` | 玩家名。 |
| `server_role` | `TEXT` | `NOT NULL DEFAULT 'user'` | 服务器级角色（`user/admin`）。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |

## 3. `map_templates`

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

## 4. `map_instances`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 实例主键。 |
| `owner_id` | `BIGINT` | `NOT NULL FK -> users(id)` | 所有者。 |
| `template_id` | `BIGINT` | 可空 FK -> map_templates(id) | 来源模板。 |
| `source_type` | `TEXT` | `NOT NULL` | 来源（`template/upload`）。 |
| `internal_name` | `TEXT` | `NOT NULL UNIQUE` | 内部世界名。 |
| `alias` | `TEXT` | `NOT NULL UNIQUE` | 玩家入口别名。 |
| `status` | `TEXT` | `NOT NULL` | 状态机状态。 |
| `storage_type` | `TEXT` | `NOT NULL` | 存储层（`ssd/hdd`）。 |
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
| `role` | `TEXT` | `NOT NULL` | 成员角色（`owner/admin/member`）。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |

补充：`UNIQUE(instance_id, user_id)`。

## 6. `load_tasks`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 任务主键。 |
| `instance_id` | `BIGINT` | `NOT NULL FK -> map_instances(id)` | 关联实例。 |
| `status` | `TEXT` | `NOT NULL` | 状态（`pending/running/completed/failed`）。 |
| `error_code` | `TEXT` | 可空 | 错误码。 |
| `error_msg` | `TEXT` | 可空 | 错误信息。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 入队时间。 |
| `started_at` | `TIMESTAMPTZ` | 可空 | 开始时间。 |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 更新时间。 |
| `finished_at` | `TIMESTAMPTZ` | 可空 | 结束时间。 |

## 7. `audit_log`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `BIGSERIAL` | PK | 审计主键。 |
| `actor_user_id` | `BIGINT` | 可空 FK -> users(id) | 操作者。 |
| `instance_id` | `BIGINT` | 可空 FK -> map_instances(id) | 关联实例。 |
| `action` | `TEXT` | `NOT NULL` | 动作类型。 |
| `description` | `TEXT` | `NOT NULL DEFAULT ''` | 简述。 |
| `payload_json` | `JSONB` | `NOT NULL DEFAULT '{}'` | 结构化上下文。 |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL DEFAULT NOW()` | 创建时间。 |

## 8. `user_requests` (Idempotency)

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

## 9. Go Mapping

对应文件：`internal/pgsql/sqlmodel_i.go`

- `User` -> `users`
- `MapTemplate` -> `map_templates`
- `MapInstance` -> `map_instances`
- `InstanceMember` -> `instance_members`
- `LoadTask` -> `load_tasks`
- `AuditLog` -> `audit_log`
- `UserRequest` -> `user_requests`
- `SchemaMigration` -> `schema_migrations`
