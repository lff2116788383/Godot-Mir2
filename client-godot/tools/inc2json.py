#!/usr/bin/env python3
"""把 mir2x 的 C++ 数据表（.inc）解析为 JSON。

设计要点：
  1. .inc 是 C++ 指定初始化器语法（`.name = u8"xx"`），不是标准 JSON
  2. 用「括号配对 + 字段切分」的轻量解析器，不依赖 C++ 编译器
  3. 只翻译数据，不做业务判断（判断逻辑留在 GDScript 侧）

用法：
    python tools/inc2json.py <输入.inc> <输出.json>
    python tools/inc2json.py --all <mir2x/common/src> <输出目录>

支持语法：
    { .k = v, .k { .k2 = v2 }, }
    u8"中文"、0X00FF、true/false、负数
"""
import json
import os
import re
import sys

# ---------------- 词法预处理 ----------------


def strip_comments(src: str) -> str:
    """去掉 // 注释与 /* */ 注释，保留字符串内的内容。"""
    out = []
    i = 0
    n = len(src)
    while i < n:
        c = src[i]
        if c == '"' or c == "'":
            q = c
            out.append(c)
            i += 1
            while i < n:
                if src[i] == "\\" and i + 1 < n:
                    out.append(src[i:i + 2])
                    i += 2
                    continue
                out.append(src[i])
                if src[i] == q:
                    i += 1
                    break
                i += 1
            continue
        if c == "/" and i + 1 < n and src[i + 1] == "/":
            while i < n and src[i] != "\n":
                i += 1
            continue
        if c == "/" and i + 1 < n and src[i + 1] == "*":
            i += 2
            while i + 1 < n and not (src[i] == "*" and src[i + 1] == "/"):
                i += 1
            i += 2
            continue
        out.append(c)
        i += 1
    return "".join(out)


def split_top_level(s: str, seps=",") -> list:
    """按顶层分隔符切分，忽略括号/花括号/引号内部的。"""
    parts = []
    buf = []
    depth = 0
    in_str = False
    q = ""
    i = 0
    while i < len(s):
        c = s[i]
        if in_str:
            buf.append(c)
            if c == "\\" and i + 1 < len(s):
                buf.append(s[i + 1])
                i += 2
                continue
            if c == q:
                in_str = False
            i += 1
            continue
        if c in "\"'":
            in_str = True
            q = c
            buf.append(c)
            i += 1
            continue
        if c in "{[(":
            depth += 1
            buf.append(c)
            i += 1
            continue
        if c in "}])":
            depth -= 1
            buf.append(c)
            i += 1
            continue
        if c in seps and depth == 0:
            parts.append("".join(buf).strip())
            buf = []
            i += 1
            continue
        buf.append(c)
        i += 1
    tail = "".join(buf).strip()
    if tail:
        parts.append(tail)
    return parts


# ---------------- 值解析 ----------------


def parse_scalar(v: str):
    v = v.strip()
    if not v:
        return None
    # u8"..." / "..." / u8'x'
    m = re.match(r'^(?:u8|u|U|L)?(["\'])(.*)\1$', v, re.S)
    if m:
        body = m.group(2)
        body = body.encode("utf-8").decode("unicode_escape").encode("latin1", "ignore").decode("utf-8", "ignore") \
            if "\\" in body else body
        return body
    low = v.lower()
    if low == "true":
        return True
    if low == "false":
        return False
    if low in ("nullptr", "null"):
        return None
    # 十六进制 / 十进制整数
    try:
        if low.startswith("0x") or low.startswith("0X"):
            return int(v, 16)
        if re.match(r"^-?\d+$", v):
            return int(v)
        if re.match(r"^-?\d*\.\d+$", v):
            return float(v)
    except ValueError:
        pass
    return v.strip()


def parse_block(s: str) -> dict:
    """解析 { .k = v, .k2 { ... }, } 为 dict。

    若块内全是无名子块（数组），返回 {"_array": [...]}。
    """
    s = s.strip()
    if s.startswith("{"):
        s = s[1:]
    if s.endswith("}"):
        s = s[:-1]

    # 先判断是否为纯无名块数组
    parts = split_top_level(s, ",")
    non_empty = [x.strip() for x in parts if x.strip()]
    if non_empty and all(x.startswith("{") and x.endswith("}") for x in non_empty):
        return {"_array": [parse_block(x) for x in non_empty]}

    result = {}
    for item in split_top_level(s, ","):
        item = item.strip()
        if not item:
            continue
        # .key { ... }  （嵌套块）
        m = re.match(r"^\.(\w+)\s*(\{.*\})$", item, re.S)
        if m:
            result[m.group(1)] = parse_block(m.group(2))
            continue
        # .key = value
        m = re.match(r"^\.(\w+)\s*=\s*(.*)$", item, re.S)
        if m:
            key, val = m.group(1), m.group(2).strip()
            if val.startswith("{"):
                result[key] = parse_block(val)
            else:
                result[key] = parse_scalar(val)
            continue
        # 无名块：数组元素，形如 {.x = 1, .y = 2}
        if item.startswith("{") and item.endswith("}"):
            result.setdefault("_list", []).append(parse_block(item))
            continue
    if "_list" in result and len(result) == 1:
        return {"_array": result["_list"]}
    return result


def parse_inc(path: str) -> list:
    """解析整个 .inc：顶层是一串 { ... }, 元素，下标即 ID。"""
    src = open(path, encoding="utf-8", errors="replace").read()
    src = strip_comments(src)
    # 去掉 #pragma / #include 之类预处理行
    src = "\n".join(l for l in src.splitlines() if not l.strip().startswith("#"))
    entries = []
    i = 0
    n = len(src)
    while i < n:
        if src[i] == "{":
            depth = 0
            start = i
            in_str = False
            q = ""
            while i < n:
                c = src[i]
                if in_str:
                    if c == "\\":
                        i += 2
                        continue
                    if c == q:
                        in_str = False
                    i += 1
                    continue
                if c in "\"'":
                    in_str = True
                    q = c
                    i += 1
                    continue
                if c == "{":
                    depth += 1
                elif c == "}":
                    depth -= 1
                    if depth == 0:
                        i += 1
                        break
                i += 1
            block = src[start:i]
            entries.append(parse_block(block))
        else:
            i += 1
    return entries


def convert(inc_path: str, out_path: str, id_offset: int = 0) -> dict:
    entries = parse_inc(inc_path)
    # 第一个通常是空 {}，代表 ID 0 占位，保留以维持下标对齐
    records = {}
    for idx, e in enumerate(entries):
        if not e:
            continue
        records[str(idx + id_offset)] = e
    meta = {
        "_source": os.path.basename(inc_path),
        "_total_slots": len(entries),
        "_valid_records": len(records),
    }
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump({"_meta": meta, "records": records}, f, ensure_ascii=False, indent=1)
    return meta


def main(argv):
    if len(argv) >= 2 and argv[1] == "--all":
        src_dir = argv[2] if len(argv) > 2 else "."
        out_dir = argv[3] if len(argv) > 3 else "."
        os.makedirs(out_dir, exist_ok=True)
        targets = [
            "itemrecord.inc",
            "monsterrecord.inc",
            "maprecord.inc",
            "magicrecord.inc",
            "buffrecord.inc",
        ]
        for t in targets:
            p = os.path.join(src_dir, t)
            if not os.path.exists(p):
                print("skip (not found):", t)
                continue
            out = os.path.join(out_dir, t.replace(".inc", ".json"))
            meta = convert(p, out)
            print("%-22s slots=%-7d valid=%-7d -> %s" % (
                t, meta["_total_slots"], meta["_valid_records"], out))
        return 0

    if len(argv) < 3:
        print(__doc__)
        return 1
    meta = convert(argv[1], argv[2])
    print(json.dumps(meta, ensure_ascii=False, indent=1))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
