#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
从 skillboard.hpp 提取 63 个魔法图标的坐标与图标 ID，生成数据表。

依据：client/src/gui/skillboard/skillboard.hpp 的 m_iconGfxList
格式：{DBCOM_MAGICID(u8"火球术"), 0X05001000, 0, 0, false}
      = {魔法ID, 图标ID, 列, 行, 是否被动}

输出：data/skill_icons.json
  { "magics": { "火球术": {"icon": 0x05001000, "col":0, "row":0, "passive":false} },
    "_meta": {...} }
"""

import os
import re
import json
import sys


def main(argv):
    src = argv[1] if len(argv) > 1 else r'E:/Desktop/game/mir2x/client/src'
    out = argv[2] if len(argv) > 2 else 'client-godot/data/skill_icons.json'

    hpp = os.path.join(src, 'gui', 'skillboard', 'skillboard.hpp')
    if not os.path.exists(hpp):
        print('找不到:', hpp)
        return 1

    code = open(hpp, encoding='utf-8', errors='replace').read()

    # 定位 m_iconGfxList 表体
    i = code.find('m_iconGfxList')
    if i < 0:
        print('找不到 m_iconGfxList')
        return 1
    start = code.find('{', code.find('{', i) + 1)
    end = code.find('};', start)
    body = code[start:end]

    # 逐行解析：{DBCOM_MAGICID(u8"名字"), 0X05001000, 0, 0, false}
    pat = re.compile(
        r'\{\s*DBCOM_MAGICID\(\s*u8"([^"]+)"\s*\)\s*,\s*'
        r'(0[Xx][0-9A-Fa-f]+)\s*,\s*'
        r'(\d+)\s*,\s*(\d+)\s*,\s*'
        r'(true|false)\s*\}'
    )

    magics = {}
    for m in pat.finditer(body):
        name = m.group(1)
        magics[name] = {
            'icon': int(m.group(2), 16),
            'col': int(m.group(3)),
            'row': int(m.group(4)),
            'passive': m.group(5) == 'true',
        }

    # 按元素类型分组（火/冰/雷/风/神圣/暗黑/幻影/无）
    # 从注释里拿分组信息。注意：第一个分组注释（// 火）出现在
    # m_iconGfxList 表体之前，所以要先在 body 之外找一遍
    groups = {}
    current = None

    # 先在表体开始之前找最近的 // xxx 注释作为首个分组名
    pre = code[:start]
    for line in reversed(pre.splitlines()):
        cm = re.search(r'//\s*(火|冰|雷|风|神圣|暗黑|幻影|无)\s*$', line.strip())
        if cm:
            current = cm.group(1)
            groups.setdefault(current, [])
            break

    for line in body.splitlines():
        cm = re.search(r'//\s*(火|冰|雷|风|神圣|暗黑|幻影|无)\s*$', line.strip())
        if cm:
            current = cm.group(1)
            groups.setdefault(current, [])
        mm = pat.search(line)
        if mm and current:
            groups[current].append(mm.group(1))

    meta = {
        'source': 'client/src/gui/skillboard/skillboard.hpp m_iconGfxList',
        'total': len(magics),
        'groups': {k: len(v) for k, v in groups.items()},
        'passive': sum(1 for v in magics.values() if v['passive']),
        # 面板区域（getPageRectange）
        'page_rect': {'x': 18, 'y': 44, 'w': 304, 'h': 329},
    }

    os.makedirs(os.path.dirname(out) or '.', exist_ok=True)
    with open(out, 'w', encoding='utf-8') as fp:
        json.dump({'_meta': meta, 'magics': magics, 'groups': groups},
                  fp, ensure_ascii=False, indent=1)

    print('已生成', out)
    print('魔法数:', meta['total'])
    print('分组:', meta['groups'])
    print('被动技:', meta['passive'])
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
