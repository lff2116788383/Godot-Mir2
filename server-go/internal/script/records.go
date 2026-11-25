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
