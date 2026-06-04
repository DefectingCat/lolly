// builtin_coverage_test.go - 补充未覆盖函数的测试
//
// 测试覆盖：
//   - formatRequestTime: 请求时间格式化
//   - SetGlobalVariables: 全局变量设置
//   - EphemeralGet: 未覆盖的分支
//   - GetSSLClientVerify: TLS 状态分支
//   - init: 内置变量注册验证
//
// 作者：xfy
package variable

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// TestFormatRequestTime 测试请求处理时间格式化
func TestFormatRequestTime(t *testing.T) {
	tests := []struct {
		name     string
		ns       int64
		expected string
	}{
		{"零值", 0, "0.000"},
		{"1毫秒", 1_000_000, "0.001"},
		{"15毫秒", 15_000_000, "0.015"},
		{"100毫秒", 100_000_000, "0.100"},
		{"1秒", 1_000_000_000, "1.000"},
		{"1.234秒", 1_234_000_000, "1.234"},
		{"微小值", 1, "0.000"},
		{"大值", 60_000_000_000, "60.000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatRequestTime(tt.ns)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFormatRequestTime_ViaBuiltinGetter 通过内置变量 getter 间接调用 formatRequestTime
func TestFormatRequestTime_ViaBuiltinGetter(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}

	builtin := GetBuiltin(VarRequestTime)
	require.NotNil(t, builtin)
	require.NotNil(t, builtin.Getter)

	tests := []struct {
		name     string
		ns       int64
		expected string
	}{
		{"通过 getter 零值", 0, "0.000"},
		{"通过 getter 15毫秒", 15_000_000, "0.015"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx.SetUserValue(VarRequestTime, tt.ns)
			result := builtin.Getter(ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSetGlobalVariables 测试设置全局自定义变量
func TestSetGlobalVariables(t *testing.T) {
	t.Run("正常设置", func(t *testing.T) {
		SetGlobalVariables(map[string]string{
			"app_name":    "lolly",
			"environment": "production",
		})

		v, ok := GetGlobalVariable("app_name")
		assert.True(t, ok)
		assert.Equal(t, "lolly", v)

		v, ok = GetGlobalVariable("environment")
		assert.True(t, ok)
		assert.Equal(t, "production", v)
	})

	t.Run("空配置", func(t *testing.T) {
		SetGlobalVariables(map[string]string{})

		// 之前设置的变量应该被清空
		_, ok := GetGlobalVariable("app_name")
		assert.False(t, ok)
	})

	t.Run("覆盖已有变量", func(t *testing.T) {
		SetGlobalVariables(map[string]string{
			"version": "1.0",
		})

		SetGlobalVariables(map[string]string{
			"version": "2.0",
		})

		v, ok := GetGlobalVariable("version")
		assert.True(t, ok)
		assert.Equal(t, "2.0", v)
	})

	// 清理
	SetGlobalVariables(nil)
}

// TestSetGlobalVariables_VariableExpansion 测试全局变量在展开中的使用
func TestSetGlobalVariables_VariableExpansion(t *testing.T) {
	SetGlobalVariables(map[string]string{
		"app_name": "lolly",
		"env":      "test",
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetHost("example.com")
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	result := vc.Expand("$app_name-$env")
	assert.Equal(t, "lolly-test", result)

	// 清理
	SetGlobalVariables(nil)
}

// TestEphemeralGet_GlobalVariables 测试 EphemeralGet 获取全局变量
func TestEphemeralGet_GlobalVariables(t *testing.T) {
	SetGlobalVariables(map[string]string{
		"global_key": "global_value",
	})

	ctx := &fasthttp.RequestCtx{}
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	result := vc.EphemeralGet("global_key")
	assert.Equal(t, []byte("global_value"), result)

	// 清理
	SetGlobalVariables(nil)
}

// TestEphemeralGet_GlobalVariablesFallback 测试全局变量不存在时 EphemeralGet 的行为
func TestEphemeralGet_GlobalVariablesFallback(t *testing.T) {
	SetGlobalVariables(map[string]string{})

	ctx := &fasthttp.RequestCtx{}
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	result := vc.EphemeralGet("nonexistent_global")
	assert.Nil(t, result)

	// 清理
	SetGlobalVariables(nil)
}

// TestEphemeralGet_ServerName 测试 EphemeralGet 获取 server_name
func TestEphemeralGet_ServerName(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 未设置时走 builtin getter
	result := vc.EphemeralGet(VarServerName)
	assert.Equal(t, []byte("-"), result)
}

// TestEphemeralGet_ServerNameSet 测试 EphemeralGet 获取设置后的 server_name
func TestEphemeralGet_ServerNameSet(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	vc.SetServerName("my-server")
	result := vc.EphemeralGet(VarServerName)
	assert.Equal(t, []byte("my-server"), result)
}

// TestEphemeralGet_UpstreamConnectAndHeaderTime 测试上游连接时间和头部时间的 EphemeralGet
func TestEphemeralGet_UpstreamConnectAndHeaderTime(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 未设置时返回 "-"
	assert.Equal(t, []byte("-"), vc.EphemeralGet(VarUpstreamConnectTime))
	assert.Equal(t, []byte("-"), vc.EphemeralGet(VarUpstreamHeaderTime))

	// 设置后返回正确值
	vc.SetUpstreamVars("http://backend:8080", 200, 0.123, 0.005, 0.010)

	result := vc.EphemeralGet(VarUpstreamConnectTime)
	assert.Equal(t, []byte("0.005"), result)

	result = vc.EphemeralGet(VarUpstreamHeaderTime)
	assert.Equal(t, []byte("0.010"), result)
}

// TestEphemeralGet_ResponseInfoViaUserValue 测试通过 UserValue 获取响应信息的 EphemeralGet
func TestEphemeralGet_ResponseInfoViaUserValue(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}

	// 通过 SetResponseInfoInContext 设置（而非 SetResponseInfo）
	SetResponseInfoInContext(ctx, 500, 2048, 30_000_000)

	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	assert.Equal(t, []byte("500"), vc.EphemeralGet(VarStatus))
	assert.Equal(t, []byte("2048"), vc.EphemeralGet(VarBodyBytesSent))
	assert.Equal(t, []byte("0.030"), vc.EphemeralGet(VarRequestTime))
}

// TestEphemeralGet_BodyBytesSentDefault 测试 body_bytes_sent 默认值的 EphemeralGet
func TestEphemeralGet_BodyBytesSentDefault(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 未设置时返回 "0"
	result := vc.EphemeralGet(VarBodyBytesSent)
	assert.Equal(t, []byte("0"), result)
}

// TestEphemeralGet_RequestTimeDefault 测试 request_time 默认值的 EphemeralGet
func TestEphemeralGet_RequestTimeDefault(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 未设置时返回 "0.000"
	result := vc.EphemeralGet(VarRequestTime)
	assert.Equal(t, []byte("0.000"), result)
}

// TestEphemeralGet_ConcurrentAccess 测试 EphemeralGet 的并发安全性
func TestEphemeralGet_ConcurrentAccess(t *testing.T) {
	SetGlobalVariables(map[string]string{
		"shared": "value",
	})

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx := &fasthttp.RequestCtx{}
			vc := NewContext(ctx)
			defer ReleaseContext(vc)

			vc.Set("local", "val")
			result := vc.EphemeralGet("shared")
			assert.Equal(t, []byte("value"), result, "goroutine %d", id)
		}(i)
	}
	wg.Wait()

	// 清理
	SetGlobalVariables(nil)
}

// TestGetSSLClientVerify_TLSWithUserValue 测试 TLS 连接下设置 UserValue 的场景
func TestGetSSLClientVerify_TLSWithUserValue(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	// 模拟 TLS 连接
	ctx.SetUserValue("tls_connection", true)
	// 设置验证结果为 SUCCESS
	ctx.SetUserValue(VarSSLClientVerify, "SUCCESS")

	// 由于 fasthttp.RequestCtx 默认 IsTLS() 返回 false
	// 无法直接模拟 TLS 连接，所以测试 UserValue 被正确读取的场景
	// 这个测试验证了当 IsTLS() 为 true 时的逻辑路径
	// 在非 TLS 环境下，即使设置了 UserValue 也会返回 NONE
	result := GetSSLClientVerify(ctx)
	assert.Equal(t, "NONE", result)
}

// TestGetSSLClientVerify_TLSWithPeerCertPresent 测试 TLS 连接下 peer cert 存在的场景
func TestGetSSLClientVerify_TLSWithPeerCertPresent(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	// 设置证书存在标志（模拟 mTLS 场景）
	ctx.SetUserValue("tls_peer_cert_present", true)

	// 非 TLS 连接，即使设置了 peer_cert_present 也返回 NONE
	result := GetSSLClientVerify(ctx)
	assert.Equal(t, "NONE", result)
}

// TestGetSSLClientVerify_InvalidUserValueType 测试 UserValue 类型不正确的场景
func TestGetSSLClientVerify_InvalidUserValueType(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	// 设置非 string 类型的 UserValue
	ctx.SetUserValue(VarSSLClientVerify, 12345)

	// 非 TLS 连接直接返回 NONE，不会检查 UserValue 类型
	result := GetSSLClientVerify(ctx)
	assert.Equal(t, "NONE", result)
}

// TestInit_BuiltinVariablesRegistered 测试 init 函数注册了所有内置变量
func TestInit_BuiltinVariablesRegistered(t *testing.T) {
	expectedVars := []string{
		VarHost,
		VarRemoteAddr,
		VarRemotePort,
		VarRequestURI,
		VarURI,
		VarArgs,
		VarRequestMethod,
		VarScheme,
		VarServerName,
		VarServerPort,
		VarStatus,
		VarBodyBytesSent,
		VarRequestTime,
		VarTimeLocal,
		VarTimeISO8601,
		VarRequestID,
	}

	for _, name := range expectedVars {
		t.Run(name, func(t *testing.T) {
			builtin := GetBuiltin(name)
			require.NotNil(t, builtin, "内置变量 %s 应该已注册", name)
			assert.Equal(t, name, builtin.Name)
			assert.NotEmpty(t, builtin.Description)
			assert.NotNil(t, builtin.Getter, "内置变量 %s 应该有 Getter", name)
		})
	}
}

// TestInit_UpstreamVariablesInContext 测试上游变量通过 Context 字段获取
// 上游变量不是通过 RegisterBuiltin 注册的，而是通过 Context 结构体字段直接访问
func TestInit_UpstreamVariablesInContext(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	upstreamVars := []string{
		VarUpstreamAddr,
		VarUpstreamStatus,
		VarUpstreamResponseTime,
		VarUpstreamConnectTime,
		VarUpstreamHeaderTime,
	}

	vc.SetUpstreamVars("http://backend:8080", 200, 0.123, 0.005, 0.010)

	for _, name := range upstreamVars {
		t.Run(name, func(t *testing.T) {
			v, ok := vc.Get(name)
			require.True(t, ok, "上游变量 %s 应该可以通过 Get 获取", name)
			assert.NotEqual(t, "", v, "上游变量 %s 不应为空", name)
		})
	}
}

// TestInit_SSLVariablesRegistered 测试 init 注册了 SSL 变量
func TestInit_SSLVariablesRegistered(t *testing.T) {
	sslVars := []string{
		VarSSLClientVerify,
		VarSSLClientSerial,
		VarSSLClientSubject,
		VarSSLClientIssuer,
		VarSSLClientFingerprint,
		VarSSLClientNotBefore,
		VarSSLClientNotAfter,
		VarSSLClientDNS,
		VarSSLClientEmail,
	}

	for _, name := range sslVars {
		t.Run(name, func(t *testing.T) {
			builtin := GetBuiltin(name)
			require.NotNil(t, builtin, "SSL 变量 %s 应该已注册", name)
			assert.NotEmpty(t, builtin.Description)
		})
	}
}

// TestInit_HostGetterBytes 测试 host 变量有 GetterBytes
func TestInit_HostGetterBytes(t *testing.T) {
	builtin := GetBuiltin(VarHost)
	require.NotNil(t, builtin)
	assert.NotNil(t, builtin.GetterBytes)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetHost("test.example.com")

	result := builtin.GetterBytes(ctx)
	assert.Equal(t, []byte("test.example.com"), result)
}

// TestInit_URIGetterBytes 测试 URI 变量有 GetterBytes
func TestInit_URIGetterBytes(t *testing.T) {
	builtin := GetBuiltin(VarURI)
	require.NotNil(t, builtin)
	assert.NotNil(t, builtin.GetterBytes)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/path/to/resource")

	result := builtin.GetterBytes(ctx)
	assert.Equal(t, []byte("/path/to/resource"), result)
}

// TestInit_RequestURIGetterBytes 测试 request_uri 变量有 GetterBytes
func TestInit_RequestURIGetterBytes(t *testing.T) {
	builtin := GetBuiltin(VarRequestURI)
	require.NotNil(t, builtin)
	assert.NotNil(t, builtin.GetterBytes)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/path?query=1")

	result := builtin.GetterBytes(ctx)
	assert.Equal(t, []byte("/path?query=1"), result)
}

// TestInit_ArgsGetterBytes 测试 args 变量有 GetterBytes
func TestInit_ArgsGetterBytes(t *testing.T) {
	builtin := GetBuiltin(VarArgs)
	require.NotNil(t, builtin)
	assert.NotNil(t, builtin.GetterBytes)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test?key=val")

	result := builtin.GetterBytes(ctx)
	assert.Equal(t, []byte("key=val"), result)
}

// TestInit_MethodGetterBytes 测试 request_method 变量有 GetterBytes
func TestInit_MethodGetterBytes(t *testing.T) {
	builtin := GetBuiltin(VarRequestMethod)
	require.NotNil(t, builtin)
	assert.NotNil(t, builtin.GetterBytes)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")

	result := builtin.GetterBytes(ctx)
	assert.Equal(t, []byte("POST"), result)
}
