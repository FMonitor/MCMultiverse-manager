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
	"mcmm/internal/servertap"
)

const serverTapReadyMaxRetries = 5
const serverTapCommandMaxRetries = 3
const serverTapRetryDelay = 5 * time.Second
const failInstanceUpdateTimeout = 3 * time.Second
const fixedInstanceNetworkName = "mcmultiverse-manager_mcmm-network"

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
	if opts.ServerTapPort <= 0 {
		opts.ServerTapPort = 4567
	}
	if opts.ServerTapTimeout < 0 {
		opts.ServerTapTimeout = 0
	}
	if strings.TrimSpace(opts.InstanceNetwork) != "" && strings.TrimSpace(opts.InstanceNetwork) != fixedInstanceNetworkName {
		log.Component("worker").Warnf("instance_network=%s is ignored; forcing %s", opts.InstanceNetwork, fixedInstanceNetworkName)
	}
	opts.InstanceNetwork = fixedInstanceNetworkName
	if strings.TrimSpace(opts.InstanceTapURLPattern) == "" {
		opts.InstanceTapURLPattern = fmt.Sprintf("http://mcmm-inst-%%d:%d", opts.ServerTapPort)
	}
	if strings.TrimSpace(opts.BootstrapAdminName) == "" {
		opts.BootstrapAdminName = "LCMonitor"
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
		w.failInstanceByID(instanceID, fmt.Sprintf("read instance: %v", err))
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
		w.failInstanceByID(instanceID, fmt.Sprintf("read instance: %v", err))
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
		w.failInstanceByID(instanceID, fmt.Sprintf("read instance: %v", err))
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
		w.failInstanceByID(instanceID, fmt.Sprintf("read instance: %v", err))
		return fmt.Errorf("read instance: %w", err)
	}

	if err := w.setStatus(ctx, &inst, StatusStopping); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("set stopping: %v", err))
		return err
	}
	if err := w.stopCompose(ctx, inst.ID); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("stop compose: %v", err))
		return err
	}
	if err := w.setStatus(ctx, &inst, StatusOff); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("set off: %v", err))
		return err
	}
	if err := w.archiveWorld(inst.ID); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("archive world: %v", err))
		return err
	}

	inst.ArchivedAt = toNullTime(w.opts.Now())
	if err := w.setStatus(ctx, &inst, StatusArchived); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("set archived: %v", err))
		return err
	}
	return nil
}

func (w *WorkerI) DeleteArchived(ctx context.Context, instanceID int64) error {
	inst, err := w.repos.MapInstance.Read(ctx, instanceID)
	if err != nil {
		w.failInstanceByID(instanceID, fmt.Sprintf("read instance: %v", err))
		return fmt.Errorf("read instance: %w", err)
	}
	if Status(inst.Status) != StatusArchived {
		return fmt.Errorf("instance %d is not archived (status=%s)", instanceID, inst.Status)
	}
	archiveDir := w.archiveDirPath(instanceID)
	_ = os.RemoveAll(archiveDir)
	_ = os.RemoveAll(instanceDir(w.opts.InstanceRootDir, instanceID))
	return nil
}

func (w *WorkerI) runStartFlow(ctx context.Context, inst pgsql.MapInstance, gameVersion string, sourceWorldPath string) error {
	if err := w.setStatus(ctx, &inst, StatusPreparing); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("set preparing: %v", err))
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
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("set starting: %v", err))
		return err
	}
	if err := w.startCompose(ctx, inst.ID); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("start compose: %v", err))
		return err
	}
	time.Sleep(10 * time.Second)
	if err := w.configureInstanceAccess(ctx, inst); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("configure access: %v", err))
		return err
	}

	inst.GameVersion = gameVersion
	inst.ArchivedAt = toNullTimeZero()
	inst.LastActiveAt = toNullTime(w.opts.Now())
	inst.HealthStatus = string(HealthHealthy)
	inst.LastErrorMsg = sql.NullString{}
	inst.LastHealthAt = toNullTime(w.opts.Now())
	if err := w.setStatus(ctx, &inst, StatusOn); err != nil {
		_ = w.failInstance(ctx, &inst, fmt.Sprintf("set on: %v", err))
		return err
	}
	return nil
}

func (w *WorkerI) configureInstanceAccess(ctx context.Context, inst pgsql.MapInstance) error {
	tapURL := fmt.Sprintf(w.opts.InstanceTapURLPattern, inst.ID)
	conn, err := servertap.NewConnectorWithAuth(tapURL, w.opts.ServerTapTimeout, w.opts.ServerTapAuthName, w.opts.ServerTapAuthKey)
	if err != nil {
		return err
	}

	var lastErr error
	for i := 0; i < serverTapReadyMaxRetries; i++ {
		lastErr = executeServerTapWithRetry(ctx, conn, inst.ID, "whitelist on", 1, w.logger)
		if lastErr == nil {
			break
		}
		w.logger.Warnf("instance=%d servertap ready check failed (%d/%d): %v", inst.ID, i+1, serverTapReadyMaxRetries, lastErr)
		time.Sleep(serverTapRetryDelay)
	}
	if lastErr != nil {
		return lastErr
	}

	processed := map[string]struct{}{}
	// Grant all DB admins OP+whitelist on each instance.
	admins, err := w.repos.User.ListByRole(ctx, "admin")
	if err != nil {
		return err
	}
	if len(admins) == 0 {
		w.logger.Warnf("instance=%d no admin users found in DB", inst.ID)
	} else {
		names := make([]string, 0, len(admins))
		for _, a := range admins {
			names = append(names, a.MCName)
		}
		w.logger.Infof("instance=%d granting admin access to %d users: %s", inst.ID, len(admins), strings.Join(names, ","))
	}
	for _, a := range admins {
		if err := allowAndOpUser(ctx, conn, inst.ID, a.MCName, processed, w.logger); err != nil {
			return err
		}
	}
	// Backward compatibility: ensure configured bootstrap admin is also granted.
	admin := strings.TrimSpace(w.opts.BootstrapAdminName)
	if admin != "" {
		if err := allowAndOpUser(ctx, conn, inst.ID, admin, processed, w.logger); err != nil {
			return err
		}
	}

	owner, err := w.repos.User.Read(ctx, inst.OwnerID)
	if err != nil {
		return err
	}
	if err := allowAndOpUser(ctx, conn, inst.ID, owner.MCName, processed, w.logger); err != nil {
		return err
	}
	return nil
}

func allowAndOpUser(
	ctx context.Context,
	conn *servertap.Connector,
	instanceID int64,
	name string,
	processed map[string]struct{},
	logger interface {
		Warnf(string, ...any)
	},
) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	key := strings.ToLower(name)
	if _, exists := processed[key]; exists {
		return nil
	}
	if err := executeServerTapWithRetry(ctx, conn, instanceID, "whitelist add "+name, serverTapCommandMaxRetries, logger); err != nil {
		return err
	}
	if err := executeServerTapWithRetry(ctx, conn, instanceID, servertap.NewCommandBuilder("op").Arg(name).Build(), serverTapCommandMaxRetries, logger); err != nil {
		return err
	}
	processed[key] = struct{}{}
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
	inst.HealthStatus = string(classifyHealthFailure(reason))
	inst.LastErrorMsg = sql.NullString{String: reason, Valid: true}
	inst.LastHealthAt = toNullTime(w.opts.Now())
	inst.Status = string(StatusOff)
	inst.UpdatedAt = w.opts.Now()
	dbCtx, cancel := context.WithTimeout(context.Background(), failInstanceUpdateTimeout)
	defer cancel()
	return w.repos.MapInstance.Update(dbCtx, *inst)
}

func (w *WorkerI) failInstanceByID(instanceID int64, reason string) {
	w.logger.Errorf("instance=%d failed: %s", instanceID, reason)
	dbCtx, cancel := context.WithTimeout(context.Background(), failInstanceUpdateTimeout)
	defer cancel()
	inst, err := w.repos.MapInstance.Read(dbCtx, instanceID)
	if err != nil {
		w.logger.Errorf("instance=%d fail-state read error: %v", instanceID, err)
		return
	}
	if err := w.failInstance(dbCtx, &inst, reason); err != nil {
		w.logger.Errorf("instance=%d fail-state update error: %v", instanceID, err)
	}
}

func (w *WorkerI) prepareInstanceVolume(instanceID int64, sourceWorldPath string) error {
	base := instanceDir(w.opts.InstanceRootDir, instanceID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	whitelistFile := filepath.Join(base, "whitelist.json")
	if err := ensureFileWithDefault(whitelistFile, []byte("[]\n")); err != nil {
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
	coreSrc := filepath.Join(versionDir, jarName)
	cacheSrc := filepath.Join(versionDir, "cache")
	versionsSrc := filepath.Join(versionDir, "versions")
	coreDst := filepath.Join(base, jarName)
	cacheDst := filepath.Join(base, "cache")
	versionsDst := filepath.Join(base, "versions")

	if err := copyFile(coreSrc, coreDst, 0o644); err != nil {
		return fmt.Errorf("copy core jar: %w", err)
	}
	if isDir(cacheSrc) {
		if err := os.RemoveAll(cacheDst); err != nil {
			return err
		}
		if err := copyDir(cacheSrc, cacheDst); err != nil {
			return fmt.Errorf("copy cache: %w", err)
		}
	} else if err := os.MkdirAll(cacheDst, 0o755); err != nil {
		return err
	}
	if isDir(versionsSrc) {
		if err := os.RemoveAll(versionsDst); err != nil {
			return err
		}
		if err := copyDir(versionsSrc, versionsDst); err != nil {
			return fmt.Errorf("copy versions: %w", err)
		}
	} else if err := os.MkdirAll(versionsDst, 0o755); err != nil {
		return err
	}

	coreMount, err := filepath.Abs(coreDst)
	if err != nil {
		return err
	}
	cacheMount, err := filepath.Abs(cacheDst)
	if err != nil {
		return err
	}
	versionsMount, err := filepath.Abs(versionsDst)
	if err != nil {
		return err
	}
	worldMount, err := filepath.Abs(filepath.Join(base, "world"))
	if err != nil {
		return err
	}
	netherMount, err := filepath.Abs(filepath.Join(base, "world_nether"))
	if err != nil {
		return err
	}
	endMount, err := filepath.Abs(filepath.Join(base, "world_the_end"))
	if err != nil {
		return err
	}
	whitelistMount, err := filepath.Abs(filepath.Join(base, "whitelist.json"))
	if err != nil {
		return err
	}

	composePath := filepath.Join(base, "docker-compose.yml")
	content := fmt.Sprintf(`services:
  mcmm-inst-%d:
    image: %s
    container_name: mcmm-inst-%d
    restart: unless-stopped
    environment:
      JAVA_TOOL_OPTIONS: "-Xms1G -Xmx2G"
      PAPER_JAR: "%s"
    volumes:
      - %s:/data/server/%s:ro
      - %s:/data/server/cache
      - %s:/data/server/versions
      - %s:/data/server/world
      - %s:/data/server/world_nether
      - %s:/data/server/world_the_end
      - %s:/data/server/whitelist.json
    networks:
      - %s
networks:
  %s:
    external: true
`, instanceID, imageTag, instanceID, jarName,
		coreMount, jarName,
		cacheMount,
		versionsMount,
		worldMount,
		netherMount,
		endMount,
		whitelistMount,
		w.opts.InstanceNetwork,
		w.opts.InstanceNetwork,
	)
	return os.WriteFile(composePath, []byte(content), 0o644)
}

func (w *WorkerI) startCompose(ctx context.Context, instanceID int64) error {
	composePath := filepath.Join(instanceDir(w.opts.InstanceRootDir, instanceID), "docker-compose.yml")
	if err := ensureDockerNetwork(ctx, w.opts.InstanceNetwork); err != nil {
		return fmt.Errorf("ensure network %s: %w", w.opts.InstanceNetwork, err)
	}
	return runCmd(ctx, "docker", "compose", "-f", composePath, "up", "-d")
}

func (w *WorkerI) stopCompose(ctx context.Context, instanceID int64) error {
	composePath := filepath.Join(instanceDir(w.opts.InstanceRootDir, instanceID), "docker-compose.yml")
	return runCmd(ctx, "docker", "compose", "-f", composePath, "down")
}

func (w *WorkerI) archiveWorld(instanceID int64) error {
	src := instanceDir(w.opts.InstanceRootDir, instanceID)
	if err := os.MkdirAll(w.opts.ArchiveRootDir, 0o755); err != nil {
		return err
	}
	dst := w.archiveDirPath(instanceID)
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := moveDir(src, dst); err != nil {
		return err
	}
	w.logger.Infof("instance=%d archived into %s", instanceID, dst)
	return nil
}

func (w *WorkerI) archiveDirPath(instanceID int64) string {
	return filepath.Join(w.opts.ArchiveRootDir, fmt.Sprintf("instance-%d", instanceID))
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
		StatusOff:       {StatusPreparing: true, StatusStarting: true, StatusArchived: true},
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

func runCmd(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w, output=%s", bin, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureDockerNetwork(ctx context.Context, network string) error {
	network = strings.TrimSpace(network)
	if network == "" {
		return nil
	}
	inspectErr := runCmd(ctx, "docker", "network", "inspect", network)
	if inspectErr == nil {
		return nil
	}
	return runCmd(ctx, "docker", "network", "create", "--driver", "bridge", network)
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

func ensureFileWithDefault(path string, content []byte) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
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

func moveDir(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-device fallback: copy then delete source.
	if err := copyDir(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func toNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: true}
}

func toNullTimeZero() sql.NullTime {
	return sql.NullTime{}
}

func classifyHealthFailure(reason string) HealthStatus {
	lower := strings.ToLower(reason)
	if strings.Contains(lower, "context deadline exceeded") ||
		strings.Contains(lower, "servertap") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "i/o timeout") {
		return HealthUnreachable
	}
	return HealthStartFailed
}

func executeServerTapWithRetry(
	ctx context.Context,
	conn *servertap.Connector,
	instanceID int64,
	command string,
	maxRetries int,
	logger interface {
		Warnf(string, ...any)
	},
) error {
	if maxRetries <= 0 {
		maxRetries = 1
	}
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		_, lastErr = conn.Execute(ctx, servertap.ExecuteRequest{Command: command})
		if lastErr == nil {
			return nil
		}
		if i < maxRetries-1 {
			logger.Warnf("instance=%d servertap command failed (%d/%d) cmd=%q err=%v", instanceID, i+1, maxRetries, command, lastErr)
			time.Sleep(serverTapRetryDelay)
		}
	}
	return lastErr
}

func Now() time.Time {
	return time.Now()
}
