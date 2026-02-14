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
	serverTapPort      = 4567
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
	logger.Info("[ok] Configuration loaded")
	config.LogSummary(cfg)

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
		InstanceRootDir:    cfg.InstanceRootPath,
		VersionRootDir:     cfg.VersionRootPath,
		ComposeTemplateDir: cfg.VersionRootPath,
		ArchiveRootDir:     cfg.ArchiveRootPath,
		DefaultGameVersion: defaultGameVersion,
		ServerTapPort:      serverTapPort,
		ServerTapAuthKey:   cfg.ServerTapKey,
		ServerTapAuthName:  cfg.ServerTapAuthHeader,
		Now:                time.Now,
	})
	if err != nil {
		logger.Fatalf("Failed to initialize worker: %v", err)
	}
	logger.Info("[ok] Worker initialized")

	logger.Info("[step] Runtime bootstrap self-check")
	if err := bootstrapRuntimeSelfCheck(startCtx, cfg, repos, workerSvc, logger); err != nil {
		logger.Warnf("runtime bootstrap self-check warnings: %v", err)
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

func bootstrapRuntimeSelfCheck(ctx context.Context, cfg config.Config, repos pgsql.Repos, w worker.Worker, logger interface {
	Infof(string, ...any)
	Warnf(string, ...any)
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
	for _, ver := range versions {
		if err := ensureServerImage(ctx, repos, ver); err != nil {
			failed = append(failed, fmt.Sprintf("%s: ensure server image: %v", ver, err))
			continue
		}

		instanceID, err := repos.MapInstance.Create(ctx, pgsql.MapInstance{
			Alias:       "bootstrap-" + strings.ReplaceAll(ver, ".", "-"),
			OwnerID:     admin.ID,
			SourceType:  "empty",
			GameVersion: ver,
			AccessMode:  "privacy",
			Status:      string(worker.StatusWaiting),
		})
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: create instance: %v", ver, err))
			continue
		}
		_, _ = repos.InstanceMember.Create(ctx, pgsql.InstanceMember{InstanceID: instanceID, UserID: admin.ID, Role: "owner"})

		if err := w.StartEmpty(ctx, instanceID, ver); err != nil {
			failed = append(failed, fmt.Sprintf("%s: start empty: %v", ver, err))
			continue
		}
		if err := applyBootstrapCommands(ctx, cfg, admin.MCName, instanceID); err != nil {
			failed = append(failed, fmt.Sprintf("%s: bootstrap commands: %v", ver, err))
		}
		if err := w.StopAndArchive(ctx, instanceID); err != nil {
			failed = append(failed, fmt.Sprintf("%s: stop/archive: %v", ver, err))
			continue
		}
		if err := w.DeleteArchived(ctx, instanceID); err != nil {
			failed = append(failed, fmt.Sprintf("%s: delete archived: %v", ver, err))
		}
	}

	if len(failed) == 0 {
		return nil
	}
	return errors.New(strings.Join(failed, "; "))
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

func applyBootstrapCommands(ctx context.Context, cfg config.Config, ownerName string, instanceID int64) error {
	tapURL := fmt.Sprintf("http://127.0.0.1:%d", worker.InstanceTapPort(instanceID))
	conn, err := servertap.NewConnectorWithAuth(tapURL, 8*time.Second, cfg.ServerTapAuthHeader, cfg.ServerTapKey)
	if err != nil {
		return err
	}
	svc := servertap.NewServiceC(conn)
	if _, err := conn.Execute(ctx, servertap.ExecuteRequest{Command: "whitelist on"}); err != nil {
		return err
	}
	if _, err := conn.Execute(ctx, servertap.ExecuteRequest{Command: "whitelist add " + ownerName}); err != nil {
		return err
	}
	if _, err := svc.OPUser(ctx, ownerName); err != nil {
		return err
	}
	return nil
}
