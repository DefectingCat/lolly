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

// NewRateLimitedWriter 创建限速写入器
func NewRateLimitedWriter(w io.Writer, rate, burst int64) *RateLimitedWriter {
	return &RateLimitedWriter{
		writer:    w,
		rate:      rate,
		bucket:    burst,
		maxBucket: burst,
		lastTime:  time.Now(),
	}
}

// Write 实现 io.Writer 接口，使用令牌桶算法限速
func (w *RateLimitedWriter) Write(p []byte) (int, error) {
	if w.rate <= 0 {
		return w.writer.Write(p)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// 计算新增令牌
	now := time.Now()
	elapsed := now.Sub(w.lastTime).Seconds()
	w.lastTime = now

	// 补充令牌
	newTokens := int64(elapsed * float64(w.rate))
	w.bucket += newTokens
	if w.bucket > w.maxBucket {
		w.bucket = w.maxBucket
	}

	// 消耗令牌
	n := len(p)
	if int64(n) <= w.bucket {
		w.bucket -= int64(n)
		return w.writer.Write(p)
	}

	// 令牌不足，分批写入
	written := 0
	for written < n {
		if w.bucket <= 0 {
			// 等待新令牌
			waitTime := time.Duration(float64(1) / float64(w.rate) * float64(time.Second))
			time.Sleep(waitTime)
			w.bucket = w.rate // 简化：每秒补充 rate 个令牌
		}

		chunk := min(int64(n-written), w.bucket)

		nw, err := w.writer.Write(p[written : written+int(chunk)])
		written += nw
		w.bucket -= int64(nw)

		if err != nil {
			return written, err
		}
	}

	return written, nil
}
