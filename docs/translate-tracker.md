# 翻译进度跟踪

> 分层规则见 `docs/translation-layers.md`
> 状态：⬜未开始 / 🟨进行中 / ✅完成 / ⚠️有问题

## 总览

| 批次 | 内容 | 层 | 状态 | 目标文件 | 行数(源) |
|---|---|---|---|---|---|
| B1 | 协议 + 方向 + 动作节点 + UID + 结构 | L0 | ✅ | ~15 | ~2400 |
| B2 | 数据表（5 个 .inc → JSON） | L0 | ✅ | 5 + 生成器 | 690KB |
| B3 | 动作状态机（hero 动作部分） | L0 | ✅ | ~4 | ~2000 |
| B4 | 魔法系统 | L0 | ✅ | ~8 | ~1400 |
| B5 | 物品逻辑 | L0 | ✅ | ~5 | ~1200 |
| B6 | 怪物帧表 + 基类逻辑 | L0 | ✅ | 40 | ~3000 |
| B7 | 界面逻辑（背包/聊天/小地图） | L4 | 🟨 | 28 | ~9000 |
| B8 | XML 排版引擎 | L0 | 🟨 | 6 | ~3450 |
| B9 | 渲染与输入（Godot 重写） | L2/L3 | 🟨 | 按需 | ~8000 |
| B11 | 实体同步与怪物渲染 | L0+L2 | ✅ | 6 | ~1600 |
| B10 | 统一联调（mock 服务端） | — | ✅ | — | — |

## B1 明细

| 源文件 | 目标文件 | 状态 | 备注 |
|---|---|---|---|
| `common/src/pathf.hpp`<br>`common/src/pathf.cpp` | `scripts/action/path_finder.gd` | ✅ | 双向 A*，势函数 reduced cost 完整翻译 |
| `common/src/mir2xmapdata.hpp` | `scripts/data/map_loader.gd` | ✅ | .bin 解析，8 张地图验证 |
| `common/src/msgf.hpp`<br>`common/src/zcompf.cpp` | `scripts/net/mir_framer.gd` | ✅ | 帧编解码，LEB128 + XOR |
| `common/src/clientmsg.hpp` | `scripts/net/proto_tables.gd` | ✅ | 87 条 CM（脚本生成） |
| `common/src/servermsg.hpp` | `scripts/net/proto_tables.gd` | ✅ | 115 条 SM（脚本生成） |
| `common/src/protocoldef.hpp` | `scripts/core/protocol_def.gd` | ✅ | 枚举全集，632 行全翻 |
| `common/src/actionnode.hpp` | `scripts/action/action_node.gd`<br>`scripts/action/action_type.gd` | ✅ | 15 种 Action + 28B 编解码 |
| `common/src/motion.hpp` | `scripts/action/motion_type.gd` | ✅ | 46 种 Motion |
| `common/src/uidf.cpp`<br>`common/src/uidf.hpp` | `scripts/core/uid.gd` | ✅ | UID 位布局 |
| `common/src/sditem.hpp` | `scripts/item/sd_item.gd` | ✅ | 物品结构 + 序列化 |
| `common/src/sysconst.hpp` | `scripts/core/sys_const.gd` | ✅ | 系统常量全集 |
| `client/src/motionnode.hpp` | `scripts/action/motion_node.gd` | ✅ | 帧数表 + 帧推进 + 触发器 |
| `client/src/creaturemovable.cpp` | `scripts/action/creature_movable.gd` | ✅ | 移动队列 + LDistance2 |
| `client/src/hero.cpp:509-760` | `scripts/action/hero_action_parser.gd` | ✅ | parseAction 骨架（魔法特效留 B4） |
| `common/src/statetrigger.hpp` | `scripts/action/state_trigger.gd` | ⬜ | 已被 MotionNode.add_trigger 覆盖同语义 |
| `client/src/basemagic.hpp`<br>`client/src/basemagic.cpp` | `scripts/magic/base_magic.gd` | ✅ | 基类 + update/done 语义 |
| `client/src/followuidmagic.*` | `scripts/magic/follow_uid_magic.gd` | ✅ | 追踪型 + 旋转对齐 + 命中判定 |
| `client/src/fixedlocmagic.hpp` | `scripts/magic/fixed_loc_magic.gd` | ✅ | 固定型 + 火焰灰烬/火墙 |
| `client/src/attachmagic.hpp` | `scripts/magic/attach_magic.gd` | ✅ | 附着型 + 雷电/法环/治疗 |
| `client/src/magicdb.*` | `scripts/magic/magic_db.gd` | ✅ | 26位文件名补码解析（已用真实素材验证） |
| `common/src/itemrecord.hpp` | `scripts/item/item_record.gd` | ✅ | 物品判定 + 穿戴校验 |
| `client/src/itemflex.cpp`<br>`client/src/itembox.hpp`<br>`client/src/itemalign.hpp` | `scripts/item/item_layout.gd` | ✅ | 三种对齐布局 + 背包/腰带坐标 |
| `common/src/serdesmsg.cpp`（SDInventory） | `scripts/item/sd_inventory.gd` | ✅ | 背包容器：堆叠/seqID/删除/合并 |
| `client/src/messagestackboard.cpp` | `scripts/ui/message_stack.gd` | ✅ | 消息栈：超限移除最旧 |
| `client/src/minimapboard.cpp` | `scripts/ui/mini_map.gd` | ✅ | 小地图：缩放锚点 + 边界夹取 |
| `client/src/token.hpp` | `scripts/ui/xml_token.gd` | ✅ | 排版单元 W1/W/W2/H1/H2 模型 |
| `client/src/xmltypeset.cpp`（核心） | `scripts/ui/xml_typeset.gd` | 🟨 | 换行/对齐/填充/坐标（内容解析待 B9） |
| `client/src/pngtexoffdb.hpp`<br>`client/src/hero.cpp:1411-1460` | `scripts/actor/human_gfx.gd` | ✅ | humanKey 位编码 + 四层 gfxID 公式 |
| `client/src/hero.cpp:76-150` | `scripts/actor/human_actor.gd` | ✅ | 七层绘制顺序（影子/身/武器/头） |
| `client/src/processrunnet.cpp:151-273` | `scripts/core/co_manager.gd` | ✅ | on_SM_ACTION 完整分发 |
| `client/src/processrunnet.cpp:279-345` | `scripts/core/co_manager.gd` | ✅ | on_SM_CORECORD + 视野裁剪 |
| `common/src/servermsg.hpp:204` | `scripts/core/co_record.gd` | ✅ | SMCORecord 49B（Level 偏移 45 实测） |
| `client/src/clientmonster.cpp:371-560` | `scripts/actor/monster_actor.gd` | ✅ | 怪物动作队列 + 死亡强制队列 |
| `client/src/pngtexoffdb.cpp:20-50` | `scripts/data/apng_loader.gd` | ✅ | APNG 拆帧 + 12 位补码偏移 |
| `common/src/monsterrecord.hpp` | `scripts/item/monster_record.gd` | ✅ | 怪物表 364 条查询 |
| 40 个 `client*.hpp`（帧表部分） | `data/monster_frames.json`<br>`scripts/actor/monster_frames.gd` | ✅ | 12 个类自定义帧表（脚本提取） |
| `client/src/clientmonster.cpp`<br>`client/src/clientstandmonster.hpp` | `scripts/actor/client_monster.gd` | ✅ | 默认帧表 + gfxID 合成 + standMode |
| `common/src/*.inc`（5 个） | `data/*.json` | ✅ | 1258 物品/364 怪物/603 地图/116 魔法/21 Buff |

## 测试覆盖（143 项全过）

| 测试 | 项数 | 覆盖 |
|---|---|---|
| `test_path.gd` | 25 | 方向工具、直线/对角/绕障、随机路径 |
| `test_data.gd` | 51 | 物品表 1258 条、UID 位布局、ActionNode 28B 编解码、魔法文件名补码、物品布局 |
| `test_monster.gd` | 28 | 默认帧表、TaoDog 双状态、Zuma 帧重定向、贴图键合成 |
| `test_ui.gd` | 39 | 背包堆叠/seqID/删除/合并、消息栈、小地图换算 |
| `test_typeset.gd` | 20 | Token 模型、换行判定、三种对齐、两端填充、垂直排版 |
| `test_human.gd` | 31 | humanKey 位编码、四层 gfxID、文件名解码、真实素材查找 |
| `anim_selftest.gd` | 12 | 动作切换（STAND↔WALK）、4 帧循环推进、朝向同步（场景内运行） |
| `net_selftest.gd` | 19 | 连接/PING/登录/查人/进图/进场/聊天（需先启动 mock） |
| `sync_selftest.gd` | 29 | CORECORD 解析（Mon/Ply）、APNG 拆帧、怪物表、实体创建、视野裁剪 |

## 已完成但需回归的

| 项 | 状态 | 说明 |
|---|---|---|
| 地图渲染 | ✅ | 瓦片层，800x800 解析 180ms，有效率 100% |
| 主角逐格移动 | ⚠️ | 寻路已对，移动推进有卡格 bug 待修（B10 统一处理） |
| 纹理缓存 | ✅ | LRU，命中率 94.9% |
