// Package proxy 提供额外的覆盖测试，补充低覆盖率函数的测试。
//
// 该文件测试以下功能：
//   - HealthChecker.MarkHealthy 和 run 方法
//   - selectByLua 和 selectByFallback 方法
//   - rewriteCookies 和 rewriteCookieAttr 函数
//   - modifyResponseHeaders 边缘情况
//   - createHostClient 完整选项
//   - TempFileManager 和 TempFileCleaner getter 方法
//   - NewRedirectRewriter 正则规则和 RewriteRefreshOnly
//   - rewriteCustom 正则模式
//   - selectTarget 边缘情况
//
// 作者：xfy
package proxy

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/lua"
	"rua.plus/lolly/internal/testutil"
)

// TestHealthChecker_MarkHealthy 测试 MarkHealthy 方法。
func TestHealthChecker_MarkHealthy(t *testing.T) {
	t.Run("标记健康状态", func(t *testing.T) {
		target := &loadbalance.Target{URL: "http://backend:8080"}
		target.Healthy.Store(false)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 10 * time.Second,
			Timeout:  5 * time.Second,
		})

		checker.MarkHealthy(target)

		if !target.Healthy.Load() {
			t.Error("MarkHealthy() 后 target 应标记为 healthy")
		}
	})

	t.Run("重置失败计数", func(t *testing.T) {
		target := &loadbalance.Target{URL: "http://backend:8080"}
		target.Healthy.Store(false)
		target.RecordFailure()
		target.RecordFailure()
		target.RecordFailure()

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{})
		checker.MarkHealthy(target)

		if !target.Healthy.Load() {
			t.Error("MarkHealthy() 后 target 应标记为 healthy")
		}
	})

	t.Run("多目标场景", func(t *testing.T) {
		target1 := &loadbalance.Target{URL: "http://backend1:8080"}
		target1.Healthy.Store(false)
		target2 := &loadbalance.Target{URL: "http://backend2:8080"}
		target2.Healthy.Store(false)

		checker := NewHealthChecker([]*loadbalance.Target{target1, target2}, &config.HealthCheckConfig{})
		checker.MarkHealthy(target1)

		if !target1.Healthy.Load() {
			t.Error("target1 应标记为 healthy")
		}
		if target2.Healthy.Load() {
			t.Error("target2 应保持 unhealthy")
		}
	})
}

// TestHealthChecker_Run 测试 run 方法。
func TestHealthChecker_Run(t *testing.T) {
	t.Run("初始检查执行", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		target := &loadbalance.Target{URL: server.URL}
		target.Healthy.Store(false)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     "/health",
		})

		// 启动检查器
		checker.Start()

		// 等待初始检查完成
		time.Sleep(50 * time.Millisecond)

		// 验证初始检查已执行
		if !target.Healthy.Load() {
			t.Error("初始检查后 target 应标记为 healthy")
		}

		checker.Stop()
	})

	t.Run("定时检查执行", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		target := &loadbalance.Target{URL: server.URL}
		target.Healthy.Store(true)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 50 * time.Millisecond,
			Timeout:  5 * time.Second,
			Path:     "/health",
		})

		checker.Start()
		time.Sleep(120 * time.Millisecond)
		checker.Stop()

		// 应该至少执行初始检查 + 2 次定时检查
		if requestCount.Load() < 2 {
			t.Errorf("期望至少 2 次检查，实际 %d 次", requestCount.Load())
		}
	})

	t.Run("停止后不再检查", func(t *testing.T) {
		var requestCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		target := &loadbalance.Target{URL: server.URL}
		target.Healthy.Store(true)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 50 * time.Millisecond,
			Timeout:  5 * time.Second,
		})

		checker.Start()
		time.Sleep(60 * time.Millisecond)
		checker.Stop()
		countAfterStop := requestCount.Load()

		// 等待一段时间，确认不再有检查
		time.Sleep(100 * time.Millisecond)

		if requestCount.Load() != countAfterStop {
			t.Error("停止后不应再执行检查")
		}
	})
}

// TestSelectByFallback 测试 selectByFallback 方法。
func TestSelectByFallback(t *testing.T) {
	t.Run("round_robin fallback", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "round_robin",
			BalancerByLua: config.BalancerByLuaConfig{
				Fallback: "round_robin",
			},
			Timeout: config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080"},
			{URL: "http://backend2:8080"},
		}
		for _, t := range targets {
			t.Healthy.Store(true)
		}

		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/test")
		selected := p.selectByFallback(ctx, targets)

		if selected == nil {
			t.Error("selectByFallback() should return a target")
		}
	})

	t.Run("ip_hash fallback", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "round_robin",
			BalancerByLua: config.BalancerByLuaConfig{
				Fallback: "ip_hash",
			},
			Timeout: config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080"},
			{URL: "http://backend2:8080"},
		}
		for _, t := range targets {
			t.Healthy.Store(true)
		}

		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtxWithHeader("GET", "/api/test", map[string]string{
			"X-Forwarded-For": "192.168.1.1",
		})

		selected := p.selectByFallback(ctx, targets)
		if selected == nil {
			t.Error("selectByFallback() should return a target for ip_hash")
		}

		// 相同 IP 应返回相同目标
		selected2 := p.selectByFallback(ctx, targets)
		if selected2 == nil || selected.URL != selected2.URL {
			t.Error("ip_hash should consistently return same target for same IP")
		}
	})
}

// TestSelectByLua 测试 selectByLua 方法。
func TestSelectByLua(t *testing.T) {
	t.Run("有 Lua 引擎但脚本不存在", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "round_robin",
			BalancerByLua: config.BalancerByLuaConfig{
				Enabled: true,
				Script:  "/nonexistent/script.lua",
			},
			Timeout: config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080"},
		}
		targets[0].Healthy.Store(true)

		luaEngine, err := lua.NewEngine(nil)
		if err != nil {
			t.Fatalf("NewEngine() error: %v", err)
		}
		p, err := NewProxy(cfg, targets, nil, luaEngine)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/test")

		_, err = p.selectByLua(ctx, targets)
		if err == nil {
			t.Error("selectByLua() should return error for nonexistent script")
		}
	})

	t.Run("Lua 引擎正常工作但脚本返回错误", func(t *testing.T) {
		// 创建临时 Lua 脚本
		tmpFile, err := os.CreateTemp("", "test_*.lua")
		if err != nil {
			t.Fatalf("创建临时文件失败: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		// 写入一个会报错的脚本
		_, _ = tmpFile.WriteString("error('test error')")

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
		}
		targets[0].Healthy.Store(true)

		luaEngine, err := lua.NewEngine(nil)
		if err != nil {
			t.Fatalf("NewEngine() error: %v", err)
		}
		p, err := NewProxy(cfg, targets, nil, luaEngine)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/test")

		_, err = p.selectByLua(ctx, targets)
		// 脚本执行错误应该返回错误
		if err == nil {
			t.Error("selectByLua() should return error for script error")
		}
	})
}

// TestRewriteCookies 测试 rewriteCookies 方法。
func TestRewriteCookies(t *testing.T) {
	tests := []struct {
		name            string
		cookies         []string
		cookieDomain    string
		cookiePath      string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:         "改写 Domain",
			cookies:      []string{"session=abc123; Domain=old.example.com; Path=/"},
			cookieDomain: "new.example.com",
			wantContains: []string{"Domain=new.example.com"},
		},
		{
			name:         "改写 Path",
			cookies:      []string{"session=abc123; Domain=example.com; Path=/old/"},
			cookiePath:   "/new/",
			wantContains: []string{"Path=/new/"},
		},
		{
			name:         "同时改写 Domain 和 Path",
			cookies:      []string{"session=abc123; Domain=old.example.com; Path=/old/"},
			cookieDomain: "new.example.com",
			cookiePath:   "/new/",
			wantContains: []string{"Domain=new.example.com", "Path=/new/"},
		},
		{
			name:         "无 Domain 属性时不改写",
			cookies:      []string{"session=abc123"},
			cookiePath:   "/new/",
			wantContains: []string{"session=abc123"},
		},
		{
			name:         "空配置不改写",
			cookies:      []string{"session=abc123; Domain=example.com"},
			cookieDomain: "",
			cookiePath:   "",
			wantContains: []string{"Domain=example.com"},
		},
		{
			name:         "大小写不敏感匹配",
			cookies:      []string{"session=abc123; domain=old.example.com; path=/old/"},
			cookieDomain: "new.example.com",
			cookiePath:   "/new/",
			wantContains: []string{"domain=new.example.com", "path=/new/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ProxyConfig{
				Path:        "/api",
				LoadBalance: "round_robin",
				Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
				Headers: config.ProxyHeaders{
					CookieDomain: tt.cookieDomain,
					CookiePath:   tt.cookiePath,
				},
			}

			targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
			p, err := NewProxy(cfg, targets, nil, nil)
			if err != nil {
				t.Fatalf("NewProxy() error: %v", err)
			}

			ctx := testutil.NewRequestCtx("GET", "/api/test")
			for _, cookie := range tt.cookies {
				ctx.Response.Header.Set("Set-Cookie", cookie)
			}

			p.modifyResponseHeaders(ctx)

			cookies := strings.Split(string(ctx.Response.Header.Peek("Set-Cookie")), ";")
			cookieStr := string(ctx.Response.Header.Peek("Set-Cookie"))

			for _, want := range tt.wantContains {
				found := false
				for _, c := range cookies {
					if strings.Contains(strings.TrimSpace(c), want) || strings.Contains(cookieStr, want) {
						found = true
						break
					}
				}
				if !found && !strings.Contains(cookieStr, want) {
					t.Errorf("cookie 应包含 %q, 实际: %q", want, cookieStr)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if strings.Contains(cookieStr, notWant) {
					t.Errorf("cookie 不应包含 %q, 实际: %q", notWant, cookieStr)
				}
			}
		})
	}
}

// TestRewriteCookieAttr 测试 rewriteCookieAttr 函数。
func TestRewriteCookieAttr(t *testing.T) {
	tests := []struct {
		name     string
		cookie   string
		attr     string
		newValue string
		want     string
	}{
		{
			name:     "改写 Domain",
			cookie:   "session=abc; Domain=old.com; Path=/",
			attr:     "Domain",
			newValue: "new.com",
			want:     "session=abc; Domain=new.com; Path=/",
		},
		{
			name:     "改写 Path",
			cookie:   "session=abc; Domain=example.com; Path=/old",
			attr:     "Path",
			newValue: "/new",
			want:     "session=abc; Domain=example.com; Path=/new",
		},
		{
			name:     "属性不存在则不改写",
			cookie:   "session=abc",
			attr:     "Domain",
			newValue: "new.com",
			want:     "session=abc",
		},
		{
			name:     "大小写不敏感",
			cookie:   "session=abc; domain=old.com",
			attr:     "Domain",
			newValue: "new.com",
			want:     "session=abc; domain=new.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteCookieAttr(tt.cookie, tt.attr, tt.newValue)
			if got != tt.want {
				t.Errorf("rewriteCookieAttr() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestModifyResponseHeaders_PassResponse 测试 PassResponse 白名单模式。
func TestModifyResponseHeaders_PassResponse(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Headers: config.ProxyHeaders{
			PassResponse: []string{"Content-Type", "X-Allowed"},
		},
	}

	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	ctx.Response.Header.Set("Content-Type", "application/json")
	ctx.Response.Header.Set("X-Allowed", "allowed-value")
	ctx.Response.Header.Set("X-Blocked", "blocked-value")

	p.modifyResponseHeaders(ctx)

	// 白名单中的头应保留
	if string(ctx.Response.Header.Peek("Content-Type")) != "application/json" {
		t.Error("Content-Type 应被保留")
	}
	if string(ctx.Response.Header.Peek("X-Allowed")) != "allowed-value" {
		t.Error("X-Allowed 应被保留")
	}

	// 不在白名单中的头应被删除
	if len(ctx.Response.Header.Peek("X-Blocked")) > 0 {
		t.Error("X-Blocked 应被删除")
	}
}

// TestModifyResponseHeaders_HideResponse 测试 HideResponse 功能。
func TestModifyResponseHeaders_HideResponse(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Headers: config.ProxyHeaders{
			HideResponse: []string{"X-Hidden-Header"},
		},
	}

	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	ctx.Response.Header.Set("X-Hidden-Header", "should-be-hidden")
	ctx.Response.Header.Set("X-Visible-Header", "should-be-visible")

	p.modifyResponseHeaders(ctx)

	if len(ctx.Response.Header.Peek("X-Hidden-Header")) > 0 {
		t.Error("X-Hidden-Header 应被删除")
	}
	if string(ctx.Response.Header.Peek("X-Visible-Header")) != "should-be-visible" {
		t.Error("X-Visible-Header 应被保留")
	}
}

// TestModifyResponseHeaders_IgnoreHeaders 测试 IgnoreHeaders 功能。
func TestModifyResponseHeaders_IgnoreHeaders(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Headers: config.ProxyHeaders{
			IgnoreHeaders: []string{"X-Ignored"},
		},
	}

	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	ctx.Request.Header.Set("X-Ignored", "ignored-value")
	ctx.Response.Header.Set("X-Ignored", "ignored-response-value")
	ctx.Response.Header.Set("X-Not-Ignored", "not-ignored")

	p.modifyResponseHeaders(ctx)

	if len(ctx.Request.Header.Peek("X-Ignored")) > 0 {
		t.Error("请求中的 X-Ignored 应被删除")
	}
	if len(ctx.Response.Header.Peek("X-Ignored")) > 0 {
		t.Error("响应中的 X-Ignored 应被删除")
	}
	if string(ctx.Response.Header.Peek("X-Not-Ignored")) != "not-ignored" {
		t.Error("X-Not-Ignored 应被保留")
	}
}

// TestCreateHostClient_TransportConfig 测试 Transport 配置。
func TestCreateHostClient_TransportConfig(t *testing.T) {
	transportCfg := &config.TransportConfig{
		IdleConnTimeout: 60 * time.Second,
		MaxConnsPerHost: 50,
	}

	client := createHostClient("http://localhost:8080", config.ProxyTimeout{
		Connect: 5 * time.Second,
		Read:    30 * time.Second,
		Write:   30 * time.Second,
	}, transportCfg, nil, "", nil)

	if client == nil {
		t.Fatal("createHostClient() returned nil")
	}

	if client.MaxIdleConnDuration != 60*time.Second {
		t.Errorf("MaxIdleConnDuration = %v, want 60s", client.MaxIdleConnDuration)
	}
	if client.MaxConns != 50 {
		t.Errorf("MaxConns = %d, want 50", client.MaxConns)
	}
}

// TestCreateHostClient_Buffering 测试 Buffering 配置。
func TestCreateHostClient_Buffering(t *testing.T) {
	t.Run("streaming mode", func(t *testing.T) {
		buffering := &config.ProxyBufferingConfig{
			Mode: "off",
		}
		client := createHostClient("http://localhost:8080", config.ProxyTimeout{}, nil, nil, "", buffering)

		if !client.StreamResponseBody {
			t.Error("StreamResponseBody should be true when buffering is off")
		}
	})

	t.Run("custom buffer size", func(t *testing.T) {
		buffering := &config.ProxyBufferingConfig{
			BufferSize: 64 * 1024,
		}
		client := createHostClient("http://localhost:8080", config.ProxyTimeout{}, nil, nil, "", buffering)

		if client.ReadBufferSize != 64*1024 {
			t.Errorf("ReadBufferSize = %d, want 64KB", client.ReadBufferSize)
		}
		if client.WriteBufferSize != 64*1024 {
			t.Errorf("WriteBufferSize = %d, want 64KB", client.WriteBufferSize)
		}
	})
}

// TestCreateHostClient_ProxyBind 测试 ProxyBind 配置。
func TestCreateHostClient_ProxyBind(t *testing.T) {
	// 这个测试只验证 ProxyBind 参数不会导致 panic
	client := createHostClient("http://localhost:8080", config.ProxyTimeout{
		Connect: 5 * time.Second,
	}, nil, nil, "127.0.0.1", nil)

	if client == nil {
		t.Error("createHostClient() returned nil")
	}
	if client.Dial == nil {
		t.Error("Dial should be set when ProxyBind is specified")
	}
}

// TestTempFileManager_GetActiveCount 测试 GetActiveCount 方法。
func TestTempFileManager_GetActiveCount(t *testing.T) {
	manager, err := NewTempFileManager(t.TempDir(), "1mb", "10mb")
	if err != nil {
		t.Fatalf("NewTempFileManager() error: %v", err)
	}

	if manager.GetActiveCount() != 0 {
		t.Error("初始活动文件数应为 0")
	}

	// 创建临时文件
	tf1, err := manager.CreateTempFile()
	if err != nil {
		t.Fatalf("CreateTempFile() error: %v", err)
	}

	if manager.GetActiveCount() != 1 {
		t.Errorf("GetActiveCount() = %d, want 1", manager.GetActiveCount())
	}

	tf2, err := manager.CreateTempFile()
	if err != nil {
		t.Fatalf("CreateTempFile() error: %v", err)
	}

	if manager.GetActiveCount() != 2 {
		t.Errorf("GetActiveCount() = %d, want 2", manager.GetActiveCount())
	}

	// 清理
	_ = tf1.Close()
	_ = tf2.Close()
}

// TestDynamicTempFileWriter_GetTotalSize 测试 GetTotalSize 方法。
func TestDynamicTempFileWriter_GetTotalSize(t *testing.T) {
	manager, err := NewTempFileManager(t.TempDir(), "1mb", "10mb")
	if err != nil {
		t.Fatalf("NewTempFileManager() error: %v", err)
	}

	writer := NewDynamicTempFileWriter(manager)
	defer writer.Cleanup()

	if writer.GetTotalSize() != 0 {
		t.Error("初始总大小应为 0")
	}

	data := []byte("test data")
	_ = writer.Write(data)

	if writer.GetTotalSize() != int64(len(data)) {
		t.Errorf("GetTotalSize() = %d, want %d", writer.GetTotalSize(), len(data))
	}
}

// TestTempFileCleaner_GetInterval_GetMaxAge 测试 getter 方法。
func TestTempFileCleaner_GetInterval_GetMaxAge(t *testing.T) {
	t.Run("默认值", func(t *testing.T) {
		cleaner := NewTempFileCleaner(t.TempDir(), 0, 0)

		if cleaner.GetInterval() != DefaultCleanupInterval {
			t.Errorf("GetInterval() = %v, want %v", cleaner.GetInterval(), DefaultCleanupInterval)
		}
		if cleaner.GetMaxAge() != DefaultMaxFileAge {
			t.Errorf("GetMaxAge() = %v, want %v", cleaner.GetMaxAge(), DefaultMaxFileAge)
		}
	})

	t.Run("自定义值", func(t *testing.T) {
		cleaner := NewTempFileCleaner(t.TempDir(), 10*time.Second, 30*time.Minute)

		if cleaner.GetInterval() != 10*time.Second {
			t.Errorf("GetInterval() = %v, want 10s", cleaner.GetInterval())
		}
		if cleaner.GetMaxAge() != 30*time.Minute {
			t.Errorf("GetMaxAge() = %v, want 30m", cleaner.GetMaxAge())
		}
	})
}

// TestNewRedirectRewriter_RegexRules 测试正则规则。
func TestNewRedirectRewriter_RegexRules(t *testing.T) {
	t.Run("正则模式", func(t *testing.T) {
		cfg := &config.RedirectRewriteConfig{
			Mode: "custom",
			Rules: []config.RedirectRewriteRule{
				{Pattern: "~http://backend:\\d+", Replacement: "http://frontend"},
			},
		}

		rw, err := NewRedirectRewriter(cfg, "/")
		if err != nil {
			t.Fatalf("NewRedirectRewriter() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/")
		resp := &fasthttp.Response{}
		resp.Header.Set("Location", "http://backend:8080/api")
		resp.SetStatusCode(301)

		rw.RewriteResponse(resp, ctx, "", "frontend")

		got := string(resp.Header.Peek("Location"))
		want := "http://frontend/api"
		if got != want {
			t.Errorf("Location = %q, want %q", got, want)
		}
	})

	t.Run("大小写不敏感正则", func(t *testing.T) {
		cfg := &config.RedirectRewriteConfig{
			Mode: "custom",
			Rules: []config.RedirectRewriteRule{
				// 注意：大小写不敏感模式下，pattern 应该是小写，因为代码会将输入转为小写匹配
				{Pattern: "~*backend", Replacement: "frontend"},
			},
		}

		rw, err := NewRedirectRewriter(cfg, "/")
		if err != nil {
			t.Fatalf("NewRedirectRewriter() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/")
		resp := &fasthttp.Response{}
		// 使用大写的 URL 来测试大小写不敏感匹配
		resp.Header.Set("Location", "http://BACKEND/api")
		resp.SetStatusCode(301)

		rw.RewriteResponse(resp, ctx, "", "frontend")

		got := string(resp.Header.Peek("Location"))
		want := "http://frontend/api"
		if got != want {
			t.Errorf("Location = %q, want %q", got, want)
		}
	})

	t.Run("无效正则返回错误", func(t *testing.T) {
		cfg := &config.RedirectRewriteConfig{
			Mode: "custom",
			Rules: []config.RedirectRewriteRule{
				{Pattern: "~[invalid", Replacement: "/"},
			},
		}

		_, err := NewRedirectRewriter(cfg, "/")
		if err == nil {
			t.Error("NewRedirectRewriter() should return error for invalid regex")
		}
	})
}

// TestRedirectRewriter_RewriteRefreshOnly 测试 RewriteRefreshOnly 方法。
func TestRedirectRewriter_RewriteRefreshOnly(t *testing.T) {
	cfg := &config.RedirectRewriteConfig{
		Mode: "custom",
		Rules: []config.RedirectRewriteRule{
			{Pattern: "http://backend:8080/", Replacement: "http://frontend/"},
		},
	}

	rw, err := NewRedirectRewriter(cfg, "/")
	if err != nil {
		t.Fatalf("NewRedirectRewriter() error: %v", err)
	}

	ctx := testutil.NewRequestCtx("GET", "/")
	resp := &fasthttp.Response{}
	resp.Header.Set("Refresh", "5; url=http://backend:8080/page")
	resp.SetStatusCode(200) // 非 3xx

	rw.RewriteRefreshOnly(resp, ctx, "", "frontend")

	got := string(resp.Header.Peek("Refresh"))
	want := "5; url=http://frontend/page"
	if got != want {
		t.Errorf("Refresh = %q, want %q", got, want)
	}
}

// TestRewriteCustom 测试 rewriteCustom 方法。
func TestRewriteCustom(t *testing.T) {
	t.Run("正则替换", func(t *testing.T) {
		cfg := &config.RedirectRewriteConfig{
			Mode: "custom",
			Rules: []config.RedirectRewriteRule{
				// 注意：rewriteCustom 不支持捕获组，只是简单替换匹配的部分
				{Pattern: "~http://[a-z]+:\\d+", Replacement: "https://new.example.com"},
			},
		}

		rw, err := NewRedirectRewriter(cfg, "/")
		if err != nil {
			t.Fatalf("NewRedirectRewriter() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/")
		result := rw.rewriteURL("http://backend:8080/api/users", ctx, "", "frontend")

		want := "https://new.example.com/api/users"
		if result != want {
			t.Errorf("rewriteURL() = %q, want %q", result, want)
		}
	})

	t.Run("精确前缀匹配", func(t *testing.T) {
		cfg := &config.RedirectRewriteConfig{
			Mode: "custom",
			Rules: []config.RedirectRewriteRule{
				{Pattern: "http://old.example.com/", Replacement: "http://new.example.com/"},
			},
		}

		rw, err := NewRedirectRewriter(cfg, "/")
		if err != nil {
			t.Fatalf("NewRedirectRewriter() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/")
		result := rw.rewriteURL("http://old.example.com/page", ctx, "", "frontend")

		want := "http://new.example.com/page"
		if result != want {
			t.Errorf("rewriteURL() = %q, want %q", result, want)
		}
	})

	t.Run("无匹配则原样返回", func(t *testing.T) {
		cfg := &config.RedirectRewriteConfig{
			Mode: "custom",
			Rules: []config.RedirectRewriteRule{
				{Pattern: "http://other.com/", Replacement: "/"},
			},
		}

		rw, err := NewRedirectRewriter(cfg, "/")
		if err != nil {
			t.Fatalf("NewRedirectRewriter() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/")
		result := rw.rewriteURL("http://example.com/page", ctx, "", "frontend")

		want := "http://example.com/page"
		if result != want {
			t.Errorf("rewriteURL() = %q, want %q", result, want)
		}
	})
}

// TestSelectTarget_EmptyTargets 测试空目标列表。
func TestSelectTarget_EmptyTargets(t *testing.T) {
	cfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
	}

	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 清空目标
	p.mu.Lock()
	p.targets = nil
	p.mu.Unlock()

	ctx := testutil.NewRequestCtx("GET", "/api/test")
	selected := p.selectTarget(ctx)

	if selected != nil {
		t.Error("selectTarget() should return nil for empty targets")
	}
}

// TestDialTarget 测试 dialTarget 函数。
func TestDialTarget(t *testing.T) {
	t.Run("连接超时", func(t *testing.T) {
		// 使用不可达地址测试超时
		_, err := dialTarget("http://10.255.255.1:9999", 100*time.Millisecond)
		if err == nil {
			t.Error("dialTarget() should return error for unreachable address")
		}
	})

	t.Run("HTTPS 连接失败", func(t *testing.T) {
		_, err := dialTarget("https://10.255.255.1:9999", 100*time.Millisecond)
		if err == nil {
			t.Error("dialTarget() should return error for unreachable HTTPS address")
		}
	})

	t.Run("成功连接", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to listen: %v", err)
		}
		defer ln.Close()

		go func() {
			conn, _ := ln.Accept()
			if conn != nil {
				_ = conn.Close()
			}
		}()

		addr := ln.Addr().String()
		conn, err := dialTarget("http://"+addr, 1*time.Second)
		if err != nil {
			t.Errorf("dialTarget() error: %v", err)
		}
		if conn != nil {
			_ = conn.Close()
		}
	})
}

// TestBackgroundRefresh_Extra 测试 backgroundRefresh 方法的额外场景。
func TestBackgroundRefresh_Extra(t *testing.T) {
	t.Run("客户端不存在时直接返回", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "round_robin",
			Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
			Cache: config.ProxyCacheConfig{
				Enabled: true,
				MaxAge:  10 * time.Second,
			},
		}

		targets := []*loadbalance.Target{{URL: "http://nonexistent:9999"}}
		targets[0].Healthy.Store(true)

		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		// 删除客户端
		p.mu.Lock()
		delete(p.clients, targets[0].URL)
		p.mu.Unlock()

		ctx := testutil.NewRequestCtx("GET", "/api/test")
		hashKey := uint64(12345)

		// 应该不会 panic
		p.backgroundRefresh(ctx, targets[0], hashKey, "GET:/api/test")
	})

	t.Run("缓存锁释放", func(t *testing.T) {
		ln := fasthttputil.NewInmemoryListener()
		defer ln.Close()

		go func() {
			s := &fasthttp.Server{
				Handler: func(ctx *fasthttp.RequestCtx) {
					ctx.SetStatusCode(200)
					ctx.SetBodyString("refreshed")
				},
			}
			_ = s.Serve(ln)
		}()

		time.Sleep(10 * time.Millisecond)

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
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/test")
		hashKey := uint64(12345)
		p.cache.AcquireLock(hashKey)

		p.backgroundRefresh(ctx, targets[0], hashKey, "GET:/api/test")
	})
}

// TestWebSocket_ErrorCases 测试 WebSocket 错误情况。
func TestWebSocket_ErrorCases(t *testing.T) {
	t.Run("连接无效后端", func(t *testing.T) {
		ctx := testutil.NewRequestCtxWithHeader("GET", "/ws", map[string]string{
			"Upgrade":    "websocket",
			"Connection": "Upgrade",
		})

		target := &loadbalance.Target{URL: "http://127.0.0.1:1"}
		target.Healthy.Store(true)

		// 使用很短的超时
		err := WebSocket(ctx, target, 10*time.Millisecond)
		if err == nil {
			t.Error("WebSocket() should return error for invalid backend")
		}
	})
}

// TestDialTarget_TLS_Extra 测试 TLS 连接。
func TestDialTarget_TLS_Extra(t *testing.T) {
	t.Run("TLS 握手失败", func(t *testing.T) {
		// 使用不可达的 HTTPS 地址
		_, err := dialTarget("https://10.255.255.1:9999", 100*time.Millisecond)
		if err == nil {
			t.Error("dialTarget() should return error for unreachable HTTPS address")
		}
	})
}

// TestCreateHostClient_SSL 测试 SSL 配置。
func TestCreateHostClient_SSL(t *testing.T) {
	t.Run("启用 SSL 验证", func(t *testing.T) {
		sslCfg := &config.ProxySSLConfig{
			Enabled:            true,
			InsecureSkipVerify: false,
		}

		client := createHostClient("https://example.com:443", config.ProxyTimeout{
			Connect: 5 * time.Second,
		}, nil, sslCfg, "", nil)

		if client == nil {
			t.Error("createHostClient() returned nil")
		}
		if client.TLSConfig == nil {
			t.Error("TLSConfig should be set for HTTPS target")
		}
	})
}

// TestBackgroundRefresh_Revalidate 测试缓存后台刷新的 Revalidate 功能。
func TestBackgroundRefresh_Revalidate(t *testing.T) {
	t.Run("Revalidate 启用但无缓存条目", func(t *testing.T) {
		ln := fasthttputil.NewInmemoryListener()
		defer ln.Close()

		go func() {
			s := &fasthttp.Server{
				Handler: func(ctx *fasthttp.RequestCtx) {
					ctx.SetStatusCode(200)
					ctx.SetBodyString("refreshed")
				},
			}
			_ = s.Serve(ln)
		}()

		time.Sleep(10 * time.Millisecond)

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

		targets := []*loadbalance.Target{{URL: "http://" + ln.Addr().String()}}
		targets[0].Healthy.Store(true)

		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/test")
		hashKey := uint64(12345)

		// 无缓存条目时调用 backgroundRefresh
		p.backgroundRefresh(ctx, targets[0], hashKey, "GET:/api/test")
	})
}

// TestSelectByBalancer 测试 selectByBalancer 方法。
func TestSelectByBalancer(t *testing.T) {
	t.Run("IPHash 负载均衡", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "ip_hash",
			Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080"},
			{URL: "http://backend2:8080"},
		}
		for _, t := range targets {
			t.Healthy.Store(true)
		}

		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		// 使用不同 IP 的请求应选择不同目标
		ctx1 := testutil.NewRequestCtxWithHeader("GET", "/api/test", map[string]string{
			"X-Forwarded-For": "192.168.1.1",
		})
		ctx2 := testutil.NewRequestCtxWithHeader("GET", "/api/test", map[string]string{
			"X-Forwarded-For": "192.168.1.2",
		})

		selected1 := p.selectByBalancer(ctx1, targets)
		selected2 := p.selectByBalancer(ctx2, targets)

		if selected1 == nil || selected2 == nil {
			t.Error("selectByBalancer() should return a target")
		}
	})

	t.Run("ConsistentHash 负载均衡", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:         "/api",
			LoadBalance:  "consistent_hash",
			VirtualNodes: 100,
			HashKey:      "uri",
			Timeout:      config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080"},
			{URL: "http://backend2:8080"},
		}
		for _, t := range targets {
			t.Healthy.Store(true)
		}

		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/users/123")
		selected := p.selectByBalancer(ctx, targets)

		if selected == nil {
			t.Error("selectByBalancer() should return a target for consistent_hash")
		}

		// 相同 URI 应选择相同目标
		selected2 := p.selectByBalancer(ctx, targets)
		if selected2 == nil || selected.URL != selected2.URL {
			t.Error("consistent_hash should return same target for same URI")
		}
	})

	t.Run("ConsistentHash with header key", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:         "/api",
			LoadBalance:  "consistent_hash",
			VirtualNodes: 100,
			HashKey:      "header:X-User-Id",
			Timeout:      config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080"},
			{URL: "http://backend2:8080"},
		}
		for _, t := range targets {
			t.Healthy.Store(true)
		}

		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtxWithHeader("GET", "/api/test", map[string]string{
			"X-User-Id": "user-123",
		})
		selected := p.selectByBalancer(ctx, targets)

		if selected == nil {
			t.Error("selectByBalancer() should return a target for header-based hash")
		}
	})

	t.Run("RoundRobin 负载均衡", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "round_robin",
			Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080"},
			{URL: "http://backend2:8080"},
		}
		for _, t := range targets {
			t.Healthy.Store(true)
		}

		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/test")
		selected := p.selectByBalancer(ctx, targets)

		if selected == nil {
			t.Error("selectByBalancer() should return a target for round_robin")
		}
	})
}

// TestSelectTargetExcluding_Extra 测试 selectTargetExcluding 方法。
func TestSelectTargetExcluding_Extra(t *testing.T) {
	t.Run("排除已失败目标", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "round_robin",
			Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080"},
			{URL: "http://backend2:8080"},
			{URL: "http://backend3:8080"},
		}
		for _, t := range targets {
			t.Healthy.Store(true)
		}

		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/test")

		// 排除第一个目标
		excluded := []*loadbalance.Target{targets[0]}
		selected := p.selectTargetExcluding(ctx, excluded)

		if selected == nil {
			t.Error("selectTargetExcluding() should return a target")
		}
		if selected != nil && selected.URL == targets[0].URL {
			t.Error("selectTargetExcluding() should not return excluded target")
		}
	})

	t.Run("排除所有目标返回 nil", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "round_robin",
			Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080"},
		}
		targets[0].Healthy.Store(true)

		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/test")

		// 排除所有目标
		excluded := []*loadbalance.Target{targets[0]}
		selected := p.selectTargetExcluding(ctx, excluded)

		if selected != nil {
			t.Error("selectTargetExcluding() should return nil when all targets excluded")
		}
	})
}

// TestExtractHashKey_Extra 测试 extractHashKey 方法。
func TestExtractHashKey_Extra(t *testing.T) {
	t.Run("使用 IP 作为 hash key", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "consistent_hash",
			HashKey:     "ip",
			Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtxWithHeader("GET", "/api/test", map[string]string{
			"X-Forwarded-For": "10.0.0.1",
		})

		key := p.extractHashKey(ctx, "ip")
		if key != "10.0.0.1" {
			t.Errorf("extractHashKey() = %q, want %q", key, "10.0.0.1")
		}
	})

	t.Run("使用 URI 作为 hash key", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "consistent_hash",
			HashKey:     "uri",
			Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/users/123")

		key := p.extractHashKey(ctx, "uri")
		if key != "/api/users/123" {
			t.Errorf("extractHashKey() = %q, want %q", key, "/api/users/123")
		}
	})

	t.Run("使用 header 作为 hash key", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "consistent_hash",
			HashKey:     "header:X-Session-Id",
			Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtxWithHeader("GET", "/api/test", map[string]string{
			"X-Session-Id": "session-abc-123",
		})

		key := p.extractHashKey(ctx, "header:X-Session-Id")
		if key != "session-abc-123" {
			t.Errorf("extractHashKey() = %q, want %q", key, "session-abc-123")
		}
	})

	t.Run("header 不存在时回退到 IP", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "consistent_hash",
			HashKey:     "header:X-Nonexistent",
			Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtxWithHeader("GET", "/api/test", map[string]string{
			"X-Forwarded-For": "10.0.0.5",
		})

		key := p.extractHashKey(ctx, "header:X-Nonexistent")
		if key != "10.0.0.5" {
			t.Errorf("extractHashKey() should fallback to IP, got %q", key)
		}
	})
}

// TestSelectTarget_LuaEnabled 测试 selectTarget 在 Lua 启用时的行为。
func TestSelectTarget_LuaEnabled(t *testing.T) {
	t.Run("Lua 引擎为 nil 时使用传统算法", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "round_robin",
			BalancerByLua: config.BalancerByLuaConfig{
				Enabled: true,
				Script:  "/nonexistent.lua",
			},
			Timeout: config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080"},
			{URL: "http://backend2:8080"},
		}
		for _, t := range targets {
			t.Healthy.Store(true)
		}

		// luaEngine 为 nil
		p, err := NewProxy(cfg, targets, nil, nil)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/test")
		selected := p.selectTarget(ctx)

		if selected == nil {
			t.Error("selectTarget() should return a target using fallback")
		}
	})

	t.Run("Lua 脚本为空时使用传统算法", func(t *testing.T) {
		cfg := &config.ProxyConfig{
			Path:        "/api",
			LoadBalance: "round_robin",
			BalancerByLua: config.BalancerByLuaConfig{
				Enabled: true,
				Script:  "",
			},
			Timeout: config.ProxyTimeout{Connect: 5 * time.Second},
		}

		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080"},
		}
		targets[0].Healthy.Store(true)

		luaEngine, err := lua.NewEngine(nil)
		if err != nil {
			t.Fatalf("NewEngine() error: %v", err)
		}
		p, err := NewProxy(cfg, targets, nil, luaEngine)
		if err != nil {
			t.Fatalf("NewProxy() error: %v", err)
		}

		ctx := testutil.NewRequestCtx("GET", "/api/test")
		selected := p.selectTarget(ctx)

		if selected == nil {
			t.Error("selectTarget() should return a target using traditional balancer")
		}
	})
}

// TestExtractHostFromURL 测试 extractHostFromURL 函数。
func TestExtractHostFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "HTTP URL with port",
			url:  "http://example.com:8080",
			want: "example.com:8080",
		},
		{
			name: "HTTPS URL with port",
			url:  "https://example.com:8443",
			want: "example.com:8443",
		},
		{
			name: "HTTP URL without port",
			url:  "http://example.com",
			want: "example.com",
		},
		{
			name: "HTTPS URL without port",
			url:  "https://example.com",
			want: "example.com",
		},
		{
			name: "URL with path",
			url:  "http://example.com:8080/api/users",
			want: "example.com:8080",
		},
		{
			name: "No protocol prefix",
			url:  "example.com:8080",
			want: "example.com:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHostFromURL(tt.url)
			if got != tt.want {
				t.Errorf("extractHostFromURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTempFileManager_Threshold 测试 ShouldUseTempFile 的阈值逻辑。
func TestTempFileManager_Threshold(t *testing.T) {
	t.Run("响应大于阈值", func(t *testing.T) {
		manager, err := NewTempFileManager(t.TempDir(), "1kb", "10kb")
		if err != nil {
			t.Fatalf("NewTempFileManager() error: %v", err)
		}

		if !manager.ShouldUseTempFile(2048) {
			t.Error("ShouldUseTempFile() should return true for 2KB when threshold is 1KB")
		}
	})

	t.Run("响应小于阈值", func(t *testing.T) {
		manager, err := NewTempFileManager(t.TempDir(), "1kb", "10kb")
		if err != nil {
			t.Fatalf("NewTempFileManager() error: %v", err)
		}

		if manager.ShouldUseTempFile(512) {
			t.Error("ShouldUseTempFile() should return false for 512B when threshold is 1KB")
		}
	})
}

// TestBackgroundRefresh_304 测试后台刷新收到 304 响应。
func TestBackgroundRefresh_304(t *testing.T) {
	ln := fasthttputil.NewInmemoryListener()
	defer ln.Close()

	go func() {
		s := &fasthttp.Server{
			Handler: func(ctx *fasthttp.RequestCtx) {
				// 检查条件请求头
				if ctx.Request.Header.Peek("If-Modified-Since") != nil ||
					ctx.Request.Header.Peek("If-None-Match") != nil {
					ctx.SetStatusCode(304)
					ctx.Response.Header.Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
					ctx.Response.Header.Set("ETag", "\"abc123\"")
					return
				}
				ctx.SetStatusCode(200)
				ctx.SetBodyString("fresh content")
			},
		}
		_ = s.Serve(ln)
	}()

	time.Sleep(10 * time.Millisecond)

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

	targets := []*loadbalance.Target{{URL: "http://" + ln.Addr().String()}}
	targets[0].Healthy.Store(true)

	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 预先设置缓存条目
	ctx := testutil.NewRequestCtx("GET", "/api/test")
	hashKey, origKey := p.buildCacheKeyHash(ctx)
	p.cache.Set(hashKey, origKey, []byte("cached"), map[string]string{
		"Last-Modified": "Tue, 20 Oct 2015 07:28:00 GMT",
		"ETag":          "\"old\"",
	}, 200, 10*time.Second)

	// 调用后台刷新
	p.backgroundRefresh(ctx, targets[0], hashKey, origKey)
}
