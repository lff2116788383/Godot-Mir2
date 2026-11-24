# mock-server

仅用于让 `client-godot` 客户端能跑通联调，**不是服务端实现**。

## 定位

| 是 | 不是 |
|---|---|
| 按真实 mir2x 二进制协议应答 | 不是生产级服务端 |
| 内存态、启动即用 | 不做持久化 / 数据库 |
| 够客户端走完登录→进图→移动→攻击→聊天 | 不做真实战斗计算、掉落、行会、交易 |

## 硬规则

**响应必须严格按 `docs/protocol.md` 的格式生成。**
如果 mock 为了“方便”放宽格式，客户端就会形成错误假设，
等接真实服务端时全部返工，而且会把客户端 bug 掩盖成 mock bug。

## 协议要点（详见 docs/protocol.md）

```
[headCode(1B)] [respID:LEB128 ?] [bodySize:LEB128 ?] [mask ?] [body ?]
```

- `headCode & 0x80` = hasResp
- type 1（定长+XOR）：`dataCount(LEB128) + mask + 非零字节`
- type 2（定长）：直接放 body
- type 3（变长）：`bodySize(LEB128) + body`
- 结构体一律 `#pragma pack(1)`，小端

## 计划范围

| 阶段 | 内容 |
|---|---|
| MVP | `CM_PING` / `CM_LOGIN` / `CM_QUERYCHAR` / `CM_ONLINE` / `CM_ACTION` / `CM_PLAYERSAY` |
| 后续 | 背包、穿戴、怪物、聊天历史 |
