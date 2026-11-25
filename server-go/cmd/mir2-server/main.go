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

	// 物品表（穿戴校验/药剂效果/技能书/属性计算都依赖它）
	itemsPath := filepath.Join("content", "items.json")
	itemDB, err := game.LoadItemDB(itemsPath)
	if err != nil {
		slog.Error("load item db", "path", itemsPath, "error", err)
		os.Exit(1)
	}
	// 金币 itemID 不能硬编码：按表里的 type == "金币" 解析一次
	game.GoldItemID = itemDB.GoldID()
	slog.Info("item db loaded", "count", itemDB.Count(), "gold_item_id", game.GoldItemID)

	// ★ 把物品表接入拍卖分类筛选。
	//
	//	不做这一步，storage 的 checkAuctionCategory 恒返回 0，
	//	拍卖列表**永远是空的**（上架/购买都正常，只有浏览看不到东西）。
	//	详见 internal/game/auction_category.go 的长注释。
	game.InstallAuctionCategories(itemDB)

	// 魔法表（施法校验/MP 消耗/伤害区间/元素抗性都依赖它）
	magicsPath := filepath.Join("content", "magics.json")
	magicDB, err := game.LoadMagicDB(magicsPath)
	if err != nil {
		slog.Error("load magic db", "path", magicsPath, "error", err)
		os.Exit(1)
	}
	slog.Info("magic db loaded", "count", magicDB.Count(), "learnable", magicDB.LearnableCount())

	// buff 表（21 条，其中 16 条有属性修正）
	buffPath := filepath.Join("content", "buffs.json")
	buffDB, err := game.LoadBuffDB(buffPath)
	if err != nil {
		slog.Error("load buff db", "path", buffPath, "error", err)
		os.Exit(1)
	}
	game.BindBuffResolver(buffDB)
	slog.Info("buff db loaded", "count", buffDB.Count())
	// 元素抗性解析器只在启动时绑一次（每帧重设会数据竞争）
	game.BindElementResolver(magicDB)

	// 地图名表：NPC 脚本文件名用的是地图名（道馆_1.阿天_1...），
	// 服务端按 mapID 索引，两边要能互查。
	mapNames := game.LoadMapNames("")
	world.SetMapNames(mapNames)
	slog.Info("map names loaded", "count", len(mapNames))

	// 属性计算与施法都要读物品/魔法表，回填到 worldService
	worldService.Items = itemDB
	worldService.Magics = magicDB

	// 世界时钟：技能冷却/buff 到期/地面物品过期全靠它推进。
	// 不启动的话 NowMS 恒为 0，所有冷却判断都会退化成「永久冷却中」。
	ticker := game.NewMapTicker(worlds, worldService, broadcaster, players, slog.Default())
	worldService.Ticker = ticker
	ticker.Start(ctx)
	defer ticker.Stop()

	inventoryHandler := &game.InventoryHandler{
		Players:   players,
		Items:     itemDB,
		Worlds:    worlds,
		Broadcast: broadcaster,
		ItemRepo:  storage.NewItemRepository(store),
		Sessions:  loginHandler.Sessions,
	}

	spellHandler := &game.SpellHandler{
		Players:   players,
		Worlds:    worlds,
		Items:     itemDB,
		Magics:    magicDB,
		Broadcast: broadcaster,
		Sessions:  loginHandler.Sessions,
		Logger:    slog.Default(),
		ItemRepo:  storage.NewItemRepository(store),
		KeySaver:  storage.NewItemRepository(store),
	}
	actionHandler.Spells = spellHandler

	// 公会战管理器：要在「击杀结算」之前构造（PR 点判定要用）。
	//
	// 宣战入口是 Lua 的 server.player.declareGuildWar（原版没有 CM_ 消息），
	// 但登录推战列表、到期弹提示、PK 点免除这三条是纯服务端行为。
	goldRepo := storage.NewGoldRepository(store)
	guildWarManager := game.NewGuildWarManager(players, broadcaster,
		storage.NewGuildRepository(store), goldRepo)

	// 击杀结算器：物理攻击与法术共用一个实例。
	//
	// ★ 必须是同一个实例，不能各建一个：
	//   结算逻辑有两份就会漂移（一边加了归属保护、另一边忘了），
	//   而且玩家会遇到「用法术打有掉落、用刀砍没有」这种诡异现象。
	//
	// ★ GuildWars 必须在 KillSettler 之前建好：
	//   击杀结算要用它判断「杀的这个人是公会战敌人吗」来决定加不加 PK 点。
	//   漏接的话公会战互杀会一路刷成红名（见 kill_settle.go applyPKPoint）。
	killSettler := &game.KillSettler{
		Players:   players,
		Worlds:    worlds,
		Items:     itemDB,
		Broadcast: broadcaster,
		Logger:    slog.Default(),
		GuildWars: guildWarManager,
		Chars:     storage.NewCharacterRepository(store),
	}
	actionHandler.Kills = killSettler

	// 装备磨损 / 修理 / 金币入账
	durabilityHandler := &game.DurabilityHandler{
		Items:     itemDB,
		Worlds:    worlds,
		Broadcast: broadcaster,
		ItemRepo:  storage.NewItemRepository(store),
		Golds:     storage.NewGoldRepository(store),
	}
	actionHandler.Durability = durabilityHandler
	worldService.Durability = durabilityHandler
	worldService.ItemRepo = storage.NewItemRepository(store)
	spellHandler.Kills = killSettler
	// ticker 早于 killSettler 创建（它要随地图就绪立刻启动），
	// 这里回填 —— 守卫杀怪的结算走 ticker。
	ticker.Kills = killSettler

	deathHandler := &game.DeathHandler{
		Players:   players,
		Worlds:    worlds,
		Broadcast: broadcaster,
		Logger:    slog.Default(),
		Chars:     storage.NewCharacterRepository(store),
	}

	shopHandler := &game.ShopHandler{
		Players:   players,
		Worlds:    worlds,
		Items:     itemDB,
		Broadcast: broadcaster,
		Logger:    slog.Default(),
		Golds:     storage.NewGoldRepository(store),
		Chars:     storage.NewCharacterRepository(store),
	}

	// 怪物表（刷怪需要完整数值：hp/dc/ac/元素抗性/掉落）
	monstersPath := filepath.Join("content", "monsters.json")
	monsterDB, err := world.LoadMonsterDB(monstersPath)
	if err != nil {
		slog.Error("load monster db", "path", monstersPath, "error", err)
		os.Exit(1)
	}
	slog.Info("monster db loaded", "count", monsterDB.Count())

	// 地图脚本运行时（刷怪/守卫/安全区）
	mapScripts := game.NewMapScriptManager(monsterDB, slog.Default())
	defer mapScripts.Close()
	worldService.MapScripts = mapScripts

	// NPC：加载原版 Lua 脚本（每个 NPC 一个独立 Lua state）
	npcManager := game.NewNPCManager(itemDB, magicDB, slog.Default())
	defer npcManager.Close()

	npcHandler := &game.NPCHandler{
		Players:   players,
		NPCs:      npcManager,
		Worlds:    worldService,
		Broadcast: broadcaster,
		Logger:    slog.Default(),
	}
	worldService.NPCs = npcManager

	// 聊天：私聊/群聊走 DB，原地说话走 AOI。
	chatHandler := &game.ChatHandler{
		Players:   players,
		Sessions:  loginHandler.Sessions,
		Broadcast: broadcaster,
		Logger:    slog.Default(),
		Chat:      storage.NewChatRepository(store),
		// 原地说话只传给视野内的玩家（原版 m_inViewCOList）
		//
		// 用 ObserversAt（哪些实体能看到这一格）而不是「同图所有人」：
		// 隔了半张地图还听得见就不合理。
		Viewers: func(ctx context.Context, mapID uint32, x, y int) []uint64 {
			uids, err := worlds.ObserversAt(ctx, mapID, x, y)
			if err != nil {
				return nil
			}
			return uids
		},
	}

	// 组队（内存态，队长为权威）+ 仓库（走 DB）
	teamManager := game.NewTeamManager(players, broadcaster, slog.Default())
	securedHandler := &game.SecuredHandler{
		Players:   players,
		Broadcast: broadcaster,
		Logger:    slog.Default(),
		Items:     itemDB,
		Repo:      storage.NewItemRepository(store),
	}
	worldService.Secured = securedHandler
	worldService.Teams = teamManager

	// 面对面交易（双人协商状态机）
	tradeManager := game.NewDirectTradeManager(players, broadcaster, slog.Default())
	worldService.Trades = tradeManager

	// 公会（入库）+ 摆摊（入库，卖家可离线）
	guildHandler := &game.GuildHandler{
		Players:   players,
		Broadcast: broadcaster,
		Logger:    slog.Default(),
		Repo:      storage.NewGuildRepository(store),
	}
	stallHandler := &game.StallHandler{
		Players:   players,
		Broadcast: broadcaster,
		Logger:    slog.Default(),
		Repo:      storage.NewStallRepository(store),
		Items:     itemDB,
		Golds:     storage.NewGoldRepository(store),
	}

	// 拍卖行 / 邮件 / 好友
	//
	// ★ 投递服务必须先于拍卖构造：拍卖成交后买卖双方都靠它收货，
	//   所以 AuctionHandler.Deliveries 指向同一个实例。
	auctionRepo := storage.NewAuctionRepository(store)
	deliveryRepo := storage.NewDeliveryRepository(store)
	chatRepo := storage.NewChatRepository(store)

	deliveryService := game.NewDeliveryService(deliveryRepo, chatRepo, itemDB,
		players, broadcaster, slog.Default())

	auctionHandler := &game.AuctionHandler{
		Players:    players,
		Broadcast:  broadcaster,
		Logger:     slog.Default(),
		Repo:       auctionRepo,
		Items:      itemDB,
		Golds:      goldRepo,
		Deliveries: deliveryService,
		Stalls:     storage.NewStallRepository(store),
	}
	deliveryHandler := &game.DeliveryHandler{
		Players:    players,
		Broadcast:  broadcaster,
		Logger:     slog.Default(),
		Repo:       deliveryRepo,
		Items:      itemDB,
		Golds:      goldRepo,
		Deliveries: deliveryService,
	}
	friendHandler := &game.FriendHandler{
		Players:   players,
		Broadcast: broadcaster,
		Logger:    slog.Default(),
		Repo:      chatRepo,
	}

	worldService.GuildWars = guildWarManager
	worldService.Deliveries = deliveryService
	worldService.DeliveryRepo = deliveryRepo
	// ★ NPC 脚本的 server.player.* 走 uidRemoteCall，数据源在这里
	worldService.Guilds = storage.NewGuildRepository(store)
	worldService.Shops = shopHandler
	worldService.Golds = goldRepo

	// 拍卖到期清理：过期物品退回卖家（走投递），而不是静默删除
	auctionSweeper := game.NewAuctionSweeper(auctionRepo, deliveryService, itemDB, slog.Default())
	go auctionSweeper.Run(context.Background())

	// 公会邀请通知走系统聊天消息（原版 postSystemMessage），
	// 所以 GuildHandler 需要 chat 仓储
	guildHandler.Chat = chatRepo

	if err := game.RegisterAllRoutes(dispatcher, loginHandler, actionHandler,
		inventoryHandler, spellHandler, deathHandler, shopHandler, npcHandler, chatHandler,
		teamManager, securedHandler, tradeManager, guildHandler, stallHandler,
		auctionHandler, deliveryHandler, friendHandler); err != nil {
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
			"action,equipwear,grabwear,equipbelt,grabelt,dropitem,consumeitem,"+
			"setmagickey,die,revive,addexp,addhealth,querysellitemlist,buy",
	)
	if err := server.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
		slog.Error("legacy TCP server stopped", "error", err)
		os.Exit(1)
	}
}
