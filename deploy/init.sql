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
CREATE INDEX IF NOT EXISTS idx_map_templates_game_version ON map_templates (game_version);

CREATE TABLE IF NOT EXISTS server_images (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  game_version TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_server_images_game_version ON server_images (game_version);

CREATE TABLE IF NOT EXISTS map_instances (
  id BIGSERIAL PRIMARY KEY,
  owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  template_id BIGINT REFERENCES map_templates(id) ON DELETE SET NULL,
  source_type TEXT NOT NULL CHECK (source_type IN ('template', 'upload')),
  game_version TEXT NOT NULL DEFAULT 'unknown',
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_active_at TIMESTAMPTZ,
  archived_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_map_instances_owner_id ON map_instances (owner_id);
CREATE INDEX IF NOT EXISTS idx_map_instances_template_id ON map_instances (template_id);
CREATE INDEX IF NOT EXISTS idx_map_instances_game_version ON map_instances (game_version);
CREATE INDEX IF NOT EXISTS idx_map_instances_status ON map_instances (status);

CREATE TABLE IF NOT EXISTS instance_members (
  id BIGSERIAL PRIMARY KEY,
  instance_id BIGINT NOT NULL REFERENCES map_instances(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('owner', 'member')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (instance_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_instance_members_user_id ON instance_members (user_id);

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
