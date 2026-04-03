// Package server 提供 Goroutine 池，限制并发数量，减少调度开销。
package server

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
)

// GoroutinePool Goroutine 池配置。
type GoroutinePool struct {
	maxWorkers  int32          // 最大 worker 数
	minWorkers  int32          // 最小 worker 数（预热）
	idleTimeout time.Duration  // 穴闲超时
	taskQueue   chan Task      // 任务队列
	workers     int32          // 当前 worker 数
	idleWorkers int32          // 穴闲 worker 数
	running     atomic.Bool    // 运行状态
	wg          sync.WaitGroup // 等待所有 worker
	ctx         context.Context
	cancel      context.CancelFunc
}

// Task 任务函数类型。
type Task func(*fasthttp.RequestCtx)

// PoolConfig 池配置。
type PoolConfig struct {
	MaxWorkers  int           // 最大并发数
	MinWorkers  int           // 预热 worker 数
	IdleTimeout time.Duration // 穴闲超时
	QueueSize   int           // 任务队列大小
}

// NewGoroutinePool 创建 Goroutine 池。
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

	// 预热 worker
	for i := 0; i < cfg.MinWorkers; i++ {
		p.startWorker()
	}

	return p
}

// Start 启动池。
func (p *GoroutinePool) Start() {
	p.running.Store(true)
}

// Stop 停止池。
func (p *GoroutinePool) Stop() {
	p.running.Store(false)
	p.cancel()
	p.wg.Wait()
}

// Submit 提交任务。
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
		if p.idleWorkers == 0 && p.workers < p.maxWorkers {
			p.startWorker()
		}
		return nil
	default:
		// 队列满，需要启动新 worker 或直接执行
		if p.workers < p.maxWorkers {
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

// startWorker 启动一个 worker。
func (p *GoroutinePool) startWorker() {
	p.workers++
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
				task(nil) // 注意：fasthttp.RequestCtx 需要在任务中传入

			case <-idleTimer.C:
				// 穴闲超时，退出 worker（保持最小数量）
				atomic.AddInt32(&p.idleWorkers, -1)
				if p.workers > p.minWorkers {
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

// Stats 返回池统计信息。
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

// PoolStats 池统计信息。
type PoolStats struct {
	Workers     int32
	IdleWorkers int32
	MaxWorkers  int32
	MinWorkers  int32
	QueueLen    int32
	QueueCap    int32
}

// WrapHandler 使用池包装 fasthttp 处理器。
func (p *GoroutinePool) WrapHandler(handler fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// 使用池执行处理器
		p.Submit(ctx, func(innerCtx *fasthttp.RequestCtx) {
			handler(ctx)
		})
	}
}
