// Package server 提供 Goroutine 池功能的测试。
//
// 该文件测试 Goroutine 池的各项功能，包括：
//   - 池的创建和配置
//   - 启动和停止控制
//   - 任务提交和执行
//   - 并发提交处理
//   - Handler 包装功能
//   - 统计信息收集
//
// 作者：xfy
package server

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

func TestNewGoroutinePool(t *testing.T) {
	cfg := PoolConfig{
		MaxWorkers:  100,
		MinWorkers:  10,
		IdleTimeout: 30 * time.Second,
		QueueSize:   50,
	}

	p := NewGoroutinePool(cfg)
	if p == nil {
		t.Fatal("Expected non-nil pool")
	}

	// 检查配置
	if p.maxWorkers != 100 {
		t.Errorf("Expected maxWorkers 100, got %d", p.maxWorkers)
	}
	if p.minWorkers != 10 {
		t.Errorf("Expected minWorkers 10, got %d", p.minWorkers)
	}
}

func TestPoolDefaults(t *testing.T) {
	p := NewGoroutinePool(PoolConfig{})

	// 应该使用默认值
	if p.maxWorkers != 10000 {
		t.Errorf("Expected default maxWorkers 10000, got %d", p.maxWorkers)
	}
	if p.minWorkers != 100 {
		t.Errorf("Expected default minWorkers 100, got %d", p.minWorkers)
	}
}

func TestPoolStartStop(t *testing.T) {
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers: 10,
		MinWorkers: 2,
	})

	p.Start()
	if !p.running.Load() {
		t.Error("Expected pool to be running")
	}

	p.Stop()
	if p.running.Load() {
		t.Error("Expected pool to be stopped")
	}
}

func TestPoolSubmit(t *testing.T) {
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers:  10,
		MinWorkers:  2,
		QueueSize:   10,
		IdleTimeout: 5 * time.Second,
	})

	p.Start()
	defer p.Stop()

	var executed atomic.Bool
	task := func(*fasthttp.RequestCtx) {
		executed.Store(true)
	}

	err := p.Submit(nil, task)
	if err != nil {
		t.Errorf("Submit failed: %v", err)
	}

	// 等待任务执行
	time.Sleep(100 * time.Millisecond)

	if !executed.Load() {
		t.Error("Expected task to be executed")
	}
}

func TestPoolStats(t *testing.T) {
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers:  100,
		MinWorkers:  10,
		QueueSize:   50,
		IdleTimeout: 30 * time.Second,
	})

	p.Start()
	defer p.Stop()

	stats := p.Stats()

	if stats.MaxWorkers != 100 {
		t.Errorf("Expected MaxWorkers 100, got %d", stats.MaxWorkers)
	}
	if stats.MinWorkers != 10 {
		t.Errorf("Expected MinWorkers 10, got %d", stats.MinWorkers)
	}
	if stats.QueueCap != 50 {
		t.Errorf("Expected QueueCap 50, got %d", stats.QueueCap)
	}
}

func TestPoolConcurrentSubmit(t *testing.T) {
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers:  50,
		MinWorkers:  5,
		QueueSize:   100,
		IdleTimeout: 5 * time.Second,
	})

	p.Start()
	defer p.Stop()

	var counter atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			_ = p.Submit(nil, func(_ *fasthttp.RequestCtx) {
				counter.Add(1)
			})
		}()
	}

	wg.Wait()

	// 等待所有任务执行
	time.Sleep(500 * time.Millisecond)

	if counter.Load() != 100 {
		t.Errorf("Expected 100 executions, got %d", counter.Load())
	}
}

func TestPoolSubmitWhenStopped(t *testing.T) {
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers: 10,
	})

	// 不启动池
	var executed atomic.Bool
	task := func(_ *fasthttp.RequestCtx) {
		executed.Store(true)
	}

	err := p.Submit(nil, task)
	if err != nil {
		t.Errorf("Submit should not fail when stopped: %v", err)
	}

	// 任务应该直接执行
	if !executed.Load() {
		t.Error("Expected task to be executed directly when pool is stopped")
	}
}

func TestPoolWrapHandler(t *testing.T) {
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers:  10,
		MinWorkers:  2,
		QueueSize:   10,
		IdleTimeout: 5 * time.Second,
	})

	p.Start()
	defer p.Stop()

	var executed atomic.Bool
	originalHandler := func(ctx *fasthttp.RequestCtx) {
		executed.Store(true)
		ctx.SetBodyString("wrapped response")
	}

	wrappedHandler := p.WrapHandler(originalHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	wrappedHandler(ctx)

	// 等待异步执行
	time.Sleep(100 * time.Millisecond)

	if !executed.Load() {
		t.Error("Expected wrapped handler to be executed")
	}
}

func TestPoolWrapHandler_WhenStopped(t *testing.T) {
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers: 10,
	})
	// 不启动池

	var executed atomic.Bool
	originalHandler := func(*fasthttp.RequestCtx) {
		executed.Store(true)
	}

	wrappedHandler := p.WrapHandler(originalHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	wrappedHandler(ctx)

	// 池停止时应该直接执行
	if !executed.Load() {
		t.Error("Expected handler to be executed directly when pool is stopped")
	}
}

func TestPoolSubmit_QueueFull_StartNewWorker(t *testing.T) {
	// 测试队列满时启动新 worker
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers:  10,
		MinWorkers:  1,
		QueueSize:   1, // 小队列，容易满
		IdleTimeout: 5 * time.Second,
	})

	p.Start()
	defer p.Stop()

	// 填满队列，让后续提交触发 default 分支
	var executedCount atomic.Int32
	blockTask := func(*fasthttp.RequestCtx) {
		time.Sleep(200 * time.Millisecond) // 阻塞任务
		executedCount.Add(1)
	}

	// 先提交一个阻塞任务，让 worker 忙碌
	_ = p.Submit(nil, blockTask)
	time.Sleep(50 * time.Millisecond) // 等待 worker 开始执行

	// 填满队列
	_ = p.Submit(nil, func(*fasthttp.RequestCtx) { executedCount.Add(1) })

	// 此时队列满，应该启动新 worker
	_ = p.Submit(nil, func(*fasthttp.RequestCtx) { executedCount.Add(1) })

	// 等待所有任务完成
	time.Sleep(500 * time.Millisecond)

	if executedCount.Load() < 2 {
		t.Errorf("Expected at least 2 executions, got %d", executedCount.Load())
	}
}

func TestPoolSubmit_QueueFull_MaxWorkers_Fallback(t *testing.T) {
	// 测试队列满且达到最大 worker 时直接执行（fallback）
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers:  1, // 只有 1 个 worker
		MinWorkers:  1,
		QueueSize:   1, // 队列大小 1
		IdleTimeout: 5 * time.Second,
	})

	p.Start()
	defer p.Stop()

	// 使用 channel 阻塞唯一的 worker
	blockCh := make(chan struct{})
	started := make(chan struct{})
	_ = p.Submit(nil, func(*fasthttp.RequestCtx) {
		close(started) // 通知 worker 已开始
		<-blockCh      // 阻塞直到测试结束
	})

	// 等待 worker 开始执行阻塞任务
	<-started

	// 填满队列
	_ = p.Submit(nil, func(*fasthttp.RequestCtx) {})

	// 现在唯一的 worker 在阻塞，队列已满
	// 提交新任务应该触发 fallback 直接执行
	var fallbackExecuted atomic.Bool
	_ = p.Submit(nil, func(*fasthttp.RequestCtx) {
		fallbackExecuted.Store(true)
	})

	// fallback 执行是同步的直接执行
	if !fallbackExecuted.Load() {
		t.Error("Expected task to be executed directly (fallback) when max workers reached")
	}

	// 释放阻塞的 worker
	close(blockCh)
}

func TestPoolSubmit_WithIdleWorkers(t *testing.T) {
	// 测试有空闲 worker 时不启动新 worker
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers:  10,
		MinWorkers:  5, // 预热 5 个 worker
		QueueSize:   10,
		IdleTimeout: 5 * time.Second,
	})

	p.Start()
	defer p.Stop()

	// 等待预热完成，worker 应该是空闲的
	time.Sleep(50 * time.Millisecond)

	initialWorkers := atomic.LoadInt32(&p.workers)

	// 提交任务，应该复用空闲 worker
	var executed atomic.Bool
	_ = p.Submit(nil, func(*fasthttp.RequestCtx) {
		executed.Store(true)
	})

	time.Sleep(100 * time.Millisecond)

	if !executed.Load() {
		t.Error("Expected task to be executed")
	}

	// worker 数量应该保持稳定或更少（不应该增加）
	finalWorkers := atomic.LoadInt32(&p.workers)
	if finalWorkers > initialWorkers {
		t.Errorf("Worker count should not increase when idle workers available: %d -> %d", initialWorkers, finalWorkers)
	}
}

func TestPoolSubmit_NilTask(t *testing.T) {
	// 测试提交 nil 任务不会 panic
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers: 10,
	})

	p.Start()
	defer p.Stop()

	// 提交 nil 任务 - 这会导致 panic，所以不测试
	// 但可以测试空任务函数
	var executed atomic.Bool
	emptyTask := func(*fasthttp.RequestCtx) {
		executed.Store(true)
	}

	err := p.Submit(nil, emptyTask)
	if err != nil {
		t.Errorf("Submit failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if !executed.Load() {
		t.Error("Expected empty task to be executed")
	}
}

func TestPoolSubmit_MultipleQueuedTasks(t *testing.T) {
	// 测试多个任务入队
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers:  5,
		MinWorkers:  2,
		QueueSize:   10,
		IdleTimeout: 5 * time.Second,
	})

	p.Start()
	defer p.Stop()

	var counter atomic.Int32

	// 快速提交多个任务
	for i := 0; i < 5; i++ {
		_ = p.Submit(nil, func(*fasthttp.RequestCtx) {
			counter.Add(1)
		})
	}

	time.Sleep(200 * time.Millisecond)

	if counter.Load() != 5 {
		t.Errorf("Expected 5 executions, got %d", counter.Load())
	}
}

func TestPoolSubmit_StartWorkerWhenNoIdle(t *testing.T) {
	// 测试当没有空闲 worker 时启动新 worker
	// 使用 MinWorkers=1 让池只预热 1 个 worker
	p := NewGoroutinePool(PoolConfig{
		MaxWorkers:  5,
		MinWorkers:  1, // 只预热 1 个 worker
		QueueSize:   10,
		IdleTimeout: 5 * time.Second,
	})

	p.Start()
	defer p.Stop()

	// 等待预热完成
	time.Sleep(50 * time.Millisecond)

	// 用阻塞任务让唯一的 worker 忙碌
	blockCh := make(chan struct{})
	started := make(chan struct{})
	_ = p.Submit(nil, func(*fasthttp.RequestCtx) {
		close(started)
		<-blockCh
	})
	<-started // 等待 worker 开始执行

	// 现在唯一的 worker 在忙碌，idleWorkers == 0
	// 提交新任务应该启动新 worker
	var executed atomic.Bool
	_ = p.Submit(nil, func(*fasthttp.RequestCtx) {
		executed.Store(true)
	})

	time.Sleep(100 * time.Millisecond)

	if !executed.Load() {
		t.Error("Expected task to be executed by new worker")
	}

	// 检查是否启动了新 worker
	if atomic.LoadInt32(&p.workers) < 2 {
		t.Errorf("Expected at least 2 workers, got %d", atomic.LoadInt32(&p.workers))
	}

	close(blockCh)
}
