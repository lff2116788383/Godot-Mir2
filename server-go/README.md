# mir2x server-go

`mir2x` 服务端的 Go 复刻工程。

- 复刻方案：[REPLICATION_PLAN.md](REPLICATION_PLAN.md)
- 迁移清单：[MIGRATION_MANIFEST.md](MIGRATION_MANIFEST.md)
- 参考基线：[baseline/README.md](baseline/README.md)

## 当前阶段

B0 已完成：工程骨架、参考基线、资源基线、迁移索引和批次检查脚本。

B1 进行中：

- 已完成消息 ID、消息属性表、TCP 分帧、varint、XOR 压缩；
- 已完成全部 fixed client/server payload 类型和 wire codec；
- 已完成 C++ ABI 布局 fixture 和固定消息尺寸全量对照测试；
- 已完成 cereal PortableBinary writer/reader；
- 已完成 `cerealf` 压缩标记、XOR 与 ZSTD 编解码；
- 已完成两批 `serdesmsg` 对象及 C++ golden fixture 对照测试；
- 累计完成 47 个 C++ cereal golden 样本，覆盖地图、玩家、行会、聊天、排行和 peer 配置等消息；
- 剩余依赖 `SDItem`、runtime config、Lua 变量和复杂嵌套结构的 `serdesmsg` 对象，以及真实端到端报文。

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
