package game

import (
	"math/rand"
	"sync"
	"time"
)

// randUint32 返回一个随机数（并发安全）。
//
// ★ 不用 math/rand 的全局函数：
//
//	Go 1.20+ 的全局 Rand 自带锁，但在高并发下会成为热点
//	（刷怪每次 randGLoc 都要 N 次随机）。这里用 per-P 的池化源。
var randPool = sync.Pool{
	New: func() any {
		return rand.New(rand.NewSource(time.Now().UnixNano()))
	},
}

func randUint32() uint32 {
	r := randPool.Get().(*rand.Rand)
	v := r.Uint32()
	randPool.Put(r)
	return v
}
