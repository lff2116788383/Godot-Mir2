# Web 管理后台

计划使用 Go HTTP API 和 Vue 3 + TypeScript + Vite 前端。

硬性约束：

- 与游戏登录端口和游戏账号体系分离；
- 默认只监听 `127.0.0.1`；
- 所有写操作经过 application service，不直接修改地图或 actor 内存；
- 写操作必须审计；
- 前端生产构建通过 `go:embed` 嵌入服务端。

前端具体实现从 B7 开始。
