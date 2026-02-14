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

// Options are fixed deployment inputs for worker runtime.
type Options struct {
	InstanceRootDir    string
	VersionRootDir     string
	ComposeTemplateDir string
	ArchiveRootDir     string
	DefaultGameVersion string
	ServerTapPort      int
	ServerTapAuthKey   string
	ServerTapAuthName  string
	Now                func() time.Time
}
