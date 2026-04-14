// Package server 提供了 Goroutine 池的基准测试。
//
// 该文件测试 GoroutinePool 的性能，包括：
//   - 任务提交吞吐量
//   - 并发任务处理性能
//   - 阻塞路径性能（队列满时）
//   - 队列满时的 fallback 行为
//   - Worker 空闲回收机制
//
// 作者：xfy
package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

// BenchmarkGoroutinePoolSubmit 测试任务提交吞吐量。
// 测量单协程下向池提交任务的性能。
func BenchmarkGoroutinePoolSubmit(b *testing.B) {
	pool := NewGoroutinePool(PoolConfig{
		MaxWorkers:  100,
		MinWorkers:  10,
		IdleTimeout: 60 * time.Second,
		QueueSize:   1000,
	})
	pool.Start()
	defer pool.Stop()

	ctx := &fasthttp.RequestCtx{}
	task := func(_ *fasthttp.RequestCtx) {
		// 空任务，只测量提交开销
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Submit(ctx, task)
	}
}

// BenchmarkGoroutinePoolParallel 测试并发任务处理性能。
// 使用多协程并行提交任务，模拟真实高并发场景。
func BenchmarkGoroutinePoolParallel(b *testing.B) {
	pool := NewGoroutinePool(PoolConfig{
		MaxWorkers:  100,
		MinWorkers:  10,
		IdleTimeout: 60 * time.Second,
		QueueSize:   1000,
	})
	pool.Start()
	defer pool.Stop()

	task := func(_ *fasthttp.RequestCtx) {
		// 模拟微小工作负载
		sum := 0
		for j := 0; j < 100; j++ {
			sum += j
		}
		_ = sum
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := &fasthttp.RequestCtx{}
		for pb.Next() {
			_ = pool.Submit(ctx, task)
		}
	})
}

// BenchmarkGoroutinePoolSubmit_BlockingPath 测试阻塞路径性能。
// 模拟队列满时触发阻塞写入的场景（pool.go:183）。
func BenchmarkGoroutinePoolSubmit_BlockingPath(b *testing.B) {
	pool := NewGoroutinePool(PoolConfig{
		MaxWorkers:  10,
		MinWorkers:  0,
		IdleTimeout: 60 * time.Second,
		QueueSize:   1, // 极小的队列，强制触发阻塞路径
	})
	pool.Start()
	defer pool.Stop()

	// 预填充任务使队列饱和
	ctx := &fasthttp.RequestCtx{}
	slowTask := func(_ *fasthttp.RequestCtx) {
		time.Sleep(10 * time.Millisecond)
	}

	// 提交任务使队列保持满状态
	for i := 0; i < 5; i++ {
		go func() {
			for {
				_ = pool.Submit(ctx, slowTask)
			}
		}()
	}

	// 等待队列饱和
	time.Sleep(50 * time.Millisecond)

	task := func(_ *fasthttp.RequestCtx) {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 这会触发阻塞路径：队列满 -> 启动新 worker -> 阻塞写入
		_ = pool.Submit(ctx, task)
	}
}

// BenchmarkGoroutinePoolQueueFull 测试队列满时的 fallback 行为。
// 当达到最大 worker 数且队列满时，任务直接执行。
func BenchmarkGoroutinePoolQueueFull(b *testing.B) {
	pool := NewGoroutinePool(PoolConfig{
		MaxWorkers:  1, // 只有 1 个 worker
		MinWorkers:  1,
		IdleTimeout: 60 * time.Second,
		QueueSize:   0, // 无缓冲队列
	})
	pool.Start()
	defer pool.Stop()

	// 占用唯一的 worker
	ctx := &fasthttp.RequestCtx{}
	blockingTask := func(_ *fasthttp.RequestCtx) {
		time.Sleep(time.Second)
	}
	go pool.Submit(ctx, blockingTask)

	// 等待 worker 被占用
	time.Sleep(10 * time.Millisecond)

	task := func(_ *fasthttp.RequestCtx) {
		// 模拟微小工作负载
		sum := 0
		for j := 0; j < 10; j++ {
			sum += j
		}
		_ = sum
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 这会触发 fallback：直接执行任务
		_ = pool.Submit(ctx, task)
	}
}

// BenchmarkGoroutinePoolWorkerRecycle 测试 Worker 空闲回收性能。
// 测量空闲 worker 超时退出的效率。
func BenchmarkGoroutinePoolWorkerRecycle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		pool := NewGoroutinePool(PoolConfig{
			MaxWorkers:  50,
			MinWorkers:  5,
			IdleTimeout: 1 * time.Millisecond, // 极短的空闲超时
			QueueSize:   100,
		})
		pool.Start()

		// 提交一些任务创建临时 worker
		ctx := &fasthttp.RequestCtx{}
		task := func(_ *fasthttp.RequestCtx) {
			time.Sleep(100 * time.Microsecond)
		}

		for j := 0; j < 30; j++ {
			go pool.Submit(ctx, task)
		}

		// 等待任务完成
		time.Sleep(20 * time.Millisecond)

		// 等待空闲回收
		time.Sleep(50 * time.Millisecond)

		pool.Stop()
	}
}

// BenchmarkGoroutinePoolSubmitWithWork 测试带实际工作负载的任务提交。
// 模拟真实场景：任务有实际计算工作。
func BenchmarkGoroutinePoolSubmitWithWork(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, workers := range sizes {
		b.Run(fmt.Sprintf("Workers%d", workers), func(b *testing.B) {
			pool := NewGoroutinePool(PoolConfig{
				MaxWorkers:  int(workers),
				MinWorkers:  workers / 10,
				IdleTimeout: 60 * time.Second,
				QueueSize:   workers * 10,
			})
			pool.Start()
			defer pool.Stop()

			ctx := &fasthttp.RequestCtx{}
			task := func(_ *fasthttp.RequestCtx) {
				// 模拟中等计算量
				sum := 0
				for i := 0; i < 1000; i++ {
					sum += i
				}
				_ = sum
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = pool.Submit(ctx, task)
			}
		})
	}
}

// BenchmarkGoroutinePoolMinWorkers 测试预热 worker 的性能影响。
// 比较有预热和无预热场景的性能差异。
func BenchmarkGoroutinePoolMinWorkers(b *testing.B) {
	b.Run("WithMinWorkers", func(b *testing.B) {
		pool := NewGoroutinePool(PoolConfig{
			MaxWorkers:  100,
			MinWorkers:  50,
			IdleTimeout: 60 * time.Second,
			QueueSize:   1000,
		})
		pool.Start()
		defer pool.Stop()

		ctx := &fasthttp.RequestCtx{}
		task := func(_ *fasthttp.RequestCtx) {}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = pool.Submit(ctx, task)
		}
	})

	b.Run("NoMinWorkers", func(b *testing.B) {
		pool := NewGoroutinePool(PoolConfig{
			MaxWorkers:  100,
			MinWorkers:  0,
			IdleTimeout: 60 * time.Second,
			QueueSize:   1000,
		})
		pool.Start()
		defer pool.Stop()

		ctx := &fasthttp.RequestCtx{}
		task := func(_ *fasthttp.RequestCtx) {}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = pool.Submit(ctx, task)
		}
	})
}
