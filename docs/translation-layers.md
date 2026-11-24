# 翻译分层方案：哪些用 Godot API，哪些纯 C++ 翻译

> 依据：对 `client/src` 268 个文件的 SDL 依赖实测（见文末数据）
> 原则：**能纯翻译的一律纯翻译；只有 Godot 确实无法等价表达时才用 Godot API**

---

## 一、核心结论

SDL 的用途非常集中，实测只涉及 4 类能力：

| SDL 能力 | 使用文件数 | Godot 对应 |
|---|---|---|
| `SDL_Texture` 纹理 | 26 | `Texture2D` / `ImageTexture` |
| `SDL_Event` 事件 | 71 | `InputEvent` |
| `SDL_GetTicks` 计时 | 22 | 引擎 `delta` / `Time` |
| `SDL_BlendMode` 混合 | 34 | `CanvasItem` 混合模式 |
| `SDL_mixer` 音频 | 10 | `AudioStreamPlayer` |

**268 个文件中 146 个完全不依赖 SDL** —— 这些可直接逐行翻译，占 54.5%。

---

## 二、分层划分

### L0 纯翻译层（不碰 Godot 渲染 API）

**特征**：纯数据、纯算法、状态机、协议。**零 Godot 依赖**，只可用 `RefCounted` 等基础类型。

| 模块 | 原实现 | 行数 | 翻译方式 |
|---|---|---|---|
| 协议编解码 | `clientmsg.hpp` `servermsg.hpp` `msgf.hpp` `zcompf.cpp` | ~900 | 直接用 `PackedByteArray` |
| cerealf 序列化 | `cerealf.hpp` | ~180 | 同上 |
| 方向与坐标 | `pathf.hpp:60-160` | ~100 | 纯函数 |
| 寻路 A\* | `pathf.hpp:191-512` `pathf.cpp` | 1030 | ✅ 已完成 |
| 动作节点 | `actionnode.hpp` | 722 | 纯结构 + 联合体 |
| 动作类型 | `motion.hpp` | 129 | 纯枚举 |
| 数据表 | `itemrecord.inc` `monsterrecord.inc` `maprecord.inc` `magicrecord.inc` | 690KB | 转 JSON，脚本生成 |
| UID 规则 | `uidf.cpp` `uidf.hpp` | ~200 | 位运算 |
| 地图数据 | `mir2xmapdata.hpp` | 330 | ✅ 已完成 |
| 物品定义 | `sditem.hpp` | 300 | 纯结构 |
| 魔法定义 | `magicrecord.hpp` | 300 | 纯结构 |

**翻译规则**：
- 逐行对应，函数名保持一致（如 `getNextDir` → `get_next_dir`）
- 原版注释一并翻译，注明对应源码行号
- 不引入 Godot 类型（`Vector2i` 可用，但不用 `Node2D`）

### L1 序列化/资源层（用 Godot 的文件与图像 API）

**特征**：读写文件、解码图片。**只用 Godot 的 IO / Image，不用 Node**。

| 模块 | 原实现 | Godot API | 说明 |
|---|---|---|---|
| 文件读取 | `fileptr.hpp` `filesys.cpp` | `FileAccess` | 等价替换 |
| PNG 解码 | `imgf.cpp` | `Image.load_from_file` | 等价替换 |
| 纹理池 | `inndb.hpp` `pngtexoffdb.hpp` | `ImageTexture` + 自建 LRU | 逻辑翻译，存储换 Godot |
| ZSDB 打包库 | `zsdb.cpp` | `FileAccess` + 自建索引 | 需翻译格式（可选） |
| 音频解码 | `SDL_mixer` | `AudioStreamPlayer` | 直接用 Godot |

### L2 渲染层（**必须用 Godot API**）

**特征**：把像素画到屏幕上。**这部分不翻译，重新实现**。

| 原实现 | 行数 | Godot 方案 | 为什么必须换 |
|---|---|---|---|
| `sdldevice.cpp` | 1708 | `RenderingServer` / `Camera2D` | SDL 设备管理，Godot 内置 |
| `gfx*board.cpp` | ~1700 | `Node2D` + `Sprite2D` | 纯绘制调用 |
| `widget.cpp` | 1194 | `Control` 子树 | 控件绘制 |
| `textshadowboard.cpp` | — | `Label` + `LabelSettings` | 文字渲染 |
| `layoutboard.cpp` | 999 | `Container` 体系 | 布局 |
| `texaniboard.cpp` `imageboard.cpp` | — | `AnimatedSprite2D` / `Sprite2D` | 贴图显示 |
| `clientmap.cpp` 绘制部分 | — | `TileMapLayer` / 自建瓦片池 | 地图绘制 |
| `motioneffect.cpp` 绘制部分 | — | `CPUParticles2D` / 自绘 | 特效 |

**注意**：L2 只换「怎么画」，**不换「画什么」**。例如 `hero.cpp` 里「第 x 帧用哪个 gfxID」是 L0 逻辑，「把这张图画到 (px,py)」是 L2 渲染——必须拆开。

### L3 输入层（用 Godot InputEvent）

| 原实现 | 行数 | Godot 方案 |
|---|---|---|
| `sdldevice.cpp` 事件循环 | 71 文件涉及 | `_input` / `_unhandled_input` |
| `bevent.hpp` 事件定义 | — | 翻译为信号或自定义 InputEvent |
| `imeboard.cpp` `ime.cpp` | ~300 | `LineEdit` 内置 IME |
| 鼠标/键盘 | — | `InputEventMouseButton` / `InputEventKey` |

**关键点**：`bevent.hpp` 里的事件类型枚举**要完整翻译**，因为逻辑代码依赖它。只是产生事件的源头从 SDL 换成 Godot。

### L4 界面逻辑层（混合：逻辑翻译 + 控件用 Godot）

**特征**：28 个 board 面板。**布局数据与交互逻辑翻译，绘制交给 Godot Control**。

| 部分 | 处理方式 |
|---|---|
| 面板坐标、尺寸、层级 | **翻译**（照搬原版数值） |
| 按钮点击、列表选择、滚动 | **翻译逻辑**，事件源换 Godot |
| 数据绑定（背包格 → 物品） | **翻译** |
| 实际绘制（圆角、描边、渐变） | **用 Godot** |
| 文字排版算法（`xmltypeset.cpp` 1730 行） | **翻译**（这是逻辑，不是绘制） |

---

## 三、判定规则（怎么决定一个文件属于哪层）

按顺序判断，第一个命中即定：

| 序 | 判据 | 归属 |
|---|---|---|
| 1 | 文件里出现 `SDL_Render*` / `SDL_Blit*` / `SDL_FillSurface*` | **L2 渲染** → 重写 |
| 2 | 文件里出现 `SDL_Texture` 且主要用于**画** | **L2 渲染** → 重写 |
| 3 | 文件里出现 `SDL_PollEvent` / `SDL_Event` | **L3 输入** → 事件源换 Godot，逻辑翻译 |
| 4 | 文件里只有 `SDL_GetTicks` | **L0 纯翻译**（改成传 `delta` 参数） |
| 5 | 文件里出现 `SDL_mixer` | **L1** → `AudioStreamPlayer` |
| 6 | 文件不含 `SDL` | **L0 纯翻译** |

> 实测：命中规则 6 的有 **146 个文件**，这是纯翻译的主体。

---

## 四、必须先拆开的三个文件

原版有几个「巨文件」把逻辑与渲染混在一起，**翻译前必须先拆**。

| 文件 | 行数 | 混杂内容 | 拆分方案 |
|---|---|---|---|
| `hero.cpp` | 1733 | 动作状态机（L0）+ 贴图选择（L0）+ 绘制（L2） | 拆成 `hero_motion.gd`（L0）+ `hero_render.gd`（L2） |
| `processrun.cpp` | 3264 | 流程控制（L0）+ 输入处理（L3）+ 各类绘制（L2） | 按职责拆 5~6 个文件 |
| `myhero.cpp` | 890 | 动作队列（L0）+ 碰撞（L0）+ 输入（L3） | 拆 3 个文件 |
| `clientmap.cpp` | — | 地图数据（L0）+ 渲染（L2） | 拆 2 个 |
| `sdldevice.cpp` | 1708 | 设备管理（L2 整体重写） | 不翻译，直接用 Godot |

---

## 五、翻译工作流（每批的操作规范）

### 每批固定动作

1. **登记**：在 `docs/translate-tracker.md` 加一行（源文件 → 目标文件 → 层 → 状态）
2. **读源**：完整读 C++ 原文，包括注释
3. **翻译**：逐函数对应，保留原注释并加源码行号
4. **标注**：文件头写清「对应 pathf.cpp:212-278」这类定位
5. **不测**：本批只翻译，不跑测试
6. **批末统一联调**：一批翻完后统一编译 + 跑测试

### 批次划分（按依赖顺序，不按大小）

| 批次 | 内容 | 预估文件 |
|---|---|---|
| **B1** | L0 基础：协议、方向、动作节点、UID、物品/魔法结构 | ~15 |
| **B2** | L0 数据表：4 个 `.inc` 转 JSON + 查询 API | 4 源 → 1 生成器 |
| **B3** | L0 动作状态机：`hero.cpp` 的动作部分 + `motionnode.hpp` | ~4 |
| **B4** | L0 魔法：`basemagic` `followuidmagic` `fixedlocmagic` `attachmagic` | ~8 |
| **B5** | L0 物品逻辑：`itembox.cpp` `itemflex.cpp` | ~5 |
| **B6** | L0 怪物：40 个 `client*.hpp` 的特有逻辑 | 40 |
| **B7** | L4 界面逻辑：28 个 board 的非绘制部分 | 28 |
| **B8** | L0 XML 排版：`xmltypeset.cpp` `xmlparagraph.cpp` | 6 |
| **B9** | L2/L3 渲染与输入：按 Godot 习惯重写 | 按需 |
| **B10** | 统一联调 + 修复 | — |

---

## 六、进度跟踪

进度记录在 `docs/translate-tracker.md`，每批完成更新一次。

**当前状态**：

| 批次 | 状态 | 已翻文件 | 说明 |
|---|---|---|---|
| B1 | 进行中 | `pathf.hpp`+`pathf.cpp`(1030行) ✅<br>`mir2xmapdata.hpp` ✅<br>`msgf.hpp`+`zcompf.cpp` ✅ | 寻路 25 项自检通过 |
| B2 | 未开始 | — | |
| B3 | 未开始 | — | |
| B4~B10 | 未开始 | — | |

---

## 七、实测数据附录

### SDL API 使用频次（Top 15）

| API | 次数 | 归属层 |
|---|---|---|
| `SDL_Texture` | 83 | L2 |
| `SDL_Event` | 83 | L3 |
| `SDL_GetError` | 62 | — |
| `SDL_BLENDMODE_BLEND` | 34 | L2 |
| `SDL_EVENT_MOUSE_BUTTON_DOWN` | 25 | L3 |
| `SDL_GetTicks` | 22 | L0 |
| `SDL_EVENT_KEY_DOWN` | 22 | L3 |
| `SDL_EVENT_MOUSE_MOTION` | 21 | L3 |
| `SDL_FillSurfaceRect` | 15 | L2 |
| `SDL_EVENT_MOUSE_BUTTON_UP` | 14 | L3 |
| `SDL_BlendMode` | 14 | L2 |
| `SDL_SetRenderLogicalPresentation` | 12 | L2 |
| `SDL_ShowCursor` | 12 | L2 |
| `SDL_DestroyTexture` | 12 | L2 |
| `SDL_mixer` | 10 | L1 |

### 文件分布

| 类别 | 文件数 | 占比 |
|---|---|---|
| 不依赖 SDL（L0 纯翻译） | **146** | 54.5% |
| 依赖 SDL | 122 | 45.5% |
| 其中：仅用 Texture（画） | 26 | 9.7% |
| 其中：用 Event（输入） | 71 | 26.5% |
