# mir2x server-go

`mir2x` 服务端的 Go 复刻工程。

- 复刻方案：[REPLICATION_PLAN.md](REPLICATION_PLAN.md)
- 迁移清单：[MIGRATION_MANIFEST.md](MIGRATION_MANIFEST.md)
- 参考基线：[baseline/README.md](baseline/README.md)

## 当前阶段

B0、B1、B2 已完成，下一步进入 B3。

### B1 已完成

- 旧客户端协议全部固定消息、100 个 serdes/Lua fixture；
- `serdesmsg.hpp` 94/94 声明类型覆盖；
- PortableBinary、`cerealf`、XOR、ZSTD；
- 8 个完整 client/server framing golden packet；
- packet、varint、XOR 短时 fuzz 通过。

真实 C++ 客户端/服务端会话抓包归入 B8，`serdesmsg.cpp` 领域辅助方法归入对应玩法批次。

### B2 已完成

- 原样嵌入 22 张表、11 个索引和 guild trigger；
- `jobValid`、`SYS_PLY_VIEWR`、`checkAuctionCategory`；
- SQLite foreign key、WAL、busy timeout、事务和 STRICT 表；
- 账号、角色、金币、背包、装备、任务、聊天、行会、拍卖、摆摊、投递和直接交易存储层；
- `cerealf` BLOB 兼容；
- JSON 配置和内容 SHA-256 校验。

静态物品/怪物/魔法/地图记录解析归入 B4/B5，拍卖购买税务/投递和摆摊付款编排归入 B6。
## 常用命令

```powershell
# 查看版本
go run ./cmd/mir2-server --version

# 运行批次检查
pwsh ./scripts/check.ps1

# race 检查需要 CGO 和 C 编译器
$env:CGO_ENABLED = '1'
pwsh ./scripts/check.ps1 -Race
```

## 说明

当前模块路径是临时值，正式发布前可以调整。游戏协议、数据库、Lua 脚本和资源兼容性优先于内部代码风格。
