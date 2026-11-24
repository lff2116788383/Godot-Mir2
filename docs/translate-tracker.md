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
| B12 | 怪物动作与像素位移 | L0+L2 | ✅ | 4 | ~500 |
| C1 | UI 框架（ROI/控件树/绘制/7 类控件） | L4 | ✅ | 10 | ~2400 |
| C2 | 主界面（底部栏/快捷栏/小地图/消息/背包） | L4 | ✅ | 2 | ~600 |
| C3 | XML 解析 + NPC 对话面板 | L0+L4 | ✅ | 4 | ~800 |
| C4 | 战斗表现（飘字/淡出/受击）+ 物品图标 | L2 | ✅ | 3 | ~600 |
| C5 | 面板：技能/队伍/行会/设置/物品列表（5 类） | L4 | ✅ | 8 | ~2400 |
| C6 | 32 个怪物类行为差异（数据化） | L0 | ✅ | 3 | ~900 |
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
| `client/src/clientcreature.hpp:150` | `scripts/actor/monster_actor.gd` | ✅ | updateMotion 动作推进 |
| `client/src/creaturemovable.cpp:191` | `scripts/actor/monster_actor.gd` | ✅ | moveNextMotion 队列切换 |
| `client/src/creaturemovable.cpp:216` | `scripts/action/creature_movable.gd` | ✅ | getShift 像素位移 |
| `client/src/clientmonster.cpp:640-672` | `scripts/action/creature_movable.gd` | ✅ | 怪物专用 MOTION_MON_WALK |
| `client/src/widget.roi.hpp` | `scripts/ui/framework/ui_roi.gd` | ✅ | ROI 求交裁剪 |
| `client/src/widget.hpp`+`widget.cpp` | `scripts/ui/framework/ui_widget.gd` | ✅ | 控件树/属性求值/尺寸计算 |
| `client/src/sdldevice.hpp:240-440` | `scripts/ui/framework/ui_canvas.gd` | ✅ | 绘制原语 + ROI 裁剪 |
| `client/src/buttonbase.cpp:51-130` | `scripts/ui/framework/ui_button.gd` | ✅ | 三态机 + 焦点 |
| `client/src/labelboard.*` | `scripts/ui/framework/ui_label.gd` | ✅ | 文字标签 |
| `client/src/imageboard.*` | `scripts/ui/framework/ui_image.gd` | ✅ | 图片 + 动画 |
| `client/src/scrollcontainer.cpp:376-550` | `scripts/ui/framework/ui_scroll.gd` | ✅ | 滚动 + 拖动 |
| `client/src/checkbox.*`+`radioselector.*` | `scripts/ui/framework/ui_checkbox.gd` | ✅ | 复选/单选 |
| `client/src/sliderbase.*`+`texslider.*` | `scripts/ui/framework/ui_slider.gd` | ✅ | 滑块 |
| `client/src/inputline.cpp:46-170` | `scripts/ui/framework/ui_input_line.gd` | ✅ | 输入 + 光标 + IME 三档 |
| `client/src/guimanager.cpp` | `scripts/ui/game_ui.gd` | ✅ | 面板布局与装配 |
| `client/src/xmlparagraph.cpp:320-410` | `scripts/ui/xml_layout.gd` | ✅ | XML 解析（layout/par/emoji/image/text） |
| `client/src/xmltypeset.cpp:624-780` | `scripts/ui/xml_typeset_builder.gd` | ✅ | Token 构建与排版 |
| `client/src/gui/npcchatboard` | `scripts/ui/ui_npc_chat.gd` | ✅ | NPC 对话面板 |
| `client/src/ascendstr.cpp` | `scripts/actor/fight_effects.gd` | ✅ | 飘字轨迹/淡出/死亡 |
| `client/src/inventoryboard.cpp:182-200` | `scripts/item/item_gfx.gd` | ✅ | 物品图标（pkgGfxID\|0x01000000） |
| `client/src/gui/skillboard` | `scripts/ui/ui_skill_board.gd` | ✅ | 63 魔法图标 × 8 组 × 坐标表 |
| `client/src/gui/teamstateboard` | `scripts/ui/ui_team_board.gd` | ✅ | 600x440，5 页签 |
| `client/src/guildboard` | `scripts/ui/ui_guild_board.gd` | ✅ | 3 页签，成员 13 行 |
| `client/src/gui/runtimeconfigboard` | `scripts/ui/ui_config_board.gd` | ✅ | 4 分组 10 配置项 |
| `client/src/itemlistboard`+`purchase`+`auction`+`stall` | `scripts/ui/ui_item_list_board.gd` | ✅ | 参数化复用 5 种面板 |
| 32 个 `client*.cpp/.hpp` | `data/monster_behaviors.json`<br>`scripts/actor/monster_behaviors.gd` | ✅ | 行为差异数据化 |
| 40 个 `client*.hpp`（帧表部分） | `data/monster_frames.json`<br>`scripts/actor/monster_frames.gd` | ✅ | 12 个类自定义帧表（脚本提取） |
| `client/src/clientmonster.cpp`<br>`client/src/clientstandmonster.hpp` | `scripts/actor/client_monster.gd` | ✅ | 默认帧表 + gfxID 合成 + standMode |
| `common/src/*.inc`（5 个） | `data/*.json` | ✅ | 1258 物品/364 怪物/603 地图/116 魔法/21 Buff |

| `client/src/gui/controlboard/*`（7 文件 2520 行） | `scripts/ui/ui_control_board.gd` | ✅ | 三段布局 178+中段+166、日志窗 344x84、命令行 344x15、头像 82x97、九宫格 9 钮 |
| `client/src/gui/myherostateboard/*` | `scripts/ui/ui_hero_state_board.gd` | ✅ | 328x466，11 装备格，衣服/头盔/武器相对锚点 |
| `client/src/gui/herostateboard/*` | `scripts/ui/ui_hero_state_board.gd` | ✅ | 284x348，格子 43x43，交易/加好友按钮 |
| `client/src/gui/quickaccessboard/*` | `scripts/ui/ui_quick_access_board.gd` | ✅ | 280x48，6 格 (17+42i, 6, 36x36) |
| `client/src/gui/overheadboard/*`（502 行） | `scripts/ui/ui_over_head.gd` | ✅ | 血条宽=图标数*10+2、显数字/满血隐藏规则 |
| `client/src/gui/underbodyboard/*` | `scripts/ui/ui_under_body.gd` | ✅ | 名字/行会[头衔]/配偶/称号，空行跳过 |
| `client/src/gui/stallbar/*` | `scripts/ui/ui_stall_bar.gd` | ✅ | max(name.w,60)+10 |
| `client/src/gui/purchaseboard/*`（901 行） | `scripts/ui/ui_purchase_board.gd` | ✅ | 格 38x38、行距 42、Ext1/Ext2 展开 |
| `client/src/gui/directtradeboard/*`（1174 行） | `scripts/ui/ui_direct_trade_board.gd` | ✅ | 494x330，5 态机、金钱解析/千分位 |
| `client/src/gui/stallboard/*`（1007 行） | `scripts/ui/ui_stall_board.gd` | ✅ | 282x390，三模式、按钮显隐三规则 |
| `client/src/gui/auctionboard/*`（2609 行） | `scripts/ui/ui_auction_board.gd` | ✅ | 720x440，13 行、列宽 108/140/70/142 |
| `client/src/gui/friendchatboard/*`（4584 行） | `scripts/ui/ui_friend_chat_board.gd` | ✅ | 5 页、九宫格拉伸、气泡 35x39 |
| `client/src/queststateboard.cpp` | `scripts/ui/ui_quest_board.gd` | ✅ | 363x446，描述区 (40,100) 270 宽，折叠 |
| `client/src/horseboard.cpp` | `scripts/ui/ui_horse_board.gd` | ✅ | 257x322，四钮 y=231 |
| `client/src/minimapboard.cpp`（608 行） | `scripts/ui/ui_mini_map_board.gd` | ✅ | 200x200/扩展 80%x50%，缩放拖动 |
| `client/src/inputstringboard.cpp`（333 行） | `scripts/ui/ui_input_string_board.gd` | ✅ | 358x268，输入/选择/确认三模式 |
| `client/src/menuboard.cpp`+`menuitem`+`pullmenu` | `scripts/ui/ui_menu_board.gd` | ✅ | 菜单项/分隔/勾选/箭头三帧 |
| `client/src/soundeffectdb.*`+`bgmusicdb.*` | `scripts/audio/audio_manager.gd` | ✅ | 848 音效 + 25 BGM，4 段 ID 规则 |
| `client/src/sdldevice.cpp:1533-1710` | `scripts/audio/audio_manager.gd` | ✅ | BGM 单轨 + SFX 128 轨、3D 定位 |
| `client/src/progusedb.*` | `scripts/data/proguse_tex.gd` | ✅ | UI 图库 1509 张（此前误判为缺素材） |

| `client/src/processlogo.cpp/.hpp`（105 行） | `scripts/ui/login/ui_login_process.gd` | ✅ | 淡入淡出 r1/r2=0.3、总 5s |
| `client/src/processlogin.cpp/.hpp`（317 行） | `scripts/ui/login/ui_login_process.gd` | ✅ | 800x600，4 钮、2 输入框、Tab 切换、密码星号 |
| `client/src/processselectchar.cpp/.hpp`（482 行） | `scripts/ui/login/ui_select_char_process.gd` | ✅ | 4 钮、角色锚点 (430,300)、4 态动画机 |
| `client/src/processcreatechar.cpp/.hpp`（384 行） | `scripts/ui/login/ui_create_char_process.gd` | ✅ | 3 职业钮、男女预览、女性战士 y+40 hack |
| `client/src/processcreateaccount.cpp`（373 行） | `scripts/ui/login/ui_account_process.gd` | ✅ | 440x310 框，3 输入框 |
| `client/src/processchangepassword.cpp`（440 行） | `scripts/ui/login/ui_account_process.gd` | ✅ | 440x331 框，4 输入框 |
| `common/src/idstrf.cpp` | `scripts/ui/login/ui_account_process.gd` | ✅ | isEmail / isPassword（8 位+4 类各 1）/ isCharName |
| `client/src/selectcharframecount.inc`（30 条） | `scripts/data/select_char_gfx.gd` | ✅ | (job,gender,anim) -> 帧数 |
| `common/src/jobf.hpp` | `scripts/data/select_char_gfx.gd` | ✅ | firstJob/jobGfxIndex/jobName/jobValid |
| `client/src/pngtexoffdb.hpp:55` humanKey | `scripts/data/select_char_gfx.gd` | ✅ | anim3-8 层9 影10 性11 职12-19 |
| `client/src/client.cpp` 进程调度 | `scripts/ui/login/login_flow.gd` | ✅ | ProcessID 1-9 状态机 + 网络应答分发 |

| `client/src/processrun.cpp:2388 drawObject` | `scripts/data/map_objects.gd` | ✅ | 物件层：底部对齐格底、动画仅 5 个 fileIndex |
| `client/src/processrun.cpp:295-425` 分层 | `scripts/data/map_object_layer.gd` | ✅ | 8 层顺序，角色夹在两批物件之间 |
| `common/src/mir2xmapdata.hpp:CELL` | `scripts/data/map_loader.gd` | ✅ | LAND/LIGHT/2×OBJECT 全解析（157K 物件、115 光照） |
| `client/src/processrunnet.cpp` on_SM_HEALTH 等 11 类 | `scripts/net/sd_msg_parser.gd` | ✅ | 状态/背包/腰带/装备/魔法/buff/视野 |
| `client/src/processrunnet.cpp:466-486` | `scripts/net/sd_msg_parser.gd` | ✅ | 地面物品列表同步 |
| `client/src/processrunnet.cpp:105-126,738-790` | `scripts/core/main.gd` | ✅ | 聊天/广播/裸文本/打击格/NPC |
| `client/src/processrun.cpp:352-359` | `scripts/core/co_manager.gd` | ✅ | 角色按 y 排序（Godot y_sort） |

## 测试覆盖（930 项全过）

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
| `sync_selftest.gd` | 33 | CORECORD 解析（Mon/Ply）、APNG 拆帧、怪物表、实体创建、视野裁剪、动作同步 |
| `test_ui_framework.gd` | 60 | ROI 裁剪、控件树、尺寸计算、按钮三态、滑块、复选、输入、滚动 |
| `ui_selftest.gd` | 17 | 面板装配、按钮点击开关面板、布局位置（场景内运行） |
| `test_c3_c4.gd` | 42 | XML 解析、排版换行、NPC 面板、飘字、物品图标 |
| `c3_c4_scene.gd` | 13 | NPC 对话弹出、背包注入、飘字推进速率、死亡淡出（场景内） |
| `test_c5_c6.gd` | 45 | 技能 63 图标、队伍 5 页、列表分页、行会、设置、行为表 |
| `c5_c6_scene.gd` | 9 | 8 面板装配、控制栏动作开关面板、行为描述加载（场景内） |

## 已完成但需回归的

| 项 | 状态 | 说明 |
|---|---|---|
| 地图渲染 | ✅ | 瓦片层，800x800 解析 180ms，有效率 100% |
| 主角逐格移动 | ⚠️ | 寻路已对，移动推进有卡格 bug 待修（B10 统一处理） |
| 纹理缓存 | ✅ | LRU，命中率 94.9% |
| `test_c7_c8.gd` | 235 | 17 个新面板几何、状态机、音效 ID 合成、3D 定位、素材可达 |
| `c7_c8_scene.gd` | 40 | 13 面板装配、控制栏贴底布局、音频 Autoload、UI 贴图实载（场景内） |
| `test_c9.gd` | 205 | Logo 三段淡入淡出、登录布局、选人帧表/图库、建号、账号密码校验、APNG 调色板 |
| `c9net_scene.gd` | 15 | 完整链路：连接→登入→查人→进游戏（需 mock 服务端） |

## C7+C8 关键结论

| 项 | 结论 |
|---|---|
| **UI 图库** | `texture/proguse/` 就是 UI 图库，1509 张 PNG，文件名 = 8 位 hex texID。**此前「素材缺 UI 图库」的判断是错的**，面板可像素级还原 |
| **尺寸自证** | 0x12=800x133、0x13=456x131、0x22=127x41 与 controlboard.hpp 注释完全吻合 |
| **头像 id** | `0x02000000 + jobGfxIndex*2 + (gender?0:1)`，实测 02000000..02000005 共 6 张 84x94（3 职业 × 2 性别） |
| **怪物音效** | 表里无 seff 覆盖配置，全部走默认 `0x02000000 + lookID*16 + offset`；seff 段 647 个文件 |
| **魔法音效** | `0x04000000 + seffBase*16 + stageIndex`，seffBase 已从 .inc 回填 53 条 |
| **3D 定位** | distance 用**欧氏距离**（不是切比雪夫），angle = 90 - 180*atan2(-dGY,dGX)/π |
| **headless 音频** | 无音频驱动时建 128 个 AudioStreamPlayer 会让 Godot 静默退出（无任何输出），必须显式跳过 |
| `c9net_scene.gd` | 15 | 完整链路：连接→登入→查人→进游戏（需 mock 服务端） |
| `test_c10.gd` | 54 | 地图物件解析、状态消息解析、协议码一致性 |

## C9 关键结论

| 项 | 结论 |
|---|---|
| **选人图库编码** | 74 张 APNG，`key = anim<<3 \| layer<<9 \| shadow<<10 \| gender<<11 \| job<<12` |
| **gender 位（重要）** | **1 = 男，0 = 女**。初版写反了，导致帧表与实载整体互换；用「战士男 anim1=20 帧 vs 女=11 帧」校准 |
| **调色板 PNG** | 31 张 shadow 层图是 colorType 3（调色板）。APNG 拆帧重建时必须带 PLTE（+ 可选 tRNS），否则报 `IDAT: Missing PLTE before IDAT` |
| **CM 编码** | type 1 消息（CM_LOGIN 等）是「掩码 + 压缩 body」，不是裸 body；必须走 `net.send()` 让它按 sm_table 选路 |
| **协议体尺寸** | CM_LOGIN 136 / CM_CREATECHAR 70 / CM_DELETECHAR 68 / CM_CREATEACCOUNT 203 / CM_CHANGEPASSWORD 201 |
| **画布适配** | 原版逻辑分辨率固定 800x600，Godot 下需 `scale = min(vw/800, vh/600)` + 居中，点坐标再反向换算 |
| **女性战士** | 建号界面 y 要 +40（源码 TODO hack，注释写明「seems offset-y is mis-aligned」） |
| **按钮显隐** | 选人界面「开始/创建/删除」三个按钮都 = `hasChar()`，无角色时只有「退出」可点 |
