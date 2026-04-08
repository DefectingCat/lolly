// Package server 提供 pprof 性能分析的底层实现。
//
// 该文件封装 runtime/pprof 的调用，为 fasthttp 提供流式输出支持。
//
// 作者：xfy
package server

import (
	"bufio"
	"io"
	"runtime"
	"runtime/pprof"
	"sync"
)

var (
	cpuProfileMu     sync.Mutex
	cpuProfileActive bool
)

// startCPUProfile 启动 CPU profile 采集。
//
// 参数：
//   - w: 输出 writer
//
// 返回值：
//   - error: 启动失败时的错误
func startCPUProfile(w io.Writer) error {
	cpuProfileMu.Lock()
	defer cpuProfileMu.Unlock()

	if cpuProfileActive {
		return nil // 已在采集，忽略
	}

	if err := pprof.StartCPUProfile(w); err != nil {
		return err
	}

	cpuProfileActive = true
	return nil
}

// stopCPUProfile 厉止 CPU profile 采集。
func stopCPUProfile() {
	cpuProfileMu.Lock()
	defer cpuProfileMu.Unlock()

	if cpuProfileActive {
		pprof.StopCPUProfile()
		cpuProfileActive = false
	}
}

// writeHeapProfile 写入内存分配 profile。
func writeHeapProfile(w io.Writer) {
	runtime.GC() // 先执行 GC，获取更准确的数据
	_ = pprof.WriteHeapProfile(w)
}

// writeGoroutineProfile 写入 Goroutine stack traces。
func writeGoroutineProfile(w io.Writer) {
	p := pprof.Lookup("goroutine")
	if p != nil {
		_ = p.WriteTo(w, 0)
	}
}

// writeBlockProfile 写入阻塞 profile。
func writeBlockProfile(w io.Writer) {
	p := pprof.Lookup("block")
	if p != nil {
		_ = p.WriteTo(w, 0)
	}
}

// writeMutexProfile 写入锁竞争 profile。
func writeMutexProfile(w io.Writer) {
	p := pprof.Lookup("mutex")
	if p != nil {
		_ = p.WriteTo(w, 0)
	}
}

// bufioWriterAdapter 将 bufio.Writer 包装为 io.Writer，自动 Flush。
type bufioWriterAdapter struct {
	w *bufio.Writer
}

func (a *bufioWriterAdapter) Write(p []byte) (n int, err error) {
	n, err = a.w.Write(p)
	if err == nil {
		_ = a.w.Flush()
	}
	return n, err
}

// wrapBufioWriter 将 bufio.Writer 包装为 io.Writer。
func wrapBufioWriter(w *bufio.Writer) io.Writer {
	return &bufioWriterAdapter{w: w}
}
