#!/usr/bin/env python3
"""提取 mir2x 怪物专属类的帧数表，生成 JSON。

背景：
  40 个 Client* 怪物类里，15 个重写了 getFrameGfxSeq()（帧数表），
  其余只是构造逻辑差异。帧数表是最关键的数据（决定动画怎么播），
  且量大易错，所以用脚本抽取而不是手抄。

用法：
    python tools/extract_monster_frames.py <mir2x/client/src> <输出 json>
"""
import json
import os
import re
import sys

# MOTION_* 常量值（来自 motion.hpp）
MOTION_VALUES = {
    "MOTION_MON_STAND": 35,
    "MOTION_MON_WALK": 36,
    "MOTION_MON_ATTACK0": 37,
    "MOTION_MON_HITTED": 38,
    "MOTION_MON_DIE": 39,
    "MOTION_MON_ATTACK1": 40,
    "MOTION_MON_SPELL0": 41,
    "MOTION_MON_SPELL1": 42,
    "MOTION_MON_SPAWN": 43,
    "MOTION_MON_SPECIAL": 44,
}


def strip_comments(src):
    src = re.sub(r"//[^\n]*", "", src)
    src = re.sub(r"/\*.*?\*/", "", src, flags=re.S)
    return src


def extract_class_blocks(src):
    """切出每个 class X: public Y { ... } 的块"""
    blocks = {}
    for m in re.finditer(r"class\s+(Client\w+)\s*(?::\s*public\s+(\w+))?\s*\{", src):
        name = m.group(1)
        start = m.end() - 1
        depth = 0
        i = start
        while i < len(src):
            c = src[i]
            if c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
                if depth == 0:
                    break
            i += 1
        blocks[name] = src[start:i + 1]
    return blocks


def extract_frame_table(block):
    """从类块里提取 getFrameGfxSeq 的 switch 分支

    返回 {"default": {...}, "modes": [{"cond": "...", "table": {...}}]}
    原版结构通常是：
        if(m_standMode){ switch(motion){ ... } } else { switch(motion){ ... } }
    或直接 switch(motion){ ... }
    """
    m = re.search(r"getFrameGfxSeq\s*\([^)]*\)\s*const\s*(?:override\s*)?\{", block)
    if not m:
        return None

    # 取函数体
    start = m.end() - 1
    depth = 0
    i = start
    while i < len(block):
        if block[i] == "{":
            depth += 1
        elif block[i] == "}":
            depth -= 1
            if depth == 0:
                break
        i += 1
    body = block[start:i + 1]

    result = {"default": {}, "modes": []}

    # 判断是否有 if(m_standMode) 分支
    if "m_standMode" in body:
        # 按 if / else 切成两段
        parts = re.split(r"\}\s*else\s*\{", body, maxsplit=1)
        if len(parts) == 2:
            result["modes"].append({"cond": "standMode", "table": _parse_switch(parts[0])})
            result["modes"].append({"cond": "!standMode", "table": _parse_switch("{" + parts[1])})
            return result

    result["default"] = _parse_switch(body)
    return result


def _parse_switch(text):
    """解析 switch(motion){ case X: { return {...}; } ... }

    支持：
      - 单行：  case X: return {.count = 4};
      - 多行：  case X: { return { .gfxMotionID=..., .begin=..., .count=..., .reverse=... }; }
      - 合并：  case X: case Y: return {...};
    字段：gfxLookID / gfxMotionID / begin / count / reverse
    """
    table = {}
    # 逐个 case 段处理：从 case 到下一个 case/default
    case_iter = list(re.finditer(r"case\s+(MOTION_\w+)\s*:", text))
    for idx, cm in enumerate(case_iter):
        motion = cm.group(1)
        seg_start = cm.end()
        seg_end = case_iter[idx + 1].start() if idx + 1 < len(case_iter) else len(text)
        seg = text[seg_start:seg_end]
        # 在该段内找 return { ... }（允许跨行）
        rm = re.search(r"return\s*\{(.*?)\}\s*;", seg, re.S)
        if not rm:
            continue
        body = rm.group(1)
        entry = {}
        for key, pat in (
                ("gfxLookID", r"\.gfxLookID\s*=\s*(0[Xx][0-9A-Fa-f]+|\d+)"),
                ("gfxMotionID", r"\.gfxMotionID\s*=\s*(MOTION_\w+)"),
                ("begin", r"\.begin\s*=\s*(0[Xx][0-9A-Fa-f]+|\d+)"),
                ("count", r"\.count\s*=\s*(\d+)"),
                ("reverse", r"\.reverse\s*=\s*(true|false)"),
        ):
            mm = re.search(pat, body)
            if not mm:
                continue
            v = mm.group(1)
            if key == "gfxMotionID":
                entry[key] = MOTION_VALUES.get(v, 0)
            elif key == "reverse":
                entry[key] = (v == "true")
            else:
                entry[key] = int(v, 0)
        if entry and motion in MOTION_VALUES:
            table[str(MOTION_VALUES[motion])] = entry
    return table


def main(argv):
    src_dir = argv[1] if len(argv) > 1 else "."
    out_path = argv[2] if len(argv) > 2 else "monster_frames.json"

    result = {}
    files = sorted(f for f in os.listdir(src_dir) if f.startswith("client") and f.endswith(".hpp"))
    for fn in files:
        raw = open(os.path.join(src_dir, fn), encoding="utf-8", errors="replace").read()
        src = strip_comments(raw)
        for cls, block in extract_class_blocks(src).items():
            if cls in ("ClientMonster", "ClientCreature", "ClientStandMonster", "ClientNPC"):
                continue
            ft = extract_frame_table(block)
            if ft and (ft["default"] or ft["modes"]):
                # 找出怪物名（isMonster(u8"xxx")）
                mon = re.search(r'isMonster\(\s*u8"([^"]+)"', block)
                result[cls] = {
                    "file": fn,
                    "monster_name": mon.group(1) if mon else "",
                    "frames": ft,
                }

    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(result, f, ensure_ascii=False, indent=1)

    print("提取 %d 个怪物类的帧数表 -> %s" % (len(result), out_path))
    for k, v in result.items():
        n = len(v["frames"]["default"])
        m = sum(len(x["table"]) for x in v["frames"]["modes"])
        print("  %-26s %-12s default=%d modes=%d" % (k, v["monster_name"], n, m))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
