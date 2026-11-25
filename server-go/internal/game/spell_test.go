package game

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/lff2116788383/godot-mir2/server-go/internal/common"
	"github.com/lff2116788383/godot-mir2/server-go/internal/protocol"
	"github.com/lff2116788383/godot-mir2/server-go/internal/world"
)

// ==================== B5 第三批测试：技能 / 死亡复活 / 金币商店 ====================

// b5spellFixture 在 b5Fixture 基础上装好物品表与魔法表。
type b5spellFixture struct {
	*b5Fixture
	Items     *ItemDB
	Magics    *MagicDB
	Spells    *SpellHandler
	Deaths    *DeathHandler
	Shops     *ShopHandler
	Ticker    *MapTicker
	clockMS   int64
	clockStep int64
}

func newB5SpellFixture(t *testing.T) *b5spellFixture {
	t.Helper()
	base := newB5Fixture(t)

	items, err := LoadItemDB("../../content/items.json")
	if err != nil {
		t.Fatalf("load item db: %v", err)
	}
	magics, err := LoadMagicDB("../../content/magics.json")
	if err != nil {
		t.Fatalf("load magic db: %v", err)
	}
	GoldItemID = items.GoldID()

	f := &b5spellFixture{
		b5Fixture: base, Items: items, Magics: magics,
		clockMS: 1000, clockStep: 100,
	}
	f.Spells = &SpellHandler{
		Players: base.Players, Worlds: base.Worlds.Worlds,
		Items: items, Magics: magics, Broadcast: base.Cast,
		Sessions: base.Sessions, Logger: discardLogger(),
	}
	f.Deaths = &DeathHandler{
		Players: base.Players, Worlds: base.Worlds.Worlds,
		Broadcast: base.Cast, Logger: discardLogger(), Chars: base.Chars,
	}
	f.Shops = &ShopHandler{
		Players: base.Players, Worlds: base.Worlds.Worlds,
		Items: items, Broadcast: base.Cast, Logger: discardLogger(),
		Chars: base.Chars,
	}
	f.Actions.Spells = f.Spells

	// 世界时钟用可控时钟，避免测试依赖真实时间
	f.Ticker = NewMapTicker(base.Worlds.Worlds, base.Worlds, base.Cast,
		base.Players, discardLogger())
	f.Ticker.SetClock(func() int64 { return f.clockMS })
	base.Worlds.Ticker = f.Ticker
	// 时钟基准 1000ms（不用 0）：施法时若世界时钟恰好是 0，
	// 冷却记录下来的值也是 0，断言「已记录冷却」时 0 与「没记录」
	// 无法区分（map 的零值就是 0）。基准非 0 才能区分这两种状态。
	f.clockMS = 1000

	return f
}

// mkPlayer 建玩家并推进一次世界时钟。
//
// 为什么要顺带推进：地图的 NowMS 初始是 0（MapState 零值），
// 而技能冷却记录的是「施放时刻的世界时钟」。若进场后不推进，
// 第一次施法记下 0，后续任何一次 advanceClock 都会让
// `now - 0` 直接跨过冷却窗口 —— 测试就测不出「冷却中」这个状态。
func (f *b5spellFixture) mkPlayer(t *testing.T, dbid uint32, name string, x, y int) *Player {
	t.Helper()
	p := f.b5Fixture.makePlayer(t, dbid, name, x, y)
	f.Ticker.Tick(f.ctx)
	return p
}

// advanceClock 推进世界时钟（顺带跑一次 tick）。
func (f *b5spellFixture) advanceClock(ms int64) {
	f.clockMS += ms
	f.Ticker.Tick(f.ctx)
}

// learnMagic 让玩家学会某技能并返回 magicID。
func (f *b5spellFixture) learnMagic(t *testing.T, p *Player, name string) uint32 {
	t.Helper()
	id, ok := f.Magics.FindByName(name)
	if !ok {
		t.Fatalf("magic %q not found in db", name)
	}
	p.Magics = append(p.Magics, id)
	if p.MagicCooldown == nil {
		p.MagicCooldown = make(map[uint32]int64)
	}
	if p.MagicTrain == nil {
		p.MagicTrain = make(map[uint32]int)
	}
	return id
}

// spellAction 构造一个施法动作。
func spellAction(p *Player, magicID uint32, aimUID uint64, aimX, aimY int) common.ActionNode {
	return common.ActionNode{
		Type:      common.ActionSpell,
		Direction: p.Direction,
		X:         int16(p.X),
		Y:         int16(p.Y),
		AimX:      int16(aimX),
		AimY:      int16(aimY),
		AimUID:    aimUID,
		ExtParam:  magicIDExtParam(magicID),
	}
}

// magicIDExtParam 把 magicID 编码进 ActionNode.ExtParam（前 4 字节小端）。
func magicIDExtParam(magicID uint32) [8]byte {
	var ext [8]byte
	binary.LittleEndian.PutUint32(ext[:4], magicID)
	return ext
}

// spawnMonster 在地图上刷一只怪，返回它的 UID。
func (f *b5spellFixture) spawnMonster(t *testing.T, mapID uint32, uid uint64,
	x, y int, hp int32, rec world.MonsterRecord) {
	t.Helper()
	entity := world.Entity{
		UID: uid, Type: world.EntityMonster, Name: rec.Name,
		X: x, Y: y, Direction: world.DirDown,
		HP: hp, MaxHP: hp, MP: rec.MP, MaxMP: rec.MP,
		Speed: 600, ViewRange: 10,
		Stats: world.CombatStatsOf(rec),
	}
	if _, err := f.Worlds.Worlds.SpawnMonster(f.ctx, mapID, entity, rec); err != nil {
		t.Fatalf("spawn monster: %v", err)
	}
}

// ---------- 魔法表 ----------

func TestMagicDBLoadsAllRecords(t *testing.T) {
	db, err := LoadMagicDB("../../content/magics.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if db.Count() != 116 {
		t.Fatalf("magic count = %d, want 116", db.Count())
	}
	if db.LearnableCount() != 63 {
		t.Fatalf("learnable = %d, want 63", db.LearnableCount())
	}

	// 关键 ID 必须与客户端一致（magicrecord.inc 的数组下标）
	cases := []struct {
		name string
		want uint32
	}{
		{"物理攻击", 1},
		{"雷电术", 14},
		{"魔法盾", 15},
		{"召唤骷髅", 16},
		{"火球术", 17},
		{"治愈术", 19},
	}
	for _, c := range cases {
		got, ok := db.FindByName(c.name)
		if !ok {
			t.Fatalf("magic %q not found", c.name)
		}
		if got != c.want {
			t.Fatalf("%q magicID = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestMagicRecordFields(t *testing.T) {
	db, err := LoadMagicDB("../../content/magics.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	lightning, ok := db.FindByName("雷电术")
	if !ok {
		t.Fatal("雷电术 not found")
	}
	rec, _ := db.Get(lightning)
	if rec.ElemID != ElemLight {
		t.Fatalf("雷电术 elem = %q, want light", rec.ElemID)
	}
	if rec.MP != 7 || rec.CoolDown != 800 {
		t.Fatalf("雷电术 mp=%d cd=%d, want 7/800", rec.MP, rec.CoolDown)
	}
	if rec.Power[0] != 15 || rec.Power[1] != 19 {
		t.Fatalf("雷电术 power = %v, want [15 19]", rec.Power)
	}
	// 没写 castRange 的条目默认 CRT_LONG（magicrecord.hpp:288）
	if rec.CastRange.TypeID != CastRangeLong {
		t.Fatalf("雷电术 castRange = %v, want CRT_LONG", rec.CastRange.TypeID)
	}
	if !rec.Learnable {
		t.Fatal("雷电术 should be learnable")
	}

	// 特效条目不可学
	if fx, ok := db.FindByName("魔法特效_雷电术"); ok {
		fxRec, _ := db.Get(fx)
		if fxRec.Learnable {
			t.Fatal("魔法特效_雷电术 should not be learnable")
		}
	}
}

func TestMagicMPCost(t *testing.T) {
	rec := MagicRecord{MP: 7, MPInc: 6}
	if got := rec.MPCost(0); got != 7 {
		t.Fatalf("train 0 cost = %d, want 7", got)
	}
	if got := rec.MPCost(1); got != 13 {
		t.Fatalf("train 1 cost = %d, want 13", got)
	}
	if got := rec.MPCost(3); got != 25 {
		t.Fatalf("train 3 cost = %d, want 25", got)
	}
	// 越界等级要钳住，不能算出负数或超长
	if got := rec.MPCost(-5); got != 7 {
		t.Fatalf("negative train cost = %d, want 7", got)
	}
	if got := rec.MPCost(99); got != 25 {
		t.Fatalf("overflow train cost = %d, want 25", got)
	}
}

func TestElementFromName(t *testing.T) {
	cases := map[string]world.Element{
		"火": world.ElemFire, "冰": world.ElemIce, "雷": world.ElemLight,
		"风": world.ElemWind, "神圣": world.ElemHoly, "暗黑": world.ElemDark,
		"幻影": world.ElemPhantom, "": world.ElemNone, "奇怪": world.ElemNone,
	}
	for name, want := range cases {
		if got := world.ElementFromName(name); got != want {
			t.Fatalf("ElementFromName(%q) = %v, want %v", name, got, want)
		}
	}
}

// ---------- 属性 ----------

func TestCombatStatsIncludeEquipment(t *testing.T) {
	db, err := LoadItemDB("../../content/items.json")
	if err != nil {
		t.Fatalf("load items: %v", err)
	}

	p := &Player{Job: 1, Level: 10, Wear: make(map[int]*Item)}
	base := p.CombatStatsFor(db)

	// 找一件有 dc 加成的武器
	var weaponID uint32
	for id, rec := range db.records {
		if rec.Type == TypeWeapon && rec.Equip != nil && rec.Equip.DC[1] > 0 {
			weaponID = id
			break
		}
	}
	if weaponID == 0 {
		t.Fatal("no weapon with dc bonus found")
	}
	p.Wear[WLGWeapon] = &Item{ItemID: weaponID, Count: 1}
	withWeapon := p.CombatStatsFor(db)

	if withWeapon.DC.Max <= base.DC.Max {
		t.Fatalf("equipping weapon should raise DC: base=%d with=%d", base.DC.Max, withWeapon.DC.Max)
	}

	// 物品表缺失时不能崩，退化成等级属性
	if nilStats := p.CombatStatsFor(nil); nilStats.DC.Max != base.DC.Max {
		t.Fatalf("nil item db should fall back to level stats, got %+v", nilStats.DC)
	}
}

func TestCombatStatsBasicSwordHitBonus(t *testing.T) {
	db, err := LoadItemDB("../../content/items.json")
	if err != nil {
		t.Fatalf("load items: %v", err)
	}
	p := &Player{Job: 1, Level: 10, Wear: make(map[int]*Item)}
	before := p.CombatStatsFor(db)

	swordID, ok := db.FindByName("基本剑术")
	if !ok {
		t.Skip("基本剑术 not in item db")
	}
	p.Magics = []uint32{swordID}
	after := p.CombatStatsFor(db)

	if after.DCHit != before.DCHit+2 {
		t.Fatalf("基本剑术 should give +2 dcHit: %d -> %d", before.DCHit, after.DCHit)
	}
}

// ---------- 施法 ----------

func TestSpellRequiresLearnedMagic(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "mage", 100, 100)
	f.Cast.reset()

	fireball, _ := f.Magics.FindByName("火球术")
	action := spellAction(p, fireball, 0, 101, 100)
	if err := f.Spells.Cast(f.ctx, p.ChannelID, action); err != nil {
		t.Fatalf("cast: %v", err)
	}

	// 没学 -> 不广播 SM_CASTMAGIC，但要回 reportStand 让客户端收敛
	if n := f.Cast.count(protocol.SM_CASTMAGIC); n != 0 {
		t.Fatalf("unlearned magic should not cast, got %d SM_CASTMAGIC", n)
	}
	if n := len(f.Cast.of(p.ChannelID, protocol.SM_ACTION)); n == 0 {
		t.Fatal("rejected cast should send reportStand so client can recover")
	}
}

func TestSpellRequiresMP(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "mage", 100, 100)
	p.Job = 2
	p.MP = 1 // 火球术要 3 点
	f.learnMagic(t, p, "火球术")
	fireball, _ := f.Magics.FindByName("火球术")
	f.Cast.reset()

	if err := f.Spells.Cast(f.ctx, p.ChannelID, spellAction(p, fireball, 0, 101, 100)); err != nil {
		t.Fatalf("cast: %v", err)
	}
	if n := f.Cast.count(protocol.SM_CASTMAGIC); n != 0 {
		t.Fatalf("no MP should not cast, got %d", n)
	}
	if p.MP != 1 {
		t.Fatalf("MP should not be consumed on failure, got %d", p.MP)
	}
}

func TestSpellConsumesMPAndBroadcastsCast(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "mage", 100, 100)
	p.Job = 2
	p.MP = 50
	lightning := f.learnMagic(t, p, "雷电术")

	// 目标：一只怪
	rec := world.MonsterRecord{Name: "鸡", HP: 500, Exp: 10}
	monsterUID := mustMonsterUID(t, 1001)
	f.spawnMonster(t, 1, monsterUID, 102, 100, 500, rec)

	f.Cast.reset()
	action := spellAction(p, lightning, monsterUID, 102, 100)
	if err := f.Spells.Cast(f.ctx, p.ChannelID, action); err != nil {
		t.Fatalf("cast: %v", err)
	}

	// MP 扣了 7
	if p.MP != 43 {
		t.Fatalf("MP = %d, want 43", p.MP)
	}
	// 广播了 SM_CASTMAGIC
	if n := f.Cast.count(protocol.SM_CASTMAGIC); n == 0 {
		t.Fatal("should broadcast SM_CASTMAGIC")
	}
	// 冷却已记录
	if p.MagicCooldown[lightning] == 0 {
		t.Fatal("cooldown should be recorded")
	}
}

func TestSpellCooldownBlocksRepeatedCast(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "mage", 100, 100)
	p.Job = 2
	p.MP = 500
	lightning := f.learnMagic(t, p, "雷电术")

	rec := world.MonsterRecord{Name: "鸡", HP: 5000, Exp: 10}
	monsterUID := mustMonsterUID(t, 1002)
	f.spawnMonster(t, 1, monsterUID, 102, 100, 5000, rec)

	action := spellAction(p, lightning, monsterUID, 102, 100)
	if err := f.Spells.Cast(f.ctx, p.ChannelID, action); err != nil {
		t.Fatalf("first cast: %v", err)
	}
	mpAfterFirst := p.MP

	// 冷却 800ms，时钟只走 100ms -> 第二次必须被拒
	f.advanceClock(100)
	f.Cast.reset()
	if err := f.Spells.Cast(f.ctx, p.ChannelID, action); err != nil {
		t.Fatalf("second cast: %v", err)
	}
	if n := f.Cast.count(protocol.SM_CASTMAGIC); n != 0 {
		t.Fatalf("cooling magic should not cast, got %d", n)
	}
	if p.MP != mpAfterFirst {
		t.Fatalf("MP should not drop while cooling: %d -> %d", mpAfterFirst, p.MP)
	}

	// 时钟走过冷却 -> 能放了
	f.advanceClock(800)
	f.Cast.reset()
	if err := f.Spells.Cast(f.ctx, p.ChannelID, action); err != nil {
		t.Fatalf("third cast: %v", err)
	}
	if n := f.Cast.count(protocol.SM_CASTMAGIC); n == 0 {
		t.Fatal("should cast again after cooldown")
	}
}

func TestSpellDamageKillsMonsterAndGivesExp(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "wizard", 100, 100)
	p.Job = 2
	p.MP = 500
	p.Level = 30 // 提高 MC，确保能打死
	_ = f.Worlds.Worlds.SetStats(f.ctx, 1, p.UID, p.CombatStatsFor(f.Items))

	lightning := f.learnMagic(t, p, "雷电术")

	// 掉落 1 件「金创药（小）」，必掉
	potionID, ok := f.Items.FindByName("金创药（小）")
	if !ok {
		t.Fatal("金创药（小）not in item db")
	}
	rec := world.MonsterRecord{
		Name: "鸡", HP: 1, Exp: 25,
		DropItems: [][]world.DropItem{{{Item: "金创药（小）", ProbRecip: 1, Count: 1}}},
	}
	monsterUID := mustMonsterUID(t, 1003)
	f.spawnMonster(t, 1, monsterUID, 102, 100, 1, rec)

	expBefore := p.Exp
	f.Cast.reset()
	action := spellAction(p, lightning, monsterUID, 102, 100)

	// ★ 法术有 25% 基础未命中（HitProbability 下限 0.5、双方命中/闪避都 0
	//   时是 0.75），所以不能「放一次就断言怪死了」。
	//   真实玩家也是连放几发，这里循环到怪死为止（最多 10 次）。
	const maxCasts = 10
	for i := 0; i < maxCasts; i++ {
		if err := f.Spells.Cast(f.ctx, p.ChannelID, action); err != nil {
			t.Fatalf("cast #%d: %v", i, err)
		}
		if e, found, err := f.Worlds.Worlds.Entity(f.ctx, 1, monsterUID); err == nil && found && e.HP <= 0 {
			break
		}
		// 冷却 800ms，推进到刚过冷却
		f.advanceClock(int64(rec0CoolDown(f, lightning) + 10))
	}

	// 怪死了
	e, found, err := f.Worlds.Worlds.Entity(f.ctx, 1, monsterUID)
	if err != nil || !found {
		t.Fatalf("monster gone entirely, want dead corpse: found=%v err=%v", found, err)
	}
	if e.HP > 0 {
		t.Fatalf("monster should be dead after %d casts, HP=%d", maxCasts, e.HP)
	}

	// 经验给了
	if p.Exp <= expBefore {
		t.Fatalf("should gain exp: %d -> %d", expBefore, p.Exp)
	}

	// 掉落落地了：地面应该有物品
	ids, err := f.Worlds.Worlds.VisibleEntities(f.ctx, 1, p.UID)
	_ = ids
	if err != nil {
		t.Fatalf("visible: %v", err)
	}
	_ = potionID
}

func TestSpellHealRestoresHP(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "taoist", 100, 100)
	p.Job = 3
	p.MP = 100
	p.HP = 20
	p.MaxHP = 100
	_ = f.Worlds.Worlds.MutateEntity(f.ctx, 1, p.UID, func(e *world.Entity) error {
		e.HP = 20
		e.MaxHP = 100
		return nil
	})

	heal := f.learnMagic(t, p, "治愈术")
	rec, _ := f.Magics.Get(heal)

	f.Cast.reset()
	// 治疗自己：aimUID 传 0（客户端点自己时就是这样）
	if err := f.Spells.Cast(f.ctx, p.ChannelID, spellAction(p, heal, 0, p.X, p.Y)); err != nil {
		t.Fatalf("cast: %v", err)
	}

	expected := int32(20) + int32(rec.Power[0])
	if p.HP != expected {
		t.Fatalf("HP = %d, want %d (20 + power %d)", p.HP, expected, rec.Power[0])
	}
	if p.HP > p.MaxHP {
		t.Fatalf("heal should not exceed MaxHP, got %d/%d", p.HP, p.MaxHP)
	}
}

func TestSpellHealCapsAtMaxHP(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "taoist", 100, 100)
	p.Job = 3
	p.MP = 100
	p.HP = 98
	p.MaxHP = 100

	heal := f.learnMagic(t, p, "治愈术")
	if err := f.Spells.Cast(f.ctx, p.ChannelID, spellAction(p, heal, 0, p.X, p.Y)); err != nil {
		t.Fatalf("cast: %v", err)
	}
	if p.HP != 100 {
		t.Fatalf("HP should cap at MaxHP 100, got %d", p.HP)
	}
}

func TestSpellDeadCasterCannotCast(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "ghost", 100, 100)
	p.Job = 2
	p.MP = 100
	p.HP = 0
	fireball := f.learnMagic(t, p, "火球术")
	f.Cast.reset()

	if err := f.Spells.Cast(f.ctx, p.ChannelID, spellAction(p, fireball, 0, 101, 100)); err != nil {
		t.Fatalf("cast: %v", err)
	}
	if n := f.Cast.count(protocol.SM_CASTMAGIC); n != 0 {
		t.Fatalf("dead caster should not cast, got %d", n)
	}
}

func TestSpellZeroMagicIDIsRejected(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "noob", 100, 100)
	p.Job = 2
	p.MP = 100
	f.Cast.reset()

	// magicID=0 不能兜底成物理攻击（那会让法术动作打出物理伤害）
	action := common.ActionNode{
		Type: common.ActionSpell, Direction: p.Direction,
		X: int16(p.X), Y: int16(p.Y), AimX: 101, AimY: 100,
	}
	if err := f.Spells.Cast(f.ctx, p.ChannelID, action); err != nil {
		t.Fatalf("cast: %v", err)
	}
	if n := f.Cast.count(protocol.SM_CASTMAGIC); n != 0 {
		t.Fatalf("magicID=0 should be rejected, got %d casts", n)
	}
	if n := f.Cast.count(protocol.SM_STRIKEGRID); n != 0 {
		t.Fatalf("magicID=0 must not produce damage, got %d strike grids", n)
	}
}

// ---------- 快捷键 ----------

func TestSetMagicKeyValidation(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "mage", 100, 100)
	p.Job = 2
	p.MagicKeys = make(map[uint32]uint8)
	lightning := f.learnMagic(t, p, "雷电术")

	// 合法键
	if err := f.Spells.SetMagicKey(f.ctx, p.ChannelID,
		protocol.CMSetMagicKey{MagicID: lightning, Key: 'a'}); err != nil {
		t.Fatalf("set key: %v", err)
	}
	if p.MagicKeys[lightning] != 'a' {
		t.Fatalf("key not saved, got %v", p.MagicKeys)
	}

	// 1-6 被腰带占用，必须拒绝
	for _, key := range []byte{'1', '2', '3', '4', '5', '6'} {
		if ValidMagicKey(key) {
			t.Fatalf("key %q should be reserved for belt", key)
		}
	}
	// 0 与 7-9 合法
	for _, key := range []byte{'0', '7', '8', '9'} {
		if !ValidMagicKey(key) {
			t.Fatalf("key %q should be valid", key)
		}
	}
	// 大写字母不合法（原版只认小写）
	if ValidMagicKey('A') {
		t.Fatal("uppercase key should be invalid")
	}
}

func TestSetMagicKeyRejectsUnlearned(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "mage", 100, 100)
	p.Job = 2
	p.MagicKeys = make(map[uint32]uint8)

	lightning, _ := f.Magics.FindByName("雷电术")
	// 没学却设键 -> 不写入
	if err := f.Spells.SetMagicKey(f.ctx, p.ChannelID,
		protocol.CMSetMagicKey{MagicID: lightning, Key: 'a'}); err != nil {
		t.Fatalf("set key: %v", err)
	}
	if _, exists := p.MagicKeys[lightning]; exists {
		t.Fatal("should not bind key for unlearned magic")
	}
}

func TestSetMagicKeyRebindsOldMagic(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "mage", 100, 100)
	p.Job = 2
	p.MagicKeys = make(map[uint32]uint8)
	lightning := f.learnMagic(t, p, "雷电术")
	fireball := f.learnMagic(t, p, "火球术")

	_ = f.Spells.SetMagicKey(f.ctx, p.ChannelID,
		protocol.CMSetMagicKey{MagicID: lightning, Key: 'a'})
	// 同一个键换绑到火球术：雷电术的绑定必须被清掉
	_ = f.Spells.SetMagicKey(f.ctx, p.ChannelID,
		protocol.CMSetMagicKey{MagicID: fireball, Key: 'a'})

	if _, exists := p.MagicKeys[lightning]; exists {
		t.Fatal("old magic binding should be removed when key is reused")
	}
	if p.MagicKeys[fireball] != 'a' {
		t.Fatal("new binding not set")
	}
}

// ---------- 死亡与复活 ----------

func TestRequestDieSetsZeroHP(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "victim", 100, 100)
	f.Cast.reset()

	if err := f.Deaths.RequestDie(f.ctx, p.ChannelID); err != nil {
		t.Fatalf("die: %v", err)
	}

	if p.HP != 0 {
		t.Fatalf("HP = %d, want 0", p.HP)
	}
	// world 侧也必须归零
	e, ok, err := f.Worlds.Worlds.Entity(f.ctx, 1, p.UID)
	if err != nil || !ok {
		t.Fatalf("entity missing: %v", err)
	}
	if e.HP != 0 {
		t.Fatalf("world entity HP = %d, want 0", e.HP)
	}

	if n := f.Cast.count(protocol.SM_NOTIFYDEAD); n == 0 {
		t.Fatal("should broadcast SM_NOTIFYDEAD")
	}
	if n := f.Cast.count(protocol.SM_DEADFADEOUT); n == 0 {
		t.Fatal("should broadcast SM_DEADFADEOUT")
	}
}

func TestRequestDieIdempotent(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "victim", 100, 100)
	_ = f.Deaths.RequestDie(f.ctx, p.ChannelID)
	f.Cast.reset()

	// 再死一次不应该重复广播
	if err := f.Deaths.RequestDie(f.ctx, p.ChannelID); err != nil {
		t.Fatalf("second die: %v", err)
	}
	if n := f.Cast.count(protocol.SM_NOTIFYDEAD); n != 0 {
		t.Fatalf("already dead should not re-broadcast death, got %d", n)
	}
}

func TestRequestReviveRequiresDead(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "alive", 100, 100)
	f.Cast.reset()

	// 活着不能复活（否则等于免费回城）
	if err := f.Deaths.RequestRevive(f.ctx, p.ChannelID); err != nil {
		t.Fatalf("revive: %v", err)
	}
	if n := f.Cast.count(protocol.SM_REVIVE); n != 0 {
		t.Fatalf("alive player should not revive, got %d SM_REVIVE", n)
	}
	if p.X != 100 || p.Y != 100 {
		t.Fatalf("alive player should not move, at (%d,%d)", p.X, p.Y)
	}
}

func TestRequestReviveRestoresTenthHP(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "victim", 100, 100)
	// Enter 会按等级重算 MaxMP（30+level*3=60），
	// 所以这里要用 world 侧的真实值来断言，而不是自己写死 50。
	p.MaxHP = 100
	p.MaxMP = 40
	_ = f.Deaths.RequestDie(f.ctx, p.ChannelID)
	f.Cast.reset()

	if err := f.Deaths.RequestRevive(f.ctx, p.ChannelID); err != nil {
		t.Fatalf("revive: %v", err)
	}

	if p.HP != 10 {
		t.Fatalf("HP = %d, want MaxHP/10 = 10", p.HP)
	}
	if want := p.MaxMP / ReviveHealthDivisor; p.MP != want {
		t.Fatalf("MP = %d, want MaxMP/10 = %d", p.MP, want)
	}
	if n := f.Cast.count(protocol.SM_REVIVE); n == 0 {
		t.Fatal("should broadcast SM_REVIVE")
	}
}

func TestReviveNeverGivesZeroHP(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "weak", 100, 100)
	// 低级角色 MaxHP=3 -> 3/10=0，必须钳到 1，否则复活即死
	p.MaxHP = 3
	p.MaxMP = 3
	_ = f.Deaths.RequestDie(f.ctx, p.ChannelID)

	if err := f.Deaths.RequestRevive(f.ctx, p.ChannelID); err != nil {
		t.Fatalf("revive: %v", err)
	}
	if p.HP < 1 {
		t.Fatalf("revive HP = %d, must be at least 1", p.HP)
	}
	if p.MP < 1 {
		t.Fatalf("revive MP = %d, must be at least 1", p.MP)
	}
}

func TestReviveRelocatesBeforeStanding(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "victim", 120, 120)
	_ = f.Deaths.RequestDie(f.ctx, p.ChannelID)

	if err := f.Deaths.RequestRevive(f.ctx, p.ChannelID); err != nil {
		t.Fatalf("revive: %v", err)
	}

	// 复活点（道馆_1 的 405,120）在当前测试地图之外，
	// 会被就近找一个可站立格；关键是「不在死亡点原地复活」
	if p.X == 120 && p.Y == 120 {
		t.Fatal("revive should relocate the body, not stand up where it died")
	}
	if p.HP <= 0 {
		t.Fatalf("revived player should have positive HP, got %d", p.HP)
	}
}

// ---------- 金币 / 经验 ----------

func TestGainExpLevelsUp(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "newbie", 100, 100)
	p.Level = 1
	p.Exp = 0

	before := p.Level
	f.Shops.GainExp(f.ctx, p, 100000)

	if p.Level <= before {
		t.Fatalf("should level up: %d -> %d", before, p.Level)
	}
	// 升级要回满血
	if p.HP != p.MaxHP {
		t.Fatalf("level up should refill HP: %d/%d", p.HP, p.MaxHP)
	}
	if n := f.Cast.of(p.ChannelID, protocol.SM_EXP); len(n) == 0 {
		t.Fatal("should send SM_EXP")
	}
}

func TestGoldAddAndCap(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "rich", 100, 100)
	p.Gold = 0

	total := f.Shops.AddGold(f.ctx, p, 1000)
	if total != 1000 {
		t.Fatalf("gold = %d, want 1000", total)
	}

	// 超过上限要钳住，不能回绕成小数
	p.Gold = MaxGold - 10
	total = f.Shops.AddGold(f.ctx, p, 1000)
	if total != MaxGold {
		t.Fatalf("gold should cap at MaxGold %d, got %d", MaxGold, total)
	}
	if total < MaxGold-10 {
		t.Fatalf("gold wrapped around: %d", total)
	}
}

func TestBuySuccess(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "buyer", 100, 100)
	p.Gold = 10000

	potionID, ok := f.Items.FindByName("金创药（小）")
	if !ok {
		t.Fatal("金创药（小）not found")
	}
	rec, _ := f.Items.Get(potionID)
	f.Cast.reset()

	npcUID := mustNPCUID(t, 24)
	if err := f.Shops.Buy(f.ctx, p.ChannelID, protocol.CMBuy{
		NPCUID: npcUID, ItemID: potionID, SeqID: 1, Count: 2,
	}); err != nil {
		t.Fatalf("buy: %v", err)
	}

	// 金币扣了 price*2
	wantGold := uint64(10000 - rec.Price*2)
	if p.Gold != wantGold {
		t.Fatalf("gold = %d, want %d (price=%d x2)", p.Gold, wantGold, rec.Price)
	}
	// 背包里有 2 个
	var got uint32
	for _, it := range p.Inventory {
		if it.ItemID == potionID {
			got += it.Count
		}
	}
	if got != 2 {
		t.Fatalf("inventory count = %d, want 2", got)
	}
	// 回了 SUCCEED 而不是 ERROR
	if n := f.Cast.of(p.ChannelID, protocol.SM_BUYSUCCEED); len(n) == 0 {
		t.Fatal("should send SM_BUYSUCCEED")
	}
	if n := f.Cast.of(p.ChannelID, protocol.SM_BUYERROR); len(n) != 0 {
		t.Fatalf("should not send SM_BUYERROR, got %d", len(n))
	}
}

func TestBuyInsufficientGold(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "poor", 100, 100)
	p.Gold = 0

	potionID, _ := f.Items.FindByName("金创药（小）")
	f.Cast.reset()

	npcUID := mustNPCUID(t, 24)
	if err := f.Shops.Buy(f.ctx, p.ChannelID, protocol.CMBuy{
		NPCUID: npcUID, ItemID: potionID, SeqID: 1, Count: 1,
	}); err != nil {
		t.Fatalf("buy: %v", err)
	}

	if p.Gold != 0 {
		t.Fatalf("gold should stay 0, got %d", p.Gold)
	}
	if len(p.Inventory) != 0 {
		t.Fatalf("should get no item, inventory = %v", p.Inventory)
	}
	if n := f.Cast.of(p.ChannelID, protocol.SM_BUYERROR); len(n) == 0 {
		t.Fatal("should send SM_BUYERROR")
	}
}

func TestBuyBadCount(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "buyer", 100, 100)
	p.Gold = 10000
	potionID, _ := f.Items.FindByName("金创药（小）")
	npcUID := mustNPCUID(t, 24)

	for _, count := range []uint64{0, MaxBuyCount + 1} {
		f.Cast.reset()
		if err := f.Shops.Buy(f.ctx, p.ChannelID, protocol.CMBuy{
			NPCUID: npcUID, ItemID: potionID, SeqID: 1, Count: count,
		}); err != nil {
			t.Fatalf("buy count %d: %v", count, err)
		}
		if n := f.Cast.of(p.ChannelID, protocol.SM_BUYERROR); len(n) == 0 {
			t.Fatalf("count %d should be rejected", count)
		}
	}
	if p.Gold != 10000 {
		t.Fatalf("gold should be untouched, got %d", p.Gold)
	}
}

func TestBuyUnknownItem(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "buyer", 100, 100)
	p.Gold = 10000
	f.Cast.reset()

	if err := f.Shops.Buy(f.ctx, p.ChannelID, protocol.CMBuy{
		NPCUID: mustNPCUID(t, 24), ItemID: 999999, SeqID: 1, Count: 1,
	}); err != nil {
		t.Fatalf("buy: %v", err)
	}
	if n := f.Cast.of(p.ChannelID, protocol.SM_BUYERROR); len(n) == 0 {
		t.Fatal("unknown item should be rejected")
	}
}

func TestQuerySellItemList(t *testing.T) {
	f := newB5SpellFixture(t)
	p := f.mkPlayer(t, 1, "browser", 100, 100)
	potionID, ok := f.Items.FindByName("金创药（小）")
	if !ok {
		t.Fatal("potion not found")
	}
	f.Cast.reset()

	if err := f.Shops.QuerySellItemList(f.ctx, p.ChannelID, protocol.CMQuerySellItemList{
		NPCUID: mustNPCUID(t, 24), ItemID: potionID,
	}); err != nil {
		t.Fatalf("query: %v", err)
	}
	msgs := f.Cast.of(p.ChannelID, protocol.SM_SELLITEMLIST)
	if len(msgs) != 1 {
		t.Fatalf("should get 1 SM_SELLITEMLIST, got %d", len(msgs))
	}
	// 解回来核对内容
	var list protocol.SDSellItemList
	if err := protocol.CerealDeserialize(msgs[0].Payload, &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.List) != 1 || list.List[0].Item.ItemID != potionID {
		t.Fatalf("sell list content wrong: %+v", list.List)
	}
	if len(list.List[0].CostList) != 1 || list.List[0].CostList[0].ItemID != GoldItemID {
		t.Fatalf("cost should be gold: %+v", list.List[0].CostList)
	}
}

// ---------- 世界时钟 ----------

func TestTickerAdvancesWorldClock(t *testing.T) {
	f := newB5SpellFixture(t)
	// 先建图但**不推进**：此时 NowMS 还是 MapState 的零值
	f.b5Fixture.makePlayer(t, 1, "alice", 100, 100)

	if now := f.Worlds.Worlds.NowMS(f.ctx, 1); now != 0 {
		t.Fatalf("initial NowMS = %d, want 0", now)
	}
	f.advanceClock(500)
	if now := f.Worlds.Worlds.NowMS(f.ctx, 1); now != f.clockMS {
		t.Fatalf("after advance NowMS = %d, want %d", now, f.clockMS)
	}
	// 再推进一次要跟着走
	f.advanceClock(300)
	if now := f.Worlds.Worlds.NowMS(f.ctx, 1); now != f.clockMS {
		t.Fatalf("NowMS = %d, want %d", now, f.clockMS)
	}
}

func TestTickerTracksMapOnLoad(t *testing.T) {
	f := newB5SpellFixture(t)
	// LoadMap 已由 makePlayer 触发；Track 必须被调用，
	// 否则 Advance 从来不会跑，NowMS 恒为 0
	f.mkPlayer(t, 1, "alice", 100, 100)
	f.advanceClock(1000)

	if now := f.Worlds.Worlds.NowMS(f.ctx, 1); now == 0 {
		t.Fatal("map should be tracked by ticker, NowMS still 0")
	}
}

// ---------- 击杀结算 ----------

func TestCommitDeathGivesExpAndDrops(t *testing.T) {
	f := newB5SpellFixture(t)
	f.mkPlayer(t, 1, "killer", 100, 100)

	rec := world.MonsterRecord{
		Name: "鹿", HP: 1, Exp: 30,
		DropItems: [][]world.DropItem{{{Item: "金创药（小）", ProbRecip: 1, Count: 1}}},
	}
	uid := mustMonsterUID(t, 2001)
	f.spawnMonster(t, 1, uid, 105, 105, 1, rec)

	// 先打死
	if err := f.Worlds.Worlds.MutateEntity(f.ctx, 1, uid, func(e *world.Entity) error {
		e.HP = 0
		return nil
	}); err != nil {
		t.Fatalf("kill: %v", err)
	}

	result, ok, err := f.Worlds.Worlds.CommitDeath(f.ctx, 1, uid)
	if err != nil || !ok {
		t.Fatalf("commit: ok=%v err=%v", ok, err)
	}
	if result.Exp != 30 {
		t.Fatalf("exp = %d, want 30", result.Exp)
	}
	if len(result.Drops) != 1 || result.Drops[0].Item != "金创药（小）" {
		t.Fatalf("drops = %+v", result.Drops)
	}
	if result.X != 105 || result.Y != 105 {
		t.Fatalf("drop position = (%d,%d), want (105,105)", result.X, result.Y)
	}

	// 第二次不能再结算（防重复掉落）
	if _, ok2, _ := f.Worlds.Worlds.CommitDeath(f.ctx, 1, uid); ok2 {
		t.Fatal("second CommitDeath should be a no-op")
	}
}

// ---------- 边界 ----------

func TestPlayerHasItemCountAcrossStacks(t *testing.T) {
	p := &Player{}
	p.Inventory = []Item{
		{ItemID: 6, SeqID: 1, Count: 30},
		{ItemID: 6, SeqID: 2, Count: 40},
	}
	if !p.HasItemCount(6, 70) {
		t.Fatal("should count across stacks (30+40=70)")
	}
	if p.HasItemCount(6, 71) {
		t.Fatal("71 should be insufficient")
	}

	// 跨格移除：先从尾部扣
	removed := p.RemoveItemByID(6, 50)
	if removed != 50 {
		t.Fatalf("removed = %d, want 50", removed)
	}
	var left uint32
	for _, it := range p.Inventory {
		if it.ItemID == 6 {
			left += it.Count
		}
	}
	if left != 20 {
		t.Fatalf("left = %d, want 20", left)
	}
}

func TestPlayerHasMagic(t *testing.T) {
	p := &Player{Magics: []uint32{14, 17}}
	if !p.HasMagic(14) || !p.HasMagic(17) {
		t.Fatal("should have both magics")
	}
	if p.HasMagic(99) {
		t.Fatal("should not have unlearned magic")
	}
}

func TestItemDBGoldID(t *testing.T) {
	db, err := LoadItemDB("../../content/items.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	id := db.GoldID()
	if id == 0 {
		t.Fatal("gold item not found")
	}
	rec, ok := db.Get(id)
	if !ok || rec.Type != TypeGold {
		t.Fatalf("GoldID %d is not type 金币: %+v", id, rec)
	}
	// 二次调用走缓存，结果必须一致
	if again := db.GoldID(); again != id {
		t.Fatalf("GoldID not cached: %d vs %d", id, again)
	}
}

// 让 unused import 不报错（context 在 helper 签名里用到）
var _ = context.Background

// mustMonsterUID 构造一个合法的怪物 UID。
func mustMonsterUID(t *testing.T, id uint32) uint64 {
	t.Helper()
	uid, err := common.BuildMonsterUID(id, 0, 0)
	if err != nil {
		t.Fatalf("BuildMonsterUID(%d): %v", id, err)
	}
	return uid
}

// mustNPCUID 构造一个合法的 NPC UID。
func mustNPCUID(t *testing.T, id uint32) uint64 {
	t.Helper()
	uid, err := common.BuildNPCUID(id, 0, 0)
	if err != nil {
		t.Fatalf("BuildNPCUID(%d): %v", id, err)
	}
	return uid
}

// rec0CoolDown 返回某技能的冷却毫秒数（测试用）。
func rec0CoolDown(f *b5spellFixture, magicID uint32) int {
	rec, ok := f.Magics.Get(magicID)
	if !ok {
		return 0
	}
	return rec.CoolDown
}

// TestGoldIDIsDeterministic 是 GoldID 的回归测试。
//
// 背景：金币在物品表里有 5 条面额（金币（小）.. 金币（超），id 1..5）。
// 初版用 `for id := range db.records { return id }`，Go 的 map 遍历
// 顺序随机，导致**每次进程启动返回的金币 itemID 都不一样**
// （实测同一份数据两次启动分别是 1 和 4）。
//
// 危害：购买时 costList 用某个面额计价，addGold 又按另一条面额找，
// 两边对不上，表现为「扣了钱金币数不变」。
//
// 这条测试用「重复加载多个 DB 实例」来暴露 map 顺序问题：
// 单实例多次调用会命中缓存，测不出随机性。
func TestGoldIDIsDeterministic(t *testing.T) {
	const rounds = 20
	var first uint32
	for i := 0; i < rounds; i++ {
		db, err := LoadItemDB("../../content/items.json")
		if err != nil {
			t.Fatalf("load #%d: %v", i, err)
		}
		id := db.GoldID()
		if id == 0 {
			t.Fatalf("round %d: gold item not found", i)
		}
		if i == 0 {
			first = id
			continue
		}
		if id != first {
			t.Fatalf("round %d: GoldID = %d, first was %d (map iteration order leaked)", i, id, first)
		}
	}

	// 必须是面额最小的那条（金币（小），id=1），不是随机面额
	db, err := LoadItemDB("../../content/items.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := db.GoldID(); got != 1 {
		t.Fatalf("GoldID = %d, want 1 (金币（小）, the smallest denomination)", got)
	}
	rec, _ := db.Get(1)
	if rec.Name != "金币（小）" {
		t.Fatalf("item 1 is %q, want 金币（小）", rec.Name)
	}

	// 全部 5 种面额都要能识别成金币类型
	goldCount := 0
	for _, r := range db.records {
		if r.Type == TypeGold {
			goldCount++
		}
	}
	if goldCount != 5 {
		t.Fatalf("gold denominations = %d, want 5", goldCount)
	}
}
