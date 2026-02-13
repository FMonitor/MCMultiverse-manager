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
}

type Status string

const (
	StatusPendingApproval Status = "pending_approval"
	StatusQueued          Status = "queued"
	StatusPreparingSource Status = "preparing_source"
	StatusProvisioning    Status = "provisioning_instance"
	StatusStarting        Status = "starting_container"
	StatusOn              Status = "on"
	StatusStopping        Status = "stopping_container"
	StatusOff             Status = "off"
	StatusArchiving       Status = "archiving"
	StatusArchived        Status = "archived"
	StatusFailed          Status = "failed"
)

// Options are fixed deployment inputs for worker runtime.
type Options struct {
	InstanceRootDir    string
	VersionRootDir     string
	ComposeTemplateDir string
	DefaultGameVersion string
	ServerTapPort      int
	ServerTapAuthKey   string
	ServerTapAuthName  string
	Now                func() time.Time
}
