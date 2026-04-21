// Package server 提供虚拟主机管理器的测试。
package server

import (
	"testing"

	"github.com/valyala/fasthttp"
)

// mockHandler 创建一个记录调用的 mock handler
func mockHandler(name string, called *bool) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		*called = true

		_, _ = ctx.WriteString(name)
	}
}

// TestVHostManager_Handler 测试虚拟主机选择器功能。
func TestVHostManager_Handler(t *testing.T) {
	t.Run("匹配已知主机", func(t *testing.T) {
		manager := NewVHostManager()
		hostCalled := false
		_ = manager.AddHost("example.com", mockHandler("example", &hostCalled))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("example.com")

		handler(ctx)

		if !hostCalled {
			t.Error("期望 example.com handler 被调用，但未被调用")
		}
		if string(ctx.Response.Body()) != "example" {
			t.Errorf("响应体 = %q, want %q", string(ctx.Response.Body()), "example")
		}
	})

	t.Run("匹配带端口的主机", func(t *testing.T) {
		manager := NewVHostManager()
		hostCalled := false
		_ = manager.AddHost("example.com", mockHandler("example", &hostCalled))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("example.com:8080")

		handler(ctx)

		if !hostCalled {
			t.Error("期望 example.com handler 被调用（端口应被忽略），但未被调用")
		}
		if string(ctx.Response.Body()) != "example" {
			t.Errorf("响应体 = %q, want %q", string(ctx.Response.Body()), "example")
		}
	})

	t.Run("无匹配使用默认主机", func(t *testing.T) {
		manager := NewVHostManager()
		exampleCalled := false
		defaultCalled := false
		_ = manager.AddHost("example.com", mockHandler("example", &exampleCalled))
		manager.SetDefault(mockHandler("default", &defaultCalled))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("unknown.com")

		handler(ctx)

		if exampleCalled {
			t.Error("不期望 example.com handler 被调用")
		}
		if !defaultCalled {
			t.Error("期望默认 handler 被调用，但未被调用")
		}
		if string(ctx.Response.Body()) != "default" {
			t.Errorf("响应体 = %q, want %q", string(ctx.Response.Body()), "default")
		}
	})

	t.Run("无匹配无默认返回404", func(t *testing.T) {
		manager := NewVHostManager()
		exampleCalled := false
		_ = manager.AddHost("example.com", mockHandler("example", &exampleCalled))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("unknown.com")

		handler(ctx)

		if exampleCalled {
			t.Error("不期望 example.com handler 被调用")
		}
		if ctx.Response.StatusCode() != fasthttp.StatusNotFound {
			t.Errorf("状态码 = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusNotFound)
		}
	})

	t.Run("IPv6地址Host", func(t *testing.T) {
		manager := NewVHostManager()
		ipv6Called := false
		_ = manager.AddHost("[::1]", mockHandler("ipv6", &ipv6Called))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("[::1]:8080")

		handler(ctx)

		if !ipv6Called {
			t.Error("期望 [::1] handler 被调用，但未被调用")
		}
		if string(ctx.Response.Body()) != "ipv6" {
			t.Errorf("响应体 = %q, want %q", string(ctx.Response.Body()), "ipv6")
		}
	})

	t.Run("空Host使用默认", func(t *testing.T) {
		manager := NewVHostManager()
		defaultCalled := false
		manager.SetDefault(mockHandler("default", &defaultCalled))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("")

		handler(ctx)

		if !defaultCalled {
			t.Error("期望默认 handler 被调用，但未被调用")
		}
		if string(ctx.Response.Body()) != "default" {
			t.Errorf("响应体 = %q, want %q", string(ctx.Response.Body()), "default")
		}
	})

	t.Run("空Host无默认返回404", func(t *testing.T) {
		manager := NewVHostManager()

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("")

		handler(ctx)

		if ctx.Response.StatusCode() != fasthttp.StatusNotFound {
			t.Errorf("状态码 = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusNotFound)
		}
	})
}

// TestVHostManager_AddHost 测试添加虚拟主机功能。
func TestVHostManager_AddHost(t *testing.T) {
	t.Run("添加单个主机", func(t *testing.T) {
		manager := NewVHostManager()
		called := false
		_ = manager.AddHost("test.com", mockHandler("test", &called))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("test.com")

		handler(ctx)

		if !called {
			t.Error("期望添加的主机 handler 被调用")
		}
	})

	t.Run("添加多个主机", func(t *testing.T) {
		manager := NewVHostManager()
		host1Called := false
		host2Called := false
		_ = manager.AddHost("host1.com", mockHandler("host1", &host1Called))
		_ = manager.AddHost("host2.com", mockHandler("host2", &host2Called))

		handler := manager.Handler()

		// 测试 host1
		ctx1 := &fasthttp.RequestCtx{}
		ctx1.Request.SetHost("host1.com")
		handler(ctx1)
		if !host1Called {
			t.Error("期望 host1 handler 被调用")
		}

		// 测试 host2
		ctx2 := &fasthttp.RequestCtx{}
		ctx2.Request.SetHost("host2.com")
		handler(ctx2)
		if !host2Called {
			t.Error("期望 host2 handler 被调用")
		}
	})

	t.Run("覆盖已存在的主机", func(t *testing.T) {
		manager := NewVHostManager()
		firstCalled := false
		secondCalled := false
		_ = manager.AddHost("test.com", mockHandler("first", &firstCalled))
		_ = manager.AddHost("test.com", mockHandler("second", &secondCalled))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("test.com")

		handler(ctx)

		if firstCalled {
			t.Error("不期望第一个 handler 被调用（应被覆盖）")
		}
		if !secondCalled {
			t.Error("期望第二个 handler 被调用")
		}
	})
}

// TestVHostManager_SetDefault 测试设置默认主机功能。
func TestVHostManager_SetDefault(t *testing.T) {
	t.Run("设置默认主机", func(t *testing.T) {
		manager := NewVHostManager()
		defaultCalled := false
		manager.SetDefault(mockHandler("default", &defaultCalled))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("nonexistent.com")

		handler(ctx)

		if !defaultCalled {
			t.Error("期望默认 handler 被调用")
		}
	})

	t.Run("覆盖默认主机", func(t *testing.T) {
		manager := NewVHostManager()
		firstCalled := false
		secondCalled := false
		manager.SetDefault(mockHandler("first", &firstCalled))
		manager.SetDefault(mockHandler("second", &secondCalled))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("unknown.com")

		handler(ctx)

		if firstCalled {
			t.Error("不期望第一个默认 handler 被调用（应被覆盖）")
		}
		if !secondCalled {
			t.Error("期望第二个默认 handler 被调用")
		}
	})
}

// TestVHostManager_WildcardPrefix 测试前缀通配符 *.example.com。
func TestVHostManager_WildcardPrefix(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		host        string
		shouldMatch bool
	}{
		{"exact subdomain", "*.example.com", "www.example.com", true},
		{"nested subdomain", "*.example.com", "api.www.example.com", true},
		{"no subdomain", "*.example.com", "example.com", false},
		{"different domain", "*.example.com", "www.other.com", false},
		{"longest match", "*.b.example.com", "a.b.example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewVHostManager()
			called := false
			_ = manager.AddHost(tt.pattern, mockHandler("wildcard", &called))

			handler := manager.Handler()
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetHost(tt.host)

			handler(ctx)

			if called != tt.shouldMatch {
				t.Errorf("expected match %v, got %v", tt.shouldMatch, called)
			}
		})
	}
}

// TestVHostManager_WildcardSuffix 测试后缀通配符 example.*。
func TestVHostManager_WildcardSuffix(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		host        string
		shouldMatch bool
	}{
		{"match com", "example.*", "example.com", true},
		{"match net", "example.*", "example.net", true},
		{"match org", "example.*", "example.org", true},
		{"no match subdomain", "example.*", "www.example.com", false},
		{"no match different prefix", "example.*", "other.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewVHostManager()
			called := false
			_ = manager.AddHost(tt.pattern, mockHandler("suffix", &called))

			handler := manager.Handler()
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetHost(tt.host)

			handler(ctx)

			if called != tt.shouldMatch {
				t.Errorf("expected match %v, got %v", tt.shouldMatch, called)
			}
		})
	}
}

// TestVHostManager_Regex 测试正则匹配。
func TestVHostManager_Regex(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		host        string
		shouldMatch bool
		wantErr     bool
	}{
		{"match digits", "~^api[0-9]+\\.example\\.com$", "api1.example.com", true, false},
		{"match digits 2", "~^api[0-9]+\\.example\\.com$", "api99.example.com", true, false},
		{"no match letters", "~^api[0-9]+\\.example\\.com$", "apiX.example.com", false, false},
		{"invalid regex", "~[invalid", "", false, true},
		{"match any subdomain", "~.*\\.example\\.com", "www.example.com", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewVHostManager()
			called := false
			err := manager.AddHost(tt.pattern, mockHandler("regex", &called))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error for invalid regex")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			handler := manager.Handler()
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetHost(tt.host)

			handler(ctx)

			if called != tt.shouldMatch {
				t.Errorf("expected match %v, got %v", tt.shouldMatch, called)
			}
		})
	}
}

// TestVHostManager_MatchPriority 测试匹配优先级。
func TestVHostManager_MatchPriority(t *testing.T) {
	t.Run("exact over wildcard", func(t *testing.T) {
		manager := NewVHostManager()
		exactCalled := false
		wildcardCalled := false

		_ = manager.AddHost("www.example.com", mockHandler("exact", &exactCalled))
		_ = manager.AddHost("*.example.com", mockHandler("wildcard", &wildcardCalled))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("www.example.com")

		handler(ctx)

		if !exactCalled {
			t.Error("expected exact match to be called")
		}
		if wildcardCalled {
			t.Error("expected wildcard to NOT be called when exact match exists")
		}
	})

	t.Run("longest wildcard prefix", func(t *testing.T) {
		manager := NewVHostManager()
		shortCalled := false
		longCalled := false

		_ = manager.AddHost("*.example.com", mockHandler("short", &shortCalled))
		_ = manager.AddHost("*.b.example.com", mockHandler("long", &longCalled))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("a.b.example.com")

		handler(ctx)

		if shortCalled {
			t.Error("expected short wildcard to NOT be called")
		}
		if !longCalled {
			t.Error("expected longest wildcard match to be called")
		}
	})
}

// TestVHostManager_FindHost 测试 FindHost 方法。
func TestVHostManager_FindHost(t *testing.T) {
	manager := NewVHostManager()
	_ = manager.AddHost("exact.com", mockHandler("exact", new(bool)))
	_ = manager.AddHost("*.wildcard.com", mockHandler("wildcard", new(bool)))
	_ = manager.AddHost("suffix.*", mockHandler("suffix", new(bool)))
	_ = manager.AddHost("~^regex.*", mockHandler("regex", new(bool)))
	manager.SetDefault(mockHandler("default", new(bool)))

	tests := []struct {
		host     string
		wantName string
	}{
		{"exact.com", "exact.com"},
		{"www.wildcard.com", "*.wildcard.com"},
		{"suffix.net", "suffix.*"},
		{"regex123", "~^regex.*"},
		{"unknown.com", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			vhost := manager.FindHost(tt.host)
			if vhost == nil {
				t.Fatal("expected non-nil vhost")
			}
			if vhost.name != tt.wantName {
				t.Errorf("expected name %q, got %q", tt.wantName, vhost.name)
			}
		})
	}
}

// TestVHostManager_FindHost_NilDefault 测试无默认主机时返回 nil。
func TestVHostManager_FindHost_NilDefault(t *testing.T) {
	manager := NewVHostManager()
	_ = manager.AddHost("example.com", mockHandler("example", new(bool)))

	vhost := manager.FindHost("unknown.com")
	if vhost != nil {
		t.Error("expected nil when no default host and no match")
	}
}

// TestVHostManager_AddHost_InvalidRegex 测试无效正则表达式。
func TestVHostManager_AddHost_InvalidRegex(t *testing.T) {
	manager := NewVHostManager()
	err := manager.AddHost("~[invalid(regex", mockHandler("test", new(bool)))

	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

// TestVHostManager_PortStripping 测试端口剥离逻辑。
func TestVHostManager_PortStripping(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"无端口", "example.com", "example.com"},
		{"标准HTTP端口", "example.com:80", "example.com"},
		{"标准HTTPS端口", "example.com:443", "example.com"},
		{"自定义端口", "example.com:8080", "example.com"},
		{"IPv6 localhost带端口", "[localhost]:8080", "[localhost]"},
		{"IPv6 loopback带端口", "[::1]:8080", "[::1]"},
		{"IPv6完整地址带端口", "[2001:db8::1]:443", "[2001:db8::1]"},
		{"IPv6无端口", "[::1]", "[::1]"},
		{"IPv6完整地址无端口", "[2001:db8::1]", "[2001:db8::1]"},
		{"空字符串", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewVHostManager()
			called := false
			_ = manager.AddHost(tt.expected, mockHandler("matched", &called))

			handler := manager.Handler()
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetHost(tt.input)

			handler(ctx)

			if !called {
				t.Errorf("Host %q 期望匹配 %q，但未匹配", tt.input, tt.expected)
			}
		})
	}

	// IPv6 数字地址测试
	t.Run("IPv6数字地址", func(t *testing.T) {
		manager := NewVHostManager()
		ipv6Called := false
		manager.AddHost("[::1]", mockHandler("ipv6", &ipv6Called))

		handler := manager.Handler()
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetHost("[::1]:8080")

		handler(ctx)

		if !ipv6Called {
			t.Error("期望 [::1] handler 被调用，但未被调用")
		}
		if string(ctx.Response.Body()) != "ipv6" {
			t.Errorf("响应体 = %q, want %q", string(ctx.Response.Body()), "ipv6")
		}
	})
}
