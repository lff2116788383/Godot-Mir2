extends RefCounted
class_name SDInventory
## 背包容器
##
## 【逐行翻译】common/src/serdesmsg.cpp:50-215 + serdesmsg.hpp:564-615
##
## 关键规则（serdesmsg.hpp:566-568 的注释）：
##   「保持 item 列表封装，不对外暴露，因为有 seqID 重复检查」
##   即同一个 itemID 下 seqID 必须唯一，(itemID, seqID) 是唯一键。
##
## 堆叠规则（serdesmsg.cpp:143-156）：
##   「只支持改一个物品，不能自动拆分合并 (55+56) -> (99,12)」
##   即：找到第一个同 itemID 且相加不超过 99 的就合并，否则新开一格。

const SDItemRef := preload("res://scripts/item/sd_item.gd")
const ItemRecordRef := preload("res://scripts/item/item_record.gd")
const SysConstRef9 := preload("res://scripts/core/sys_const.gd")

## 内部物品列表（对应 m_list）
var m_list: Array = []


# ---------------- 内部工具 ----------------

## getItemIDSeqSet（serdesmsg.cpp:50）
## 返回所有 (itemID << 32 | seqID)，同时校验无重复
func get_item_id_seq_set() -> Dictionary:
	var result := {}
	for it in m_list:
		var k: int = it.item_id_seq()
		if result.has(k):
			push_error("发现重复物品: itemID=%d seqID=%d" % [it.item_id, it.seq_id])
			continue
		result[k] = true
	return result


## itemIDSeq 辅助（sditem.hpp:186）
func _item_id_seq(item_id: int, seq_id: int) -> int:
	return (item_id << 32) | seq_id


# ---------------- 查询 ----------------

## has（serdesmsg.cpp:63）
## 统计某 itemID 的总数量；seqID=0 表示不限定
func has(item_id: int, seq_id: int = 0) -> int:
	var count := 0
	for it in m_list:
		if it.item_id == item_id and (seq_id == 0 or it.seq_id == seq_id):
			count += it.count
	return count


## find（serdesmsg.cpp:109）
## 返回物品或 null
func find(item_id: int, seq_id: int = 0):
	for it in m_list:
		if it.item_id == item_id and (seq_id == 0 or it.seq_id == seq_id):
			return it
	return null


## getQualityItemList（serdesmsg.cpp:79）
## 按品质筛选，取够 count 个；不够返回 null
func get_quality_item_list(item_id: int, min_quality: int, count: int):
	if count <= 0 or min_quality < 0:
		return null

	var result: Array = []
	var remain := count
	for it in m_list:
		if it.item_id != item_id:
			continue
		# 品质存在 EA_QUALITY 属性里
		var q_bytes: PackedByteArray = it.get_ext_attr(SDItemRef.EA_QUALITY)
		if q_bytes.is_empty():
			continue
		var quality := int(q_bytes.decode_u64(0)) if q_bytes.size() >= 8 else 0
		if quality < min_quality:
			continue

		var selected = SDItemRef.new()
		selected.item_id = it.item_id
		selected.seq_id = it.seq_id
		selected.count = mini(remain, it.count)
		selected.duration = it.duration.duplicate()
		selected.ext_attr_list = it.ext_attr_list.duplicate()
		remain -= selected.count
		result.append(selected)
		if remain == 0:
			return result
	return null


# ---------------- 增删 ----------------

## add（serdesmsg.cpp:129）
## keep_seq_id=true 时保留给定 seqID（需保证不重复）
## 返回实际加入的物品
func add(new_item, keep_seq_id: bool = false):
	# 原版只断言 newItem 非空（serdesmsg.cpp:131 fflassert(newItem)），
	# 不校验 seqID —— seqID 正是这里要分配的，不能用 is_valid() 提前拦掉
	if new_item == null:
		return null
	if new_item.item_id == 0 or new_item.count == 0:
		return null
	var seq_set := get_item_id_seq_set()

	if keep_seq_id:
		if seq_set.has(new_item.item_id_seq()):
			push_error("seqID 重复: itemID=%d seqID=%d" % [new_item.item_id, new_item.seq_id])
			return null
		m_list.append(new_item)
		return new_item

	# 可堆叠物品：找第一个能合并的
	var ir := ItemRecordRef.new()
	if ir.packable(new_item.item_id):
		for it in m_list:
			if it.item_id != new_item.item_id:
				continue
			# 只改一个物品，不做拆分合并
			if it.count + new_item.count <= SysConstRef9.SYS_INVGRIDMAXHOLD:
				it.count += new_item.count
				return it

	# 分配新 seqID
	var seq_id := 1
	while true:
		if not seq_set.has(_item_id_seq(new_item.item_id, seq_id)):
			new_item.seq_id = seq_id
			m_list.append(new_item)
			return new_item
		seq_id += 1
	return null


## remove（serdesmsg.cpp:169）
## 返回 {removed_count, seq_id, remaining_item_or_null}
##   strict=false : 数量不够也删（删光）
##   strict=true  : 数量必须精确匹配才删
func remove(item_id: int, seq_id: int, count: int, strict: bool = false) -> Dictionary:
	if count <= 0:
		return {"count": 0, "seq_id": 0, "item": null}

	for i in range(m_list.size()):
		var it = m_list[i]
		if it.item_id != item_id:
			continue
		if seq_id > 0 and it.seq_id != seq_id:
			continue

		if count < it.count:
			it.count -= count
			return {"count": count, "seq_id": it.seq_id, "item": it}

		if not strict or count == it.count:
			var removed_count: int = it.count
			var removed_seq: int = it.seq_id
			m_list.remove_at(i)
			return {"count": removed_count, "seq_id": removed_seq, "item": null}

	return {"count": 0, "seq_id": 0, "item": null}


## merge（serdesmsg.cpp:202）
## 把 fromSeqID 的物品合并进 toSeqID
func merge(item_id: int, from_seq_id: int, to_seq_id: int) -> bool:
	if from_seq_id == 0 or to_seq_id == 0:
		return false

	var ir := ItemRecordRef.new()
	if not ir.packable(item_id):
		return false

	var from_item = find(item_id, from_seq_id)
	var to_item = find(item_id, to_seq_id)
	if from_item == null or to_item == null:
		return false
	if from_item == to_item:
		return false

	# 合并后不能超上限
	if to_item.count + from_item.count > SysConstRef9.SYS_INVGRIDMAXHOLD:
		return false

	to_item.count += from_item.count
	var idx := m_list.find(from_item)
	if idx >= 0:
		m_list.remove_at(idx)
	return true


# ---------------- 便捷 ----------------

func clear() -> void:
	m_list.clear()


func size() -> int:
	return m_list.size()


## 整包替换（SM_INVENTORY 用）
## 对应 processrunnet.cpp:829 getInvPack().setInventory(...)
func set_items(items: Array) -> void:
	m_list = items.duplicate()


func item_list() -> Array:
	return m_list


## 是否装得下（背包上限 100 格，sysconst.hpp:83-84）
func capacity() -> int:
	return SysConstRef9.SYS_INVGRIDGW * SysConstRef9.SYS_INVGRIDGH


func is_full() -> bool:
	return m_list.size() >= capacity()


func to_str() -> String:
	return "SDInventory{%d 项}" % m_list.size()
