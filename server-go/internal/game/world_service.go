package game

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/script"
	"github.com/lff2116788383/godot-mir2/server-go/internal/storage"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== 玩家进入世界（servicecorenet.cpp:101 之后的流程）====================
//
// 原版 CM_ONLINE 成功后：
//
//	1. 从 tbl_char 读出位置（map/mapx/mapy）
//	2. 构造 SDInitPlayer
//	3. 向地图 actor 发 AM_ADDTOMAP，等待进场
//	4. 地图把它加进网格 + 建立 AOI 观察圈
//	5. 回 SM_ONLINEOK（含 mapUID 与 ActionNode）
//	6. 后续地图通过 AM_OBSERVE 把视野里的其他 CO 推给它
//
// 这里同样是这个顺序，只是 AM_* 换成直接调用 world。
// 关键点：**AOI 事件必须真的转成网络消息发出去**，否则玩家进去看不到任何人。

// WorldService 负责「玩家 ↔ 地图」的桥接。
type WorldService struct {
	Players   *PlayerManager
	Worlds    *world.World
	Broadcast Broadcaster
	Chars     *storage.CharacterRepository
	Logger    *slog.Logger

	// Items 物品表，用于算玩家战斗属性（CombatStatsFor）。
	// 为 nil 时属性退化成「只有等级基础值」，装备不生效。
	Items *ItemDB
	// Magics 魔法表，用于施法校验与伤害结算。为 nil 时所有技能被拒。
	Magics *MagicDB
	// NPCs NPC 脚本管理器。进场时用它加载该地图的 NPC。
	NPCs *NPCManager
	// Durability 装备磨损/修理/金币入账（NPC 修理与拾取金币要用）。
	Durability *DurabilityHandler
	// ItemRepo 角色物品落库（进场时加载背包/装备/腰带）。
	//
	// ★ 之前没有这个字段，`Enter` 只建了个空 Wear map ——
	//	重登后装备和背包全部消失，而且不报错（客户端看到的就是空背包）。
	ItemRepo *storage.ItemRepository
	// Secured 仓库（NPC 寄存/取回）。
	Secured *SecuredHandler
	// Teams 组队管理器（下线时要清理队伍关系）。
	Teams *TeamManager
	// Trades 面对面交易（下线时要取消交易）。
	Trades *DirectTradeManager
	// MapScripts 地图脚本管理器（刷怪/守卫/安全区）。
	MapScripts *MapScriptManager
	// Ticker 世界时钟驱动器。地图建好后要 Track 它，
	// 否则这张图的 NowMS 永远不动（技能冷却永远不结束）。
	Ticker *MapTicker

	// GuildWars 公会战管理器。进场时推一次完整战列表（原版 refreshGuildWarList(true)）。
	//
	// ★ 不推的后果：登录后客户端「交战行会」面板永远是空的，
	// 玩家不知道自己在打仗，也看不出「杀敌对不会红名」。
	GuildWars *GuildWarManager
	// Deliveries 投递服务。进场时补推「未领取的投递」。
	//
	// ★ 这就是原版的「邮箱」：不是独立协议，而是把未领取的投递
	// 重新生成成系统聊天消息推下去。不做这一步的话，
	// 离线期间收到的货款要等玩家自己翻历史聊天才看得到。
	Deliveries *DeliveryService
	// DeliveryRepo 未领取列表查询（配合 Deliveries 用）
	DeliveryRepo *storage.DeliveryRepository

	// Guilds 公会仓储（NPC 脚本的 server.player.isGuildLeader / getGuildName 要用）
	//
	// ★ 这两个字段是 uidRemoteCall 的数据源：行政官员问「你是不是会长」时，
	//   走 server.player.isGuildLeader → uidRemoteCall → executeRemoteCall，
	//   那里读的就是 Guilds。
	Guilds *storage.GuildRepository
	// Shops 商店/经验（remoteCall 的 addExp 要用）
	Shops *ShopHandler
	// Golds 金币落库（createGuild 扣建会费要用）
	Golds GoldRepository
	// Quests 任务系统（玩家登录时要重放任务状态）
	Quests *QuestService
	// questActivateOnce 保证任务只激活一次（首张地图就绪时）
	questActivateOnce sync.Once

	// mapCache mapID -> *world.MapData，避免每次进场都读文件
	mapCache map[uint32]*world.MapData
	// assetsRoot 地图素材目录
	assetsRoot string
}

// NewWorldService 创建世界服务。
func NewWorldService(players *PlayerManager, worlds *world.World, broadcast Broadcaster,
	chars *storage.CharacterRepository, assetsRoot string) *WorldService {
	return &WorldService{
		Players:    players,
		Worlds:     worlds,
		Broadcast:  broadcast,
		Chars:      chars,
		Logger:     slog.Default(),
		mapCache:   make(map[uint32]*world.MapData),
		assetsRoot: assetsRoot,
	}
}

// AssetsRoot 解析地图素材根目录。
//
// 优先环境变量 MIR2X_ASSETS，其次几个约定位置。
// 找不到时返回空串，调用方应准备一块兜底平地（否则玩家进不去）。
func ResolveAssetsRoot() string {
	if root := os.Getenv("MIR2X_ASSETS"); root != "" {
		return root
	}
	candidates := []string{
		"E:/Desktop/game/mir2x_assets-main",
		"../mir2x_assets-main",
		"../../mir2x_assets-main",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}

// LoadMap 载入一张地图（带缓存），并在 world 里建好对应地图 actor。
//
// 载不到真实素材时退化为一块 200x200 全可走平地 —— 这样至少能进游戏、
// 能移动、能看到彼此，而不是让 B5 卡在「缺素材」上。
func (s *WorldService) LoadMap(ctx context.Context, mapID uint32) error {
	if _, ok := s.mapCache[mapID]; ok {
		return nil
	}

	var data *world.MapData
	if s.assetsRoot != "" {
		path := filepath.Join(s.assetsRoot, "map", fmt.Sprintf("%d.bin", mapID))
		if _, err := os.Stat(path); err == nil {
			loaded, err := world.LoadMapFile(path)
			if err != nil {
				s.Logger.Warn("load map failed, falling back to flat ground",
					"map", mapID, "path", path, "error", err)
			} else {
				data = loaded
			}
		}
	}
	if data == nil {
		flat, err := world.NewMapData(200, 200)
		if err != nil {
			return fmt.Errorf("create fallback map: %w", err)
		}
		for y := 0; y < 200; y++ {
			for x := 0; x < 200; x++ {
				_ = flat.SetLand(x, y, world.Land{CanWalk: true})
			}
		}
		data = flat
		s.Logger.Warn("using fallback flat map", "map", mapID, "size", "200x200")
	}

	s.mapCache[mapID] = data
	if err := s.Worlds.CreateMap(mapID, data); err != nil {
		// 地图已存在（重启/重连）不算错误
		if s.Worlds.MapCount() > 0 {
			s.trackMap(mapID)
			return nil
		}
		return fmt.Errorf("create map actor: %w", err)
	}
	s.trackMap(mapID)
	s.Logger.Info("map ready", "map", mapID, "size", fmt.Sprintf("%dx%d", data.Width, data.Height))

	// 加载这张地图的 NPC（原版 servermap.cpp:1701 loadNPChar）
	if err := s.loadMapNPCs(ctx, mapID); err != nil {
		// NPC 加载失败不影响地图可用性，只记日志
		s.Logger.Warn("load map npcs failed", "map", mapID, "error", err)
	}
	// 加载地图脚本（刷怪/守卫/安全区，原版 servermap.cpp:577）
	if err := s.loadMapScript(ctx, mapID); err != nil {
		s.Logger.Warn("load map script failed", "map", mapID, "error", err)
	}

	// ★ 首张地图就绪后激活任务。
	//
	//	任务脚本顶层要 getNPCharUID，而 NPC 是按图懒加载的。
	//	等第一张图（玩家要进的那张）的 NPC 建好，才第一次执行任务脚本。
	//	用 sync.Once 保证只做一次：后续进别的地图不再重复激活。
	if s.Quests != nil && s.NPCs != nil {
		s.questActivateOnce.Do(func() {
			s.Quests.ActivateAfterWorldLoaded(s.NPCs)
		})
	}
	return nil
}

// loadMapScript 加载本图的地图脚本。
//
// 顺序必须在 NPC 之后：地图脚本的 addGuard 会在安全区上放守卫，
// 安全区要先由脚本自己 addSafeZone 建立（脚本内顺序），
// 但 NPC 与人无关，先加载不影响。
func (s *WorldService) loadMapScript(ctx context.Context, mapID uint32) error {
	if s.MapScripts == nil {
		return nil
	}
	mapName := world.MapNameOf(mapID)
	if mapName == "" {
		return nil
	}
	_, err := s.MapScripts.LoadMap(ctx, mapID, mapName, s.Worlds, s.Players)
	return err
}

// buildNPCHost 构造一个 NPC 的宿主适配器。
//
// ★ 抽成独立方法是为了让「启动预加载」和「进图加载」共用同一套依赖：
//
//	预加载时如果 host 缺字段，NPC 脚本一执行 setNPCSell 就空指针崩，
//	这个 NPC 直接被跳过 —— 而任务要靠它挂 handler，于是任务也挂不上。
//
//	实测踩过：host 传 nil 时 340 个 NPC 只成功 91 个，
//	表现为「任务激活只有 12 个」。
func (s *WorldService) buildNPCHost(npcUID uint64) *npcHostAdapter {
	return &npcHostAdapter{
		NPCUID:     npcUID,
		Players:    s.Players,
		Worlds:     s.Worlds,
		Broadcast:  s.Broadcast,
		Items:      s.Items,
		Logger:     s.Logger,
		Durability: s.Durability,
		Secured:    s.Secured,
		Guilds:     s.Guilds,
		GuildWars:  s.GuildWars,
		Magics:     s.Magics,
		Shops:      s.Shops,
		Golds:      s.Golds,
	}
}

// loadMapNPCs 为本图加载 NPC 脚本并在世界层生成实体。
//
// ★ NPC 实体必须进世界层，否则：
//
//  1. 玩家看不到 NPC（AOI 不会下发不存在的东西）
//  2. 客户端点不到（点击靠 SM_CORECORD 里的 UID）
//  3. 距离校验无从谈起（要读 NPC 的坐标）
//
// 原版 NPC 是独立 Actor 走 AM_ADDCO 注册进来的，
// 这里直接 AddEntity，语义等价（都在同一张图上、都参与 AOI）。
func (s *WorldService) loadMapNPCs(ctx context.Context, mapID uint32) error {
	if s.NPCs == nil {
		return nil
	}
	mapName := world.MapNameOf(mapID)
	if mapName == "" {
		return nil
	}

	loaded, err := s.NPCs.LoadMap(ctx, mapID, mapName, s.buildNPCHost)
	if err != nil {
		return err
	}

	spawned := 0
	for _, inst := range loaded {
		entity := world.Entity{
			UID:       inst.NPCUID,
			Type:      world.EntityNPC,
			Name:      inst.Def.Name,
			X:         inst.Def.X,
			Y:         inst.Def.Y,
			Direction: world.Direction(inst.Def.GfxDir + 1), // gfxDir 0..7 -> DIR 1..8
			HP:        1, MaxHP: 1,
			// NPC 不参与战斗，属性留空；HP=1 是为了让 Dead() 的
			// `HP <= 0` 判断不把 NPC 当成尸体
			ViewRange: 0,
		}
		if _, err := s.Worlds.AddEntity(ctx, mapID, entity); err != nil {
			s.Logger.Debug("spawn npc failed", "npc", inst.Def.Name, "error", err)
			continue
		}
		spawned++
	}
	s.Logger.Info("map npc spawned", "map", mapName, "loaded", len(loaded), "spawned", spawned)
	return nil
}

// trackMap 让 ticker 开始驱动这张地图。
//
// ★ 必须在 CreateMap 成功后立刻做：
//
//	NowMS 只在 World.Advance 被调用时推进，没人调它就恒为 0。
//	而技能冷却判断是 `nowMS - last < coolDown`，两个 0 相减
//	得到 0，永远小于任何正数冷却 —— 表现是「第一次放技能后
//	这个技能再也放不出来」。
func (s *WorldService) trackMap(mapID uint32) {
	if s.Ticker != nil {
		s.Ticker.Track(mapID)
	}
}

// Enter 把玩家放进世界。
//
// 返回的 Player 已经登记进 PlayerManager，且地图里已有对应实体。
func (s *WorldService) Enter(ctx context.Context, ch storage.Character, channelID uint64) (*Player, error) {
	mapID := ch.Map
	if mapID == 0 {
		mapID = DefaultSpawnMap
	}
	if err := s.LoadMap(ctx, mapID); err != nil {
		return nil, err
	}

	mapUID, err := common.GetBaseMapUID(mapID, 0)
	if err != nil {
		return nil, fmt.Errorf("build map uid: %w", err)
	}
	playerUID := common.GetPlayerUID(ch.DBID)

	level := DisplayLevel(ch.Exp)
	hp, maxHP := ch.HP, ch.HP
	if hp <= 0 {
		// 老角色血量为 0（没练过）：给一组初始值，否则进门就死
		maxHP = 30 + int32(level)*5
		hp = maxHP
	}
	mp, maxMP := ch.MP, 30+int32(level)*3
	if mp <= 0 {
		mp = maxMP
	}
	viewRange := int(ch.ViewRange)
	if viewRange <= 0 {
		viewRange = 10 // 原版默认视野
	}

	p := &Player{
		ChannelID: channelID,
		DBID:      ch.DBID,
		UID:       playerUID,
		MapID:     mapID,
		MapUID:    mapUID,
		Name:      ch.Name,
		Gender:    ch.Gender,
		Job:       ch.Job,
		Hair:      ch.Hair,
		HairColor: ch.HairColor,
		Level:     level,
		HP:        hp,
		MaxHP:     maxHP,
		MP:        mp,
		MaxMP:     maxMP,
		Exp:       ch.Exp,
		Gold:      ch.Gold,
		PKPoint:   ch.PKPoint,
		ViewRange: viewRange,
		Direction: common.DirectionDown,
		X:         int(ch.MapX),
		Y:         int(ch.MapY),
		Wear:      make(map[int]*Item),
		// ★ 这三个 map 必须在进场时初始化，不能留给首次使用时懒建。
		//   施法路径会直接写 p.MagicCooldown[magicID] = nowMS，
		//   nil map 写入是 panic（不是静默失败），一次技能就把服务端打挂。
		MagicTrain:    make(map[uint32]int),
		MagicCooldown: make(map[uint32]int64),
		MagicKeys:     make(map[uint32]uint8),
	}

	// 加载背包 / 装备 / 腰带
	//
	// ★ 顺序必须在「算属性」之前：
	//
	//	WorldEntity 里的 Stats 是 CombatStatsFor 算的，那个函数读 p.Wear。
	//	先算属性再加载装备，玩家进图时是「裸装属性」，
	//	要等下次换装才恢复 —— 表现是「重登后攻击力变低了」。
	s.loadPlayerItems(ctx, p)

	// 进地图
	events, err := s.Worlds.AddEntity(ctx, mapID, p.WorldEntity(mapUID, s.Items))
	if err != nil {
		return nil, fmt.Errorf("add player to map: %w", err)
	}
	s.Players.Add(p)

	// 落地坐标可能被占地拒绝；读回真实位置
	if e, ok, err := s.Worlds.Entity(ctx, mapID, playerUID); err == nil && ok {
		p.SyncFromWorld(e)
	}

	// ★ 关键：把 AOI 事件转成网络消息，否则玩家进去看不到任何人
	s.dispatchObserverEvents(ctx, p, events)

	// 登录补推：公会战列表 + 未领取的投递
	//
	// ★ 顺序在 AOI 之后：先让玩家看到世界，再补推「你在打仗」「你有邮件」。
	//   反了的话这两类消息会插在进场消息之前，客户端还没建好 UI 就收到了。
	s.pushLoginExtras(ctx, p)

	s.Logger.Info("player entered world",
		"name", p.Name, "uid", common.GetUIDString(p.UID),
		"map", mapID, "at", fmt.Sprintf("(%d,%d)", p.X, p.Y),
		"aoi_events", len(events))
	return p, nil
}

// pushLoginExtras 登录时补推「战列表」与「未领取投递」（原版 Player 构造尾部）。
//
// 两件事都在同一个地方做，因为它们共享一个性质：
// **只在登录时需要，且都靠系统聊天 / SM_GUILDWARLIST 走**。
func (s *WorldService) pushLoginExtras(ctx context.Context, p *Player) {
	s.pushLoginStates(ctx, p)
	if s.GuildWars != nil {
		s.GuildWars.OnPlayerLogin(ctx, p.DBID)
	}
	s.repushUnclaimedDeliveries(ctx, p)
	// 任务状态重放（原版 _RSVD_NAME_restoreQuestStates）
	//
	// ★ 必须在「地图/NPC 都就绪」之后：
	//	任务状态函数会调 setupNPCQuestBehavior / setupMapGridTrigger，
	//	那些要按地图名+NPC名找对象。地图没加载就全部找不到。
	if s.Quests != nil {
		s.Quests.OnPlayerLogin(ctx, p)
	}
}

// pushLoginStates 推送「重登后客户端才有的状态」（原版 player.cpp:2426-2460）。
//
// ★★ 这一组之前**一条都没发**，是实打实的功能缺失 ★★
//
// 客户端的 main.gd 已经实现了这 5 条消息的解析，服务端从来不推，
// 后果是每次重登都是「新客户端」状态：
//
//	SM_LEARNEDMAGICLIST  技能栏空白 → 玩家以为技能丢了
//	SM_BUFFIDLIST        buff 图标不显示 → 不知道自己身上有幽灵盾
//	SM_VIEWRANGE         视野用客户端默认值 → 与服务器实际视野不一致，
//	                     会出现「看得见但打不到」或反之
//	SM_PLAYERCONFIG      自动喝药/魔法快捷键全丢 → 每次重登要重设
//	SM_SAFEZONELIST      安全区不显示 → 站在安全区里还以为会被打
//
// 顺序照抄原版 postRebindOK：外观/属性 → 背包 → 魔法 → 配置 → buff → 视野。
// 反了不会崩，但客户端 UI 会闪（先画空背包再填内容）。
func (s *WorldService) pushLoginStates(ctx context.Context, p *Player) {
	if s.Broadcast == nil || p == nil || p.ChannelID == 0 {
		return
	}

	// 1. 已学魔法（原版 player.cpp:2426）
	if s.ItemRepo != nil {
		if list, err := s.ItemRepo.LoadLearnedMagic(ctx, p.DBID); err == nil {
			if payload, err := protocol.CerealSerialize(protocol.SDLearnedMagicList{MagicList: list}); err == nil {
				_ = s.Broadcast.SendTo(p.ChannelID, protocol.SM_LEARNEDMAGICLIST, payload, 0)
			}
			// 顺带把 MagicTrain 填上：施法伤害与耗蓝要用修炼等级
			for _, m := range list {
				if p.MagicTrain == nil {
					p.MagicTrain = make(map[uint32]int)
				}
				p.MagicTrain[m.MagicID] = int(m.Exp)
			}
		} else if s.Logger != nil {
			s.Logger.Warn("load learned magic failed", "dbid", p.DBID, "error", err)
		}

		// 2. 玩家配置（快捷键 + 运行时配置，原版 player.cpp:2427）
		if cfg, err := s.ItemRepo.LoadPlayerConfig(ctx, p.DBID); err == nil {
			if payload, err := protocol.CerealSerialize(cfg); err == nil {
				_ = s.Broadcast.SendTo(p.ChannelID, protocol.SM_PLAYERCONFIG, payload, 0)
			}
			// 快捷键回填，否则 SM_SETMAGICKEY 改过的绑定重登就没了
			if p.MagicKeys == nil {
				p.MagicKeys = make(map[uint32]uint8)
			}
			for magicID, key := range cfg.MagicKeyList.KeyList {
				if magicID != 0 {
					p.MagicKeys[magicID] = key
				}
			}
		}
	}

	// 3. buff 列表（原版 player.cpp:2453）
	//
	// ★ UID 必须是**自己**的：客户端据此判断「这个 buff 列表是谁的」。
	//   传 0 会让客户端把图标挂到空实体上。
	if payload, err := protocol.CerealSerialize(protocol.SDBuffIDList{
		UID:    p.UID,
		IDList: p.Buffs,
	}); err == nil {
		_ = s.Broadcast.SendTo(p.ChannelID, protocol.SM_BUFFIDLIST, payload, 0)
	}

	// 4. 视野范围（原版 player.cpp:2829 SMViewRange 是定长消息）
	if payload, err := protocol.AppendWirePayload(nil, protocol.SMViewRange{
		ViewRange: uint8(p.ViewRange),
	}); err == nil {
		_ = s.Broadcast.SendTo(p.ChannelID, protocol.SM_VIEWRANGE, payload, 0)
	}

	// 5. 安全区（原版 servermapop.cpp 在新玩家进图时逐个下发）
	s.pushSafeZones(ctx, p)
}

// pushSafeZones 把当前地图的安全区推给玩家（SDSafeZoneList）。
//
// ★ 为什么是「一条消息一个区域」而不是「一个列表」：
//
//	协议结构 SDSafeZoneList 里是 MapUID + **单个** Zone
//	（见 serdes_second_batch.go:444），不是数组。
//	原版也是每条消息带一整张地图的合并结果 ——
//	一张图的所有矩形/圆形都在**同一个 Zone** 里，所以只发一条。
//
// ★ 单位转换：world.SafeZone 用 int（内部算法方便），
//   协议 SDSafeZone 用 int32。必须逐项转换，不能强转切片
//   （[]  int 和 []int32 内存布局不同）。
func (s *WorldService) pushSafeZones(ctx context.Context, p *Player) {
	if s.Worlds == nil || s.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	zone, err := s.Worlds.SafeZone(ctx, p.MapID)
	if err != nil || zone.Empty() {
		return
	}

	proto := protocol.SDSafeZone{
		RectList:   make([][4]int32, 0, len(zone.Rects)),
		CircleList: make([][3]int32, 0, len(zone.Circles)),
	}
	for _, r := range zone.Rects {
		proto.RectList = append(proto.RectList, [4]int32{
			int32(r[0]), int32(r[1]), int32(r[2]), int32(r[3]),
		})
	}
	for _, c := range zone.Circles {
		proto.CircleList = append(proto.CircleList, [3]int32{
			int32(c[0]), int32(c[1]), int32(c[2]),
		})
	}

	payload, err := protocol.CerealSerialize(protocol.SDSafeZoneList{
		MapUID: p.MapUID,
		Zone:   proto,
	})
	if err != nil {
		return
	}
	_ = s.Broadcast.SendTo(p.ChannelID, protocol.SM_SAFEZONELIST, payload, 0)
}

// repushUnclaimedDeliveries 把未领取的投递重新推给玩家。
//
// ★ 这不是「重新创建投递」，只是把 tbl_delivery 里 fld_claimed=0 的记录
// 再生成一遍系统聊天消息推下去。投递本身还在，record 也没变，
// 玩家点「收取」用原来的 record 就能领。
//
// 为什么需要这一步：
//
//	原版登录时会 RetrieveLatest 拉最近聊天记录，里面自然包含
//	之前推过的投递消息。但聊天记录有**条数上限**（默认几十条），
//	玩家离线久了、或者中间聊了几百句，那条投递消息就被挤出窗口了。
//	这时「投递还在但玩家看不见」，等于丢件。
//
//	所以补推是必要的，而且只推 unclaimed —— 已领的不能再推，
//	否则玩家点进去会拿到 CDERR_INVALID_CLAIM。
func (s *WorldService) repushUnclaimedDeliveries(ctx context.Context, p *Player) {
	if s.DeliveryRepo == nil || s.Broadcast == nil || p.ChannelID == 0 {
		return
	}
	pending, err := s.DeliveryRepo.ListUnclaimed(ctx, p.DBID)
	if err != nil || len(pending) == 0 {
		return
	}
	from := protocol.NewSDChatPeerID(protocol.CPR_SPECIAL, SysChatDBID)
	to := protocol.NewSDChatPeerID(protocol.CPR_PLAYER, p.DBID)

	// ★ 一次最多推 10 条：玩家离线期间可能堆了几百件，
	//   全推会把登录包撑爆（每条都是一段 XML）。
	const maxRepush = 10
	shown := 0
	for _, d := range pending {
		if shown >= maxRepush {
			break
		}
		items, decErr := decodeDeliveryPayload(d.Payload)
		if decErr != nil {
			continue
		}
		body := buildDeliveryXML("你有一份未领取的系统投递：", s.Items, items, d.Record)
		var seq protocol.SDChatMessageDBSeq
		if d.MessageID != nil {
			seq = protocol.SDChatMessageDBSeq{ID: *d.MessageID, Timestamp: d.Timestamp}
		}
		list := protocol.SDChatMessageList{{
			HasSeq:  d.MessageID != nil,
			Seq:     seq,
			From:    from,
			To:      to,
			Message: body,
		}}
		payload, encErr := protocol.CerealSerialize(list)
		if encErr != nil {
			continue
		}
		_ = s.Broadcast.SendTo(p.ChannelID, protocol.SM_CHATMESSAGELIST, payload, 0)
		shown++
	}
	if len(pending) > shown {
		text := fmt.Sprintf("你还有 %d 份未领取的系统投递，清完手上这些后再登录查看。", len(pending)-shown)
		_ = s.Broadcast.SendTo(p.ChannelID, protocol.SM_TEXT, []byte(text), 0)
	}
}

// dispatchObserverEvents 把 AOI 事件翻译成 SM 消息。
//
// 这层是 B4 留下的「最后一公里」：world 层已经算出了该通知谁，
// 但之前没有任何代码把它变成网络包，所以进图后世界是空的。
//
// 事件语义（internal/world/aoi.go）：
//
//	COAppear      observer 应该看到 target 出现了 -> 推 SM_CORECORD
//	CODisappear   observer 应该把 target 移出视野 -> 推 SM_CODISAPPEAR
//	ObserverAdd   观察者自己开始观察某格（不需直接发消息，实体靠 COAppear 通知）
//	ObserverRemove 观察者不再观察某格（同上）
func (s *WorldService) dispatchObserverEvents(ctx context.Context, self *Player, events []world.ObserverEvent) {
	for _, ev := range events {
		switch ev.Kind {
		case world.COAppear:
			s.sendCOAppear(ctx, ev)
		case world.CODisappear:
			s.sendCODisappear(ctx, ev)
		default:
			// ObserverAdd / ObserverRemove 是观察圈自身变化，
			// 实体出现/消失由 COAppear / CODisappear 承载，这里无需处理
		}
	}
}

// sendCOAppear 把「某个实体出现在你的视野里」发给观察者。
//
// 原版是 AM_OBSERVE：观察者收到后向对方索取 CORecord，
// 拿到坐标/名字/外观后创建本地实体。这里直接推 SM_CORECORD（省一次往返）。
func (s *WorldService) sendCOAppear(ctx context.Context, ev world.ObserverEvent) {
	// 观察者必须是玩家才有连接
	observer, ok := s.Players.Get(ev.Observer)
	if !ok {
		return
	}
	record, err := s.BuildCORecord(ctx, observer.MapID, ev.Target)
	if err != nil {
		return
	}
	record.MapUID = observer.MapUID
	payload, err := protocol.AppendWirePayload(nil, record)
	if err != nil {
		return
	}
	if s.Broadcast != nil {
		_ = s.Broadcast.SendTo(observer.ChannelID, protocol.SM_CORECORD, payload, 0)
	}
}

// sendCODisappear 把「某个实体离开你的视野」发给观察者。
func (s *WorldService) sendCODisappear(ctx context.Context, ev world.ObserverEvent) {
	observer, ok := s.Players.Get(ev.Observer)
	if !ok {
		return
	}
	payload, err := protocol.AppendWirePayload(nil, protocol.SMCODisappear{
		UID:    ev.Target,
		MapUID: observer.MapUID,
	})
	if err != nil {
		return
	}
	if s.Broadcast != nil {
		_ = s.Broadcast.SendTo(observer.ChannelID, protocol.SM_CODISAPPEAR, payload, 0)
	}
}

// BuildCORecord 构造某个实体的 SM_CORECORD。
func (s *WorldService) BuildCORecord(ctx context.Context, mapID uint32, uid uint64) (protocol.SMCORecord, error) {
	e, ok, err := s.Worlds.Entity(ctx, mapID, uid)
	if err != nil {
		return protocol.SMCORecord{}, err
	}
	if !ok {
		return protocol.SMCORecord{}, fmt.Errorf("entity %s not found", common.GetUIDString(uid))
	}

	rec := protocol.SMCORecord{
		UID:    e.UID,
		MapUID: 0, // 同图，调用方补
		Action: common.ActionNode{
			Type:      common.ActionSpawn,
			Direction: uint8(e.Direction),
			X:         int16(e.X),
			Y:         int16(e.Y),
			AimX:      int16(e.X),
			AimY:      int16(e.Y),
		},
	}
	// Union 5 字节按类型解释：玩家放 dbid+level，怪物放 lookID
	switch e.Type {
	case world.EntityPlayer:
		// Player.Union 的布局见 client/src/co_record.gd：
		// 玩家：dbid(4) + level(1)，Level 在偏移 4
		if p, exists := s.Players.Get(uid); exists {
			putUint32(rec.Union[:], p.DBID)
			rec.Union[4] = uint8(p.Level)
		}
	case world.EntityMonster:
		// 怪物：lookID 在低 2 字节（客户端据此找贴图）
		putUint16(rec.Union[:], uint16(common.UIDID(uid)&0xFFFF))
	}
	return rec, nil
}

func putUint32(dst []byte, v uint32) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
	dst[3] = byte(v >> 24)
}

func putUint16(dst []byte, v uint16) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
}

// Snapshot 返回 uid 视野内可见的实体列表（进场时一次性下发）。
func (s *WorldService) Snapshot(ctx context.Context, mapID uint32, uid uint64) ([]protocol.SMCORecord, error) {
	visible, err := s.Worlds.VisibleEntities(ctx, mapID, uid)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.SMCORecord, 0, len(visible))
	for _, other := range visible {
		rec, err := s.BuildCORecord(ctx, mapID, other)
		if err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

// SendSnapshot 把某个玩家视野内的所有实体推给他。
//
// 进场时调用：AOI 事件只覆盖「新进入视野」的增量，
// 而玩家刚进场时周围已经站着的实体不会产生增量事件，需要主动推一次。
func (s *WorldService) SendSnapshot(ctx context.Context, mapID uint32, uid uint64) {
	player, ok := s.Players.Get(uid)
	if !ok {
		return
	}
	records, err := s.Snapshot(ctx, mapID, uid)
	if err != nil || len(records) == 0 {
		return
	}
	for _, rec := range records {
		rec.MapUID = player.MapUID
		payload, err := protocol.AppendWirePayload(nil, rec)
		if err != nil {
			continue
		}
		if s.Broadcast != nil {
			_ = s.Broadcast.SendTo(player.ChannelID, protocol.SM_CORECORD, payload, 0)
		}
	}
	s.Logger.Info("snapshot sent", "name", player.Name, "entities", len(records))
}

// Leave 让玩家离开世界。
//
// 对应原版 AM_OFFLINE / AM_CLOSEMAP：
//  1. 从地图移除实体（产生 CODisappear 事件）
//  2. 把消失事件发给仍能看到他的玩家
//  3. 落库位置（下次登录从这里开始）
//  4. 从 PlayerManager 移除
func (s *WorldService) Leave(ctx context.Context, channelID uint64) {
	player, ok := s.Players.ByChannel(channelID)
	if !ok {
		return
	}

	// 离开前先清理队伍关系。
	//
	// ★ 必须在移除 PlayerManager 之前：
	//
	//	TeamManager 推送新队伍列表时要按 UID 查在线玩家。
	//	先移除的话，剩下的人收到的是「成员列表里有个查不到的人」——
	//	客户端会显示一个空名字的队员，而且永远占着名额。
	if s.Teams != nil {
		s.Teams.OnLeaveGame(ctx, player.UID)
	}
	// 交易也要清理：不取消的话对方永远卡在「等待交易中」
	if s.Trades != nil {
		s.Trades.OnLeaveGame(ctx, player.UID)
	}

	// 落库：位置与血量。失败不阻断离开流程。
	if s.Chars != nil {
		if err := s.Chars.UpdatePosition(ctx, player.DBID, player.MapID, int32(player.X), int32(player.Y)); err != nil {
			s.Logger.Warn("save position failed", "dbid", player.DBID, "error", err)
		}
		if err := s.Chars.UpdateHealth(ctx, player.DBID, player.HP, player.MP); err != nil {
			s.Logger.Warn("save health failed", "dbid", player.DBID, "error", err)
		}
		if err := s.Chars.UpdateExp(ctx, player.DBID, player.Exp); err != nil {
			s.Logger.Warn("save exp failed", "dbid", player.DBID, "error", err)
		}
	}

	_, removed, events, err := s.Worlds.RemoveEntity(ctx, player.MapID, player.UID)
	if err != nil || !removed {
		s.Logger.Warn("remove player from map failed", "uid", common.GetUIDString(player.UID), "error", err)
	}

	// 广播消失（放在移除 PlayerManager 之前，因为 sendCODisappear 要查观察者）
	s.dispatchObserverEvents(ctx, player, events)

	s.Players.Remove(player.UID)
	s.Logger.Info("player left world", "name", player.Name, "uid", common.GetUIDString(player.UID))
}

// loadPlayerItems 从库里读回角色的背包 / 装备 / 腰带。
//
// 每件物品都带持久值（SDItem.Duration[2]），
// 损坏的装备（Duration[0]==0）加载后依然损坏 —— 不会「重登修复装备」。
func (s *WorldService) loadPlayerItems(ctx context.Context, p *Player) {
	if s.ItemRepo == nil || p == nil {
		return
	}

	if items, err := s.ItemRepo.LoadInventory(ctx, p.DBID); err == nil {
		p.Inventory = make([]Item, 0, len(items))
		for _, it := range items {
			p.Inventory = append(p.Inventory, fromSDItem(it))
		}
	} else if s.Logger != nil {
		s.Logger.Warn("load inventory failed", "dbid", p.DBID, "error", err)
	}

	if wear, err := s.ItemRepo.LoadWear(ctx, p.DBID); err == nil {
		for slot, it := range wear.List {
			if it.ItemID == 0 {
				continue
			}
			item := fromSDItem(it)
			p.Wear[int(slot)] = &item
		}
	} else if s.Logger != nil {
		s.Logger.Warn("load wear failed", "dbid", p.DBID, "error", err)
	}

	if belt, err := s.ItemRepo.LoadBelt(ctx, p.DBID); err == nil {
		for slot, it := range belt.List {
			if it.ItemID == 0 || slot < 0 || slot >= len(p.Belt) {
				continue
			}
			item := fromSDItem(it)
			p.Belt[slot] = &item
		}
	} else if s.Logger != nil {
		s.Logger.Warn("load belt failed", "dbid", p.DBID, "error", err)
	}

	if s.Logger != nil {
		s.Logger.Info("player items loaded", "dbid", p.DBID,
			"inventory", len(p.Inventory), "wear", len(p.Wear))
	}
}

// PreloadAllNPCs 启动时预加载**全部**地图的 NPC 脚本。
//
// ★★ 这是任务系统能工作的前提 ★★
//
//	任务脚本顶层会 getNPCharUID(mapName, npcName)，
//	而 getNPCharUID 通过地图 actor 查 NPC。
//	NPC 没加载 → 查不到 → 任务无法挂 handler → 玩家点 NPC 看不到任务。
//
//	原版启动时所有地图 activate、所有 NPC 建好，正是这个道理。
//	代价约 34MB（339 个 NPC × 一个 Lua 状态），原版同等开销。
//
// ★ 只用真实 host，不能用 nil：
//
//	host 为 nil 时 NPC 脚本里的 setNPCSell 会空指针崩，
//	340 个 NPC 只能成功 91 个，任务跟着大批失败。
func (s *WorldService) PreloadAllNPCs(ctx context.Context) (loaded int, failed int) {
	if s.NPCs == nil {
		return 0, 0
	}

	byMap, err := script.ScanNPCDir(filepath.Join(s.NPCs.ScriptRoot, "npc"))
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("preload scan failed", "error", err)
		}
		return 0, 0
	}

	mapNameToID := make(map[string]uint32, 700)
	for id := uint32(1); id < 2000; id++ {
		if name := world.MapNameOf(id); name != "" {
			mapNameToID[name] = id
		}
	}

	for mapName, defs := range byMap {
		mapID, ok := mapNameToID[mapName]
		if !ok {
			continue
		}
		// ★ 复用 LoadMap：它会建 host、加载脚本、注册到 npcs 表。
		//	差别是不 AddEntity（预加载时地图可能还没建）。
		insts, err := s.NPCs.LoadMap(ctx, mapID, mapName, s.buildNPCHost)
		if err != nil {
			failed += len(defs)
			continue
		}
		loaded += len(insts)
	}
	if s.Logger != nil {
		s.Logger.Info("npc preloaded", "loaded", loaded, "failed", failed)
	}
	return loaded, failed
}
