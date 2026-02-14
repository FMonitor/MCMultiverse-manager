CREATE TABLE IF NOT EXISTS users (
  id BIGSERIAL PRIMARY KEY,
  mc_uuid UUID NOT NULL UNIQUE,
  mc_name TEXT NOT NULL UNIQUE,
  server_role TEXT NOT NULL DEFAULT 'user' CHECK (server_role IN ('user', 'admin')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_users_mc_name ON users (mc_name);

CREATE TABLE IF NOT EXISTS map_templates (
  id BIGSERIAL PRIMARY KEY,
  tag TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  game_version TEXT NOT NULL,
  blob_path TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_map_templates_game_version ON map_templates (game_version);

CREATE TABLE IF NOT EXISTS server_images (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  game_version TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_server_images_game_version ON server_images (game_version);

INSERT INTO server_images (id, name, game_version) VALUES
  ('runtime-java16', 'MiniMap Java16 Runtime', '1.16.5'),
  ('runtime-java17', 'MiniMap Java17 Runtime', '1.20.1'),
  ('runtime-java21', 'MiniMap Java21 Runtime', '1.21.1')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS map_instances (
  id BIGSERIAL PRIMARY KEY,
  alias TEXT NOT NULL UNIQUE,
  owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  template_id BIGINT REFERENCES map_templates(id) ON DELETE SET NULL,
  source_type TEXT NOT NULL CHECK (source_type IN ('template', 'upload', 'empty')),
  game_version TEXT NOT NULL,
  access_mode TEXT NOT NULL DEFAULT 'privacy' CHECK (access_mode IN ('privacy', 'public')),
  status TEXT NOT NULL CHECK (status IN ('Waiting', 'Preparing', 'Starting', 'On', 'Stopping', 'Off', 'Archived')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_active_at TIMESTAMPTZ,
  archived_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_map_instances_owner_id ON map_instances (owner_id);
CREATE INDEX IF NOT EXISTS idx_map_instances_template_id ON map_instances (template_id);
CREATE INDEX IF NOT EXISTS idx_map_instances_game_version ON map_instances (game_version);
CREATE INDEX IF NOT EXISTS idx_map_instances_status ON map_instances (status);
CREATE INDEX IF NOT EXISTS idx_map_instances_access_mode ON map_instances (access_mode);

CREATE TABLE IF NOT EXISTS instance_members (
  id BIGSERIAL PRIMARY KEY,
  instance_id BIGINT NOT NULL REFERENCES map_instances(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('owner', 'member')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (instance_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_instance_members_user_id ON instance_members (user_id);

CREATE TABLE IF NOT EXISTS user_requests (
  id BIGSERIAL PRIMARY KEY,
  request_id UUID NOT NULL UNIQUE,
  request_type TEXT NOT NULL,
  actor_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  target_instance_id BIGINT REFERENCES map_instances(id) ON DELETE SET NULL,
  template_id BIGINT REFERENCES map_templates(id) ON DELETE SET NULL,
  requested_alias TEXT,
  status TEXT NOT NULL CHECK (
    status IN (
      'pending', 'approved', 'rejected', 'canceled',
      'processing', 'succeeded', 'failed',
      'accepted'
    )
  ),
  reviewed_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  review_note TEXT,
  response_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_code TEXT,
  error_msg TEXT,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_requests_request_type ON user_requests (request_type);
CREATE INDEX IF NOT EXISTS idx_user_requests_actor_user_id ON user_requests (actor_user_id);
CREATE INDEX IF NOT EXISTS idx_user_requests_target_instance_id ON user_requests (target_instance_id);
CREATE INDEX IF NOT EXISTS idx_user_requests_status ON user_requests (status);
