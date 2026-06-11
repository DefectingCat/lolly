// Package accesslog 提供访问日志功能的测试。
//
// 该文件测试访问日志模块的各项功能，包括：
//   - 访问日志中间件创建
//   - 请求处理和响应记录
//   - 请求持续时间记录
//   - 日志格式化
//
// 作者：xfy
package accesslog

import (
	"bytes"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

func TestAccessLog_Name(t *testing.T) {
	al := New(&config.LoggingConfig{})
	if al.Name() != "accesslog" {
		t.Errorf("expected name 'accesslog', got '%s'", al.Name())
	}
}

func TestAccessLog_Process(t *testing.T) {
	al := New(&config.LoggingConfig{
		Access: config.AccessLogConfig{Format: "json"},
	})

	// 创建一个简单的 handler
	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
		ctx.SetBodyString("hello")
	}

	// 包装 handler
	wrapped := al.Process(handler)

	// 创建模拟请求上下文
	var ctx fasthttp.RequestCtx
	ctx.Init(&fasthttp.Request{}, nil, nil)

	// 执行
	wrapped(&ctx)

	// 验证响应未被修改
	if ctx.Response.StatusCode() != 200 {
		t.Errorf("expected status 200, got %d", ctx.Response.StatusCode())
	}
	if !bytes.Equal(ctx.Response.Body(), []byte("hello")) {
		t.Errorf("expected body 'hello', got '%s'", ctx.Response.Body())
	}

	// 清理
	_ = al.Close()
}

func TestAccessLog_ProcessWithDuration(t *testing.T) {
	al := New(&config.LoggingConfig{
		Access: config.AccessLogConfig{Format: "json"},
	})

	// 创建一个有延迟的 handler
	handler := func(ctx *fasthttp.RequestCtx) {
		time.Sleep(10 * time.Millisecond)
		ctx.SetStatusCode(201)
		ctx.SetBodyString("created")
	}

	wrapped := al.Process(handler)

	var ctx fasthttp.RequestCtx
	ctx.Init(&fasthttp.Request{}, nil, nil)

	start := time.Now()
	wrapped(&ctx)
	elapsed := time.Since(start)

	// 验证延迟被记录（至少 10ms）
	if elapsed < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", elapsed)
	}

	_ = al.Close()
}

func TestAccessLog_SampleRateAlwaysRecordErrors(t *testing.T) {
	al := New(&config.LoggingConfig{
		Access: config.AccessLogConfig{
			Format:     "json",
			SampleRate: 0.0, // 理论上不采样成功请求，但错误始终记录
		},
	})

	// 非 2xx 请求应始终记录
	for _, status := range []int{199, 300, 400, 500} {
		if !al.shouldLog(status) {
			t.Errorf("status %d should always be logged regardless of sample rate", status)
		}
	}

	_ = al.Close()
}

func TestAccessLog_SampleRateDistribution(t *testing.T) {
	al := New(&config.LoggingConfig{
		Access: config.AccessLogConfig{
			Format:     "json",
			SampleRate: 0.1, // 10% 采样
		},
	})

	// 重置计数器以便测试
	al.sampleCounter.Store(0)

	logged := 0
	for i := 0; i < 1000; i++ {
		if al.shouldLog(200) {
			logged++
		}
	}

	// 1000 个请求，10% 采样，应记录约 100 个（允许 20% 误差）
	if logged < 80 || logged > 120 {
		t.Errorf("expected ~100 logged requests with 10%% sample rate, got %d", logged)
	}

	_ = al.Close()
}
