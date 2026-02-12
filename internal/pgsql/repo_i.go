package pgsql

import (
	"context"
	"database/sql"
	"encoding/json"
)

// i-layer implementations.

type SchemaMigrationRepoI struct{ connector SQLConnector }

func NewSchemaMigrationRepoI(connector SQLConnector) *SchemaMigrationRepoI {
	return &SchemaMigrationRepoI{connector: connector}
}

func (r *SchemaMigrationRepoI) Create(ctx context.Context, migration SchemaMigration) error {
	_, err := r.connector.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, applied_at)
		VALUES ($1, NOW())
	`, migration.Version)
	return err
}

func (r *SchemaMigrationRepoI) Read(ctx context.Context, version string) (SchemaMigration, error) {
	var m SchemaMigration
	err := r.connector.QueryRowContext(ctx, `
		SELECT version, applied_at
		FROM schema_migrations
		WHERE version = $1
	`, version).Scan(&m.Version, &m.AppliedAt)
	if err != nil {
		return SchemaMigration{}, err
	}
	return m, nil
}

func (r *SchemaMigrationRepoI) Update(ctx context.Context, migration SchemaMigration) error {
	_, err := r.connector.ExecContext(ctx, `
		UPDATE schema_migrations
		SET applied_at = NOW()
		WHERE version = $1
	`, migration.Version)
	return err
}

func (r *SchemaMigrationRepoI) Delete(ctx context.Context, version string) error {
	_, err := r.connector.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version)
	return err
}

type UserRepoI struct{ connector SQLConnector }

func NewUserRepoI(connector SQLConnector) *UserRepoI { return &UserRepoI{connector: connector} }

func (r *UserRepoI) Create(ctx context.Context, user User) (int64, error) {
	var id int64
	err := r.connector.QueryRowContext(ctx, `
		INSERT INTO users (mc_uuid, mc_name, server_role, created_at)
		VALUES ($1, $2, $3, NOW())
		RETURNING id
	`, user.MCUUID, user.MCName, user.ServerRole).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *UserRepoI) Read(ctx context.Context, id int64) (User, error) {
	var user User
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, mc_uuid, mc_name, server_role, created_at
		FROM users WHERE id = $1
	`, id).Scan(&user.ID, &user.MCUUID, &user.MCName, &user.ServerRole, &user.CreatedAt)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *UserRepoI) ReadByUUID(ctx context.Context, mcUUID string) (User, error) {
	var user User
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, mc_uuid, mc_name, server_role, created_at
		FROM users WHERE mc_uuid = $1
	`, mcUUID).Scan(&user.ID, &user.MCUUID, &user.MCName, &user.ServerRole, &user.CreatedAt)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *UserRepoI) Update(ctx context.Context, user User) error {
	_, err := r.connector.ExecContext(ctx, `
		UPDATE users
		SET mc_uuid = $2, mc_name = $3, server_role = $4
		WHERE id = $1
	`, user.ID, user.MCUUID, user.MCName, user.ServerRole)
	return err
}

func (r *UserRepoI) Delete(ctx context.Context, id int64) error {
	_, err := r.connector.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

type MapTemplateRepoI struct{ connector SQLConnector }

func NewMapTemplateRepoI(connector SQLConnector) *MapTemplateRepoI {
	return &MapTemplateRepoI{connector: connector}
}

func (r *MapTemplateRepoI) Create(ctx context.Context, template MapTemplate) (int64, error) {
	var id int64
	err := r.connector.QueryRowContext(ctx, `
		INSERT INTO map_templates (tag, display_name, version, game_version, size_bytes, sha256_hash, blob_path, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING id
	`, template.Tag, template.DisplayName, template.Version, template.GameVersion, template.SizeBytes, template.SHA256Hash, template.BlobPath).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *MapTemplateRepoI) Read(ctx context.Context, id int64) (MapTemplate, error) {
	var t MapTemplate
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, tag, display_name, version, game_version, size_bytes, sha256_hash, blob_path, created_at
		FROM map_templates WHERE id = $1
	`, id).Scan(&t.ID, &t.Tag, &t.DisplayName, &t.Version, &t.GameVersion, &t.SizeBytes, &t.SHA256Hash, &t.BlobPath, &t.CreatedAt)
	if err != nil {
		return MapTemplate{}, err
	}
	return t, nil
}

func (r *MapTemplateRepoI) ReadByTag(ctx context.Context, tag string) (MapTemplate, error) {
	var t MapTemplate
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, tag, display_name, version, game_version, size_bytes, sha256_hash, blob_path, created_at
		FROM map_templates WHERE tag = $1
	`, tag).Scan(&t.ID, &t.Tag, &t.DisplayName, &t.Version, &t.GameVersion, &t.SizeBytes, &t.SHA256Hash, &t.BlobPath, &t.CreatedAt)
	if err != nil {
		return MapTemplate{}, err
	}
	return t, nil
}

func (r *MapTemplateRepoI) ListByGameVersion(ctx context.Context, gameVersion string) ([]MapTemplate, error) {
	rows, err := r.connector.QueryContext(ctx, `
		SELECT id, tag, display_name, version, game_version, size_bytes, sha256_hash, blob_path, created_at
		FROM map_templates
		WHERE game_version = $1
		ORDER BY created_at DESC, id DESC
	`, gameVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]MapTemplate, 0)
	for rows.Next() {
		var t MapTemplate
		if err := rows.Scan(&t.ID, &t.Tag, &t.DisplayName, &t.Version, &t.GameVersion, &t.SizeBytes, &t.SHA256Hash, &t.BlobPath, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *MapTemplateRepoI) ListGameVersions(ctx context.Context) ([]string, error) {
	rows, err := r.connector.QueryContext(ctx, `
		SELECT DISTINCT game_version
		FROM map_templates
		ORDER BY game_version DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *MapTemplateRepoI) Update(ctx context.Context, template MapTemplate) error {
	_, err := r.connector.ExecContext(ctx, `
		UPDATE map_templates
		SET tag = $2, display_name = $3, version = $4, game_version = $5, size_bytes = $6, sha256_hash = $7, blob_path = $8
		WHERE id = $1
	`, template.ID, template.Tag, template.DisplayName, template.Version, template.GameVersion, template.SizeBytes, template.SHA256Hash, template.BlobPath)
	return err
}

func (r *MapTemplateRepoI) Delete(ctx context.Context, id int64) error {
	_, err := r.connector.ExecContext(ctx, `DELETE FROM map_templates WHERE id = $1`, id)
	return err
}

type GameServerRepoI struct{ connector SQLConnector }

func NewGameServerRepoI(connector SQLConnector) *GameServerRepoI {
	return &GameServerRepoI{connector: connector}
}

func (r *GameServerRepoI) Create(ctx context.Context, server GameServer) error {
	_, err := r.connector.ExecContext(ctx, `
		INSERT INTO game_servers (
			id, name, game_version, root_path, servertap_url, servertap_key, servertap_auth_header, enabled, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
	`, server.ID, server.Name, server.GameVersion, server.RootPath, server.ServerTapURL, server.ServerTapKey, server.ServerTapAuthHeader, server.Enabled)
	return err
}

func (r *GameServerRepoI) Read(ctx context.Context, id string) (GameServer, error) {
	var s GameServer
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, name, game_version, root_path, servertap_url, servertap_key, servertap_auth_header, enabled, created_at, updated_at
		FROM game_servers
		WHERE id = $1
	`, id).Scan(
		&s.ID, &s.Name, &s.GameVersion, &s.RootPath, &s.ServerTapURL, &s.ServerTapKey, &s.ServerTapAuthHeader, &s.Enabled, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return GameServer{}, err
	}
	return s, nil
}

func (r *GameServerRepoI) List(ctx context.Context) ([]GameServer, error) {
	rows, err := r.connector.QueryContext(ctx, `
		SELECT id, name, game_version, root_path, servertap_url, servertap_key, servertap_auth_header, enabled, created_at, updated_at
		FROM game_servers
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]GameServer, 0)
	for rows.Next() {
		var s GameServer
		if err := rows.Scan(&s.ID, &s.Name, &s.GameVersion, &s.RootPath, &s.ServerTapURL, &s.ServerTapKey, &s.ServerTapAuthHeader, &s.Enabled, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *GameServerRepoI) Update(ctx context.Context, server GameServer) error {
	_, err := r.connector.ExecContext(ctx, `
		UPDATE game_servers
		SET name = $2,
		    game_version = $3,
		    root_path = $4,
		    servertap_url = $5,
		    servertap_key = $6,
		    servertap_auth_header = $7,
		    enabled = $8,
		    updated_at = NOW()
		WHERE id = $1
	`, server.ID, server.Name, server.GameVersion, server.RootPath, server.ServerTapURL, server.ServerTapKey, server.ServerTapAuthHeader, server.Enabled)
	return err
}

func (r *GameServerRepoI) Delete(ctx context.Context, id string) error {
	_, err := r.connector.ExecContext(ctx, `DELETE FROM game_servers WHERE id = $1`, id)
	return err
}

type MapInstanceRepoI struct{ connector SQLConnector }

func NewMapInstanceRepoI(connector SQLConnector) *MapInstanceRepoI {
	return &MapInstanceRepoI{connector: connector}
}

func (r *MapInstanceRepoI) Create(ctx context.Context, inst MapInstance) (int64, error) {
	var id int64
	err := r.connector.QueryRowContext(ctx, `
		INSERT INTO map_instances (owner_id, template_id, server_id, source_type, game_version, internal_name, alias, status, storage_type, created_at, updated_at, last_active_at, archived_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW(), $10, $11)
		RETURNING id
	`, inst.OwnerID, inst.TemplateID, inst.ServerID, inst.SourceType, inst.GameVersion, inst.InternalName, inst.Alias, inst.Status, inst.StorageType, inst.LastActiveAt, inst.ArchivedAt).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *MapInstanceRepoI) Read(ctx context.Context, id int64) (MapInstance, error) {
	var inst MapInstance
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, owner_id, template_id, server_id, source_type, game_version, internal_name, alias, status, storage_type, created_at, updated_at, last_active_at, archived_at
		FROM map_instances WHERE id = $1
	`, id).Scan(&inst.ID, &inst.OwnerID, &inst.TemplateID, &inst.ServerID, &inst.SourceType, &inst.GameVersion, &inst.InternalName, &inst.Alias, &inst.Status, &inst.StorageType, &inst.CreatedAt, &inst.UpdatedAt, &inst.LastActiveAt, &inst.ArchivedAt)
	if err != nil {
		return MapInstance{}, err
	}
	return inst, nil
}

func (r *MapInstanceRepoI) ReadByAlias(ctx context.Context, alias string) (MapInstance, error) {
	var inst MapInstance
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, owner_id, template_id, server_id, source_type, game_version, internal_name, alias, status, storage_type, created_at, updated_at, last_active_at, archived_at
		FROM map_instances WHERE alias = $1
	`, alias).Scan(&inst.ID, &inst.OwnerID, &inst.TemplateID, &inst.ServerID, &inst.SourceType, &inst.GameVersion, &inst.InternalName, &inst.Alias, &inst.Status, &inst.StorageType, &inst.CreatedAt, &inst.UpdatedAt, &inst.LastActiveAt, &inst.ArchivedAt)
	if err != nil {
		return MapInstance{}, err
	}
	return inst, nil
}

func (r *MapInstanceRepoI) Update(ctx context.Context, inst MapInstance) error {
	_, err := r.connector.ExecContext(ctx, `
		UPDATE map_instances
		SET owner_id = $2, template_id = $3, server_id = $4, source_type = $5, game_version = $6, internal_name = $7, alias = $8, status = $9, storage_type = $10, updated_at = NOW(), last_active_at = $11, archived_at = $12
		WHERE id = $1
	`, inst.ID, inst.OwnerID, inst.TemplateID, inst.ServerID, inst.SourceType, inst.GameVersion, inst.InternalName, inst.Alias, inst.Status, inst.StorageType, inst.LastActiveAt, inst.ArchivedAt)
	return err
}

func (r *MapInstanceRepoI) Delete(ctx context.Context, id int64) error {
	_, err := r.connector.ExecContext(ctx, `DELETE FROM map_instances WHERE id = $1`, id)
	return err
}

type InstanceMemberRepoI struct{ connector SQLConnector }

func NewInstanceMemberRepoI(connector SQLConnector) *InstanceMemberRepoI {
	return &InstanceMemberRepoI{connector: connector}
}

func (r *InstanceMemberRepoI) Create(ctx context.Context, member InstanceMember) (int64, error) {
	var id int64
	err := r.connector.QueryRowContext(ctx, `
		INSERT INTO instance_members (instance_id, user_id, role, created_at)
		VALUES ($1, $2, $3, NOW())
		RETURNING id
	`, member.InstanceID, member.UserID, member.Role).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *InstanceMemberRepoI) Read(ctx context.Context, id int64) (InstanceMember, error) {
	var member InstanceMember
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, instance_id, user_id, role, created_at
		FROM instance_members WHERE id = $1
	`, id).Scan(&member.ID, &member.InstanceID, &member.UserID, &member.Role, &member.CreatedAt)
	if err != nil {
		return InstanceMember{}, err
	}
	return member, nil
}

func (r *InstanceMemberRepoI) Update(ctx context.Context, member InstanceMember) error {
	_, err := r.connector.ExecContext(ctx, `
		UPDATE instance_members
		SET instance_id = $2, user_id = $3, role = $4
		WHERE id = $1
	`, member.ID, member.InstanceID, member.UserID, member.Role)
	return err
}

func (r *InstanceMemberRepoI) Delete(ctx context.Context, id int64) error {
	_, err := r.connector.ExecContext(ctx, `DELETE FROM instance_members WHERE id = $1`, id)
	return err
}

type LoadTaskRepoI struct{ connector SQLConnector }

func NewLoadTaskRepoI(connector SQLConnector) *LoadTaskRepoI {
	return &LoadTaskRepoI{connector: connector}
}

func (r *LoadTaskRepoI) Create(ctx context.Context, task LoadTask) (int64, error) {
	var id int64
	err := r.connector.QueryRowContext(ctx, `
		INSERT INTO load_tasks (instance_id, status, error_code, error_msg, created_at, started_at, updated_at, finished_at)
		VALUES ($1, $2, $3, $4, NOW(), $5, NOW(), $6)
		RETURNING id
	`, task.InstanceID, task.Status, task.ErrorCode, task.ErrorMsg, task.StartedAt, task.FinishedAt).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *LoadTaskRepoI) Read(ctx context.Context, id int64) (LoadTask, error) {
	var task LoadTask
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, instance_id, status, error_code, error_msg, created_at, started_at, updated_at, finished_at
		FROM load_tasks WHERE id = $1
	`, id).Scan(&task.ID, &task.InstanceID, &task.Status, &task.ErrorCode, &task.ErrorMsg, &task.CreatedAt, &task.StartedAt, &task.UpdatedAt, &task.FinishedAt)
	if err != nil {
		return LoadTask{}, err
	}
	return task, nil
}

func (r *LoadTaskRepoI) Update(ctx context.Context, task LoadTask) error {
	_, err := r.connector.ExecContext(ctx, `
		UPDATE load_tasks
		SET instance_id = $2, status = $3, error_code = $4, error_msg = $5, started_at = $6, updated_at = NOW(), finished_at = $7
		WHERE id = $1
	`, task.ID, task.InstanceID, task.Status, task.ErrorCode, task.ErrorMsg, task.StartedAt, task.FinishedAt)
	return err
}

func (r *LoadTaskRepoI) Delete(ctx context.Context, id int64) error {
	_, err := r.connector.ExecContext(ctx, `DELETE FROM load_tasks WHERE id = $1`, id)
	return err
}

type AuditLogRepoI struct{ connector SQLConnector }

func NewAuditLogRepoI(connector SQLConnector) *AuditLogRepoI {
	return &AuditLogRepoI{connector: connector}
}

func (r *AuditLogRepoI) Create(ctx context.Context, al AuditLog) (int64, error) {
	var id int64
	err := r.connector.QueryRowContext(ctx, `
		INSERT INTO audit_log (actor_user_id, instance_id, action, description, payload_json, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING id
	`, al.ActorUserID, al.InstanceID, al.Action, al.Description, al.PayloadJSON).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *AuditLogRepoI) Read(ctx context.Context, id int64) (AuditLog, error) {
	var al AuditLog
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, actor_user_id, instance_id, action, description, payload_json, created_at
		FROM audit_log WHERE id = $1
	`, id).Scan(&al.ID, &al.ActorUserID, &al.InstanceID, &al.Action, &al.Description, &al.PayloadJSON, &al.CreatedAt)
	if err != nil {
		return AuditLog{}, err
	}
	return al, nil
}

func (r *AuditLogRepoI) Update(ctx context.Context, al AuditLog) error {
	_, err := r.connector.ExecContext(ctx, `
		UPDATE audit_log
		SET actor_user_id = $2, instance_id = $3, action = $4, description = $5, payload_json = $6
		WHERE id = $1
	`, al.ID, al.ActorUserID, al.InstanceID, al.Action, al.Description, al.PayloadJSON)
	return err
}

func (r *AuditLogRepoI) Delete(ctx context.Context, id int64) error {
	_, err := r.connector.ExecContext(ctx, `DELETE FROM audit_log WHERE id = $1`, id)
	return err
}

type UserRequestRepoI struct{ connector SQLConnector }

func NewUserRequestRepoI(connector SQLConnector) *UserRequestRepoI {
	return &UserRequestRepoI{connector: connector}
}

func (r *UserRequestRepoI) Create(ctx context.Context, req UserRequest) (int64, error) {
	var id int64
	err := r.connector.QueryRowContext(ctx, `
		INSERT INTO user_requests (
			request_id, request_type, actor_user_id, target_instance_id, status,
			response_payload, error_code, error_msg, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id
	`, req.RequestID, req.RequestType, req.ActorUserID, req.TargetInstanceID, req.Status, req.ResponsePayload, req.ErrorCode, req.ErrorMsg).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *UserRequestRepoI) Read(ctx context.Context, id int64) (UserRequest, error) {
	var req UserRequest
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, request_id, request_type, actor_user_id, target_instance_id, status,
		       response_payload, error_code, error_msg, created_at, updated_at
		FROM user_requests WHERE id = $1
	`, id).Scan(
		&req.ID,
		&req.RequestID,
		&req.RequestType,
		&req.ActorUserID,
		&req.TargetInstanceID,
		&req.Status,
		&req.ResponsePayload,
		&req.ErrorCode,
		&req.ErrorMsg,
		&req.CreatedAt,
		&req.UpdatedAt,
	)
	if err != nil {
		return UserRequest{}, err
	}
	return req, nil
}

func (r *UserRequestRepoI) ReadByRequestID(ctx context.Context, requestID string) (UserRequest, error) {
	var req UserRequest
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, request_id, request_type, actor_user_id, target_instance_id, status,
		       response_payload, error_code, error_msg, created_at, updated_at
		FROM user_requests WHERE request_id = $1
	`, requestID).Scan(
		&req.ID,
		&req.RequestID,
		&req.RequestType,
		&req.ActorUserID,
		&req.TargetInstanceID,
		&req.Status,
		&req.ResponsePayload,
		&req.ErrorCode,
		&req.ErrorMsg,
		&req.CreatedAt,
		&req.UpdatedAt,
	)
	if err != nil {
		return UserRequest{}, err
	}
	return req, nil
}

func (r *UserRequestRepoI) Update(ctx context.Context, req UserRequest) error {
	_, err := r.connector.ExecContext(ctx, `
		UPDATE user_requests
		SET request_type = $2,
		    actor_user_id = $3,
		    target_instance_id = $4,
		    status = $5,
		    response_payload = $6,
		    error_code = $7,
		    error_msg = $8,
		    updated_at = NOW()
		WHERE id = $1
	`, req.ID, req.RequestType, req.ActorUserID, req.TargetInstanceID, req.Status, req.ResponsePayload, req.ErrorCode, req.ErrorMsg)
	return err
}

func (r *UserRequestRepoI) Delete(ctx context.Context, id int64) error {
	_, err := r.connector.ExecContext(ctx, `DELETE FROM user_requests WHERE id = $1`, id)
	return err
}

func (r *UserRequestRepoI) CreateAcceptedIfNotExists(
	ctx context.Context,
	requestID string,
	requestType string,
	actorUserID sql.NullInt64,
	targetInstanceID sql.NullInt64,
) (UserRequest, bool, error) {
	var id int64
	err := r.connector.QueryRowContext(ctx, `
		INSERT INTO user_requests (
			request_id, request_type, actor_user_id, target_instance_id, status,
			response_payload, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, 'accepted', $5, NOW(), NOW())
		ON CONFLICT (request_id) DO NOTHING
		RETURNING id
	`, requestID, requestType, actorUserID, targetInstanceID, json.RawMessage(`{}`)).Scan(&id)
	if err == sql.ErrNoRows {
		existing, readErr := r.ReadByRequestID(ctx, requestID)
		if readErr != nil {
			return UserRequest{}, false, readErr
		}
		return existing, false, nil
	}
	if err != nil {
		return UserRequest{}, false, err
	}

	created, err := r.Read(ctx, id)
	if err != nil {
		return UserRequest{}, true, err
	}
	return created, true, nil
}

func (r *UserRequestRepoI) MarkRequestResult(
	ctx context.Context,
	requestID string,
	status string,
	responsePayload json.RawMessage,
	errorCode sql.NullString,
	errorMsg sql.NullString,
) error {
	if len(responsePayload) == 0 {
		responsePayload = json.RawMessage(`{}`)
	}
	_, err := r.connector.ExecContext(ctx, `
		UPDATE user_requests
		SET status = $2,
		    response_payload = $3,
		    error_code = $4,
		    error_msg = $5,
		    updated_at = NOW()
		WHERE request_id = $1
	`, requestID, status, responsePayload, errorCode, errorMsg)
	return err
}

var _ SchemaMigrationRepo = (*SchemaMigrationRepoI)(nil)
var _ UserRepo = (*UserRepoI)(nil)
var _ MapTemplateRepo = (*MapTemplateRepoI)(nil)
var _ GameServerRepo = (*GameServerRepoI)(nil)
var _ MapInstanceRepo = (*MapInstanceRepoI)(nil)
var _ InstanceMemberRepo = (*InstanceMemberRepoI)(nil)
var _ LoadTaskRepo = (*LoadTaskRepoI)(nil)
var _ AuditLogRepo = (*AuditLogRepoI)(nil)
var _ UserRequestRepo = (*UserRequestRepoI)(nil)
