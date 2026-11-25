#!/usr/bin/env python3
"""从 monsterrecord.inc 提取服务端怪物表。

输出 content/monsters.json，字段对应 MonsterRecord（server/src/monster.cpp）：
  lookID / hp / mp / exp / dc / ac / mc / mac / dcHit / mcHit / dcDodge / mcDodge
  view / walkWait / walkStep / attackWait / behaveMode / acElem / dropItemList

索引即 DBCOM_MONSTERID：第 0 项是空占位 `{}`，块序号即 ID。

★ 与 extract_magics.py 同样必须按「顶层块」切分：
  monsterrecord.inc 里每条都有嵌套的 acElem{} 与 dropItemList{ {{...}} }，
  按「任意 .name」切分会把嵌套结构当记录，索引整体错位。
"""
import json
import os
import re
import sys

SRC = os.environ.get(
    "MIR2X_MONSTERRECORD",
    "E:/Desktop/game/mir2x/common/src/monsterrecord.inc",
)
OUT = os.path.join(os.path.dirname(__file__), "..", "..", "content", "monsters.json")


def skip_string(text, i):
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


def top_level_blocks(text):
    blocks = []
    i, n = 0, len(text)
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


def strip_comments(block):
    out = []
    i, n = 0, len(block)
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


def find_nested(pattern, block):
    """取 `.<field> { ... }` 的块体（花括号配对）。"""
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


def find_pair(pattern, block, default=(0, 0)):
    m = re.search(pattern, block)
    if not m:
        return list(default)
    parts = [p.strip() for p in m.group(1).split(",") if p.strip()]
    out = []
    for i in range(2):
        if i < len(parts):
            try:
                out.append(int(parts[i], 0))
            except ValueError:
                out.append(0)
        else:
            out.append(0)
    return out


# dropItemList 结构（monsterrecord.hpp:104-109）：
#
#   {                                     // 外层：互斥组（按顺序 roll，命中一个就退出组）
#       { .item = u8"药", .probRecip = 1, .count = 1, .repeat = 1 },
#       { .item = u8"钱", .probRecip = 2, .count = 1 },
#   }
#
# 注意每个内层项也可能带嵌套，用花括号配对逐项解析。
def strip_outer_braces(block):
    """去掉最外层一对花括号，只留内部内容。

    ★ 为什么需要这一步：
        top_level_blocks 的语义是「把每个完整的顶层块作为一个元素」，
        所以 top_level_blocks("{ {a}, {b} }") 返回 **1 个**元素（整块），
        而不是 2 个。要拿到内部的 {a} 和 {b}，必须先剥掉外层括号。
    """
    block = block.strip()
    if not (block.startswith("{") and block.endswith("}")):
        return block
    return block[1:-1]


def parse_drop_list(block):
    body = find_nested(r'\.dropItemList\s*\{', block)
    if not body:
        return []
    groups = []
    for group_block in top_level_blocks(body):
        group_block = strip_outer_braces(group_block)
        items = []
        for item_block in top_level_blocks(group_block):
            item = find_str(r'\.item\s*=\s*u8"([^"]*)"', item_block, "")
            if not item:
                continue
            items.append({
                "item": item,
                "probRecip": find_int(r'\.probRecip\s*=\s*(-?\d+)', item_block, 1),
                "count": find_int(r'\.count\s*=\s*(-?\d+)', item_block, 1),
                "repeat": find_int(r'\.repeat\s*=\s*(-?\d+)', item_block, 1),
            })
        if items:
            groups.append(items)
    return groups


def parse_block(block):
    name = find_str(r'\.name\s*=\s*u8"([^"]*)"', block, "")
    if not name:
        return None

    rec = {
        "name": name,
        "lookID": find_int(r'\.lookID\s*=\s*(-?[\dXx]+)', block),
        # ★ 不能写 r'\b\.hp'：`.` 不是单词字符，`\b` 在「空白与点」
        #   之间不成立，整条正则永远不匹配 → 全表 hp=0。
        #   上一轮 extract_magics.py 的 mp 字段就是踩了同一个坑。
        "hp": find_int(r'\.hp\s*=\s*(-?\d+)', block),
        "mp": find_int(r'\.mp\s*=\s*(-?\d+)', block),
        "exp": find_int(r'\.exp\s*=\s*(-?\d+)', block),
        "dc": find_pair(r'\.dc\s*=\s*\{([^}]*)\}', block),
        "mc": find_pair(r'\.mc\s*=\s*\{([^}]*)\}', block),
        "ac": find_pair(r'\.ac\s*=\s*\{([^}]*)\}', block),
        "mac": find_pair(r'\.mac\s*=\s*\{([^}]*)\}', block),
        "dcHit": find_int(r'\.dcHit\s*=\s*(-?\d+)', block),
        "mcHit": find_int(r'\.mcHit\s*=\s*(-?\d+)', block),
        "dcDodge": find_int(r'\.dcDodge\s*=\s*(-?\d+)', block),
        "mcDodge": find_int(r'\.mcDodge\s*=\s*(-?\d+)', block),
        "view": find_int(r'\.view\s*=\s*(-?\d+)', block),
        "walkWait": find_int(r'\.walkWait\s*=\s*(-?\d+)', block),
        "walkStep": find_int(r'\.walkStep\s*=\s*(-?\d+)', block),
        "attackWait": find_int(r'\.attackWait\s*=\s*(-?\d+)', block),
    }

    # acElem 是元素抗性（负值 = 弱点）
    elem_body = find_nested(r'\.acElem\s*\{', block)
    if elem_body:
        rec["acElem"] = {
            k: find_int(r'\.%s\s*=\s*(-?\d+)' % k, elem_body)
            for k in ("fire", "ice", "light", "wind", "holy", "dark", "phantom")
        }

    rec["dropItemList"] = parse_drop_list(block)
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
    for idx, raw in enumerate(blocks):
        rec = parse_block(strip_comments(raw))
        if rec is None:
            continue
        rec["monsterID"] = idx
        records[str(idx)] = rec

    out = {
        "_meta": {
            "source": SRC,
            "count": len(records),
            "note": "monsterID = .inc 顶层数组下标（第 0 项为空占位）",
        },
        "records": records,
    }
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with open(OUT, "w", encoding="utf-8") as fh:
        json.dump(out, fh, ensure_ascii=False, indent=1, sort_keys=True)

    print(f"写出 {len(records)} 条 -> {OUT}")

    for probe in ["半兽人", "鸡", "沙漠蜥蜴", "大刀卫士"]:
        for k, v in records.items():
            if v["name"] == probe:
                print(f"  校验 {probe}: id={k} hp={v['hp']} exp={v['exp']} "
                      f"dc={v['dc']} ac={v['ac']} view={v['view']} "
                      f"drops={len(v['dropItemList'])}")
                break

    dropCnt = sum(1 for v in records.values() if v["dropItemList"])
    print(f"  有掉落的怪物: {dropCnt}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
