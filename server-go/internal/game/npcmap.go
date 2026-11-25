package game

import (
	"os"

	"github.com/lff2116788383/godot-mir2/server-go/internal/script"
)

// LoadMapNames 读 maprecord.inc 得到「mapID -> 地图名」。
//
// 路径优先级：显式配置 > 环境变量 MIR2X_MAPRECORD > 几个约定位置。
// 读不到就返回空表（NPC 不会加载，但服务端其他功能不受影响）。
func LoadMapNames(configured string) map[uint32]string {
	candidates := []string{}
	if configured != "" {
		candidates = append(candidates, configured)
	}
	if env := os.Getenv("MIR2X_MAPRECORD"); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates,
		"E:/Desktop/game/mir2x/common/src/maprecord.inc",
		"../../mir2x/common/src/maprecord.inc",
		"../mir2x/common/src/maprecord.inc",
	)

	for _, path := range candidates {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return script.ParseMapRecordInc(string(raw))
	}
	return map[uint32]string{}
}
