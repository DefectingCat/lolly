// Package server 提供 pprof 性能分析的底层实现。
//
// 该文件封装 runtime/pprof 的调用，为 fasthttp 提供流式输出支持。
// 包含以下核心功能：
//   - CPU profile 的启动和停止
//   - 内存分配 profile 的写入
//   - Goroutine stack traces 的写入
//   - 阻塞和锁竞争 profile 的写入
//
// 主要用途：
//
//	作为 pprof.go 的底层实现层，处理与 runtime/pprof 的直接交互。
//
// 注意事项：
//   - CPU profile 使用互斥锁保护，确保并发安全
//   - 所有写入操作均使用 io.Writer 接口，支持流式输出
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
	// cpuProfileMu 保护 cpuProfileActive 状态的互斥锁。
	//
	// 用于确保 CPU profiling 启动和停止操作的线程安全性，
	// 防止并发调用 startCPUProfile/stopCPUProfile 导致的状态不一致。
	cpuProfileMu sync.Mutex

	// cpuProfileActive 标记当前 CPU profile 是否处于采集状态。
	//
	// 值为 true 表示正在采集，false 表示未采集。
	// 该变量由 cpuProfileMu 保护，不应直接访问。
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

// stopCPUProfile 停止 CPU profile 采集。
//
// 终止正在进行的 CPU profile 采集，并将缓冲数据刷新到输出。
// 若当前无活跃采集，则忽略该调用。
//
// 注意事项：
//   - 该函数由 cpuProfileMu 保护，调用前无需额外加锁
func stopCPUProfile() {
	cpuProfileMu.Lock()
	defer cpuProfileMu.Unlock()

	if cpuProfileActive {
		pprof.StopCPUProfile()
		cpuProfileActive = false
	}
}

// writeHeapProfile 写入内存分配 profile。
//
// 执行 GC 后采集内存分配数据并写入输出流。
// GC 可确保获取更准确的内存使用数据。
//
// 参数：
//   - w: 输出 writer，用于写入 profile 数据
func writeHeapProfile(w io.Writer) {
	runtime.GC() // 先执行 GC，获取更准确的数据
	//nolint:errcheck
	_ = pprof.WriteHeapProfile(w)
}

// writeGoroutineProfile 写入 Goroutine stack traces。
//
// 采集当前所有 Goroutine 的栈追踪信息并写入输出流。
//
// 参数：
//   - w: 输出 writer，用于写入 profile 数据
func writeGoroutineProfile(w io.Writer) {
	p := pprof.Lookup("goroutine")
	if p != nil {
		//nolint:errcheck
		_ = p.WriteTo(w, 0)
	}
}

// writeBlockProfile 写入阻塞 profile。
//
// 采集阻塞操作的 profile 数据并写入输出流。
//
// 参数：
//   - w: 输出 writer，用于写入 profile 数据
func writeBlockProfile(w io.Writer) {
	p := pprof.Lookup("block")
	if p != nil {
		//nolint:errcheck
		_ = p.WriteTo(w, 0)
	}
}

// writeMutexProfile 写入锁竞争 profile。
//
// 采集互斥锁竞争的 profile 数据并写入输出流。
//
// 参数：
//   - w: 输出 writer，用于写入 profile 数据
func writeMutexProfile(w io.Writer) {
	p := pprof.Lookup("mutex")
	if p != nil {
		//nolint:errcheck
		_ = p.WriteTo(w, 0)
	}
}

// bufioWriterAdapter 将 bufio.Writer 包装为 io.Writer，自动 Flush。
//
// 该结构体实现 io.Writer 接口，在每次写入后自动调用 Flush，
// 确保数据立即发送到客户端，适用于流式响应场景。
type bufioWriterAdapter struct {
	// w 被包装的 bufio.Writer
	w *bufio.Writer
}

// Write 实现 io.Writer 接口，写入数据并自动 Flush。
//
// 参数：
//   - p: 待写入的字节切片
//
// 返回值：
//   - n: 实际写入的字节数
//   - err: 写入过程中遇到的错误
func (a *bufioWriterAdapter) Write(p []byte) (n int, err error) {
	n, err = a.w.Write(p)
	if err == nil {
		//nolint:errcheck
		_ = a.w.Flush()
	}
	return n, err
}

// wrapBufioWriter 将 bufio.Writer 包装为 io.Writer。
//
// 创建一个 bufioWriterAdapter 实例，使 bufio.Writer 能够
// 在每次写入后自动 Flush，满足流式输出的需求。
//
// 参数：
//   - w: 待包装的 bufio.Writer
//
// 返回值：
//   - io.Writer: 包装后的 io.Writer 接口实例
func wrapBufioWriter(w *bufio.Writer) io.Writer {
	return &bufioWriterAdapter{w: w}
}
