package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mcmm/internal/config"
	"mcmm/internal/log"
	"mcmm/internal/pgsql"
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

	logger.Info("[step] Initializing PostgreSQL connector")
	connector := pgsql.NewConnector(cfg.DBURL)
	startCtx, startCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startCancel()

	if err := connector.Connect(startCtx); err != nil {
		logger.Fatalf("Failed to connect database: %v", err)
	}
	defer connector.Close()
	logger.Info("[ok] Database connected")

	logger.Info("[step] Building repository set")
	repos := pgsql.NewRepos(connector)
	_ = repos
	logger.Info("[ok] Repositories assembled")

	logger.Info("[ok] Service bootstrap completed")
	logger.Info("--- MCMultiverse Manager is running ---")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("--- Stopping MCMultiverse Manager ---")
	logger.Info("[step] Closing database connector")
	if err := connector.Close(); err != nil {
		logger.Warnf("database close warning: %v", err)
	} else {
		logger.Info("[ok] Database connector closed")
	}
	logger.Info("--- Shutdown complete ---")
}
