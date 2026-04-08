// variable_integration_test.go - 变量系统集成测试
//
// 测试变量系统与日志、代理、重写的端到端集成
//
// 作者：xfy
package integration

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/middleware/rewrite"
	"rua.plus/lolly/internal/variable"
)

// TestVariableEndToEndInLogging 测试变量在访问日志中的端到端使用
func TestVariableEndToEndInLogging(t *testing.T) {
	// 创建内存日志输出
	var buf bytes.Buffer

	cfg := &config.LoggingConfig{
		Access: config.AccessLogConfig{
			// 使用完整的 nginx 风格格式
			Format: "$remote_addr - $remote_user [$time_local] \"$request_method $request_uri $scheme\" $status $body_bytes_sent \"$http_user_agent\"",
		},
	}

	logger := logging.New(cfg)

	// 模拟请求
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/users?page=1")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetHost("example.com")
	ctx.Request.Header.Set("User-Agent", "TestAgent/1.0")

	// 记录访问日志
	logger.LogAccess(ctx, 200, 1024, 50*time.Millisecond)

	// 验证输出（这里主要验证不 panic）
	_ = buf.String()
	t.Log("Logging with variables completed successfully")
}

// TestVariableInProxyHeaders 测试代理请求头中的变量
func TestVariableInProxyHeaders(t *testing.T) {
	// 创建请求
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/test")
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.SetHost("api.example.com")
	ctx.Request.Header.Set("X-Custom-Header", "original")

	// 测试变量展开
	vc := variable.NewVariableContext(ctx)
	defer variable.ReleaseVariableContext(vc)

	// 模拟代理配置中的 header 设置
	tests := []struct {
		template string
		contains []string
	}{
		{"X-Forwarded-Host: $host", []string{"api.example.com"}},
		{"X-Real-IP: $remote_addr", []string{"0.0.0.0"}},
		{"X-Request-ID: $request_id", []string{"-"}}, // 未设置时为 -
	}

	for _, tt := range tests {
		result := vc.Expand(tt.template)
		for _, expected := range tt.contains {
			if !strings.Contains(result, expected) {
				t.Errorf("Expand(%q) = %q, expected to contain %q", tt.template, result, expected)
			}
		}
	}
}

// TestVariableInRewriteRules 测试重写规则中的变量
func TestVariableInRewriteRules(t *testing.T) {
	// 创建带有变量的重写规则
	rules := []config.RewriteRule{
		{
			Pattern:     "^/api/(.*)$",
			Replacement: "/v1/$1",
			Flag:        "break",
		},
		{
			Pattern:     "^/redirect/(.*)$",
			Replacement: "/new/$1",
			Flag:        "redirect",
		},
	}

	mw, err := rewrite.New(rules)
	if err != nil {
		t.Fatalf("failed to create rewrite middleware: %v", err)
	}

	// 测试第一个规则
	t.Run("rewrite_with_capture", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/users")
		ctx.Request.Header.SetMethod("GET")

		var capturedPath string
		next := func(c *fasthttp.RequestCtx) {
			capturedPath = string(c.Path())
		}

		handler := mw.Process(next)
		handler(ctx)

		if capturedPath != "/v1/users" {
			t.Errorf("expected path '/v1/users', got %q", capturedPath)
		}
	})

	// 测试重定向规则
	t.Run("redirect_with_capture", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/redirect/old-page")
		ctx.Request.Header.SetMethod("GET")

		handler := mw.Process(func(c *fasthttp.RequestCtx) {
			t.Error("should not reach next handler for redirect")
		})
		handler(ctx)

		// 验证重定向状态码
		if ctx.Response.StatusCode() != 302 {
			t.Errorf("expected status 302, got %d", ctx.Response.StatusCode())
		}
	})
}

// TestVariableCompatibilityWithNginx 测试与 nginx 变量格式的兼容性
func TestVariableCompatibilityWithNginx(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test/path?foo=bar&baz=qux")
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.SetHost("example.com")
	ctx.Request.Header.Set("User-Agent", "TestAgent/1.0")
	ctx.Request.Header.Set("Referer", "http://referrer.com")

	// 设置响应信息
	variable.SetResponseInfoInContext(ctx, 201, 2048, 100000000) // 100ms

	vc := variable.NewVariableContext(ctx)
	defer variable.ReleaseVariableContext(vc)

	// 设置 HTTP 头变量（logging 中会自动设置这些）
	vc.Set("http_referer", "http://referrer.com")
	vc.Set("http_user_agent", "TestAgent/1.0")
	vc.Set("remote_user", "-")

	// 测试 nginx 风格的组合日志格式
	logFormat := "$remote_addr - $remote_user [$time_local] \"$request_method $request_uri $scheme\" $status $body_bytes_sent \"$http_referer\" \"$http_user_agent\""
	result := vc.Expand(logFormat)

	// 验证结果包含期望的部分
	expectedParts := []string{
		"POST",                // method
		"/test/path",          // path
		"foo=bar",             // query string
		"http",                // scheme
		"201",                 // status
		"2048",                // body_bytes_sent
		"http://referrer.com", // referer
		"TestAgent/1.0",       // user agent
	}

	for _, part := range expectedParts {
		if !strings.Contains(result, part) {
			t.Errorf("log output missing %q: got %q", part, result)
		}
	}

	t.Logf("Generated log: %s", result)
}

// TestVariablePerformance 测试变量展开性能
func TestVariablePerformance(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/v1/users/123?active=true")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetHost("api.example.com")

	vc := variable.NewVariableContext(ctx)
	defer variable.ReleaseVariableContext(vc)

	// 常见日志格式模板
	template := "$remote_addr - $remote_user [$time_local] \"$request_method $request_uri $scheme\" $status $body_bytes_sent \"$http_user_agent\""

	// 执行多次展开
	start := time.Now()
	iterations := 10000
	for i := 0; i < iterations; i++ {
		_ = vc.Expand(template)
	}
	elapsed := time.Since(start)

	// 计算平均时间
	avg := elapsed / time.Duration(iterations)
	t.Logf("Average expansion time: %v (iterations: %d)", avg, iterations)

	// 验证性能在合理范围内（< 1μs 每次）
	if avg > time.Microsecond {
		t.Errorf("average time %v exceeds 1μs", avg)
	}
}

// TestVariableEdgeCases 测试变量的边界情况
func TestVariableEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*fasthttp.RequestCtx)
		template string
		expected string
	}{
		{
			name: "empty_template",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.SetRequestURI("/test")
			},
			template: "",
			expected: "",
		},
		{
			name: "no_variables",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.SetRequestURI("/test")
			},
			template: "static text without variables",
			expected: "static text without variables",
		},
		{
			name: "undefined_variable",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.SetRequestURI("/test")
			},
			template: "$undefined_var",
			expected: "$undefined_var", // 保持原样
		},
		{
			name: "mixed_defined_undefined",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.SetRequestURI("/test")
				ctx.Request.Header.SetHost("example.com")
			},
			template: "$host-$undefined",
			expected: "example.com-$undefined",
		},
		{
			name: "special_characters_in_value",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.SetRequestURI("/test%20path")
				ctx.Request.Header.SetHost("example.com")
			},
			template: "$uri",
			expected: "/test path", // fasthttp Path() 返回解码后的路径
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			tt.setup(ctx)

			vc := variable.NewVariableContext(ctx)
			defer variable.ReleaseVariableContext(vc)

			result := vc.Expand(tt.template)
			if result != tt.expected {
				t.Errorf("Expand(%q) = %q, want %q", tt.template, result, tt.expected)
			}
		})
	}
}

// TestVariableAllBuiltins 测试所有内置变量
func TestVariableAllBuiltins(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test/path?foo=bar")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetHost("example.com")

	// 设置响应信息
	variable.SetResponseInfoInContext(ctx, 200, 1024, 50000000) // 50ms

	vc := variable.NewVariableContext(ctx)
	defer variable.ReleaseVariableContext(vc)

	// 测试所有内置变量
	builtinVars := []string{
		"host",
		"remote_addr",
		"remote_port",
		"request_uri",
		"uri",
		"args",
		"request_method",
		"scheme",
		"server_name",
		"server_port",
		"status",
		"body_bytes_sent",
		"request_time",
		"time_local",
		"time_iso8601",
		"request_id",
	}

	for _, varName := range builtinVars {
		t.Run(varName, func(t *testing.T) {
			value, ok := vc.Get(varName)
			if !ok {
				t.Errorf("builtin variable %q not found", varName)
				return
			}
			if value == "" && varName != "args" {
				t.Logf("Warning: %q is empty", varName)
			}
			t.Logf("%s = %q", varName, value)
		})
	}
}
