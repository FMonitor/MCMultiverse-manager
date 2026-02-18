package worker

import (
	"context"
	"time"

	"mcmm/internal/pgsql"
)

type Worker interface {
	StartFromTemplate(ctx context.Context, instanceID int64, template pgsql.MapTemplate) error
	StartFromUpload(ctx context.Context, instanceID int64, uploadWorldPath string) error
	StartEmpty(ctx context.Context, instanceID int64, gameVersion string) error
	StartExisting(ctx context.Context, instanceID int64) error
	StopOnly(ctx context.Context, instanceID int64) error
	StopAndArchive(ctx context.Context, instanceID int64) error
	DeleteArchived(ctx context.Context, instanceID int64) error
}

type Status string

const (
	StatusWaiting   Status = "Waiting"
	StatusPreparing Status = "Preparing"
	StatusStarting  Status = "Starting"
	StatusOn        Status = "On"
	StatusStopping  Status = "Stopping"
	StatusOff       Status = "Off"
	StatusArchived  Status = "Archived"
)

type HealthStatus string

const (
	HealthUnknown     HealthStatus = "unknown"
	HealthHealthy     HealthStatus = "healthy"
	HealthStartFailed HealthStatus = "start_failed"
	HealthUnreachable HealthStatus = "unreachable"
)

// Options are fixed deployment inputs for worker runtime.
type Options struct {
	InstanceRootDir       string
	VersionRootDir        string
	ComposeTemplateDir    string
	ArchiveRootDir        string
	DefaultGameVersion    string
	ServerTapPort         int
	ServerTapTimeout      time.Duration
	InstanceNetwork       string
	InstanceTapURLPattern string
	ServerTapAuthKey      string
	ServerTapAuthName     string
	BootstrapAdminName    string
	Now                   func() time.Time
}
