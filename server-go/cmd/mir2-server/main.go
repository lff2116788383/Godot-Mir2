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
	"github.com/lff2116788383/godot-mir2/server-go/internal/game"
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

	// ---- 业务层装配 ----
	// B4 阶段：登录链路（CM_LOGIN / CM_QUERYCHAR / CM_ONLINE /
	// CM_CREATEACCOUNT / CM_CHANGEPASSWORD / CM_CREATECHAR / CM_DELETECHAR）
	loginHandler := game.BuildHandler(store)
	dispatcher := transport.NewDispatcher()
	if err := game.RegisterBootstrapRoutes(dispatcher, loginHandler); err != nil {
		slog.Error("register routes", "error", err)
		os.Exit(1)
	}
	serverHandler := game.NewServerHandler(dispatcher, loginHandler)

	server := transport.NewServer(transport.Options{
		Address: cfg.GameListenAddr,
		Handler: serverHandler,
		Logger:  slog.Default(),
	})
	slog.Info("starting mir2x server-go",
		"version", version.Version,
		"game_listen", cfg.GameListenAddr,
		"database", cfg.DatabasePath,
		"routes", "ping,login,querychar,online,account,password,createchar,deletechar",
	)
	if err := server.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
		slog.Error("legacy TCP server stopped", "error", err)
		os.Exit(1)
	}
}
