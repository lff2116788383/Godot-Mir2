package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config contains paths and storage settings owned by the Go server.
type Config struct {
	DatabasePath  string        `json:"database_path"`
	ContentPath   string        `json:"content_path"`
	WebListenAddr string        `json:"web_listen_addr"`
	BusyTimeout   time.Duration `json:"busy_timeout"`
}

// Default returns the development configuration.
func Default() Config {
	return Config{
		DatabasePath:  "mir2x.db3",
		ContentPath:   "content",
		WebListenAddr: "127.0.0.1:8080",
		BusyTimeout:   5 * time.Second,
	}
}

// Validate rejects incomplete or unsafe settings.
func (c Config) Validate() error {
	if strings.TrimSpace(c.DatabasePath) == "" {
		return errors.New("database_path is empty")
	}
	if strings.TrimSpace(c.ContentPath) == "" {
		return errors.New("content_path is empty")
	}
	if strings.TrimSpace(c.WebListenAddr) == "" {
		return errors.New("web_listen_addr is empty")
	}
	if c.BusyTimeout <= 0 {
		return errors.New("busy_timeout must be positive")
	}
	return nil
}

// Load reads a strict JSON configuration file.
func Load(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, errors.New("config path is empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	cfg := Default()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}

// Save writes an indented JSON configuration file.
func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}
