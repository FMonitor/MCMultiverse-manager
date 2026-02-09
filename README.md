# Minecraft 地图管理系统（Map Management System）设计文档

> 目标：为 Minecraft 服务器提供“地图/世界”的**创建、导入、权限、传送、回收**的全流程管理能力。
>
> 关键词：Paper / Multiverse / LuckPerms / ServerTap / 世界模板仓库 / 副本实例 / 共享世界 / 任务队列 / 可观测性

---

## 1. 背景与目标

### 1.1 背景

Minecraft 服务器中，“世界（world）”既是游戏内容载体，也是最重的资源对象：

* 体积大（数百 MB ~ 数 GB）
* 导入/加载耗时长
* 生命周期多样（长期共享、短期副本、一次性线性地图）
* 权限边界复杂（管理员、创建者、朋友、访客）

因此需要一个统一的地图管理系统，使玩家能够通过 Web 页面选择地图或上传地图，并在游戏内快速进入，同时保证权限与资源安全。

### 1.2 目标

系统需要实现：

* **地图模板（Map Preset）管理**：预置地图的存储、版本、元数据
* **世界实例（World Instance）管理**：基于模板创建副本、创建共享世界
* **导入与加载**：自动将世界导入 Paper 服务器并可进入
* **权限管理**：基于 LuckPerms，为创建者/管理员赋权
* **游戏内传送体验**：玩家使用简短 alias 进入世界
* **任务队列与幂等**：导入任务可重试、可审计
* **存储分层**：SSD 热存储 + HDD 冷存储

### 1.3 非目标（MVP 不做）

* 多服务器集群调度（单机先跑通）
* 世界快照/增量存储（先用全量复制）
* 世界内细粒度区域权限（例如 WorldGuard 区域）

---

## 2. 需求与用例

### 2.1 世界生命周期

系统支持两种世界类型：

#### A. 共享世界（Shared World）

* 玩家上传/注册一个世界（或从模板创建）
* 世界长期存在
* 玩家可邀请朋友

#### B. 副本/临时世界（Instance / Ephemeral）

* 从模板创建副本
* 用于线性地图或短期游玩
* 到期自动回收/删除

### 2.2 主要用例

* 玩家在 Web 选择模板创建世界实例
* 玩家在 Web 上传世界（后续阶段）
* 玩家在游戏内使用 alias 进入世界
* 管理员查看/删除世界
* 系统自动回收长期无人使用的临时世界

---

## 3. 总体架构

### 3.1 架构图（逻辑）

* 前端（Web UI）

  * 登录/绑定 Minecraft UUID
  * 地图模板列表
  * 世界实例列表与状态
  * 创建/删除/归档

* 后端（Core Orchestrator）

  * API（REST）
  * 数据库（用户、模板、实例、任务、审计）
  * 任务队列（导入、删除、回收）
  * 存储管理（SSD/HDD）
  * 与 Minecraft 服务器交互（ServerTap）

* Minecraft 服务端（Paper）

  * Multiverse-Core（世界导入与管理）
  * LuckPerms（权限）
  * （可选）自研 WorldAlias/WorldMenu 插件（优化入口体验）
  * ServerTap（提供 REST API，后端执行命令）

---

## 4. 技术选型

### 4.1 服务端

* Paper（推荐）
* Multiverse-Core（世界管理）
* LuckPerms（权限管理）
* ServerTap（REST API 执行命令）

### 4.2 后端

* 推荐：Python（FastAPI）或 Node.js（NestJS）
* 数据库：PostgreSQL（推荐）或 SQLite（MVP 可用）
* 任务队列：

  * MVP：内置队列（单进程）
  * 稳定版：Redis + RQ/Celery/BullMQ

### 4.3 存储

* 热存储：1TB SSD
* 冷存储：8TB HDD
* 文件格式：

  * 模板/归档：`tar.zst`（推荐）或 `zip`
  * 元数据：JSON

---

## 5. 目录布局与存储策略

### 5.1 服务器根目录约束

Multiverse 导入世界时通常要求 world folder 位于 server root 下（`/mv import <worldName>` 仅使用名称，不支持路径）。因此系统需要在导入前将世界目录 materialize 到 server root。

### 5.2 推荐目录布局

假设 Minecraft server root：`/mcserver/`

* `/mcserver/`

  * `paper.jar`
  * `plugins/`
  * `world/`（主世界）
  * `world_nether/`
  * `world_the_end/`
  * `i_12345/`（系统导入的世界实例目录）
  * `i_12346/`
  * `staging/`（解压/校验临时目录，不由 MV 管理）

系统数据目录：`/data/map-system/`

* `templates/`（模板缓存）
* `instances/`（归档实例）
* `uploads/`（用户上传暂存）
* `blobs/`（模板/世界包 blob）
* `logs/`

### 5.3 热/冷策略

* SSD：

  * 活跃世界实例目录（server root）
  * staging 解压目录
  * 热门模板缓存

* HDD：

  * 模板仓库 blob
  * 冷实例归档（tar.zst）

---

## 6. 命名规范（非常重要）

### 6.1 内部世界名（internalName）

内部世界名用于：

* 目录名
* Multiverse world name
* 后端与服务端交互

规范：

* `i_<instanceId>`
* 示例：`i_10452`

要求：

* 全局唯一
* 只包含 `[a-z0-9_]+`
* 永不改变

### 6.2 对外 alias（玩家入口名）

玩家使用 alias 进入世界。

规范建议：

* `safeUsername-<index>`
* 示例：`lcmonitor-0`、`lcmonitor-1`

规则：

* 仅允许 `[a-z0-9_-]{1,24}`
* 全部转小写
* alias 可修改（内部名不变）

### 6.3 模板 tag

模板用于标识地图类型：

* `skyblock`
* `portal-escape`
* `ctm-01`

---

## 7. 数据模型（数据库）

### 7.1 Users

* `id` (PK)
* `mc_uuid` (unique)
* `mc_name`
* `created_at`
* `last_seen_at`

### 7.2 MapPresets (Templates)

* `id` (PK)
* `tag` (unique)
* `display_name`
* `version`（语义化版本或递增）
* `description`
* `size_bytes`
* `digest`（sha256）
* `blob_path`（在 HDD/对象存储的位置）
* `created_at`

### 7.3 WorldInstances

* `id` (PK)
* `instance_type`（shared / ephemeral / persistent）
* `owner_user_id` (FK)
* `template_id` (FK, nullable)
* `internal_name`（例如 `i_12345`）
* `alias`（例如 `lcmonitor-0`）
* `status`（见状态机）
* `storage_tier`（ssd/hdd）
* `created_at`
* `expires_at`（临时世界）
* `last_active_at`

### 7.4 InstanceMembers

* `id` (PK)
* `instance_id` (FK)
* `user_id` (FK)
* `role`（owner / member / visitor）

### 7.5 Tasks (Jobs)

* `id` (PK)
* `task_type`（create/import/delete/archive/restore）
* `instance_id` (FK)
* `status`（queued/running/success/failed）
* `error_code`
* `error_message`
* `created_at`
* `started_at`
* `finished_at`

### 7.6 AuditLog

* `id` (PK)
* `actor_user_id`（nullable，系统任务为空）
* `action`
* `instance_id`（nullable）
* `payload_json`
* `created_at`

---

## 8. 世界状态机

推荐状态：

* `REQUESTED`
* `QUEUED`
* `FETCHING_TEMPLATE`
* `EXTRACTING`
* `MATERIALIZING_TO_ROOT`
* `IMPORTING`
* `CONFIGURING_PERMS`
* `READY`
* `FAILED`
* `DELETING`
* `ARCHIVED`

原则：

* 每一步可重试
* 失败时必须可清理（staging 与 root 残留）
* 每个状态写入 AuditLog

---

## 9. 后端与 Minecraft 交互设计

### 9.1 交互方式

MVP 采用 ServerTap：

* 后端通过 HTTP 调用 ServerTap
* ServerTap 执行命令并返回结果
* ServerTap 监听 `127.0.0.1` 或内网端口

### 9.2 命令白名单

后端不得执行任意命令，必须白名单：

* Multiverse：`mv import`、`mv remove`、`mv unload`、`mv load`、`mv tp`
* LuckPerms：`lp user`、`lp group`、`lp context`（必要子集）
* 传送入口插件命令（若存在）

### 9.3 幂等策略

* `createInstance` 请求必须携带 `request_id`（UUID）
* 后端记录 request_id -> instance_id
* 重复请求返回同一 instance

---

## 10. 世界导入流程（从模板创建副本）

### 10.1 流程概述

1. 前端请求创建实例（template + alias）
2. 后端创建 WorldInstances 记录（status=REQUESTED）
3. 入队 Task(create)
4. Worker 执行：

   * 拉取模板 blob（HDD -> SSD cache）
   * staging 解压
   * 校验大小/文件数/hash
   * 移动到 server root，目录名=internal_name
   * 执行 `/mv import <internal_name> normal`
   * 执行权限配置（LuckPerms）
   * 标记 READY

### 10.2 staging 校验规则（建议）

* 解压后总大小限制（例如 5GB）
* 文件数限制（例如 200k）
* 禁止符号链接
* 只允许特定文件结构（region, level.dat 等）

---

## 11. 世界删除与回收

### 11.1 临时世界回收

* expires_at 到期
* 或 last_active_at 超过阈值（例如 7 天）

回收策略：

* 直接删除（最省空间）
* 或归档为 tar.zst 到 HDD（保留历史）

### 11.2 删除流程

1. `/mv unload <internal_name>`
2. `/mv remove <internal_name>`（若支持）
3. 删除 server root 下目录
4. 标记 ARCHIVED 或 DELETED

---

## 12. 权限设计（LuckPerms）

### 12.1 角色

* Admin（服务器管理员）
* Owner（世界创建者）
* Member（被邀请玩家）
* Visitor（只读/旁观）

### 12.2 权限原则

* 不给 Owner 真正 OP（除非完全信任）
* 尽可能使用 LuckPerms 的 context（世界维度上下文）进行 scoped 权限

示例策略：

* Owner：

  * 允许在该世界放置/破坏/使用容器
  * 允许邀请成员
* Member：

  * 允许游玩
* Visitor：

  * 只允许进入与观光

---

## 13. 游戏内体验设计

### 13.1 入口命令

玩家不需要使用 internal_name。

推荐入口：

* `/world list`
* `/world go <alias>`
* `/world invite <player>`

实现方式：

* MVP：使用 Multiverse alias（如果稳定可用）
* 稳定版：自研轻量插件 WorldAlias/WorldMenu

### 13.2 alias 分配策略

* 每个用户维护 `next_world_index`
* 默认 alias：`<safeUsername>-<index>`
* 用户可修改 alias（校验规则同上）

---

## 14. API 设计（后端）

### 14.1 用户与绑定

* `POST /api/v1/link/request`：请求绑定码
* `POST /api/v1/link/confirm`：确认绑定

（玩家在游戏内执行 `/link <code>`，由插件或命令接口完成）

### 14.2 模板

* `GET /api/v1/templates`
* `GET /api/v1/templates/{tag}`

### 14.3 世界实例

* `POST /api/v1/instances`（template_id + type + alias）
* `GET /api/v1/instances`（我的世界列表）
* `GET /api/v1/instances/{id}`
* `POST /api/v1/instances/{id}/teleport`（可选）
* `DELETE /api/v1/instances/{id}`

### 14.4 管理员

* `GET /api/v1/admin/instances`
* `POST /api/v1/admin/templates`

---

## 15. 安全设计

### 15.1 交互安全

* ServerTap 仅监听 `127.0.0.1`
* 后端与 ServerTap 使用 token
* 后端命令白名单

### 15.2 文件安全

* staging 解压限制
* 禁止 symlink
* 目录名只使用 internal_name

### 15.3 权限安全

* 不允许普通用户触发“任意命令”
* 删除/导入等重操作需要审计

---

## 16. 可观测性与运维

### 16.1 日志

* task_id 贯穿整个导入流程
* 每个阶段写入 structured log（JSON）

### 16.2 指标

* 世界导入耗时
* 导入失败率
* staging 解压耗时
* SSD/HDD 空间使用
* 队列长度

### 16.3 告警

* SSD 剩余 < 10%
* 导入失败连续 N 次
* 任务堆积超过阈值

---

## 17. 开发里程碑

### Milestone 1：最小闭环（MVP）

* 模板列表
* 创建实例（从模板）
* 导入 + READY
* alias 入口（可先用 MV alias）

### Milestone 2：权限与邀请

* LuckPerms 角色
* 邀请成员

### Milestone 3：回收与归档

* 临时世界 TTL
* 自动删除/归档

### Milestone 4：上传地图

* Web 上传
* 校验 + 审核
* 上架模板

### Milestone 5：自研控制插件

* 替代部分命令执行
* 更强幂等与状态反馈

---

## 18. 风险与对策

* **导入失败残留文件**：

  * staging 与 root 清理机制
* **并发导入导致磁盘/CPU 抖动**：

  * 队列 + 并发限制
* **玩家恶意上传超大世界**：

  * 解压限制 + 审核
* **权限配置失误导致越权**：

  * 默认最小权限 + 审计

---

## 19. 附录：命令示例（参考）

### 19.1 Multiverse

* 导入：`/mv import i_12345 normal`
* 传送：`/mv tp i_12345 <player>`
* 卸载：`/mv unload i_12345`

### 19.2 LuckPerms

* 添加组：`/lp user <player> parent add world_owner`
* 上下文权限（示意）：`/lp user <player> permission set <perm> true world=i_12345`

---

## 20. 未来扩展

* 多服调度（实例创建在空闲节点）
* 模板版本回滚
* 世界快照
* 世界 GUI 管理菜单
* 玩家游玩数据统计与排行榜
