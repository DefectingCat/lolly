package limitrate

import (
	"io"
	"sync"
	"time"
)

// RateLimitedWriter 限速写入器，使用令牌桶算法
type RateLimitedWriter struct {
	writer    io.Writer
	rate      int64     // 字节/秒
	bucket    int64     // 当前令牌数
	maxBucket int64     // 令牌桶最大容量
	lastTime  time.Time // 上次更新时间
	mu        sync.Mutex
}


