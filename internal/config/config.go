package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const appName = "gitolite-tui"

type Config struct {
	Host     string `json:"host"`
	User     string `json:"user,omitempty"`
	LogLimit int    `json:"log_limit,omitempty"`
}

func Default() Config {
	return Config{User: "git", LogLimit: 30}
}

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find XDG config directory: %w", err)
	}
	return filepath.Join(dir, appName, "config.json"), nil
}

func Load() (Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return cfg, fmt.Errorf("read config %s: %w", path, err)
		}
	} else if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	if host := os.Getenv("GITOLITE_HOST"); host != "" {
		cfg.Host = host
	}
	if user := os.Getenv("GITOLITE_USER"); user != "" {
		cfg.User = user
	}
	if cfg.User == "" {
		cfg.User = "git"
	}
	if cfg.LogLimit <= 0 {
		cfg.LogLimit = 30
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure config %s: %w", path, err)
	}
	return nil
}

func CacheRoot() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find XDG cache directory: %w", err)
	}
	return filepath.Join(dir, appName, "repos"), nil
}
