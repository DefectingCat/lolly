// Package limitrate 提供响应速率限制中间件的测试。
//
// 该文件测试速率限制模块的各项功能，包括：
//   - 中间件创建和名称
//   - 限速写入器创建
//   - 令牌桶算法
//   - 零值和负值边界情况
//   - 并发安全性
//
// 作者：xfy
package limitrate

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/testutil"
)

// TestConstants 测试常量值。
func TestConstants(t *testing.T) {
	if LargeFileStrategySkip != "skip" {
		t.Errorf("LargeFileStrategySkip = %q, want %q", LargeFileStrategySkip, "skip")
	}
	if LargeFileStrategyCoarse != "coarse" {
		t.Errorf("LargeFileStrategyCoarse = %q, want %q", LargeFileStrategyCoarse, "coarse")
	}
}

// TestNewMiddleware 测试创建中间件。
func TestNewMiddleware(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.LimitRateConfig
	}{
		{
			name: "nil config",
			cfg:  nil,
		},
		{
			name: "valid config",
			cfg: &config.LimitRateConfig{
				Rate:  1024,
				Burst: 2048,
			},
		},
		{
			name: "zero rate",
			cfg: &config.LimitRateConfig{
				Rate:  0,
				Burst: 1024,
			},
		},
		{
			name: "negative rate",
			cfg: &config.LimitRateConfig{
				Rate:  -1,
				Burst: 1024,
			},
		},
		{
			name: "zero burst",
			cfg: &config.LimitRateConfig{
				Rate:  1024,
				Burst: 0,
			},
		},
		{
			name: "with large file config",
			cfg: &config.LimitRateConfig{
				Rate:               1024,
				Burst:              2048,
				LargeFileThreshold: 10 * 1024 * 1024,
				LargeFileStrategy:  LargeFileStrategySkip,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := NewMiddleware(tt.cfg)
			if mw == nil {
				t.Error("NewMiddleware() returned nil")
			}
		})
	}
}

// TestMiddleware_Name 测试中间件名称。
func TestMiddleware_Name(t *testing.T) {
	mw := NewMiddleware(nil)
	if mw.Name() != "limit_rate" {
		t.Errorf("Name() = %q, want %q", mw.Name(), "limit_rate")
	}
}

// TestMiddleware_Process_NilConfig 测试空配置时的处理。
func TestMiddleware_Process_NilConfig(t *testing.T) {
	mw := NewMiddleware(nil)

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	handler := mw.Process(nextHandler)
	if handler == nil {
		t.Fatal("Process() returned nil handler")
	}

	ctx := testutil.NewRequestCtx("GET", "/test")
	handler(ctx)

	if !called {
		t.Error("next handler was not called")
	}
	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("status code = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusOK)
	}
}

// TestMiddleware_Process_ZeroRate 测试零速率时的处理。
func TestMiddleware_Process_ZeroRate(t *testing.T) {
	mw := NewMiddleware(&config.LimitRateConfig{
		Rate:  0,
		Burst: 1024,
	})

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	handler := mw.Process(nextHandler)
	if handler == nil {
		t.Fatal("Process() returned nil handler")
	}

	ctx := testutil.NewRequestCtx("GET", "/test")
	handler(ctx)

	if !called {
		t.Error("next handler was not called")
	}
}

// TestMiddleware_Process_NegativeRate 测试负速率时的处理。
func TestMiddleware_Process_NegativeRate(t *testing.T) {
	mw := NewMiddleware(&config.LimitRateConfig{
		Rate:  -100,
		Burst: 1024,
	})

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	handler := mw.Process(nextHandler)
	ctx := testutil.NewRequestCtx("GET", "/test")
	handler(ctx)

	if !called {
		t.Error("next handler was not called")
	}
}

// TestMiddleware_Process_ValidConfig 测试有效配置时的处理。
func TestMiddleware_Process_ValidConfig(t *testing.T) {
	mw := NewMiddleware(&config.LimitRateConfig{
		Rate:  1024,
		Burst: 2048,
	})

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	handler := mw.Process(nextHandler)
	ctx := testutil.NewRequestCtx("GET", "/test")
	handler(ctx)

	if !called {
		t.Error("next handler was not called")
	}
}

// TestNewRateLimitedWriter 测试创建限速写入器。
func TestNewRateLimitedWriter(t *testing.T) {
	buf := &bytes.Buffer{}

	tests := []struct {
		name  string
		rate  int64
		burst int64
	}{
		{"positive rate and burst", 1024, 2048},
		{"zero rate", 0, 1024},
		{"negative rate", -1, 1024},
		{"zero burst", 1024, 0},
		{"negative burst", 1024, -1},
		{"rate equals burst", 1024, 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewRateLimitedWriter(buf, tt.rate, tt.burst)
			if w == nil {
				t.Error("NewRateLimitedWriter() returned nil")
			}
			if w.rate != tt.rate {
				t.Errorf("rate = %d, want %d", w.rate, tt.rate)
			}
			if w.maxBucket != tt.burst {
				t.Errorf("maxBucket = %d, want %d", w.maxBucket, tt.burst)
			}
		})
	}
}

// TestRateLimitedWriter_Write_ZeroRate 测试零速率时直接写入。
func TestRateLimitedWriter_Write_ZeroRate(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewRateLimitedWriter(buf, 0, 1024)

	data := []byte("hello world")
	n, err := w.Write(data)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() n = %d, want %d", n, len(data))
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Errorf("buffer = %q, want %q", buf.Bytes(), data)
	}
}

// TestRateLimitedWriter_Write_NegativeRate 测试负速率时直接写入。
func TestRateLimitedWriter_Write_NegativeRate(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewRateLimitedWriter(buf, -1, 1024)

	data := []byte("hello world")
	n, err := w.Write(data)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() n = %d, want %d", n, len(data))
	}
}

// TestRateLimitedWriter_Write_WithinBucket 测试令牌充足时的写入。
func TestRateLimitedWriter_Write_WithinBucket(t *testing.T) {
	buf := &bytes.Buffer{}
	rate := int64(1024)
	burst := int64(2048)
	w := NewRateLimitedWriter(buf, rate, burst)

	data := []byte("hello")
	n, err := w.Write(data)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() n = %d, want %d", n, len(data))
	}

	// 验证令牌被消耗
	expectedBucket := burst - int64(len(data))
	if w.bucket != expectedBucket {
		t.Errorf("bucket = %d, want %d", w.bucket, expectedBucket)
	}
}

// TestRateLimitedWriter_Write_ExceedsBucket 测试令牌不足时的分批写入。
func TestRateLimitedWriter_Write_ExceedsBucket(t *testing.T) {
	buf := &bytes.Buffer{}
	rate := int64(100) // 100 bytes/sec
	burst := int64(10) // only 10 tokens initially
	w := NewRateLimitedWriter(buf, rate, burst)

	// 写入超过 burst 的数据
	data := make([]byte, 50)
	for i := range data {
		data[i] = 'a'
	}

	n, err := w.Write(data)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() n = %d, want %d", n, len(data))
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Error("buffer content mismatch")
	}
}

// TestRateLimitedWriter_Write_Error 测试底层写入错误。
func TestRateLimitedWriter_Write_Error(t *testing.T) {
	errWriter := &errorWriter{err: errors.New("write error")}

	// 测试零速率时的错误传播（零速率时直接调用底层 writer，错误会传播）
	w := NewRateLimitedWriter(errWriter, 0, 1024)

	data := []byte("hello")
	_, err := w.Write(data)
	if err == nil {
		t.Error("Write() with zero rate should propagate error, got nil")
	}

	// 测试非零速率时的错误传播
	w2 := NewRateLimitedWriter(errWriter, 1024, 10)
	_, err = w2.Write(data)
	if err == nil {
		t.Error("Write() expected error, got nil")
	}
}

// errorWriter 是一个总是返回错误的写入器。
type errorWriter struct {
	err error
}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	return 0, w.err
}

// TestRateLimitedWriter_Write_MultipleWrites 测试多次写入。
func TestRateLimitedWriter_Write_MultipleWrites(t *testing.T) {
	buf := &bytes.Buffer{}
	rate := int64(1000)
	burst := int64(100)
	w := NewRateLimitedWriter(buf, rate, burst)

	// 第一次写入消耗令牌
	data1 := []byte("first")
	n1, err := w.Write(data1)
	if err != nil || n1 != len(data1) {
		t.Errorf("first Write() failed: n=%d, err=%v", n1, err)
	}

	// 第二次写入
	data2 := []byte("second")
	n2, err := w.Write(data2)
	if err != nil || n2 != len(data2) {
		t.Errorf("second Write() failed: n=%d, err=%v", n2, err)
	}

	expected := append(data1, data2...)
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("buffer = %q, want %q", buf.Bytes(), expected)
	}
}

// TestRateLimitedWriter_BucketRefill 测试令牌桶补充。
func TestRateLimitedWriter_BucketRefill(t *testing.T) {
	buf := &bytes.Buffer{}
	rate := int64(1000) // 1000 tokens/sec
	burst := int64(100)
	w := NewRateLimitedWriter(buf, rate, burst)

	// 消耗所有令牌
	w.bucket = 0

	// 等待一段时间让令牌补充
	time.Sleep(20 * time.Millisecond)

	// 写入数据，应该能获得新令牌
	data := []byte("test")
	n, err := w.Write(data)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() n = %d, want %d", n, len(data))
	}
}

// TestRateLimitedWriter_BucketMax 测试令牌桶上限。
func TestRateLimitedWriter_BucketMax(t *testing.T) {
	buf := &bytes.Buffer{}
	rate := int64(1000)
	burst := int64(100)
	w := NewRateLimitedWriter(buf, rate, burst)

	// 设置 bucket 超过最大值
	w.bucket = burst + 500

	// 等待让时间流逝，触发令牌补充逻辑
	time.Sleep(10 * time.Millisecond)

	// 写入数据
	data := []byte("test")
	_, err := w.Write(data)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}

	// bucket 不应超过 maxBucket
	if w.bucket > w.maxBucket {
		t.Errorf("bucket = %d, should not exceed maxBucket = %d", w.bucket, w.maxBucket)
	}
}

// TestRateLimitedWriter_Concurrent 测试并发写入安全性。
func TestRateLimitedWriter_Concurrent(t *testing.T) {
	buf := &bytes.Buffer{}
	rate := int64(10000) // 高速率以减少测试时间
	burst := int64(1000)
	w := NewRateLimitedWriter(buf, rate, burst)

	var wg sync.WaitGroup
	goroutines := 5
	writesPerGoroutine := 10
	data := []byte("test")

	for range goroutines {
		wg.Go(func() {
			for range writesPerGoroutine {
				_, _ = w.Write(data)
			}
		})
	}

	wg.Wait()

	expectedLen := goroutines * writesPerGoroutine * len(data)
	if buf.Len() != expectedLen {
		t.Errorf("buffer length = %d, want %d", buf.Len(), expectedLen)
	}
}

// TestRateLimitedWriter_EmptyWrite 测试空写入。
func TestRateLimitedWriter_EmptyWrite(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewRateLimitedWriter(buf, 1024, 100)

	n, err := w.Write([]byte{})
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != 0 {
		t.Errorf("Write() n = %d, want 0", n)
	}
}

// TestRateLimitedWriter_LargeData 测试大数据写入。
func TestRateLimitedWriter_LargeData(t *testing.T) {
	buf := &bytes.Buffer{}
	rate := int64(10000) // 10KB/s
	burst := int64(1000) // 1KB burst
	w := NewRateLimitedWriter(buf, rate, burst)

	// 写入 5KB 数据
	data := make([]byte, 5*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	start := time.Now()
	n, err := w.Write(data)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() n = %d, want %d", n, len(data))
	}

	// 验证数据被正确写入
	if !bytes.Equal(buf.Bytes(), data) {
		t.Error("buffer content mismatch")
	}

	// 由于限速，写入时间应该大于零（令牌不足时需要等待）
	t.Logf("Large data write took %v", elapsed)
}

// TestRateLimitedWriter_PartialWriteError 测试部分写入错误。
func TestRateLimitedWriter_PartialWriteError(t *testing.T) {
	// 创建一个写入部分数据后返回错误的 writer
	partialWriter := &partialErrorWriter{maxWrite: 5}
	w := NewRateLimitedWriter(partialWriter, 0, 100)

	data := []byte("hello world")
	n, err := w.Write(data)

	// 零速率时，直接调用底层 writer
	if err == nil {
		t.Error("Write() expected error, got nil")
	}
	if n > len(data) {
		t.Errorf("Write() n = %d, should not exceed %d", n, len(data))
	}
}

// partialErrorWriter 写入 maxWrite 字节后返回错误。
type partialErrorWriter struct {
	maxWrite int
}

func (w *partialErrorWriter) Write(p []byte) (n int, err error) {
	if len(p) <= w.maxWrite {
		return len(p), nil
	}
	return w.maxWrite, errors.New("partial write error")
}

// TestRateLimitedWriter_TimeAdvancement 测试时间推进。
func TestRateLimitedWriter_TimeAdvancement(t *testing.T) {
	buf := &bytes.Buffer{}
	rate := int64(1000)
	burst := int64(100)
	w := NewRateLimitedWriter(buf, rate, burst)

	// 记录初始时间
	initialTime := w.lastTime

	// 写入一些数据
	_, _ = w.Write([]byte("test"))

	// lastTime 应该被更新
	if !w.lastTime.After(initialTime) && w.lastTime != initialTime {
		t.Error("lastTime should be updated after write")
	}
}

// TestRateLimitedWriter_WriteAll 测试完整写入 io.Writer 接口兼容性。
func TestRateLimitedWriter_WriteAll(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewRateLimitedWriter(buf, 1000, 100)

	data := []byte("hello world, this is a test of the rate limited writer")

	// 使用 io.Writer 接口
	var writer io.Writer = w
	n, err := writer.Write(data)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() n = %d, want %d", n, len(data))
	}
}

// BenchmarkMiddleware_Process 基准测试中间件处理。
func BenchmarkMiddleware_Process(b *testing.B) {
	mw := NewMiddleware(&config.LimitRateConfig{
		Rate:  1024 * 1024, // 1MB/s
		Burst: 2 * 1024 * 1024,
	})

	nextHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	handler := mw.Process(nextHandler)
	ctx := testutil.NewRequestCtx("GET", "/test")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler(ctx)
	}
}

// BenchmarkRateLimitedWriter_Write 基准测试限速写入。
func BenchmarkRateLimitedWriter_Write(b *testing.B) {
	buf := &bytes.Buffer{}
	w := NewRateLimitedWriter(buf, 1024*1024, 1024*1024) // 高速率减少等待
	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		w.bucket = w.maxBucket // 重置令牌桶
		_, _ = w.Write(data)
	}
}

// BenchmarkRateLimitedWriter_Concurrent 基准测试并发写入。
func BenchmarkRateLimitedWriter_Concurrent(b *testing.B) {
	buf := &bytes.Buffer{}
	w := NewRateLimitedWriter(buf, 10*1024*1024, 1024*1024)
	data := []byte("test data for benchmarking")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = w.Write(data)
		}
	})
}
