# 迁移清单

本文件记录 C++/数据源到 Go 包的批次映射。文件级明细见 `migration/source-index.csv`。

状态定义：

- `pending`：尚未开始；
- `in-progress`：当前批次正在迁移；
- `ported`：已按原逻辑迁移；
- `adapted`：为 Go 或新架构做了受控适配；
- `deferred`：明确延期，必须记录原因；
- `obsolete`：确认不再需要。

## 批次总览

| ID | C++/数据源 | Go 目标 | 批次 | 状态 | 备注 |
|---|---|---|---|---|---|
| C01 | `common/src` 基础常量、数学、坐标、路径、ID | `internal/common`、`internal/protocol` | B1 | in-progress | `ActionNode`、StaticBuffer、StaticVector 已完成 |
| C02 | `protocoldef.hpp`、`clientmsg.hpp`、`servermsg.hpp`、`cerealf.hpp`、`serdesmsg.hpp` | `internal/protocol` | B1 | adapted | 100 个 serdes/Lua golden 对象；94/94 声明类型全覆盖 |
| C03 | `actormsg.hpp`、`actormsgpack.hpp`、`msgf.hpp` | `internal/actor`、`internal/protocol` | B3 | pending | 消息包络与路由 |
| C04 | `*.inc` 静态记录表 | `internal/common` 或生成代码 | B4/B5 | pending | 随所属玩法批次迁移 |
| C05 | `server/src/server.sql` | `internal/storage` | B2 | ported | schema 原样嵌入 |
| C06 | 账号、角色、物品、行会、拍卖、摆摊、邮件数据库代码 | `internal/storage` | B2 | adapted | 持久化和数据访问完成；买断税务/投递编排归 B6 |
| C07 | `actorpool.*`、`delaydriver.*`、`dispatcher.*` | `internal/actor` | B3 | pending | 地图串行执行优先 |
| C08 | `netdriver.*`、`actornetdriver.*`、`servicecorenet.*`、`serverpeer.*` | `internal/transport`、`internal/game` | B3/B4 | partial | 登录链路 7 条消息已接（CM_LOGIN/QUERYCHAR/ONLINE/CREATEACCOUNT/CHANGEPASSWORD/CREATECHAR/DELETECHAR）；peer 多进程待 B6 |
| C09 | `servermap.*`、`servermapop.*` | `internal/world` | B4 | done | 网格/占用/寻路/移动/地图 .bin 加载/AOI 观察者圈/地面物品/火墙全部完成 |
| C10 | `battleobject.*`、`monster.*`、`buff*.*`、怪物子类 | `internal/world` | B4 | partial | 伤害结算/Buff/仇恨/安全区/刷新完成；19 个怪物子类与 buffact 待补 |
| C11 | `player.*`、`playerop.*`、`playernet.*`、背包和物品逻辑 | `internal/game` | B5 | in-progress | 进世界/CM_ACTION/AOI 广播/离开落库 ✅；穿脱装备/腰带/丢弃/使用 ✅；技能/交易/邮件未做 |
| C12 | `serverluacoroutinerunner.*`、`serverluamodule.*`、Lua 脚本 | `internal/script` | B5 | pending | 默认复用原 Lua 脚本 |
| C13 | `npchar.*`、`npcharop.*`、NPC 脚本 | `internal/game`、`internal/script` | B5 | pending | |
| C14 | `quest.*`、`questdb.*`、`questop.*`、任务脚本 | `internal/game`、`internal/script` | B5 | pending | 协程语义高风险 |
| C15 | 聊天、行会、交易、拍卖、摆摊、邮件 | `internal/game`、`internal/storage` | B6 | pending | |
| C16 | FLTK 管理窗口和监控窗口 | `internal/admin`、`web/admin` | B7 | pending | 改为 Web，不复刻 FLTK |

## 批次完成记录

### B0：工程准备与参考基线

状态：**完成**

- 建立 Go module 和目录骨架；
- 建立批次检查脚本 `scripts/check.ps1` / `scripts/check.sh`；
- 冻结源仓库提交 `58264934b5562cd283db7dc7daddec496b2de86e`；
- 保存工作树 patch、未跟踪文件清单和 1024 个源文件 SHA-256；
- 定位并记录资源与数据基线：
  - `E:\Desktop\game\mir2x_assets-main`：202965 个文件，约 1.44 GB，`version=1`；
  - `E:\Desktop\game\mir2x_data`：Git 提交 `9c6580c17feab74f72ff5e3b22546565e505f8e6`，约 1.89 GB；
- 建立 310 行 C++ 文件级迁移索引。

### B1：公共类型、协议和编解码

状态：**完成**

已完成：

- 生成 89 个 `CM_*` 和 117 个 `SM_*` 消息常量；
- 从 C++ 注册表生成每条消息的 framing kind 和固定负载长度；
- 实现旧协议 TCP 分帧：
  - head byte / response bit；
  - base-128 response ID；
  - empty、fixed raw、fixed compressed、variable 四类消息；
- 实现 `msgf::encodeLength` 对应的 varint；
- 实现 `zcompf::xorEncode/xorDecode` 和 `xorEncode64/xorDecode64`；
- 实现 `common.ActionNode` 28 字节 wire codec；
- 实现基础 `StaticBuffer`、`StaticVector` 语义；
- 从 C++ 头文件生成 ABI 布局 fixture，记录 MS bitfield ABI 的关键尺寸和位布局；
- 迁移全部固定长度 client/server payload 类型；
- 实现通用固定 payload wire 编解码；
- 实现 ABI 敏感的 `CMChatMessageHeader`、`CMRequestLatestChatMessage`、`SMOnlineOK`、`SMCORecord` codec；
- 实现 `StaticBuffer`、`StaticVector` 的 packed wire 表示；
- 对全部固定长度消息做 Go 编码长度与 C++ `sizeof` 全量对照测试；
- `cerealf` PortableBinary writer/reader；
- `cerealf` 的 CF_NONE/CF_XOR/CF_ZSTD 压缩封装；
- 第一批 13 个核心 `serdesmsg` 对象：
  - `SDSendPackage`、`SDObserverNetPackage`
  - `SDInitQuest`、`SDInitGuard`、`SDInitPlayer`、`SDInitNPChar`、`SDInitMonster`
  - `SDNPCXMLLayout`
  - `SDChatGroupMember`、`SDChatPeerPlayerVar`、`SDChatPeerGroupVar`
  - `SDChatPeer`、`SDChatPeerID`
- 第二批 34 个 `serdesmsg` 对象，覆盖：
  - inventory/input/NPC sell 请求；
  - player、guild、guild war、ranking；
  - ground item、firewall、safe zone；
  - learned magic、magic key；
  - chat message、NPC event；
  - `std::map`、`std::unordered_map`、optional、嵌套 vector；
  - peer config/index/list；
  - `SDChatPeerList`、`SDUIDList`。
- 第三批 34 个 item-centric 对象，覆盖：
  - `SDItem` 核心 wire 结构；
  - drop item/odds/on-die/on-hit；
  - belt、wear、secure item；
  - sell item、buy cost、pickup；
  - inventory、grant item；
  - direct trade request/offer/result；
  - equip/grab wear/belt；
  - auction、stall、item storage；
  - `SDWLDesp`、`SDUIDWLDesp`、`SDStartGameScene`。
- 第四批队伍、Buff、Health、runtime config 对象：
  - `SDRuntimeConfig`、`SDPlayerConfig`；
  - `SDBuffedAbility`、`SDHealth`、`SDBuffIDList`；
  - `SDRegisterQuest`、`SDQueryQuestUID`；
  - `SDTeamJobReq`、`SDTeamReq`、`SDTeamPlayer`；
  - `SDTeamCandidate`、`SDRequestJoinTeam`、`SDTeamMemberList`；
  - `SDTransferTeamLeader`、`SDTeamInfo`。
- 第五批递归 Lua 变量对象：
  - nil、array、table、integer、double、bool、string；
  - `luaVarWrapper` 的 nil/非 nil pointer 语义；
  - `SDRemoteCall`、`SDRemoteCallResult`、`SDSendNotify`。
- 建立可重复生成的 C++ cereal golden fixture；
- 对 100 个 serdes/Lua fixture 做 Go/C++ 字节级对照和反序列化测试；
- 新增 8 个完整 framing golden packet：
  - empty、fixed raw、fixed XOR、wide XOR64；
  - response ID；
  - variable payload；
  - client 与 server 两个方向。
- packet decoder 使用全部 framing golden 作为 fuzz seed，fuzz 通过；
- varint 与 XOR decoder 独立 fuzz 通过。
- 对 optional 缺省、空集合、map 多元素、Item/inventory/wear、runtime config、health、team 和递归 Lua 类型做边界测试；
- 协议与公共类型测试、race 测试通过。

批次结论：

- 协议声明、序列化、framing 和短时 fuzz 已完成；
- 真实 C++ 运行会话抓包移交 B8；
- `serdesmsg.cpp` 领域辅助逻辑移交对应玩法批次；
- `std::unordered_map` 多元素顺序差异已记录。

移交后续批次：

- 真实 C++ 客户端/服务端运行会话抓包和握手链路校验；
- `serdesmsg.cpp` 中的辅助序列化逻辑；
- 扩大协议对象 fuzz、深度/内存边界和长时间 fuzz。
- `std::unordered_map` 的顺序兼容说明与 fuzz 验证（Go 侧当前按 key 排序，语义兼容但多元素字节序列不保证与 C++ 迭代顺序一致）。
### B2：SQLite、配置和数据加载

状态：**完成**

完成内容：

- 原样嵌入 22 张表、11 个索引和 guild trigger；
- 纯 Go `modernc.org/sqlite` 驱动；
- 实现 `jobValid`、`SYS_PLY_VIEWR`、`checkAuctionCategory`；
- DSN 级 foreign key、WAL、busy timeout、synchronous 和 immediate transaction；
- `STRICT` 表、外键、事务回滚和自定义函数测试；
- 账号、角色仓储；
- inventory/secured/wear/belt/learned magic/player config 仓储；
- 金币原子增减；
- 角色 Lua 变量 BLOB；
- 动态任务表/字段；
- 聊天、群组、好友、黑名单；
- 行会、邀请、排名、公告和行会战；
- 拍卖、摆摊、投递和直接交易数据层；
- `cerealf` BLOB 兼容；
- JSON 配置；
- 内容清单和 SHA-256 校验。

边界说明：

- 物品/怪物/魔法/地图静态记录解析归入 B4/B5；
- 拍卖购买税务/投递编排、摆摊买家付款编排归入 B6；
- B2 的结束条件是持久化和数据访问基础设施稳定通过测试，而不是所有玩法业务事务落地。
### B3：Actor、网络和服务核心

状态：**核心完成**

完成内容：

- `internal/actor` 串行邮箱和调度；
- 消息顺序、actor stop、system shutdown、定时消息测试；
- `internal/transport` legacy TCP server；
- session 读写队列、response ID、超时和连接限额；
- malformed packet 隔离和生命周期回调；
- dispatcher 路由和 `CM_PING`/`SM_PING` bootstrap；
- 服务启动集成：配置、SQLite、actor system、TCP listener、信号退出。

边界说明：

- 登录、建角、进图等业务 handler 在 B5 注册；
- master/slave 多进程与 peer 协议后续补齐。
### B4：世界、地图与战斗运行时

状态：**运行时骨架完成（部分子项延后）**

完成内容：

- `internal/world/geometry.go`：8 方向、切比雪夫距离、`hopValid`（1/2/diagonal 平方距离规则）；
- `internal/world/mapdata.go`：`MapData`/`Land` 静态地形；
- `internal/world/map_state.go`：格子占用、实体所有权、A* 寻路（对角需两侧都可走）、
  移动校验（hop + 视线 + 占用）、世界时钟 `Advance`；
- `internal/world/damage.go`：`DamageNode`/`PlainPhyDamage`/`MagicDamage`、
  命中率 `sigmoid(delta/2.5)/2+0.5`、物理/魔法减伤、元素抗性放大；
- `internal/world/buff.go`：`Buff`/`BuffList`，seqID 自增、按 buffSeq 升序、
  叠加上限、按来源批量移除、到期事件；
- `internal/world/combat_state.go`：`SafeZone`（矩形+圆形）、`OffenderList` 仇恨；
- `internal/world/monster.go`：`MonsterRecord`、掉落互斥链表 `RollDrop`、
  AI 冷却与 2 秒路径缓存、视野、`BehaveMode`、刷新管理器；
- `map_state.go` 追加：`Combat`（完整伤害链路 + 仇恨 + 吸血）、
  `CommitDeath`（掉落只结算一次）、`DropOwner`、`AddBuff`、`SetSafeZone`、
  `SafeZoneBlocksAttack`、`MutateEntity`（原地修改实体）；
- 56 项单元测试覆盖以上全部行为。

关键行为锁定（易错点已写进注释与测试）：

| 项 | 结论 |
|---|---|
| 命中率取值范围 | `(0.5, 1.0)`，**下限是 0.5 不是 0**（公式外层 `+0.5`），差值 0 时是 0.75 |
| 掉落表结构 | 外层分组独立判定、内层是**互斥链**（按序 roll，命中即退出该组） |
| 吸血回复 | `min(damage, 20)`（monster.cpp:756） |
| 视野 0 的怪 | 钳到至少 1，否则永远不主动攻击 |
| 仇恨归属 | 掉落给仇恨最高者；`activeTime` 用于衰减 |
| 安全区 | 保护站在区内者；火墙/固定打击/宠物伤害绕过 |
| 实体读取 | `Entity()` 是**值拷贝**，修改必须走 `MutateEntity` |

边界说明（延后项）：

- `BattleObject` 的协程（`requestMove`/`requestJump`/`requestMapSwitch`）改为同步返回，
  异步动作队列在 B5 与 session 联调时补；
- 怪物子类特化（34 个类）：帧表差异已在客户端复刻中数据化，
  服务端只需 AI 差异，待 B4 收尾逐类核对；
- `servermap.cpp` 的 AOI/视野广播、地图 `.bin` 加载未做（属于 B4 剩余）；
- Buff 的 act/aura 细分（`buffact*`）只保留计数语义，细化在 B5。

### B4 收尾：AOI / 地图加载 / 地面物品 / 火墙 / 登录链路

状态：**世界运行时完成，登录链路已与客户端联调通过**

完成内容：

- `internal/world/maploader.go`：地图 `.bin` 加载（末尾 zstd 标记、端序字节、
  位域拆成独立字段）。实测 map 0 解析结果与客户端逐字节验证值完全一致：
  `800x800 walkable=350478 (54.8%) objects ground=105190 over0=52074 lights=115`
- `internal/world/aoi.go`：AOI 观察者圈（`doCircle` 三重条件、半径 `(r-1)²`、
  增量移动、视野变化、动作广播去重）
- `internal/world/ground_item.go`：地面物品（同批共享 groundSeq、**120 秒**整批过期、
  尾部优先拾取、只收集可走格）
- `internal/world/firewall.go`：火墙（`dps` 是每秒攻击次数、
  `attackGap = max(1, 1000/dps)`、只烧同格玩家与怪物）
- `internal/common/uid.go`：UID 编码（位布局 type@59 id@35 peer@31 seq@0）
- `internal/game/session.go`：登录会话（三阶段串行、多重登录判定）
- `internal/game/login.go`：7 条登录消息处理器
- `internal/game/wire.go` + `server_handler.go`：transport 适配与路由注册
- `cmd/mir2-server/main.go`：从「只回 PING」改为真正的登录服务
- `tools/seed`：建测试账号/角色的工具

**真服务端联调结果**：客户端 `--login --run-test c9c8` 连本服务端，
15 项断言全过，`SM_ONLINEOK` 177 字节 —— 与原版/mock 完全一致。

关键行为锁定（易错点）：

| 项 | 结论 |
|---|---|
| AOI 半径语义 | `doCircle(cx,cy,r)` 用 `LDistance2 <= (r-1)²`，**实际半径 r-1**；LDistance2 是**欧氏距离平方**不是切比雪夫 |
| 地图瓦片总数 | map 0 有 159,997/160,000 有效块，缺的 3 块是地图本身内容 |
| LAND 可走判定 | 序列化后 canWalk 是**独立字节**，不是位域里的 bit7 |
| 地面物品过期 | 固定 **120 秒**，按 groundSeq **整批**清除 |
| 火墙 dps | 是**每秒攻击次数**不是伤害值，`attackGap = max(1, 1000/dps)` |
| 火墙目标 | 只烧**同格**玩家与怪物，不含创造者与 NPC |
| 玩家 UID | **特例**：`(5<<59) | dbid`，dbid 在**最低位**，不走 id 段 |
| 任务/Lua 对象 UID | 同上，id 在最低位 |
| 登录校验 | **不校验** id/password 格式（原版注释明确允许 test/123456） |
| 会话清理 | 断线必须 `Drop(channelID)`，否则同账号重连被判 MULTILOGIN |

### B5 第一批：玩家进世界 + 动作链路（已与客户端端到端联调）

状态：**核心闭环打通**

完成内容：

- `internal/game/player.go`：玩家运行时（位置/HP/MP/经验/金币/背包/装备/腰带/魔法/buff）、
  `PlayerManager`、职业属性派生 `PlayerCombatStats`、等级派生 `DisplayLevel`
- `internal/game/action.go`：`CM_ACTION` 全链路
  - `handleStand`（位置重同步）
  - `handleMove`（estimateHop 校验 → 步长推导 → 移动 → 广播）
  - `handleAttack`（距离校验 → `world.Combat` 结算 → 广播动作/伤害/未命中/死亡）
  - `handlePickUp`（距离校验 → world 拾取 → 进背包 → 推 SM_INVENTORY）
- `internal/game/world_service.go`：玩家 ↔ 地图桥接
  - `Enter`：建 Player → `world.AddEntity` → **把 AOI 事件转成 SM_CORECORD/SM_CODISAPPEAR**
  - `SendSnapshot`：进场时把视野内已有实体一次性推给新玩家
  - `Leave`：移除实体 → 广播消失 → 落库位置/血量/经验
- `internal/game/wire.go` / `server_handler.go`：动作路由注册 + 断线钩子
- `world` 补：`VisibleEntities`（我能看到谁）、`CanOccupy`、`MoveEntityFull`、
  `DropItem`、`PickupOne`、`SetStats`
- `transport` 补：`Broadcaster`（按 channelID 主动推）、`SessionByID`、`SetHandler`

**端到端验证**（`e2e_test.go`，真 TCP + 真协议 + 真 SQLite）：
两个客户端连同一服务端，bob 进场能看到 alice，alice 收到 bob 出现通知，
alice 移动时 bob 收到 `SM_ACTION`。客户端 `--login --run-test c9c8` 连本服务端 15 项全过。

关键行为锁定（都是踩过才知道的）：

| 项 | 结论 |
|---|---|
| **StaticBuffer<64> 线上大小** | **68 字节**，不是 67。C++ 是 `u16 + u8[65]`=67，但 u16 要求 2 字节对齐，结构体被 padding 到 68。客户端与 mock 历史上都按 67/69 写，导致服务端读长度字段错位到 `0x3100`=12544，表现是「连上了但登录永远没回应」 |
| **maxStep 语义** | 是**单跳步长**（1/2/3），不是速度。`LDistance2{1,2}→1`、`{4,8}→2`、`{9,18}→3`。把 moveSpeed(600) 当 maxStep 传会让 `HopValid` 全部失败，玩家「怎么都走不动」 |
| **estimateHop 与 hopStep 不同** | estimateHop 只有 `distance²∈{1,2}` 算一跳（判断「报文起点=当前位置吗」）；hopStep 的 {4,8,9,18} 是移动步长推导。混用会把 2 格外起点当成正常 |
| **SM_HEALTH 编码** | 是 MessageVariable，body 必须 `CerealSerialize`。`SDHealth` 含 `SDTaggedValMap`（map），wire 编码器不支持，走 `AppendWirePayload` 会静默丢弃 —— 表现是「打中了但血量不变」 |
| **攻击 magicID=0** | `DamageNode.Valid()` 要求 `MagicID != 0`，extParam 空时必须兜底成 `MagicIDPlainAttack(1)`，否则攻击完全没反应 |
| **AOI 事件必须真的发包** | B4 算对了事件但没人发，进程内测试发现不了。必须有**端到端**测试断言网络包 |
| **进场顺序** | 先进地图再回 SM_ONLINEOK。落地坐标可能被占用/不可走，要读回真实位置，否则客户端拿到的坐标与服务端不一致，进场第一步就「被拉回」 |
| **离场落库** | `Leave` 要写位置/血量/经验，否则下次登录回到出生点、血满 |

### B5 第二批：背包与装备（已端到端验证）

状态：**6 条消息全部打通**

完成内容：

- `content/items.json` + `tools/extract/extract_items.py`：从 `itemrecord.inc`
  提取 1258 条物品，**逐条与客户端 data/itemrecord.json 的 name/pkgGfxID 校验一致**
- `internal/game/itemdb.go`：物品表加载、`Wearable`/`Beltable`/`Packable`/
  `IsPotion`/`IsMagicBook` 判定、WLG 槽位常量（1..11）
- `internal/game/inventory.go`：背包容器（`AddItem`/`removeItem`/`findItem`）、
  装备槽、腰带、药剂效果
- `internal/game/inventory_op.go`：6 条消息处理器

覆盖的消息：

| CM | SM | 要点 |
|---|---|---|
| CM_REQUESTEQUIPWEAR | SM_EQUIPWEAR / SM_EQUIPWEARERROR | 旧装备回背包 |
| CM_REQUESTGRABWEAR | SM_GRABWEAR / SM_GRABWEARERROR | 脱下来永远进背包 |
| CM_REQUESTEQUIPBELT | SM_EQUIPBELT / SM_EQUIPBELTERROR | 旧物品回背包 |
| CM_REQUESTGRABBELT | SM_GRABBELT / SM_GRABBELTERROR | 槽位 0..5 |
| CM_DROPITEM | 地面物品 | 先落地成功再扣背包 |
| CM_CONSUMEITEM | SM_HEALTH / SM_INVENTORY | 药水/秘籍 |

关键行为锁定（都是踩过才知道的）：

| 项 | 结论 |
|---|---|
| **只有 packable 才堆叠** | 药水/卷轴/护身符/药粉才合并，上限 99（SYS_INVGRIDMAXHOLD）。武器/衣服/戒指同 itemID 也各占一格。初版对所有物品合并，两把同名武器会变成 count=2 |
| **错误类消息是定长** | SM_EQUIPWEARERROR 等是 `MessageFixedCompressed`（10 字节），走 `AppendWirePayload`；`SM_EQUIPWEAR` 等成功消息是 `MessageVariable`，走 `CerealSerialize`。选错客户端解码错位 |
| **WLG 槽位值** | DRESS=1 HELMET=2 WEAPON=3 SHOES=4 NECKLACE=5 ARMRING0/1=6/7 RING0/1=8/9 TORCH=10 CHARM=11。L 段与 W 段分界在 SHOES，凭记忆写必错 |
| **腰带只有 6 格** | slot 0..5，越界回 EQBERR_BADSLOT |
| **丢弃必须先落地再扣背包** | 初版先扣再落，落地失败时用 `addItemNoRec` 归还 —— 归还的是新格（seqID 变了），原格数量已扣掉，玩家实际少东西 |
| **普通技能书不能学** | 只有「秘籍」（book.magic 非空）才能学。普通技能书是原料 |
| **itemID 必须与客户端一致** | 两边错一个 ID，客户端就显示成别的图标，且日志看不出来 |
