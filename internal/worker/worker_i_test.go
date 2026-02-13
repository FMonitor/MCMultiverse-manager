package worker

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcmm/internal/pgsql"
)

type mapInstanceRepoMock struct {
	readFn   func(ctx context.Context, id int64) (pgsql.MapInstance, error)
	updateFn func(ctx context.Context, inst pgsql.MapInstance) error
}

func (m mapInstanceRepoMock) Create(ctx context.Context, inst pgsql.MapInstance) (int64, error) {
	return 0, nil
}
func (m mapInstanceRepoMock) Read(ctx context.Context, id int64) (pgsql.MapInstance, error) {
	return m.readFn(ctx, id)
}
func (m mapInstanceRepoMock) Update(ctx context.Context, inst pgsql.MapInstance) error {
	return m.updateFn(ctx, inst)
}
func (m mapInstanceRepoMock) Delete(ctx context.Context, id int64) error { return nil }

func TestRuntimeImageByVersion(t *testing.T) {
	tests := []struct {
		version string
		want    string
		ok      bool
	}{
		{"1.16.5", "mcmm-mini:java16-jlink", true},
		{"1.18.2", "mcmm-mini:java17-jlink", true},
		{"1.20.1", "mcmm-mini:java17-jlink", true},
		{"1.21.1", "mcmm-mini:java21-jlink", true},
		{"1.15.2", "", false},
	}
	for _, tc := range tests {
		got, err := runtimeImageByVersion(tc.version)
		if tc.ok && err != nil {
			t.Fatalf("version=%s unexpected error: %v", tc.version, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("version=%s expected error", tc.version)
		}
		if tc.ok && got != tc.want {
			t.Fatalf("version=%s got=%s want=%s", tc.version, got, tc.want)
		}
	}
}

func TestCanTransit(t *testing.T) {
	if !canTransit(StatusPendingApproval, StatusQueued) {
		t.Fatalf("pending_approval -> queued should be allowed")
	}
	if canTransit(StatusOn, StatusArchived) {
		t.Fatalf("on -> archived should not be allowed")
	}
	if !canTransit(StatusOff, StatusArchiving) {
		t.Fatalf("off -> archiving should be allowed")
	}
}

func TestPrepareComposeFile(t *testing.T) {
	tmp := t.TempDir()
	versionDir := filepath.Join(tmp, "version", "1.21.1")
	if err := os.MkdirAll(filepath.Join(versionDir, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(versionDir, "versions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "paper-1.21.1-133.jar"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	instRoot := filepath.Join(tmp, "instance")
	if err := os.MkdirAll(filepath.Join(instRoot, "101"), 0o755); err != nil {
		t.Fatal(err)
	}

	w, err := NewWorkerI(pgsql.Repos{}, Options{
		InstanceRootDir:    instRoot,
		VersionRootDir:     filepath.Join(tmp, "version"),
		ComposeTemplateDir: filepath.Join(tmp, "compose"),
		DefaultGameVersion: "1.21.1",
		ServerTapPort:      4567,
		Now:                time.Now,
	})
	if err != nil {
		t.Fatalf("new worker failed: %v", err)
	}
	if err := w.prepareComposeFile(101, "1.21.1"); err != nil {
		t.Fatalf("prepare compose failed: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(instRoot, "101", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read compose failed: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "mcmm-mini:java21-jlink") {
		t.Fatalf("compose should include java21 image, got:\n%s", content)
	}
	if !strings.Contains(content, "/data/server/cache") || !strings.Contains(content, "/data/server/versions") {
		t.Fatalf("compose should include cache/versions mounts, got:\n%s", content)
	}
}

func TestSetStatusWithMockRepo(t *testing.T) {
	var updated pgsql.MapInstance
	mock := mapInstanceRepoMock{
		readFn: func(ctx context.Context, id int64) (pgsql.MapInstance, error) {
			return pgsql.MapInstance{}, nil
		},
		updateFn: func(ctx context.Context, inst pgsql.MapInstance) error {
			updated = inst
			return nil
		},
	}
	repos := pgsql.Repos{MapInstance: mock}
	now := time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC)
	w, err := NewWorkerI(repos, Options{
		InstanceRootDir:    t.TempDir(),
		VersionRootDir:     t.TempDir(),
		ComposeTemplateDir: t.TempDir(),
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("new worker failed: %v", err)
	}

	inst := pgsql.MapInstance{
		ID:         1,
		Status:     string(StatusQueued),
		TemplateID: sql.NullInt64{},
	}
	if err := w.setStatus(context.Background(), &inst, StatusProvisioning); err != nil {
		t.Fatalf("set status failed: %v", err)
	}
	if updated.Status != string(StatusProvisioning) {
		t.Fatalf("updated status mismatch: got=%s", updated.Status)
	}
	if !updated.UpdatedAt.Equal(now) {
		t.Fatalf("updated_at mismatch: got=%v want=%v", updated.UpdatedAt, now)
	}
}
