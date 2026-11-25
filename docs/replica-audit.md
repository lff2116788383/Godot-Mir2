# Godot-Mir2 客户端复刻进度审计（P0/P1 更新版）

> 审计日期：2026-11-25
> 原版基线：`E:\Desktop\game\mir2x`，提交 `5826493`
> Godot 基线：`C:\Users\Administrator\Desktop\github\Godot-Mir2`，分支 `client-godot`
> 本文以当前代码实测和协议表为准，旧版939 项全过 / SM 33/86等结论已不再使用。

---

## 一、总体结论

| 维度 | 当前进度 | 说明 |
|---|---:|---|
| P0 核心链路 | **约 95%** | 登录、查角色、进图、服务端场景回放、地图 UID、动作上行、世界点击、魔法、物品基础链路已打通 |
| P1 系统接线 | **约 90%** | 商店、交易、摆摊、拍卖、仓库、队伍、行会、好友、任务、小地图、安全区、火墙、BGM 已接基础协议和 UI |
| P2 精细化 | **约 55%** | 已补怪物攻击魔法、standMode 同步、多段攻击、多阶段魔法、排行榜、悬停、Tooltip、Emoji、F3 调试层和 GM 命令 |
| 完整客户端复刻 | **约 65%70%** | P2 特殊怪物、像素级 UI、IME、调试工具、边缘玩法仍需继续 |
| 真实生产服务端验收 | **约 20%30%** | 当前只在 Go mock 上做了链路验证，尚需真实 C++ 服务端/正式服务端回归 |

### 本轮最重要的变化

1. **协议码修复**：建账号、改密码、在线错误、静态缓冲区偏移等错误已修正。
2. **新增 `WorldSession`**：把场景进入、实体交互、聊天、队伍、行会、交易、商店、任务等状态集中接线。
3. **CM 封装补齐**：`MirClient` 现有 **86 个客户端消息封装**。
4. **SM 分支覆盖补齐**：原版 `ProcessRun` 的 **86 个 SM 处理分支**均已有对应入口；登录流程 14 个 SM 分支也已修正。
5. **服务端场景回放**：登录后收到 `SM_ONLINEOK` / `SM_STARTGAMESCENE` 后按 `mapUID` 加载地图，并回放场景实体。
6. **移动/攻击/拾取/施法开始上行**：不再只是本地动画。
7. **新增地图表、魔法特效、头顶名字/HP、安全区/火墙绘制、小地图和 BGM 切换**。
8. **修复 UI 子控件事件 ROI 传递错误**，面板点击坐标开始正确。

---

## 二、当前代码与数据规模

| 项目 | 原版客户端 | 当前 Godot | 说明 |
|---|---:|---:|---|
| 代码文件 | 407 个 cpp/hpp/inc | 97 个运行时 GDScript | 当前另有 tools 测试脚本 |
| 代码行数 | 59,409 | 25,639 | 约 43%（含数据/布局/协议封装） |
| 核心数据表 | item / monster / map / magic / buff | 1258 / 364 / 603 / 116 / 21 | 核心表全部转为 JSON |
| 派生数据 | 怪物类、技能图标 | 32 类行为、12 类自定义帧表、63 技能图标 | 特殊怪物仍需继续逐类验收 |
| CM 封装 | 86 个业务码 | 86 个 | 见 `scripts/net/mir_client.gd` |
| ProcessRun SM 分支 | 86 个 | 86 个有处理入口 | 核心状态链路已覆盖，边缘表现仍需真实包回归 |
| 登录流程 SM 分支 | 14 个 | 14 个 | 枚举值已改为引用 `MirClient` 常量 |
| 主面板 | GUIManager 21 个 | 21 个均有布局/实现入口 | 部分面板仍是简化实现 |

---

## 三、P0 完成情况

| 功能 | 状态 | 实现位置 | 说明 |
|---|---|---|---|
| 协议常量纠错 |  | `login_flow.gd`、`mir_client.gd` | 建账号 32、改密码 6、在线错误 22 等已修正 |
| 静态缓冲区布局 |  | `mir_client.gd`、`ui_account_process.gd`、mock | StaticBuffer<64> 按 68 字节布局 |
| CM 上行封装 |  | `mir_client.gd` | 86 个 CM 封装齐全 |
| SM 解析扩展 |  | `sd_msg_parser.gd` | 在线、场景、安全区、魔法、队伍、行会、好友、拍卖、摆摊、任务等 |
| 登录查角色进游戏 |  | `login_flow.gd`、`main.gd` | mock 实测 18/18 通过 |
| 服务端场景进入 |  | `WorldSession.apply_start_scene`、`main.apply_server_scene` | 按 mapUID、坐标、朝向进入；支持场景帧缓存回放 |
| 地图表/多地图加载 |  | `map_record.gd`、`main.gd` | 支持 mapIDbin、BGM、小地图 ID、地图缓存 |
| 主角移动上行 |  | `player_actor.gd`、`world_session.gd` | 本地 A* + `CM_ACTION` |
| 怪物/玩家点击攻击 |  | `world_session.gd` | 邻格攻击，远距离走向后攻击 |
| 拾取 |  | `world_session.gd` | 点击地面物品，接近后发 `CM_PICKUP` + 动作 |
| NPC 对话入口 |  | `world_session.gd`、`ui_npc_chat.gd` | 点击 NPC 发 `SYS_ENTER`，XML 显示与事件回发已接 |
| 魔法特效与伤害请求 |  | `world_session.gd`、`magic_effect_actor.gd` | `SM_CASTMAGIC` 播放 APNG；技能点击后点怪施法 |
| 背包/装备/腰带更新 |  | `main.gd`、`sd_msg_parser.gd` | 更新、删除、装备、脱装、腰带消息已处理 |
| 背包双击操作 |  | `game_ui.gd`、`world_session.gd` | 喝药、穿戴、放腰带；拍卖/摆摊窗口下转为登记/上架 |
| 装备槽点击脱装 |  | `ui_hero_state_board.gd`、`world_session.gd` | 点击装备格发 `CM_REQUESTGRABWEAR` |
| 世界坐标/地图同步 |  | `co_manager.gd`、`world_session.gd` | 主角换图触发地图重载；其他实体换图移除 |
| 连接帧缓存 |  | `main.gd` | 登录期间缓存 STARTGAMESCENE/CORECORD/ACTION，进图后回放 |

---

## 四、P1 完成情况

### 4.1 商店、交易、摆摊、拍卖、仓库

| 功能 | 状态 | 说明 |
|---|---|---|
| NPC 商品列表 |  | `SM_NPCSELL` / `SM_SELLITEMLIST`  PurchaseBoard |
| 购买 |  | PurchaseBoard on_buy  `CM_BUY` |
| 直接交易请求/接受/拒绝 |  | 输入框确认，响应 `CM_RESPONDDIRECTTRADE` |
| 交易锁定/确认/完成/关闭 |  | 交易报价更新、锁定、提交、取消、完成/错误消息 |
| 摆摊查询/浏览/购买 |  | `CM_QUERYSTALL` / `SM_QUERYSTALLOK` / `CM_BUYSTALLITEM` |
| 摆摊上架/取下/开始/收摊 |  | 双击背包上架；物品操作按钮取下；开始/收摊已接 |
| 拍卖查询/购买/撤拍 |  | AuctionBoard + `CM_QUERYAUCTIONITEMLIST` / `CM_BUYAUCTIONITEM` / `CM_UNREGISTERAUCTIONITEM` |
| 拍卖登记 |  | 拍卖面板打开时双击背包物品登记，默认价格取物品价格，备注玩家寄售 |
| 仓库/寄存列表 |  | `SM_SHOWSECUREDITEMLIST`，打开背包和寄存面板 |
| 仓库取回 |  | 通用物品列表按钮  `CM_REQUESTRETRIEVESECUREDITEM` |
| 装备耐久 |  | `SM_WEARITEMDURATION` 更新装备耐久并刷新人物状态面板 |
| 装备/腰带错误提示 |  | 四类错误消息已转系统日志 |

### 4.2 队伍、行会、好友、聊天

| 功能 | 状态 | 说明 |
|---|---|---|
| 队伍成员/申请/附近队伍 |  | `SM_TEAMMEMBERLIST` / `SM_TEAMCANDIDATE` / `SM_TEAMINFO` 绑定 TeamBoard |
| 加入/离开/查询/转让队长/跟随 |  | UI/命令及 CM 封装已接 |
| 创建队伍/设置要求 |  | `CM_CREATETEAM` / `CM_SETTEAMREQUIREMENT` 已封装 |
| 行会公告/成员/聊天 |  | `SM_GUILDINFO` / `SM_GUILDCHAT` 绑定 GuildBoard |
| 行会邀请/退出/踢人/转让/解散/公告 |  | 命令与 CM 封装已接 |
| 行会战列表 |  | `SM_GUILDWARLIST` 状态保存 |
| 好友列表/添加/接受/拒绝/黑名单 |  | FriendChatBoard 和 CM 封装已接 |
| 好友搜索/会话预览/私聊 |  | 搜索、打开会话、消息发送/接收解析已接 |
| 本地说话/广播/行会聊天 |  | 控制栏命令 `普通 / !广播 / @行会` |
| 聊天历史/送达回执 |  | `SM_CHATMESSAGELIST`、`SM_QUERYCHATMESSAGEOK`、`SM_CHATMESSAGEOK` |

### 4.3 任务、NPC、显示、小地图、音频

| 功能 | 状态 | 说明 |
|---|---|---|
| 任务列表/更新 |  | `SM_QUESTDESPLIST` / `SM_QUESTDESPUPDATE`  QuestBoard |
| NPC XML 对话 |  | `SDNPCChat` 字符串布局已修正为 uid + eventPath + XML |
| NPC 提交输入 |  | `SM_STARTINPUT`  输入框  `CM_NPCEVENT(commitTag)` |
| 小地图 |  | 地图 miniMapID、角色格坐标、拖动/缩放面板 |
| BGM |  | 登录、建号、进图地图 BGM |
| 头顶名字/HP |  | 人形与怪物节点增加名字和 HP 百分比标签 |
| 安全区绘制 |  | 矩形/圆形边界和浅色区域 |
| 火墙绘制 |  | 地面火焰格 |
| 设置面板 |  | 音量设置本地生效；配置项值保存 |
| 技能面板 |  | 已学魔法列表  技能图标；选中后点怪施法 |
| 快捷栏 |  | 数字键 16 使用腰带物品并发 `CM_REQUESTCONSUMEBELT` |
| 系统配置/排行消息 |  | `SM_PLAYERCONFIG`、`SM_RANKINGLIST` 已解析/存储 |

---

## 五、渲染与表现完成度

| 模块 | 现状 |
|---|---|
| 地图瓦片 | 视口裁剪、瓦片池化、单图加载、地图缓存 |
| 地图物件 | GROUND / OVERGROUND0 / OVERGROUND1、地面物品、动画物件 |
| 光照 | 简单圆形色光；尚未达到原版完整暗化/光照混合 |
| 人物 | 七层结构核心层已接：武器后层、影子、身体0/1、头/头盔、武器前层 |
| 怪物 | APNG 帧表、方向、动作队列、32 类行为数据、12 类自定义帧表 |
| 魔法 | APNG 特效节点，按 MagicDB gfxID 查找素材 |
| 战斗表现 | MISS/伤害/治疗飘字、受击闪烁、死亡淡出 |
| 排序 | 主角与场景实体使用 y_sort，地图物件按层分 z |
| 特效层 | 安全区、火墙、魔法、飘字已分层 |

---

## 六、仍属于 P2 / 后续事项

1. **32 类怪物特殊状态机逐类验收**：食人花变身、神兽、触龙神、祖玛教主等特殊逻辑。
2. **魔法系统深度还原**：三类魔法飞行/旋转/命中/附着细节与真实服务端包逐字段回归。
3. **UI 像素级复刻**：部分面板仍是简化布局，需要逐面板与原版截图比对。
4. **IME 候选框、完整表情包、字体间距**：原生输入已可用，逐字候选框和完整表情包仍需素材整理。
5. **GfxDebugBoard、GM 命令、客户端 Lua 调试**。
6. **完整排行榜 UI**：消息已解析，独立排行面板尚未从 RuntimeConfigBoard 拆出。
7. **真实 C++ 服务端联调**：当前主要验证渠道是 `tools/mock-server`。
8. **正式导出/资源打包流程**。
9. **特殊地图切换点、随机传送、地图预加载策略**：当前按服务端地图 UID 即时加载并缓存。

---

## 七、本轮测试结果（集中测试）

### 直接自检

12 个 SceneTree 自检全部通过：

| 测试 | 结果 |
|---|---:|
| `test_data.gd` | 51 / 0 |
| `test_path.gd` | 25 / 0 |
| `test_human.gd` | 31 / 0 |
| `test_monster.gd` | 28 / 0 |
| `test_typeset.gd` | 22 / 0 |
| `test_ui_framework.gd` | 60 / 0 |
| `test_ui.gd` | 39 / 0 |
| `test_c3_c4.gd` | 42 / 0 |
| `test_c5_c6.gd` | 45 / 0 |
| `test_c7_c8.gd` | 235 / 0 |
| `test_c9.gd` | 205 / 0 |
| `test_c10.gd` | 54 / 0 |
| **合计** | **837 / 0** |

### 真实 TCP mock 联调

`c9c8` 场景通过 **18/18**：

- 登录
- 查询角色
- 职业/性别/动画帧
- 在线进图
- `WorldSession` 创建
- `SM_STARTGAMESCENE` 回放
- 服务端场景实体回放
- 主角 UID 同步

### mock 协议修正

本轮同时修正 mock 的：

1. `SM_QUERYCHAROK` 68/69/70 字段偏移。
2. `SM_STARTGAMESCENE` 的 cereal 尾压缩标志位置。
3. 登录静态缓冲区偏移。

---

## 八、P2 本轮已推进

| 功能 | 状态 | 说明 |
|---|---|---|
| 怪物攻击动作差异化 |  | `attack_motion` 数据驱动 ATTACK0/ATTACK1 |
| 怪物攻击魔法 |  | 已覆盖暗黑战士、掷斧骷髅、楔蛾、雷电/僧侣僵尸、蚂蚁道士、红衣法师、祖玛弓箭手、沙漠风魔、爆毒蚂蚁、诺玛法老、沙漠树魔、霸王教主、神兽、祖玛教主等；动态 magicID 优先 |
| 怪物 standMode |  | SPAWN 重置、STAND/TRANSF 根据 standModeReq 同步、攻击前自动变身 |
| 怪物多段攻击 |  | `attack_segments` 数据驱动；攻击附带 magicID、自身/目标施法位置也已数据化 |
| 怪物死亡魔法 |  | 爆裂蜘蛛、红衣法师、沙漠石人、祖玛教主等死亡特效已接 |
| 怪物出生魔法 |  | 僧侣僵尸地洞、沙漠石人石坑等 SPAWN 动作帧特效 |
| 头顶信息增强 |  | 名字颜色、行会/职位、队长标记与 HP 百分比 |
| 邻接地图预加载 |  | 按地图 switchList 后台加载邻接地图，进入时优先命中缓存 |
| 导出骨架 |  | Windows Desktop 导出预设、`tools/export_client.ps1`、外置资源启动脚本 |
| 多阶段魔法 |  | `启动/运行` 两阶段、`固定/跟随/附着` 三种图元、飞行插值和朝向旋转 |
| Emoji |  | 支持 `texture/emoji` 外部图集；缺图使用 `emojidef.png` 兜底 |
| 直接交易放物品 |  | 交易窗口打开时双击背包装备/物品加入本方报价并同步 |
| 世界鼠标悬停 |  | 显示格坐标、实体名字、目标框、地面物品名 |
| 背包物品 Tooltip |  | 悬停显示名称、类型、耐久、描述 |
| 排行榜面板 |  | 新增 `UiRankingBoard`；`/ranking [type]` 查询 |
| F3 调试层 |  | 网格、实体 UID/碰撞框、主角格坐标、FPS |
| GM 调试命令 |  | `/debug`、`/tp x y`、`/querygold`、`/querymap` |
| 队伍/行会命令 |  | `/team ...`、`/guild ...`、`/friend add ...` |
| 头顶名字/HP |  | 人形/怪物通用标签 |
| 安全区/火墙 |  | 世界层绘制 |

### P2 仍未完成

1. 32 个怪物专属类逐类行为回归（已覆盖主要攻击/死亡魔法；召唤、特殊运动和少量静态帧差异仍需继续）。
2. 魔法飞行旋转、命中点、尾迹、APNG 帧延迟的逐帧精修。
3. 全 21 面板的像素级布局、按钮状态、悬浮态和原版贴图对齐。
4. 完整 IMEBoard、表情图集、字体间距和中文输入候选框。
5. 自动寻路失败后的绕远策略、地图预加载优先级。
6. 真实服务端多地图、跨服、长时间在线稳定性回归。
7. 资源打包和安装包自动化：导出骨架已添加，但 1.34GB 外置资源仍依赖 `MIR2X_ASSETS`，尚未打增量包。

## 九、结论

本轮已经把原版客户端最关键、最容易被 UI 空壳掩盖的链路补上：

- 协议不再大面积抄错；
- 登录后进入的是服务端指定地图和坐标；
- 场景帧不会在登录切进程时丢失；
- 玩家动作开始上行；
- 面板开始接真实数据和真实 CM；
- 队伍、行会、好友、商店、交易、摆摊、拍卖、仓库、任务、小地图、音频均有基础闭环。

当前项目已经超过只能看不能玩的静态复刻阶段，进入可在 mock/真实协议服务端上继续逐系统联调的阶段。

下一阶段应集中在 **P2 特殊怪物行为、真实服务端回归和 UI 像素级精修**，而不是继续无边界新增空壳面板。
