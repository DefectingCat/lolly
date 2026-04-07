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
	task := func(ctx *fasthttp.RequestCtx) {
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
			_ = p.Submit(nil, func(ctx *fasthttp.RequestCtx) {
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
	task := func(ctx *fasthttp.RequestCtx) {
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
	originalHandler := func(ctx *fasthttp.RequestCtx) {
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
