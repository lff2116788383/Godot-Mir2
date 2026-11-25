#!/usr/bin/env python3
"""从 buffrecord.inc 提取 buff 表。

【本脚本的两个关键点】

1. **必须用顶层花括号配对扫描**，不能按 ``.name = u8"..."`` 切分。

   buffrecord.inc 里每条记录内嵌 ``.actList { { .name = u8"HP", ... } }``，
   按「任意 .name」切会把嵌套的 act 当成独立 buff ——
   57 条记录会变成几百条，而且名字全是 "HP"/"MP"。

   这与 extract_magics.py 踩过的是同一个坑（magicrecord.inc 里也有嵌套 gfxList）。

2. **先剥离注释再解析**。

   inc 里大量 ``//`` 行注释含有中文和括号，不剥离会让括号配对错位。

【产出结构】

    {
      "records": {
        "1": {
          "buffID": 1,
          "name": "治愈术",
          "favor": 1,
          "dispellable": true,
          "attrs": [{"attr": "HP", "value": 0, "percentage": 20}]
        }
      }
    }

``attrs`` 只提取 ``attributeModifier`` 那类 act ——
trigger/aura/controller/attackModifier 需要运行时机制（定时触发、范围检测），
本轮不提取，留给后续批次。
"""

import json
import os
import re
import sys

SRC = os.environ.get(
    "MIR2X_BUFFRECORD",
    "E:/Desktop/game/mir2x/common/src/buffrecord.inc",
)
OUT = os.environ.get("MIR2X_BUFF_OUT", "content/buffs.json")


def strip_comments(text):
    """去掉 // 行注释和 /* */ 块注释，保留字符串内容。"""
    out = []
    i, n = 0, len(text)
    while i < n:
        ch = text[i]
        if ch == '"':
            out.append(ch)
            i += 1
            while i < n:
                if text[i] == "\\":
                    out.append(text[i])
                    if i + 1 < n:
                        out.append(text[i + 1])
                    i += 2
                    continue
                out.append(text[i])
                if text[i] == '"':
                    i += 1
                    break
                i += 1
            continue
        if ch == "/" and i + 1 < n and text[i + 1] == "/":
            while i < n and text[i] != "\n":
                i += 1
            continue
        if ch == "/" and i + 1 < n and text[i + 1] == "*":
            i += 2
            while i + 1 < n and not (text[i] == "*" and text[i + 1] == "/"):
                i += 1
            i += 2
            continue
        out.append(ch)
        i += 1
    return "".join(out)


def skip_string(text, i):
    """从引号处跳到字符串结束之后。"""
    i += 1
    n = len(text)
    while i < n:
        if text[i] == "\\":
            i += 2
            continue
        if text[i] == '"':
            return i + 1
        i += 1
    return i


def top_level_blocks(text):
    """返回所有顶层 { ... } 块（含花括号）。

    ★ 遇到 ``{`` 就配对到它的 ``}``，然后跳过整块 ——
    内层块不会被单独返回。这与 extract_monsters.py 的写法一致。
    """
    blocks = []
    i, n = 0, len(text)
    while i < n:
        ch = text[i]
        if ch == '"':
            i = skip_string(text, i)
            continue
        if ch == "{":
            start = i
            depth = 0
            j = i
            while j < n:
                c = text[j]
                if c == '"':
                    j = skip_string(text, j)
                    continue
                if c == "{":
                    depth += 1
                elif c == "}":
                    depth -= 1
                    if depth == 0:
                        break
                j += 1
            blocks.append(text[start:j + 1])
            i = j + 1
            continue
        i += 1
    return blocks


def find_int(pattern, block, default=0):
    m = re.search(pattern, block)
    if not m:
        return default
    try:
        return int(m.group(1))
    except (ValueError, IndexError):
        return default


def find_str(pattern, block, default=""):
    m = re.search(pattern, block)
    return m.group(1) if m else default


def parse_acts(block):
    """提取 actList 里的三类 act：attributeModifier / trigger / aura。

    返回 dict：
      attrs     [{"attr","value","percentage"}]
      triggers  [{"name","duration","on","tps","value","percentage"}]
      auras     [{"name","radius","lingerIn","lingerOut"}]
      attackMods[{"name","prob"}]

    【依据】buffrecord.hpp:215-260 BuffActRecordRef：

        .attributeModifier { .arg }           属性修正
        .trigger { .on, .tps, .arg }          定时/事件触发
        .aura    { .radius, .linger{in,out} } 光环
        .attackModifier { .prob }             攻击概率触发

    ★ trigger 的 .arg 语义容易搞错：

        .arg = buffArgWrapper<BuffValuePercentage>(20, 5)

    第一个数是**固定值**，第二个是**百分比**。而 attributeModifier 里
    单个参数时是百分比（见「治愈术」的 .arg = (20) → +20%）。

    两种 act 的参数个数不同，共用一个解析函数会算错。
    """
    out = {"attrs": [], "triggers": [], "auras": [], "attackMods": []}
    m = re.search(r"\.actList\s*\{", block)
    if not m:
        return out

    start = block.index("{", m.start())
    depth, j, n = 0, start, len(block)
    while j < n:
        c = block[j]
        if c == '"':
            j = skip_string(block, j)
            continue
        if c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                break
        j += 1
    body = block[start + 1:j]

    for act in top_level_blocks(body):
        name = find_str(r'\.name\s*=\s*u8"([^"]*)"', act)
        if not name:
            continue

        if "attributeModifier" in act:
            args = re.search(r"buffArgWrapper<\s*\w+\s*>\s*\(([^)]*)\)", act)
            value, percentage = 0, 0
            if args:
                parts = [p.strip() for p in args.group(1).split(",") if p.strip()]
                try:
                    if len(parts) == 1:
                        percentage = int(parts[0])
                    elif len(parts) >= 2:
                        value, percentage = int(parts[0]), int(parts[1])
                except ValueError:
                    pass
            out["attrs"].append({"attr": name, "value": value, "percentage": percentage})

        if ".trigger" in act or re.search(r"\btrigger\b", act):
            args = re.search(r"buffArgWrapper<\s*\w+\s*>\s*\(([^)]*)\)", act)
            value, percentage = 0, 0
            if args:
                parts = [p.strip() for p in args.group(1).split(",") if p.strip()]
                try:
                    if len(parts) == 1:
                        value = int(parts[0])
                    elif len(parts) >= 2:
                        value, percentage = int(parts[0]), int(parts[1])
                except ValueError:
                    pass
            out["triggers"].append({
                "name": name,
                "duration": find_int(r"\.duration\s*=\s*(-?\d+)", act),
                "on": find_str(r"\.on\s*=\s*(\w+)", act, "BATGR_NONE"),
                "tps": find_int(r"\.tps\s*=\s*(-?\d+)", act),
                "value": value,
                "percentage": percentage,
            })

        if re.search(r"\baura\b", act):
            out["auras"].append({
                "name": name,
                "radius": find_int(r"\.radius\s*=\s*(-?\d+)", act),
                "lingerIn": find_int(r"\.in\s*=\s*(-?\d+)", act),
                "lingerOut": find_int(r"\.out\s*=\s*(-?\d+)", act),
            })

        if "attackModifier" in act:
            out["attackMods"].append({
                "name": name,
                "prob": find_int(r"\.prob\s*=\s*(-?\d+)", act),
            })
    return out


def parse_attrs(block):
    """提取 actList 里所有 attributeModifier 的 (attr, value, percentage)。

    【依据】buffrecord.hpp:248-256 + BuffValuePercentage（:34-43）：

        .attributeModifier { .arg = buffArgWrapper<BuffValuePercentage>(20), }

    括号里可能是一个数（只有百分比）或两个数（固定值, 百分比）。
    """
    attrs = []
    # 找 .actList { ... } 的内部
    m = re.search(r"\.actList\s*\{", block)
    if not m:
        return attrs

    # 从 actList 的 { 开始配对
    start = block.index("{", m.start())
    depth = 0
    j = start
    n = len(block)
    while j < n:
        c = block[j]
        if c == '"':
            j = skip_string(block, j)
            continue
        if c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                break
        j += 1
    body = block[start + 1:j]

    for act in top_level_blocks(body):
        if "attributeModifier" not in act:
            continue
        name = find_str(r'\.name\s*=\s*u8"([^"]*)"', act)
        if not name:
            continue
        # buffArgWrapper<BuffValuePercentage>(20) 或 (1, 20)
        args = re.search(
            r"buffArgWrapper<\s*\w+\s*>\s*\(([^)]*)\)", act)
        value, percentage = 0, 0
        if args:
            parts = [p.strip() for p in args.group(1).split(",") if p.strip()]
            try:
                if len(parts) == 1:
                    percentage = int(parts[0])
                elif len(parts) >= 2:
                    value = int(parts[0])
                    percentage = int(parts[1])
            except ValueError:
                pass
        attrs.append({"attr": name, "value": value, "percentage": percentage})
    return attrs


def main():
    if not os.path.exists(SRC):
        print(f"找不到 {SRC}，请设 MIR2X_BUFFRECORD", file=sys.stderr)
        sys.exit(1)

    raw = open(SRC, encoding="utf-8", errors="replace").read()
    text = strip_comments(raw)
    blocks = top_level_blocks(text)

    records = {}
    for idx, block in enumerate(blocks):
        name = find_str(r'\.name\s*=\s*u8"([^"]*)"', block)
        if not name:
            continue  # 第 0 块是空占位 {}

        fav = find_str(r"\.favor\s*=\s*(-?\d+)", block, "0")
        records[str(idx)] = {
            "buffID": idx,
            "name": name,
            "favor": int(fav) if fav.lstrip("-").isdigit() else 0,
            "dispellable": find_int(r"\.dispellable\s*=\s*(-?\d+)", block) != 0,
            "stackCount": find_int(r"\.stackCount\s*=\s*(-?\d+)", block),
            **parse_acts(block),
        }

    # buffrecord.inc 里 buffID 从 1 开始（第 0 块是空占位）
    payload = {
        "source": "buffrecord.inc",
        "count": len(records),
        "records": records,
    }
    os.makedirs(os.path.dirname(OUT) or ".", exist_ok=True)
    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)

    with_attrs = sum(1 for r in records.values() if r["attrs"])
    with_trig = sum(1 for r in records.values() if r["triggers"])
    with_aura = sum(1 for r in records.values() if r["auras"])
    with_atk = sum(1 for r in records.values() if r["attackMods"])
    print(f"buff {len(records)} 条：属性修正 {with_attrs} / 触发 {with_trig} / 光环 {with_aura} / 攻击修正 {with_atk}")
    for rec in list(records.values())[:4]:
        print(f"  {rec['buffID']:3d} {rec['name']:<10s}")
        if rec["attrs"]:
            print(f"      attrs={rec['attrs']}")
        if rec["triggers"]:
            print(f"      triggers={rec['triggers']}")
        if rec["auras"]:
            print(f"      auras={rec['auras']}")


if __name__ == "__main__":
    main()
