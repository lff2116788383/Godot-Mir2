#!/usr/bin/env python3
"""从 itemrecord.inc 提取服务端物品表。

输出 content/items.json，字段：
  name, type, weight, price, pkgGfxID, shape,
  potion{hp,mp,time}, dope{hp,mp,dc,mc,sc,speed,time},
  book{job,level,magic}

只提取服务端逻辑需要的字段，装备属性（dc/ac/mc 等）留给后续批次 ——
本批的穿戴校验只看 type（wearable/beltable 全靠 type 字符串匹配）。

索引即 DBCOM_ITEMID：原版靠 .inc 的数组下标定位物品，
第 0 项是 `{}`（空占位），所以有效 itemID 从 1 开始。
"""
import json
import os
import re
import sys

SRC = os.environ.get(
    "MIR2X_ITEMRECORD",
    "E:/Desktop/game/mir2x/common/src/itemrecord.inc",
)
OUT = os.path.join(os.path.dirname(__file__), "..", "..", "content", "items.json")


def parse_records(text):
    """把 .inc 按 '{  .name = u8"..."' 切分成记录块。

    不能按行解析：字段跨多行、有嵌套的 potion{...} / book{...} 块。
    这里用「遇到 `{` 且紧跟 `.name`」作为记录起点，靠花括号计数找结尾。
    """
    records = []
    i = 0
    n = len(text)
    while i < n:
        # 找记录起点
        m = re.compile(r'\{\s*\.name\s*=\s*u8"').search(text, i)
        if not m:
            break
        start = m.start()
        # 从起点做花括号配对
        depth = 0
        j = start
        while j < n:
            ch = text[j]
            if ch == '"':
                # 跳过字符串
                j += 1
                while j < n:
                    if text[j] == '\\':
                        j += 2
                        continue
                    if text[j] == '"':
                        break
                    j += 1
            elif ch == '{':
                depth += 1
            elif ch == '}':
                depth -= 1
                if depth == 0:
                    break
            j += 1
        records.append(text[start:j + 1])
        i = j + 1
    return records


def first_match(pattern, block, default=None):
    m = re.search(pattern, block)
    return m.group(1) if m else default


def parse_int(pattern, block, default=0):
    v = first_match(pattern, block)
    if v is None:
        return default
    try:
        return int(v)
    except ValueError:
        return default


def parse_block(name, block):
    rec = {
        "name": first_match(r'\.name\s*=\s*u8"([^"]*)"', block, ""),
        "type": first_match(r'\.type\s*=\s*u8"([^"]*)"', block, ""),
    }
    if not rec["name"]:
        return None

    rec["weight"] = parse_int(r'\.weight\s*=\s*(-?\d+)', block)
    rec["pkgGfxID"] = parse_int(r'\.pkgGfxID\s*=\s*(0[xX][0-9A-Fa-f]+|-?\d+)', block)
    rec["price"] = parse_int(r'\.price\s*=\s*(-?\d+)', block)
    rec["shape"] = parse_int(r'\.shape\s*=\s*(-?\d+)', block)

    # pkgGfxID 可能是 0X0078 这种十六进制
    raw_gfx = first_match(r'\.pkgGfxID\s*=\s*(0[xX][0-9A-Fa-f]+|-?\d+)', block)
    if raw_gfx:
        try:
            rec["pkgGfxID"] = int(raw_gfx, 0)
        except ValueError:
            rec["pkgGfxID"] = 0

    # potion { .hp = 30, .mp = 0, .time = 1 }
    pm = re.search(r'\.potion\s*\{(.*?)\}', block, re.S)
    if pm:
        body = pm.group(1)
        rec["potion"] = {
            "hp": parse_int(r'\.hp\s*=\s*(-?\d+)', body),
            "mp": parse_int(r'\.mp\s*=\s*(-?\d+)', body),
            "time": parse_int(r'\.time\s*=\s*(-?\d+)', body),
        }

    # dope { ... }
    dm = re.search(r'\.dope\s*\{(.*?)\}', block, re.S)
    if dm:
        body = dm.group(1)
        rec["dope"] = {
            "hp": parse_int(r'\.hp\s*=\s*(-?\d+)', body),
            "mp": parse_int(r'\.mp\s*=\s*(-?\d+)', body),
            "dc": parse_int(r'\.dc\s*=\s*(-?\d+)', body),
            "mc": parse_int(r'\.mc\s*=\s*(-?\d+)', body),
            "sc": parse_int(r'\.sc\s*=\s*(-?\d+)', body),
            "speed": parse_int(r'\.speed\s*=\s*(-?\d+)', body),
            "time": parse_int(r'\.time\s*=\s*(-?\d+)', body),
        }

    # book { .job = u8"战士", .level = 7, .magic = u8"..." }
    bm = re.search(r'\.book\s*\{(.*?)\n\s*\}', block, re.S)
    if bm:
        body = bm.group(1)
        rec["book"] = {
            "job": first_match(r'\.job\s*=\s*u8"([^"]*)"', body, ""),
            "level": parse_int(r'\.level\s*=\s*(-?\d+)', body),
            "magic": first_match(r'\.magic\s*=\s*u8"([^"]*)"', body, ""),
        }

    return rec


def main():
    if not os.path.exists(SRC):
        print(f"source not found: {SRC}", file=sys.stderr)
        return 1

    with open(SRC, encoding="utf-8", errors="replace") as fh:
        text = fh.read()

    blocks = parse_records(text)
    print(f"扫描到 {len(blocks)} 个记录块")

    # 原版数组第 0 项是 `{}`，index 即 itemID。
    # 我们按「出现顺序」建立 1..N 的索引：
    # 第 k 个记录块对应 itemID = k（因为第 0 项是空占位）。
    records = {}
    skipped = 0
    for idx, block in enumerate(blocks, start=1):
        rec = parse_block(f"item{idx}", block)
        if rec is None:
            skipped += 1
            continue
        rec["itemID"] = idx
        records[str(idx)] = rec

    out = {
        "_meta": {
            "source": SRC,
            "count": len(records),
            "note": "itemID = .inc 数组下标（第 0 项为空占位）",
        },
        "records": records,
    }
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with open(OUT, "w", encoding="utf-8") as fh:
        json.dump(out, fh, ensure_ascii=False, indent=1, sort_keys=True)

    print(f"写出 {len(records)} 条（跳过 {skipped}）-> {OUT}")

    # 抽样校验
    for probe in ["金创药（小）", "金币（小）"]:
        for k, v in records.items():
            if v["name"] == probe:
                print(f"  校验 {probe} -> itemID={k} type={v['type']} potion={v.get('potion')}")
                break
    types = {}
    for v in records.values():
        types[v["type"]] = types.get(v["type"], 0) + 1
    print("  type 分布前 15:")
    for t, c in sorted(types.items(), key=lambda x: -x[1])[:15]:
        print(f"    {t or '(空)'}: {c}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
