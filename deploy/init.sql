CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
  id BIGSERIAL PRIMARY KEY,
  mc_uuid UUID NOT NULL UNIQUE,
  mc_name TEXT NOT NULL,
  server_role TEXT NOT NULL DEFAULT 'user' CHECK (server_role IN ('user', 'admin')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_users_mc_name ON users (mc_name);

CREATE TABLE IF NOT EXISTS map_templates (
  id BIGSERIAL PRIMARY KEY,
  tag TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  version TEXT NOT NULL,
  game_version TEXT NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  sha256_hash TEXT NOT NULL,
  blob_path TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_map_templates_sha256_hash ON map_templates (sha256_hash);

CREATE TABLE IF NOT EXISTS game_servers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  game_version TEXT NOT NULL,
  root_path TEXT NOT NULL,
  servertap_url TEXT NOT NULL,
  servertap_key TEXT NOT NULL DEFAULT '',
  servertap_auth_header TEXT NOT NULL DEFAULT 'key',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_game_servers_game_version ON game_servers (game_version);
CREATE INDEX IF NOT EXISTS idx_game_servers_enabled ON game_servers (enabled);

CREATE TABLE IF NOT EXISTS map_instances (
  id BIGSERIAL PRIMARY KEY,
  owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  template_id BIGINT REFERENCES map_templates(id) ON DELETE SET NULL,
  server_id TEXT REFERENCES game_servers(id) ON DELETE SET NULL,
  source_type TEXT NOT NULL CHECK (source_type IN ('template', 'upload')),
  game_version TEXT NOT NULL DEFAULT 'unknown',
  internal_name TEXT NOT NULL UNIQUE CHECK (internal_name ~ '^[a-z0-9_]+$'),
  alias TEXT NOT NULL UNIQUE CHECK (alias ~ '^[a-z0-9_-]{1,24}$'),
  status TEXT NOT NULL,
  storage_type TEXT NOT NULL CHECK (storage_type IN ('ssd', 'hdd')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_active_at TIMESTAMPTZ,
  archived_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_map_instances_owner_id ON map_instances (owner_id);
CREATE INDEX IF NOT EXISTS idx_map_instances_template_id ON map_instances (template_id);
CREATE INDEX IF NOT EXISTS idx_map_instances_server_id ON map_instances (server_id);
CREATE INDEX IF NOT EXISTS idx_map_instances_game_version ON map_instances (game_version);
CREATE INDEX IF NOT EXISTS idx_map_instances_status ON map_instances (status);
CREATE INDEX IF NOT EXISTS idx_map_instances_storage_type ON map_instances (storage_type);

CREATE TABLE IF NOT EXISTS instance_members (
  id BIGSERIAL PRIMARY KEY,
  instance_id BIGINT NOT NULL REFERENCES map_instances(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (instance_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_instance_members_user_id ON instance_members (user_id);

CREATE TABLE IF NOT EXISTS load_tasks (
  id BIGSERIAL PRIMARY KEY,
  instance_id BIGINT NOT NULL REFERENCES map_instances(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
  error_code TEXT,
  error_msg TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  started_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_load_tasks_instance_id ON load_tasks (instance_id);
CREATE INDEX IF NOT EXISTS idx_load_tasks_status ON load_tasks (status);
CREATE INDEX IF NOT EXISTS idx_load_tasks_created_at ON load_tasks (created_at);

CREATE TABLE IF NOT EXISTS audit_log (
  id BIGSERIAL PRIMARY KEY,
  actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  instance_id BIGINT REFERENCES map_instances(id) ON DELETE SET NULL,
  action TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor_user_id ON audit_log (actor_user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_instance_id ON audit_log (instance_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log (created_at);

-- user_requests is the idempotency request table with a shorter name.
CREATE TABLE IF NOT EXISTS user_requests (
  id BIGSERIAL PRIMARY KEY,
  request_id UUID NOT NULL UNIQUE,
  request_type TEXT NOT NULL CHECK (request_type IN ('create_instance', 'delete_instance', 'archive_instance', 'restore_instance')),
  actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  target_instance_id BIGINT REFERENCES map_instances(id) ON DELETE SET NULL,
  status TEXT NOT NULL CHECK (status IN ('accepted', 'processing', 'succeeded', 'failed')),
  response_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_code TEXT,
  error_msg TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_requests_request_type ON user_requests (request_type);
CREATE INDEX IF NOT EXISTS idx_user_requests_actor_user_id ON user_requests (actor_user_id);
CREATE INDEX IF NOT EXISTS idx_user_requests_target_instance_id ON user_requests (target_instance_id);
CREATE INDEX IF NOT EXISTS idx_user_requests_status ON user_requests (status);
