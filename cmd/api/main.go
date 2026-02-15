package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"mcmm/internal/cmdreceiver"
	"mcmm/internal/config"
	"mcmm/internal/log"
	"mcmm/internal/pgsql"
	"mcmm/internal/servertap"
	"mcmm/internal/worker"
)

const (
	startupTimeout     = 10 * time.Second
	defaultGameVersion = "1.21.1"
)

func main() {
	log.SetupLogger(log.LevelDebug)
	logger := log.Logger.With("component", "main")
	logger.Info("--- Starting MCMultiverse Manager ---")

	logger.Info("[step] Loading configuration")
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}
	config.LogSummary(cfg)
	logger.Info("[ok] Configuration loaded")

	logger.Info("[step] Preparing runtime directories")
	if err := ensureDirs([]string{cfg.TemplateRootPath, cfg.InstanceRootPath, cfg.VersionRootPath, cfg.ArchiveRootPath}); err != nil {
		logger.Fatalf("Failed to prepare runtime directories: %v", err)
	}
	logger.Infof("[ok] Runtime directories ready (template=%s instance=%s version=%s archive=%s)",
		cfg.TemplateRootPath, cfg.InstanceRootPath, cfg.VersionRootPath, cfg.ArchiveRootPath)

	logger.Info("[step] Initializing PostgreSQL connector")
	connector := pgsql.NewConnector(cfg.DBURL)
	startCtx, startCancel := context.WithTimeout(context.Background(), startupTimeout)
	defer startCancel()
	if err := connector.Connect(startCtx); err != nil {
		logger.Fatalf("Failed to connect database: %v", err)
	}
	defer connector.Close()
	logger.Info("[ok] Database connected")

	logger.Info("[step] Building repository set")
	repos := pgsql.NewRepos(connector)
	logger.Info("[ok] Repositories assembled")

	logger.Info("[step] Initializing worker")
	workerSvc, err := worker.NewWorkerI(repos, worker.Options{
		InstanceRootDir:       cfg.InstanceRootPath,
		VersionRootDir:        cfg.VersionRootPath,
		ComposeTemplateDir:    cfg.VersionRootPath,
		ArchiveRootDir:        cfg.ArchiveRootPath,
		DefaultGameVersion:    defaultGameVersion,
		ServerTapPort:         cfg.MiniServerTapPort,
		InstanceNetwork:       cfg.InstanceNetwork,
		InstanceTapURLPattern: cfg.MiniTapHostPattern,
		ServerTapAuthKey:      cfg.ServerTapKey,
		ServerTapAuthName:     cfg.ServerTapAuthHeader,
		BootstrapAdminName:    cfg.BootstrapAdminName,
		Now:                   time.Now,
	})
	if err != nil {
		logger.Fatalf("Failed to initialize worker: %v", err)
	}
	logger.Info("[ok] Worker initialized")

	logger.Info("[step] Verifying lobby ServerTap connectivity")
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer verifyCancel()
	if err := verifyLobbyServerTap(verifyCtx, cfg, logger); err != nil {
		logger.Warnf("[warn] Lobby ServerTap check failed: %v", err)
	} else {
		logger.Info("[ok] Lobby ServerTap reachable")
	}

	logger.Info("[step] Runtime bootstrap self-check")
	if err := bootstrapRuntimeSelfCheck(context.Background(), cfg, repos, workerSvc, logger); err != nil {
		logger.Errorf("runtime bootstrap self-check failed: %v", err)
	} else {
		logger.Info("[ok] Runtime bootstrap self-check completed")
	}

	logger.Info("[step] Starting HTTP server")
	mux := http.NewServeMux()
	cmdService := cmdreceiver.NewServiceI(repos, workerSvc, defaultGameVersion)
	cmdHandler := cmdreceiver.NewHandlerI(cmdService)
	cmdHandler.Register(mux)
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}
	go func() {
		logger.Infof("[ok] HTTP listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("HTTP server failed: %v", err)
		}
	}()

	logger.Info("[ok] Service bootstrap completed")
	logger.Info("--- MCMultiverse Manager is running ---")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("--- Stopping MCMultiverse Manager ---")
	logger.Info("[step] Shutting down HTTP server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Warnf("http shutdown warning: %v", err)
	} else {
		logger.Info("[ok] HTTP server stopped")
	}

	logger.Info("[step] Closing database connector")
	if err := connector.Close(); err != nil {
		logger.Warnf("database close warning: %v", err)
	} else {
		logger.Info("[ok] Database connector closed")
	}
	logger.Info("--- Shutdown complete ---")
}

func ensureDirs(dirs []string) error {
	for _, dir := range dirs {
		clean := filepath.Clean(dir)
		if clean == "." || clean == "/" {
			return fmt.Errorf("refuse unsafe directory path: %q", dir)
		}
		if err := os.MkdirAll(clean, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", clean, err)
		}
	}
	return nil
}

func verifyLobbyServerTap(ctx context.Context, cfg config.Config, logger interface {
	Infof(string, ...any)
	Warnf(string, ...any)
	Errorf(string, ...any)
}) error {
	conn, err := servertap.NewConnectorWithAuth(cfg.LobbyServerTapURL, 5*time.Second, cfg.ServerTapAuthHeader, cfg.ServerTapKey)
	if err != nil {
		return err
	}
	resp, err := conn.Execute(ctx, servertap.ExecuteRequest{Command: "list"})
	if err != nil {
		return err
	}
	logger.Infof("[main] lobby servertap command=list status=%d body_bytes=%d", resp.StatusCode, len(resp.RawBody))
	return nil
}

func bootstrapRuntimeSelfCheck(ctx context.Context, cfg config.Config, repos pgsql.Repos, w worker.Worker, logger interface {
	Infof(string, ...any)
	Warnf(string, ...any)
	Errorf(string, ...any)
}) error {
	versions, err := detectRunnableVersions(cfg.VersionRootPath)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		logger.Warnf("no runnable versions found under %s", cfg.VersionRootPath)
		return nil
	}

	admin, err := ensureBootstrapAdmin(ctx, repos, cfg.BootstrapAdminUUID, cfg.BootstrapAdminName)
	if err != nil {
		return fmt.Errorf("ensure bootstrap admin: %w", err)
	}

	var failed []string
	logFail := func(version string, msg string, err error) {
		line := fmt.Sprintf("%s: %s: %v", version, msg, err)
		failed = append(failed, line)
		logger.Errorf("[bootstrap] %s", line)
	}

	for _, ver := range versions {
		existingVersion, readErr := repos.GameVersion.Read(ctx, ver)
		if readErr == nil && existingVersion.Status == "verified" {
			logger.Infof("[bootstrap] %s already verified in DB, skip self-check", ver)
			continue
		}
		if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
			logFail(ver, "read game_version", readErr)
			continue
		}

		coreJar, jarErr := detectCoreJarName(cfg.VersionRootPath, ver)
		if jarErr != nil {
			logFail(ver, "detect core jar", jarErr)
			continue
		}
		if err := ensureServerImage(ctx, repos, ver); err != nil {
			logFail(ver, "ensure server image", err)
			continue
		}
		runtimeID := sql.NullString{String: "runtime-" + strings.ReplaceAll(ver, ".", "_"), Valid: true}

		instanceID, err := repos.MapInstance.Create(ctx, pgsql.MapInstance{
			Alias:       "bootstrap-" + strings.ReplaceAll(ver, ".", "-"),
			OwnerID:     admin.ID,
			SourceType:  "empty",
			GameVersion: ver,
			AccessMode:  "privacy",
			Status:      string(worker.StatusWaiting),
		})
		if err != nil {
			alias := "bootstrap-" + strings.ReplaceAll(ver, ".", "-")
			existing, readErr := repos.MapInstance.ReadByAlias(ctx, alias)
			if readErr != nil {
				logFail(ver, "create instance", err)
				continue
			}
			instanceID = existing.ID
		}
		_, _ = repos.InstanceMember.Create(ctx, pgsql.InstanceMember{InstanceID: instanceID, UserID: admin.ID, Role: "owner"})

		if err := w.StartEmpty(ctx, instanceID, ver); err != nil {
			logFail(ver, "start empty", err)
			continue
		}
		if err := w.StopAndArchive(ctx, instanceID); err != nil {
			logFail(ver, "stop/archive", err)
			continue
		}
		// if err := w.DeleteArchived(ctx, instanceID); err != nil {
		// 	logFail(ver, "delete archived", err)
		// 	continue
		// }
		_ = repos.GameVersion.UpsertCheckResult(ctx, ver, runtimeID, coreJar, "verified", sql.NullString{})
	}

	if len(failed) == 0 {
		return nil
	}
	return errors.New(fmt.Sprintf("%d version checks failed", len(failed)))
}

func detectCoreJarName(versionRoot string, version string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(versionRoot, version, "paper-*.jar"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no paper jar found under %s/%s", versionRoot, version)
	}
	sort.Strings(matches)
	return filepath.Base(matches[len(matches)-1]), nil
}

func detectRunnableVersions(versionRoot string) ([]string, error) {
	entries, err := os.ReadDir(versionRoot)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		ver := e.Name()
		jars, _ := filepath.Glob(filepath.Join(versionRoot, ver, "paper-*.jar"))
		if len(jars) > 0 {
			out = append(out, ver)
		}
	}
	sort.Strings(out)
	return out, nil
}

func ensureBootstrapAdmin(ctx context.Context, repos pgsql.Repos, uuid, name string) (pgsql.User, error) {
	u, err := repos.User.ReadByUUID(ctx, uuid)
	if err == nil {
		if u.ServerRole != "admin" {
			u.ServerRole = "admin"
			_ = repos.User.Update(ctx, u)
		}
		return u, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return pgsql.User{}, err
	}
	id, err := repos.User.Create(ctx, pgsql.User{MCUUID: uuid, MCName: name, ServerRole: "admin"})
	if err != nil {
		return pgsql.User{}, err
	}
	return repos.User.Read(ctx, id)
}

func ensureServerImage(ctx context.Context, repos pgsql.Repos, version string) error {
	id := "runtime-" + strings.ReplaceAll(version, ".", "_")
	err := repos.ServerImage.Create(ctx, pgsql.ServerImage{ID: id, Name: "Runtime " + version, GameVersion: version})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil
		}
		return err
	}
	return nil
}
