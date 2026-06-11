// Package proxy 提供低覆盖率函数的补充测试。
//
// 该文件专注于提升以下函数的测试覆盖率：
//   - proxyDebugLog (0%)
//   - ServeHTTP (47.3%)
//   - selectTarget (46.7%)
//   - backgroundRefresh (41.9%)
//   - selectByLua (39.1%)
//   - WebSocket (15.4%)
//   - dialTarget (46.7%)
//   - DNS 相关函数 (0%)
//
// 作者：xfy
package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/lua"
	"rua.plus/lolly/internal/resolver"
	"rua.plus/lolly/internal/testutil"
)

// TestProxyDebugLog 测试 proxyDebugLog 不会 panic，覆盖各类型分支。
func TestProxyDebugLog(t *testing.T) {
	t.Run("各种 kv 类型", func(t *testing.T) {
		proxyDebugLog("测试消息",
			"str_key", "字符串值",
			"int_key", 42,
			"bool_key", true,
			"iface_key", []string{"a", "b"},
		)
	})

	t.Run("非字符串 key 跳过", func(t *testing.T) {
		proxyDebugLog("非字符串key",
			123, "value",
			"valid_key", "value",
		)
	})

	t.Run("奇数 kv 参数", func(t *testing.T) {
		proxyDebugLog("奇数参数",
			"key1", "val1",
			"key2",
		)
	})

	t.Run("空消息", func(t *testing.T) {
		proxyDebugLog("")
	})

	t.Run("空 kv", func(t *testing.T) {
		proxyDebugLog("无参数")
	})
}

// TestServeHTTP_WithRealBackend 测试使用真实后端的 ServeHTTP 完整流程。
func TestServeHTTP_WithRealBackend(t *testing.T) {
	backend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(200)
			ctx.SetBodyString("backend response")
			ctx.Response.Header.Set("X-Backend", "true")
		},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() { _ = backend.Serve(ln) }()
	time.Sleep(20 * time.Millisecond)

	backendAddr := "http://" + ln.Addr().String()

	t.Run("GET 请求转发", func(t *testing.T) {
		cfg := testutil.NewTestProxyConfig("/")
		targets := []*loadbalance.Target{{URL: backendAddr}}
		targets[0].Healthy.Store(true)

		p, err := NewProxy(cfg, targets, nil, nil)
		require.NoError(t, err)

		ctx := testutil.NewRequestCtx("GET", "/test")
		p.ServeHTTP(ctx)

		assert.Equal(t, 200, ctx.Response.StatusCode())
		assert.Equal(t, "backend response", string(ctx.Response.Body()))
	})

	t.Run("POST 请求转发", func(t *testing.T) {
		cfg := testutil.NewTestProxyConfig("/")
		targets := []*loadbalance.Target{{URL: backendAddr}}
		targets[0].Healthy.Store(true)

		p, err := NewProxy(cfg, targets, nil, nil)
		require.NoError(t, err)

		ctx := testutil.NewRequestCtxWithBody("POST", "/submit", "request body")
		p.ServeHTTP(ctx)

		assert.Equal(t, 200, ctx.Response.StatusCode())
	})

	t.Run("PUT 请求转发", func(t *testing.T) {
		cfg := testutil.NewTestProxyConfig("/")
		targets := []*loadbalance.Target{{URL: backendAddr}}
		targets[0].Healthy.Store(true)

		p, err := NewProxy(cfg, targets, nil, nil)
		require.NoError(t, err)

		ctx := testutil.NewRequestCtxWithBody("PUT", "/resource", "put body")
		p.ServeHTTP(ctx)

		assert.Equal(t, 200, ctx.Response.StatusCode())
	})
}

// TestServeHTTP_ConnectionRefused 测试连接被拒绝时的错误处理。
func TestServeHTTP_ConnectionRefused(t *testing.T) {
	t.Run("返回 502 Bad Gateway", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/",
			LoadBalance: "round_robin",
			Timeout:     config.ProxyTimeout{Connect: 100 * time.Millisecond, Read: 100 * time.Millisecond, Write: 100 * time.Millisecond},
		}

		targets := []*loadbalance.Target{{URL: "http://127.0.0.1:1"}}
		targets[0].Healthy.Store(true)

		p, err := NewProxy(cfg, targets, nil, nil)
		require.NoError(t, err)

		ctx := testutil.NewRequestCtx("GET", "/test")
		p.ServeHTTP(ctx)

		assert.True(t, ctx.Response.StatusCode() == 502 || ctx.Response.StatusCode() == 504,
			"expected 502 or 504, got %d", ctx.Response.StatusCode())
	})
}

// TestServeHTTP_Timeout 测试请求超时场景。
func TestServeHTTP_Timeout(t *testing.T) {
	// 创建一个延迟很大的后端
	backend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			time.Sleep(5 * time.Second)
			ctx.SetStatusCode(200)
		},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() { _ = backend.Serve(ln) }()
	time.Sleep(20 * time.Millisecond)

	backendAddr := "http://" + ln.Addr().String()

	cfg := &config.ProxyConfig{
		Path:        "/",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 50 * time.Millisecond, Read: 50 * time.Millisecond, Write: 50 * time.Millisecond},
	}

	targets := []*loadbalance.Target{{URL: backendAddr}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/slow")
	p.ServeHTTP(ctx)

	assert.True(t, ctx.Response.StatusCode() == 504 || ctx.Response.StatusCode() == 502,
		"expected timeout error (504 or 502), got %d", ctx.Response.StatusCode())
}

// TestServeHTTP_NextUpstreamFailover 测试故障转移到下一个后端。
func TestServeHTTP_NextUpstreamFailover(t *testing.T) {
	// 创建健康的后端
	healthyBackend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(200)
			ctx.SetBodyString("healthy backend")
		},
	}

	healthyLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer healthyLn.Close()

	go func() { _ = healthyBackend.Serve(healthyLn) }()
	time.Sleep(20 * time.Millisecond)

	healthyAddr := "http://" + healthyLn.Addr().String()

	cfg := &config.ProxyConfig{
		Path:        "/",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 100 * time.Millisecond, Read: 100 * time.Millisecond, Write: 100 * time.Millisecond},
		NextUpstream: config.NextUpstreamConfig{
			Tries:     3,
			HTTPCodes: []int{502, 503, 504},
		},
	}

	// 第一个目标不可达，第二个健康
	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:1"},
		{URL: healthyAddr},
	}
	targets[0].Healthy.Store(true)
	targets[1].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/test")
	p.ServeHTTP(ctx)

	assert.Equal(t, 200, ctx.Response.StatusCode())
	assert.Equal(t, "healthy backend", string(ctx.Response.Body()))
}

// TestServeHTTP_RetryOnHTTPError 测试后端返回错误状态码时重试。
func TestServeHTTP_RetryOnHTTPError(t *testing.T) {
	// 坏后端：返回 502
	badBackend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(502)
			ctx.SetBodyString("bad gateway")
		},
	}
	badLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer badLn.Close()
	go func() { _ = badBackend.Serve(badLn) }()

	// 好后端：返回 200
	goodBackend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(200)
			ctx.SetBodyString("ok")
		},
	}
	goodLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer goodLn.Close()
	go func() { _ = goodBackend.Serve(goodLn) }()

	time.Sleep(20 * time.Millisecond)

	cfg := &config.ProxyConfig{
		Path:        "/",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 1 * time.Second, Read: 1 * time.Second, Write: 1 * time.Second},
		NextUpstream: config.NextUpstreamConfig{
			Tries:     3,
			HTTPCodes: []int{502},
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://" + badLn.Addr().String()},
		{URL: "http://" + goodLn.Addr().String()},
	}
	targets[0].Healthy.Store(true)
	targets[1].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/test")
	p.ServeHTTP(ctx)

	assert.Equal(t, 200, ctx.Response.StatusCode())
}

// TestServeHTTP_XAccelRedirect 测试 X-Accel-Redirect 内部重定向。
// /internal/ 和 /admin/ 前缀的路径不做内部重定向，直接返回原始响应。
func TestServeHTTP_XAccelRedirect(t *testing.T) {
	t.Run("/internal/ 前缀不做重定向", func(t *testing.T) {
		backend := &fasthttp.Server{
			Handler: func(ctx *fasthttp.RequestCtx) {
				ctx.Response.Header.Set("X-Accel-Redirect", "/internal/secret")
				ctx.SetStatusCode(200)
				ctx.SetBodyString("raw response")
			},
		}

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()

		go func() { _ = backend.Serve(ln) }()
		time.Sleep(20 * time.Millisecond)

		cfg := testutil.NewTestProxyConfig("/")
		targets := []*loadbalance.Target{{URL: "http://" + ln.Addr().String()}}
		targets[0].Healthy.Store(true)

		p, err := NewProxy(cfg, targets, nil, nil)
		require.NoError(t, err)

		ctx := testutil.NewRequestCtx("GET", "/api/redirect")
		p.ServeHTTP(ctx)

		assert.Equal(t, 200, ctx.Response.StatusCode())
		assert.Equal(t, "raw response", string(ctx.Response.Body()))
	})

	t.Run("非 /internal/ 路径做内部重定向", func(t *testing.T) {
		backend := &fasthttp.Server{
			Handler: func(ctx *fasthttp.RequestCtx) {
				ctx.Response.Header.Set("X-Accel-Redirect", "/other/path")
				ctx.SetStatusCode(200)
			},
		}

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()

		go func() { _ = backend.Serve(ln) }()
		time.Sleep(20 * time.Millisecond)

		cfg := testutil.NewTestProxyConfig("/")
		targets := []*loadbalance.Target{{URL: "http://" + ln.Addr().String()}}
		targets[0].Healthy.Store(true)

		p, err := NewProxy(cfg, targets, nil, nil)
		require.NoError(t, err)

		ctx := testutil.NewRequestCtx("GET", "/api/redirect")
		p.ServeHTTP(ctx)

		assert.Equal(t, "/other/path", string(ctx.Request.URI().Path()))
	})
}

// TestServeHTTP_ProxyURI 测试 ProxyURI 路径替换。
func TestServeHTTP_ProxyURI(t *testing.T) {
	var receivedPath string

	backend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			receivedPath = string(ctx.Path())
			ctx.SetStatusCode(200)
			ctx.SetBodyString("ok")
		},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() { _ = backend.Serve(ln) }()
	time.Sleep(20 * time.Millisecond)

	backendAddr := "http://" + ln.Addr().String()

	cfg := testutil.NewTestProxyConfig("/")
	targets := []*loadbalance.Target{{URL: backendAddr, ProxyURI: "/v2/api"}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/v1/api")
	p.ServeHTTP(ctx)

	assert.Equal(t, 200, ctx.Response.StatusCode())
	assert.Contains(t, receivedPath, "/v2/api")
}

// TestServeHTTP_SuspiciousPath 测试危险字符路径被拒绝。
func TestServeHTTP_SuspiciousPath(t *testing.T) {
	cfg := testutil.NewTestProxyConfig("/")
	targets := []*loadbalance.Target{{URL: "http://localhost:8080", ProxyURI: "/test@path"}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/test")
	p.ServeHTTP(ctx)

	assert.Equal(t, 502, ctx.Response.StatusCode())
}

// TestSelectTarget_Random 测试随机负载均衡算法。
func TestSelectTarget_Random(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "random",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}

	targets := []*loadbalance.Target{
		{URL: "http://backend1:8080"},
		{URL: "http://backend2:8080"},
		{URL: "http://backend3:8080"},
	}
	for _, target := range targets {
		target.Healthy.Store(true)
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	selected := p.selectTarget(ctx)

	require.NotNil(t, selected)
	assert.Contains(t, []string{
		"http://backend1:8080",
		"http://backend2:8080",
		"http://backend3:8080",
	}, selected.URL)
}

// TestSelectTarget_LuaSuccess 测试 Lua balancer 成功选择目标。
func TestSelectTarget_LuaSuccess(t *testing.T) {
	// 创建 Lua 脚本，选择第一个目标
	tmpFile, err := os.CreateTemp("", "balancer_*.lua")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	luaScript := `
local balancer = require("ngx.balancer")
balancer.set_current_peer(1)
`
	_, err = tmpFile.WriteString(luaScript)
	require.NoError(t, err)
	tmpFile.Close()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		BalancerByLua: config.BalancerByLuaConfig{
			Enabled: true,
			Script:  tmpFile.Name(),
		},
		Timeout: config.ProxyTimeout{Connect: 5 * time.Second},
	}

	targets := []*loadbalance.Target{
		{URL: "http://backend1:8080"},
		{URL: "http://backend2:8080"},
	}
	for _, target := range targets {
		target.Healthy.Store(true)
	}

	luaEngine, err := lua.NewEngine(nil)
	require.NoError(t, err)

	p, err := NewProxy(cfg, targets, nil, luaEngine)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	selected := p.selectTarget(ctx)

	require.NotNil(t, selected)
	assert.Equal(t, "http://backend1:8080", selected.URL)
}

// TestSelectTarget_LuaFallback 测试 Lua balancer 失败时回退到 fallback。
func TestSelectTarget_LuaFallback(t *testing.T) {
	// 创建一个不调用 set_current_peer 的脚本
	tmpFile, err := os.CreateTemp("", "balancer_noop_*.lua")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	luaScript := `-- 不调用 set_current_peer`
	_, err = tmpFile.WriteString(luaScript)
	require.NoError(t, err)
	tmpFile.Close()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		BalancerByLua: config.BalancerByLuaConfig{
			Enabled:  true,
			Script:   tmpFile.Name(),
			Fallback: "round_robin",
		},
		Timeout: config.ProxyTimeout{Connect: 5 * time.Second},
	}

	targets := []*loadbalance.Target{
		{URL: "http://backend1:8080"},
		{URL: "http://backend2:8080"},
	}
	for _, target := range targets {
		target.Healthy.Store(true)
	}

	luaEngine, err := lua.NewEngine(nil)
	require.NoError(t, err)

	p, err := NewProxy(cfg, targets, nil, luaEngine)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	selected := p.selectTarget(ctx)

	require.NotNil(t, selected)
}

// TestSelectByLua_ValidScript 测试有效的 Lua balancer 脚本。
// 注意：lua.NewEngine(nil) 不初始化 ngx 全局表，selectByLua 会返回错误，
// 但 selectTarget 会自动回退到 fallback 算法。这里直接测试 selectTarget。
func TestSelectByLua_ValidScript(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "lua_valid_*.lua")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	luaScript := `
local balancer = require("ngx.balancer")
balancer.set_current_peer(2)
`
	_, err = tmpFile.WriteString(luaScript)
	require.NoError(t, err)
	tmpFile.Close()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		BalancerByLua: config.BalancerByLuaConfig{
			Enabled:  true,
			Script:   tmpFile.Name(),
			Fallback: "round_robin",
		},
		Timeout: config.ProxyTimeout{Connect: 5 * time.Second},
	}

	targets := []*loadbalance.Target{
		{URL: "http://backend1:8080"},
		{URL: "http://backend2:8080"},
		{URL: "http://backend3:8080"},
	}
	for _, target := range targets {
		target.Healthy.Store(true)
	}

	luaEngine, err := lua.NewEngine(nil)
	require.NoError(t, err)

	p, err := NewProxy(cfg, targets, nil, luaEngine)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/api/test")

	// selectTarget 会因 Lua 错误自动回退到 fallback，仍然返回有效目标
	selected := p.selectTarget(ctx)
	require.NotNil(t, selected)
}

// TestSelectByLua_ScriptNotSelecting 测试 Lua 引擎未初始化 ngx 表时返回错误。
func TestSelectByLua_ScriptNotSelecting(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "lua_nope_*.lua")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	luaScript := `-- do nothing`
	_, err = tmpFile.WriteString(luaScript)
	require.NoError(t, err)
	tmpFile.Close()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		BalancerByLua: config.BalancerByLuaConfig{
			Enabled:  true,
			Script:   tmpFile.Name(),
			Fallback: "round_robin",
		},
		Timeout: config.ProxyTimeout{Connect: 5 * time.Second},
	}

	targets := []*loadbalance.Target{
		{URL: "http://backend1:8080"},
	}
	targets[0].Healthy.Store(true)

	luaEngine, err := lua.NewEngine(nil)
	require.NoError(t, err)

	p, err := NewProxy(cfg, targets, nil, luaEngine)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/api/test")

	// lua.NewEngine(nil) 未初始化 ngx 全局表，selectByLua 会返回错误
	_, err = p.selectByLua(ctx, targets)
	require.Error(t, err)
}

// TestBackgroundRefresh_WithCacheEntry 测试有缓存条目时的后台刷新。
func TestBackgroundRefresh_WithCacheEntry(t *testing.T) {
	backend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(200)
			ctx.SetBodyString("refreshed content")
			ctx.Response.Header.Set("Content-Type", "text/plain")
			ctx.Response.Header.Set("ETag", "\"new-etag\"")
		},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() { _ = backend.Serve(ln) }()
	time.Sleep(20 * time.Millisecond)

	backendAddr := "http://" + ln.Addr().String()

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled:    true,
			MaxAge:     10 * time.Second,
			Revalidate: true,
		},
	}

	targets := []*loadbalance.Target{{URL: backendAddr}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	hashKey, origKey := p.buildCacheKeyHash(ctx)

	p.cache.Set(hashKey, origKey, []byte("old content"), map[string]string{
		"Content-Type":  "text/plain",
		"Last-Modified": "Mon, 01 Jan 2024 00:00:00 GMT",
		"ETag":          "\"old-etag\"",
	}, 200, 10*time.Second)

	// backgroundRefresh 期望请求 URI 包含完整目标 URL
	reqCopy := fasthttp.AcquireRequest()
	ctx.Request.CopyTo(reqCopy)
	reqCopy.SetRequestURI(backendAddr + "/api/test")

	p.backgroundRefresh(reqCopy, targets[0], hashKey, origKey)
	fasthttp.ReleaseRequest(reqCopy)

	entry, ok, _ := p.cache.Get(hashKey, origKey)
	require.True(t, ok)
	assert.Equal(t, "refreshed content", string(entry.Data))
}

// TestBackgroundRefresh_RequestError 测试后台刷新请求失败时释放缓存锁。
func TestBackgroundRefresh_RequestError(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{{URL: "http://127.0.0.1:1"}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	hashKey := uint64(54321)

	p.backgroundRefresh(&ctx.Request, targets[0], hashKey, "GET:/api/test")
}

// TestDialTarget_Success 测试成功建立 TCP 连接。
func TestDialTarget_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			_ = conn.Close()
		}
	}()

	conn, err := dialTarget("http://"+ln.Addr().String(), 1*time.Second)
	require.NoError(t, err)
	require.NotNil(t, conn)
	_ = conn.Close()
}

// TestDialTarget_Timeout 测试连接超时。
func TestDialTarget_Timeout(t *testing.T) {
	_, err := dialTarget("http://10.255.255.1:9999", 50*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect")
}

// TestWebSocket_UpgradeRejected 测试后端拒绝 WebSocket 升级。
// 使用真实 TCP 连接测试 readWebSocketUpgradeResponse 返回非 101 状态码。
func TestWebSocket_UpgradeRejected(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		reader := bufio.NewReader(conn)
		_, _ = http.ReadRequest(reader)
		_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		_ = conn.Close()
	}()

	time.Sleep(20 * time.Millisecond)

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	// 发送一个请求让服务端触发
	_, _ = conn.Write([]byte("GET /ws HTTP/1.1\r\nHost: localhost\r\n\r\n"))

	resp, _, err := readWebSocketUpgradeResponse(conn, 1*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

// TestWebSocket_BackendSuccess 测试 WebSocket 函数因 Hijack 失败而返回错误。
// testutil 创建的 ctx 不支持 Hijack，验证此错误路径。
func TestWebSocket_BackendSuccess(t *testing.T) {
	ctx := testutil.NewRequestCtxWithHeader("GET", "/ws", map[string]string{
		"Upgrade":               "websocket",
		"Connection":            "Upgrade",
		"Sec-WebSocket-Key":     "dGhlIHNhbXBsZSBub25jZQ==",
		"Sec-WebSocket-Version": "13",
	})

	target := &loadbalance.Target{URL: "http://127.0.0.1:1"}
	target.Healthy.Store(true)

	err := WebSocket(ctx, target, 2*time.Second, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hijack")
}

// TestBuildWebSocketUpgradeRequest_HeaderConfig 测试 Headers 配置控制。
func TestBuildWebSocketUpgradeRequest_HeaderConfig(t *testing.T) {
	t.Run("禁用 X-Forwarded-Host", func(t *testing.T) {
		falseVal := false
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/ws")
		ctx.Request.Header.SetHost("client.example.com")

		result := buildWebSocketUpgradeRequest(ctx, "backend:8080", &config.ProxyHeaders{
			SetForwardedHost: &falseVal,
		})

		assert.NotContains(t, result, "X-Forwarded-Host")
	})

	t.Run("禁用 X-Forwarded-Proto", func(t *testing.T) {
		falseVal := false
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/ws")
		ctx.Request.Header.SetHost("client.example.com")

		result := buildWebSocketUpgradeRequest(ctx, "backend:8080", &config.ProxyHeaders{
			SetForwardedProto: &falseVal,
		})

		assert.NotContains(t, result, "X-Forwarded-Proto")
	})

	t.Run("启用所有头", func(t *testing.T) {
		trueVal := true
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/ws")
		ctx.Request.Header.SetHost("client.example.com")

		result := buildWebSocketUpgradeRequest(ctx, "backend:8080", &config.ProxyHeaders{
			SetForwardedHost:  &trueVal,
			SetForwardedProto: &trueVal,
		})

		assert.Contains(t, result, "X-Forwarded-Host: client.example.com")
		assert.Contains(t, result, "X-Forwarded-Proto: http")
	})
}

// TestDNS_StartIdempotent 测试 Start 方法是幂等的。
func TestDNS_StartIdempotent(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	mr := &mockResolver{}
	p.SetResolver(mr)

	err = p.Start()
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)

	assert.Equal(t, 1, mr.startCalls)
}

// TestDNS_RefreshDNS_LookupError 测试 DNS 刷新时查找失败。
func TestDNS_RefreshDNS_LookupError(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	mr := &mockResolver{
		lookupError: errors.New("DNS lookup failed"),
	}
	p.SetResolver(mr)

	p.refreshDNS()
}

// TestDNS_RefreshDNS_NoResolver 测试没有解析器时不执行刷新。
func TestDNS_RefreshDNS_NoResolver(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	p.refreshDNS()
}

// TestDNS_UpdateHostClientAddr 测试更新 HostClient 地址。
func TestDNS_UpdateHostClientAddr(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{{URL: "http://example.com:8080"}}

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	p.updateHostClientAddr(targets[0], "10.0.0.1")

	client := p.getClient("http://example.com:8080")
	require.NotNil(t, client)
	assert.Equal(t, "10.0.0.1:8080", client.Addr)
}

// TestDNS_UpdateHostClientAddr_DefaultPort 测试无端口时使用默认端口。
func TestDNS_UpdateHostClientAddr_DefaultPort(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		ip       string
		expected string
	}{
		{"HTTP 默认端口", "http://example.com", "10.0.0.1", "10.0.0.1:80"},
		{"HTTPS 默认端口", "https://example.com", "10.0.0.2", "10.0.0.2:443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ProxyConfig{
				Path:        "/api",
				LoadBalance: "round_robin",
				Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
			}
			targets := []*loadbalance.Target{{URL: tt.url}}

			p, err := NewProxy(cfg, targets, nil, nil)
			require.NoError(t, err)

			p.updateHostClientAddr(targets[0], tt.ip)

			client := p.getClient(tt.url)
			require.NotNil(t, client)
			assert.Equal(t, tt.expected, client.Addr)
		})
	}
}

// TestDNS_GetResolverTTL 测试 TTL 获取。
func TestDNS_GetResolverTTL(t *testing.T) {
	t.Run("无解析器返回 0", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "round_robin",
			Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		}
		targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}

		p, err := NewProxy(cfg, targets, nil, nil)
		require.NoError(t, err)

		ttl := p.getResolverTTL()
		assert.Equal(t, time.Duration(0), ttl)
	})

	t.Run("有解析器返回 30s", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "round_robin",
			Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		}
		targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}

		p, err := NewProxy(cfg, targets, nil, nil)
		require.NoError(t, err)

		p.SetResolver(&mockResolver{})

		ttl := p.getResolverTTL()
		assert.Equal(t, 30*time.Second, ttl)
	})
}

// TestDNS_RefreshDNS_Success 测试 DNS 刷新成功。
func TestDNS_RefreshDNS_Success(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{{URL: "http://example.com:8080"}}

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	mr := &mockResolver{
		lookupResults: map[string][]string{
			"example.com": {"10.0.0.1"},
		},
	}
	p.SetResolver(mr)

	p.refreshDNS()

	client := p.getClient("http://example.com:8080")
	require.NotNil(t, client)
	assert.Equal(t, "10.0.0.1:8080", client.Addr)
}

// TestDNS_StartResolverFails 测试解析器启动失败。
func TestDNS_StartResolverFails(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	mr := &mockResolver{
		startErr: errors.New("resolver start failed"),
	}
	p.SetResolver(mr)

	err = p.Start()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolver start failed")
}

// mockTargetResolver 实现 resolver.Resolver 接口的简化 mock。
type mockTargetResolver struct {
	lookupFunc func(ctx context.Context, host string) ([]string, error)
}

func (m *mockTargetResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	return m.lookupFunc(ctx, host)
}

func (m *mockTargetResolver) LookupHostWithCache(ctx context.Context, host string) ([]string, error) {
	return m.lookupFunc(ctx, host)
}

func (m *mockTargetResolver) Refresh(host string) error { return nil }

func (m *mockTargetResolver) Start() error { return nil }

func (m *mockTargetResolver) Stop() error { return nil }

func (m *mockTargetResolver) Stats() resolver.Stats { return resolver.Stats{} }

// TestServeHTTP_CacheStaleWhileRevalidate 测试缓存过期时的后台刷新。
func TestServeHTTP_CacheStaleWhileRevalidate(t *testing.T) {
	backend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(200)
			ctx.SetBodyString("fresh content")
			ctx.Response.Header.Set("Content-Type", "text/plain")
		},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() { _ = backend.Serve(ln) }()
	time.Sleep(20 * time.Millisecond)

	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{{URL: "http://" + ln.Addr().String()}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/api/cached")
	hashKey, origKey := p.buildCacheKeyHash(ctx)

	headers := map[string]string{"Content-Type": "text/plain"}
	p.cache.Set(hashKey, origKey, []byte("stale content"), headers, 200, -1*time.Second)

	p.ServeHTTP(ctx)

	assert.Equal(t, 200, ctx.Response.StatusCode())
	body := string(ctx.Response.Body())
	assert.True(t, body == "stale content" || body == "fresh content",
		"expected stale or fresh content, got: %s", body)
}

// TestServeHTTP_AllUpstreamsFailed 测试所有上游都失败时返回 502。
func TestServeHTTP_AllUpstreamsFailed(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 50 * time.Millisecond, Read: 50 * time.Millisecond, Write: 50 * time.Millisecond},
		NextUpstream: config.NextUpstreamConfig{
			Tries:     2,
			HTTPCodes: []int{502},
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:1"},
		{URL: "http://127.0.0.1:2"},
	}
	for _, target := range targets {
		target.Healthy.Store(true)
	}

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/test")
	p.ServeHTTP(ctx)

	assert.True(t, ctx.Response.StatusCode() == 502 || ctx.Response.StatusCode() == 504,
		"expected 502 or 504, got %d", ctx.Response.StatusCode())
}

// TestWriteUpgradeResponse_WriteError 测试写入升级响应失败。
func TestWriteUpgradeResponse_WriteError(t *testing.T) {
	conn1, conn2 := net.Pipe()
	_ = conn2.Close()

	resp := &http.Response{
		ProtoMajor: 1,
		ProtoMinor: 1,
		Status:     "101 Switching Protocols",
		StatusCode: 101,
		Header: http.Header{
			"Upgrade": []string{"websocket"},
		},
	}

	err := writeUpgradeResponse(conn1, resp)
	assert.Error(t, err)
	_ = conn1.Close()
}

// TestIsConnectionClosedError_ClosedConnString 测试包含 "use of closed" 的 net.Error。
func TestIsConnectionClosedError_ClosedConnString(t *testing.T) {
	// 构造一个同时是 net.Error 且包含关闭字符串的错误
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)

	_ = conn.Close()
	_ = ln.Close()

	_, readErr := conn.Read(make([]byte, 1))
	require.Error(t, readErr)
	assert.True(t, isConnectionClosedError(readErr))
}

// TestCopyData_ReadError 测试读取端错误处理。
func TestCopyData_ReadError(t *testing.T) {
	src1, src2 := net.Pipe()
	dst1, dst2 := net.Pipe()

	_ = src1.Close()
	_ = src2.Close()

	bridge := &WebSocketBridge{}

	errCh := make(chan error, 1)
	go func() {
		errCh <- bridge.copyData(dst1, src1, "test-direction")
	}()

	_ = dst1.Close()
	_ = dst2.Close()

	select {
	case err := <-errCh:
		_ = err
	case <-time.After(1 * time.Second):
		t.Error("copyData did not complete in time")
	}
}

// TestServeHTTP_CacheStoreAndHit 测试缓存存储和命中完整流程。
func TestServeHTTP_CacheStoreAndHit(t *testing.T) {
	var requestCount atomic.Int32

	backend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			count := requestCount.Add(1)
			ctx.SetStatusCode(200)
			ctx.SetBodyString(fmt.Sprintf("response-%d", count))
			ctx.Response.Header.Set("Content-Type", "text/plain")
		},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() { _ = backend.Serve(ln) }()
	time.Sleep(20 * time.Millisecond)

	backendAddr := "http://" + ln.Addr().String()

	cfg := &config.ProxyConfig{
		Path:        "/",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 1 * time.Second, Read: 1 * time.Second, Write: 1 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{{URL: backendAddr}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx1 := testutil.NewRequestCtx("GET", "/cacheable")
	p.ServeHTTP(ctx1)
	assert.Equal(t, 200, ctx1.Response.StatusCode())
	assert.Equal(t, "response-1", string(ctx1.Response.Body()))

	ctx2 := testutil.NewRequestCtx("GET", "/cacheable")
	p.ServeHTTP(ctx2)
	assert.Equal(t, 200, ctx2.Response.StatusCode())

	body := string(ctx2.Response.Body())
	assert.True(t, body == "response-1" || body == "response-2",
		"expected cached or fresh response, got: %s", body)
}

// TestServeHTTP_ConnectionClosed 测试连接关闭错误。
func TestServeHTTP_ConnectionClosed(t *testing.T) {
	backend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(200)
			ctx.SetBodyString("ok")
		},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = backend.Serve(ln) }()
	time.Sleep(20 * time.Millisecond)

	backendAddr := "http://" + ln.Addr().String()

	cfg := &config.ProxyConfig{
		Path:        "/",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 1 * time.Second, Read: 1 * time.Second, Write: 1 * time.Second},
		NextUpstream: config.NextUpstreamConfig{
			Tries:     2,
			HTTPCodes: []int{502},
		},
	}

	targets := []*loadbalance.Target{
		{URL: backendAddr},
	}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ln.Close()

	ctx := testutil.NewRequestCtx("GET", "/test")
	p.ServeHTTP(ctx)

	assert.True(t, ctx.Response.StatusCode() == 502 || ctx.Response.StatusCode() == 504)
}

// TestReadWebSocketUpgradeResponse_ReadError 测试读取升级响应失败。
func TestReadWebSocketUpgradeResponse_ReadError(t *testing.T) {
	conn1, conn2 := net.Pipe()
	_ = conn2.Close()

	_, _, err := readWebSocketUpgradeResponse(conn1, 100*time.Millisecond)
	assert.Error(t, err)
	_ = conn1.Close()
}

// TestIsConnectionClosedError_NilNetError 测试普通错误不包含关闭字符串时不被识别。
func TestIsConnectionClosedError_NilNetError(t *testing.T) {
	err := errors.New("some random error")
	assert.False(t, isConnectionClosedError(err))
}

// TestBridge_NonClosedErrors 测试桥接返回非关闭错误。
func TestBridge_NonClosedErrors(t *testing.T) {
	errConn1, errConn2 := net.Pipe()
	normalConn1, normalConn2 := net.Pipe()
	defer func() {
		_ = normalConn1.Close()
		_ = normalConn2.Close()
	}()

	bridge := NewWebSocketBridge(errConn1, normalConn1)

	_ = errConn2.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- bridge.Bridge()
	}()

	time.Sleep(50 * time.Millisecond)
	_ = normalConn2.Close()

	select {
	case err := <-errCh:
		_ = err
	case <-time.After(2 * time.Second):
		t.Error("Bridge did not complete in time")
	}
}

// TestServeHTTP_IgnoresEmptyTargetURL 测试跳过空 URL 的目标。
func TestServeHTTP_IgnoresEmptyTargetURL(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 100 * time.Millisecond},
	}

	targets := []*loadbalance.Target{
		{URL: ""},
		{URL: "http://127.0.0.1:1"},
	}
	targets[1].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	p.ServeHTTP(ctx)

	assert.True(t, ctx.Response.StatusCode() == 502 || ctx.Response.StatusCode() == 504)
}

// TestServeHTTP_WithQueryParams 测试带查询参数的请求。
func TestServeHTTP_WithQueryParams(t *testing.T) {
	var receivedURI string

	backend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			receivedURI = string(ctx.RequestURI())
			ctx.SetStatusCode(200)
			ctx.SetBodyString("ok")
		},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() { _ = backend.Serve(ln) }()
	time.Sleep(20 * time.Millisecond)

	backendAddr := "http://" + ln.Addr().String()

	cfg := testutil.NewTestProxyConfig("/")
	targets := []*loadbalance.Target{{URL: backendAddr}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtx("GET", "/search?q=test&limit=10")
	p.ServeHTTP(ctx)

	assert.Equal(t, 200, ctx.Response.StatusCode())
	assert.Contains(t, receivedURI, "q=test")
	assert.Contains(t, receivedURI, "limit=10")
}

// TestWebSocket_ReadResponseError 测试读取 WebSocket 升级响应失败。
func TestWebSocket_ReadResponseError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		reader := bufio.NewReader(conn)
		_, _ = http.ReadRequest(reader)
		_ = conn.Close()
	}()

	time.Sleep(20 * time.Millisecond)

	ctx := testutil.NewRequestCtxWithHeader("GET", "/ws", map[string]string{
		"Upgrade":    "websocket",
		"Connection": "Upgrade",
	})

	target := &loadbalance.Target{URL: "http://" + ln.Addr().String()}
	target.Healthy.Store(true)

	err = WebSocket(ctx, target, 1*time.Second, nil)
	require.Error(t, err)
}

// TestCopyData_WriteErrorNonClosed 测试写入错误（非关闭类）。
func TestCopyData_WriteErrorNonClosed(t *testing.T) {
	src1, src2 := net.Pipe()
	dst1, _ := net.Pipe()

	bridge := &WebSocketBridge{}

	errCh := make(chan error, 1)
	go func() {
		errCh <- bridge.copyData(dst1, src1, "test-dir")
	}()

	_ = dst1.Close()
	_, _ = src2.Write([]byte("data"))
	_ = src2.Close()

	select {
	case err := <-errCh:
		_ = err
	case <-time.After(1 * time.Second):
		t.Error("copyData did not complete")
	}
}

// TestWebSocket_HijackFails 测试 Hijack 失败场景。
func TestWebSocket_HijackFails(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}

	target := &loadbalance.Target{URL: "http://127.0.0.1:1"}
	target.Healthy.Store(true)

	err := WebSocket(ctx, target, 100*time.Millisecond, nil)
	require.Error(t, err)
}

// TestServeHTTP_RedirectRewrite 测试重定向改写。
func TestServeHTTP_RedirectRewrite(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	backend := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(301)
			ctx.Response.Header.Set("Location", "http://"+ln.Addr().String()+"/new-path")
		},
	}

	go func() { _ = backend.Serve(ln) }()
	time.Sleep(20 * time.Millisecond)
	defer ln.Close()

	cfg := &config.ProxyConfig{
		Path:        "/",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 1 * time.Second, Read: 1 * time.Second, Write: 1 * time.Second},
		RedirectRewrite: &config.RedirectRewriteConfig{
			Mode: "default",
		},
	}

	targets := []*loadbalance.Target{{URL: "http://" + ln.Addr().String()}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	ctx := testutil.NewRequestCtxWithHeader("GET", "/old-path", map[string]string{
		"Host": "frontend.example.com",
	})
	p.ServeHTTP(ctx)

	assert.Equal(t, 301, ctx.Response.StatusCode())
	location := string(ctx.Response.Header.Peek("Location"))
	assert.Contains(t, location, "frontend.example.com")
}

// TestBuildWebSocketUpgradeRequest_Origin 测试 Origin 头复制。
func TestBuildWebSocketUpgradeRequest_Origin(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/ws")
	ctx.Request.Header.Set("Origin", "http://client.example.com")
	ctx.Request.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate")

	result := buildWebSocketUpgradeRequest(ctx, "backend:8080", nil)

	assert.Contains(t, result, "Origin: http://client.example.com")
	assert.Contains(t, result, "Sec-WebSocket-Extensions: permessage-deflate")
}

// TestCopyData_ReaderError 测试读取端错误（非关闭类）。
func TestCopyData_ReaderError(t *testing.T) {
	src1, _ := net.Pipe()
	dst1, dst2 := net.Pipe()
	defer func() { _ = dst2.Close() }()

	_ = src1.Close()

	bridge := &WebSocketBridge{}

	errCh := make(chan error, 1)
	go func() {
		errCh <- bridge.copyData(dst1, src1, "test")
	}()

	select {
	case err := <-errCh:
		_ = err
	case <-time.After(1 * time.Second):
		t.Error("copyData did not complete in time")
	}
	_ = dst1.Close()
}

// TestIsConnectionClosedError_RegularError 测试普通错误不被识别为关闭错误。
func TestIsConnectionClosedError_RegularError(t *testing.T) {
	err := errors.New("random error")
	assert.False(t, isConnectionClosedError(err))
}

// TestIsConnectionClosedError_Nil 测试 nil 不被识别为关闭错误。
func TestIsConnectionClosedError_Nil(t *testing.T) {
	assert.False(t, isConnectionClosedError(nil))
}

// TestIsConnectionClosedError_EOF 测试 EOF 被识别为关闭错误。
func TestIsConnectionClosedError_EOF(t *testing.T) {
	assert.True(t, isConnectionClosedError(io.EOF))
}
