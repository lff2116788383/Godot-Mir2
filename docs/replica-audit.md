# 客户端复刻进度审计（对照 mir2x client/src）

> 审计基准：`E:\Desktop\game\mir2x` 提交 `5826493`，`client/src/` 共 **268 个文件、33,902 行**
> 本工程：`client-godot/scripts/` 共 **95 个文件、22,600+ 行**
> 结论日期：2026-11-24

---

## 一、总体进度

| 维度 | 原版 | 已复刻 | 完成度 |
|---|---|---|---|
| 代码行数 | 33,902 | 22,600 | **约 67%** |
| 文件数 | 268 | 95 | 35%（但原版 34 个怪物子类已数据化合并） |
| GUI 面板目录 | 16 个 | 16 个全实现 | **100%** |
| SM 协议处理器 | 86 个 | 33 个 | **38%** |
| 自检项 | — | 939 项 | 全过 |

**行数 67% 但协议 38%，说明"界面/数据"做得比"网络同步"多。**
这是本阶段最需要补的短板。

---

## 二、已完成（按批次）

### B 系列：底层地基（全完成）
| 批次 | 内容 | 与原版对应 |
|---|---|---|
| B1 | 协议枚举、常量、ActionNode、UID、物品结构 | `protocoldef.hpp` `sysconst.hpp` `actionnode` |
| B2 | 5 张 .inc → JSON（1258 物品/364 怪物/603 地图/116 魔法/21 Buff） | `common/src/*.inc` |
| B3 | 动作状态机（33 种动作帧表、frameDelay、移动队列、parseAction） | `creaturemovable.cpp` `motionnode` |
| B4 | 魔法系统（基类/追踪/固定/附着/图形库） | `basemagic.cpp` `magicdb` |
| B5 | 物品判定 + 布局算法 | `itemrecord.hpp` `invpack.cpp` |
| B6 | 怪物帧表提取 + 基类 | 12 个类自定义帧表 |
| B7 | 背包容器、消息栈、小地图 | `invpack.cpp` `messagestackboard` `minimapboard` |
| B8 | XML 排版引擎 | `xmltypeset.cpp`(1730 行) `xmlparagraph.cpp` |
| B9 | 人形七层渲染 | `hero.cpp:1411-1460` gfxID 计算 |
| B10 | 动作播放 + mock 联调 | `processrun.cpp` 动作调度 |
| B11 | 实体同步、怪物 APNG 渲染 | `clientmonster.cpp` `pngtexoffdb` |
| B12 | 怪物走路与像素位移（getShift） | `creaturemovable.cpp:216` |

### C 系列：界面与流程（全完成）
| 批次 | 内容 | 覆盖率 |
|---|---|---|
| C1+C2 | UI 框架（10 文件）+ 主界面 | 控件树/ROI 裁剪/按钮三态/滑块/输入 |
| C3+C4 | XML 解析 + NPC 对话 + 战斗飘字 | `xmlparagraph` `npcchatboard` `ascendstr` |
| C5+C6 | 5 个面板 + 32 怪物类行为数据化 | `skillboard` `teamstateboard` `guildboard` `runtimeconfigboard` `itemlistboard` |
| C7 | 14 个低频面板逐行移植 | 控制栏/状态板/快捷栏/头顶/脚下/摊位列/商店/交易/摆摊/拍卖/好友/任务/马匹/小地图/输入框/菜单 |
| C8 | 音频系统 | `soundeffectdb` `bgmusicdb` 848 音效 + 25 BGM + 3D 定位 |
| C9 | 登录 → 选人 → 建号 | `processlogo` `processlogin` `processselectchar` `processcreatechar` `processcreateaccount` `processchangepassword` |

### C10：本轮新增（补的最大缺口）
| 内容 | 对应原版 | 状态 |
|---|---|---|
| **地图物件层**（地面物件/上层物件0/上层物件1/光照） | `processrun.cpp:2388 drawObject` + `295-425` 分层 | **新增** |
| MapLoader 扩展（light 位域、2×OBJECT 7×u32） | `mir2xmapdata.hpp` CELL 结构 | **新增** |
| 角色状态消息解析（11 类） | `processrunnet.cpp` 各 on_SM_* | **新增** |
| SM 协议常量补全（73 个） | `sm_table.gd` 逐值核对 | **新增** |
| 地面物品同步与绘制 | `processrunnet.cpp:466` + `drawGroundItem` | **新增** |
| 聊天/广播/打击格/NPC 消息 | `processrunnet.cpp:105-126, 738-790` | **新增** |

---

## 三、还缺什么（按影响排序）

### 缺口 1：地图渲染仍有 3 层没做 ⚠️ 高

原版绘制顺序 8 层，现状：

| 层 | 原版 | 现状 |
|---|---|---|
| 1. 瓦片 | `drawTile` | ✅ 已做（Sprite2D 池化） |
| 2. 地面物件 OBJD_GROUND | `drawObject(OBJD_GROUND)` | ✅ **本轮新增** |
| 3. 地面魔法特效（火焰灰烬） | `FireAshEffect::drawGroundAsh` | ❌ 缺 |
| 4. 地面掉落物 | `drawGroundItem` | ✅ **本轮新增** |
| 5. 角色（按 y 排序） | `coLocList` 分组绘制 | ⚠️ 有，但**没有按 y 排序** |
| 6. 上层物件 OBJD_OVERGROUND0 | `drawObject(OVERGROUND0)` | ✅ **本轮新增** |
| 7. 火墙 | `fireWallList` | ❌ 缺 |
| 8. 上层物件 OBJD_OVERGROUND1 | `drawObject(OVERGROUND1)` | ✅ 已接入（本图 0 个） |

**还差的**：
- 角色按 y 排序（否则人会被后面的树挡住 / 挡住前面的树）
- 火墙绘制（`SM_GROUNDFIREWALLLIST` 已解析但没画）
- 地面魔法特效层（火焰灰烬）

### 缺口 2：SM 协议还有 53 个没接 ⚠️ 高

已接 33 / 86。按系统分：

| 系统 | 未接协议 | 影响 |
|---|---|---|
| **魔法/战斗** | `SM_CASTMAGIC`(已定义未处理) `SM_NEXTSTRIKE` `SM_STRIKEGRID`(已接) `SM_MISS` | 施法表现、连击 |
| **安全区** | `SM_SAFEZONELIST` | 安全区边缘显示 |
| **队伍** | `SM_TEAMINFO` `SM_TEAMMEMBERLIST` `SM_TEAMCANDIDATE` `SM_TEAMERROR` | 队伍面板无数据 |
| **行会** | `SM_GUILDINFO` `SM_GUILDERROR` `SM_GUILDCHAT` `SM_GUILDWARLIST` `SM_PLAYERGUILD` | 行会面板无数据 |
| **好友** | `SM_FRIENDLIST` `SM_ADDFRIENDACCEPTED/REJECTED` `SM_CHATMESSAGELIST` `SM_CREATECHATGROUP` | 好友面板无数据 |
| **交易** | `SM_STARTDIRECTTRADE` `SM_UPDATEDIRECTTRADE` `SM_CLOSEDIRECTTRADE` `SM_COMPLETEDIRECTTRADE` `SM_DIRECTTRADEERROR` | 交易面板空壳 |
| **拍卖** | `SM_AUCTIONITEMLIST` | 拍卖面板空壳 |
| **摆摊** | `SM_STALL` `SM_STALLERROR` `SM_PLAYERSTALL` `SM_QUERYSTALLOK` | 摆摊面板空壳 |
| **商店** | `SM_NPCSELL` `SM_SELLITEMLIST` `SM_BUYSUCCEED` `SM_BUYERROR` | 购买流程断 |
| **任务** | `SM_QUESTDESPLIST`(已解析) `SM_QUESTDESPUPDATE` | 任务面板无数据 |
| **拍卖/排行** | `SM_RANKINGLIST` | 排行榜空壳 |
| **其他** | `SM_REVIVE` `SM_OFFLINE` `SM_PARTEXT` `SM_PLAYERCONFIG` `SM_UPDATEITEM` `SM_REMOVEITEM` `SM_INVOPCOST` `SM_STARTINVOP` `SM_EQUIPWEARERROR` `SM_GRABWEARERROR` | 零散 |

### 缺口 3：processrun.cpp 的"世界交互" ⚠️ 中

| 功能 | 原版 | 现状 |
|---|---|---|
| 点击 NPC 对话 | `processEvent` 里判 `UID_NPC` | ⚠️ 有 NPC 面板，但没有点击触发 |
| 点击地面物品拾取 | `tryPickUpItem` | ❌ 缺 |
| 鼠标悬停显示物品名 | `drawItemHoverText` | ❌ 缺 |
| 鼠标位置显示坐标 | `drawMouseLocation` | ❌ 缺 |
| 攻击目标锁定（红框） | `getTargetBox` | ❌ 缺 |
| 安全区边缘虚线 | `resetSafeZoneEdge` | ❌ 缺 |
| 近身攻击判定 | `oneStepCost` `canMove` | ⚠️ 部分 |
| 用户命令 `@` | `userCommand` `luaCommand` | ❌ 缺 |

### 缺口 4：hero.cpp 的战斗逻辑 ⚠️ 中

| 功能 | 原版行 | 现状 |
|---|---|---|
| `hasSwingMagic` 挥砍带魔法 | 有 | ❌ 缺 |
| `toggleSwingMagic` | 有 | ❌ 缺 |
| 攻击目标选择 `weaponOrder` | 有 | ❌ 缺 |
| 死亡淡出 `deadFadeOut` | 有 | ✅ 有（`fight_effects`） |
| 头顶板 `getOverHeadLoc` | 有 | ✅ 有（`ui_over_head`） |
| 名字查询 `queryName` | 有 | ❌ 缺 |

### 缺口 5：怪物子类独有逻辑（数据化遗漏）⚠️ 低

34 个子类里 20 个有 cpp。我做了行为数据化，但有 1 个不可数据化的：
- `ClientCannibalPlant::onActionTransf` —— **变身动作**（食人花变形态），
  这不是帧表差异，是独立的状态机分支。当前未实现。

其余 19 个确认是纯帧表/帧数差异，已数据化。

### 缺口 6：其他系统性缺失 ⚠️ 低-中

| 项 | 原版 | 现状 |
|---|---|---|
| `gfxdebugboard.cpp`(713 行) | 调试面板（F3 显示碰撞/坐标/物件层） | ❌ 缺（开发辅助，非游戏功能） |
| `ime.cpp` `imeboard.cpp`(697 行) | 中文输入法候选窗 | ⚠️ 用 Godot 原生 IME 顶替 |
| `processconnectionerror` | 断线提示界面 | ⚠️ 只有日志 |
| `processchangepasswordnet` | 改密网络交互 | ✅ 已接（在 account 面板） |
| 多地图切换 | `loadMap` `preloadMapBin` | ❌ 只有单图 |
| 地图预加载 | `preloadMapBin` | ❌ 缺 |

---

## 四、优先级建议

| 优先级 | 内容 | 理由 |
|---|---|---|
| **P0** | 角色按 y 排序绘制 | 一层排序就能让"人走在树前后"正确，改动小收益大 |
| **P0** | `SM_CASTMAGIC` 施法表现 | 战斗手感核心，且协议已定义 |
| **P1** | 队伍/行会/好友 12 个协议 | 三个面板已做好，只差数据接入 |
| **P1** | 交易/拍卖/摆摊/商店 12 个协议 | 同上，面板全是空壳 |
| **P2** | 点击 NPC / 拾取 / 悬停提示 | 让世界"可交互" |
| **P2** | 火墙 + 地面魔法特效层 | 视觉完整性 |
| **P3** | 多地图切换 + 预加载 | 需要服务端配合 |
| **P3** | 食人花变身、GfxDebug 面板 | 边缘功能 |

---

## 五、一句话总结

**界面层（GUI 16/16、登录链路、音频）已完整复刻；**
**网络同步层只做了 38%，这是当前最主要的不完整之处。**
本轮补上了地图物件层与 15 类状态消息，把"能看见世界"做到了位，
但"世界会响应玩家操作"（交易/队伍/战斗交互）还差一大截。