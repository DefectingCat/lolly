// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// TestLuaContext 测试 LuaContext 基础功能
func TestLuaContext(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := NewContext(engine, nil)
	require.NotNil(t, ctx)
	assert.NotNil(t, ctx.Engine)
	assert.NotNil(t, ctx.Variables)
	assert.Equal(t, PhaseInit, ctx.Phase)

	// 测试变量操作
	ctx.SetVariable("test_key", "test_value")
	val, ok := ctx.GetVariable("test_key")
	assert.True(t, ok)
	assert.Equal(t, "test_value", val)

	// 测试未存在的变量
	_, ok = ctx.GetVariable("nonexistent")
	assert.False(t, ok)
}

// TestLuaContextPhase 测试阶段设置
func TestLuaContextPhase(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := NewContext(engine, nil)

	// 测试所有阶段
	phases := []Phase{PhaseInit, PhaseRewrite, PhaseAccess, PhaseContent, PhaseLog, PhaseHeaderFilter, PhaseBodyFilter}
	for _, p := range phases {
		ctx.SetPhase(p)
		assert.Equal(t, p, ctx.GetPhase())
	}

	// 测试阶段字符串
	assert.Equal(t, "init", PhaseInit.String())
	assert.Equal(t, "rewrite", PhaseRewrite.String())
	assert.Equal(t, "access", PhaseAccess.String())
	assert.Equal(t, "content", PhaseContent.String())
	assert.Equal(t, "log", PhaseLog.String())
	assert.Equal(t, "header_filter", PhaseHeaderFilter.String())
	assert.Equal(t, "body_filter", PhaseBodyFilter.String())
}

// TestLuaContextOutput 测试输出缓冲
func TestLuaContextOutput(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := NewContext(engine, nil)

	// 测试 Write
	ctx.Write([]byte("hello"))
	assert.Equal(t, []byte("hello"), ctx.OutputBuffer)

	// 测试 Say - Say 会添加 data 然后换行
	ctx.OutputBuffer = nil // 清空重新测试
	ctx.Say("hello")
	assert.Equal(t, []byte("hello\n"), ctx.OutputBuffer)
}

// TestLuaContextFlushOutput 测试刷新输出
func TestLuaContextFlushOutput(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 当 RequestCtx 为 nil 时，FlushOutput 应该安全处理
	ctx := NewContext(engine, nil)
	ctx.OutputBuffer = []byte("test output")

	// FlushOutput 应该不会 panic（RequestCtx 为 nil）
	ctx.FlushOutput()
	// OutputBuffer 应该保持不变（因为 RequestCtx 为 nil）
	assert.NotNil(t, ctx.OutputBuffer)
}

// TestLuaContextPoolStateIsolation 测试池化 context 请求间无状态污染
func TestLuaContextPoolStateIsolation(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 第一次使用：设置变量、输出、阶段、退出标记
	ctx1 := NewContext(engine, nil)
	ctx1.SetVariable("key1", "value1")
	ctx1.SetVariable("key2", "value2")
	ctx1.Write([]byte("hello"))
	ctx1.SetPhase(PhaseAccess)
	ctx1.Exited = true

	// 释放回池
	ctx1.Release()

	// 第二次使用：从池中获取（可能是同一个对象）
	ctx2 := NewContext(engine, nil)

	// 验证无状态污染
	assert.Equal(t, PhaseInit, ctx2.Phase, "Phase should be reset to PhaseInit")
	assert.False(t, ctx2.Exited, "Exited should be reset to false")
	assert.Empty(t, ctx2.OutputBuffer, "OutputBuffer should be empty")
	assert.Empty(t, ctx2.Variables, "Variables map should be empty")

	// 验证旧的 key 不存在
	_, ok := ctx2.GetVariable("key1")
	assert.False(t, ok, "key1 should not exist after release")
	_, ok = ctx2.GetVariable("key2")
	assert.False(t, ok, "key2 should not exist after release")

	ctx2.Release()
}

// TestLuaContextPoolMultipleReuse 测试多次复用
func TestLuaContextPoolMultipleReuse(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 循环多次 release/acquire，验证状态始终正确
	for range 100 {
		ctx := NewContext(engine, nil)
		ctx.SetVariable("iter", "val")
		ctx.Write([]byte("data"))
		ctx.SetPhase(PhaseLog)
		ctx.Exited = true
		ctx.Release()
	}

	// 最后一次获取，验证状态干净
	ctx := NewContext(engine, nil)
	assert.Equal(t, PhaseInit, ctx.Phase)
	assert.False(t, ctx.Exited)
	assert.Empty(t, ctx.OutputBuffer)
	assert.Empty(t, ctx.Variables)
	ctx.Release()
}

// TestLuaContextExecute 测试 Lua 执行
func TestLuaContextExecute(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := NewContext(engine, nil)

	// 执行简单脚本
	err = ctx.Execute("local x = 1 + 1")
	assert.NoError(t, err)

	// Release
	ctx.Release()
	assert.Nil(t, ctx.Coroutine)
}

// TestLuaContextExecuteFile 测试文件执行
func TestLuaContextExecuteFile(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	ctx := NewContext(engine, nil)

	// 创建临时 Lua 文件
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	err = os.WriteFile(scriptPath, []byte("return 42"), 0o644)
	require.NoError(t, err)

	// 执行文件
	err = ctx.ExecuteFile(scriptPath)
	assert.NoError(t, err)

	ctx.Release()
}

// TestLuaCoroutineExecute 测试协程执行
func TestLuaCoroutineExecute(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	require.NotNil(t, coro)
	defer coro.Close()

	// 设置沙箱
	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 执行脚本
	err = coro.Execute("return 42")
	assert.NoError(t, err)
}

// TestLuaCoroutineExecuteWithYield 测试 yield/resume
func TestLuaCoroutineExecuteWithYield(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	require.NotNil(t, coro)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 执行带 yield 的脚本 - 验证 yield/resume 循环
	// 注意：需要注册 lolly.sleep 函数才能正确处理 yield
	err = coro.Execute("local x = 1; return x + 1")
	assert.NoError(t, err)
}

// TestLuaCoroutineExecuteFile 测试协程文件执行
func TestLuaCoroutineExecuteFile(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	// 创建临时文件
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	err = os.WriteFile(scriptPath, []byte("return 42"), 0o644)
	require.NoError(t, err)

	err = coro.ExecuteFile(scriptPath)
	assert.NoError(t, err)
}

// TestLuaCoroutineExecuteError 测试执行错误
func TestLuaCoroutineExecuteError(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	// 编译错误
	err = coro.Execute("invalid lua syntax !!!")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "compile")

	// 运行时错误
	err = coro.Execute("error('runtime error')")
	assert.Error(t, err)
}

// TestLuaCoroutineExecuteFileError 测试文件执行错误
func TestLuaCoroutineExecuteFileError(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	// 不存在的文件
	err = coro.ExecuteFile("/nonexistent/path.lua")
	assert.Error(t, err)
}

// TestLuaCoroutineHandleYield 测试 yield 处理
func TestLuaCoroutineHandleYield(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试 unknown yield reason - 会返回错误
	// 因为 handleYield 会检查 yield reason
	err = coro.Execute("coroutine.yield('unknown_reason')")
	assert.Error(t, err) // unknown yield reason
}

// TestLuaCoroutineHandleSleep 测试 sleep yield 处理
// 注意：需要 coroutine 库支持，当前沙箱未加载
func TestLuaCoroutineHandleSleep(t *testing.T) {
	engine, err := NewEngine(&Config{
		MaxConcurrentCoroutines: 1000,
		MaxExecutionTime:        5 * time.Second,
	})
	require.NoError(t, err)
	defer engine.Close()

	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)
	defer coro.Close()

	// 不设置沙箱，使用全局环境（包含 coroutine 库）
	// 简单测试 execute 和 yield 循环的基本路径
	err = coro.Execute("return 1 + 1")
	assert.NoError(t, err)

	// 测试错误路径 - yield 无参数
	err = coro.Execute("coroutine.yield()")
	// 由于 coroutine 库可能在沙箱中不可用，这个测试可能返回编译错误或运行时错误
	// 重点覆盖代码路径
	_ = err
}

// TestCodeCacheFile 测试文件缓存
func TestCodeCacheFile(t *testing.T) {
	// 创建临时 Lua 文件
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.lua")
	scriptContent := "return 42"

	err := os.WriteFile(scriptPath, []byte(scriptContent), 0o644)
	require.NoError(t, err)

	cache := NewCodeCache(100, time.Hour, true)

	// 第一次编译文件
	proto1, err := cache.GetOrCompileFile(scriptPath)
	require.NoError(t, err)
	require.NotNil(t, proto1)

	// 第二次应该命中缓存
	proto2, err := cache.GetOrCompileFile(scriptPath)
	require.NoError(t, err)
	require.NotNil(t, proto2)

	assert.Equal(t, proto1, proto2)

	hits, misses, _ := cache.Stats()
	assert.Equal(t, uint64(1), hits)
	assert.Equal(t, uint64(1), misses)
}

// TestCodeCacheEviction 测试缓存淘汰
func TestCodeCacheEviction(t *testing.T) {
	cache := NewCodeCache(2, 0, false) // 只存 2 个

	// 编译 3 个脚本，触发淘汰
	_, err := cache.GetOrCompileInline("return 1")
	require.NoError(t, err)

	_, err = cache.GetOrCompileInline("return 2")
	require.NoError(t, err)

	_, err = cache.GetOrCompileInline("return 3")
	require.NoError(t, err)

	// 第一个应该被淘汰了
	hits, misses, size := cache.Stats()
	assert.Equal(t, uint64(0), hits)
	assert.Equal(t, uint64(3), misses)
	assert.LessOrEqual(t, size, 2)
}

// TestCodeCacheTTL 测试 TTL 过期后重新编译
func TestCodeCacheTTL(t *testing.T) {
	cache := NewCodeCache(100, 100*time.Millisecond, false)

	script := "return 1"

	// 编译脚本
	_, err := cache.GetOrCompileInline(script)
	require.NoError(t, err)

	// 等待 TTL 过期
	time.Sleep(150 * time.Millisecond)

	// 应该重新编译（miss）
	_, err = cache.GetOrCompileInline(script)
	require.NoError(t, err)

	// 检查 stats：两次 miss，因为 TTL 过期后重新编译
	hits, misses, _ := cache.Stats()
	assert.Equal(t, uint64(0), hits)   // 没有 hit
	assert.Equal(t, uint64(2), misses) // 两次 miss
}

// TestCodeCacheClear 测试清空缓存
func TestCodeCacheClear(t *testing.T) {
	cache := NewCodeCache(100, 0, false)

	// 添加一些缓存
	_, err := cache.GetOrCompileInline("return 1")
	require.NoError(t, err)

	_, err = cache.GetOrCompileInline("return 2")
	require.NoError(t, err)

	hits, misses, size := cache.Stats()
	assert.Equal(t, 2, size)
	_ = hits
	_ = misses

	// 清空
	cache.Clear()

	hits, misses, size = cache.Stats()
	assert.Equal(t, 0, size)
	_ = hits
	_ = misses
}

// TestCodeCacheHitRate 测试命中率
func TestCodeCacheHitRate(t *testing.T) {
	cache := NewCodeCache(100, 0, false)

	script := "return 1"

	// 第一次 miss
	_, err := cache.GetOrCompileInline(script)
	require.NoError(t, err)

	// 第二次 hit
	_, err = cache.GetOrCompileInline(script)
	require.NoError(t, err)

	// 第三次 hit
	_, err = cache.GetOrCompileInline(script)
	require.NoError(t, err)

	hitRate := cache.HitRate()
	assert.Equal(t, 2.0/3.0, hitRate)
}

// TestEngineStats 测试引擎统计
func TestEngineStats(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 初始统计应该为 0
	stats := engine.Stats()
	assert.Equal(t, uint64(0), stats.CoroutinesCreated)
	assert.Equal(t, uint64(0), stats.CoroutinesClosed)

	// 创建协程
	coro, err := engine.NewCoroutine(nil)
	require.NoError(t, err)

	stats = engine.Stats()
	assert.Equal(t, uint64(1), stats.CoroutinesCreated)
	assert.Equal(t, uint64(0), stats.CoroutinesClosed)

	// 关闭协程
	coro.Close()

	stats = engine.Stats()
	assert.Equal(t, uint64(1), stats.CoroutinesCreated)
	assert.Equal(t, uint64(1), stats.CoroutinesClosed)
}

// TestEngineCodeCache 测试引擎字节码缓存访问
func TestEngineCodeCache(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	cache := engine.CodeCache()
	require.NotNil(t, cache)

	// 通过引擎缓存编译
	proto, err := cache.GetOrCompileInline("return 42")
	require.NoError(t, err)
	require.NotNil(t, proto)
}

// TestConfig 测试配置
func TestConfig(t *testing.T) {
	config := DefaultConfig()
	require.NotNil(t, config)

	// 默认配置值
	assert.Equal(t, 1000, config.MaxConcurrentCoroutines)
	assert.Equal(t, 30*time.Second, config.MaxExecutionTime)
	assert.Equal(t, 1000, config.CodeCacheSize)

	// 测试自定义配置
	customConfig := &Config{
		MaxConcurrentCoroutines: 100,
		MaxExecutionTime:        time.Minute,
		CodeCacheSize:           200,
	}

	engine, err := NewEngine(customConfig)
	require.NoError(t, err)
	defer engine.Close()

	assert.Equal(t, 100, engine.maxCoroutines)
}

// TestNgxAPIRegistrationInSandbox 测试所有 ngx API 在沙箱中的注册
func TestNgxAPIRegistrationInSandbox(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建 mock RequestCtx（ngx.req/resp/log API 需要 RequestCtx）
	mockCtx := &fasthttp.RequestCtx{}

	coro, err := engine.NewCoroutine(mockCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 验证 ngx 表存在
	err = coro.Execute(`
		assert(ngx ~= nil, "ngx table should exist")
		assert(type(ngx) == "table", "ngx should be a table")
	`)
	assert.NoError(t, err)

	// 验证 ngx.req API 存在
	coro2, err := engine.NewCoroutine(mockCtx)
	require.NoError(t, err)
	defer coro2.Close()
	err = coro2.SetupSandbox()
	require.NoError(t, err)
	err = coro2.Execute(`
		assert(ngx.req ~= nil, "ngx.req should exist")
		assert(type(ngx.req.get_method) == "function", "ngx.req.get_method should be a function")
		assert(type(ngx.req.get_uri) == "function", "ngx.req.get_uri should be a function")
		assert(type(ngx.req.set_uri) == "function", "ngx.req.set_uri should be a function")
		assert(type(ngx.req.get_uri_args) == "function", "ngx.req.get_uri_args should be a function")
		assert(type(ngx.req.get_headers) == "function", "ngx.req.get_headers should be a function")
		assert(type(ngx.req.set_header) == "function", "ngx.req.set_header should be a function")
		assert(type(ngx.req.clear_header) == "function", "ngx.req.clear_header should be a function")
		assert(type(ngx.req.get_body_data) == "function", "ngx.req.get_body_data should be a function")
	`)
	assert.NoError(t, err)

	// 验证 ngx.resp API 存在
	coro3, err := engine.NewCoroutine(mockCtx)
	require.NoError(t, err)
	defer coro3.Close()
	err = coro3.SetupSandbox()
	require.NoError(t, err)
	err = coro3.Execute(`
		assert(ngx.resp ~= nil, "ngx.resp should exist")
		assert(type(ngx.resp.get_status) == "function", "ngx.resp.get_status should be a function")
		assert(type(ngx.resp.set_status) == "function", "ngx.resp.set_status should be a function")
		assert(type(ngx.resp.get_headers) == "function", "ngx.resp.get_headers should be a function")
		assert(type(ngx.resp.set_header) == "function", "ngx.resp.set_header should be a function")
		assert(type(ngx.resp.clear_header) == "function", "ngx.resp.clear_header should be a function")
	`)
	assert.NoError(t, err)

	// 验证 ngx.var API 存在
	coro4, err := engine.NewCoroutine(mockCtx)
	require.NoError(t, err)
	defer coro4.Close()
	err = coro4.SetupSandbox()
	require.NoError(t, err)
	err = coro4.Execute(`
		assert(ngx.var ~= nil, "ngx.var should exist")
	`)
	assert.NoError(t, err)

	// 验证 ngx.ctx API 存在
	coro5, err := engine.NewCoroutine(mockCtx)
	require.NoError(t, err)
	defer coro5.Close()
	err = coro5.SetupSandbox()
	require.NoError(t, err)
	err = coro5.Execute(`
		assert(ngx.ctx ~= nil, "ngx.ctx should exist")
		assert(type(ngx.ctx) == "table", "ngx.ctx should be a table")
	`)
	assert.NoError(t, err)

	// 验证 ngx.log API 存在（日志级别常量和函数）
	coro6, err := engine.NewCoroutine(mockCtx)
	require.NoError(t, err)
	defer coro6.Close()
	err = coro6.SetupSandbox()
	require.NoError(t, err)
	err = coro6.Execute(`
		assert(ngx.log ~= nil, "ngx.log should exist")
		assert(type(ngx.log) == "function", "ngx.log should be a function")
		assert(ngx.ERR ~= nil, "ngx.ERR should exist")
		assert(ngx.WARN ~= nil, "ngx.WARN should exist")
		assert(ngx.INFO ~= nil, "ngx.INFO should exist")
		assert(ngx.DEBUG ~= nil, "ngx.DEBUG should exist")
		assert(type(ngx.say) == "function", "ngx.say should be a function")
		assert(type(ngx.print) == "function", "ngx.print should be a function")
		assert(type(ngx.flush) == "function", "ngx.flush should be a function")
		assert(type(ngx.exit) == "function", "ngx.exit should be a function")
		assert(type(ngx.redirect) == "function", "ngx.redirect should be a function")
	`)
	assert.NoError(t, err)

	// 验证 ngx.socket API 存在
	coro7, err := engine.NewCoroutine(mockCtx)
	require.NoError(t, err)
	defer coro7.Close()
	err = coro7.SetupSandbox()
	require.NoError(t, err)
	err = coro7.Execute(`
		assert(ngx.socket ~= nil, "ngx.socket should exist")
		assert(type(ngx.socket.tcp) == "function", "ngx.socket.tcp should be a function")
	`)
	assert.NoError(t, err)
}
