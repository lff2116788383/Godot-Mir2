#!/usr/bin/env python3
"""从 mir2x C++ 头文件抽取消息表，生成 docs/protocol.md。

只读参照源码，不修改 mir2x 仓库。用法：
    python tools/gen_protocol_doc.py [mir2x根目录] [输出文件]
"""
import json
import os
import re
import sys

DEFAULT_SRC = r"E:/Desktop/game/mir2x/common/src"
DEFAULT_OUT = "docs/protocol.md"

TYPE_DESC = {
    0: "空体（无负载）",
    1: "定长 + XOR 压缩",
    2: "定长 + 不压缩",
    3: "变长 + 不压缩",
}


def parse_enum(path, start_marker):
    """解析 C 风格 enum，支持自增与别名。"""
    src = open(path, encoding="utf-8", errors="replace").read()
    i = src.find(start_marker)
    body = src[i:src.find("};", i)]
    out = []
    val = None
    for line in body.splitlines():
        line = line.split("//")[0].strip().rstrip(",")
        if not line or line.startswith("enum") or line.startswith("static_assert"):
            continue
        m = re.match(r"^([A-Z0-9_]+)\s*(?:=\s*([^,]+))?$", line)
        if not m:
            continue
        name, expr = m.group(1), m.group(2)
        if expr is None:
            val = 1 if val is None else val + 1
        else:
            expr = expr.strip()
            if expr.isdigit():
                val = int(expr)
            else:
                found = [v for n, v in out if n == expr]
                if not found:
                    continue
                val = found[0]
        out.append((name, val))
    return out


def parse_attrs(path, tag):
    """解析 _RSVD_register_<tag>(NAME, type, len) 注册表。"""
    src = open(path, encoding="utf-8", errors="replace").read()
    reg = {}
    pat = r"_RSVD_register_" + tag + r"\((SM_\w+|CM_\w+),\s*(\d)\s*(?:,\s*([^)]+))?\)"
    for m in re.finditer(pat, src):
        reg[m.group(1)] = (int(m.group(2)), (m.group(3) or "").strip())
    return reg


def table_rows(items, attr):
    out = []
    for name, code in items:
        if name.endswith("_BEGIN") or name.endswith("_END"):
            continue
        if name not in attr:
            continue
        t, ln = attr[name]
        out.append("| `{}` | `{}` | {} — {} | {} |".format(
            code, name, t, TYPE_DESC[t], ln or "—"))
    return out


def build(src_dir, out_path):
    cm = parse_enum(os.path.join(src_dir, "clientmsg.hpp"), "enum CMType")
    sm = parse_enum(os.path.join(src_dir, "servermsg.hpp"), "enum SMType")
    cmattr = parse_attrs(os.path.join(src_dir, "clientmsg.hpp"), "clientmsg")
    smattr = parse_attrs(os.path.join(src_dir, "servermsg.hpp"), "servermsg")

    cm_valid = [n for n, _ in cm if not n.endswith("_BEGIN") and not n.endswith("_END")]
    sm_valid = [n for n, _ in sm if not n.endswith("_BEGIN") and not n.endswith("_END")]

    L = []
    A = L.append

    A("# mir2x 协议对齐表（客户端复刻基线）")
    A("")
    A("> 本文件由 `tools/gen_protocol_doc.py` 从 mir2x C++ 头文件**抽取生成**，不是手抄。")
    A("> 基线提交：mir2x `5826493`；源码根：`E:\\Desktop\\game\\mir2x`（只读参照，禁止修改）。")
    A("> 本轮只服务 **client-godot 客户端分支**；服务端不在本分支内实现。")
    A("")
    A("## 0. 校验状态")
    A("")
    A("| 项 | 结果 |")
    A("|---|---|")
    A("| CM 枚举条数（有效） | {} |".format(len(cm_valid)))
    A("| SM 枚举条数（有效） | {} |".format(len(sm_valid)))
    A("| CM 注册表缺失 | {} |".format(len([n for n in cm_valid if n not in cmattr])))
    A("| SM 注册表缺失 | {} |".format(len([n for n in sm_valid if n not in smattr])))
    A("| 字节序 | 小端（PortableBinary 固定） |")
    A("| 结构对齐 | `#pragma pack(1)` |")
    A("")
    A("## 1. 传输层帧格式")
    A("")
    A("### 1.1 整数编码")
    A("")
    A("| 项 | 规则 | 依据 |")
    A("|---|---|---|")
    A("| 帧内整数 | **小端** | `cereal::PortableBinaryOutputArchive` |")
    A("| 变长长度 | **LEB128**（低 7 位，高位续读） | `msgf::encodeLength` |")
    A("| 比特域 | 按 C++ 位域低位在前 | `clientmsg.hpp:109` / `servermsg.hpp:130` |")
    A("")
    A("### 1.2 帧结构")
    A("")
    A("```")
    A("[headCode(1B)] [respID:LEB128 ?] [bodySize:LEB128 ?] [mask:maskLen B ?] [body:bodySize B ?]")
    A("```")
    A("")
    A("- `headCode & 0x80` = hasResp，表示带应答 ID。")
    A("- 消息体形态查表决定，四类：")
    A("")
    A("| type | 线上结构 | 说明 |")
    A("|---|---|---|")
    A("| 0 | 仅 headCode | 空体 |")
    A("| 1 | `respID? + dataCount(LEB128) + mask + body` | 定长 XOR 压缩 |")
    A("| 2 | `respID? + body(dataLen)` | 定长直传 |")
    A("| 3 | `respID? + bodySize(LEB128) + body` | 变长，多为 cerealf 块 |")
    A("")
    A("### 1.3 XOR 压缩（type 1）")
    A("")
    A("- `maskLen = (dataLen + 7) / 8`")
    A("- `dataLen >= 256` 时切 64 位宽：`maskLen = (dataLen + 63) / 64`，按 8 字节整块判零")
    A("- mask 第 i 位 = 原始第 i 字节是否非零；body 顺序存非零字节")
    A("")
    A("> **坑点**：解码必须用消息表里的 `dataLen`（原长），不能用收到的 `dataCount`，否则尾部丢失。")
    A("")
    A("### 1.4 cerealf 封装（type 3 负载）")
    A("")
    A("序列化结果尾部追加 1 字节压缩标记：")
    A("")
    A("| 尾字节 | 含义 |")
    A("|---|---|")
    A("| 0 | CF_NONE，未压缩 |")
    A("| 1 | CF_ZSTD，zstd 压缩 |")
    A("| 2 | CF_XOR，前 1 字节为原始长度 |")
    A("")
    A("编码细节：64 位平台 `size_t`/`vector` 计数为 **u64**；`std::string` = `u64 长度 + 原始字节`。")
    A("")
    A("### 1.5 关键定长结构体（实测 sizeof，ucrt64 g++ -std=c++20）")
    A("")
    A("| 结构体 | 字节 | 备注 |")
    A("|---|---|---|")
    A("| `CMPing` / `SMPing` | 4 | u32 tick |")
    A("| `CMLogin` | 136 | 2 × StaticBuffer<64> |")
    A("| `SMAction` / `CMAction` | 44 | u64 + u64 + ActionNode(28) |")
    A("| `ActionNode` | 28 | 位域 3 + 4×i16 + u64 union + 4B extParam |")
    A("| `SMOnlineOK` | 177 | 见定义 |")
    A("| `SMCORecord` | 49 | u64 + u64 + ActionNode + 12B union |")
    A("| `SMCastMagic` | 36 | 见定义 |")
    A("")
    A("**`StaticBuffer<N>` = 2 + (N+1) 字节**：`u16 size` + `data[N+1]`。")
    A("`size` 是实际字节数，读字符串要按 `size` 截断。")
    A("")
    A("## 2. 客户端 → 服务端（CM）")
    A("")
    A("| code | 名称 | 类型 | 定长 |")
    A("|---:|---|---|---|")
    L.extend(table_rows(cm, cmattr))
    A("")
    A("## 3. 服务端 → 客户端（SM）")
    A("")
    A("| code | 名称 | 类型 | 定长 |")
    A("|---:|---|---|---|")
    L.extend(table_rows(sm, smattr))
    A("")
    A("## 4. 客户端复刻优先级分组")
    A("")
    A("### P0 —— 走通「登录 → 进图 → 移动 → 攻击 → 聊天」必需")
    A("")
    A("| 方向 | 消息 | 用途 |")
    A("|---|---|---|")
    A("| C→S | `CM_PING` / `CM_LOGIN` / `CM_QUERYCHAR` / `CM_ONLINE` | 握手、登录、选人 |")
    A("| C→S | `CM_ACTION` | **移动/站立/攻击全部走这一条** |")
    A("| C→S | `CM_PLAYERSAY` / `CM_CHATMESSAGE` | 聊天 |")
    A("| S→C | `SM_PING` / `SM_LOGINOK` / `SM_LOGINERROR` | 握手应答 |")
    A("| S→C | `SM_QUERYCHAROK` / `SM_ONLINEOK` / `SM_STARTGAMESCENE` | 进图三件套 |")
    A("| S→C | `SM_ACTION` / `SM_CORECORD` / `SM_CODISAPPEAR` / `SM_OFFLINE` | 视野内实体同步 |")
    A("| S→C | `SM_HEALTH` / `SM_EXP` / `SM_GOLD` / `SM_INVENTORY` / `SM_BELT` | 状态面板 |")
    A("| S→C | `SM_PLAYERSAY` / `SM_CHATMESSAGELIST` / `SM_TEXT` | 聊天与公告 |")
    A("")
    A("### P1 —— 背包与穿戴")
    A("")
    A("`CM_PICKUP`、`CM_DROPITEM`、`CM_CONSUMEITEM`、`CM_REQUESTEQUIPWEAR/GRABWEAR`、")
    A("`CM_REQUESTEQUIPBELT/GRABBELT/CONSUMEBELT`、`CM_BUY` 及其 `SM_*` 应答与错误码。")
    A("")
    A("### P2 —— 后续扩展")
    A("")
    A("组队、行会、交易、摆摊、拍卖、好友、排行榜、任务、魔法。")
    A("")
    A("## 5. 关键枚举（客户端必须先实现）")
    A("")
    A("| 枚举 | 取值 | 用途 |")
    A("|---|---|---|")
    A("| `dir8_t` | 1=上, 2=右上, 3=右, 4=右下, 5=下, 6=左下, 7=左, 8=左上 | 8 方向 |")
    A("| `ActionType` | 1=SPAWN … 15=MINE | 动作类型 |")
    A("| `UIDType` | 1=COR, 2=MAP, 3=NPC, 4=MON, 5=PLY | uid 高 4 位 |")
    A("| `JobType` | 1=战士, 2=道士, 4=法师（位掩码） | 职业 |")
    A("")
    A("**UID 位布局**（`uidf.cpp`）：")
    A("")
    A("```")
    A("63      : 保留位")
    A("62..59  : type（4 bit）")
    A("58..35  : id（24 bit）")
    A("34..31  : peerIndex（4 bit）")
    A("30..0   : seq")
    A("```")
    A("")

    os.makedirs(os.path.dirname(out_path) or ".", exist_ok=True)
    with open(out_path, "w", encoding="utf-8") as f:
        f.write("\n".join(L))
    return len(cm_valid), len(sm_valid), len([n for n in cm_valid if n not in cmattr]), len([n for n in sm_valid if n not in smattr])


if __name__ == "__main__":
    src = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_SRC
    out = sys.argv[2] if len(sys.argv) > 2 else DEFAULT_OUT
    r = build(src, out)
    print("CM={} SM={} CM缺失={} SM缺失={} -> {}".format(*r, out))
