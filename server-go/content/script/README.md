# Lua 脚本目录

## 复用策略：直接加载原版脚本，不做翻译

本目录只放**从原版逐字节复制**的两份基础脚本：

| 本地文件 | 原版来源 | 说明 |
|---|---|---|
| `base.lua` | `common/src/luamodule.lua` | assertType / tableEmpty / splitString / guardScope 等工具函数 |
| `npchar.lua` | `server/src/npchar.lua` | NPC 事件分发核心（`_RSVD_NAME_npc_main`） |

**注意**：这两份文件不要手工修改。它们与原版保持 MD5 一致，
便于用 `md5sum` 校验是否被意外改动。

## 具体 NPC 脚本

341 个 NPC 脚本（`道馆_1.阿天_1.GLOC_396_116_0.LOOK_18.lua` 这种）
**不复制进来**，而是由 `MIR2X_SCRIPT` 环境变量指向原版目录：

```
MIR2X_SCRIPT="E:/Desktop/game/mir2x/server/script"
```

理由：NPC 脚本有 341 个文件、约 10 万行，复制进来只会产生
「原版更新了但副本没同步」的漂移。直接加载原版目录，
任何脚本修正都能立即生效，也让「是否与原文一致」变成
`diff` 就能回答的问题。

## 加载顺序（严格照原版 NPChar 构造）

1. 建 Lua 状态，设 `package.path`
2. 注入 `SYS_*` 常量与 `LOGTYPE_*`
3. 绑定 C 函数（`uidPostXML` / `getNPCName` / `uidQueryGold` …）
4. 执行 `base.lua`
5. 执行 `npchar.lua`
6. 执行具体 NPC 脚本（它会调 `setEventHandler`）

顺序不能乱：具体脚本第一行就是
`local dialog = require('include.dialog')`，
而 `include/dialog.lua` 依赖 `base.lua` 里的 `assertType`。
