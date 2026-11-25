package script

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
)

// ==================== 记录表提取 ====================
//
// NPC 脚本里到处是 getItemID('金创药（小）') / getMapID('道馆_1')
// 这类调用，所以运行时必须有「名字 <-> ID」双向表。
//
// 数据来源维持既有约定：用 tools/extract 的 Python 脚本生成 JSON。
// 这里只提供 Go 侧的读取与 inc 解析两种入口。

// LoadNameTableFromJSON 从 extract 生成的 JSON 读「ID -> 名字」。
//
// 兼容两种 JSON 结构：
//
//	{"records": {"1": {"name": "金币（小）"}}}   items.json / magics.json
//	{"1": {"name": "..."}}                      扁平结构
func LoadNameTableFromJSON(path string) (*NameTable, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var withRecords struct {
		Records map[string]struct {
			Name string `json:"name"`
		} `json:"records"`
	}
	if err := json.Unmarshal(raw, &withRecords); err == nil && withRecords.Records != nil {
		return nameTableFromJSONMap(withRecords.Records), nil
	}

	var flat map[string]struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &flat); err != nil {
		return nil, err
	}
	return nameTableFromJSONMap(flat), nil
}

func nameTableFromJSONMap(records map[string]struct {
	Name string `json:"name"`
}) *NameTable {
	out := make(map[uint32]string, len(records))
	for key, rec := range records {
		id, err := strconv.ParseUint(key, 10, 32)
		if err != nil {
			continue
		}
		out[uint32(id)] = rec.Name
	}
	return NewNameTable(out)
}

var mapRecordNameRe = regexp.MustCompile(`\.name\s*=\s*u8"([^"]*)"`)

// mapSwitchRe 匹配 switchList 里的一条：
//
//	{.x = 333, .y = 776, .w = 1, .h = 1, .endName = u8"边境城市_01", .endX = 564, .endY = 69}
var mapSwitchRe = regexp.MustCompile(
	`\{\s*\.x\s*=\s*(-?\d+)\s*,\s*\.y\s*=\s*(-?\d+)\s*,\s*\.w\s*=\s*(\d+)\s*,\s*\.h\s*=\s*(\d+)\s*,\s*\.endName\s*=\s*u8"([^"]*)"(?:\s*,\s*\.endX\s*=\s*(-?\d+)\s*,\s*\.endY\s*=\s*(-?\d+))?`)

// MapSwitch 是一条换图配置：踩到 (X,Y) 这片 (W,H) 区域就传到 EndMap 的 (EndX,EndY)。
//
// ★ 这是地图 record 的 switchList 项，与控制台「走门口进下一张图」是同一份数据。
//	缺了它，玩家站在比奇县门口永远传不过去。
type MapSwitch struct {
	X, Y, W, H int
	EndMap     string
	EndMapID   uint32
	EndX, EndY int
}

// ParseMapRecordSwitches 从 maprecord.inc 提取「mapID -> 该图的换图列表」。
//
// ★ 与 ParseMapRecordInc 共用同一套顶层分块：
//	switchList 是记录内的嵌套表，必须只在本块内匹配，
//	否则会把邻图的 endName 也算进来。
//
// ★ endMapID 需要名字→ID 反查，由调用方用已有的名字表解析，
//	这里先把 EndMap 名字填好、EndMapID 留 0。
func ParseMapRecordSwitches(text string, nameToID map[string]uint32) map[uint32][]MapSwitch {
	out := make(map[uint32][]MapSwitch)
	blocks := topLevelBlocks(text)
	for idx, block := range blocks {
		m := mapRecordNameRe.FindStringSubmatch(block)
		if m == nil {
			continue
		}
		matches := mapSwitchRe.FindAllStringSubmatch(block, -1)
		if len(matches) == 0 {
			continue
		}
		list := make([]MapSwitch, 0, len(matches))
		for _, sm := range matches {
			sw := MapSwitch{
				X:      atoiDef(sm[1], 0),
				Y:      atoiDef(sm[2], 0),
				W:      atoiDef(sm[3], 1),
				H:      atoiDef(sm[4], 1),
				EndMap: sm[5],
			}
			if sm[6] != "" {
				sw.EndX = atoiDef(sm[6], 0)
			}
			if sm[7] != "" {
				sw.EndY = atoiDef(sm[7], 0)
			}
			if id, ok := nameToID[sw.EndMap]; ok {
				sw.EndMapID = id
			}
			list = append(list, sw)
		}
		out[uint32(idx)] = list
	}
	return out
}

// atoiDef 解析十进制整数，失败返回 def。
func atoiDef(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// ParseMapRecordInc 从 maprecord.inc 提取「ID -> 地图名」。
//
// ★ 必须按顶层块切分，不能用「找所有 .name」：
//
//	maprecord.inc 每条记录的 switchList 里全是
//	`{.x=206, .y=52, .endName = u8"道馆_1"}` 这种嵌套项。
//	按任意 .name 匹配会把 endName 也当成记录名，
//	ID 索引整体错位（这个坑在 extract_magics.py 里已经踩过一次）。
//
//	这里简化处理：按顶层 `{` 配对扫描，每块取第一个 .name。
func ParseMapRecordInc(text string) map[uint32]string {
	out := make(map[uint32]string)
	blocks := topLevelBlocks(text)
	for idx, block := range blocks {
		m := mapRecordNameRe.FindStringSubmatch(block)
		if m == nil {
			continue // 第 0 块是空占位 {}
		}
		out[uint32(idx)] = m[1]
	}
	return out
}

// topLevelBlocks 按花括号配对切出顶层记录块（跳过字符串内的括号）。
func topLevelBlocks(text string) []string {
	var blocks []string
	i, n := 0, len(text)
	for i < n {
		switch text[i] {
		case '"':
			i = skipLuaString(text, i)
		case '{':
			start := i
			depth := 0
			j := i
			for j < n {
				switch text[j] {
				case '"':
					j = skipLuaString(text, j)
					continue
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						blocks = append(blocks, text[start:j+1])
						goto next
					}
				}
				j++
			}
			blocks = append(blocks, text[start:])
			return blocks
		next:
			i = j + 1
		default:
			i++
		}
	}
	return blocks
}

// skipLuaString 从引号位置跳到字符串结束后一位。
func skipLuaString(text string, i int) int {
	n := len(text)
	i++
	for i < n {
		if text[i] == '\\' {
			i += 2
			continue
		}
		if text[i] == '"' {
			return i + 1
		}
		i++
	}
	return i
}

var monsterRecordNameRe = regexp.MustCompile(`\.name\s*=\s*u8"([^"]*)"`)

// ParseMonsterRecordInc 从 monsterrecord.inc 提取「ID -> 怪物名」。
//
// 结构与 maprecord.inc 一样（第 0 块是空占位），用同一套顶层块解析。
// 单独一个函数只是为了以后怪物记录要提更多字段时好改。
func ParseMonsterRecordInc(text string) map[uint32]string {
	out := make(map[uint32]string)
	for idx, block := range topLevelBlocks(text) {
		m := monsterRecordNameRe.FindStringSubmatch(block)
		if m == nil {
			continue
		}
		out[uint32(idx)] = m[1]
	}
	return out
}
