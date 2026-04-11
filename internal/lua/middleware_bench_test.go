// Package lua 提供 Lua 中间件性能测试
package lua

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

// BenchmarkLuaMiddlewareOverhead 测试 Lua 中间件开销
// 目标：单请求 Lua overhead < 1ms
func BenchmarkLuaMiddlewareOverhead(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	// 创建简单的 Lua 脚本
	tmpDir := b.TempDir()
	scriptPath := filepath.Join(tmpDir, "simple.lua")
	err = os.WriteFile(scriptPath, []byte(`ngx.say("ok")`), 0644)
	if err != nil {
		b.Fatal(err)
	}

	config := LuaMiddlewareConfig{
		ScriptPath: scriptPath,
		Phase:      PhaseContent,
	}

	middleware, err := NewLuaMiddleware(engine, config)
	if err != nil {
		b.Fatal(err)
	}

	// 创建最终处理器
	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("final")
	}

	handler := middleware.Process(finalHandler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		handler(ctx)
	}
}

// BenchmarkLuaMiddlewareMultiPhase 测试多阶段执行开销
func BenchmarkLuaMiddlewareMultiPhase(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	tmpDir := b.TempDir()

	multi := NewMultiPhaseLuaMiddleware(engine, "bench")

	// rewrite phase
	rewriteScript := filepath.Join(tmpDir, "rewrite.lua")
	err = os.WriteFile(rewriteScript, []byte(`-- simple rewrite`), 0644)
	if err != nil {
		b.Fatal(err)
	}
	err = multi.AddPhase(PhaseRewrite, rewriteScript, 10*time.Second)
	if err != nil {
		b.Fatal(err)
	}

	// content phase
	contentScript := filepath.Join(tmpDir, "content.lua")
	err = os.WriteFile(contentScript, []byte(`ngx.say("content")`), 0644)
	if err != nil {
		b.Fatal(err)
	}
	err = multi.AddPhase(PhaseContent, contentScript, 10*time.Second)
	if err != nil {
		b.Fatal(err)
	}

	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("final")
	}

	handler := multi.Process(finalHandler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		handler(ctx)
	}
}

// BenchmarkLuaMiddlewareNgxExit 测试 ngx.exit 开销
func BenchmarkLuaMiddlewareNgxExit(b *testing.B) {
	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	tmpDir := b.TempDir()
	scriptPath := filepath.Join(tmpDir, "exit.lua")
	err = os.WriteFile(scriptPath, []byte(`ngx.exit(200)`), 0644)
	if err != nil {
		b.Fatal(err)
	}

	config := LuaMiddlewareConfig{
		ScriptPath: scriptPath,
		Phase:      PhaseContent,
	}

	middleware, err := NewLuaMiddleware(engine, config)
	if err != nil {
		b.Fatal(err)
	}

	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("final")
	}

	handler := middleware.Process(finalHandler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := &fasthttp.RequestCtx{}
		handler(ctx)
	}
}

// TestLuaMiddlewarePerformanceOverhead 验证性能要求：开销 < 1ms
func TestLuaMiddlewarePerformanceOverhead(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	engine, err := NewEngine(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "perf.lua")
	err = os.WriteFile(scriptPath, []byte(`ngx.say("performance test")`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	config := LuaMiddlewareConfig{
		ScriptPath: scriptPath,
		Phase:      PhaseContent,
	}

	middleware, err := NewLuaMiddleware(engine, config)
	if err != nil {
		t.Fatal(err)
	}

	finalHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("final")
	}

	handler := middleware.Process(finalHandler)

	// 测量 100 次执行的总时间
	iterations := 100
	start := time.Now()
	for i := 0; i < iterations; i++ {
		ctx := &fasthttp.RequestCtx{}
		handler(ctx)
	}
	totalDuration := time.Since(start)

	// 计算平均开销
	avgOverhead := totalDuration / time.Duration(iterations)

	t.Logf("Average overhead per request: %v", avgOverhead)

	// 验证开销 < 1ms
	if avgOverhead >= 1*time.Millisecond {
		t.Errorf("Lua middleware overhead %v exceeds 1ms threshold", avgOverhead)
	}
}
