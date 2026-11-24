# 迁移清单

本文件记录 C++/数据源到 Go 包的批次映射。文件级明细见 `migration/source-index.csv`。

状态定义：

- `pending`：尚未开始；
- `in-progress`：当前批次正在迁移；
- `ported`：已按原逻辑迁移；
- `adapted`：为 Go 或新架构做了受控适配；
- `deferred`：明确延期，必须记录原因；
- `obsolete`：确认不再需要。

## 批次总览

| ID | C++/数据源 | Go 目标 | 批次 | 状态 | 备注 |
|---|---|---|---|---|---|
| C01 | `common/src` 基础常量、数学、坐标、路径、ID | `internal/common`、`internal/protocol` | B1 | in-progress | `ActionNode`、StaticBuffer、StaticVector 已完成 |
| C02 | `protocoldef.hpp`、`clientmsg.hpp`、`servermsg.hpp`、`cerealf.hpp`、`serdesmsg.hpp` | `internal/protocol` | B1 | in-progress | 固定消息和 PortableBinary 已完成；serdes 对象迁移中 |
| C03 | `actormsg.hpp`、`actormsgpack.hpp`、`msgf.hpp` | `internal/actor`、`internal/protocol` | B1/B3 | pending | 消息包络与路由 |
| C04 | `*.inc` 静态记录表 | `internal/common` 或生成代码 | B1/B2 | pending | 优先使用转换工具生成 |
| C05 | `server/src/server.sql` | `internal/storage` | B2 | pending | 包含自定义 SQLite 函数 |
| C06 | 账号、角色、物品、行会、拍卖、摆摊、邮件数据库代码 | `internal/storage` | B2 | pending | 保留现有库兼容 |
| C07 | `actorpool.*`、`delaydriver.*`、`dispatcher.*` | `internal/actor` | B3 | pending | 地图串行执行优先 |
| C08 | `netdriver.*`、`actornetdriver.*`、`servicecorenet.*`、`serverpeer.*` | `internal/transport` | B3 | pending | 保留旧 TCP 协议 |
| C09 | `servermap.*`、`servermapop.*` | `internal/world` | B4 | pending | |
| C10 | `battleobject.*`、`monster.*`、`buff*.*`、怪物子类 | `internal/world` | B4 | pending | |
| C11 | `player.*`、`playerop.*`、`playernet.*`、背包和物品逻辑 | `internal/game` | B5 | pending | |
| C12 | `serverluacoroutinerunner.*`、`serverluamodule.*`、Lua 脚本 | `internal/script` | B5 | pending | 默认复用原 Lua 脚本 |
| C13 | `npchar.*`、`npcharop.*`、NPC 脚本 | `internal/game`、`internal/script` | B5 | pending | |
| C14 | `quest.*`、`questdb.*`、`questop.*`、任务脚本 | `internal/game`、`internal/script` | B5 | pending | 协程语义高风险 |
| C15 | 聊天、行会、交易、拍卖、摆摊、邮件 | `internal/game`、`internal/storage` | B6 | pending | |
| C16 | FLTK 管理窗口和监控窗口 | `internal/admin`、`web/admin` | B7 | pending | 改为 Web，不复刻 FLTK |

## 批次完成记录

### B0：工程准备与参考基线

状态：**完成**

- 建立 Go module 和目录骨架；
- 建立批次检查脚本 `scripts/check.ps1` / `scripts/check.sh`；
- 冻结源仓库提交 `58264934b5562cd283db7dc7daddec496b2de86e`；
- 保存工作树 patch、未跟踪文件清单和 1024 个源文件 SHA-256；
- 定位并记录资源与数据基线：
  - `E:\Desktop\game\mir2x_assets-main`：202965 个文件，约 1.44 GB，`version=1`；
  - `E:\Desktop\game\mir2x_data`：Git 提交 `9c6580c17feab74f72ff5e3b22546565e505f8e6`，约 1.89 GB；
- 建立 310 行 C++ 文件级迁移索引。

### B1：公共类型、协议和编解码

状态：**进行中**

已完成：

- 生成 89 个 `CM_*` 和 117 个 `SM_*` 消息常量；
- 从 C++ 注册表生成每条消息的 framing kind 和固定负载长度；
- 实现旧协议 TCP 分帧：
  - head byte / response bit；
  - base-128 response ID；
  - empty、fixed raw、fixed compressed、variable 四类消息；
- 实现 `msgf::encodeLength` 对应的 varint；
- 实现 `zcompf::xorEncode/xorDecode` 和 `xorEncode64/xorDecode64`；
- 实现 `common.ActionNode` 28 字节 wire codec；
- 实现基础 `StaticBuffer`、`StaticVector` 语义；
- 从 C++ 头文件生成 ABI 布局 fixture，记录 MS bitfield ABI 的关键尺寸和位布局；
- 迁移全部固定长度 client/server payload 类型；
- 实现通用固定 payload wire 编解码；
- 实现 ABI 敏感的 `CMChatMessageHeader`、`CMRequestLatestChatMessage`、`SMOnlineOK`、`SMCORecord` codec；
- 实现 `StaticBuffer`、`StaticVector` 的 packed wire 表示；
- 对全部固定长度消息做 Go 编码长度与 C++ `sizeof` 全量对照测试；
- `cerealf` PortableBinary writer/reader；
- `cerealf` 的 CF_NONE/CF_XOR/CF_ZSTD 压缩封装；
- 第一批 13 个 `serdesmsg` 对象：
  - `SDSendPackage`、`SDObserverNetPackage`
  - `SDInitQuest`、`SDInitGuard`、`SDInitPlayer`、`SDInitNPChar`、`SDInitMonster`
  - `SDNPCXMLLayout`
  - `SDChatGroupMember`、`SDChatPeerPlayerVar`、`SDChatPeerGroupVar`
  - `SDChatPeer`、`SDChatPeerID`
- 建立可重复生成的 C++ cereal golden fixture；
- 对首批 serdes 对象做 Go/C++ 字节级对照和反序列化测试；
- 协议与公共类型测试、race 测试通过。

待完成：

- `serdesmsg.hpp` 其余可变对象，重点包括 variant、optional、map、unordered_map 和 Lua 变量；
- `serdesmsg.cpp` 中的辅助序列化逻辑；
- 客户端/服务端真实 golden packet 样本；
- 大批量协议对象的 fuzz/边界测试。
