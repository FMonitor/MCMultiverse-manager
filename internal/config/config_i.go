package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	ilog "mcmm/internal/log"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTPAddr            string         `yaml:"http_addr"`
	DBURL               string         `yaml:"database_url"`
	ServerTap           string         `yaml:"servertap_url"`
	ServerTapKey        string         `yaml:"servertap_key"`
	ServerTapAuthHeader string         `yaml:"servertap_auth_header"`
	TemplateRootPath    string         `yaml:"template_root_path"`
	VersionRootPath     string         `yaml:"version_root_path"`
	InstanceRootPath    string         `yaml:"instance_root_path"`
	ArchiveRootPath     string         `yaml:"archive_root_path"`
	BootstrapAdminName  string         `yaml:"bootstrap_admin_name"`
	BootstrapAdminUUID  string         `yaml:"bootstrap_admin_uuid"`
	ServerPath          string         `yaml:"serverpath"`
	Servers             []ServerConfig `yaml:"servers"`
}

type ServerConfig struct {
	ID                  string `yaml:"id"`
	Name                string `yaml:"name"`
	GameVersion         string `yaml:"game_version"`
	RootPath            string `yaml:"root_path"`
	ServerTapURL        string `yaml:"servertap_url"`
	ServerTapKey        string `yaml:"servertap_key"`
	ServerTapAuthHeader string `yaml:"servertap_auth_header"`
	Enabled             bool   `yaml:"enabled"`
}

func Load() (Config, error) {
	logger := ilog.Component("config")
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		logger.Infof("CONFIG_PATH is set, loading: %s", p)
		return LoadFromFile(p)
	}
	path := resolveDefaultConfigPath()
	logger.Infof("using resolved config path: %s", path)
	return LoadFromFile(path)
}

func LoadFromFile(path string) (Config, error) {
	logger := ilog.Component("config")
	logger.Infof("reading config file: %s", path)
	b, err := os.ReadFile(path)
	if err != nil {
		logger.Errorf("read failed: %v", err)
		return Config{}, fmt.Errorf("failed to read config %s: %w", path, err)
	}

	var cfg Config
	logger.Infof("parsing yaml")
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		logger.Errorf("yaml parse failed: %v", err)
		return Config{}, fmt.Errorf("failed to parse yaml %s: %w", path, err)
	}

	logger.Infof("validating fields")
	if err := cfg.Validate(); err != nil {
		logger.Errorf("validation failed: %v", err)
		return Config{}, fmt.Errorf("invalid config %s: %w", path, err)
	}

	logger.Infof("config loaded successfully (http_addr=%s)", cfg.HTTPAddr)
	return cfg, nil
}

func (c *Config) Validate() error {
	if c.HTTPAddr == "" {
		return errors.New("http_addr is required")
	}
	if c.DBURL == "" {
		return errors.New("database_url is required")
	}
	if c.VersionRootPath == "" {
		c.VersionRootPath = "deploy/version"
	}
	if c.TemplateRootPath == "" {
		c.TemplateRootPath = "deploy/template"
	}
	if c.InstanceRootPath == "" {
		c.InstanceRootPath = "deploy/instance"
	}
	if c.ArchiveRootPath == "" {
		c.ArchiveRootPath = "deploy/archived"
	}
	if c.BootstrapAdminName == "" {
		c.BootstrapAdminName = "admin"
	}
	if c.BootstrapAdminUUID == "" {
		c.BootstrapAdminUUID = "00000000-0000-4000-8000-000000000001"
	}
	if len(c.Servers) == 0 && c.ServerTap == "" {
		return errors.New("servertap_url is required when servers is empty")
	}
	for i, s := range c.Servers {
		if s.ID == "" {
			return fmt.Errorf("servers[%d].id is required", i)
		}
		if s.GameVersion == "" {
			return fmt.Errorf("servers[%d].game_version is required", i)
		}
		if s.RootPath == "" {
			return fmt.Errorf("servers[%d].root_path is required", i)
		}
		if s.ServerTapURL == "" {
			return fmt.Errorf("servers[%d].servertap_url is required", i)
		}
	}
	return nil
}

func LogSummary(cfg Config) {
	logger := ilog.Component("config")
	logger.Infof("runtime paths: template=%s version=%s instance=%s archive=%s", cfg.TemplateRootPath, cfg.VersionRootPath, cfg.InstanceRootPath, cfg.ArchiveRootPath)
	if cfg.ServerTapAuthHeader == "" {
		logger.Warnf("servertap_auth_header is empty, fallback should be 'key'")
	} else {
		logger.Infof("servertap auth_header=%s", cfg.ServerTapAuthHeader)
	}
	if cfg.ServerTapKey == "" {
		logger.Warnf("servertap_key is empty")
	}
}

func resolveDefaultConfigPath() string {
	logger := ilog.Component("config")
	candidates := []string{
		"config/config.yml",
		"../config/config.yml",
		"../../config/config.yml",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			logger.Infof("found config candidate: %s", p)
			return p
		}
	}
	// fallback for better error display in LoadFromFile
	logger.Warnf("no candidate found, fallback path: %s", candidates[0])
	return filepath.Clean(candidates[0])
}
