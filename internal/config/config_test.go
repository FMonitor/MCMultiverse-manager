package config

import (
	"os"
	"path/filepath"
	"testing"

	"mcmm/internal/log"
)

func TestLoadFromFileLogsFields(t *testing.T) {
	log.SetupLogger(log.LevelDebug)
	logger := log.Logger.With("component", "test")

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	content := []byte("http_addr: :8080\n" +
		"database_url: postgres://user:pass@localhost:5432/db\n" +
		"lobby_servertap_url: http://localhost:9000\n")

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	logger.Infof("http_addr=%s", cfg.HTTPAddr)
	logger.Infof("database_url=%s", cfg.DBURL)
	logger.Infof("lobby_servertap_url=%s", cfg.LobbyServerTapURL)
}
