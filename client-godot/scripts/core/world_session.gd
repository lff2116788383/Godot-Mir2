extends Node
class_name WorldSession
## 游戏世界会话：P0/P1 功能接线层
## 原则：协议布局按 C++ 结构直译；UI 只做数据绑定和动作转发。

const Parser := preload("res://scripts/net/sd_msg_parser.gd")
const ActionNodeRef := preload("res://scripts/action/action_node.gd")
const ActionTypeRef := preload("res://scripts/action/action_type.gd")
const MotionTypeRef := preload("res://scripts/action/motion_type.gd")
const PathFinderRef := preload("res://scripts/action/path_finder.gd")
const UidRef := preload("res://scripts/core/uid.gd")
const ProtocolDefRef := preload("res://scripts/core/protocol_def.gd")
const ItemRecordRef := preload("res://scripts/item/item_record.gd")
const MagicDBRef := preload("res://scripts/magic/magic_db.gd")
const MagicEffectRef := preload("res://scripts/magic/magic_effect_actor.gd")
const MapRecordRef := preload("res://scripts/item/map_record.gd")

var main: Node = null
var net: MirClient = null
var ui = null
var co_manager = null
var player = null
var obj_layer = null
var fx = null
var inventory = null

var self_uid: int = 0
var self_name: String = ""
var map_uid: int = 0
var map_id: int = 0
var gender: int = 0
var job: int = 0
var selected_target_uid: int = 0
var selected_magic_id: int = 0
var pending_action: Dictionary = {}

var team_info: Dictionary = {}
var team_members: Array = []
var team_applicants: Array = []
var guild_info: Dictionary = {}
var guild_wars: Array = []
var friends: Array = []
var quests: Dictionary = {}
var auction_entries: Array = []
var ranking: Array = []
var safe_zone: Dictionary = {}
var firewalls: Array = []
var my_stall: Dictionary = {}
var remote_stall: Dictionary = {}
var direct_trade: Dictionary = {}
var buffs: Dictionary = {}
var magic_keys: Dictionary = {}
var runtime_config: Dictionary = {}
var party: Dictionary = {}

var _map_refresh_acc := 0.0
var _last_hero_grid := Vector2i(-99999, -99999)


func setup(ctx: Dictionary) -> void:
	main = ctx.get("main")
	net = ctx.get("net")
	ui = ctx.get("ui")
	co_manager = ctx.get("co_manager")
	player = ctx.get("player")
	obj_layer = ctx.get("obj_layer")
	fx = ctx.get("fx")
	inventory = ctx.get("inventory")
	if ctx.has("self_uid"): self_uid = int(ctx["self_uid"])
	if ctx.has("self_name"): self_name = str(ctx["self_name"])
	if ctx.has("map_uid"): map_uid = int(ctx["map_uid"])
	_bind_ui()


func _bind_ui() -> void:
	if ui == null:
		return
	if ui.purchase_board != null:
		ui.purchase_board.on_buy = _on_purchase_buy
		ui.purchase_board.on_close = func(_w): pass
	if ui.auction2 != null:
		ui.auction2.on_buy = _on_auction_buy
		ui.auction2.on_cancel = _on_auction_cancel
		ui.auction2.on_search = _on_auction_search
		ui.auction2.on_register = func(_w): _log("双击背包物品可登记拍卖")
	if ui.stall_board2 != null:
		ui.stall_board2.on_buy = _on_stall_buy
		ui.stall_board2.on_start_stall = _on_start_stall
		ui.stall_board2.on_close_stall = _on_close_stall
		ui.stall_board2.on_item_op = _on_stall_item_op
	if ui.direct_trade != null:
		ui.direct_trade.on_trade = _on_direct_trade_lock
		ui.direct_trade.on_close = func(_w): _cancel_trade()
	if ui.friend_chat != null:
		ui.friend_chat.on_send = _on_friend_send
		ui.friend_chat.on_search = _on_friend_search
		ui.friend_chat.on_add_friend = _on_friend_add
		ui.friend_chat.on_open_chat = _on_friend_open
	if ui.team_board != null:
		ui.team_board.on_member_click = _on_team_member_click
	if ui.guild_board != null:
		ui.guild_board.on_send_chat = _on_guild_chat
		ui.guild_board.on_set_notice = _on_guild_notice
	if ui.skill_board != null:
		ui.skill_board.on_skill_click = _on_skill_click
	if ui.config_board != null:
		ui.config_board.on_change = _on_config_change
	if ui.quick_access != null:
		ui.quick_access.on_slot_click = _on_quick_slot
	if ui.npc_chat != null:
		ui.npc_chat.on_npc_event = _on_npc_event
	if ui.horse_board != null:
		ui.horse_board.on_mount = func(_w): _act(MotionTypeRef.MOTION_ONHORSESTAND)
		ui.horse_board.on_dismount = func(_w): _act(MotionTypeRef.MOTION_STAND)
	if ui.secured_board != null:
		ui.secured_board.on_action = _on_secured_action
	if ui.my_state_board != null:
		ui.my_state_board.on_unequip = func(_w, slot): unequip_slot(int(slot))
	if ui.peer_state_board != null:
		ui.peer_state_board.on_trade = func(_w): _on_peer_trade()
		ui.peer_state_board.on_add_friend = func(_w): _on_peer_add_friend()
	if ui.mini_map2 != null:
		_refresh_minimap()


func bind_net(p_net: MirClient) -> void:
	net = p_net


func set_online(uid: int, pname: String, p_map_uid: int, p_gender: int, p_job: int) -> void:
	self_uid = uid
	self_name = pname
	map_uid = p_map_uid
	map_id = UidRef.get_map_id(p_map_uid)
	gender = p_gender
	job = p_job
	if co_manager != null:
		co_manager.set_my_uid(uid)
		co_manager.map_uid = map_uid


func apply_start_scene(d: Dictionary) -> void:
	var uid := int(d.get("uid", self_uid))
	var pname := str(d.get("name", self_name))
	var muid := int(d.get("map_uid", map_uid))
	var x := int(d.get("x", -1))
	var y := int(d.get("y", -1))
	var dir := int(d.get("direction", PathFinderRef.DIR_DOWN))
	set_online(uid, pname, muid, int(d.get("gender", gender)), int(d.get("job", job)))
	if main != null and main.has_method("apply_server_scene"):
		main.apply_server_scene(d)
	if co_manager != null:
		co_manager.set_map_context(Callable(main, "can_move_to"), Callable(main, "make_path_finder"))
	if player != null and x >= 0 and y >= 0:
		player.set_grid(Vector2i(x, y), dir if dir > 0 else PathFinderRef.DIR_DOWN)
	if ui != null and ui.ctrl_board != null:
		ui.ctrl_board.player = player
		ui.ctrl_board.map_id = map_id
		ui.ctrl_board.map_gx = x
		ui.ctrl_board.map_gy = y
	if d.has("wear") and ui != null and ui.my_state_board != null:
		ui.my_state_board.wear = d["wear"]
		_refresh_player_appearance(d)
	_refresh_minimap()
	var bgm := MapRecordRef.bgm_of(map_id)
	if bgm >= 0 and _audio() != null:
		_audio().play_bgm(bgm)
	_log("进入地图 %s [%d,%d]" % [MapRecordRef.name_of(map_id), x, y])


func handle_frame(code: int, body: PackedByteArray) -> bool:
	# P0：服务端驱动场景
	if code == MirClient.SM_STARTGAMESCENE:
		var d := Parser.parse_start_game_scene(body)
		if not d.is_empty():
			apply_start_scene(d)
		return true

	# P0：魔法表现
	if code == MirClient.SM_CASTMAGIC:
		var cm := Parser.parse_cast_magic(body)
		if not cm.is_empty() and int(cm.get("map_uid", map_uid)) == map_uid:
			_spawn_magic_effect(int(cm["magic"]), Vector2i(int(cm["aim_x"]), int(cm["aim_y"])), int(cm["direction"]))
			if fx != null:
				fx.on_magic_damage(int(cm["aim_x"]), int(cm["aim_y"]), 0)
		return true

	# P0/P1：Buff
	if code == MirClient.SM_BUFF:
		var bf := Parser.parse_buff(body)
		if not bf.is_empty():
			buffs[int(bf["uid"])] = {"type": int(bf["type"]), "state": int(bf["state"])}
			_log("Buff uid=%d type=%d state=%d" % [int(bf["uid"]), int(bf["type"]), int(bf["state"])])
		return true

	# 安全区 / 火墙
	if code == MirClient.SM_SAFEZONELIST:
		safe_zone = Parser.parse_safe_zone_list(body)
		if obj_layer != null:
			obj_layer.safe_zone = safe_zone
		return true
	if code == MirClient.SM_GROUNDFIREWALLLIST:
		var fw := Parser.parse_ground_firewall(body)
		firewalls = fw.get("cells", [])
		if obj_layer != null:
			obj_layer.firewalls = firewalls
		return true
	if code == MirClient.SM_REMOVEGROUNDITEM:
		var x := int(Parser.u16(body, 0)); var y := int(Parser.u16(body, 2))
		var iid := int(Parser.u32(body, 4))
		if obj_layer != null and obj_layer.has_method("remove_ground_item"):
			obj_layer.remove_ground_item(x, y, iid)
		return true

	# NPC 商店 / 商品列表
	if code == MirClient.SM_NPCSELL:
		var ns := Parser.parse_npc_sell(body)
		if ui != null and ui.purchase_board != null:
			var items: Array = []
			for iid in ns.get("items", []):
				items.append({"item_id": iid, "count": 1, "seq_id": 0,
					"price": ItemRecordRef.price(int(iid))})
			ui.purchase_board.load_sell(int(ns.get("npc_uid", 0)), items)
		return true
	if code == MirClient.SM_SELLITEMLIST:
		var sl := Parser.parse_sell_item_list(body)
		if ui != null and ui.purchase_board != null:
			ui.purchase_board.load_sell(int(sl.get("npc_uid", 0)), sl.get("items", []))
		return true
	if code == MirClient.SM_BUYSUCCEED:
		_log("购买成功 item=%d seq=%d" % [int(Parser.u32(body, 8)), int(Parser.u32(body, 12))])
		return true
	if code == MirClient.SM_BUYERROR:
		_log("购买失败 error=%d" % int(Parser.u16(body, 16)))
		return true

	if code == MirClient.SM_WEARITEMDURATION:
		var wltype := Parser.u32(body, 0)
		var dur_cur := Parser.u32(body, 4)
		var dur_max := Parser.u32(body, 8)
		var hs = main.hero_state if main != null else null
		if hs != null and hs["wear"].has(wltype):
			hs["wear"][wltype].set_duration(dur_cur, dur_max)
			if ui != null and ui.my_state_board != null:
				ui.my_state_board.wear = hs["wear"]
		return true
	if code == MirClient.SM_EQUIPWEARERROR:
		_log("装备失败 item=%d seq=%d error=%d" % [int(Parser.u32(body,0)), int(Parser.u32(body,4)), int(Parser.u16(body,8))])
		return true
	if code == MirClient.SM_GRABWEARERROR:
		_log("脱装失败 error=%d" % int(Parser.u16(body,0)))
		return true
	if code == MirClient.SM_EQUIPBELTERROR:
		_log("腰带装备失败 error=%d" % int(Parser.u16(body,8)))
		return true
	if code == MirClient.SM_GRABBELTERROR:
		_log("腰带卸下失败 error=%d" % int(Parser.u16(body,0)))
		return true
	if code == MirClient.SM_BUILDVERSION:
		var ver := Parser.rd_str(body, 0)
		_log("服务端版本：%s" % str(ver[0]))
		return true

	# 装备/材料/仓库
	if code == MirClient.SM_SHOWSECUREDITEMLIST:
		var sec := Parser.parse_show_secured_item_list(body)
		if ui != null and ui.secured_board != null:
			ui.secured_board.set_items(sec.get("items", []))
			ui.secured_board.show = true
		return true
	if code == MirClient.SM_INVOPCOST:
		_log("背包操作消耗 invOp=%d item=%d cost=%d" % [int(Parser.u32(body,0)), int(Parser.u32(body,4)), int(Parser.u32(body,12))])
		return true
	if code == MirClient.SM_NPCERROR:
		_log("NPC 错误 code=%d" % int(Parser.u8(body,0)))
		return true

	# 队伍
	if code == MirClient.SM_TEAMMEMBERLIST:
		var tm := Parser.parse_team_member_list(body)
		team_members = tm.get("members", [])
		team_info = {"leader": tm.get("leader", 0), "req": tm.get("req", {})}
		if ui != null and ui.team_board != null: ui.team_board.members = team_members
		return true
	if code == MirClient.SM_TEAMINFO:
		var ti := Parser.parse_team_info(body)
		ti["count"] = int(ti.get("member_count", 0))
		ti["max"] = int(ti.get("req", {}).get("max_member", 0))
		if int(ti["max"]) <= 0:
			ti["max"] = 5
		if ui != null and ui.team_board != null:
			ui.team_board.nearby.append(ti)
		return true
	if code == MirClient.SM_TEAMCANDIDATE:
		var tc := Parser.parse_team_candidate(body)
		team_applicants.append(tc)
		if ui != null and ui.team_board != null: ui.team_board.applicants = team_applicants
		_log("%s 申请加入队伍" % str(tc.get("name","?")))
		return true
	if code == MirClient.SM_TEAMERROR:
		_log("队伍错误 code=%d" % int(Parser.u8(body,0)))
		return true

	# 好友 / 聊天
	if code == MirClient.SM_FRIENDLIST:
		var fl := Parser.parse_friend_list(body)
		friends = fl.get("friends", [])
		if ui != null and ui.friend_chat != null: ui.friend_chat.friends = friends
		return true
	if code == MirClient.SM_ADDFRIENDACCEPTED:
		var fp := Parser.parse_chat_peer(body)
		if not fp.is_empty():
			friends.append(fp)
			if ui != null and ui.friend_chat != null: ui.friend_chat.friends = friends
			_log("%s 接受了你的好友申请" % str(fp.get("name","?")))
		return true
	if code == MirClient.SM_ADDFRIENDREJECTED:
		var fp2 := Parser.parse_chat_peer(body)
		_log("%s 拒绝了好友申请" % str(fp2.get("name","?")))
		return true
	if code == MirClient.SM_QUERYCHATPEERLISTOK:
		var pl := Parser.parse_chat_peer_list(body)
		if ui != null and ui.friend_chat != null:
			ui.friend_chat.friends = pl.get("peers", [])
			ui.friend_chat.set_ui_page(ui.friend_chat.PAGE_FRIENDLIST)
		return true
	if code == MirClient.SM_QUERYCHATMESSAGEOK:
		var cm := Parser.parse_chat_message(body)
		_append_chat_message(cm)
		return true
	if code == MirClient.SM_CHATMESSAGELIST:
		for m in Parser.parse_chat_message_list(body).get("messages", []):
			_append_chat_message(m)
		return true
	if code == MirClient.SM_CHATMESSAGEOK:
		var seqinfo := Parser.parse_chat_message_seq(body)
		_log("消息已送达 #%d" % int(seqinfo.get("id",0)))
		return true
	if code == MirClient.SM_ADDFRIENDOK:
		var fp3 := Parser.parse_chat_peer(body)
		if not fp3.is_empty():
			friends.append(fp3)
			if ui != null and ui.friend_chat != null: ui.friend_chat.friends = friends
		return true
	if code == MirClient.SM_GUILDCHAT:
		var nr := Parser.rd_str(body, 8)
		var name_ := str(nr[0])
		var content := Parser.parse_raw_text(body.slice(int(nr[1]), body.size()))
		if ui != null and ui.guild_board != null:
			ui.guild_board.chat_lines.append({"name": name_, "text": content})
		_log("[行会] %s: %s" % [name_, content])
		return true

	# 行会
	if code == MirClient.SM_GUILDINFO:
		guild_info = Parser.parse_guild_info(body)
		if ui != null and ui.guild_board != null:
			ui.guild_board.guild_name = str(guild_info.get("name", ""))
			ui.guild_board.notice = str(guild_info.get("notice", ""))
			ui.guild_board.members = guild_info.get("members", [])
		return true
	if code == MirClient.SM_GUILDWARLIST:
		guild_wars = Parser.parse_guild_war_list(body).get("wars", [])
		return true
	if code == MirClient.SM_PLAYERGUILD:
		var pg := Parser.parse_player_guild(body)
		_log("行会信息 %s / %s" % [str(pg.get("guild_name","")), str(pg.get("title",""))])
		return true
	if code == MirClient.SM_GUILDERROR:
		_log("行会错误 code=%d" % int(Parser.u8(body,0)))
		return true

	# 任务
	if code == MirClient.SM_QUESTDESPLIST:
		var ql := Parser.parse_quest_desp_list(body)
		quests.clear()
		for q in ql.get("quests", []): quests[str(q.get("id",""))] = q
		_refresh_quests()
		return true
	if code == MirClient.SM_QUESTDESPUPDATE:
		var qu := Parser.parse_quest_desp_update(body)
		var qid := str(qu.get("name",""))
		if qid != "":
			if not quests.has(qid):
				quests[qid] = {"id": qid, "title": qid, "xml": "", "folded": false}
			quests[qid]["xml"] = str(qu.get("desp",""))
		_refresh_quests()
		return true

	# 拍卖 / 摆摊 / 交易
	if code == MirClient.SM_AUCTIONITEMLIST:
		var al := Parser.parse_auction_item_list(body)
		auction_entries = al.get("entries", [])
		if ui != null and ui.auction2 != null:
			ui.auction2.set_entries(auction_entries, int(uidf_dbid(self_uid)))
		return true
	if code == MirClient.SM_AUCTIONREGISTERERROR or code == MirClient.SM_AUCTIONBUYERROR or code == MirClient.SM_AUCTIONUNREGISTERERROR:
		_log("拍卖行错误 code=%d" % int(Parser.u8(body,0)))
		return true
	if code == MirClient.SM_STALL or code == MirClient.SM_QUERYSTALLOK:
		var st := Parser.parse_stall(body)
		st["seller_uid"] = int(st.get("dbid", 0))
		if int(st.get("dbid", 0)) == uidf_dbid(self_uid):
			my_stall = st
		else:
			remote_stall = st
		if ui != null and ui.stall_board2 != null:
			ui.stall_board2.load_stall(st, self_name, uidf_dbid(self_uid))
		return true
	if code == MirClient.SM_STALLERROR:
		_log("摆摊错误 code=%d" % int(Parser.u8(body,0)))
		return true
	if code == MirClient.SM_PLAYERSTALL:
		return true
	if code == MirClient.SM_REQUESTDIRECTTRADE:
		var tr := Parser.parse_direct_trade_request(body)
		if ui != null and ui.input_board != null:
			ui.input_board.wait_choice("%s 请求与你交易" % str(tr.get("name","?")), ["接受", "拒绝"])
			ui.input_board.on_choice = func(idx): if net != null: net.respond_direct_trade(int(tr.get("uid",0)), idx == 0)
		return true
	if code == MirClient.SM_STARTDIRECTTRADE:
		var st := Parser.parse_direct_trade_request(body)
		direct_trade = st
		if ui != null and ui.direct_trade != null:
			ui.direct_trade.begin_trade(int(st.get("uid",0)), str(st.get("name","")))
		return true
	if code == MirClient.SM_UPDATEDIRECTTRADE:
		var offer := Parser.parse_direct_trade_offer(body)
		direct_trade = offer
		if ui != null and ui.direct_trade != null:
			if int(offer.get("uid", 0)) == self_uid:
				ui.direct_trade.my_items = offer.get("items", [])
				ui.direct_trade.my_gold_text = str(int(offer.get("gold", 0)))
				ui.direct_trade.locked = bool(offer.get("locked", false))
				ui.direct_trade.confirmed = bool(offer.get("confirmed", false))
			else:
				ui.direct_trade.set_peer_offer(int(offer.get("gold", 0)), offer.get("items", []))
				ui.direct_trade.peer_locked = bool(offer.get("locked", false))
				ui.direct_trade.peer_confirmed = bool(offer.get("confirmed", false))
		return true
	if code == MirClient.SM_COMPLETEDIRECTTRADE:
		if ui != null and ui.direct_trade != null: ui.direct_trade.complete_trade(); ui.direct_trade.show = false
		direct_trade.clear()
		return true
	if code == MirClient.SM_CLOSEDIRECTTRADE:
		if ui != null and ui.direct_trade != null: ui.direct_trade.close_trade()
		direct_trade.clear()
		return true
	if code == MirClient.SM_DIRECTTRADEERROR:
		_log("交易错误 code=%d" % int(Parser.u8(body,0)))
		return true

	if code == MirClient.SM_STARTINPUT:
		var si := Parser.parse_start_input(body)
		if ui != null and ui.input_board != null:
			ui.input_board.wait_input(str(si.get("title", "")))
			ui.input_board.on_submit = func(text):
				if net != null:
					net.npc_event(int(si.get("uid", 0)), "", str(si.get("commit_tag", "")), text)
		return true
	if code == MirClient.SM_STARTINVOP:
		var io := Parser.parse_start_invop(body)
		_log("背包操作开始 invOp=%d" % int(io.get("inv_op", 0)))
		return true
	if code == MirClient.SM_PLAYERNAME:
		var pn := Parser.parse_player_name(body)
		if co_manager != null:
			co_manager.set_actor_name(int(pn.get("uid", 0)), str(pn.get("name", "")), int(pn.get("name_color", 0)))
		return true
	if code == MirClient.SM_PLAYERWLDESP:
		var wd := Parser.parse_player_wl_desp(body)
		if co_manager != null:
			co_manager.set_actor_wear(int(wd.get("uid", 0)), wd.get("wear", {}),
				int(wd.get("hair", 1)), int(wd.get("hair_color", 0)))
		return true
	if code == MirClient.SM_PLAYERTEAMLEADER:
		var tl := Parser.u64(body, 0)
		var lead := Parser.u8(body, 8) != 0
		if co_manager != null:
			var node = co_manager.find_uid(tl)
			if node != null and node.has_method("set_display_name"):
				node.set_display_name(("队长 " if lead else "") + str(node.display_name))
		return true
	if code == MirClient.SM_OFFLINE:
		var off_uid := Parser.u64(body, 0)
		if co_manager != null:
			co_manager.remove_uid(off_uid)
		return true
	if code == MirClient.SM_REVIVE:
		var muid := Parser.u64(body, 0)
		var rx := Parser.u32(body, 8)
		var ry := Parser.u32(body, 12)
		var rd := Parser.u16(body, 16)
		apply_start_scene({"uid": self_uid, "name": self_name, "map_uid": muid,
			"x": rx, "y": ry, "direction": rd})
		return true
	if code == MirClient.SM_CLAIMDELIVERYERROR:
		_log("领取邮件附件失败 code=%d" % int(Parser.u8(body,0)))
		return true

	if code == MirClient.SM_PLAYERCONFIG:
		var pc := Parser.parse_player_config(body)
		magic_keys = pc.get("magic_keys", {})
		runtime_config = pc.get("runtime", {})
		_log("运行配置已同步：快捷键 %d 项，配置 %d 项" % [magic_keys.size(), runtime_config.size()])
		return true
	if code == MirClient.SM_NEXTSTRIKE:
		_log("连击就绪")
		return true
	if code == MirClient.SM_PING:
		var tick := Parser.u32(body, 0)
		var now := Time.get_ticks_msec() & 0xFFFFFFFF
		_log("网络延迟：%d ms" % int(now - tick))
		return true
	if code == MirClient.SM_CREATECHATGROUP:
		var gp := Parser.parse_chat_peer(body)
		if not gp.is_empty() and ui != null and ui.friend_chat != null:
			ui.friend_chat.previews.append(gp)
		return true

	# 排行
	if code == MirClient.SM_RANKINGLIST:
		var rl := Parser.parse_ranking_list(body)
		ranking = rl.get("entries", [])
		if ui != null and ui.ranking_board != null:
			ui.ranking_board.set_entries(int(rl.get("type", 0)), ranking)
		_log("排行榜已更新：%d 条" % ranking.size())
		return true

	# 地面物品失败/商店等错误
	if code == MirClient.SM_PICKUPERROR:
		_log("无法拾取：item=%d" % int(Parser.u32(body,0)))
		return true

	return false


func describe_cell(g: Vector2i) -> Dictionary:
	var hit = co_manager.find_at_grid(g.x, g.y) if co_manager != null else null
	var names: Array = []
	if obj_layer != null:
		for it in obj_layer.ground_items:
			if int(it.get("gx", -1)) == g.x and int(it.get("gy", -1)) == g.y:
				names.append(ItemRecordRef.name(int(it.get("item_id", 0))))
	return {"hit": hit, "items": names}


func handle_world_click(g: Vector2i, shift: bool) -> bool:
	if co_manager == null:
		return false
	var hit = co_manager.find_at_grid(g.x, g.y)
	if hit != null:
		var rec = hit.get("record")
		var node = hit.get("node")
		var kind := str(hit.get("kind", ""))
		if kind == "mon":
			_target_and_attack(rec.uid)
			return true
		if kind == "npc":
			selected_target_uid = rec.uid
			if net != null:
				net.query_npc_name(rec.uid)
				net.npc_event(rec.uid, "", "SYS_ENTER", "")
			return true
		if kind == "ply":
			selected_target_uid = rec.uid
			if shift:
				_target_and_attack(rec.uid)
			else:
				if net != null:
					net.query_corecord(rec.uid)
					net.query_player_name(rec.uid)
				if ui != null and ui.peer_state_board != null:
					ui.peer_state_board.show = true
			return true
	if obj_layer != null:
		for it in obj_layer.ground_items:
			if int(it.get("gx",-1)) == g.x and int(it.get("gy",-1)) == g.y:
				_target_pickup(g, int(it.get("item_id",0)))
				return true
	return false


func _on_map_change_requested(new_map_uid: int, action) -> void:
	var x: int = player.grid.x if player != null else 0
	var y: int = player.grid.y if player != null else 0
	if action != null and int(action.aim_x) > 0: x = int(action.aim_x)
	if action != null and int(action.aim_y) > 0: y = int(action.aim_y)
	apply_start_scene({"uid": self_uid, "name": self_name, "map_uid": new_map_uid,
		"x": x, "y": y, "direction": int(action.direction) if action != null else 5})


func send_move(dst: Vector2i) -> void:
	if player == null:
		return
	var action = ActionNodeRef.make_move(100, player.grid.x, player.grid.y, dst.x, dst.y, 0)
	action.direction = PathFinderRef.get_off_dir(player.grid.x, player.grid.y, dst.x, dst.y)
	_send_action(action)


func request_nearest_pickup() -> void:
	if obj_layer == null or player == null:
		return
	var best = null
	var best_d := 1 << 30
	for it in obj_layer.ground_items:
		var g := Vector2i(int(it.get("gx", -1)), int(it.get("gy", -1)))
		if g.x < 0: continue
		var d := absi(g.x - player.grid.x) + absi(g.y - player.grid.y)
		if d < best_d:
			best_d = d; best = it
	if best != null:
		_target_pickup(Vector2i(int(best["gx"]), int(best["gy"])), int(best.get("item_id", 0)))


func _target_and_attack(uid: int) -> void:
	selected_target_uid = uid
	if net != null:
		net.query_corecord(uid)
	var node = co_manager.find_uid(uid) if co_manager != null else null
	if node == null:
		return
	var tg: Vector2i = node.grid
	if _adjacent(tg):
		_send_attack(uid, tg)
	else:
		var dst := _approach_grid(tg)
		if dst.x >= 0 and player != null and player.request_move(dst, 2):
			pending_action = {"kind": "attack", "uid": uid}


func _send_attack(uid: int, tg: Vector2i) -> void:
	if player == null:
		return
	var dir := PathFinderRef.get_off_dir(player.grid.x, player.grid.y, tg.x, tg.y)
	if selected_magic_id > 0:
		_cast_magic_at(uid, tg, dir)
		return
	var action = ActionNodeRef.make_attack(100, player.grid.x, player.grid.y, uid)
	action.direction = dir
	action.aim_x = tg.x
	action.aim_y = tg.y
	_send_action(action)
	player.play_action_motion(MotionTypeRef.MOTION_ONEVSWING, dir)


func _cast_magic_at(uid: int, tg: Vector2i, dir: int) -> void:
	if net == null or player == null:
		return
	var action = ActionNodeRef.make_spell(100, player.grid.x, player.grid.y,
		tg.x, tg.y, uid, selected_magic_id, 0)
	action.direction = dir
	_send_action(action)
	net.request_magic_damage(selected_magic_id, uid)
	player.play_action_motion(MotionTypeRef.MOTION_SPELL0, dir)
	_spawn_magic_effect(selected_magic_id, tg, dir)


func _target_pickup(g: Vector2i, item_id: int) -> void:
	if item_id <= 0:
		return
	if g == player.grid or _adjacent(g):
		_send_pickup(g)
	else:
		pending_action = {"kind": "pickup", "grid": g}
		if player != null:
			var dst := _approach_grid(g)
			if player.request_move(dst, 2):
				send_move(dst)


func _send_pickup(g: Vector2i) -> void:
	if net == null or player == null:
		return
	var action = ActionNodeRef.make_pickup(100, g.x, g.y)
	action.direction = PathFinderRef.get_off_dir(player.grid.x, player.grid.y, g.x, g.y)
	_send_action(action)
	net.pickup(g.x, g.y, map_uid)
	player.play_action_motion(MotionTypeRef.MOTION_ATTACKMODE, action.direction)


func _send_action(action) -> void:
	if net != null and action != null:
		net.send_action(self_uid, map_uid, action.to_bytes())


func on_player_move_finished() -> void:
	if pending_action.is_empty():
		return
	var p := pending_action
	pending_action = {}
	if str(p.get("kind","")) == "attack":
		var node = co_manager.find_uid(int(p["uid"])) if co_manager != null else null
		if node != null and _adjacent(node.grid):
			_send_attack(int(p["uid"]), node.grid)
	elif str(p.get("kind","")) == "pickup":
		_send_pickup(p["grid"])


func _adjacent(tg: Vector2i) -> bool:
	return player != null and maxi(absi(player.grid.x - tg.x), absi(player.grid.y - tg.y)) <= 1


func _approach_grid(tg: Vector2i) -> Vector2i:
	if player == null:
		return tg
	var best: Vector2i = player.grid
	var best_d := 1 << 30
	for dy in range(-1, 2):
		for dx in range(-1, 2):
			if dx == 0 and dy == 0: continue
			var g := tg + Vector2i(dx, dy)
			if main != null and main.has_method("can_move_to") and not main.can_move_to(g.x, g.y):
				continue
			var d := absi(g.x - player.grid.x) + absi(g.y - player.grid.y)
			if d < best_d:
				best_d = d; best = g
	return best


func handle_command(text: String) -> bool:
	var t := text.strip_edges()
	if t.is_empty() or net == null:
		return false
	var parts := t.split(" ", false)
	if parts.size() >= 1 and parts[0] == "/debug":
		if main != null and main.get("debug_overlay") != null:
			main.debug_overlay.toggle()
		return true
	if parts.size() >= 3 and parts[0] == "/tp":
		if net != null:
			net.request_space_move(map_uid, int(parts[1]), int(parts[2]))
		return true
	if parts.size() >= 1 and parts[0] == "/querygold":
		if net != null:
			net.query_gold()
		return true
	if parts.size() >= 1 and parts[0] == "/querymap":
		if net != null:
			net.query_base_map_uid(map_id)
		return true
	if parts.size() >= 2 and parts[0] == "/team":
		match parts[1]:
			"create": net.create_team()
			"leave": net.request_leave_team(self_uid)
			"join": 
				if parts.size() >= 3: net.request_join_team(int(parts[2]))
			"info":
				if parts.size() >= 3: net.query_team_info(int(parts[2]))
			"leader":
				if parts.size() >= 3: net.transfer_team_leader(int(parts[2]))
			"follow":
				if parts.size() >= 3: net.set_follow_team_leader(parts[2] == "1")
		return true
	if parts.size() >= 2 and parts[0] == "/guild":
		match parts[1]:
			"invite":
				if parts.size() >= 3: net.invite_guild_member(parts[2])
			"leave": net.leave_guild()
			"kick":
				if parts.size() >= 3: net.kick_guild_member(int(parts[2]))
			"leader":
				if parts.size() >= 3: net.transfer_guild_leader(int(parts[2]))
			"disband": net.disband_guild()
			"info": net.query_guild_info()
			"notice":
				net.set_guild_notice(t.substr(t.find("notice") + 7).strip_edges())
		return true
	if parts.size() >= 1 and parts[0] == "/ranking":
		var rt := 1
		if parts.size() >= 2: rt = int(parts[1])
		net.query_ranking(rt)
		return true
	if parts.size() >= 3 and parts[0] == "/friend" and parts[1] == "add":
		net.add_friend(int(parts[2]))
		return true
	if t.begins_with("!"):
		net.player_broadcast(t.substr(1)); _log("[广播] " + t.substr(1)); return true
	if t.begins_with("@"):
		net.guild_chat(t.substr(1)); _log("[行会] " + t.substr(1)); return true
	if t.begins_with("/"):
		net.say(t.substr(1)); return true
	net.say(t)
	return true


func _on_config_change(_w, key: String, value) -> void:
	if key.contains("背景音乐"):
		_audio().set_bgm_volume(float(value) / 100.0)
	elif key.contains("音效音量"):
		_audio().set_sfx_volume(float(value) / 100.0)
	_log("设置 %s = %s" % [key, str(value)])


func _on_skill_click(_w, magic_name: String) -> void:
	selected_magic_id = MagicDBRef.find_id_by_name(magic_name)
	if selected_magic_id > 0:
		_log("选择技能：%s，点击怪物施放" % magic_name)


func activate_inventory_item(item) -> void:
	if item == null:
		return
	if ui != null and ui.stall_board2 != null and ui.stall_board2.show:
		if net != null:
			net.add_stall_item(int(item.item_id), int(item.seq_id),
				maxi(1, ItemRecordRef.price(int(item.item_id))))
			_log("已请求上架摆摊：%s" % ItemRecordRef.name(int(item.item_id)))
		return
	if ui != null and ui.auction2 != null and ui.auction2.show:
		if net != null:
			net.register_auction_item(int(item.item_id), int(item.seq_id),
				maxi(1, ItemRecordRef.price(int(item.item_id))), "玩家寄售")
			_log("已请求登记拍卖：%s" % ItemRecordRef.name(int(item.item_id)))
		return
	use_inventory_item(item)


func use_inventory_item(item) -> void:
	if net == null or item == null:
		return
	var iid := int(item.item_id)
	if ItemRecordRef.is_potion(iid) or ItemRecordRef.is_dope(iid):
		net.consume_item(iid, int(item.seq_id), 1)
		return
	for slot in range(ProtocolDefRef.WLG_BEGIN, ProtocolDefRef.WLG_END):
		if ItemRecordRef.wearable(iid, slot):
			net.request_equip_wear(iid, int(item.seq_id), slot)
			return
	if ItemRecordRef.beltable(iid):
		for slot in range(6):
			if ui == null or ui.quick_access == null or ui.quick_access.belt[slot] == null:
				net.request_equip_belt(iid, int(item.seq_id), slot)
				return
	_log("该物品当前不可使用或穿戴")


func unequip_slot(slot: int) -> void:
	if net != null:
		net.request_grab_wear(slot)


func _on_quick_slot(slot: int) -> void:
	if net == null:
		return
	if ui != null and ui.quick_access != null and slot >= 0 and slot < ui.quick_access.belt.size():
		if ui.quick_access.belt[slot] != null:
			net.request_consume_belt(slot)


func _on_npc_event(_w, event_path: String) -> void:
	if net != null and selected_target_uid != 0:
		net.npc_event(selected_target_uid, "", event_path, "")


func _on_secured_action(_w, idx: int) -> void:
	if net == null or ui == null or ui.secured_board == null:
		return
	var items: Array = ui.secured_board.items
	if idx < 0 or idx >= items.size():
		return
	var it = items[idx]
	net.request_retrieve_secured_item(int(it.item_id), int(it.seq_id))


func _on_purchase_buy(item_id: int, seq_id: int, _price: int) -> void:
	if net != null and ui != null and ui.purchase_board != null:
		net.buy(int(ui.purchase_board.npc_uid), item_id, seq_id, 1)


func _on_auction_buy(idx: int) -> void:
	var e = ui.auction2.selected_entry() if ui.auction2 != null else null
	if e != null and net != null: net.buy_auction_item(int(e.get("auction_id",0)))

func _on_auction_cancel() -> void:
	var e = ui.auction2.selected_entry() if ui.auction2 != null else null
	if e != null and net != null: net.unregister_auction_item(int(e.get("auction_id",0)))

func _on_auction_search(_keyword: String, cat: int, _page: int) -> void:
	if net != null: net.query_auction_item_list(cat)


func _on_stall_buy(idx: int, price: int) -> void:
	if net == null or ui == null or ui.stall_board2 == null: return
	var it = ui.stall_board2.item_list()[idx] if idx >= 0 and idx < ui.stall_board2.item_list().size() else null
	if it != null:
		net.buy_stall_item(int(ui.stall_board2.stall.get("seller_uid",0)),
			int(it.get("item_id",0)), int(it.get("seq_id",0)),
			int(it.get("count",1)), price)

func _on_start_stall(_w) -> void:
	if net != null: net.start_stall(self_name + "的摊位")

func _on_close_stall(_w) -> void:
	if net != null: net.close_stall()

func _on_stall_item_op(_w) -> void:
	if net == null or ui == null or ui.stall_board2 == null:
		return
	var it = ui.stall_board2.selected_item()
	if it != null and ui.stall_board2.is_seller():
		net.remove_stall_item(int(it.get("item_id", 0)), int(it.get("seq_id", 0)))


func _on_direct_trade_lock() -> void:
	if net == null or ui == null or ui.direct_trade == null:
		return
	var tb = ui.direct_trade
	if tb.confirmed:
		net.commit_direct_trade(int(tb.peer_uid))
	else:
		net.update_direct_trade_offer(int(tb.peer_uid), tb.my_gold(), not tb.locked, tb.my_items)
		tb.apply_lock_ack()

func _cancel_trade() -> void:
	if net != null and ui != null and ui.direct_trade != null:
		net.cancel_direct_trade(int(ui.direct_trade.peer_uid))


func _append_chat_message(m: Dictionary) -> void:
	if ui == null or ui.friend_chat == null:
		return
	ui.friend_chat.add_message(int(m.get("from",0)) == UidRef.get_player_dbid(self_uid),
		str(m.get("from", "?")), str(m.get("message", "")))


func _on_friend_send(text: String) -> void:
	if net == null or ui == null or ui.friend_chat == null: return
	var peer: String = str(ui.friend_chat.cur_peer)
	if peer.is_valid_int():
		net.chat_message(int(peer), text)
	else:
		_log("好友会话 ID 无效: " + peer)

func _on_friend_search(keyword: String) -> void:
	if net != null: net.query_chat_peer_list(keyword)

func _on_friend_add(peer) -> void:
	if net == null: return
	if peer is Dictionary and peer.has("id"):
		net.add_friend(int(peer["id"]))
	elif str(peer).is_valid_int():
		net.add_friend(int(peer))

func _on_friend_open(peer_id: String) -> void:
	if ui != null and ui.friend_chat != null:
		ui.friend_chat.open_chat(peer_id, peer_id)

func _on_peer_trade() -> void:
	if net != null and selected_target_uid != 0:
		net.request_direct_trade(selected_target_uid)

func _on_peer_add_friend() -> void:
	if net != null and selected_target_uid != 0:
		net.add_friend(selected_target_uid)

func _on_team_member_click(_w, member) -> void:
	if net != null and member is Dictionary and member.has("uid"):
		net.request_join_team(int(member["uid"]))

func _on_guild_chat(_w, text: String) -> void:
	if net != null:
		net.guild_chat(text)
		if ui.guild_board != null:
			ui.guild_board.chat_lines.append({"name": self_name, "text": text})

func _on_guild_notice(_w, text: String) -> void:
	if net != null: net.set_guild_notice(text)


func _spawn_magic_effect(magic_id: int, g: Vector2i, dir: int) -> void:
	var e := MagicDBRef.gfx_entry(magic_id)
	if e.is_empty():
		return
	var node := MagicEffectRef.new()
	add_child(node)
	node.setup(magic_id, e, g, clampi(dir, 0, 15), bool(e.get("loop", false)))


func _refresh_player_appearance(d: Dictionary) -> void:
	if main == null or player == null:
		return
	var wear: Dictionary = d.get("wear", {})
	var dress := 0; var helmet := 0; var weapon := 0
	if wear.has(ProtocolDefRef.WLG_DRESS):
		dress = ItemRecordRef.pkg_gfx_id(int(wear[ProtocolDefRef.WLG_DRESS].item_id))
	if wear.has(ProtocolDefRef.WLG_HELMET):
		helmet = ItemRecordRef.pkg_gfx_id(int(wear[ProtocolDefRef.WLG_HELMET].item_id))
	if wear.has(ProtocolDefRef.WLG_WEAPON):
		weapon = ItemRecordRef.pkg_gfx_id(int(wear[ProtocolDefRef.WLG_WEAPON].item_id))
	if player.get("_human") != null:
		player._human.set_appearance(dress, int(d.get("hair", 1)), helmet, weapon,
			int(d.get("gender", gender)) == 0, Color.WHITE)


func _refresh_quests() -> void:
	if ui != null and ui.quest_board != null:
		var arr: Array = []
		for k in quests.keys(): arr.append(quests[k])
		ui.quest_board.set_quests(arr)
		ui.quest_board.show = arr.size() > 0


func _refresh_minimap() -> void:
	if ui == null or ui.mini_map2 == null or player == null:
		return
	var mini := MapRecordRef.minimap_of(map_id)
	if mini >= 0:
		ui.mini_map2.set_map(mini, 128, 128)
	ui.mini_map2.set_hero(player.grid.x, player.grid.y)


func _process(dt: float) -> void:
	_map_refresh_acc += dt
	if _map_refresh_acc < 0.2:
		return
	_map_refresh_acc = 0.0
	if player == null:
		return
	if player.grid != _last_hero_grid:
		_last_hero_grid = player.grid
		_refresh_minimap()
		if ui != null and ui.ctrl_board != null:
			ui.ctrl_board.map_gx = player.grid.x
			ui.ctrl_board.map_gy = player.grid.y


func _act(_motion: int) -> void:
	if net == null:
		return
	# 上下马由物品/动作链路处理；这里保留 UI 点击反馈
	_log("骑乘操作请求")


func _audio():
	return get_node_or_null("/root/Audio")


func _log(text: String, log_type: int = 1) -> void:
	if ui != null and ui.ctrl_board != null:
		ui.ctrl_board.add_log(text, log_type)
	elif ui != null and ui.has_method("_push_sys_msg"):
		ui._push_sys_msg(text)
	else:
		print("[world] ", text)


static func uidf_dbid(uid: int) -> int:
	return UidRef.get_player_dbid(uid)
