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
	HTTPAddr            string `yaml:"http_addr"`
	DBURL               string `yaml:"database_url"`
	ServerTap           string `yaml:"servertap_url"`
	ServerTapKey        string `yaml:"servertap_key"`
	ServerTapAuthHeader string `yaml:"servertap_auth_header"`
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

func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return errors.New("http_addr is required")
	}
	if c.DBURL == "" {
		return errors.New("database_url is required")
	}
	if c.ServerTap == "" {
		return errors.New("servertap_url is required")
	}
	return nil
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
