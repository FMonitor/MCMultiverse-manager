package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTPAddr  string `yaml:"http_addr"`
	DBURL     string `yaml:"database_url"`
	ServerTap string `yaml:"servertap_url"`
}

func Load() (Config, error) {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		return LoadFromFile(p)
	}
	return LoadFromFile(resolveDefaultConfigPath())
}

func LoadFromFile(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse yaml %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config %s: %w", path, err)
	}

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
	candidates := []string{
		"config/config.yml",
		"../config/config.yml",
		"../../config/config.yml",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// fallback for better error display in LoadFromFile
	return filepath.Clean(candidates[0])
}
