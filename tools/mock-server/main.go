package main

// mir2x mock 服务端
//
// 定位：只让 client-godot 客户端跑通联调，不追求服务端正确性。
// 硬规则：响应必须严格按 docs/protocol.md 的格式生成，
//        否则会把客户端 bug 掩盖成 mock bug。
//
// 启动：
//     go run .            # 监听 :7000
//     go run . -port 7001

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"
)

// 服务端消息码（从 tables.go 的 smTable 反查，这里只列用到的）
const (
	smPing            = 3
	smUID             = 4
	smLoginOK         = 5
	smLoginError      = 6
	smQueryCharOK     = 11
	smQueryCharError  = 12
	smOnlineOK        = 20
	smOnlineError     = 22
	smStartGameScene  = 23
	smPlayerConfig    = 24
	smAction          = 28
	smCORecord        = 29
	smHealth          = 30
	smExp             = 34
	smGold            = 47
	smText            = 51
	smPlayerSay       = 83
	smBuildVersion    = 64
	smOffline         = 39
	smCODisappear     = 63
)

// 客户端消息码
const (
	cmPing        = 1
	cmLogin       = 2
	cmQueryChar   = 3
	cmOnline      = 7
	cmAction      = 8
	cmPlayerSay   = 44
	cmQueryGold   = 25
)

// 错误码（protocoldef.hpp）
const (
	loginErrNone      = 0
	loginErrNoAccount = 1
	loginErrMulti     = 2

	onlineErrNone    = 0
	onlineErrNoLogin = 1
	onlineErrMulti   = 2
	onlineErrNoChar  = 3

	queryCharErrNoLogin = 1
	queryCharErrNoChar  = 2
)

// 动作类型（actionnode.hpp）
const (
	actSpawn   = 1
	actStand   = 2
	actMove    = 3
	actAttack  = 7
	actHitted  = 11
	actDie     = 13
)

// 出生点（比奇县_0，与客户端一致）
const (
	spawnX   = 400
	spawnY   = 402
	playerDBID = 1001

	// mapUID 必须是 uidf::buildMapUID 编码后的值，不能是裸 mapID
	//   type(MAP=2) << 59 | id(1) << 35 | peerIndex(1) << 31
	//   = 0x1000000880000000
	mapUID = uint64(2)<<uidTypeShift | uint64(1)<<uidIDShift | uint64(1)<<uidPeerShift
)

// Server 服务端状态
type Server struct {
	mu      sync.Mutex
	players map[string]*Player  // 账号 -> 玩家
	clients map[net.Conn]*Session
	// 场景内固定怪物（用于验证 SM_CORECORD / SM_ACTION）
	sceneMonsters []MonsterSpawn
}

// Session 一个连接
type Session struct {
	conn    net.Conn
	account string
	online  bool
}

// Player 玩家状态
type Player struct {
	Account string
	DBID    uint32
	Name    string
	Job     uint8
	Gender  uint8
	X       int
	Y       int
	HP      int
	MaxHP   int
	MP      int
	MaxMP   int
	Gold    uint64
	Exp     uint64
}

func newServer() *Server {
	return &Server{
		players: make(map[string]*Player),
		clients: make(map[net.Conn]*Session),
		// 默认场景怪物：摆在小范围，方便客户端视野内能看到
		// MonsterID 取自 data/monsterrecord.json：
		//   1=沙漠蜥蜴 2=夜行鬼 3=夜行鬼09
		// 摆在主角 (400,402) 周围 4-6 格，确保初始视口内全部可见
		sceneMonsters: []MonsterSpawn{
			{MonsterID: 1, X: 396, Y: 399, Dir: 5, Seq: 1},   // 左上
			{MonsterID: 2, X: 404, Y: 400, Dir: 5, Seq: 2},   // 右上
			{MonsterID: 3, X: 400, Y: 406, Dir: 4, Seq: 3},   // 下方
		},
	}
}

func main() {
	port := flag.Int("port", 7000, "监听端口")
	flag.Parse()

	s := newServer()
	addr := fmt.Sprintf(":%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("监听失败: %v", err)
	}
	log.Printf("mock 服务端已启动: %s", addr)
	log.Printf("协议严格对齐 mir2x，客户端无需任何适配")

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept 失败: %v", err)
			continue
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	sess := &Session{conn: conn}
	s.mu.Lock()
	s.clients[conn] = sess
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
	}()

	log.Printf("[连接] %s", conn.RemoteAddr())
	br := bufio.NewReader(conn)

	for {
		f, err := ReadFrame(br)
		if err != nil {
			log.Printf("[断开] %s: %v", conn.RemoteAddr(), err)
			return
		}
		s.dispatch(sess, f)
	}
}

func (s *Server) dispatch(sess *Session, f *Frame) {
	switch f.HeadCode {
	case cmPing:
		s.onPing(sess, f)
	case cmLogin:
		s.onLogin(sess, f)
	case cmQueryChar:
		s.onQueryChar(sess, f)
	case cmOnline:
		s.onOnline(sess, f)
	case cmAction:
		s.onAction(sess, f)
	case cmPlayerSay:
		s.onPlayerSay(sess, f)
	case cmQueryGold:
		s.onQueryGold(sess, f)
	default:
		log.Printf("[未处理] CM %d (len=%d)", f.HeadCode, len(f.Body))
	}
}

// ---- CM_PING ----
// CMPing { uint32 Tick }，应答回同一个 tick
func (s *Server) onPing(sess *Session, f *Frame) {
	if len(f.Body) != 4 {
		return
	}
	tick := getU32(f.Body, 0)
	body := make([]byte, 4)
	putU32(body, 0, tick)
	WriteFrame(sess.conn, smPing, f.RespID, body)
}

// ---- CM_LOGIN ----
// CMLogin { StaticBuffer<64> id; StaticBuffer<64> password; } = 136 字节
// 每个 StaticBuffer = u16 size + 65 字节 data
func (s *Server) onLogin(sess *Session, f *Frame) {
	if len(f.Body) != 136 {
		return
	}
	idSize := getU16(f.Body, 0)
	id := string(f.Body[2 : 2+idSize])
	pwOff := 2 + 65
	pwSize := getU16(f.Body, pwOff)
	pw := string(f.Body[pwOff+2 : pwOff+2+int(pwSize)])

	log.Printf("[登录] 账号=%q 密码=%q", id, pw)

	// 任意非空账号密码都通过（mock 不校验）
	if id == "" || pw == "" {
		WriteFrame(sess.conn, smLoginError, f.RespID, []byte{loginErrNoAccount})
		return
	}

	s.mu.Lock()
	sess.account = id
	if _, ok := s.players[id]; !ok {
		s.players[id] = &Player{
			Account: id,
			DBID:    playerDBID,
			Name:    "测试角色",
			Job:     1, // 战士
			Gender:  1,
			X:       spawnX,
			Y:       spawnY,
			HP:      100, MaxHP: 100,
			MP:      50, MaxMP: 50,
			Gold:    1000,
			Exp:     0,
		}
	}
	s.mu.Unlock()

	WriteFrame(sess.conn, smLoginOK, f.RespID, nil)
}

// ---- CM_QUERYCHAR ----
// 空体。应答 SM_QUERYCHAROK = 78 字节
// SMQueryCharOK { StaticBuffer<64> name; u8 gender; u8 job; u64 exp; }
func (s *Server) onQueryChar(sess *Session, f *Frame) {
	s.mu.Lock()
	p := s.players[sess.account]
	s.mu.Unlock()

	if p == nil {
		WriteFrame(sess.conn, smQueryCharError, f.RespID, []byte{queryCharErrNoLogin})
		return
	}

	body := make([]byte, 78)
	name := []byte(p.Name)
	putU16(body, 0, uint16(len(name)))
	copy(body[2:], name)
	body[2+65] = p.Gender
	body[2+65+1] = p.Job
	putU64(body, 2+65+2, p.Exp)

	WriteFrame(sess.conn, smQueryCharOK, f.RespID, body)
}

// ---- CM_ONLINE ----
// 空体。应答 SM_ONLINEOK = 177 字节
// SMOnlineOK { u64 uid; StaticBuffer<128> name; u8 gender:1; u8 job:3; u64 mapUID; ActionNode action; }
func (s *Server) onOnline(sess *Session, f *Frame) {
	s.mu.Lock()
	p := s.players[sess.account]
	s.mu.Unlock()

	if p == nil {
		WriteFrame(sess.conn, smOnlineError, f.RespID, []byte{onlineErrNoLogin})
		return
	}

	uid := buildPlayerUID(p.DBID)
	body := make([]byte, 177)
	putU64(body, 0, uid)

	name := []byte(p.Name)
	putU16(body, 8, uint16(len(name)))
	copy(body[10:], name)

	// 偏移 8 + 132 = 140：gender:1 + job:3 位域
	body[140] = (p.Gender & 0x1) | ((p.Job & 0x7) << 1)

	// 141: u64 mapUID
	putU64(body, 141, mapUID)

	// 149: ActionNode (28 字节)
	act := makeActionNode(actSpawn, 5, p.X, p.Y, p.X, p.Y, 0, 0, 0)
	copy(body[149:], act)

	sess.online = true
	WriteFrame(sess.conn, smOnlineOK, f.RespID, body)

	// 紧接着下发进场场景
	s.sendStartGameScene(sess, p, uid)

	// 再下发场景内实体（怪物）
	s.sendSceneEntities(sess, p)

	log.Printf("[进图] %s uid=%d 位置(%d,%d)", p.Name, uid, p.X, p.Y)
}

// sendStartGameScene 下发 SM_STARTGAMESCENE（type 3，cerealf 序列化）
// 这是 type 3，body 需要 cereal PortableBinary 编码
func (s *Server) sendStartGameScene(sess *Session, p *Player, uid uint64) {
	var w cerealWriter

	// endian 标记
	w.byte(1)

	// SDStartGameScene:
	//   uid u64, mapUID u64, x i32, y i32, direction i32,
	//   desp(SDWLDesp), name string, nameColor u32, viewRange i32
	w.u64(uid)
	w.u64(mapUID)
	w.i32(int32(p.X))
	w.i32(int32(p.Y))
	w.i32(5) // DIR_DOWN

	// SDWLDesp { SDWear wear; u32 hair; u32 hairColor; }
	// SDWear 是 unordered_map<int, SDItem>，空表 = u64 0
	w.u64(0)      // wear 空
	w.u32(1)      // hair
	w.u32(0)      // hairColor

	// name string
	w.str(p.Name)
	// nameColor
	w.u32(0xFFFFFFFF)
	// viewRange
	w.i32(15)

	WriteFrame(sess.conn, smStartGameScene, 0, w.buf)
}

// ---- CM_ACTION ----
// CMAction { u64 UID; u64 mapUID; ActionNode action; } = 44 字节
// 服务端把动作广播回客户端（回显）
func (s *Server) onAction(sess *Session, f *Frame) {
	if len(f.Body) != 44 {
		return
	}
	uid := getU64(f.Body, 0)
	muid := getU64(f.Body, 8)
	act := f.Body[16:44]

	// 更新玩家位置
	actType := int(act[0] & 0x1F)
	if actType == actMove || actType == actStand {
		s.mu.Lock()
		if p := s.players[sess.account]; p != nil {
			p.X = int(int16(binaryLE16(act[8:10])))
			p.Y = int(int16(binaryLE16(act[10:12])))
		}
		s.mu.Unlock()
	}

	// 回显给本人（真实服务端会广播给视野内所有人）
	body := make([]byte, 44)
	putU64(body, 0, uid)
	putU64(body, 8, muid)
	copy(body[16:], act)
	WriteFrame(sess.conn, smAction, 0, body)
}

// ---- CM_PLAYERSAY ----
// CMPlayerSay { char content[128] }，应答 SM_PLAYERSAY { u64 uid; char content[128]; } = 136 字节
func (s *Server) onPlayerSay(sess *Session, f *Frame) {
	if len(f.Body) < 128 {
		return
	}
	content := trimNul(f.Body[:128])

	s.mu.Lock()
	p := s.players[sess.account]
	s.mu.Unlock()
	if p == nil {
		return
	}

	uid := buildPlayerUID(p.DBID)
	body := make([]byte, 136)
	putU64(body, 0, uid)
	copy(body[8:], content)
	WriteFrame(sess.conn, smPlayerSay, 0, body)

	log.Printf("[聊天] %s: %s", p.Name, string(content))
}

// ---- CM_QUERYGOLD ----
func (s *Server) onQueryGold(sess *Session, f *Frame) {
	s.mu.Lock()
	p := s.players[sess.account]
	s.mu.Unlock()
	if p == nil {
		return
	}
	body := make([]byte, 8)
	putU64(body, 0, p.Gold)
	WriteFrame(sess.conn, smGold, f.RespID, body)
}

// ---- 工具 ----

// buildPlayerUID 构造玩家 UID（uidf.cpp 位布局）
// type=5(PLY) 在 bit59-62，dbid 在 bit35-58
func buildPlayerUID(dbid uint32) uint64 {
	return (uint64(5) << 59) | (uint64(dbid&0xFFFFFF) << 35)
}

// makeActionNode 构造 28 字节 ActionNode
func makeActionNode(actType, dir, x, y, aimX, aimY, speed int, aimUID uint64, standMode int) []byte {
	b := make([]byte, 28)
	// type:5 | speed:9 在第一个 u16
	w0 := uint16(actType&0x1F) | (uint16(speed&0x1FF) << 5)
	w0 |= uint16(dir&0x01) << 15
	putU16(b, 0, w0)
	b[2] = byte((dir >> 1) & 0x0F)

	putU16(b, 4, uint16(int16(x)))
	putU16(b, 6, uint16(int16(y)))
	putU16(b, 8, uint16(int16(aimX)))
	putU16(b, 10, uint16(int16(aimY)))
	putU64(b, 12, aimUID)
	b[20] = byte(standMode)
	return b
}

func binaryLE16(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }

func trimNul(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

// cerealWriter cereal PortableBinary 编码器
type cerealWriter struct {
	buf []byte
}

func (w *cerealWriter) byte(v byte) { w.buf = append(w.buf, v) }

func (w *cerealWriter) u32(v uint32) {
	b := make([]byte, 4)
	putU32(b, 0, v)
	w.buf = append(w.buf, b...)
}

func (w *cerealWriter) i32(v int32) { w.u32(uint32(v)) }

func (w *cerealWriter) u64(v uint64) {
	b := make([]byte, 8)
	putU64(b, 0, v)
	w.buf = append(w.buf, b...)
}

func (w *cerealWriter) str(s string) {
	w.u64(uint64(len(s)))
	w.buf = append(w.buf, []byte(s)...)
}
