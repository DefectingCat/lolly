// Package limitrate 提供基于令牌桶算法的请求速率限制功能。
//
// 包含速率限制响应写入器相关的逻辑，用于处理被限制的请求响应。
//
// 作者：xfy
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


