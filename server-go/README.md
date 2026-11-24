# mir2x server-go

`mir2x` 服务端的 Go 复刻工程。

- 复刻方案：[REPLICATION_PLAN.md](REPLICATION_PLAN.md)
- 迁移清单：[MIGRATION_MANIFEST.md](MIGRATION_MANIFEST.md)
- 参考基线：[baseline/README.md](baseline/README.md)

## 当前阶段

B0、B1、B2 已完成；B3 运行/传输核心已完成；下一批进入 B4。

### B3 已完成

- actor 串行邮箱、消息顺序、停止和定时消息；
- legacy TCP server、session、读写队列和生命周期；
- 消息 dispatcher 和 `CM_PING` -> `SM_PING` bootstrap handler；
- 可执行服务启动：SQLite 初始化、actor system、TCP listener、优雅退出；
- 真实 TCP ping 往返、response ID、坏包隔离、会话生命周期测试。

B3 的多进程 master/slave 和完整登录/游戏 handler 分别留给后续分布式阶段与 B5 业务层。
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
