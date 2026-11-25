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
