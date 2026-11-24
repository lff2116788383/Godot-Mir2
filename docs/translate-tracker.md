# 翻译进度跟踪

> 分层规则见 `docs/translation-layers.md`
> 状态：⬜未开始 / 🟨进行中 / ✅完成 / ⚠️有问题

## 总览

| 批次 | 内容 | 层 | 状态 | 目标文件 | 行数(源) |
|---|---|---|---|---|---|
| B1 | 协议 + 方向 + 动作节点 + UID + 结构 | L0 | ✅ | ~15 | ~2400 |
| B2 | 数据表（5 个 .inc → JSON） | L0 | ✅ | 5 + 生成器 | 690KB |
| B3 | 动作状态机（hero 动作部分） | L0 | ✅ | ~4 | ~2000 |
| B4 | 魔法系统 | L0 | ⬜ | ~8 | ~1400 |
| B5 | 物品逻辑 | L0 | ⬜ | ~5 | ~1200 |
| B6 | 40 个怪物特有逻辑 | L0 | ⬜ | 40 | ~3000 |
| B7 | 28 个界面面板逻辑 | L4 | ⬜ | 28 | ~9000 |
| B8 | XML 排版引擎 | L0 | ⬜ | 6 | ~3450 |
| B9 | 渲染与输入（Godot 重写） | L2/L3 | ⬜ | 按需 | ~8000 |
| B10 | 统一联调 | — | ⬜ | — | — |

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
| `common/src/statetrigger.hpp` | `scripts/action/state_trigger.gd` | ⬜ | 状态触发（随 B4） |
| `common/src/*.inc`（5 个） | `data/*.json` | ✅ | 1258 物品/364 怪物/603 地图/116 魔法/21 Buff |

## 已完成但需回归的

| 项 | 状态 | 说明 |
|---|---|---|
| 地图渲染 | ✅ | 瓦片层，800x800 解析 180ms，有效率 100% |
| 主角逐格移动 | ⚠️ | 寻路已对，移动推进有卡格 bug 待修（B10 统一处理） |
| 纹理缓存 | ✅ | LRU，命中率 94.9% |
