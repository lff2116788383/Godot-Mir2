package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/lff2116788383/godot-mir2/server-go/internal/actor"
	"github.com/lff2116788383/godot-mir2/server-go/internal/config"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
	"github.com/lff2116788383/godot-mir2/server-go/internal/transport"
	"github.com/lff2116788383/godot-mir2/server-go/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	configPath := flag.String("config", "", "path to JSON configuration")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Full())
		return
	}

	cfg := config.Default()
	if *configPath != "" {
		loaded, err := config.Load(*configPath)
		if err != nil {
			slog.Error("load configuration", "error", err)
			os.Exit(1)
		}
		cfg = loaded
	}
	if err := cfg.Validate(); err != nil {
		slog.Error("validate configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	store, err := storage.Open(ctx, storage.OpenOptions{
		Path: cfg.DatabasePath, CreateIfMissing: true, BusyTimeout: cfg.BusyTimeout,
	})
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := store.Close(); err != nil {
			slog.Error("close database", "error", err)
		}
	}()

	actorSystem := actor.NewSystem(ctx, actor.Options{Logger: slog.Default()})
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.BusyTimeout)
		defer cancel()
		if err := actorSystem.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown actor system", "error", err)
		}
	}()

	server := transport.NewServer(transport.Options{
		Address: cfg.GameListenAddr,
		Handler: transport.PingHandler{},
		Logger:  slog.Default(),
	})
	slog.Info("starting mir2x server-go",
		"version", version.Version,
		"game_listen", cfg.GameListenAddr,
		"database", cfg.DatabasePath,
	)
	if err := server.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
		slog.Error("legacy TCP server stopped", "error", err)
		os.Exit(1)
	}
}
