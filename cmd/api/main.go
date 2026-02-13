package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"mcmm/internal/cmdreceiver"
	"mcmm/internal/config"
	"mcmm/internal/log"
	"mcmm/internal/pgsql"
	"mcmm/internal/worker"
)

const (
	startupTimeout = 10 * time.Second

	// Runtime directory layout (fixed paths).
	instanceRootDir   = "deploy/instance"
	mapStorageRootDir = "deploy/version"
	composeRootDir    = "deploy/version"

	// Unified ServerTap defaults for all mini servers.
	serverTapPort       = 4567
	serverTapAuthHeader = "key"
	defaultGameVersion  = "1.21.1"
)

var requiredDirs = []string{
	instanceRootDir,
	mapStorageRootDir,
	composeRootDir,
}

func main() {
	log.SetupLogger(log.LevelDebug)
	logger := log.Logger.With("component", "main")
	logger.Info("--- Starting MCMultiverse Manager ---")

	logger.Info("[step] Preparing fixed runtime directories")
	if err := ensureDirs(requiredDirs); err != nil {
		logger.Fatalf("Failed to prepare runtime directories: %v", err)
	}
	logger.Infof("[ok] Runtime directories ready (instance=%s map=%s compose=%s)",
		instanceRootDir, mapStorageRootDir, composeRootDir)

	logger.Info("[step] Loading configuration")
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}
	logger.Info("[ok] Configuration loaded")

	logger.Infof("[const] unified servertap auth_header=%s port=%d", serverTapAuthHeader, serverTapPort)
	if cfg.ServerTapKey == "" {
		logger.Warn("[config] servertap_key is empty")
	}

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
		InstanceRootDir:    instanceRootDir,
		VersionRootDir:     mapStorageRootDir,
		ComposeTemplateDir: composeRootDir,
		DefaultGameVersion: defaultGameVersion,
		ServerTapPort:      serverTapPort,
		ServerTapAuthKey:   cfg.ServerTapKey,
		ServerTapAuthName:  serverTapAuthHeader,
		Now:                time.Now,
	})
	if err != nil {
		logger.Fatalf("Failed to initialize worker: %v", err)
	}
	_ = workerSvc
	logger.Info("[ok] Worker initialized")

	logger.Info("[step] Starting HTTP server")
	mux := http.NewServeMux()
	cmdService := cmdreceiver.NewServiceI(repos, workerSvc, defaultGameVersion)
	cmdHandler := cmdreceiver.NewHandlerI(cmdService)
	cmdHandler.Register(mux)
	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: mux,
	}
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

	logger.Info("⚙ Closing database connector")
	if err := connector.Close(); err != nil {
		logger.Warnf("database close warning: %v", err)
	} else {
		logger.Info("√ Database connector closed")
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
