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
| 玩家 | 7,837 | ❌ 未做（`internal/game` 还是空包） | **0%** |
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
| "完整游戏可玩" | ❌ 不能（无玩家系统，进图后世界是空的） |

**综合完成度约 55%，"可用性"约 40%**（上一轮是 50% / 20%）。

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