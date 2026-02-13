package pgsql

import (
	"database/sql"
	"encoding/json"
	"time"
)

type User struct {
	ID         int64     `db:"id"`
	MCUUID     string    `db:"mc_uuid"`
	MCName     string    `db:"mc_name"`
	ServerRole string    `db:"server_role"`
	CreatedAt  time.Time `db:"created_at"`
}

type MapTemplate struct {
	ID          int64     `db:"id"`
	Tag         string    `db:"tag"`
	DisplayName string    `db:"display_name"`
	Version     string    `db:"version"`
	GameVersion string    `db:"game_version"`
	SizeBytes   int64     `db:"size_bytes"`
	SHA256Hash  string    `db:"sha256_hash"`
	BlobPath    string    `db:"blob_path"`
	CreatedAt   time.Time `db:"created_at"`
}

type MapInstance struct {
	ID           int64         `db:"id"`
	OwnerID      int64         `db:"owner_id"`
	TemplateID   sql.NullInt64 `db:"template_id"`
	SourceType   string        `db:"source_type"`
	GameVersion  string        `db:"game_version"`
	Status       string        `db:"status"`
	CreatedAt    time.Time     `db:"created_at"`
	UpdatedAt    time.Time     `db:"updated_at"`
	LastActiveAt sql.NullTime  `db:"last_active_at"`
	ArchivedAt   sql.NullTime  `db:"archived_at"`
}

type ServerImage struct {
	ID          string `db:"id"`
	Name        string `db:"name"`
	GameVersion string `db:"game_version"`
}

type InstanceMember struct {
	ID         int64     `db:"id"`
	InstanceID int64     `db:"instance_id"`
	UserID     int64     `db:"user_id"`
	Role       string    `db:"role"`
	CreatedAt  time.Time `db:"created_at"`
}

// UserRequest is idempotency request model with a shorter name.
type UserRequest struct {
	ID               int64           `db:"id"`
	RequestID        string          `db:"request_id"`
	RequestType      string          `db:"request_type"`
	ActorUserID      sql.NullInt64   `db:"actor_user_id"`
	TargetInstanceID sql.NullInt64   `db:"target_instance_id"`
	Status           string          `db:"status"`
	ResponsePayload  json.RawMessage `db:"response_payload"`
	ErrorCode        sql.NullString  `db:"error_code"`
	ErrorMsg         sql.NullString  `db:"error_msg"`
	CreatedAt        time.Time       `db:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at"`
}
