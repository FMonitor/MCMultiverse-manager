package worker

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mcmm/internal/log"
	"mcmm/internal/pgsql"
)

type WorkerI struct {
	repos  pgsql.Repos
	opts   Options
	logger interface {
		Infof(string, ...any)
		Warnf(string, ...any)
		Errorf(string, ...any)
	}
}

func NewWorkerI(repos pgsql.Repos, opts Options) (*WorkerI, error) {
	if opts.InstanceRootDir == "" || opts.VersionRootDir == "" || opts.ComposeTemplateDir == "" {
		return nil, errors.New("worker options: required paths must be set")
	}
	if opts.ArchiveRootDir == "" {
		opts.ArchiveRootDir = "deploy/archived"
	}
	if opts.DefaultGameVersion == "" {
		opts.DefaultGameVersion = "1.21.1"
	}
	if opts.Now == nil {
		opts.Now = Now
	}
	return &WorkerI{
		repos:  repos,
		opts:   opts,
		logger: log.Component("worker"),
	}, nil
}

func (w *WorkerI) StartFromTemplate(ctx context.Context, instanceID int64, template pgsql.MapTemplate) error {
	inst, err := w.repos.MapInstance.Read(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("read instance: %w", err)
	}
	version := inst.GameVersion
	if version == "" || version == "unknown" {
		version = template.GameVersion
		if version == "" {
			version = w.opts.DefaultGameVersion
		}
	}
	return w.runStartFlow(ctx, inst, version, template.BlobPath)
}

func (w *WorkerI) StartFromUpload(ctx context.Context, instanceID int64, uploadWorldPath string) error {
	inst, err := w.repos.MapInstance.Read(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("read instance: %w", err)
	}
	version := inst.GameVersion
	if version == "" || version == "unknown" {
		version = w.opts.DefaultGameVersion
	}
	return w.runStartFlow(ctx, inst, version, uploadWorldPath)
}

func (w *WorkerI) StartEmpty(ctx context.Context, instanceID int64, gameVersion string) error {
	inst, err := w.repos.MapInstance.Read(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("read instance: %w", err)
	}
	if strings.TrimSpace(gameVersion) == "" {
		gameVersion = w.opts.DefaultGameVersion
	}
	return w.runStartFlow(ctx, inst, gameVersion, "")
}

func (w *WorkerI) StopAndArchive(ctx context.Context, instanceID int64) error {
	inst, err := w.repos.MapInstance.Read(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("read instance: %w", err)
	}

	if err := w.setStatus(ctx, &inst, StatusStopping); err != nil {
		return err
	}
	if err := w.stopCompose(ctx, inst.ID); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("stop compose: %v", err))
		return err
	}
	if err := w.setStatus(ctx, &inst, StatusOff); err != nil {
		return err
	}
	if err := w.archiveWorld(inst.ID); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("archive world: %v", err))
		return err
	}

	inst.ArchivedAt = toNullTime(w.opts.Now())
	if err := w.setStatus(ctx, &inst, StatusArchived); err != nil {
		return err
	}
	return nil
}

func (w *WorkerI) DeleteArchived(ctx context.Context, instanceID int64) error {
	inst, err := w.repos.MapInstance.Read(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("read instance: %w", err)
	}
	if Status(inst.Status) != StatusArchived {
		return fmt.Errorf("instance %d is not archived (status=%s)", instanceID, inst.Status)
	}
	archiveFile := w.archivePath(instanceID)
	_ = os.Remove(archiveFile)
	_ = os.RemoveAll(instanceDir(w.opts.InstanceRootDir, instanceID))
	return nil
}

func (w *WorkerI) runStartFlow(ctx context.Context, inst pgsql.MapInstance, gameVersion string, sourceWorldPath string) error {
	if err := w.setStatus(ctx, &inst, StatusPreparing); err != nil {
		return err
	}
	if err := w.prepareInstanceVolume(inst.ID, sourceWorldPath); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("prepare instance volume: %v", err))
		return err
	}
	if err := w.prepareComposeFile(inst.ID, gameVersion); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("prepare compose: %v", err))
		return err
	}
	if err := w.setStatus(ctx, &inst, StatusStarting); err != nil {
		return err
	}
	if err := w.startCompose(ctx, inst.ID); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("start compose: %v", err))
		return err
	}

	inst.GameVersion = gameVersion
	inst.ArchivedAt = toNullTimeZero()
	inst.LastActiveAt = toNullTime(w.opts.Now())
	if err := w.setStatus(ctx, &inst, StatusOn); err != nil {
		return err
	}
	return nil
}

func (w *WorkerI) setStatus(ctx context.Context, inst *pgsql.MapInstance, to Status) error {
	from := Status(inst.Status)
	if inst.Status == "" {
		from = StatusWaiting
	}
	if !canTransit(from, to) {
		return fmt.Errorf("invalid status transition: %s -> %s", from, to)
	}
	inst.Status = string(to)
	inst.UpdatedAt = w.opts.Now()
	w.logger.Infof("instance=%d status: %s -> %s", inst.ID, from, to)
	return w.repos.MapInstance.Update(ctx, *inst)
}

func (w *WorkerI) failInstance(ctx context.Context, inst *pgsql.MapInstance, reason string) error {
	w.logger.Errorf("instance=%d failed: %s", inst.ID, reason)
	inst.Status = string(StatusOff)
	inst.UpdatedAt = w.opts.Now()
	return w.repos.MapInstance.Update(ctx, *inst)
}

func (w *WorkerI) prepareInstanceVolume(instanceID int64, sourceWorldPath string) error {
	base := instanceDir(w.opts.InstanceRootDir, instanceID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	worldDir := filepath.Join(base, "world")
	netherDir := filepath.Join(base, "world_nether")
	endDir := filepath.Join(base, "world_the_end")
	for _, d := range []string{worldDir, netherDir, endDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	if strings.TrimSpace(sourceWorldPath) == "" {
		return nil
	}
	templateRoot, worldSrc := resolveTemplateWorldPaths(sourceWorldPath)
	if !isDir(worldSrc) {
		return fmt.Errorf("source world path is not dir: %s", worldSrc)
	}
	if err := clearDir(worldDir); err != nil {
		return err
	}
	if err := clearDir(netherDir); err != nil {
		return err
	}
	if err := clearDir(endDir); err != nil {
		return err
	}
	if err := copyDir(worldSrc, worldDir); err != nil {
		return err
	}
	// Optional dimensions: some template only has overworld.
	netherSrc := filepath.Join(templateRoot, "world_nether")
	if isDir(netherSrc) {
		if err := copyDir(netherSrc, netherDir); err != nil {
			return err
		}
	}
	endSrc := filepath.Join(templateRoot, "world_the_end")
	if isDir(endSrc) {
		if err := copyDir(endSrc, endDir); err != nil {
			return err
		}
	}
	w.logger.Infof("instance=%d prepared volume from template=%s", instanceID, templateRoot)
	return nil
}

func (w *WorkerI) prepareComposeFile(instanceID int64, version string) error {
	versionDir := filepath.Join(w.opts.VersionRootDir, version)
	jarName, err := detectPaperJar(versionDir)
	if err != nil {
		return err
	}
	imageTag, err := runtimeImageByVersion(version)
	if err != nil {
		return err
	}

	base := instanceDir(w.opts.InstanceRootDir, instanceID)
	composePath := filepath.Join(base, "docker-compose.yml")
	gamePort, tapPort := instancePorts(instanceID)
	content := fmt.Sprintf(`services:
  mcmm-inst-%d:
    image: %s
    container_name: mcmm-inst-%d
    restart: unless-stopped
    environment:
      JAVA_TOOL_OPTIONS: "-Xms1G -Xmx2G"
    ports:
      - "%d:25565"
      - "%d:%d"
    volumes:
      - %s:/data/server/%s:ro
      - %s:/data/server/cache
      - %s:/data/server/versions
      - %s:/data/server/world
      - %s:/data/server/world_nether
      - %s:/data/server/world_the_end
`, instanceID, imageTag, instanceID, gamePort, tapPort, w.opts.ServerTapPort,
		filepath.Join(versionDir, jarName), jarName,
		filepath.Join(versionDir, "cache"),
		filepath.Join(versionDir, "versions"),
		filepath.Join(base, "world"),
		filepath.Join(base, "world_nether"),
		filepath.Join(base, "world_the_end"),
	)
	return os.WriteFile(composePath, []byte(content), 0o644)
}

func (w *WorkerI) startCompose(ctx context.Context, instanceID int64) error {
	composePath := filepath.Join(instanceDir(w.opts.InstanceRootDir, instanceID), "docker-compose.yml")
	return runCmd(ctx, "docker", "compose", "-f", composePath, "up", "-d")
}

func (w *WorkerI) stopCompose(ctx context.Context, instanceID int64) error {
	composePath := filepath.Join(instanceDir(w.opts.InstanceRootDir, instanceID), "docker-compose.yml")
	return runCmd(ctx, "docker", "compose", "-f", composePath, "down")
}

func (w *WorkerI) archiveWorld(instanceID int64) error {
	base := instanceDir(w.opts.InstanceRootDir, instanceID)
	src := filepath.Join(base, "world")
	if err := os.MkdirAll(w.opts.ArchiveRootDir, 0o755); err != nil {
		return err
	}
	dst := w.archivePath(instanceID)
	return tarGzDir(src, dst)
}

func (w *WorkerI) archivePath(instanceID int64) string {
	return filepath.Join(w.opts.ArchiveRootDir, fmt.Sprintf("instance-%d.tar.gz", instanceID))
}

func canTransit(from, to Status) bool {
	if from == Status("") {
		from = StatusWaiting
	}
	allowed := map[Status]map[Status]bool{
		StatusWaiting:   {StatusPreparing: true},
		StatusPreparing: {StatusStarting: true, StatusOff: true},
		StatusStarting:  {StatusOn: true, StatusOff: true},
		StatusOn:        {StatusStopping: true},
		StatusStopping:  {StatusOff: true},
		StatusOff:       {StatusStarting: true, StatusArchived: true},
		StatusArchived:  {},
	}
	if next, ok := allowed[from]; ok {
		return next[to]
	}
	return false
}

func runtimeImageByVersion(version string) (string, error) {
	switch {
	case strings.HasPrefix(version, "1.16"):
		return "mcmm-mini:java16-jlink", nil
	case strings.HasPrefix(version, "1.17"), strings.HasPrefix(version, "1.18"), strings.HasPrefix(version, "1.19"), strings.HasPrefix(version, "1.20"):
		return "mcmm-mini:java17-jlink", nil
	case strings.HasPrefix(version, "1.21"):
		return "mcmm-mini:java21-jlink", nil
	default:
		return "", fmt.Errorf("unsupported game version: %s", version)
	}
}

func detectPaperJar(versionDir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(versionDir, "paper-*.jar"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no paper jar found under %s", versionDir)
	}
	return filepath.Base(matches[0]), nil
}

func instanceDir(root string, id int64) string {
	return filepath.Join(root, strconv.FormatInt(id, 10))
}

func instancePorts(id int64) (game int64, tap int64) {
	// deterministic per instance to reduce collision in local dev.
	return 30000 + (id % 1000), 31000 + (id % 1000)
}

func resolveTemplateWorldPaths(input string) (templateRoot string, worldPath string) {
	clean := filepath.Clean(input)
	// If caller passes ".../<template>/world", infer template root.
	if filepath.Base(clean) == "world" {
		parent := filepath.Dir(clean)
		if isDir(parent) {
			return parent, clean
		}
	}
	// If caller passes template root, prefer "<root>/world" when present.
	candidate := filepath.Join(clean, "world")
	if isDir(candidate) {
		return clean, candidate
	}
	// Fallback: treat input itself as world dir.
	return filepath.Dir(clean), clean
}

func InstanceTapPort(id int64) int64 {
	_, tap := instancePorts(id)
	return tap
}

func runCmd(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w, output=%s", bin, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func clearDir(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(path, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

func tarGzDir(srcDir, dstTarGz string) error {
	f, err := os.Create(dstTarGz)
	if err != nil {
		return err
	}
	defer f.Close()
	gzw := gzip.NewWriter(f)
	defer gzw.Close()
	tw := tar.NewWriter(gzw)
	defer tw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tw, file)
		return err
	})
}

func toNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: true}
}

func toNullTimeZero() sql.NullTime {
	return sql.NullTime{}
}

func Now() time.Time {
	return time.Now()
}
