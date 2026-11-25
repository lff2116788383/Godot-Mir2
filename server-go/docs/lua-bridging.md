# Lua 复用方案说明（回应「Lua 不能直接复用吗」）

> 日期：2026-11-25
> 对应文档：replication-work-document v0.5 §5.5、§6.4、决策 D5

## 结论

**没有冲突。文档要求「Lua 桥接而非翻译」，我的实现正是桥接，而且已经跑通了。**

三个问题逐条回答：

| 问题 | 回答 |
|---|---|
| 方案与文档有冲突吗？ | 没有。文档 D5 明确「Lua 桥接，不翻译」，本实现完全遵循 |
| Lua 不能直接复用吗？ | **能，而且已经在直接复用**。原版脚本零修改运行 |
| 这个不用翻译吧？ | 不用。一行都没翻译 |

## 复用的证据

### 1. 基础脚本：逐字节复制

```sh
$ md5sum content/script/base.lua      E:/Desktop/game/mir2x/common/src/luamodule.lua
83c089fc67ab35800bb000381b0e531d      content/script/base.lua
83c089fc67ab35800bb000381b0e531d      E:/Desktop/game/mir2x/common/src/luamodule.lua

$ md5sum content/script/npchar.lua    E:/Desktop/game/mir2x/server/src/npchar.lua
58df728abcecf3c2c6d7644ab89a5df4      content/script/npchar.lua
58df728abcecf3c2c6d7644ab89a5df4      E:/Desktop/game/mir2x/server/src/npchar.lua
```

`base.lua` 是 `luamodule.lua`（工具函数库），`npchar.lua` 是 NPC 事件分发核心
（700 行，含 `_RSVD_NAME_npc_main` / 菜单生成 / 事件路径解析）。两份都是原样复制。

### 2. NPC 脚本：直接加载原版目录，不复制

341 个 NPC 脚本（`道馆_1.阿天_1.GLOC_396_116_0.LOOK_18.lua` 这种）
**没有复制进仓库**，由 `MIR2X_SCRIPT` 指向原版目录直接加载：

```sh
MIR2X_SCRIPT="E:/Desktop/game/mir2x/server/script" ./mir2-server
```

理由：约 10 万行，复制会产生「上游更新了但副本没同步」的漂移。
直接指向原版，脚本修正立刻生效。

### 3. 加载结果：161 / 161 全通过

```
$ go run ./tools/npccheck
地图 86 张，NPC 共 340 个
记录表：物品 1258 / 怪物 364 / 地图 603

=== 非空脚本 161 个：成功 161 / 失败 0；空占位 179 个 ===
```

（179 个「失败」是原版 0 字节占位文件，地图上摆了 NPC 但没写对话，
比如「士官_1」「小老板_1」。不是加载失败。）

### 4. 真实对话输出

跑 `道馆_1.阿天_1` 的 `SYS_ENTER`（玩家点 NPC），脚本产出：

```xml
<layout>
<par>这里是沙巴克城<t color="red"></t>行会的领地。</par>
<par>贫道是仓库管理员，需要的话，可以把东西暂时寄存在我这里。</par>
<par></par>
<par><event id="npc_goto_secure" single="1">寄存</event>物品</par>
<par><event id="npc_goto_get_back" single="1">取回</event>物品</par>
<par><event id="npc_goto_set_password" single="1">设置</event>仓库密码</par>
<par><event id="npc_goto_daily_quest" single="1">对今日的任务进行了解</event></par>
<par><event id="_RSVD_NAME_EXIT__14208236065" single="1" close="1">关闭</event></par>
</layout>
```

这段 XML 是**原版脚本自己生成的**，Go 侧只负责：
1. 绑定 `uidPostXML` 等 C 函数
2. 把生成的 XML 打成 `SM_NPCXMLLAYOUT` 发给客户端

服务端实跑（`MIR2X_SCRIPT` 指向原版）日志：

```
INFO map names loaded count=603
INFO npc loaded map=比奇县_0 count=25 skippedEmpty=26
INFO map npc spawned map=比奇县_0 loaded=25 spawned=25
INFO player entered world name=测试角色 map=1 at=(400,402)
```

## 桥接与翻译的分界线

| 部分 | 做法 | 为什么 |
|---|---|---|
| NPC 对话内容 | **Lua 原样运行** | 内容要长期维护，翻译后每次改文案都要重编译 |
| 事件分发/菜单生成 | **Lua 原样运行** | 700 行逻辑，341 个脚本依赖其精确语义 |
| 任务流程 | **Lua 原样运行** | 含 `uidRemoteCall` 动态执行，无法静态翻译 |
| 地图刷怪配置 | **Lua 原样运行** | 数据即代码 |
| 玩家/怪物/地图/战斗 | **Go 重写** | 热路径，性能敏感，且有明确的数据结构 |
| 协议编解码 | **Go 重写** | 必须字节级对齐 |

## 复用过程中必须解决的 4 个兼容性缺口

这些不是「翻译」，是让 Lua 5.1 解释器能跑 Lua 5.3 脚本的**桥接层**。
文档 §5.5 也预判了这类问题（「纯 Go 引擎 POC 必须验证协程、标准库行为」）。

| # | 缺口 | 现象 | 处理 |
|---|---|---|---|
| 1 | `table.pack` / `table.unpack` 缺失（5.2+） | `base.lua:29` 报 "attempt to call a non-function object"，所有脚本第一步就挂 | 补 polyfill，语义照 5.2（带 `n` 字段） |
| 2 | `math.type` 缺失（5.3+） | `assertType(x, 'integer')` 全部失败 | 补 polyfill，按「能否无损转 int64」判定 |
| 3 | `coroutine.running()` 语义不同 | 5.1 主线程返回 `nil`，5.2+ 返回 `(thread, true)`。原版沙箱靠它区分主线程/协程，语义不对会让所有玩家的 TLS 退化成全局变量（**两人同时点同一 NPC 时对话内容串号**） | 包装成 5.2 形态 |
| 4 | `//` 整数除法是 5.3 语法 | **编译期**语法错误，polyfill 救不了 | 加载时改写源码为 `math.floor(a / b)`，原文件不动。影响面：3 文件 6 处 |

第 4 条特别说明：`//` 必须展开成 `math.floor(a/b)` 而不是 `/`，
因为调用方都是 `string.format('%d', a // b)`，`%d` 遇浮点会报错。

## 与原版的一个真实差异（已记录）

原版 `getTLSTable()` 在主线程调用会主动 `error()`，设计前提是
**每个 NPC 事件都在独立协程里执行**（`m_luaRunner->spawn`）。

第一次实现直接 `CallGlobal` 调用，报错信息是
`npchar.lua:403: attempt to call a non-function object` ——
看起来像函数没绑定，实际是 `getTLSTable` 内部主动抛错。
定位花了一些时间，因为报错行号指向的是一个「存在的函数调用」。

修法是照原版用协程执行（`L.NewThread()` + `L.Resume`）。

## 尚未实现（诚实标注）

| 项目 | 状态 | 影响 |
|---|---|---|
| `uidRemoteCall` | 空实现，返回 nil | 254 处调用会走 else 分支。「设置仓库密码」「远程查别的 NPC」这类跨 NPC 联动不可用 |
| 事件 ID 的 AES 加密 | 未加密 | 客户端只是原样回传，链路能通；但缺少「防改包点未授权选项」 |
| 仓库/交易/修理 | 空实现 | 相关对话框能弹出，但点确认无效果 |
| `SM_NPCXMLLAYOUT` 的 XML 里 `<t>` 颜色标签 | 原样透传 | 客户端 `xml_layout.gd` 支持解析，无需服务端处理 |

这几项不影响「NPC 能对话」这个主链路，逐条记在 `REPLICA_AUDIT.md`。

## 下一步

1. 把 `uidRemoteCall` 接上（actor 请求/响应 + 超时），解锁 254 处调用
2. 让 `CM_NPCEVENT` 真正走通客户端（客户端当前还没发这条消息）
3. 地图刷怪脚本（`server/script/map/*.lua`）同样复用
4. 任务系统（`quest.lua` 1819 行 + `quest/` 目录）同样复用
