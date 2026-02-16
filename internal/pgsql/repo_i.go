package pgsql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// i-layer implementations.

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

func (r *UserRepoI) ReadByName(ctx context.Context, mcName string) (User, error) {
	var user User
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, mc_uuid, mc_name, server_role, created_at
		FROM users WHERE mc_name = $1
	`, mcName).Scan(&user.ID, &user.MCUUID, &user.MCName, &user.ServerRole, &user.CreatedAt)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *UserRepoI) ListByRole(ctx context.Context, role string) ([]User, error) {
	rows, err := r.connector.QueryContext(ctx, `
		SELECT id, mc_uuid, mc_name, server_role, created_at
		FROM users
		WHERE LOWER(server_role) = LOWER($1)
		ORDER BY id ASC
	`, role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.MCUUID, &u.MCName, &u.ServerRole, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
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
		INSERT INTO map_templates (tag, display_name, game_version, blob_path, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id
	`, template.Tag, template.DisplayName, template.GameVersion, template.BlobPath).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *MapTemplateRepoI) Read(ctx context.Context, id int64) (MapTemplate, error) {
	var t MapTemplate
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, tag, display_name, game_version, blob_path, created_at
		FROM map_templates WHERE id = $1
	`, id).Scan(&t.ID, &t.Tag, &t.DisplayName, &t.GameVersion, &t.BlobPath, &t.CreatedAt)
	if err != nil {
		return MapTemplate{}, err
	}
	return t, nil
}

func (r *MapTemplateRepoI) ReadByTag(ctx context.Context, tag string) (MapTemplate, error) {
	var t MapTemplate
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, tag, display_name, game_version, blob_path, created_at
		FROM map_templates WHERE tag = $1
	`, tag).Scan(&t.ID, &t.Tag, &t.DisplayName, &t.GameVersion, &t.BlobPath, &t.CreatedAt)
	if err != nil {
		return MapTemplate{}, err
	}
	return t, nil
}

func (r *MapTemplateRepoI) List(ctx context.Context) ([]MapTemplate, error) {
	rows, err := r.connector.QueryContext(ctx, `
		SELECT id, tag, display_name, game_version, blob_path, created_at
		FROM map_templates
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]MapTemplate, 0)
	for rows.Next() {
		var t MapTemplate
		if err := rows.Scan(&t.ID, &t.Tag, &t.DisplayName, &t.GameVersion, &t.BlobPath, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *MapTemplateRepoI) ListByGameVersion(ctx context.Context, gameVersion string) ([]MapTemplate, error) {
	rows, err := r.connector.QueryContext(ctx, `
		SELECT id, tag, display_name, game_version, blob_path, created_at
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
		if err := rows.Scan(&t.ID, &t.Tag, &t.DisplayName, &t.GameVersion, &t.BlobPath, &t.CreatedAt); err != nil {
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
		SET tag = $2, display_name = $3, game_version = $4, blob_path = $5
		WHERE id = $1
	`, template.ID, template.Tag, template.DisplayName, template.GameVersion, template.BlobPath)
	return err
}

func (r *MapTemplateRepoI) Delete(ctx context.Context, id int64) error {
	_, err := r.connector.ExecContext(ctx, `DELETE FROM map_templates WHERE id = $1`, id)
	return err
}

type ServerImageRepoI struct{ connector SQLConnector }

func NewServerImageRepoI(connector SQLConnector) *ServerImageRepoI {
	return &ServerImageRepoI{connector: connector}
}

func (r *ServerImageRepoI) Create(ctx context.Context, image ServerImage) error {
	_, err := r.connector.ExecContext(ctx, `
		INSERT INTO server_images (id, name, game_version)
		VALUES ($1, $2, $3)
	`, image.ID, image.Name, image.GameVersion)
	return err
}

func (r *ServerImageRepoI) Read(ctx context.Context, id string) (ServerImage, error) {
	var image ServerImage
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, name, game_version
		FROM server_images
		WHERE id = $1
	`, id).Scan(&image.ID, &image.Name, &image.GameVersion)
	if err != nil {
		return ServerImage{}, err
	}
	return image, nil
}

func (r *ServerImageRepoI) List(ctx context.Context) ([]ServerImage, error) {
	rows, err := r.connector.QueryContext(ctx, `
		SELECT id, name, game_version
		FROM server_images
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ServerImage, 0)
	for rows.Next() {
		var image ServerImage
		if err := rows.Scan(&image.ID, &image.Name, &image.GameVersion); err != nil {
			return nil, err
		}
		out = append(out, image)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ServerImageRepoI) Update(ctx context.Context, image ServerImage) error {
	_, err := r.connector.ExecContext(ctx, `
		UPDATE server_images
		SET name = $2, game_version = $3
		WHERE id = $1
	`, image.ID, image.Name, image.GameVersion)
	return err
}

func (r *ServerImageRepoI) Delete(ctx context.Context, id string) error {
	_, err := r.connector.ExecContext(ctx, `DELETE FROM server_images WHERE id = $1`, id)
	return err
}

type GameVersionRepoI struct{ connector SQLConnector }

func NewGameVersionRepoI(connector SQLConnector) *GameVersionRepoI {
	return &GameVersionRepoI{connector: connector}
}

func (r *GameVersionRepoI) UpsertCheckResult(ctx context.Context, version string, runtimeImageID sql.NullString, coreJar string, status string, checkMessage sql.NullString) error {
	_, err := r.connector.ExecContext(ctx, `
		INSERT INTO game_versions (game_version, runtime_image_id, core_jar, status, check_message, last_checked_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW(), NOW())
		ON CONFLICT (game_version) DO UPDATE
		SET runtime_image_id = EXCLUDED.runtime_image_id,
		    core_jar = EXCLUDED.core_jar,
		    status = EXCLUDED.status,
		    check_message = EXCLUDED.check_message,
		    last_checked_at = EXCLUDED.last_checked_at,
		    updated_at = NOW()
	`, version, runtimeImageID, coreJar, status, checkMessage)
	return err
}

func (r *GameVersionRepoI) Read(ctx context.Context, version string) (GameVersion, error) {
	var v GameVersion
	err := r.connector.QueryRowContext(ctx, `
		SELECT game_version, runtime_image_id, core_jar, status, check_message, last_checked_at, created_at, updated_at
		FROM game_versions
		WHERE game_version = $1
	`, version).Scan(&v.GameVersion, &v.RuntimeImageID, &v.CoreJar, &v.Status, &v.CheckMessage, &v.LastCheckedAt, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return GameVersion{}, err
	}
	return v, nil
}

func (r *GameVersionRepoI) ListVerified(ctx context.Context) ([]GameVersion, error) {
	rows, err := r.connector.QueryContext(ctx, `
		SELECT game_version, runtime_image_id, core_jar, status, check_message, last_checked_at, created_at, updated_at
		FROM game_versions
		WHERE status = 'verified'
		ORDER BY game_version DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]GameVersion, 0)
	for rows.Next() {
		var v GameVersion
		if err := rows.Scan(&v.GameVersion, &v.RuntimeImageID, &v.CoreJar, &v.Status, &v.CheckMessage, &v.LastCheckedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type MapInstanceRepoI struct{ connector SQLConnector }

func NewMapInstanceRepoI(connector SQLConnector) *MapInstanceRepoI {
	return &MapInstanceRepoI{connector: connector}
}

func (r *MapInstanceRepoI) Create(ctx context.Context, inst MapInstance) (int64, error) {
	alias := inst.Alias
	if alias == "" {
		alias = fmt.Sprintf("inst-%d", time.Now().UnixNano())
	}
	accessMode := inst.AccessMode
	if accessMode == "" {
		accessMode = "privacy"
	}
	healthStatus := inst.HealthStatus
	if healthStatus == "" {
		healthStatus = "unknown"
	}
	var id int64
	err := r.connector.QueryRowContext(ctx, `
		INSERT INTO map_instances (
			alias, owner_id, template_id, source_type, game_version, access_mode, status,
			health_status, last_error_msg, last_health_at,
			created_at, updated_at, last_active_at, archived_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW(), $11, $12)
		RETURNING id
	`, alias, inst.OwnerID, inst.TemplateID, inst.SourceType, inst.GameVersion, accessMode, inst.Status, healthStatus, inst.LastErrorMsg, inst.LastHealthAt, inst.LastActiveAt, inst.ArchivedAt).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *MapInstanceRepoI) Read(ctx context.Context, id int64) (MapInstance, error) {
	var inst MapInstance
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, alias, owner_id, template_id, source_type, game_version, access_mode, status, health_status, last_error_msg, last_health_at, created_at, updated_at, last_active_at, archived_at
		FROM map_instances WHERE id = $1
	`, id).Scan(
		&inst.ID,
		&inst.Alias,
		&inst.OwnerID,
		&inst.TemplateID,
		&inst.SourceType,
		&inst.GameVersion,
		&inst.AccessMode,
		&inst.Status,
		&inst.HealthStatus,
		&inst.LastErrorMsg,
		&inst.LastHealthAt,
		&inst.CreatedAt,
		&inst.UpdatedAt,
		&inst.LastActiveAt,
		&inst.ArchivedAt,
	)
	if err != nil {
		return MapInstance{}, err
	}
	return inst, nil
}

func (r *MapInstanceRepoI) ReadByAlias(ctx context.Context, alias string) (MapInstance, error) {
	var inst MapInstance
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, alias, owner_id, template_id, source_type, game_version, access_mode, status, health_status, last_error_msg, last_health_at, created_at, updated_at, last_active_at, archived_at
		FROM map_instances WHERE alias = $1
	`, alias).Scan(
		&inst.ID,
		&inst.Alias,
		&inst.OwnerID,
		&inst.TemplateID,
		&inst.SourceType,
		&inst.GameVersion,
		&inst.AccessMode,
		&inst.Status,
		&inst.HealthStatus,
		&inst.LastErrorMsg,
		&inst.LastHealthAt,
		&inst.CreatedAt,
		&inst.UpdatedAt,
		&inst.LastActiveAt,
		&inst.ArchivedAt,
	)
	if err != nil {
		return MapInstance{}, err
	}
	return inst, nil
}

func (r *MapInstanceRepoI) ListByOwner(ctx context.Context, ownerID int64) ([]MapInstance, error) {
	rows, err := r.connector.QueryContext(ctx, `
		SELECT id, alias, owner_id, template_id, source_type, game_version, access_mode, status, health_status, last_error_msg, last_health_at, created_at, updated_at, last_active_at, archived_at
		FROM map_instances
		WHERE owner_id = $1
		ORDER BY id DESC
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MapInstance, 0)
	for rows.Next() {
		var inst MapInstance
		if err := rows.Scan(
			&inst.ID, &inst.Alias, &inst.OwnerID, &inst.TemplateID, &inst.SourceType,
			&inst.GameVersion, &inst.AccessMode, &inst.Status, &inst.HealthStatus, &inst.LastErrorMsg, &inst.LastHealthAt, &inst.CreatedAt, &inst.UpdatedAt,
			&inst.LastActiveAt, &inst.ArchivedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *MapInstanceRepoI) List(ctx context.Context) ([]MapInstance, error) {
	rows, err := r.connector.QueryContext(ctx, `
		SELECT id, alias, owner_id, template_id, source_type, game_version, access_mode, status, health_status, last_error_msg, last_health_at, created_at, updated_at, last_active_at, archived_at
		FROM map_instances
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MapInstance, 0)
	for rows.Next() {
		var inst MapInstance
		if err := rows.Scan(
			&inst.ID, &inst.Alias, &inst.OwnerID, &inst.TemplateID, &inst.SourceType,
			&inst.GameVersion, &inst.AccessMode, &inst.Status, &inst.HealthStatus, &inst.LastErrorMsg, &inst.LastHealthAt, &inst.CreatedAt, &inst.UpdatedAt,
			&inst.LastActiveAt, &inst.ArchivedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *MapInstanceRepoI) Update(ctx context.Context, inst MapInstance) error {
	accessMode := inst.AccessMode
	if accessMode == "" {
		accessMode = "privacy"
	}
	_, err := r.connector.ExecContext(ctx, `
		UPDATE map_instances
		SET alias = $2,
		    owner_id = $3,
		    template_id = $4,
		    source_type = $5,
		    game_version = $6,
		    access_mode = $7,
		    status = $8,
		    health_status = $9,
		    last_error_msg = $10,
		    last_health_at = $11,
		    updated_at = NOW(),
		    last_active_at = $12,
		    archived_at = $13
		WHERE id = $1
	`, inst.ID, inst.Alias, inst.OwnerID, inst.TemplateID, inst.SourceType, inst.GameVersion, accessMode, inst.Status, inst.HealthStatus, inst.LastErrorMsg, inst.LastHealthAt, inst.LastActiveAt, inst.ArchivedAt)
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

func (r *InstanceMemberRepoI) ListByInstance(ctx context.Context, instanceID int64) ([]InstanceMember, error) {
	rows, err := r.connector.QueryContext(ctx, `
		SELECT id, instance_id, user_id, role, created_at
		FROM instance_members
		WHERE instance_id = $1
		ORDER BY id ASC
	`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]InstanceMember, 0)
	for rows.Next() {
		var m InstanceMember
		if err := rows.Scan(&m.ID, &m.InstanceID, &m.UserID, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
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

func (r *InstanceMemberRepoI) DeleteByInstanceAndUser(ctx context.Context, instanceID int64, userID int64) error {
	_, err := r.connector.ExecContext(ctx, `
		DELETE FROM instance_members
		WHERE instance_id = $1 AND user_id = $2
	`, instanceID, userID)
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
			request_id, request_type, actor_user_id, target_instance_id, template_id,
			requested_alias, status, reviewed_by_user_id, review_note, response_payload,
			error_code, error_msg, expires_at, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
		RETURNING id
	`, req.RequestID, req.RequestType, req.ActorUserID, req.TargetInstanceID, req.TemplateID, req.RequestedAlias,
		req.Status, req.ReviewedByUserID, req.ReviewNote, req.ResponsePayload, req.ErrorCode, req.ErrorMsg, req.ExpiresAt).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *UserRequestRepoI) Read(ctx context.Context, id int64) (UserRequest, error) {
	var req UserRequest
	err := r.connector.QueryRowContext(ctx, `
		SELECT id, request_id, request_type, actor_user_id, target_instance_id, template_id,
		       requested_alias, status, reviewed_by_user_id, review_note, response_payload,
		       error_code, error_msg, expires_at, created_at, updated_at
		FROM user_requests WHERE id = $1
	`, id).Scan(
		&req.ID,
		&req.RequestID,
		&req.RequestType,
		&req.ActorUserID,
		&req.TargetInstanceID,
		&req.TemplateID,
		&req.RequestedAlias,
		&req.Status,
		&req.ReviewedByUserID,
		&req.ReviewNote,
		&req.ResponsePayload,
		&req.ErrorCode,
		&req.ErrorMsg,
		&req.ExpiresAt,
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
		SELECT id, request_id, request_type, actor_user_id, target_instance_id, template_id,
		       requested_alias, status, reviewed_by_user_id, review_note, response_payload,
		       error_code, error_msg, expires_at, created_at, updated_at
		FROM user_requests WHERE request_id = $1
	`, requestID).Scan(
		&req.ID,
		&req.RequestID,
		&req.RequestType,
		&req.ActorUserID,
		&req.TargetInstanceID,
		&req.TemplateID,
		&req.RequestedAlias,
		&req.Status,
		&req.ReviewedByUserID,
		&req.ReviewNote,
		&req.ResponsePayload,
		&req.ErrorCode,
		&req.ErrorMsg,
		&req.ExpiresAt,
		&req.CreatedAt,
		&req.UpdatedAt,
	)
	if err != nil {
		return UserRequest{}, err
	}
	return req, nil
}

func (r *UserRequestRepoI) ListByActor(ctx context.Context, actorUserID int64, limit int) ([]UserRequest, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.connector.QueryContext(ctx, `
		SELECT id, request_id, request_type, actor_user_id, target_instance_id, template_id,
		       requested_alias, status, reviewed_by_user_id, review_note, response_payload,
		       error_code, error_msg, expires_at, created_at, updated_at
		FROM user_requests
		WHERE actor_user_id = $1
		ORDER BY id DESC
		LIMIT $2
	`, actorUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]UserRequest, 0)
	for rows.Next() {
		var req UserRequest
		if err := rows.Scan(
			&req.ID, &req.RequestID, &req.RequestType, &req.ActorUserID, &req.TargetInstanceID, &req.TemplateID,
			&req.RequestedAlias, &req.Status, &req.ReviewedByUserID, &req.ReviewNote, &req.ResponsePayload,
			&req.ErrorCode, &req.ErrorMsg, &req.ExpiresAt, &req.CreatedAt, &req.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *UserRequestRepoI) ListPending(ctx context.Context, limit int) ([]UserRequest, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.connector.QueryContext(ctx, `
		SELECT id, request_id, request_type, actor_user_id, target_instance_id, template_id,
		       requested_alias, status, reviewed_by_user_id, review_note, response_payload,
		       error_code, error_msg, expires_at, created_at, updated_at
		FROM user_requests
		WHERE status = 'pending'
		ORDER BY id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]UserRequest, 0)
	for rows.Next() {
		var req UserRequest
		if err := rows.Scan(
			&req.ID, &req.RequestID, &req.RequestType, &req.ActorUserID, &req.TargetInstanceID, &req.TemplateID,
			&req.RequestedAlias, &req.Status, &req.ReviewedByUserID, &req.ReviewNote, &req.ResponsePayload,
			&req.ErrorCode, &req.ErrorMsg, &req.ExpiresAt, &req.CreatedAt, &req.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *UserRequestRepoI) Update(ctx context.Context, req UserRequest) error {
	_, err := r.connector.ExecContext(ctx, `
		UPDATE user_requests
		SET request_type = $2,
		    actor_user_id = $3,
		    target_instance_id = $4,
		    template_id = $5,
		    requested_alias = $6,
		    status = $7,
		    reviewed_by_user_id = $8,
		    review_note = $9,
		    response_payload = $10,
		    error_code = $11,
		    error_msg = $12,
		    expires_at = $13,
		    updated_at = NOW()
		WHERE id = $1
	`, req.ID, req.RequestType, req.ActorUserID, req.TargetInstanceID, req.TemplateID, req.RequestedAlias,
		req.Status, req.ReviewedByUserID, req.ReviewNote, req.ResponsePayload, req.ErrorCode, req.ErrorMsg, req.ExpiresAt)
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
			request_id, request_type, actor_user_id, target_instance_id, status, response_payload,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, 'accepted', $5, NOW(), NOW())
		ON CONFLICT (request_id) DO NOTHING
		RETURNING id
	`, requestID, requestType, actorUserID.Int64, targetInstanceID, json.RawMessage(`{}`)).Scan(&id)
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

var _ UserRepo = (*UserRepoI)(nil)
var _ MapTemplateRepo = (*MapTemplateRepoI)(nil)
var _ ServerImageRepo = (*ServerImageRepoI)(nil)
var _ GameVersionRepo = (*GameVersionRepoI)(nil)
var _ MapInstanceRepo = (*MapInstanceRepoI)(nil)
var _ InstanceMemberRepo = (*InstanceMemberRepoI)(nil)
var _ UserRequestRepo = (*UserRequestRepoI)(nil)
