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
	al.Close()
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

	al.Close()
}