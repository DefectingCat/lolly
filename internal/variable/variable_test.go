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

	for b.Loop() {
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

	for b.Loop() {
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

	for b.Loop() {
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

	for b.Loop() {
		_ = vc.Expand(template)
	}
}

// BenchmarkPoolGetPut 基准测试：池的 Get/Put 性能
func BenchmarkPoolGetPut(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		vc := NewContext(ctx)
		ReleaseContext(vc)
	}
}



// TestPoolReuse 测试池复用
func TestPoolReuse(t *testing.T) {
	ctx := mockRequestCtx(t)

	// 获取和释放多个 context，确保没有 panic
	for i := range 10 {
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

	// 测试 NewContext 和 ReleaseContext
	vc := NewContext(ctx)
	if vc == nil {
		t.Fatal("NewContext returned nil")
	}

	// 设置一些值
	vc.Set("test", "value")

	// 释放
	ReleaseContext(vc)

	// 再次获取应该被清空
	vc2 := NewContext(ctx)
	if _, ok := vc2.Get("test"); ok {
		t.Error("expected context to be cleared after ReleaseContext")
	}
	ReleaseContext(vc2)
}

// TestPoolPutNil 测试 ReleaseContext nil
func TestPoolPutNil(_ *testing.T) {
	// 不应该 panic
	ReleaseContext(nil)
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

	for b.Loop() {
		_, _ = vc.Get(VarUpstreamAddr)
		_, _ = vc.Get(VarUpstreamStatus)
		_, _ = vc.Get(VarUpstreamResponseTime)
	}
}







// TestEphemeralGet 测试 EphemeralGet 方法
func TestEphemeralGet(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	tests := []struct {
		name     string
		varName  string
		expected []byte
	}{
		{"host", VarHost, []byte("example.com")},
		{"uri", VarURI, []byte("/test/path")},
		{"request_uri", VarRequestURI, []byte("/test/path?foo=bar&baz=qux")},
		{"args", VarArgs, []byte("foo=bar&baz=qux")},
		{"request_method", VarRequestMethod, []byte("GET")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := vc.EphemeralGet(tt.varName)
			if string(result) != string(tt.expected) {
				t.Errorf("EphemeralGet(%q) = %q, want %q", tt.varName, result, tt.expected)
			}
		})
	}
}

// TestEphemeralGetCustomVariable 测试自定义变量的 EphemeralGet
func TestEphemeralGetCustomVariable(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 设置自定义变量
	vc.Set("custom_var", "custom_value")

	result := vc.EphemeralGet("custom_var")
	if string(result) != "custom_value" {
		t.Errorf("EphemeralGet('custom_var') = %q, want 'custom_value'", result)
	}
}

// TestEphemeralGetUndefined 测试未定义变量的 EphemeralGet
func TestEphemeralGetUndefined(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	result := vc.EphemeralGet("undefined_var")
	if result != nil {
		t.Errorf("EphemeralGet('undefined_var') = %q, want nil", result)
	}
}

// TestEphemeralGetCache 测试 EphemeralGet 的缓存
func TestEphemeralGetCache(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 第一次获取
	result1 := vc.EphemeralGet(VarHost)
	// 第二次获取应该命中缓存
	result2 := vc.EphemeralGet(VarHost)

	// 验证缓存返回相同的结果
	if string(result1) != string(result2) {
		t.Errorf("EphemeralGet cache: %q != %q", result1, result2)
	}
}

// TestPersistentGet 测试 PersistentGet 方法
func TestPersistentGet(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	tests := []struct {
		name     string
		varName  string
		expected string
	}{
		{"host", VarHost, "example.com"},
		{"uri", VarURI, "/test/path"},
		{"request_method", VarRequestMethod, "GET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := vc.PersistentGet(tt.varName)
			if result != tt.expected {
				t.Errorf("PersistentGet(%q) = %q, want %q", tt.varName, result, tt.expected)
			}
		})
	}
}

// TestPersistentGetUndefined 测试未定义变量的 PersistentGet
func TestPersistentGetUndefined(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	result := vc.PersistentGet("undefined_var")
	if result != "" {
		t.Errorf("PersistentGet('undefined_var') = %q, want empty string", result)
	}
}

// TestGetBackwardCompatibility 测试 Get 方法的向后兼容性
func TestGetBackwardCompatibility(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// Get 应该返回与 PersistentGet 相同的结果
	tests := []string{VarHost, VarURI, VarRequestMethod, VarRequestURI}

	for _, varName := range tests {
		getResult, ok := vc.Get(varName)
		if !ok {
			t.Errorf("Get(%q) returned not found", varName)
			continue
		}
		persistentResult := vc.PersistentGet(varName)
		if getResult != persistentResult {
			t.Errorf("Get(%q) = %q, PersistentGet(%q) = %q, should be equal",
				varName, getResult, varName, persistentResult)
		}
	}
}

// TestEphemeralGetUpstreamVariables 测试上游变量的 EphemeralGet
func TestEphemeralGetUpstreamVariables(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 未设置时应该返回默认值 "-"
	defaultTests := []struct {
		varName  string
		expected []byte
	}{
		{VarUpstreamAddr, []byte("-")},
		{VarUpstreamStatus, []byte("-")},
		{VarUpstreamResponseTime, []byte("-")},
	}

	for _, tt := range defaultTests {
		t.Run(tt.varName+"_default", func(t *testing.T) {
			result := vc.EphemeralGet(tt.varName)
			if string(result) != string(tt.expected) {
				t.Errorf("EphemeralGet(%q) = %q, want %q", tt.varName, result, tt.expected)
			}
		})
	}

	// 设置上游变量
	vc.SetUpstreamVars("http://backend:8080", 200, 0.123, 0.001, 0.045)

	setTests := []struct {
		varName  string
		expected []byte
	}{
		{VarUpstreamAddr, []byte("http://backend:8080")},
		{VarUpstreamStatus, []byte("200")},
		{VarUpstreamResponseTime, []byte("0.123")},
	}

	for _, tt := range setTests {
		t.Run(tt.varName+"_set", func(t *testing.T) {
			result := vc.EphemeralGet(tt.varName)
			if string(result) != string(tt.expected) {
				t.Errorf("EphemeralGet(%q) = %q, want %q", tt.varName, result, tt.expected)
			}
		})
	}
}

// TestEphemeralGetResponseInfo 测试响应信息的 EphemeralGet
func TestEphemeralGetResponseInfo(t *testing.T) {
	ctx := mockRequestCtx(t)
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 设置响应信息
	vc.SetResponseInfo(200, 1024, 15000000) // 15ms

	tests := []struct {
		varName  string
		expected []byte
	}{
		{VarStatus, []byte("200")},
		{VarBodyBytesSent, []byte("1024")},
		{VarRequestTime, []byte("0.015")},
	}

	for _, tt := range tests {
		t.Run(tt.varName, func(t *testing.T) {
			result := vc.EphemeralGet(tt.varName)
			if string(result) != string(tt.expected) {
				t.Errorf("EphemeralGet(%q) = %q, want %q", tt.varName, result, tt.expected)
			}
		})
	}
}

// BenchmarkEphemeralGet 基准测试：EphemeralGet 零拷贝性能
func BenchmarkEphemeralGet(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = vc.EphemeralGet(VarHost)
	}
}

// BenchmarkEphemeralGetCached 基准测试：EphemeralGet 缓存命中性能
func BenchmarkEphemeralGetCached(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 预热缓存
	_ = vc.EphemeralGet(VarHost)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = vc.EphemeralGet(VarHost)
	}
}

// BenchmarkPersistentGet 基准测试：PersistentGet 性能
func BenchmarkPersistentGet(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = vc.PersistentGet(VarHost)
	}
}

// BenchmarkEphemeralGetMultiple 基准测试：获取多个变量的 EphemeralGet
func BenchmarkEphemeralGetMultiple(b *testing.B) {
	ctx := setupBenchmarkRequestCtx()
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	vars := []string{VarHost, VarURI, VarRequestMethod, VarRequestURI, VarArgs}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		for _, name := range vars {
			_ = vc.EphemeralGet(name)
		}
	}
}
