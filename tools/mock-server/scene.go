package main

import (
	"log"
	"time"
)

// 场景实体下发（mock）
//
// 目的：让客户端能验证 SM_CORECORD / SM_ACTION 的解析与渲染。
// 真实服务端由 Lua 脚本（server/script/map/*.lua 的 addMonster）生成，
// 这里按固定坐标写死，只为联调。
//
// SMCORecord 结构（common/src/servermsg.hpp:204，49 字节）：
//   [0]  UID       u64
//   [8]  mapUID    u64
//   [16] action    ActionNode (28B)
//   [44] union     12B
//          UID_MON: MonsterID u32 + 8B 填充
//          UID_PLY: gender:1 job:3 teamLeader:1 stall:1 (1B) + 3B 填充 + Level u32
//          UID_NPC: NPCID u32 + 8B 填充

const (
	uidTypeShift   = 59
	uidTypeMon     = 4
	uidIDShift     = 35
	uidIDMask      = 0xFFFFFF
	uidPeerShift   = 31
)

// buildMonsterUID 对应 uidf::buildMonsterUID
//   type 59-62 | id 35-58 | peerIndex 31-34 | seq 0-30
func buildMonsterUID(monsterID, peerIndex, seq uint64) uint64 {
	return (uidTypeMon << uidTypeShift) |
		((monsterID & uidIDMask) << uidIDShift) |
		((peerIndex & 0xF) << uidPeerShift) |
		(seq & 0x7FFFFFFF)
}

// MonsterSpawn 一只待下发的怪
type MonsterSpawn struct {
	MonsterID uint32
	X, Y      int
	Dir       int
	Seq       uint64
}

// buildCORecord 组装 SMCORecord（49 字节）
func buildCORecord(uid, mapUID uint64, act []byte, monsterID uint32) []byte {
	b := make([]byte, 49)
	putU64(b, 0, uid)
	putU64(b, 8, mapUID)
	copy(b[16:16+28], act)
	putU32(b, 44, monsterID)
	// 45-48 保留填充（原版 _SMCORecord_Monster 只有 u32，其余对齐填充）
	return b
}

// buildSMAction 组装 SMAction（44 字节）
func buildSMAction(uid, mapUID uint64, act []byte) []byte {
	b := make([]byte, 44)
	putU64(b, 0, uid)
	putU64(b, 8, mapUID)
	copy(b[16:16+28], act)
	return b
}

// sendSceneEntities 在进图后下发场景内实体
func (s *Server) sendSceneEntities(sess *Session, p *Player) {
	for _, m := range s.sceneMonsters {
		uid := buildMonsterUID(uint64(m.MonsterID), 1, m.Seq)
		// 站位动作（ACTION_STAND=2）
		act := makeActionNode(actStand, m.Dir, m.X, m.Y, m.X, m.Y, 100, 0, 0)
		body := buildCORecord(uid, mapUID, act, m.MonsterID)
		WriteFrame(sess.conn, smCORecord, 0, body)
	}
	log.Printf("[场景] 已下发 %d 只怪", len(s.sceneMonsters))
}

// broadcastMonsterMove 让一只怪走一格，用于验证动作同步
func (s *Server) broadcastMonsterMove(sess *Session, m MonsterSpawn, nx, ny int) {
	uid := buildMonsterUID(uint64(m.MonsterID), 1, m.Seq)
	// ACTION_MOVE=3，x/y 起点，aimX/aimY 终点
	act := makeActionNode(actMove, m.Dir, m.X, m.Y, nx, ny, 100, 0, 0)
	body := buildSMAction(uid, mapUID, act)
	WriteFrame(sess.conn, smAction, 0, body)
}

// demoMonsterWalk 演示怪物移动：每 1.5 秒让第一只怪走一步
// 用于验证 SM_ACTION -> 客户端动作队列 -> 逐格移动
func (s *Server) demoMonsterWalk(sess *Session) {
	time.Sleep(1500 * time.Millisecond)
	m := s.sceneMonsters[0]
	cx, cy := m.X, m.Y
	// 左右往返 4 次
	for i := 0; i < 4; i++ {
		nx := cx + 2
		if i%2 == 1 {
			nx = cx
		}
		s.broadcastMonsterMove(sess, MonsterSpawn{
			MonsterID: m.MonsterID, X: cx, Y: cy, Dir: 5, Seq: m.Seq,
		}, nx, cy)
		log.Printf("[演示] 怪物从 (%d,%d) 走到 (%d,%d)", cx, cy, nx, cy)
		time.Sleep(1500 * time.Millisecond)
	}
}
