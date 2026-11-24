extends SceneTree
## C5+C6 自检：面板 + 怪物行为表

const SkillRef := preload("res://scripts/ui/ui_skill_board.gd")
const TeamRef := preload("res://scripts/ui/ui_team_board.gd")
const ItemListRef := preload("res://scripts/ui/ui_item_list_board.gd")
const GuildRef := preload("res://scripts/ui/ui_guild_board.gd")
const ConfigRef := preload("res://scripts/ui/ui_config_board.gd")
const BehaviorsRef := preload("res://scripts/actor/monster_behaviors.gd")
const MotionTypeRef := preload("res://scripts/action/motion_type.gd")

var _pass := 0
var _fail := 0


func _ok(cond: bool, name: String, extra: String = "") -> void:
	if cond:
		_pass += 1
		print("  [PASS] ", name)
	else:
		_fail += 1
		print("  [FAIL] ", name, ("  " + extra) if extra else "")


func _init() -> void:
	print("=== C5+C6 自检 ===")
	_test_skill()
	_test_team()
	_test_item_list()
	_test_guild()
	_test_config()
	_test_behaviors()
	print("=== 结果: 通过 %d, 失败 %d ===" % [_pass, _fail])
	quit(0)


# ==================== C5: 技能面板 ====================

func _test_skill() -> void:
	print("\n[C5-1] 技能面板")
	var sk := SkillRef.new()
	_ok(not sk.is_open(), "默认关闭")
	_ok(sk.w == 360 and sk.h == 400, "尺寸 360x400（原版 getPageRectange 区域）")

	# 63 个魔法图标
	_ok(sk._loaded, "技能图标表已加载")
	var tabs := SkillRef.TABS
	_ok(tabs.size() == 8, "8 个页签", str(tabs.size()))

	# 每个页签都有技能
	var total := 0
	for i in range(tabs.size()):
		sk.cur_tab = i
		var n := sk.tab_skills().size()
		total += n
		print("    [info] 页签 %s: %d 个技能" % [tabs[i], n])
	_ok(total == 63, "全部 63 个魔法", str(total))

	# 火球术的图标信息
	sk.cur_tab = 0
	var info := sk.magic_info("火球术")
	_ok(not info.is_empty(), "火球术存在")
	if not info.is_empty():
		_ok(int(info["icon"]) == 0x05001000, "火球术图标 ID",
			"0x%X" % int(info["icon"]))
		_ok(int(info["col"]) == 0 and int(info["row"]) == 0, "火球术在 (0,0)")

	# 被动技
	_ok(sk.magic_info("基本剑术").get("passive", false), "基本剑术是被动技")

	# 学习
	sk.learn("火球术")
	_ok(sk.learned.has("火球术"), "学习后记录")


# ==================== C5: 队伍面板 ====================

func _test_team() -> void:
	print("\n[C5-2] 队伍面板")
	var tm := TeamRef.new()
	_ok(tm.w == 600 and tm.h == 440, "尺寸 600x440（teamstateboard.hpp:35-36）")
	_ok(TeamRef.PAGE_NAMES.size() == 5, "5 个页签")

	tm.members = [
		{"name": "张三", "level": 30, "hp_ratio": 0.8},
		{"name": "李四", "level": 28, "hp_ratio": 0.5},
	]
	tm.nearby = [{"name": "打宝队", "leader": "王五", "count": 3, "max": 5}]
	tm.applicants = [{"name": "赵六", "level": 25}]
	_ok(tm.members.size() == 2, "成员 2 人")
	_ok(tm.nearby.size() == 1, "附近 1 队")
	_ok(tm.applicants.size() == 1, "申请 1 人")


# ==================== C5: 通用列表面板 ====================

func _test_item_list() -> void:
	print("\n[C5-3] 物品列表面板")
	# 五种配置
	for kind in ["list", "purchase", "auction", "stall", "secured"]:
		var b := ItemListRef.new("Test_" + kind)
		b.configure(kind)
		_ok(b.w > 0 and b.h > 0, "%s 尺寸有效 (%dx%d)" % [kind, b.w, b.h])

	# 分页
	var b2 := ItemListRef.new("Page")
	b2.configure("list")
	b2.cols = 6
	b2.rows = 5
	var items: Array = []
	for i in range(40):
		items.append({"item_id": 1, "count": i + 1})
	b2.set_items(items)
	_ok(b2.page_count() == 2, "40 件 / 30 每页 = 2 页", str(b2.page_count()))
	_ok(b2.page_items().size() == 30, "首页 30 件", str(b2.page_items().size()))
	b2.page = 1
	_ok(b2.page_items().size() == 10, "次页 10 件", str(b2.page_items().size()))

	# 空列表
	var b3 := ItemListRef.new("Empty")
	b3.set_items([])
	_ok(b3.page_items().is_empty(), "空列表无内容")
	_ok(b3.page_count() == 0, "空列表 0 页")


# ==================== C5: 行会面板 ====================

func _test_guild() -> void:
	print("\n[C5-4] 行会面板")
	var g := GuildRef.new()
	_ok(g.w == 566 and g.h == 380, "尺寸 566x380（guildboard.hpp 推导）")
	_ok(GuildRef.PAGE_NOTICE == 0 and GuildRef.PAGE_MEMBER == 1 and GuildRef.PAGE_CHAT == 2,
		"3 页枚举对齐")
	_ok(GuildRef.MEMBER_ROW_COUNT == 13, "成员列表 13 行（hpp:46）")
	_ok(GuildRef.MEMBER_ROW_H == 20, "每行 20px（hpp:45）")

	g.guild_name = "测试行会"
	g.notice = "本行会招收活跃玩家"
	g.members = [{"name": "会长甲", "level": 40, "job": 0, "rank": 0, "online": true}]
	_ok(g.members.size() == 1, "成员已设置")


# ==================== C5: 设置面板 ====================

func _test_config() -> void:
	print("\n[C5-5] 设置面板")
	var c := ConfigRef.new()
	_ok(c.w == 380 and c.h == 400, "尺寸 380x400")
	_ok(not c.groups.is_empty(), "有配置分组", str(c.groups))
	print("    [info] 分组: %s" % str(c.groups))

	# 默认值
	_ok(bool(c.values.get("平滑移动", false)) == true, "平滑移动默认开")
	_ok(int(c.values.get("音效音量", 0)) == 80, "音效音量默认 80")

	# 分组内配置项
	var items := c.items_of_group("音效")
	_ok(items.size() == 2, "音效组 2 项", str(items.size()))


# ==================== C6: 怪物行为 ====================

func _test_behaviors() -> void:
	print("\n[C6] 怪物行为表")
	var n := BehaviorsRef.count()
	_ok(n == 32, "32 个怪物类", str(n))

	var st := BehaviorsRef.stats()
	print("    [info] %s" % str(st))
	_ok(int(st["stand_mode"]) == 7, "7 个有趴站切换", str(st["stand_mode"]))
	_ok(int(st["custom_frames"]) == 13, "13 个有自定义帧表", str(st["custom_frames"]))
	_ok(int(st["attack1"]) == 2, "2 个用 ATTACK1", str(st["attack1"]))

	# 具体类的行为
	_ok(BehaviorsRef.has_stand_mode("ClientTaoDog"), "神兽有趴站切换")
	_ok(not BehaviorsRef.has_stand_mode("ClientBombSpider"), "炸弹蜘蛛无趴站切换")

	var atk := BehaviorsRef.attack_motion_of("ClientMinotaurGuardian")
	_ok(atk == MotionTypeRef.MOTION_MON_ATTACK1, "牛魔守卫用 ATTACK1",
		str(atk))
	var atk2 := BehaviorsRef.attack_motion_of("ClientTaoDog")
	_ok(atk2 == MotionTypeRef.MOTION_MON_ATTACK0, "神兽用 ATTACK0")

	# 未知类返回默认
	var def := BehaviorsRef.of("ClientNotExist")
	_ok(bool(def.get("is_default", false)), "未知类返回默认行为")
	_ok(str(def.get("attack_motion", "")) == "MOTION_MON_ATTACK0", "默认为 ATTACK0")
