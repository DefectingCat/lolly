package loadbalance

import (
	"sync/atomic"
	"time"
)

// EWMAStats 使用原子操作实现的 EWMA（指数加权移动平均）统计器。
type EWMAStats struct {
	headerTime   atomic.Int64 // 首字节时间的 EWMA（纳秒）
	lastByteTime atomic.Int64 // 完整响应时间的 EWMA（纳秒）
	sampleCount  atomic.Int64 // 样本计数
}

const defaultAlphaScale = 300 // alpha = 0.3

func NewEWMAStats() *EWMAStats {
	return &EWMAStats{}
}

func (e *EWMAStats) Record(headerTime, lastByteTime time.Duration) {
	e.recordAtomic(&e.headerTime, headerTime)
	e.recordAtomic(&e.lastByteTime, lastByteTime)
	e.sampleCount.Add(1)
}

func (e *EWMAStats) recordAtomic(ptr *atomic.Int64, newValue time.Duration) {
	newNano := newValue.Nanoseconds()
	for {
		old := ptr.Load()
		if old == 0 {
			if ptr.CompareAndSwap(0, newNano) {
				return
			}
			continue
		}
		updated := (defaultAlphaScale*newNano + (1000-defaultAlphaScale)*old) / 1000
		if ptr.CompareAndSwap(old, updated) {
			return
		}
	}
}

func (e *EWMAStats) HeaderTime() time.Duration {
	return time.Duration(e.headerTime.Load())
}

func (e *EWMAStats) LastByteTime() time.Duration {
	return time.Duration(e.lastByteTime.Load())
}

func (e *EWMAStats) SampleCount() int64 {
	return e.sampleCount.Load()
}

func (e *EWMAStats) Reset() {
	e.headerTime.Store(0)
	e.lastByteTime.Store(0)
	e.sampleCount.Store(0)
}
