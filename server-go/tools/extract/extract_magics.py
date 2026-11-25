#!/usr/bin/env python3
"""从 magicrecord.inc 提取服务端魔法表。

输出 content/magics.json，字段：
  name, elem, req{level[3], train[3], job, prior},
  castRange{type, distance}, castDelay, coolDown, checkGround,
  mp, mpInc, power[2], addPower[2], seffBase, learnable

索引即 DBCOM_MAGICID：原版靠 .inc 的顶层数组下标定位魔法，
第 0 项是 `{}`（空占位），所以有效 magicID 从 1 开始。

★ 必须按「顶层块」切分，不能套用 extract_items.py 的做法：
  magicrecord.inc 的每条记录里都嵌套 gfxList{ {.ref{ .name=... }} }，
  按「任意出现 .name」切分会把嵌套块也当成记录，索引整体错位。
  这里改成花括号配对的顶层扫描，只有 depth 从 0 变 1 的那一层才算记录。

learnable 字段是本脚本的派生结论，不是 inc 里的原文：
  只有带 req.job 的条目才是玩家能学的真技能；其余的 .name 要么是
  「魔法特效_*」这类给 gfxList 引用的共享特效，要么是怪物的专属魔法
  （如「诺玛法老_火球术」），都不能进玩家技能表。
  之前客户端建 magic_db 时踩过：把特效条目当技能，技能面板出现空白图标。
"""
import json
import os
import re
import sys

SRC = os.environ.get(
    "MIR2X_MAGICRECORD",
    "E:/Desktop/game/mir2x/common/src/magicrecord.inc",
)
OUT = os.path.join(os.path.dirname(__file__), "..", "..", "content", "magics.json")

# magicrecord.hpp:38 magicElemID —— 中文名到 MET_* 的映射
ELEM_ID = {
    "火": "fire",
    "冰": "ice",
    "雷": "light",
    "风": "wind",
    "神圣": "holy",
    "暗黑": "dark",
    "幻影": "phantom",
}

# magicrecord.hpp:256-261 DCCastRangeType
CAST_RANGE_ID = {
    "CRT_NONE": 0,
    "CRT_DIR": 1,
    "CRT_LONG": 2,
    "CRT_LIMITED": 3,
}


def top_level_blocks(text):
    """按花括号配对切出顶层记录块。

    只有在 depth 从 0 变 1 时才开始一段新记录；
    字符串内部的花括号要跳过，否则 u8"{...}" 这种字面量会打乱计数。
    """
    blocks = []
    i = 0
    n = len(text)
    while i < n:
        ch = text[i]
        if ch == '"':
            i = skip_string(text, i)
            continue
        if ch == '{':
            start = i
            depth = 0
            j = i
            while j < n:
                c = text[j]
                if c == '"':
                    j = skip_string(text, j)
                    continue
                if c == '{':
                    depth += 1
                elif c == '}':
                    depth -= 1
                    if depth == 0:
                        break
                j += 1
            blocks.append(text[start:j + 1])
            i = j + 1
            continue
        i += 1
    return blocks


def skip_string(text, i):
    """从引号位置跳到字符串结束后一位。"""
    n = len(text)
    i += 1
    while i < n:
        if text[i] == '\\':
            i += 2
            continue
        if text[i] == '"':
            return i + 1
        i += 1
    return i


def strip_comments(block):
    """去掉 // 行注释。

    inc 里大量 `.name = u8"物理攻击", // TODO ...` 与
    `// used by 地狱火, 火墙` 这类注释，直接正则匹配会把注释里的
    u8"..." 也当成字段值。注释只在字符串外才算注释。
    """
    out = []
    i = 0
    n = len(block)
    while i < n:
        ch = block[i]
        if ch == '"':
            j = skip_string(block, i)
            out.append(block[i:j])
            i = j
            continue
        if ch == '/' and i + 1 < n and block[i + 1] == '/':
            while i < n and block[i] != '\n':
                i += 1
            continue
        out.append(ch)
        i += 1
    return "".join(out)


def find_str(pattern, block, default=""):
    m = re.search(pattern, block)
    return m.group(1) if m else default


def find_int(pattern, block, default=0):
    m = re.search(pattern, block)
    if not m:
        return default
    try:
        return int(m.group(1), 0)
    except ValueError:
        return default


def find_int_list(pattern, block, count, default=None):
    """解析 `.power = {15, 19}` 这种定长数组。"""
    if default is None:
        default = [0] * count
    m = re.search(pattern, block)
    if not m:
        return list(default)
    parts = [p.strip() for p in m.group(1).split(",") if p.strip()]
    values = []
    for i in range(count):
        if i < len(parts):
            try:
                values.append(int(parts[i], 0))
            except ValueError:
                values.append(0)
        else:
            values.append(0)
    return values


def find_nested(pattern, block):
    """取 `.<field> { ... }` 的块体（花括号配对，容忍嵌套）。"""
    m = re.search(pattern, block)
    if not m:
        return ""
    start = block.index("{", m.start())
    depth = 0
    j = start
    while j < len(block):
        if block[j] == '"':
            j = skip_string(block, j)
            continue
        if block[j] == '{':
            depth += 1
        elif block[j] == '}':
            depth -= 1
            if depth == 0:
                return block[start + 1:j]
        j += 1
    return ""


def parse_block(block):
    rec = {
        "name": find_str(r'\.name\s*=\s*u8"([^"]*)"', block, ""),
    }
    if not rec["name"]:
        return None

    elem = find_str(r'\.elem\s*=\s*u8"([^"]*)"', block, "")
    rec["elem"] = elem
    rec["elemID"] = ELEM_ID.get(elem, "")

    req = find_nested(r'\.req\s*\{', block)
    rec["req"] = {
        "level": find_int_list(r'\.level\s*=\s*\{([^}]*)\}', req, 3),
        "train": find_int_list(r'\.train\s*=\s*\{([^}]*)\}', req, 3),
        "job": find_str(r'\.job\s*=\s*u8"([^"]*)"', req, ""),
        "prior": find_str(r'\.prior\s*=\s*u8"([^"]*)"', req, ""),
    }

    # ★ castRange 的默认值不是 CRT_NONE。
    #   magicrecord.hpp:288 `const DCCastRange castRange { .type = CRT_LONG };`
    #   也就是说没写 castRange 的条目一律是「无距离限制的远程」，
    #   距离默认 0。若按 CRT_NONE 处理，雷电术/火球术这类主力技能
    #   会被判成「施法距离非法」而永远放不出来。
    cast = find_nested(r'\.castRange\s*\{', block)
    cast_type = find_str(r'\.type\s*=\s*(CRT_\w+)', cast, "CRT_LONG")
    rec["castRange"] = {
        "type": cast_type,
        "typeID": CAST_RANGE_ID.get(cast_type, 2),
        "distance": find_int(r'\.distance\s*=\s*(-?\d+)', cast),
    }

    rec["castDelay"] = find_int(r'\.castDelay\s*=\s*(-?\d+)', block)
    rec["coolDown"] = find_int(r'\.coolDown\s*=\s*(-?\d+)', block)
    rec["checkGround"] = find_int(r'\.checkGround\s*=\s*(-?\d+)', block)
    # ★ 不能写成 `\b\.mp`：\b 在这里要求「单词字符与点之间」的词边界，
    #   而 .mp 前面永远是空白/换行，匹配必然失败，结果全表 mp=0，
    #   施法就变成「永远不耗蓝」。
    rec["mp"] = find_int(r'\.mp\s*=\s*(-?\d+)', block)
    rec["mpInc"] = find_int(r'\.mpInc\s*=\s*(-?\d+)', block)
    rec["power"] = find_int_list(r'\.power\s*=\s*\{([^}]*)\}', block, 2)
    rec["addPower"] = find_int_list(r'\.addPower\s*=\s*\{([^}]*)\}', block, 2)

    # seffBase 是 optional<int>，没写就是 null
    seff_raw = re.search(r'\.seffBase\s*=\s*(-?\d+)', block)
    rec["seffBase"] = int(seff_raw.group(1)) if seff_raw else None

    # 派生结论：带 req.job 的才是玩家可学技能
    rec["learnable"] = bool(rec["req"]["job"])
    return rec


def main():
    if not os.path.exists(SRC):
        print(f"source not found: {SRC}", file=sys.stderr)
        return 1

    with open(SRC, encoding="utf-8", errors="replace") as fh:
        text = fh.read()

    blocks = top_level_blocks(text)
    print(f"扫描到 {len(blocks)} 个顶层块")

    records = {}
    # ★ 下标从 0 起算：.inc 的第 0 个顶层块就是空占位 `{}`，
    #   它的数组下标是 0，所以「块序号 == magicID」。
    #   初版写成 start=1，整表 magicID 全体偏大 1 —— 物理攻击变成 2、
    #   雷电术变成 15，客户端按 14 发过来的魔法查不到记录。
    for idx, raw in enumerate(blocks):
        rec = parse_block(strip_comments(raw))
        if rec is None:
            continue
        rec["magicID"] = idx
        records[str(idx)] = rec

    learnable = {k: v for k, v in records.items() if v["learnable"]}
    out = {
        "_meta": {
            "source": SRC,
            "count": len(records),
            "learnableCount": len(learnable),
            "note": "magicID = .inc 顶层数组下标（第 0 项为空占位）",
        },
        "records": records,
    }
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with open(OUT, "w", encoding="utf-8") as fh:
        json.dump(out, fh, ensure_ascii=False, indent=1, sort_keys=True)

    print(f"写出 {len(records)} 条（可学技能 {len(learnable)}）-> {OUT}")

    for probe in ["物理攻击", "雷电术", "火球术", "召唤骷髅"]:
        for k, v in records.items():
            if v["name"] == probe:
                print(f"  校验 {probe}: id={k} elem={v['elemID']} mp={v['mp']} "
                      f"cd={v['coolDown']} power={v['power']} add={v['addPower']} "
                      f"range={v['castRange']['type']}x{v['castRange']['distance']} "
                      f"req={v['req']['level']}/{v['req']['job']}")
                break

    jobs = {}
    for v in learnable.values():
        jobs[v["req"]["job"]] = jobs.get(v["req"]["job"], 0) + 1
    print("  可学技能按职业:", jobs)
    return 0


if __name__ == "__main__":
    sys.exit(main())
