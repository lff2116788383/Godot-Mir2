package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestConfigRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.json")
	cfg := Config{
		DatabasePath: "data/mir2x.db3", ContentPath: "content", WebListenAddr: "127.0.0.1:9090",
		BusyTimeout: 3 * time.Second,
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != cfg {
		t.Fatalf("loaded = %+v, want %+v", loaded, cfg)
	}
}

func TestConfigValidation(t *testing.T) {
	t.Parallel()

	cases := []Config{
		{},
		{DatabasePath: "db", ContentPath: "", WebListenAddr: "127.0.0.1:1", BusyTimeout: time.Second},
		{DatabasePath: "db", ContentPath: "content", WebListenAddr: "", BusyTimeout: time.Second},
		{DatabasePath: "db", ContentPath: "content", WebListenAddr: "127.0.0.1:1", BusyTimeout: 0},
	}
	for _, cfg := range cases {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected invalid config: %+v", cfg)
		}
	}
}
