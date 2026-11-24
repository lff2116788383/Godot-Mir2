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
		DatabasePath: "data/mir2x.db3", ContentPath: "content",
		GameListenAddr: "127.0.0.1:7001", WebListenAddr: "127.0.0.1:9090",
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

	valid := Config{DatabasePath: "db", ContentPath: "content", GameListenAddr: "127.0.0.1:7000", WebListenAddr: "127.0.0.1:8080", BusyTimeout: time.Second}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []Config{
		{},
		{DatabasePath: "db", ContentPath: "", GameListenAddr: "127.0.0.1:7000", WebListenAddr: "127.0.0.1:8080", BusyTimeout: time.Second},
		{DatabasePath: "db", ContentPath: "content", GameListenAddr: "", WebListenAddr: "127.0.0.1:8080", BusyTimeout: time.Second},
		{DatabasePath: "db", ContentPath: "content", GameListenAddr: "127.0.0.1:7000", WebListenAddr: "", BusyTimeout: time.Second},
		{DatabasePath: "db", ContentPath: "content", GameListenAddr: "127.0.0.1:7000", WebListenAddr: "127.0.0.1:8080", BusyTimeout: 0},
	}
	for _, cfg := range cases {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected invalid config: %+v", cfg)
		}
	}
}
