// Package server 提供了带中间件支持、虚拟主机和状态监控功能的 HTTP 服务器。
//
// 该文件实现了 Goroutine 池，用于限制并发数量、减少调度开销、
// 提升高并发场景下的性能表现。
//
// 主要功能：
//   - Worker 池管理：动态创建和销毁 Goroutine worker
//   - 任务队列：缓冲待处理的请求任务
//   - 预热机制：启动时创建最小数量的 worker
//   - 空闲回收：自动回收长时间空闲的 worker
//
// 使用示例：
//
//	pool := server.NewGoroutinePool(server.PoolConfig{
//	    MaxWorkers:  10000, // 最大并发数
//	    MinWorkers:  100,   // 预热 worker 数
//	    IdleTimeout: 60 * time.Second, // 空闲超时
//	    QueueSize:   1000,  // 任务队列大小
//	})
//	pool.Start()
//
//	// 包装处理器
//	handler = pool.WrapHandler(finalHandler)
//
// 作者：xfy
package server

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
)

// GoroutinePool Goroutine 池实现。
//
// 通过复用 Goroutine 减少创建销毁开销，控制最大并发数防止资源耗尽。
// 支持预热、任务队列和空闲 worker 回收。
//
// 注意事项：
//   - 所有方法均为并发安全
//   - 使用前需调用 Start 启动池
//   - 使用后需调用 Stop 释放资源
type GoroutinePool struct {
	maxWorkers  int32              // 最大 worker 数量
	minWorkers  int32              // 最小 worker 数量（预热）
	idleTimeout time.Duration      // 空闲超时时间
	taskQueue   chan Task          // 任务队列通道
	workers     int32              // 当前活跃 worker 数量
	idleWorkers int32              // 当前空闲 worker 数量
	running     atomic.Bool        // 池运行状态标志
	wg          sync.WaitGroup     // 等待所有 worker 退出
	ctx         context.Context    // 上下文，用于取消信号
	cancel      context.CancelFunc // 取消函数
}

// Task 任务函数类型。
//
// 定义池中执行的任务函数签名，接收请求上下文作为参数。
type Task func(*fasthttp.RequestCtx)

// PoolConfig Goroutine 池配置结构。
//
// 定义池的各项参数，包括并发限制、预热数量和超时设置。
type PoolConfig struct {
	MaxWorkers  int           // 最大并发 worker 数量
	MinWorkers  int           // 预热时创建的最小 worker 数量
	IdleTimeout time.Duration // worker 空闲超时时间，超时后回收
	QueueSize   int           // 任务队列缓冲大小
}

// NewGoroutinePool 创建 Goroutine 池实例。
//
// 根据配置创建池，设置合理的默认值，并预热最小数量的 worker。
// 默认配置：MaxWorkers=10000, MinWorkers=100, IdleTimeout=60s, QueueSize=1000。
//
// 参数：
//   - cfg: 池配置参数
//
// 返回值：
//   - *GoroutinePool: 配置好的池实例
func NewGoroutinePool(cfg PoolConfig) *GoroutinePool {
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 10000
	}
	if cfg.MinWorkers <= 0 {
		cfg.MinWorkers = 100
	}
	if cfg.MinWorkers > cfg.MaxWorkers {
		cfg.MinWorkers = cfg.MaxWorkers
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 60 * time.Second
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1000
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &GoroutinePool{
		maxWorkers:  int32(cfg.MaxWorkers),
		minWorkers:  int32(cfg.MinWorkers),
		idleTimeout: cfg.IdleTimeout,
		taskQueue:   make(chan Task, cfg.QueueSize),
		ctx:         ctx,
		cancel:      cancel,
	}

	// 预热最小数量的 worker
	for i := 0; i < cfg.MinWorkers; i++ {
		p.startWorker()
	}

	return p
}

// Start 启动 Goroutine 池。
//
// 设置运行状态标志，池开始接受任务提交。
func (p *GoroutinePool) Start() {
	p.running.Store(true)
}

// Stop 停止 Goroutine 池。
//
// 取消所有 worker，等待它们退出完成。
// 调用后池将不再接受新任务。
func (p *GoroutinePool) Stop() {
	p.running.Store(false)
	p.cancel()
	p.wg.Wait()
}

// Submit 提交任务到池。
//
// 将任务放入队列等待执行，如果池未运行则直接执行。
// 当队列满且未达到最大 worker 数时，会启动新 worker。
//
// 参数：
//   - ctx: 请求上下文（传递给任务函数）
//   - task: 待执行的任务函数
//
// 返回值：
//   - error: 当前实现总是返回 nil
func (p *GoroutinePool) Submit(ctx *fasthttp.RequestCtx, task Task) error {
	if !p.running.Load() {
		// 池未运行，直接执行
		task(ctx)
		return nil
	}

	// 尝试放入队列
	select {
	case p.taskQueue <- task:
		// 任务入队成功
		// 如果有空闲 worker 不足，可能需要启动新 worker
		if atomic.LoadInt32(&p.idleWorkers) == 0 && atomic.LoadInt32(&p.workers) < p.maxWorkers {
			p.startWorker()
		}
		return nil
	default:
		// 队列满，需要启动新 worker 或直接执行
		if atomic.LoadInt32(&p.workers) < p.maxWorkers {
			p.startWorker()
			// 重新尝试入队
			p.taskQueue <- task
			return nil
		}

		// 达到最大 worker，直接执行（fallback）
		task(ctx)
		return nil
	}
}

// startWorker 启动一个 worker Goroutine。
//
// worker 从任务队列获取任务执行，空闲超时后自动退出（保持最小数量）。
func (p *GoroutinePool) startWorker() {
	atomic.AddInt32(&p.workers, 1)
	p.wg.Add(1)

	go func() {
		defer p.wg.Done()
		defer atomic.AddInt32(&p.workers, -1)

		idleTimer := time.NewTimer(p.idleTimeout)
		defer idleTimer.Stop()

		for {
			// 标记为空闲
			atomic.AddInt32(&p.idleWorkers, 1)

			select {
			case task := <-p.taskQueue:
				// 取出任务，取消空闲标记
				atomic.AddInt32(&p.idleWorkers, -1)
				idleTimer.Reset(p.idleTimeout)

				// 执行任务
				// 注意：task 通过闭包捕获了 *fasthttp.RequestCtx，
				// 所以参数传 nil 是安全的，handler 使用闭包中的 ctx
				task(nil)

			case <-idleTimer.C:
				// 空闲超时，退出 worker（保持最小数量）
				atomic.AddInt32(&p.idleWorkers, -1)
				if atomic.LoadInt32(&p.workers) > p.minWorkers {
					return
				}
				idleTimer.Reset(p.idleTimeout)

			case <-p.ctx.Done():
				// 池关闭
				atomic.AddInt32(&p.idleWorkers, -1)
				return
			}
		}
	}()
}

// Stats 返回池的统计信息。
//
// 获取当前 worker 数量、空闲数量、队列状态等统计数据。
//
// 返回值：
//   - PoolStats: 池统计信息结构体
func (p *GoroutinePool) Stats() PoolStats {
	return PoolStats{
		Workers:     atomic.LoadInt32(&p.workers),
		IdleWorkers: atomic.LoadInt32(&p.idleWorkers),
		MaxWorkers:  p.maxWorkers,
		MinWorkers:  p.minWorkers,
		QueueLen:    int32(len(p.taskQueue)),
		QueueCap:    int32(cap(p.taskQueue)),
	}
}

// PoolStats Goroutine 池统计信息结构。
//
// 用于监控池的运行状态，包括 worker 数量和队列状态。
type PoolStats struct {
	// Workers 当前活跃 worker 数量
	Workers int32

	// IdleWorkers 当前空闲 worker 数量
	IdleWorkers int32

	// MaxWorkers 最大 worker 数量上限
	MaxWorkers int32

	// MinWorkers 最小 worker 数量下限
	MinWorkers int32

	// QueueLen 当前队列中的任务数
	QueueLen int32

	// QueueCap 队列容量上限
	QueueCap int32
}

// WrapHandler 使用 Goroutine 池包装 fasthttp 处理器。
//
// 将处理器包装为通过池执行的形式，实现并发控制。
//
// 参数：
//   - handler: 原始的请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的处理器
func (p *GoroutinePool) WrapHandler(handler fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// 使用池执行处理器
		_ = p.Submit(ctx, func(_ *fasthttp.RequestCtx) {
			handler(ctx)
		})
	}
}
