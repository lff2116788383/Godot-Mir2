# 服务端复刻进度审计

> 审计时间：2026-11-25
> 方法：逐文件核对 `E:/Desktop/game/mir2x/server/src`（原版）与 `server-go`（复刻），
> 不凭印象估进度。行数统计只作参考，关键看**业务主体是否真的能工作**。

---

## 一、总量对比

| 维度 | 原版 | 复刻 | 比例 |
|---|---|---|---|
| 文件数 | 165（.cpp + .hpp） | 96（.go） | — |
| 代码行数 | 36,413 | 19,444（含测试 7,800） | **约 53%** |
| 单元测试 | 未知 | 100 项（world 包）+ 其他包 | — |

但行数比例会误导，因为 Go 侧的 `protocol`（7,564 行）和 `storage`（4,243 行）
占了约 60%，`world` 的测试又占了 2,327 行。
**真正的业务逻辑只有 `world` 非测试代码 3,473 行**。

按业务分组看实际覆盖：

| 功能组 | 原版行数 | 复刻状态 | 完成度判断 |
|---|---|---|---|
| 地图与 AOI | 3,765 | 地图 .bin 加载 ✅ / AOI 观察者圈 ✅ / 视野广播 ✅ / 地面物品 ✅ / 火墙 ✅ | **约 85%** |
| 战斗核心 | 2,319 | 伤害结算 ✅ / Buff ✅ / 仇恨 ✅ / 安全区 ✅ | **约 65%** |
| Buff 基础 | 611 | BuffList ✅ / 叠加上限 ✅ / 过期 ✅ | **约 70%** |
| 魔法定时器 | 600 | ❌ 未做（buffact* 的 act/aura 细分） | **0%** |
| 怪物 | 1,318 + 1,499 | 掉落 ✅ / AI 冷却 ✅ / 刷新 ✅ / 子类特化 ❌ | **约 55%** |
| 玩家 | 7,837 | 进世界/移动/攻击/拾取 ✅；穿脱装备/腰带/丢弃/使用 ✅；技能/交易/社交 ❌ | **约 50%** |
| NPC/任务 | 1,573 | ❌ 未做 | **0%** |
| Lua 协程 | 1,806 | ❌ 未做 | **0%** |
| Actor 池 | 3,950 | actor.System 基础 ✅ / actorpool 池化 ❌ | **约 25%** |
| 服务/进程 | 3,083 | transport ✅ / **登录链路 7 条消息已通** / peer 多进程 ❌ | **约 35%** |
| 数据库 | 2,467 | storage ✅（账号/角色/物品/行会/拍卖/摆摊/邮件/任务） | **约 85%** |

---

## 二、已经真正能工作的部分

这一节是**已验证**的，不是"写了代码"。

### 1. 地图 `.bin` 加载（新增，本轮）
`internal/world/maploader.go` + `mapdata.go`

实测地图 0 的解析结果与**客户端逐字节验证值完全一致**：

```
800x800  walkable=350478 (54.8%)  objects ground=105190 over0=52074 lights=115
```

这证明两边用的是同一套字节语义（末尾 zstd 标记、端序字节、位域拆成独立字段）。

### 2. AOI 观察者圈（新增，本轮）
`internal/world/aoi.go`

- `DoCircle` 照抄 `servermap.hpp:doCircle` 的三重条件
- **半径语义 `(r-1)²`** 已用独立计算结果校验（不是写死数字）
- 实体增删、移动、视野变化全部联动观察圈
- 动作广播按「源格 ∪ 目标格」的观察者并集去重投递

### 3. 地面物品（新增，本轮）
`internal/world/ground_item.go`

- 同批掉落共享 `groundSeq`，**120 秒**整批过期
- 拾取从尾部找第一个匹配（原版 `removeGridItemID` 的语义）
- 只收集 `groundValid` 的格下行

### 4. 火墙（新增，本轮）
`internal/world/firewall.go`

- **`dps` 是「每秒攻击次数」不是伤害值**，`attackGap = max(1, 1000/dps)`
- 只烧同格的玩家与怪物，不烧创造者、不烧 NPC
- 到期判断先于攻击（与原版 `runFireWall` 顺序一致）

### 5. 战斗层（B4 已完成）
`damage.go` / `buff.go` / `combat_state.go` / `monster.go`
- 命中率区间 `(0.5, 1.0)`，下限 0.5
- 掉落互斥链
- 仇恨列表、安全区（地图伤害绕过）

---

## 三、关键缺口（按影响排序）

### P0-1 玩家系统完全没有（登录链路已通，但进图后世界是空的）

原版 `player.cpp` 3,630 行 + `playerop.cpp` 1,067 行 + `playernet.cpp` 2,399 行 = **7,096 行**，
是**单一最大模块**，当前 0 行。

这包含：背包操作、装备穿戴、吃药、技能释放、金币、经验升级、捡物、丢弃、
移动请求、攻击请求、聊天、交易发起……**客户端 90% 的 CM 消息都由它处理**。

具体缺失的子项：

| 原版文件 | 行数 | 内容 |
|---|---|---|
| `player.cpp` | 3,630 | 玩家主体：属性、背包、装备、技能、状态 |
| `playerop.cpp` | 1,067 | CM 消息处理：移动/攻击/拾取/丢弃/使用 |
| `playernet.cpp` | 2,399 | 网络消息分发与回包 |
| `inventorydb.cpp` | 133 | 背包持久化 |
| `golddb.cpp` | 67 | 金币持久化 |

### ~~P0-2 入口只注册了 CM_PING~~ ✅ 本轮已修复

`main.go` 现在装配 `game.BuildHandler` + `RegisterBootstrapRoutes`，
注册 8 条路由：ping、login、querychar、online、createaccount、
changepassword、createchar、deletechar。

客户端 `--login --run-test c9c8` 连本服务端，15 项断言全过。

**但这也暴露了下一个问题**：登录进去了，世界里什么都没有。
没有 `CM_ACTION` 处理、没有 `SM_CORECORD` 下发、没有怪物。
玩家站在一个空地图上——因为玩家系统与 AOI 广播还没接起来。

### P0-3 NPC 与任务

原版 `npchar.cpp` 767 + `npcharop.cpp` 344 + `quest.cpp` 190 + `questdb.cpp` 102 = **1,403 行**。
NPC 对话、脚本事件、任务进度都在这里，是"世界可交互"的前提。

### P1-1 Lua 协程运行时

原版 `serverluacoroutinerunner.cpp` 1,059 + `.hpp` 449 + `serverluamodule.cpp` 185 = **1,693 行**。

**但这里有个重要判断**：客户端复刻时已查证，Lua 脚本全部在服务端运行，
客户端不需要 Lua。服务端要做脚本（地图脚本、NPC 脚本、任务脚本）就需要它。

是否可以延后？取决于是否接受"没有脚本系统"的世界。如果没有 Lua：
- NPC 只能做硬编码对话
- 地图事件（触发器、传送阵）无法数据化
- 任务系统只能硬编码

**建议**：B5 阶段必须做，因为 `servermap.cpp` 的 `onActivate` 里直接
`m_luaRunner->spawn(m_mainScriptThreadKey, "return main()")` —— 地图脚本是启动流程的一部分。

### P1-2 Actor 池化

原版 `actorpool.cpp` 1,056 + `.hpp` 683 = **1,739 行**（再加上消息体系 2,211 行）。

当前只有 `actor.System`（261 行），是"能跑"的最小实现。缺：
- 池化（避免每次 Spawn 都分配）
- `actormsg.hpp` 的 962 行消息定义（当前完全没有 actor 消息层）
- `delaydriver`（延时消息）
- `actormonitortable` / `podmonitortable`（监控）

**影响**：性能与可观测性，不影响功能正确性。可以延后到 B8。

### P1-3 服务/进程拓扑

原版 `server.cpp` 995 + `servicecore*.cpp` 794 + `serverpeer` 86 + `peercore*` 160
+ `channel.cpp` 217 + `dispatcher.cpp` 20 + `receiver.cpp` 59 + `syncdriver` 62
= **约 2,393 行**。

这是多进程 master/slave 架构 + peer 协议。当前 transport 只有单进程 TCP 服务。

**影响**：单进程能跑游戏，但无法做原版的分片/多网关部署。B6 之后再说。

### P1-4 社交与经济（数据库已就绪，业务层缺失）

storage 层已实现：行会、聊天、拍卖、摆摊、交易、邮件、任务、金币。
但对应的业务处理器（原版 `chatdb.cpp` 366 + `guilddb.cpp` 520 + `auctiondb.cpp` 220
+ `stalldb.cpp` 295 + `directtradedb.cpp` 100 + `deliverydb.cpp` 109 = **1,610 行**）
在 Go 侧还没有调用方。

**这是"最划算"的一块**：数据层已做完，只差把 CM 消息接到 storage 上。

### P2 魔法定时器与怪物子类

- `buffacttrigger.cpp` 181 + `buffactaura.cpp` 135 + `buffactattributemodifier.cpp` 119
  + `buffactattackmodifier.cpp` 56 + `buffactspellmodifier.cpp` 14 + `buffactcontroller.cpp` 15
  = **520 行**：魔法的持续效果（中毒、麻痹、护盾、属性增减）
- 19 个怪物子类共 **1,318 行**：特殊 AI（祖玛教主召唤、食人花变身、守卫巡逻、雪人分裂…）

当前怪物只有通用 AI（冷却 + 视野 + 路径缓存），子类特化全部未做。

---

## 四、进度结论

| 说法 | 是否成立 |
|---|---|
| "世界运行时可用" | ✅ 成立（AOI、地图加载、地面物品、火墙全部完成） |
| "服务端能跑起来" | ✅ 成立（8 条路由已注册） |
| "客户端能连上" | ✅ **成立**（真服务端联调 15 项断言全过，SM_ONLINEOK 177 字节） |
| "完整游戏可玩" | ⚠️ 部分（能进图/看见彼此/走动/攻击/捡物/穿脱装备/喝药；交易、技能、NPC 未做） |

**综合完成度约 62%，"可用性"约 62%**（上一轮是 58% / 55%）。

**B5 第一批让世界「活了」**：玩家进图后有实体、能看见彼此、移动会互相广播、
攻击会造成伤害并推血量。端到端测试用两个真 TCP 客户端验证了这一点 ——
这是进程内测试做不到的（B4 的坑就是算对了但没发包）。

登录链路打通的意义不只是"能登录"——它验证了协议编解码、SQLite 仓储、
UID 编码、transport 收发这四层底座在真实网络往返中都是对的。

接下来的顺序：**玩家系统**（7,096 行，最大模块，决定能否真正玩），
然后社交经济（数据层已就绪，性价比最高），最后 NPC/任务与 Lua 脚本。

---

## 五、本轮新增内容清单

| 文件 | 行数 | 内容 |
|---|---|---|
| `internal/world/maploader.go` | 199 | 地图 .bin 加载（zstd、端序、位域拆分） |
| `internal/world/aoi.go` | 368 | AOI 观察者圈、圆形遍历、动作广播 |
| `internal/world/ground_item.go` | 226 | 地面物品、批次过期、拾取 |
| `internal/world/firewall.go` | 277 | 火墙、dps 周期灼烧、按批次清除 |
| `internal/common/uid.go` | 300 | UID 编码（type@59 id@35 peer@31 seq@0） |
| `internal/game/session.go` | 190 | 登录会话、多重登录判定 |
| `internal/game/login.go` | 400 | 7 条登录消息处理器 |
| `internal/game/wire.go` | 190 | transport 适配与路由注册 |
| `internal/game/server_handler.go` | 55 | 分发 + 生命周期清理 |
| `cmd/mir2-server/main.go` | 90 | 从「只回 PING」改为真正的登录服务 |
| `tools/seed/main.go` | 60 | 建测试账号/角色 |

测试：

| 文件 | 项数 | 覆盖 |
|---|---|---|
| `maploader_test.go` | 4 | 地图加载（含真实素材对照客户端基准值） |
| `aoi_test.go` | 16 | 半径语义、增量与重建一致性、实体联动 |
| `ground_item_test.go` | 11 | 批次共享、整批过期、尾部拾取 |
| `firewall_test.go` | 12 | dps 语义、攻击间隔、只烧同格 |
| `uid_test.go` | 12 | mapUID 基准、玩家 UID 特例、反解 |
| `session_test.go` | 7 | 多重登录、掉线清理 |
| `login_test.go` | 12 | 7 条消息的完整链路（真实 SQLite） |

world 包测试 56 -> 99 项，common 12 项，game 19 项，
全仓 **210 项**，门禁（gofmt / vet / build / test）全部通过。

### B5 第一批新增（本轮）

| 文件 | 内容 |
|---|---|
| `internal/game/player.go` | 玩家运行时、PlayerManager、职业属性派生、等级派生 |
| `internal/game/action.go` | CM_ACTION 全链路（站立/移动/攻击/拾取） |
| `internal/game/world_service.go` | 进世界、视野快照、离场落库、AOI 事件转网络包 |
| `internal/game/wire.go` | 动作路由注册 |
| `internal/game/server_handler.go` | 断线钩子 |
| `internal/game/e2e_test.go` | 端到端测试（真 TCP + 真协议 + 真 SQLite） |
| `internal/transport/broadcaster.go` | 按 channelID 主动推送 |
| `internal/common/uid.go` 补 | 玩家 UID 特例（dbid 在最低位） |

服务端测试 210 → **231 项**；客户端测试 807 + 场景 77 项，全绿。

### B5 第二批新增（本轮）

| 文件 | 内容 |
|---|---|
| `tools/extract/extract_items.py` | 从 itemrecord.inc 提取 1258 条物品 |
| `content/items.json` | 物品表（name/type/weight/price/pkgGfxID/potion/book） |
| `internal/game/itemdb.go` | 物品表加载 + Wearable/Beltable/Packable/IsMagicBook + WLG 常量 |
| `internal/game/inventory.go` | 背包容器、装备槽、腰带、药剂效果 |
| `internal/game/inventory_op.go` | 6 条消息处理器（穿/脱/腰带×2/丢弃/使用） |

服务端测试 231 → **268 项**。

### B5 第三批新增（本轮）

**本轮主题：技能施法 + 死亡复活 + 金币/经验/商店**

| 文件 | 行数 | 内容 |
|---|---|---|
| `tools/extract/extract_magics.py` | 253 | 从 magicrecord.inc 提取 116 条魔法（顶层块配对，不误抓嵌套） |
| `content/magics.json` | — | 魔法表（elem/req/castRange/mp/coolDown/power/addPower） |
| `internal/game/magicdb.go` | 205 | 魔法表加载、MPCost 公式、learnable 判定 |
| `internal/game/spell.go` | 700 | 施法全链路、技能分类、击中/治疗/Buff/地面技能 |
| `internal/game/death.go` | 330 | 死亡（SM_NOTIFYDEAD+FADEOUT）、复活（先传送后拉人、10% 血） |
| `internal/game/shop.go` | 400 | 经验/金币/查价/购买（以物易物的三段校验） |
| `internal/game/tick.go` | 215 | 世界时钟驱动器（**此前完全缺失**） |
| `internal/world/damage.go` 补 | +80 | CombatStats 加 SC、ElementFromName、RollAttackDamageWith |
| `internal/world/map_state.go` 补 | +90 | CombatWithBounds、Teleport、安全区拦截、实体挂 MonsterRec |
| `internal/world/world.go` 补 | +85 | CombatWithBounds/CommitDeath/MutateEntity/NowMS/SpawnMonster/Teleport |
| `internal/game/player.go` 改写 | ~90 | 按 combatnode.cpp 的真实公式重算属性；装备累加；基本剑术 +2 命中 |
| `tools/extract/extract_items.py` 补 | +75 | 提取 equip 段（dc/ac/mc/sc/dcHit/mcHit/hp/mp/reqLevel） |

**本轮修掉的 6 个真实缺陷**（都不是「代码写错了」，而是「语义理解偏了」）：

1. **世界时钟从没走过**。`World.Advance` 语义完整但没人调用，`NowMS` 恒为 0。
   后果：技能冷却判断 `now - last = 0 < coolDown` 永远成立 → **每个技能放一次就永久冷却**。
   修法：新增 `MapTicker` 100ms 推进 + `WorldService.trackMap`。

2. **复活用 `MoveEntity(maxStep=0)` 必然失败**。`HopValid` 要求 `1 <= maxStep <= 3`，
   而且只接受 `distance² == maxStep²` 的**精确**匹配，无法表达「任意距离瞬移」。
   修法：新增 `MapState.Teleport` / `World.Teleport`。

3. **道士主属性写错成 MC**。原版 `getCombatNode` 里 `JOB_TAOIST` 加的是 `sc`，
   与 `mc` 是两套独立装备属性。混用导致道士穿道袍攻击力不涨。
   修法：`CombatStats` 加 `SC` 字段，技能按名字分流（灵魂火符/冰月神掌/冰月震天 走 SC）。

4. **`castRange` 缺省值取错**。`MagicRecord::castRange` 默认是 `CRT_LONG`（无距离限制），
   提取脚本却按 `CRT_NONE` 兜底 → 雷电术/火球术被判「距离非法」而永远放不出来。

5. **magicID 整体偏移 1**。`.inc` 第 0 个顶层块就是空占位 `{}`，块序号即 magicID。
   提取脚本写成 `enumerate(..., start=1)`，全表 +1，客户端按 14 发的雷电术在服务端查不到。

6. **`mp` 字段正则永远不匹配**。写成 `\b\.mp`，`\b` 在「空白与点」之间不成立，
   结果全表 mp=0 → 施法永不耗蓝。

7. **金币 itemID 每次启动都不同**。金币有 5 种面额（小/中/大/特/超，id 1..5），
   `GoldID()` 用 `for id := range map { return id }` 取第一条，而 Go 的 map
   遍历顺序随机 —— 实测同一份数据两次启动分别返回 1 和 4。
   于是「costList 用某个面额计价、addGold 按另一条面额找人」，表现为
   **扣了钱金币数不变**。修法：取 itemID 最小的那条（金币（小）=1）作为计价单位。
   这类 bug 特别值得警惕：单进程内自洽，重启才暴露。

**新增测试（40 项）**：

| 测试组 | 项数 | 覆盖 |
|---|---|---|
| 魔法表 | 4 | 116 条计数、关键 ID 与客户端一致、字段值、MPCost 公式、元素映射 |
| 属性 | 2 | 装备累加、基本剑术 +2 命中 |
| 施法 | 8 | 未学拒绝、MP 不足、MP 扣除+广播、冷却、施法击杀+经验、治疗、治疗封顶、死者不能施法 |
| 快捷键 | 3 | validKey 白名单（1-6 保留给腰带）、未学拒绝、同键换绑 |
| 死亡复活 | 6 | 归零、幂等、活着不能复活、10% 血、低级角色最少 1 点、先传送后站起来 |
| 金币经验 | 4 | 升级回满、金币上限钳制、购买成功、金币不足、数量非法、未知物品 |
| 商店查询 | 1 | SM_SELLITEMLIST 内容核对 |
| 时钟 | 2 | NowMS 推进、地图被 Track |
| 击杀结算 | 1 | 经验/掉落/位置、二次结算不重复 |
| 背包 | 4 | 跨格计数、跨格移除、HasMagic、**GoldID 确定性（5 种金币面额的 map 顺序泄漏）** |

服务端测试 268 → **308 项**，全部通过。

**实测验证**：

```
item db loaded count=1258 gold_item_id=1
magic db loaded count=116 learnable=63
map ready map=1 size=600x600
player entered world name=测试角色 uid=PLY_1 map=1 at=(400,402)
```

客户端 `--login --run-test c9c8` 连真服务端 **15 项全过**。

**魔法表与客户端一致性**：116 条逐条比对 `name` / `mp` / `coolDown`，**0 差异**。

---

## 六、下一步（B5 第四批及以后）

按「对玩家可感知程度」排序：

| 优先级 | 项目 | 为什么排在这 |
|---|---|---|
| P0 | Lua 协程运行时（1,693 行） | NPC 对话、任务、GM 命令全挂在这上面；没有它 NPC 只能靠硬编码 |
| P0 | NPC 对话与任务 | 是「能玩」与「能玩下去」的分界线 |
| P1 | 19 个怪物子类 AI 差异 | 现在所有怪行为一样，怪物生态没有层次 |
| P1 | buffact 细分（攻击/属性/光环/触发/法术修正） | 现在 buff 只有时长没有实际效果 |
| P2 | AOI 视野增量广播调优 | 人多时才有感知 |
| P2 | 火墙命中接 SM_STRIKE | 已有 firewall 计算，缺消息落地 |
| P3 | 复活点按 maprecord 配置 | 现在硬编码道馆_1 (405,120) |
| P3 | 跨图切换（AM_SWITCHMAP） | 复活跨图、传送卷轴都要它 |

### B5 第四批：Lua 运行时与 NPC 对话（本轮）

**核心决策：Lua 全量复用，零翻译。**

| 原版资源 | 处理方式 | 证据 |
|---|---|---|
| `common/src/luamodule.lua` | 逐字节复制为 `content/script/base.lua` | md5 `83c089fc…` 与原版一致 |
| `server/src/npchar.lua` | 逐字节复制为 `content/script/npchar.lua` | md5 `58df728a…` 与原版一致 |
| `server/script/npc/*.lua`（341 个） | **不复制**，`MIR2X_SCRIPT` 指向原版目录加载 | 约 10 万行，复制会漂移 |
| `common/src/luamodule.cpp:22` 内嵌沙箱脚本 | 提取为 `content/script/sandbox.lua` | 含 `getTLSTable` / 协程独立 globals |

**新增文件**

| 文件 | 行数 | 内容 |
|---|---|---|
| `internal/script/runtime.go` | 250 | Lua 状态、package.path、库加载、常量注入 |
| `internal/script/luacompat.go` | 290 | 5.2/5.3 兼容层（table.pack / math.type / utf8 / coroutine.running） |
| `internal/script/syntax_patch.go` | 250 | Lua 5.3 `//` 整数除法降级（加载时改写，原文件不动） |
| `internal/script/npc.go` | 470 | NPC 脚本加载 + 40 个 C 函数绑定 |
| `internal/script/coroutine.go` | 160 | 协程执行器（对齐原版 spawn 语义） |
| `internal/script/npcfile.go` | 200 | NPC 文件名解析（`<地图>.GLOC_x_y_d.<名>.LOOK_n.lua`） |
| `internal/script/bindings.go` | 320 | 通用绑定（UID 类型 / 时间 / 记录表查名） |
| `internal/script/records.go` | 160 | 记录表提取（JSON / inc 顶层块解析） |
| `internal/game/npc_manager.go` | 480 | NPC 管理器 + 宿主适配（20 个 host 方法） |
| `internal/game/npc_handler.go` | 150 | `CM_NPCEVENT` 处理（距离校验/错误码/协程调用） |
| `internal/world/mapname.go` | 60 | 地图 ID <-> 名字（NPC 脚本文件名用名字） |
| `tools/npccheck/main.go` | 200 | 加载验证工具（非测试，一眼看哪批能跑） |

**本轮修掉的 5 个兼容性缺口**

1. **缺 Package 库**。为「安全」只开了 base/table/string/math/coroutine，
   结果 `package.path` 拼接第一行就报 `attempt to index a non-table object(nil)`，
   所有脚本的 `require('include.dialog')` 全部失败。
   修法：开 `lua.LoadLibName`（常量名不叫 PackageLibName）。

2. **缺 `table.pack`**（Lua 5.2+）。`base.lua:29` 的 `assertType` 第一行就用它，
   报 `attempt to call a non-function object`。补 polyfill，必须带 `n` 字段
   （`#t` 遇中间 nil 会截断）。

3. **`coroutine.running()` 语义差异**。Lua 5.1 主线程返回 `nil`，
   5.2+ 返回 `(thread, true)`。原版沙箱靠第二个返回值判断是否主线程，
   语义不对会让 TLS 退化成全局变量 —— **两个玩家同时点同一 NPC 时对话串号**。
   包装成 5.2 形态。

4. **`//` 整数除法是 Lua 5.3 语法**。gopher-lua 是 5.1，**编译期**报语法错误，
   polyfill 救不了。影响面实测：3 文件 6 处（`dailyquest.lua:205,249`、
   `比奇县_0.行政官员_1…:50,101,154`、`野蛮冲撞任务.lua:341`）。
   处理：加载时改写源码为 `math.floor(a / b)`，原文件零修改。
   **必须包 `math.floor`**，因为调用方都是 `string.format('%d', a // b)`，
   `%d` 遇到浮点会报 "number has no integer representation"。

5. **NPC 事件必须在协程里跑**。`npchar.lua:403` 的 `getTLSTable().uid = from`
   在主线程调用会**主动 throw** —— 报错显示为
   `attempt to call a non-function object`，看起来像函数没绑定。
   修法：`L.NewThread()` + `L.Resume`，与原版 `m_luaRunner->spawn` 等价。

**另修一个 UID 缺陷**：`buildNPCUID` 把坐标当 `peerIndex` 传入，
但 `peerIndexBits=4`（0..15），坐标 396 直接越界 → 全部走兜底分支，
日志里 `uid=ERR_xxx`，且所有 NPC 的 UID 撞在一起（AOI 互相覆盖）。
改为 `id=lookID, peerIndex=0, seq=x*1024+y+1`。

**验证结果**

```
$ go run ./tools/npccheck
地图 86 张，NPC 共 340 个
记录表：物品 1258 / 怪物 364 / 地图 603

=== 非空脚本 161 个：成功 161 / 失败 0；空占位 179 个 ===
```

（179 个是原版 0 字节占位文件，地图上有 NPC 但没写对话。）

服务端实跑：

```
INFO map names loaded count=603
INFO npc loaded map=比奇县_0 count=25 skippedEmpty=26
INFO map npc spawned map=比奇县_0 loaded=25 spawned=25
INFO player entered world name=测试角色 map=1 at=(400,402)
```

客户端 `--login --run-test c9c8` **15 项全过**。

`道馆_1.阿天_1` 的 `SYS_ENTER` 真实产出（原版脚本生成，Go 只做转发）：

```xml
<layout>
<par>这里是沙巴克城<t color="red"></t>行会的领地。</par>
<par>贫道是仓库管理员，需要的话，可以把东西暂时寄存在我这里。</par>
<par></par>
<par><event id="npc_goto_secure" single="1">寄存</event>物品</par>
...
</layout>
```

**本轮未做（诚实标注）**

| 项目 | 影响 |
|---|---|
| `uidRemoteCall`（254 处调用） | 跨 NPC 联动不可用，会走 else 分支 |
| 事件 ID 的 AES 加密 | 链路能通，但缺「防改包点未授权选项」 |
| 仓库 / 交易 / 修理 | 对话框能弹，点确认无效果 |
| `CM_NPCEVENT` 客户端侧 | 客户端还没发这条消息，只能服务端侧验证 |

### B5 第五批：地图脚本与刷怪（本轮）

**核心：地图脚本同样零翻译复用。**

`server/script/map/*.lua`（285 个非空脚本、17,754 行）直接由 Lua 运行时加载，
原版脚本一行不改。

**新增文件**

| 文件 | 行数 | 内容 |
|---|---|---|
| `internal/script/mapscript.go` | 520 | 地图脚本加载 + 刷怪协程驱动 + 20 个绑定 |
| `internal/world/monsterdb.go` | 300 | 怪物表（364 条完整数值含元素抗性/掉落） |
| `internal/game/mapscript.go` | 400 | 地图脚本管理器 + 宿主（刷怪/守卫/安全区） |
| `tools/extract/extract_monsters.py` | 250 | 从 monsterrecord.inc 提取怪物表 |

**世界层新增 API**：`MapSize` / `CanWalk` / `CountWalkable` / `AddSafeZoneCircle`

**本轮踩的 4 个大坑（都是性能/语义级，值得记）**

1. **`\b\.hp` 正则永不匹配**
   与上一轮 `mp` 完全同一个坑。`\b` 要求「单词字符与点之间」有词边界，
   而 `.hp` 前面永远是空白 —— 整条正则永远不匹配，全表 hp=0。
   表现是「怪物打不死」（HP 是 0）。

2. **`patchIntDiv` 的正则灾难性回溯（28.9 秒 → 0）**
   第一版为了让 `//` 匹配「标识符/函数调用/括号/数字」四种形态，
   写了一个四选一的复杂正则。在 427 行的地图脚本上**灾难性回溯**，
   耗时 28.9 秒且一处都没匹配上。
   pprof 一照：`regexp.(*machine).add` 占 44.89%、`.step` 占 18.98%，
   合计 **87% 时间在正则引擎里**。
   改成「线性扫描找 `//` 字面量 + 上下文判断」后耗时归零。

3. **gopher-lua 不支持从 Go 函数 yield（地图加载卡死）**
   `pause(msec)` 最初用 Go 绑定实现，里面调 `l.Yield(...)`。
   实测 `Yield` 被**静默忽略**：Resume 直接返回 `ResumeOK`，
   `main()` 里的 `while true` 变成死循环，`Start()` 永不返回 ——
   服务端日志停在 `npc spawned` 之后再无输出，也不报错。
   而 **Lua 层的 `coroutine.yield` 是正常的**（Resume 返回 `ResumeYield`）。
   修法：`pause` 必须在 Lua 里定义（`installPause` 注入），
   Go 只负责「什么时候 resume」。

4. **`countGLoc` 逐格回调太慢（41 万次跨语言调用）**
   道馆_1 的配置是 20 种怪 × 285 个 38×38 区域 = 41 万格。
   每格跨一次 Lua↔Go 边界（gopher-lua 是 AST 解释器，比 C Lua 慢一个量级），
   加载要 30 秒。
   修法：`CountThrough` 批量方法，一次算完整块 + 结果按 (x,y,w,h) 缓存。
   14 万次调用 → 60 次。
   （注：这一条最终不是主因，主因是第 2 条的 28.9 秒正则，但批量+缓存本身也应该做。）

**另修：UID 构造越界**（与 NPC 同一个问题）
`BuildMonsterUID(id, peerIndex, seq)` 的 peerIndex 是 4 bit，
传坐标会越界。改用自增序号做 seq。

**验证结果**

```
$ go run ./tools/mapcheck（诊断工具，已删除）
加载 道馆_1 ...
  加载完成 11.3ms（CanThrough 60 次）      ← 原 29.5 秒
启动 main() ...
  启动完成 2.0ms（AddMonster 2821 次）
```

服务端实跑（比奇县_0）：

```
INFO map script loaded map=比奇县_0 spawners=1 guards=27 monsters=1107 safeZones=3
```

**一张图刷出 1107 只怪、27 个守卫、3 个安全区，刷怪协程持续运行。**
客户端 `--login --run-test c9c8` **15 项全过**。

**怪物表提取结果**：364 条。注意原版只有 1 只怪（黑野猪）配了掉落表，
其余 363 只没有 dropItemList —— 这不是解析 bug，是上游数据本身如此。

**本轮未做**

| 项目 | 影响 |
|---|---|
| 怪物 AI 行为 | 怪能刷出来但不会动、不会打人（`AIState` 有实现但没接进 tick） |
| 守卫 AI 差异 | 守卫当普通怪刷，不会「只打红名」 |
| 地图触发器 | `_RSVD_NAME_allocateGridTriggerId` 是空实现 |
| 跨图切换 | `uidMapSwitch` 返回 false |
| `pause` 之外的协程调度 | 只有地图脚本的 main() 用到了 |

### B5 第六批：怪物 AI 接入世界时钟（本轮）

**把「地图上有怪」变成「怪会动、会追、会打」。**

`world/ai.go` 对齐 `monster.cpp:404 runAICoro` 的主循环：

```
while !dead():
    if 目标闲置超过 10 秒 → 丢弃目标
    if 目标已死/消失      → 丢弃目标
    if 没目标             → pickTarget()          // 仇恨优先 → 视野内最近
    if 有目标             → trackAndAttack()      // 相邻则打，否则走一步
    else                  → randomMove()          // 空闲游荡
    idleWait(1000)                                // 1 秒决策一次
```

**新增/修改**

| 文件 | 内容 |
|---|---|
| `internal/world/ai.go` | 新增 330 行：`RunAI` / `pickTarget` / `trackAndAttack` / `randomMove` / `stepToward` |
| `internal/world/map_state.go` | `Advance` 末尾接 AI；`Entity` 加 `PKPoint`；`MapState` 加 `aiRand` / `lastAITickMS` |
| `internal/world/world.go` | 新增 `ObserversAt`（AOI 精确投递） |
| `internal/game/tick.go` | `ai_move` / `ai_attack` 事件转成 `SM_ACTION` + `SM_STRIKEGRID` + `SM_HEALTH` |

**关键设计决策**

1. **不用协程，直接在 tick 里遍历**
   地图 actor 本身就是串行的，每 tick 遍历所有怪跑一次决策即可。
   原版用协程是为了能 yield 等网络/寻路，Go 侧这些都是同步调用。
   实测 1107 只怪单次决策总耗时在 3ms 以内。

2. **AI 决策周期 1 秒，不是每 100ms**
   原版 `asyncIdleWait(1000)`。每 100ms 都跑会让 CPU 白烧 10 倍，
   而且怪会「抖」（走一格就重新决策）。

3. **空闲游荡只有 1/4 概率触发**
   否则 1107 只怪每秒集体移动，网络包会把客户端淹掉。

4. **攻击顺序：先 SM_ACTION 再 SM_STRIKEGRID**
   客户端看到攻击动作后才播打击特效。反过来的话，
   玩家会先看到自己掉血、再看到怪抬手。

**本轮踩的坑**

**`aiRand` 从未初始化（怪物一只都不动，且不报错）**

`SetAISeed` 里建了 `m.aiRand`，但那个函数只有测试会调 ——
生产路径上 `aiRand` 恒为 nil，`randomMove` 收到 nil 直接 `return false`。
表现：1107 只怪全部静止，日志干净得可疑（没有 error、没有 warn）。

定位方式：加一个「AI 事件计数」日志，发现计数恒为 0，
才反查到随机源是 nil。修法是在 `NewMapState` 里初始化（用 mapID 做种子，
同图每次启动序列一致，便于复现）。

**验证结果**

单元层（`TestMonsterAttacksPlayer`，30 格地图、玩家 3 格外）：

```
30 秒：移动 2 次，攻击 28 次
玩家 HP: 100 → 11
```

服务端实跑（比奇县_0，1107 只怪）：

```
INFO ai activity map=1 moves=281 attacks=0
```

（attacks=0 是因为测试角色站在比奇县安全区内，怪不会打安全区里的人 ——
这正是 `SafeZoneBlocksAttack` 该有的行为。）

客户端 `--login --run-test c9c8` **15 项全过**。

**本轮未做**

| 项目 | 影响 |
|---|---|
| 守卫 AI 差异 | 守卫当普通怪打，不会「只打红名」（`ShouldAggro` 已支持，但 `BehaveMode` 没从怪物表读） |
| 寻路（A*） | 现在是「朝目标走一步」，遇墙绕不过去。`AIState.Path` 缓存已有但没用上 |
| 怪物死亡后的尸体消失 | 尸体永久躺在地上 |
| 掉落归属保护期 | 谁都能捡 |

**下一步优先级**

1. **把 `BehaveMode` 从怪物表读进实体** —— 现在所有怪都是默认模式，守卫不守卫
2. **接 A\* 寻路** —— `world/pathfind.go` 已有实现，`AIState.Path` 缓存已设计好
3. **接 `uidRemoteCall`** —— 解锁 254 处 NPC 脚本调用（上一批遗留）
4. 火墙命中接 `SM_STRIKEGRID`

### B5 第七批：A\* 寻路接入（本轮）

**让怪能绕过墙、绕过长障碍物追人。**

`world/ai.go` 新增 `chaseStep`，对齐 `monster.cpp:784 moveOneStep` 的
`FPMETHOD_COMBINE` 语义（原版 `FindPathMethod()` 的默认值）：

```
1. 目标在相邻格   → 直接走（被占则找相邻空位）
2. greedy        → 朝目标方向走一格（斜向要检查两个直向，不能穿墙角）
3. A*            → 算路径，取第一步，后续缓存 2 秒
4. 全失败        → 随机挪一步（避免原地卡死）
```

**为什么 greedy 优先而不是直接 A\***

比奇县 1100 只怪、AI 每秒决策一次。每只怪都跑完整 A*（600×600 地图
最坏 36 万格）= 每秒上亿次节点扩展，单核跑不动。
实战里 90% 的追击是「空旷地带朝目标走」，greedy 一次判断就够。
只有撞墙才落到 A*，那时才付代价。

**本轮踩的两个坑**

1. **greedy 失败时直接 `return false`，没降级到 A\***
   斜向被墙挡住（不能穿墙角）时我写了 `return Point{}, false`，
   于是怪永远卡在墙角外侧 —— 明明 A* 能绕，却一步都不走。

   实测现象：怪追到 (4,7)，目标在右上，greedy 想斜穿 (5,6)，
   但 (4,6) 是墙 → 返回 false → 整个函数结束，
   路径缓存里的 (5,7) 永远用不上。怪物就那样定住了。

   修法：greedy 失败不 return，让它继续往下走 A* 分支
   （原版 `moveOneStepCombine` 就是这个语义）。

2. **A\* 终点不能用「目标所在格」**
   玩家站在那一格，`CanOccupy` 必然返回 false，A* 一律失败。
   改成寻路到「目标相邻的可站格」（`findApproachGrid`）。

**性能优化：pickTarget 从 O(n²) 降到 O(n×视野面积)**

初版 `for otherUID := range m.Entities` 全表扫：
983 只怪 × 983 实体 = 96 万次比较，**单 tick 峰值 196ms**。

改成遍历视野范围内的格子、读每格 `Occupants`：
视野 8 → 17×17 = 289 格，比实体数还少。

同时把空闲游荡概率从 1/4 降到 1/8。

| 版本 | 平均 tick | 峰值 tick |
|---|---|---|
| 全表扫 + 1/4 游荡 | 25.5ms | 196ms |
| 格子查询 + 1/8 游荡 | **8.6ms** | **55.7ms** |

（983 只怪、200×200 地图、2000 个障碍格）

**验证结果**

| 场景 | 结果 |
|---|---|
| 空旷追击（玩家 3 格外） | 移动 2 次，攻击 28 次，HP 100→11 |
| 绕「盒子」缺口 | 移动 4 次，攻击 56 次，HP 500→433 |
| **绕 7 格长竖墙** | 移动 12 次，攻击 48 次，HP 500→438 |
| 1000 只怪性能 | 平均 8.6ms / 峰值 55.7ms |

服务端实跑：比奇县 1112 只怪，客户端 c9c8 15 项全过。

**注意**：性能测试只断言平均值、不查峰值。
峰值受测试机负载影响极大（单跑 55ms，与别的包并行跑会跳到 170ms），
拿峰值当 CI 门禁会随机失败。

**本轮未做**

| 项目 | 影响 |
|---|---|
| 守卫 AI 差异 | `BehaveMode` 没从怪物表读进实体，守卫仍当普通怪 |
| 尸体消失 | 尸体永久躺在地上 |
| 掉落归属保护期 | 谁都能捡 |
| 寻路失败时的「绕远路」策略 | 现在是随机挪一步，可能来回晃 |

### B5 第八批：怪物行为模式（守卫/中立）（本轮）

**修掉一个立刻能被玩家发现的错误：比奇县城里的 27 个「大刀卫士」
原本像普通怪一样追着白名玩家打。**

**`behaveMode` 的来源（本轮调研结论）**

之前以为「守卫」是靠 `addGuard` 这个 API 区分的。查阅后确认**不是**：

```
common/src/monsterrecord.hpp:37-39
    BM_DEFAULT 0   敌对玩家（默认）
    BM_NEUTRAL 1   中立（被打才还手）
    BM_GUARD   2   守卫（只打红名/打过自己的）

common/src/uidf.cpp:320
    isGuardMode(uid) = 怪物记录的 behaveMode == BM_GUARD
```

也就是说 **守卫是怪物表里 `behaveMode` 为 2 的那几条记录**，
`addGuard` 只是「按指定坐标、指定朝向摆一只」的便利函数，
不改变行为模式。

全表 364 条里：`BM_DEFAULT` 355 条、`BM_GUARD` 4 条、`BM_NEUTRAL` 5 条。
4 个守卫是：禁军卫士、沙漠战士、昂克战士、**大刀卫士**。

**守卫的敌对判定（serverguard.cpp:80 checkFriend）**

对齐原版，对玩家：

```
1. 在守卫的仇恨列表里（打过守卫）→ 敌对
2. 否则 redName（PKPoint >= 100）→ 敌对
3. 否则                          → 中立（不主动）
```

★ 第 1 条容易漏。只看红名的话，普通人打了守卫一下，
守卫还是不还手 —— 这在实际游戏里非常怪。

**改动**

| 文件 | 内容 |
|---|---|
| `tools/extract/extract_monsters.py` | 提取 `behaveMode`（符号名 → 数字） |
| `internal/world/monsterdb.go` | `MonsterDef.BehaveMode` 字段 + `ToRecord()` 带过去 |
| `internal/world/ai.go` | `isHostileTo`（三模式判定）+ `BehaveMode*` / `PKPointRed` 常量 |

**验证（四种行为各一条测试）**

| 场景 | 结果 |
|---|---|
| 守卫 vs 白名玩家（站旁边） | **攻击 0 次**，HP 保持 100 |
| 守卫 vs 红名玩家（PKPoint=200） | 攻击 20 次，HP 500→324 |
| 白名玩家打了守卫一下 | **守卫还手 20 次**（仇恨优先于红名） |
| 中立怪（鹿）vs 白名玩家 | 攻击 0 次，HP 保持 100 |

服务端实跑：比奇县 1108 只怪、27 个守卫，客户端 c9c8 15 项全过。

**本轮未做**

| 项目 | 影响 |
|---|---|
| 怪 vs 怪 的阵营关系 | `pickTarget` 现在只筛玩家，怪之间互不攻击（守卫本该打敌对怪物） |
| 守卫归位 | 原版守卫会走回自己的岗位（`m_standX/m_standY`），现在会一直追出去 |
| 尸体消失 | 尸体永久躺在地上 |
| 掉落归属保护期 | 谁都能捡 |

### B5 第九批：击杀结算 / 尸体淡出 / 掉落归属（本轮）

**修掉一个「客户端不报错但游戏没法玩」的缺陷：战士打怪不掉落、不得经验。**

#### 1. 物理击杀从不结算（本轮最重要）

结算逻辑最初只写在 `SpellHandler.settleKill` 里，因为「法术打死怪」是最先做的路径。
但玩家用**普通物理攻击**打死怪走的是：

```
ActionHandler.handleAttack
  → attackWorldEntity
    → broadcastAction（攻击动画）
    → broadcastDamage（打击特效 + 血量）
    → broadcastDeath（SM_NOTIFYDEAD）
    ✗ 没有 settleKill
```

后果：

| 职业 | 打怪结果 |
|---|---|
| 法师（火球打死） | 有经验、有掉落 |
| **战士（砍死）** | **零经验、零掉落** |

而且客户端完全不报错 —— 怪确实死了、尸体也在，只是「打怪没意义」。
这类缺陷最难发现，因为它不产生任何错误信号。

修法：把结算抽成 `KillSettler`，`main` 里创建**一个实例**同时注入
`ActionHandler.Kills` 与 `SpellHandler.Kills`。

★ 必须是同一个实例，不能各建一个 —— 两份实现迟早漂移
（一边加了归属保护、另一边忘了），而且玩家会遇到
「用法术打有掉落、用刀砍没有」这种诡异现象。

#### 2. `RollDrop` 传 nil 随机源会 panic

`World.CommitDeath` 走 actor 通道，拿不到调用方随机源，一直传 `nil`。
初版 `rollProb` 直接 `r.Intn(probRecip)` —— 怪物一旦配了掉落就是
`panic: runtime error: invalid memory address`，**整个 map actor 挂掉**。

之所以之前没暴露：法术击杀路径传了随机源，只有新接的物理路径才踩到。

修法：`rollProb` 容忍 `nil`，用包级随机源兜底（掉落不需要可复现性）。

#### 3. 尸体永不消失

原版 `monster.cpp:704`：

```cpp
if(getMR().deadFadeOut){
    addDelay(1000, [this](bool){ goGhost(); });   // 1 秒后消失
}else{
    defer([this]{ goGhost(); });                   // 立刻消失
}
```

之前 `deadFadeOut` 字段根本没提取，所有尸体永久躺在地上。

修法：
- 提取脚本加 `deadFadeOut`（364 条里 363 条为 1，1 条为 0）
- `Entity.DieAtMS` 记录死亡时刻（`Combat` 与 `Attack` 两处扣血点都记）
- `Advance` 里 `fadeOutCarcasses()` 按 `DeadFadeOut` 决定延时，到期 `RemoveEntity`
- tick 把 `corpse_fade` 转成 `SM_CODISAPPEAR`

★ 尸体清理必须在 AI **之前**：尸体还占着 `cell.Occupants`，
`CanOccupy` 返回 false，活怪走不进那一格 —— 表现为「怪绕着尸体走」。

★ 尸体消失只发 `SM_CODISAPPEAR`，不再发 `SM_DEADFADEOUT`：
后者在死亡瞬间已发过，1 秒后再发一次会让客户端重新播一遍淡出
（表现是尸体闪一下又淡一次）。

#### 4. 掉落没有归属保护

之前谁都能捡。多人打同一只怪时「谁手快谁得」——
法师放完火墙去捡的路上会被路过的战士全捡走。

修法：
- `GroundItem.Owner` / `ProtectUntilMS`（保护期 30 秒）
- `MapState.CanPickup(x, y, itemID, pickerUID)` 判权
- `MapState.DropItemsOwned` 落地时打标记
- `World.PickupOne` 从尾往前找**自己有权捡**的那一件

★ 拾取要跳过受保护物品，不能无脑取最后一件：
否则地上有别人保护期物品时，玩家点「捡」会反复失败 ——
表现是「地上有东西但怎么点都捡不起来」。

#### 5. 玩家/守卫打死怪也要结算

- 物理攻击打死 → `ActionHandler.settleKill`（本轮新增）
- 法术打死 → `SpellHandler.Cast`（已有，改为委托给同一 settler）
- **守卫打死怪** → `MapTicker` 里按 `AIAction.Dead` 调 `SettleNoKiller`

`SettleNoKiller`：只落地掉落（无归属保护），经验不给任何人。
不结算的话，「守卫帮你打死半兽人，东西凭空消失」——
玩家会以为被系统吞了。

**经验归属的已知简化**

原版 `dispatchOffenderExp` 按仇恨列表分摊（多人都打则各有份）。
当前版本简化成「最后一击的人独得」。多人围殴时会与原文有差异，
记为已知简化。

**验证**

| 层 | 结果 |
|---|---|
| `TestPhysicalKillSettlesLoot` | 经验 0→100、掉落 itemID=6 落地、归属保护生效、`SM_DEADFADEOUT` 已发 |
| `TestCorpseFadesOut` | 500ms 时尸体还在，1.1 秒后消失，产生 `corpse_fade` 事件 |
| `TestNoFadeOutKeepsCorpse` | `deadFadeOut=0` 的怪死后下一个 tick 就移除 |
| 服务端实跑 | 比奇县 1102 只怪 / 27 守卫 / 3 安全区，零 error |
| 客户端 c9c8 | **15 项全过** |

**本轮未做**

| 项目 | 影响 |
|---|---|
| 经验按仇恨分摊 | 多人围殴时只有最后一击得经验 |
| 金币入背包 | 金币（小）作为物品落地，还没接「拾取后加到 Gold 字段」 |
| 怪与怪阵营 | 守卫仍不打敌对怪物（`pickTarget` 只筛玩家） |
| 火墙击杀结算 | 火墙命中还没接 `SM_STRIKEGRID` |

### B5 第十批：聊天系统（本轮）

**补上 MMORPG 最基础的社交功能。** 之前服务端三条聊天消息全没注册 ——
客户端 `say()` 发出去，服务端不响应，表现是「按回车打字，字打出去了，谁也看不到」。

**三条消息的区别（很容易混）**

| 消息 | 语义 | 可见范围 | 入库 |
|---|---|---|---|
| `CM_PLAYERSAY` | 原地说话 | **视野内**（原版 `m_inViewCOList`） | ❌ |
| `CM_PLAYERBROADCAST` | 喇叭 | **全服** | ❌ |
| `CM_CHATMESSAGE` | 私聊 / 群聊 | 指定对端 / 群成员 | ✅ 有 msgId |

**编码方式也不同**（这是本轮最容易踩的坑）

```
CM_PLAYERSAY / CM_PLAYERBROADCAST   定长 128 字节（FixedCompressed）
CM_CHATMESSAGE                      变长：16 字节头 + 裸字符串正文
```

`CM_CHATMESSAGE` **不能走 wire 解码**：正文没有长度前缀（靠整包长度算），
按结构体字段读会越界或把正文前几个字节当成别的字段。必须手工切前 16 字节。

16 字节头的布局（`clientmsg.hpp:219-224`）：

```c
uint64_t toCPID;
uint64_t hasRef :  1;   // 与 refID 共用同一个 uint64
uint64_t refID  : 63;
```

GCC x86 位域从最低位开始 → `bit0 = hasRef`，`bit1..63 = refID`。

**关键实现决策**

1. **`CM_PLAYERSAY` 必须回给自己一份**
   客户端本地不显示自己说的话，全靠这条回包。
   不回的话玩家会以为「字没打出去」而反复重打。

2. **`CM_PLAYERSAY` 只发视野内，不是全图**
   隔了半张地图还听得见就不合理。用 `World.ObserversAt`（观察这一格的实体，
   天然过滤掉非玩家 —— `Players.Get(uid)` 查不到就跳过）。

3. **`SM_CHATMESSAGEOK` 即使发给别人也要回给自己**
   客户端靠它把「发送中」替换成「已送达」（带 msgId）。
   不回的话，自己发出去的气泡会一直转圈。

4. **入库失败不阻塞发送**
   聊天记录丢了不该影响即时通讯。先发再记日志。

5. **`truncateCString` 必须按 UTF-8 字符边界截断**
   中文一个字 3 字节，切在中间会产生非法 UTF-8，
   客户端解析时整条消息变乱码或直接丢弃。
   实测：189 字节的中文截到 126 字节（42 个字），合法。

**新增文件**

| 文件 | 行数 | 内容 |
|---|---|---|
| `internal/game/chat.go` | 300 | 三条消息的处理 + UTF-8 安全截断 |
| `internal/game/wire.go` 补 | +90 | 路由注册 + 16 字节头位域解析 |

**存储层复用**：`storage/chat.go` 早就写好（427 行，含私聊/群聊/好友/黑名单），
只需在 game 层接上。**没有新增存储代码**。

**验证**

| 测试 | 结果 |
|---|---|
| `TestSayBroadcastsToSelfAndViewers` | 自己收到 1 条 |
| `TestSayIgnoresEmptyContent` | 空白内容不广播 |
| `TestBroadcastSayReachesEveryone` | 三人各收到 1 条喇叭 |
| `TestChatMessageRepliesWithOK` | 发送者收 `CHATMESSAGEOK`、接收者收 `CHATMESSAGELIST` |
| `TestSplitChatMessageHeader` | 位域解析正确（hasRef/refID/toCPID） |
| `TestTruncateCStringUTF8Safe` | 189 字节 → 126 字节，无非法 UTF-8 |
| `TestE2ESayOverNetwork` | **真 TCP**：`say()` → 回显「有人吗」 |
| 客户端 c9c8 | 15 项全过 |

**本轮未做**

| 项目 | 影响 |
|---|---|
| 喇叭收费 + 限流 | 免费全服广播可能被用来刷屏。原版要花金币 |
| 好友 / 黑名单路由 | `storage` 已实现（`AddFriend`/`BlockPlayer`），路由未注册 |
| 群聊创建 | `CM_CREATECHATGROUP` 未注册 |
| 聊天记录查询 | `CM_QUERYCHATMESSAGE` 未注册（storage 有 `RetrieveLatest`） |

### B5 第十一批：经济闭环（装备持久 / 修理 / 金币入账）（本轮）

**目标：让金币有消耗出口，把经济系统从「只进不出」变成闭环。**

#### 1. 装备持久完全没实现（本轮核心）

之前 `Item` 结构只有 `ItemID/SeqID/Count` 三个字段 —— **没有持久度**。
后果是双重的：

| 后果 | 影响 |
|---|---|
| 装备永不磨损 | 金币没有消耗出口 → 通胀 → 商店/交易失去定价基准 |
| 装备不会损坏 | 玩家没有任何理由去 NPC 那里修理 |

`SDItem` 的持久语义有三个状态，**第三条最容易漏**（`sditem.hpp:205-212`）：

```
MaxDurability == 0                    物品没有持久概念（药品/材料）
MaxDurability >  0 && Durability > 0  正常使用中
MaxDurability >  0 && Durability == 0 **已损坏，不提供任何属性**
```

不判第三条的话，武器持久打光了照样能砍出满攻击 ——
玩家就没有理由修理，经济系统的支出端直接断掉。

**磨损触发（`player.cpp:1853` / `:2711`）**

| 场景 | 概率 | 部位 |
|---|---|---|
| 攻击一次 | 1/200（`SYS_WEAPONDURALOSSODDS`） | 武器 |
| 被击中一次 | 1/300（`SYS_ARMORDURALOSSODDS`） | 随机一件有持久的护甲 |

护甲是**随机挑一件**而不是全磨：全磨会让被围殴的玩家 5 分钟一身全碎。

**持久耗尽 = 永久销毁**

原版注释写得很明确：

```
// durability has been exhausted, this loss destroys the item permanently
// don't put it back into the inventory, it's gone
```

写成「留着但不能用」的话，玩家会以为还能修，背包里堆满尸体装备。

**修理（`player.cpp:2990`）**

| 类型 | 恢复 | 代价 |
|---|---|---|
| 普通修理 | 满持久 | **永久降低上限** `(max-curr)/10` |
| 特殊修理 | 满持久 | 不损失上限，但要更多钱 |

普通修理的余数按概率多损 1 点 —— 否则玩家可以频繁小修来回避上限损失。
上限最低保留 1，不然后续永远无法再修。

实测：持久 20/100 普通修理 → **92/92**（损失 8 点，`80/10=8`）。

**修掉一个连带缺陷：装备转换丢了 Duration**

`toSDItem` / `fromSDItem` 只搬运 `ItemID/SeqID/Count`，`Duration` 被静默丢弃。
后果：装备每次「穿上/脱下/重登」都恢复满持久 ——
磨损系统等于不存在，而且**不会有任何报错**。

#### 2. 金币拾取进 Gold，不占背包

之前金币作为一件「物品」落在地上，捡起来占背包格子 —— 打怪掉的金币**没法花**。

原版逻辑（`inventorydb.cpp:116`）：

```
if(item.isGold()){ gold += item.count; }   // 不进 inventory
else             { inventory.add(item); }
```

金币在物品表里是 5 条「金币（小/中/大/特/超）」记录，
但它们不是可持有的物品，只是数值载体。

★ 金币溢出改为**按上限截断**而不是原版的「整批拒绝」：
拒绝会让「背包 100 枚金币 + 上限只剩 50 空间」时玩家一个都捡不到，
截断至少能捡 50。

**已知简化**：地面物品结构（`SDGridItemIDList`）只有 `itemID` 没有 `count`，
所以大额金币掉落（原版可能给 `count=1600`）会被压成 1 枚。
要修需要把 count 一路带到地面物品结构，留待后续。

#### 3. 修掉一个重登数据丢失缺陷

`WorldService.Enter` 只建了个空的 `Wear` map，**没有从库里加载背包/装备/腰带**。

后果：重登后所有物品消失，而且不报错（客户端看到的就是空背包）。

修法：`loadPlayerItems` 在算属性**之前**读回三样东西，且每件都带持久值。
顺序很重要 —— `WorldEntity` 的 Stats 读 `p.Wear`，先算属性再加载装备
会让玩家进图时是「裸装属性」，要等下次换装才恢复。

**新增文件**

| 文件 | 行数 | 内容 |
|---|---|---|
| `internal/game/durability.go` | 340 | 磨损 / 销毁 / 修理 / 金币入账 |
| `internal/game/player.go` 补 | +25 | `Item.Durability` / `MaxDurability` / `Broken()` |
| `internal/game/action.go` 补 | +60 | 磨损触发、金币分拣、初始持久 |
| `internal/game/world_service.go` 补 | +55 | `loadPlayerItems` |

**验证**

| 测试 | 结果 |
|---|---|
| `TestWeaponLosesDurability` | 铁剑 10 → 9，推 `SM_WEARITEMDURATION` |
| `TestItemDestroyedWhenDurabilityExhausted` | 持久 0 再触发 → 装备消失，推 `SM_EQUIPWEAR` |
| `TestBrokenItemGivesNoStats` | DC.Max 完好 10 → **损坏 1**（不再提供属性） |
| `TestRepairRestoresAndLowersMax` | 20/100 → **92/92**（损 8 上限） |
| `TestSpecialRepairKeepsMax` | 20/100 → 100/100（上限不变） |
| `TestGoldGoesToWalletNotInventory` | Gold=500，背包 0 格，推 `SM_GOLD` |
| `TestGoldCappedAtMax` | 只剩 10 空间时只加 10 枚 |
| 服务端实跑 | `player items loaded dbid=1 inventory=0 wear=0` |
| 客户端 c9c8 | **15 项全过** |

**测试写法**

概率性断言需要「恒返回 0」的随机源。**不能用 `rand.New(rand.NewSource(0))`** ——
Go 的 math/rand 即使种子固定，`Intn(n)` 也不保证第一次返回 0，
测试会随机失败。`DurabilityHandler.Rand` 因此改成接口 `RandSource`。

**本轮未做**

| 项目 | 影响 |
|---|---|
| NPC 修理路由 | `RepairItem` 已实现，`uidRepairItem` 绑定还是空桩 |
| 修理费用计算 | 原版按物品价格与损失持久算，现在没有扣钱 |
| 金币面额换算 | 掉 `count=1600` 的金币只入账 1 枚 |
| 持久落库的完整链路 | 穿戴/脱下路径的 `Duration` 已修，但 NPC 脚本改物品属性时未覆盖 |
