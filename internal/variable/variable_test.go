// variable_test.go - 变量系统单元测试
//
// 测试覆盖：
//   - 所有内置变量求值
//   - 字符串展开（$var 和 ${var} 格式）
//   - 性能基准测试
//
// 作者：xfy
package variable

import (
	"strings"
	"testing"

	"github.com/valyala/fasthttp"
)

// mockRequestCtx 创建测试用的 fasthttp.RequestCtx
func mockRequestCtx(_ *testing.T) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}

	// 设置请求信息
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetRequestURI("/test/path?foo=bar&baz=qux")
	ctx.Request.Header.SetHost("example.com")
	ctx.Request.Header.Set("X-Custom-Header", "custom-value")
	ctx.Request.Header.Set("User-Agent", "Test-Agent/1.0")
	ctx.Request.Header.SetCookie("session", "abc123")

	return ctx
}

// TestBuiltinVariables 测试所有内置变量
func TestBuiltinVariables(t *testing.T) {
	tests := []struct {
		name     string
		varName  string
		expected string
		contains bool // 如果为 true，则检查是否包含
	}{
		{"host", VarHost, "example.com", false},
		{"uri", VarURI, "/test/path", false},
		{"request_uri", VarRequestURI, "/test/path?foo=bar&baz=qux", false},
		{"args", VarArgs, "foo=bar&baz=qux", false},
		{"request_method", VarRequestMethod, "GET", false},
		{"scheme", VarScheme, "http", false},
		{"time_iso8601", VarTimeISO8601, "", true}, // 包含格式特征
		{"time_local", VarTimeLocal, "/", true},    // 包含 /
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mockRequestCtx(t)
			vc := NewContext(ctx)
			defer ReleaseContext(vc)

			value, ok := vc.Get(tt.varName)
			if !ok && !tt.contains {
				t.Errorf("expected to find variable %s", tt.varName)
				return
			}

			if tt.contains {
				if !strings.Contains(value, tt.expected) {
					t.Errorf("%s = %q, expected to contain %q", tt.varName, value, tt.expected)
				}
			} else {
				if value != tt.expected {
					t.Errorf("%s = %q, want %q", tt.varName, value, tt.expected)
				}
			}
		})
	}
}

// TestExpandSimple 测试简单变量展开
func TestExpandSimple(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	tests := []struct {
		template string
		expected string
	}{
		{"$host", "example.com"},
		{"$uri", "/test/path"},
		{"$request_method", "GET"},
		{"$scheme", "http"},
		{"Host: $host", "Host: example.com"},
		{"$scheme://$host$uri", "http://example.com/test/path"},
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

// TestExpandBrace 测试花括号变量展开
func TestExpandBrace(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	tests := []struct {
		template string
		expected string
	}{
		{"${host}", "example.com"},
		{"${uri}", "/test/path"},
		{"Host: ${host}", "Host: example.com"},
		{"${scheme}://${host}${uri}", "http://example.com/test/path"},
		{"${host}:8080", "example.com:8080"}, // 变量后有字符
		{"pre_${uri}_post", "pre_/test/path_post"},
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

// TestExpandMixed 测试混合格式展开
func TestExpandMixed(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	tests := []struct {
		template string
		expected string
	}{
		{"$scheme://${host}$uri", "http://example.com/test/path"},
		{"${scheme}://$host${uri}", "http://example.com/test/path"},
		{"$request_method ${request_uri} HTTP/1.1", "GET /test/path?foo=bar&baz=qux HTTP/1.1"},
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

// TestExpandUndefined 测试未定义变量
func TestExpandUndefined(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	tests := []struct {
		template string
		expected string
	}{
		{"$undefined", "$undefined"},     // 保持原样
		{"${undefined}", "${undefined}"}, // 保持原样
		{"$host-$undefined", "example.com-$undefined"},
		{"$host$undefined", "example.com$undefined"},
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

// TestExpandEdgeCases 测试边界情况
func TestExpandEdgeCases(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	tests := []struct {
		template string
		expected string
	}{
		{"", ""},                           // 空字符串
		{"no variable", "no variable"},     // 无变量
		{"$", "$"},                         // 只有 $
		{"${", "${"},                       // 未闭合的 ${
		{"$123", "$123"},                   // 数字开头（不是有效变量名）
		{"test$$host", "test$example.com"}, // 双 $
		{"$host$$uri", "example.com$/test/path"},
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

// TestCustomVariable 测试自定义变量
func TestCustomVariable(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 设置自定义变量
	vc.Set("custom_var", "custom_value")
	vc.Set("app_name", "lolly")

	// 获取自定义变量
	if v, ok := vc.Get("custom_var"); !ok || v != "custom_value" {
		t.Errorf("custom_var = %q, want %q", v, "custom_value")
	}

	// 展开包含自定义变量
	result := vc.Expand("App: $app_name")
	if result != "App: lolly" {
		t.Errorf("Expand = %q, want %q", result, "App: lolly")
	}
}

// TestCustomOverridesBuiltin 测试自定义变量覆盖内置变量
func TestCustomOverridesBuiltin(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 设置同名自定义变量
	vc.Set("host", "custom.host.com")

	// 自定义变量应该覆盖内置变量
	result := vc.Expand("$host")
	if result != "custom.host.com" {
		t.Errorf("Expand = %q, want %q", result, "custom.host.com")
	}
}

// TestResponseInfoVariables 测试响应相关变量
func TestResponseInfoVariables(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 设置响应信息
	vc.SetResponseInfo(200, 1024, 15000000) // 15ms

	// 需要设置到 ctx 中才能被 builtin getter 获取
	SetResponseInfoInContext(ctx, 200, 1024, 15000000)

	tests := []struct {
		varName  string
		expected string
	}{
		{VarStatus, "200"},
		{VarBodyBytesSent, "1024"},
		{VarRequestTime, "0.015"},
	}

	for _, tt := range tests {
		t.Run(tt.varName, func(t *testing.T) {
			value, ok := vc.Get(tt.varName)
			if !ok {
				t.Errorf("expected to find variable %s", tt.varName)
				return
			}
			if value != tt.expected {
				t.Errorf("%s = %q, want %q", tt.varName, value, tt.expected)
			}
		})
	}
}

// TestExpandString 测试静态 ExpandString 函数
func TestExpandString(t *testing.T) {
	lookup := func(name string) string {
		switch name {
		case "host":
			return "example.com"
		case "port":
			return "8080"
		default:
			return ""
		}
	}

	tests := []struct {
		template string
		expected string
	}{
		{"$host:$port", "example.com:8080"},
		{"${host}:${port}", "example.com:8080"},
		{"http://$host:$port", "http://example.com:8080"},
		{"$undefined", "$undefined"}, // 未定义变量保持原样
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			result := ExpandString(tt.template, lookup)
			if result != tt.expected {
				t.Errorf("ExpandString(%q) = %q, want %q", tt.template, result, tt.expected)
			}
		})
	}
}

// TestNormalizeHeaderName 测试头名规范化
func TestNormalizeHeaderName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user_agent", "User-Agent"},
		{"content_type", "Content-Type"},
		{"x_custom_header", "X-Custom-Header"},
		{"accept", "Accept"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeHeaderName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeHeaderName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// BenchmarkExpandSimple 基准测试：简单变量展开
func BenchmarkExpandSimple(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetHost("example.com")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetRequestURI("/test")

	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	template := "$host $request_method $uri"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = vc.Expand(template)
	}
}

// BenchmarkExpandComplex 基准测试：复杂模板展开
func BenchmarkExpandComplex(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetHost("example.com")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetRequestURI("/api/v1/users?page=1&limit=10")

	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 模拟日志格式
	template := "$remote_addr - - [$time_local] \"$request_method $request_uri HTTP/1.1\" $status $body_bytes_sent"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = vc.Expand(template)
	}
}

// BenchmarkExpandNoVariable 基准测试：无变量字符串
func BenchmarkExpandNoVariable(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	template := "This is a static string without any variables"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = vc.Expand(template)
	}
}

// BenchmarkExpandBrace 基准测试：花括号变量
func BenchmarkExpandBrace(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetHost("example.com")
	ctx.Request.Header.SetMethod("GET")

	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	template := "${scheme}://${host}${uri}"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = vc.Expand(template)
	}
}

// BenchmarkPoolGetPut 基准测试：池的 Get/Put 性能
func BenchmarkPoolGetPut(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		vc := NewContext(ctx)
		ReleaseContext(vc)
	}
}

// BenchmarkExpandStringStatic 基准测试：静态 ExpandString 函数
func BenchmarkExpandStringStatic(b *testing.B) {
	lookup := func(name string) string {
		switch name {
		case "host":
			return "example.com"
		case "uri":
			return "/test"
		default:
			return ""
		}
	}

	template := "$host $uri"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ExpandString(template, lookup)
	}
}

// TestPoolReuse 测试池复用
func TestPoolReuse(t *testing.T) {
	ctx := mockRequestCtx(t)

	// 获取和释放多个 context，确保没有 panic
	for i := 0; i < 10; i++ {
		vc := NewContext(ctx)
		vc.Set("key", "value")
		if v, ok := vc.Get("key"); !ok || v != "value" {
			t.Errorf("iteration %d: expected key=value, got %s", i, v)
		}
		ReleaseContext(vc)
	}

	// 验证池在复用（第二次获取应该清除之前的值）
	vc2 := NewContext(ctx)
	if v, ok := vc2.Get("key"); ok {
		t.Errorf("expected key to be cleared after release, got %s", v)
	}
	ReleaseContext(vc2)
}

// TestMoreBuiltinVariables 测试更多内置变量
func TestMoreBuiltinVariables(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(*fasthttp.RequestCtx)
		varName     string
		expected    string
		shouldExist bool
	}{
		{
			name:        "server_port with local addr",
			setupFunc:   func(_ *fasthttp.RequestCtx) {},
			varName:     VarServerPort,
			expected:    "0", // 没有设置 local addr 时返回 "0"
			shouldExist: true,
		},
		{
			name:        "remote_addr without addr",
			setupFunc:   func(_ *fasthttp.RequestCtx) {},
			varName:     VarRemoteAddr,
			expected:    "0.0.0.0:0", // mock ctx 返回默认值
			shouldExist: true,
		},
		{
			name:        "remote_port without addr",
			setupFunc:   func(_ *fasthttp.RequestCtx) {},
			varName:     VarRemotePort,
			expected:    "0",
			shouldExist: true,
		},
		{
			name: "request_id from context",
			setupFunc: func(ctx *fasthttp.RequestCtx) {
				SetRequestIDInContext(ctx, "test-request-id-123")
			},
			varName:     VarRequestID,
			expected:    "test-request-id-123",
			shouldExist: true,
		},
		{
			name: "server_name from context",
			setupFunc: func(ctx *fasthttp.RequestCtx) {
				SetServerNameInContext(ctx, "test-server")
			},
			varName:     VarServerName,
			expected:    "test-server",
			shouldExist: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := mockRequestCtx(t)
			tt.setupFunc(ctx)
			vc := NewContext(ctx)
			defer ReleaseContext(vc)

			value, ok := vc.Get(tt.varName)
			if tt.shouldExist && !ok {
				t.Errorf("expected variable %s to exist", tt.varName)
				return
			}
			if value != tt.expected {
				t.Errorf("%s = %q, want %q", tt.varName, value, tt.expected)
			}
		})
	}
}

// TestReleaseNilContext 测试释放 nil context
func TestReleaseNilContext(_ *testing.T) {
	// 不应该 panic
	ReleaseContext(nil)
}

// TestGetBuiltin 测试获取内置变量定义
func TestGetBuiltin(t *testing.T) {
	// 存在的变量
	v := GetBuiltin("host")
	if v == nil || v.Name != "host" {
		t.Error("GetBuiltin('host') should return non-nil with name 'host'")
	}

	// 不存在的变量
	v = GetBuiltin("nonexistent")
	if v != nil {
		t.Error("GetBuiltin('nonexistent') should return nil")
	}
}

// TestGetArgVariable 测试查询参数变量
func TestGetArgVariable(t *testing.T) {
	ctx := mockRequestCtx(t) // /test/path?foo=bar&baz=qux

	tests := []struct {
		name     string
		expected string
	}{
		{"foo", "bar"},
		{"baz", "qux"},
		{"notexist", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetArgVariable(ctx, tt.name)
			if result != tt.expected {
				t.Errorf("GetArgVariable(%q) = %q, want %q", tt.name, result, tt.expected)
			}
		})
	}
}

// TestGetHTTPVariable 测试 HTTP 头变量
func TestGetHTTPVariable(t *testing.T) {
	ctx := mockRequestCtx(t)

	tests := []struct {
		name     string
		expected string
	}{
		{"user_agent", "Test-Agent/1.0"},
		{"x_custom_header", "custom-value"},
		{"not_exist", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetHTTPVariable(ctx, tt.name)
			if result != tt.expected {
				t.Errorf("GetHTTPVariable(%q) = %q, want %q", tt.name, result, tt.expected)
			}
		})
	}
}

// TestGetCookieVariable 测试 Cookie 变量
func TestGetCookieVariable(t *testing.T) {
	ctx := mockRequestCtx(t) // session=abc123

	tests := []struct {
		name     string
		expected string
	}{
		{"session", "abc123"},
		{"notexist", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCookieVariable(ctx, tt.name)
			if result != tt.expected {
				t.Errorf("GetCookieVariable(%q) = %q, want %q", tt.name, result, tt.expected)
			}
		})
	}
}

// TestEmptyTemplate 测试空模板
func TestEmptyTemplate(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	result := vc.Expand("")
	if result != "" {
		t.Errorf("Expand('') = %q, want empty string", result)
	}
}

// TestReleaseContextWithNil 测试释放 nil
func TestReleaseContextWithNil(_ *testing.T) {
	// 不应该 panic
	ReleaseContext(nil)
}

// TestExpandOnlyDollar 测试只有 $ 的情况
func TestExpandOnlyDollar(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	tests := []struct {
		template string
		expected string
	}{
		{"$", "$"},
		{"test$", "test$"},
		{"$$", "$$"}, // 两个独立的 $
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

// TestPoolFunctions 测试 Pool 相关函数
func TestPoolFunctions(t *testing.T) {
	ctx := mockRequestCtx(t)

	// 测试 PoolGet 和 PoolPut
	vc := PoolGet(ctx)
	if vc == nil {
		t.Fatal("PoolGet returned nil")
	}

	// 设置一些值
	vc.Set("test", "value")

	// 释放
	PoolPut(vc)

	// 再次获取应该被清空
	vc2 := PoolGet(ctx)
	if _, ok := vc2.Get("test"); ok {
		t.Error("expected context to be cleared after PoolPut")
	}
	PoolPut(vc2)
}

// TestPoolPutNil 测试 PoolPut nil
func TestPoolPutNil(_ *testing.T) {
	// 不应该 panic
	PoolPut(nil)
}

// TestStatsFunctions 测试统计相关函数
func TestStatsFunctions(t *testing.T) {
	// 重置统计
	ResetStats()

	// 获取初始统计
	stats := GetStats()
	if stats.Gets != 0 || stats.Puts != 0 {
		t.Error("expected empty stats after reset")
	}

	// 获取池
	p := GetPool()
	if p == nil {
		t.Error("GetPool() should return non-nil")
	}
}

// TestSetResponseInfo 测试 SetResponseInfo
func TestSetResponseInfo(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 设置响应信息
	vc.SetResponseInfo(404, 512, 25000000) // 25ms

	if vc.status != 404 {
		t.Errorf("status = %d, want 404", vc.status)
	}
	if vc.bodySize != 512 {
		t.Errorf("bodySize = %d, want 512", vc.bodySize)
	}
	if vc.duration != 25000000 {
		t.Errorf("duration = %d, want 25000000", vc.duration)
	}
}

// TestSetServerName 测试 SetServerName
func TestSetServerName(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	vc.SetServerName("my-server")
	if vc.serverName != "my-server" {
		t.Errorf("serverName = %q, want 'my-server'", vc.serverName)
	}
}

// TestEmptyExpandString 测试空模板 ExpandString
func TestEmptyExpandString(t *testing.T) {
	lookup := func(_ string) string { return "" }
	result := ExpandString("", lookup)
	if result != "" {
		t.Errorf("ExpandString('') = %q, want empty", result)
	}
}

// TestExpandStringNoVar 测试无变量模板
func TestExpandStringNoVar(t *testing.T) {
	lookup := func(_ string) string { return "" }
	result := ExpandString("hello world", lookup)
	if result != "hello world" {
		t.Errorf("ExpandString = %q, want 'hello world'", result)
	}
}

// TestTLSBuiltin 测试 HTTPS/TLS 内置变量
func TestTLSBuiltin(t *testing.T) {
	// 创建带 TLS 的上下文
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetRequestURI("/test")
	// 由于无法直接设置 TLS，scheme 变量会检查 ctx.IsTLS()
	// 这里我们测试它返回 http（默认值）

	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	scheme, ok := vc.Get("scheme")
	if !ok {
		t.Error("expected 'scheme' variable to exist")
	}
	if scheme != "http" {
		t.Errorf("scheme = %q, want 'http'", scheme)
	}
}

// TestEmptyVarNameBrace 测试空变量名 ${}
func TestEmptyVarNameBrace(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// ${} 应该保持为 ${}
	result := vc.Expand("${}")
	if result != "${}" {
		t.Errorf("Expand('${}') = %q, want '${}'", result)
	}
}

func TestBuiltinVarNames(t *testing.T) {
	names := BuiltinVarNames()
	if len(names) == 0 {
		t.Error("BuiltinVarNames() returned empty slice")
	}

	// 检查是否包含一些已知变量
	hasVar := func(name string) bool {
		for _, n := range names {
			if n == name {
				return true
			}
		}
		return false
	}

	if !hasVar("host") {
		t.Error("BuiltinVarNames() missing 'host'")
	}
	if !hasVar("uri") {
		t.Error("BuiltinVarNames() missing 'uri'")
	}
	if !hasVar("remote_addr") {
		t.Error("BuiltinVarNames() missing 'remote_addr'")
	}
}

// TestUpstreamVariables 测试上游变量
func TestUpstreamVariables(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 未设置时应该返回默认值 "-"
	tests := []struct {
		varName  string
		expected string
	}{
		{VarUpstreamAddr, "-"},
		{VarUpstreamStatus, "-"},
		{VarUpstreamResponseTime, "-"},
		{VarUpstreamConnectTime, "-"},
		{VarUpstreamHeaderTime, "-"},
	}

	for _, tt := range tests {
		t.Run(tt.varName+"_default", func(t *testing.T) {
			value, ok := vc.Get(tt.varName)
			if !ok {
				t.Errorf("expected variable %s to exist", tt.varName)
				return
			}
			if value != tt.expected {
				t.Errorf("%s = %q, want %q", tt.varName, value, tt.expected)
			}
		})
	}

	// 设置上游变量
	vc.SetUpstreamVars("http://backend:8080", 200, 0.123, 0.001, 0.045)

	// 验证设置后的值
	testsAfter := []struct {
		varName  string
		expected string
	}{
		{VarUpstreamAddr, "http://backend:8080"},
		{VarUpstreamStatus, "200"},
		{VarUpstreamResponseTime, "0.123"},
		{VarUpstreamConnectTime, "0.001"},
		{VarUpstreamHeaderTime, "0.045"},
	}

	for _, tt := range testsAfter {
		t.Run(tt.varName+"_set", func(t *testing.T) {
			value, ok := vc.Get(tt.varName)
			if !ok {
				t.Errorf("expected variable %s to exist", tt.varName)
				return
			}
			if value != tt.expected {
				t.Errorf("%s = %q, want %q", tt.varName, value, tt.expected)
			}
		})
	}
}

// TestUpstreamVariablesInExpand 测试在模板中展开上游变量
func TestUpstreamVariablesInExpand(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 设置上游变量
	vc.SetUpstreamVars("http://backend:8080", 200, 0.123, 0.001, 0.045)

	// 测试展开
	template := "$upstream_addr $upstream_status $upstream_response_time"
	result := vc.Expand(template)
	expected := "http://backend:8080 200 0.123"
	if result != expected {
		t.Errorf("Expand = %q, want %q", result, expected)
	}
}

// TestUpstreamVariablesErrorCases 测试上游变量错误情况
func TestUpstreamVariablesErrorCases(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 测试各种错误场景
	tests := []struct {
		expected map[string]string
		name     string
		addr     string
		status   int
	}{
		{
			name:   "no backend",
			addr:   "FAILED",
			status: 502,
			expected: map[string]string{
				VarUpstreamAddr:   "FAILED",
				VarUpstreamStatus: "502",
			},
		},
		{
			name:   "timeout",
			addr:   "http://backend:8080",
			status: 504,
			expected: map[string]string{
				VarUpstreamAddr:   "http://backend:8080",
				VarUpstreamStatus: "504",
			},
		},
		{
			name:   "cache hit",
			addr:   "CACHE",
			status: 200,
			expected: map[string]string{
				VarUpstreamAddr:   "CACHE",
				VarUpstreamStatus: "200",
			},
		},
		{
			name:   "websocket success",
			addr:   "ws://backend:8080",
			status: 101,
			expected: map[string]string{
				VarUpstreamAddr:   "ws://backend:8080",
				VarUpstreamStatus: "101",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vc.SetUpstreamVars(tt.addr, tt.status, 0, 0, 0)

			for varName, expected := range tt.expected {
				value, ok := vc.Get(varName)
				if !ok {
					t.Errorf("expected variable %s to exist", varName)
					continue
				}
				if value != expected {
					t.Errorf("%s = %q, want %q", varName, value, expected)
				}
			}
		})
	}
}

// TestUpstreamVariablesZeroValues 测试上游变量零值处理
func TestUpstreamVariablesZeroValues(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 测试零值应该返回 "-"
	vc.SetUpstreamVars("", 0, 0, 0, 0)

	tests := []struct {
		varName  string
		expected string
	}{
		{VarUpstreamAddr, "-"},
		{VarUpstreamStatus, "-"},
		{VarUpstreamResponseTime, "-"},
		{VarUpstreamConnectTime, "-"},
		{VarUpstreamHeaderTime, "-"},
	}

	for _, tt := range tests {
		t.Run(tt.varName, func(t *testing.T) {
			value, ok := vc.Get(tt.varName)
			if !ok {
				t.Errorf("expected variable %s to exist", tt.varName)
				return
			}
			if value != tt.expected {
				t.Errorf("%s = %q, want %q", tt.varName, value, tt.expected)
			}
		})
	}
}

// BenchmarkUpstreamVariables 基准测试：上游变量
func BenchmarkUpstreamVariables(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetHost("example.com")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.SetRequestURI("/test")

	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 设置上游变量
	vc.SetUpstreamVars("http://backend:8080", 200, 0.123, 0.001, 0.045)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = vc.Get(VarUpstreamAddr)
		_, _ = vc.Get(VarUpstreamStatus)
		_, _ = vc.Get(VarUpstreamResponseTime)
	}
}

// TestGlobalVariables 测试全局变量功能
func TestGlobalVariables(t *testing.T) {
	// 清理
	SetGlobalVariables(nil)

	// 测试设置全局变量
	SetGlobalVariables(map[string]string{
		"app_name": "lolly",
		"version":  "1.0.0",
	})

	// 测试 GetGlobalVariable
	if v, ok := GetGlobalVariable("app_name"); !ok || v != "lolly" {
		t.Errorf("GetGlobalVariable('app_name') = %q, %v, want 'lolly', true", v, ok)
	}

	if v, ok := GetGlobalVariable("notexist"); ok {
		t.Errorf("GetGlobalVariable('notexist') = %q, %v, want '', false", v, ok)
	}

	// 测试 GetAllGlobalVariables
	globals := GetAllGlobalVariables()
	if globals == nil {
		t.Error("GetAllGlobalVariables() returned nil")
	}
	if globals["app_name"] != "lolly" {
		t.Errorf("globals['app_name'] = %q, want 'lolly'", globals["app_name"])
	}

	// 测试返回副本而非引用
	globals["app_name"] = "modified"
	if v, _ := GetGlobalVariable("app_name"); v != "lolly" {
		t.Error("GetAllGlobalVariables() should return a copy, not a reference")
	}

	// 清理
	SetGlobalVariables(nil)
}

// TestNewContextWithGlobals 测试全局变量注入到请求上下文
func TestNewContextWithGlobals(t *testing.T) {
	// 设置全局变量
	SetGlobalVariables(map[string]string{
		"global_var": "global_value",
	})
	defer SetGlobalVariables(nil)

	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 全局变量应该被注入
	if v, ok := vc.Get("global_var"); !ok || v != "global_value" {
		t.Errorf("Get('global_var') = %q, %v, want 'global_value', true", v, ok)
	}

	// 展开应该包含全局变量
	result := vc.Expand("$global_var")
	if result != "global_value" {
		t.Errorf("Expand('$global_var') = %q, want 'global_value'", result)
	}
}

// TestGlobalVariablesConcurrent 测试全局变量并发访问
func TestGlobalVariablesConcurrent(_ *testing.T) {
	SetGlobalVariables(map[string]string{
		"counter": "0",
	})
	defer SetGlobalVariables(nil)

	done := make(chan bool)

	// 并发读取
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _ = GetGlobalVariable("counter")
			}
			done <- true
		}()
	}

	// 并发写入
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 50; j++ {
				SetGlobalVariables(map[string]string{"counter": "updated"})
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 15; i++ {
		<-done
	}
}
