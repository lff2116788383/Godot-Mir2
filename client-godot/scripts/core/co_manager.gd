extends Node2D
class_name CoManager
## 场景实体管理器（CharObject 容器）
##
## 【逐行翻译】client/src/processrunnet.cpp:151-345
## 对照：
##   - processrunnet.cpp:151-273  on_SM_ACTION    动作同步分发
##   - processrunnet.cpp:279-345  on_SM_CORECORD  进场记录
##   - processrun.cpp:2180-2240   onActionSpawn   刷怪（含召唤特效延迟）
##   - myhero.hpp:43-58           inViewRange     视野判定
##
## 职责：
##   1. 维护 uid -> 实体 的映射（m_coList）
##   2. 收到 SM_ACTION / SM_CORECORD 时创建或更新实体
##   3. 未知 UID 时按类型决定「查询」还是「忽视」
##
## Godot 差异：实体的「表现」是子节点（Node2D），逻辑数据留在 dict 里。
##              同步判定逻辑完全照搬原版。

const CoRecordRef := preload("res://scripts/core/co_record.gd")
const UidRef := preload("res://scripts/core/uid.gd")
const MonsterRecordRef := preload("res://scripts/item/monster_record.gd")
const ItemRecordRef := preload("res://scripts/item/item_record.gd")
const ProtocolDefRef := preload("res://scripts/core/protocol_def.gd")
const ActionNodeRef := preload("res://scripts/action/action_node.gd")
const ActionTypeRef := preload("res://scripts/action/action_type.gd")
const MonsterActorRef := preload("res://scripts/actor/monster_actor.gd")
const ActorFactoryRef := preload("res://scripts/actor/actor_factory.gd")

## 视野半径（sysconst.hpp:67 SYS_PLY_VIEWR = 15）
const SYS_PLY_VIEWR := 15

signal entity_created(uid: int, node: Node2D)
signal entity_removed(uid: int)

## uid -> { "node": Node2D, "record": CoRecord, "kind": "ply"/"mon"/"npc" }
var _entities: Dictionary = {}

## 我的角色位置提供者（用于视野判定），由外部注入
var _my_pos_provider: Callable = Callable()
var _my_uid: int = 0
## 地图变更回调（服务端消息 map_uid 不一致时触发）
var map_uid: int = 0
## 资产根目录
var assets_root: String = ""

## 地图上下文（供动作队列寻路用）
##   can_move: Callable(x:int, y:int) -> bool           该格是否可走
##   make_finder: Callable(max_step:int) -> PathFinder  构造寻路器
var _can_move: Callable = Callable()
var _make_finder: Callable = Callable()
const SYS_MAXSTEP := 10


func setup(p_assets_root: String) -> void:
	assets_root = p_assets_root


## 注入主角位置获取器，返回 Vector3i(x, y, map_uid)
func set_my_position_provider(fn: Callable) -> void:
	_my_pos_provider = fn


func set_my_uid(uid_v: int) -> void:
	_my_uid = uid_v


## 注入地图通行判定与寻路器构造器
func set_map_context(can_move: Callable, make_finder: Callable) -> void:
	_can_move = can_move
	_make_finder = make_finder


## 供实体节点使用的上下文（两者都为空时动作队列无法生成逐格移动）
func walk_context() -> Dictionary:
	return {
		"can_move": _can_move,
		"make_finder": _make_finder,
		"max_step": SYS_MAXSTEP,
	}


func find_uid(uid: int):
	var e = _entities.get(uid)
	if e == null:
		return null
	return e["node"]


func has_uid(uid: int) -> bool:
	return _entities.has(uid)


func remove_uid(uid: int) -> void:
	if not _entities.has(uid):
		return
	var e = _entities[uid]
	var node: Node2D = e["node"]
	if is_instance_valid(node):
		node.queue_free()
	_entities.erase(uid)
	entity_removed.emit(uid)


func find_at_grid(gx: int, gy: int):
	for uid in _entities.keys():
		var e = _entities[uid]
		var node: Node2D = e["node"]
		if not is_instance_valid(node):
			continue
		var g = node.get("grid")
		if g is Vector2i and g.x == gx and g.y == gy:
			return {"uid": uid, "node": node, "record": e["record"], "kind": e["kind"]}
	return null


func get_record(uid: int):
	var e = _entities.get(uid)
	return e["record"] if e != null else null


func set_action_context_all() -> void:
	for uid in _entities.keys():
		var node: Node2D = _entities[uid]["node"]
		if node != null and node.has_method("set_walk_context"):
			var ctx := walk_context()
			node.set_walk_context(ctx["can_move"], ctx["make_finder"], ctx["max_step"])


func all_uids() -> Array:
	return _entities.keys()


func count() -> int:
	return _entities.size()


# ==================== 视野判定 ====================

## myhero.hpp:43 inViewRange
##   LDistance2(heroX, heroY, x, y) <= (viewRange-1)^2
## 原版会同时检查主角的 x/y 与 currMotion 的 x/endX，这里主角只有单一位置，
## 故简化为一处；但保留 viewRange-1 的语义
func in_my_hero_view(x: int, y: int) -> bool:
	if not _my_pos_provider.is_valid():
		return true
	var p: Vector3i = _my_pos_provider.call()
	var d := maxi(absi(p.x - x), absi(p.y - y))
	return d <= SYS_PLY_VIEWR - 1


# ==================== SM_CORECORD（processrunnet.cpp:279） ====================

func on_sm_corecord(body: PackedByteArray) -> void:
	var r := CoRecordRef.parse(body)
	if r == null:
		push_warning("SMCORecord 解析失败")
		return

	# processrunnet.cpp:282 地图不符直接忽略
	if r.map_uid != map_uid:
		return

	# processrunnet.cpp:286 已存在则只更新动作
	if _entities.has(r.uid):
		_entities[r.uid]["record"] = r
		apply_action(r.uid, r.action)
		return

	# processrunnet.cpp:294-305 用「动作终点」判视野
	var cord_x: int = r.action.x
	var cord_y: int = r.action.y
	match r.action.type:
		ActionTypeRef.ACTION_MOVE, ActionTypeRef.ACTION_SPACEMOVE:
			cord_x = r.action.aim_x
			cord_y = r.action.aim_y

	# processrunnet.cpp:307 视野外不创建（避免过期记录复活已离开的实体）
	if not in_my_hero_view(cord_x, cord_y):
		return

	# processrunnet.cpp:309-343 按类型创建
	match UidRef.get_uid_type(r.uid):
		UidRef.UID_MON:
			_create_monster(r)
		UidRef.UID_PLY:
			_create_player(r)
		UidRef.UID_NPC:
			_create_npc(r)
		_:
			pass


# ==================== SM_ACTION（processrunnet.cpp:151） ====================

func on_sm_action(body: PackedByteArray) -> void:
	if body.size() < 44:
		push_warning("SMAction 长度不足: %d" % body.size())
		return
	var uid := body.decode_s64(0)
	var act_map_uid := body.decode_s64(8)
	var action = ActionNodeRef.from_bytes(body, 16)
	if action == null:
		return

	# ---- processrunnet.cpp:155 地图不一致 ----
	if act_map_uid != map_uid:
		if uid == _my_uid:
			_clear_all()
			map_uid = act_map_uid
			map_change_requested.emit(act_map_uid, action)
		else:
			# 其他实体换图 = 离开本图
			remove_uid(uid)
		return

	# ---- processrunnet.cpp:172 已知实体 ----
	if _entities.has(uid):
		# processrunnet.cpp:176 不接受 ACTION_SPAWN（实体已存在）
		# 原版注释：SPAWN 本该是第一个动作，但当前实现有时会晚到，直接忽略
		if action.type == ActionTypeRef.ACTION_SPAWN:
			return
		_entities[uid]["record"].action = action
		apply_action(uid, action)
		return

	# ---- processrunnet.cpp:204 未知实体 ----
	match UidRef.get_uid_type(uid):
		UidRef.UID_PLY:
			# processrunnet.cpp:208 玩家不能凭动作创建，必须查询完整记录
			# 原版 queryCORecord(uid)，这里发信号由上层决定
			entity_query_requested.emit(uid)
		UidRef.UID_MON:
			if action.type == ActionTypeRef.ACTION_SPAWN:
				_on_action_spawn(uid, action)
			else:
				# processrunnet.cpp:230-268 怪物可凭动作直接创建
				_create_monster_from_action(uid, action)
		UidRef.UID_NPC:
			_create_npc_from_action(uid, action)
		_:
			pass


signal entity_query_requested(uid: int)
signal map_change_requested(map_uid: int, action)


# ==================== 创建实体 ====================

func _clear_all() -> void:
	for uid in _entities.keys():
		remove_uid(uid)


func _create_monster(r) -> void:
	var node: Node2D = ActorFactoryRef.make_monster(assets_root, r.uid, r.monster_id)
	if node == null:
		return
	node.set_display_name(MonsterRecordRef.name_of(r.monster_id))
	_register(r.uid, node, r, "mon")
	apply_action(r.uid, r.action)


func _create_monster_from_action(uid: int, action) -> void:
	var mon_id := UidRef.get_monster_id(uid)
	var node: Node2D = ActorFactoryRef.make_monster(assets_root, uid, mon_id)
	if node == null:
		return
	node.set_display_name(MonsterRecordRef.name_of(mon_id))
	var r := CoRecordRef.new()
	r.uid = uid
	r.map_uid = map_uid
	r.monster_id = mon_id
	r.action = action
	_register(uid, node, r, "mon")
	apply_action(uid, action)


func _create_player(r) -> void:
	var node: Node2D = ActorFactoryRef.make_player_view(assets_root, r.uid, bool(r.gender), r.job)
	if node == null:
		return
	node.set_display_name("玩家")
	_register(r.uid, node, r, "ply")
	apply_action(r.uid, r.action)


func _create_npc(r) -> void:
	var node: Node2D = ActorFactoryRef.make_npc(assets_root, r.uid, r.npc_id)
	if node == null:
		return
	node.set_display_name("NPC")
	_register(r.uid, node, r, "npc")
	apply_action(r.uid, r.action)


func _create_npc_from_action(uid: int, action) -> void:
	var npc_id := UidRef.get_npc_id(uid)
	var node: Node2D = ActorFactoryRef.make_npc(assets_root, uid, npc_id)
	if node == null:
		return
	var r := CoRecordRef.new()
	r.uid = uid
	r.map_uid = map_uid
	r.npc_id = npc_id
	r.action = action
	_register(uid, node, r, "npc")
	apply_action(uid, action)


func _register(uid: int, node: Node2D, record, kind: String) -> void:
	# 实体之间按 y 排序（processrun.cpp:352-359 的 LocHashTable 语义）
	# Godot 的 y_sort 用 position.y 比较，正好等价「y 大的后画」
	y_sort_enabled = true
	if node.has_method("set_walk_context"):
		var ctx := walk_context()
		node.set_walk_context(ctx["can_move"], ctx["make_finder"], ctx["max_step"])
	add_child(node)
	_entities[uid] = {"node": node, "record": record, "kind": kind}
	entity_created.emit(uid, node)


# ==================== 动作应用 ====================

func set_actor_wear(uid: int, wear: Dictionary, hair: int = 1, hair_color: int = 0) -> void:
	var node = find_uid(uid)
	if node == null:
		return
	var human = node.get("human")
	if human == null:
		return
	var dress := 0
	var helmet := 0
	var weapon := 0
	if wear.has(ProtocolDefRef.WLG_DRESS):
		dress = ItemRecordRef.pkg_gfx_id(int(wear[ProtocolDefRef.WLG_DRESS].item_id))
	if wear.has(ProtocolDefRef.WLG_HELMET):
		helmet = ItemRecordRef.pkg_gfx_id(int(wear[ProtocolDefRef.WLG_HELMET].item_id))
	if wear.has(ProtocolDefRef.WLG_WEAPON):
		weapon = ItemRecordRef.pkg_gfx_id(int(wear[ProtocolDefRef.WLG_WEAPON].item_id))
	var col := Color.from_hsv(float((hair_color >> 16) & 0xFF) / 255.0,
		float((hair_color >> 8) & 0xFF) / 255.0, float(hair_color & 0xFF) / 255.0)
	if hair_color == 0:
		col = Color.WHITE
	human.set_appearance(dress, hair, helmet, weapon, bool(node.get("is_male")), col)


func set_actor_name(uid: int, nm: String, color: int = 0) -> void:
	var node = find_uid(uid)
	if node != null and node.has_method("set_display_name"):
		var c := Color(0.82, 0.9, 1.0)
		if color != 0:
			c = Color8((color >> 16) & 0xFF, (color >> 8) & 0xFF, color & 0xFF, 255)
		if node.has_method("set_subtitle"):
			node.set_display_name(nm, c)
		else:
			node.set_display_name(nm)


func set_actor_subtitle(uid: int, text: String) -> void:
	var node = find_uid(uid)
	if node != null and node.has_method("set_subtitle"):
		node.set_subtitle(text)


func set_actor_health(uid: int, hp: int, max_hp: int) -> void:
	var node = find_uid(uid)
	if node != null and node.has_method("set_health_ratio"):
		node.set_health_ratio(float(hp) / maxf(1.0, float(max_hp)))


func apply_action(uid: int, action) -> void:
	var e = _entities.get(uid)
	if e == null:
		return
	var node = e["node"]
	if not is_instance_valid(node):
		return
	if node.has_method("apply_action"):
		node.apply_action(action)


# ==================== onActionSpawn（processrun.cpp:2180） ====================
## 简化：原版对「变异骷髅/超强骷髅/神兽」有召唤动画延迟创建，
##       这里保留「非怪物 UID 则转为查询」的判定，召唤特效待 B12 补

func _on_action_spawn(uid: int, action) -> void:
	if UidRef.get_uid_type(uid) != UidRef.UID_MON:
		entity_query_requested.emit(uid)
		return
	_create_monster_from_action(uid, action)
