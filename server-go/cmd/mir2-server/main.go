package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/lff2116788383/godot-mir2/server-go/internal/actor"
	"github.com/lff2116788383/godot-mir2/server-go/internal/config"
	"github.com/lff2116788383/godot-mir2/server-go/internal/game"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
	"github.com/lff2116788383/godot-mir2/server-go/internal/transport"
	"github.com/lff2116788383/godot-mir2/server-go/internal/version"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
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
	// B5：登录链路 + 玩家进世界 + 动作处理
	loginHandler := game.BuildHandler(store)

	players := game.NewPlayerManager()
	worlds := world.NewWorld(actorSystem)
	assetsRoot := game.ResolveAssetsRoot()
	worldService := game.NewWorldService(players, worlds, nil,
		storage.NewCharacterRepository(store), assetsRoot)
	loginHandler.Worlds = worldService

	dispatcher := transport.NewDispatcher()

	server := transport.NewServer(transport.Options{
		Address: cfg.GameListenAddr,
		// Handler 稍后设置：Broadcaster 需要 server 句柄，而 server 又需要 handler，
		// 这个循环用「先建 server、再回填 Handler」解开
		Handler: nil,
		Logger:  slog.Default(),
	})

	broadcaster := transport.NewBroadcaster(server)
	worldService.Broadcast = broadcaster

	actionHandler := &game.ActionHandler{
		Players:   players,
		Sessions:  loginHandler.Sessions,
		Worlds:    worlds,
		Broadcast: broadcaster,
	}

	// 物品表（穿戴校验/药剂效果/技能书都依赖它）
	itemsPath := filepath.Join("content", "items.json")
	itemDB, err := game.LoadItemDB(itemsPath)
	if err != nil {
		slog.Error("load item db", "path", itemsPath, "error", err)
		os.Exit(1)
	}
	slog.Info("item db loaded", "count", itemDB.Count())

	inventoryHandler := &game.InventoryHandler{
		Players:   players,
		Items:     itemDB,
		Worlds:    worlds,
		Broadcast: broadcaster,
		ItemRepo:  storage.NewItemRepository(store),
		Sessions:  loginHandler.Sessions,
	}

	if err := game.RegisterAllRoutes(dispatcher, loginHandler, actionHandler, inventoryHandler); err != nil {
		slog.Error("register routes", "error", err)
		os.Exit(1)
	}

	serverHandler := game.NewServerHandler(dispatcher, loginHandler)
	serverHandler.SetDisconnectHook(func(ctx context.Context, channelID uint64) {
		worldService.Leave(ctx, channelID)
	})
	server.SetHandler(serverHandler)

	slog.Info("starting mir2x server-go",
		"version", version.Version,
		"game_listen", cfg.GameListenAddr,
		"database", cfg.DatabasePath,
		"assets", assetsRoot,
		"routes", "ping,login,querychar,online,account,password,createchar,deletechar,"+
			"action,equipwear,grabwear,equipbelt,grabelt,dropitem,consumeitem",
	)
	if err := server.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
		slog.Error("legacy TCP server stopped", "error", err)
		os.Exit(1)
	}
}
