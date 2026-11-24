#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
从 mir2x 的 33 个怪物类提取行为差异，生成 client-godot/data/monster_behaviors.json

提取项（只提取「类与类之间真正不同的部分」）：
  1. 攻击动作类型（MOTION_MON_ATTACK0 / ATTACK1）
  2. 是否重写了 onActionSpawn / onActionStand / onActionTransf
     （这三个一起重写 = 有 standMode 趴站切换，如 ClientTaoDog）
  3. 死亡特效名（"%s_死亡特效" 的查找）
  4. 攻击是否附带魔法
  5. 特殊动作（SPECIAL / SPELL0 / SPELL1）

依据：对 client/src/client*.cpp 做正则扫描，
不靠人工判断每个类的语义。
"""

import os
import re
import json
import sys


# 需要排除的非怪物类（基类与工具类）
EXCLUDE = {
    'clientcreature', 'clientmonster', 'clientstandmonster', 'clientnpc',
    'clientluamodule', 'clientargparser', 'clientpathfinder',
}


def scan_monster(src_dir, base_name):
    """扫描一个怪物类，返回其行为特征"""
    cpp = os.path.join(src_dir, base_name + '.cpp')
    hpp = os.path.join(src_dir, base_name + '.hpp')

    if not os.path.exists(hpp):
        return None

    code = ''
    if os.path.exists(cpp):
        code = open(cpp, encoding='utf-8', errors='replace').read()
    code += '\n' + open(hpp, encoding='utf-8', errors='replace').read()

    # 类名：必须从源码里取真实类名（clienttaodog -> ClientTaoDog），
    # 不能靠首字母大写拼接，否则 Clientanthealer 这种就错了
    class_name = ''
    mcls = re.search(r'^class\s+(Client\w+)\s*:\s*public', code, re.M)
    if mcls:
        class_name = mcls.group(1)
    else:
        mcls2 = re.search(r'class\s+(Client\w+)\s*:', code)
        class_name = mcls2.group(1) if mcls2 else (base_name[0].upper() + base_name[1:])

    info = {
        'class_name': class_name,
        'source_file': base_name + '.cpp',
    }

    # ---- 1. 攻击动作类型 ----
    # 有类用 ATTACK0（普通），有类用 ATTACK1（如牛魔守卫、沙漠仙人掌）
    atk = 'MOTION_MON_ATTACK1' if re.search(r'MOTION_MON_ATTACK1', code) else 'MOTION_MON_ATTACK0'
    info['attack_motion'] = atk

    # ---- 2. standMode 三段式（spawn/stand/transf 一起重写）----
    has_spawn = bool(re.search(r'(bool|void)\s+\w+::onActionSpawn', code))
    has_stand = bool(re.search(r'(bool|void)\s+\w+::onActionStand', code))
    has_transf = bool(re.search(r'(bool|void)\s+\w+::onActionTransf', code))
    info['has_stand_mode'] = has_spawn and has_stand and has_transf
    info['override_spawn'] = has_spawn
    info['override_stand'] = has_stand
    info['override_transf'] = has_transf

    # ---- 3. 死亡特效 ----
    # 注意：死亡特效在基类 clientmonster.cpp:420 统一实现（"%s_死亡特效"），
    # 不是每个怪物类自己写的，子类基本不覆盖 —— 保留字段以便将来确认
    info['death_effect'] = bool(re.search(r'_死亡特效', code))

    # ---- 4. 攻击附魔法 ----
    info['attack_magic'] = bool(re.search(r'onActionAttack[^{]*\{(?:(?!\n\}).)*addAttachMagic', code, re.S))

    # ---- 5. 特殊动作 ----
    special = []
    for mt in ('MOTION_MON_SPECIAL', 'MOTION_MON_SPELL0', 'MOTION_MON_SPELL1'):
        if re.search(r'\b' + mt + r'\b', code):
            special.append(mt)
    info['special_motions'] = special

    # ---- 6. 自定义帧表 ----
    info['custom_frame_table'] = bool(re.search(r'getFrameGfxSeq', code))

    # ---- 7. 攻击段数（连续 ATTACK0 次数，用于多段攻击）----
    atk_body = re.search(r'onActionAttack[^{]*\{(.{0,2000})', code, re.S)
    seg = 1
    if atk_body:
        # 统计 push_back 次数作为粗略段数
        seg = max(1, atk_body.group(1).count('push_back') // 2)
    info['attack_segments'] = seg

    return info


def build_class_to_name_map(src_dir):
    """从 clientmonster.cpp 的 create() 提取「类名 -> 怪物名」映射

    依据：clientmonster.cpp:748+ 的
      case DBCOM_MONSTERID(u8"蚂蚁道士"): return new ClientAntHealer(...)
    这是原版按 UID 中的 monsterID 选子类的唯一入口。
    """
    p = os.path.join(src_dir, 'clientmonster.cpp')
    if not os.path.exists(p):
        return {}
    code = open(p, encoding='utf-8', errors='replace').read()
    pat = re.compile(
        r'case\s+DBCOM_MONSTERID\(\s*u8"([^"]+)"\s*\)\s*:\s*\{?\s*return\s+new\s+(Client\w+)')
    return {m.group(2): m.group(1) for m in pat.finditer(code)}


def main(argv):
    src_dir = argv[1] if len(argv) > 1 else r'E:/Desktop/game/mir2x/client/src'
    out_path = argv[2] if len(argv) > 2 else 'client-godot/data/monster_behaviors.json'

    if not os.path.isdir(src_dir):
        print('源码目录不存在:', src_dir)
        return 1

    # 注意：33 个怪物类里只有 20 个有 .cpp，其余实现写在 .hpp 内，
    # 所以必须以 .hpp 为准枚举，否则会漏掉一半
    files = []
    for f in os.listdir(src_dir):
        if f.startswith('client') and f.endswith('.hpp'):
            base = f[:-4]
            if base in EXCLUDE:
                continue
            files.append(base)

    result = {}
    for base in sorted(files):
        info = scan_monster(src_dir, base)
        if info is None:
            continue
        # 跳过基类残留（class Client 是基类文件扫描到的）
        if info['class_name'] in ('Client', ''):
            continue
        if not info.get('custom_frame_table') and not info.get('override_spawn') \
                and info.get('attack_segments', 1) <= 1 and not info.get('special_motions'):
            # 完全没差异的（纯默认行为）也记下来，标记为 default
            info['is_default'] = True
        else:
            info['is_default'] = False
        result[info['class_name']] = info

    # 补「类名 -> 怪物名」映射，供运行时按怪物名反查行为
    cls2name = build_class_to_name_map(src_dir)
    for cls_name, info in result.items():
        if cls_name in cls2name:
            info['monster_name'] = cls2name[cls_name]

    meta = {
        'source': 'mir2x client/src/client*.cpp',
        'total': len(result),
        'with_stand_mode': sum(1 for v in result.values() if v['has_stand_mode']),
        'with_custom_frames': sum(1 for v in result.values() if v['custom_frame_table']),
        'with_death_effect': sum(1 for v in result.values() if v['death_effect']),
        'attack1': sum(1 for v in result.values() if v['attack_motion'] == 'MOTION_MON_ATTACK1'),
        'all_default': sum(1 for v in result.values() if v['is_default']),
        'with_monster_name': sum(1 for v in result.values() if v.get('monster_name')),
    }

    out = {'_meta': meta, 'classes': result}
    os.makedirs(os.path.dirname(out_path) or '.', exist_ok=True)
    with open(out_path, 'w', encoding='utf-8') as fp:
        json.dump(out, fp, ensure_ascii=False, indent=1)

    print('已生成', out_path)
    print('类数:', meta['total'])
    print('  有 standMode 切换:', meta['with_stand_mode'])
    print('  有自定义帧表  :', meta['with_custom_frames'])
    print('  有死亡特效    :', meta['with_death_effect'])
    print('  用 ATTACK1    :', meta['attack1'])
    print('  纯默认行为    :', meta['all_default'])
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
