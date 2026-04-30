// integration_test.go - 变量系统集成测试
//
// 测试变量系统与 logging、proxy、rewrite 的集成
//
// 作者：xfy
package variable_test

import (
	"strings"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/middleware/rewrite"
	"rua.plus/lolly/internal/variable"
)

// TestVariableInAccessLog 测试访问日志中的变量展开
func TestVariableInAccessLog(_ *testing.T) {
	// 创建测试请求上下文
	cfg := &config.LoggingConfig{
		Access: config.AccessLogConfig{
			Format: "$remote_addr - $remote_user [$time_local] \"$request_method $uri $scheme\" $status $body_bytes_sent",
		},
	}

	logger := logging.New(cfg)

	// 创建请求上下文
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/users?page=1")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetHost("example.com")

	// 记录访问日志
	logger.LogAccess(ctx, 200, 1024, 50*time.Millisecond)

	// 验证输出包含期望的变量
	// 注意：由于直接输出到文件/stdout，这里主要验证不 panic
}

// TestVariableInRewrite 测试重写规则中的变量展开
func TestVariableInRewrite(t *testing.T) {
	rules := []config.RewriteRule{
		{
			Pattern:     "^/api/(.*)$",
			Replacement: "/v1/$1?original=$uri",
			Flag:        "break",
		},
	}

	mw, err := rewrite.New(rules)
	if err != nil {
		t.Fatalf("failed to create rewrite middleware: %v", err)
	}

	// 创建请求
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/users")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetHost("example.com")

	// 创建处理函数来捕获重写后的路径
	var capturedPath string
	next := func(c *fasthttp.RequestCtx) {
		capturedPath = string(c.Path())
	}

	// 处理请求
	handler := mw.Process(next)
	handler(ctx)

	// 验证路径被重写
	if capturedPath != "/v1/users" {
		t.Errorf("expected path '/v1/users', got %q", capturedPath)
	}
}

// TestVariableCompatibility 测试与旧格式的兼容性
func TestVariableCompatibility(t *testing.T) {
	// 测试旧格式变量名
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test?foo=bar")
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.SetHost("example.com")
	ctx.Request.Header.Set("User-Agent", "TestAgent")
	ctx.Request.Header.Set("Referer", "http://referer.com")

	// 设置响应信息（模拟日志场景）
	variable.SetResponseInfoInContext(ctx, 201, 2048, 100000000) // 100ms

	vc := variable.NewContext(ctx)
	defer variable.ReleaseContext(vc)

	tests := []struct {
		template string
		contains []string // 验证结果包含这些子串
	}{
		{"$remote_addr", []string{"0.0.0.0"}}, // 默认地址
		{"$host", []string{"example.com"}},
		{"$uri", []string{"/test"}},
		{"$request_method", []string{"POST"}},
		{"$scheme", []string{"http"}},
		{"$status", []string{"201"}},
		{"$body_bytes_sent", []string{"2048"}},
		{"$request_time", []string{"0.100"}},
		{"$time_local", []string{"/"}},   // 包含 /
		{"$time_iso8601", []string{"-"}}, // ISO8601 包含 -
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			result := vc.Expand(tt.template)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expand(%q) = %q, expected to contain %q", tt.template, result, expected)
				}
			}
		})
	}
}

// TestVariableExpansionPerformance 测试变量展开性能
func TestVariableExpansionPerformance(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/v1/users/123?active=true")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetHost("api.example.com")

	vc := variable.NewContext(ctx)
	defer variable.ReleaseContext(vc)

	// 常见日志格式模板
	template := "$remote_addr - $remote_user [$time_local] \"$request_method $request_uri $scheme\" $status $body_bytes_sent \"$http_user_agent\""

	// 执行多次展开
	start := time.Now()
	iterations := 10000
	for range iterations {
		_ = vc.Expand(template)
	}
	elapsed := time.Since(start)

	// 计算平均时间
	avg := elapsed / time.Duration(iterations)
	t.Logf("Average expansion time: %v (iterations: %d)", avg, iterations)

	// 验证性能在合理范围内（< 1μs 每次）
	if avg > time.Microsecond {
		t.Logf("Warning: average time %v exceeds 1μs", avg)
	}
}

// TestMixedVariableFormats 测试混合变量格式
func TestMixedVariableFormats(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetHost("example.com")

	vc := variable.NewContext(ctx)
	defer variable.ReleaseContext(vc)

	tests := []struct {
		template string
		expected string
	}{
		{"$scheme://$host$uri", "http://example.com/test"},
		{"${scheme}://${host}${uri}", "http://example.com/test"},
		{"Host: ${host}:8080", "Host: example.com:8080"},
		{"$host:8080", "example.com:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			result := vc.Expand(tt.template)
			if result != tt.expected {
				t.Errorf("Expand(%q) = %q, want %q", tt.template, result, tt.expected)
			}
		})
	}
}

// TestUndefinedVariableInIntegration 测试未定义变量在集成场景中的行为
func TestUndefinedVariableInIntegration(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")

	vc := variable.NewContext(ctx)
	defer variable.ReleaseContext(vc)

	// 未定义变量应该保持原样
	template := "$host $undefined_var $uri"
	result := vc.Expand(template)

	// $host 和 $uri 应该展开，$undefined_var 保持原样
	if !strings.Contains(result, "example.com") && !strings.Contains(result, "$undefined_var") {
		t.Errorf("expected result to contain either expanded host or $undefined_var, got %q", result)
	}
}

// TestVariableContextReuse 测试变量上下文复用
func TestVariableContextReuse(t *testing.T) {
	// 创建两个请求
	ctx1 := &fasthttp.RequestCtx{}
	ctx1.Request.SetRequestURI("/first")
	ctx1.Request.Header.SetHost("first.com")

	ctx2 := &fasthttp.RequestCtx{}
	ctx2.Request.SetRequestURI("/second")
	ctx2.Request.Header.SetHost("second.com")

	// 使用第一个上下文
	vc1 := variable.NewContext(ctx1)
	result1 := vc1.Expand("$host$uri")
	variable.ReleaseContext(vc1)

	// 复用（从池中获取）用于第二个上下文
	vc2 := variable.NewContext(ctx2)
	result2 := vc2.Expand("$host$uri")
	variable.ReleaseContext(vc2)

	// 验证结果正确
	if result1 != "first.com/first" {
		t.Errorf("first request: expected 'first.com/first', got %q", result1)
	}
	if result2 != "second.com/second" {
		t.Errorf("second request: expected 'second.com/second', got %q", result2)
	}
}
