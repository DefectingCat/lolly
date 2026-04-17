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
