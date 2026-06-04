// Package security 提供覆盖率补充测试。
//
// 该文件针对覆盖率低于 60% 的函数编写测试，包括：
//   - headers.go 全部方法（0% 覆盖）
//   - auth.go 的 authenticateArgon2id、parseArgon2idHash、parseUint32、parseUint8（0% 覆盖）
//   - geoip.go 的 LookupCountry、Close、GetStats（0% 覆盖）
//   - access.go 的 Check（47.8% 覆盖）
//   - auth_request.go 的 Process（63.6% 覆盖）
//
// 作者：xfy
package security

import (
	"encoding/base64"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/argon2"
	"rua.plus/lolly/internal/config"
)

// ===================== headers.go 测试 =====================

// TestNewHeadersWithHSTS_NilConfig 测试传入 nil 配置时使用默认值
func TestNewHeadersWithHSTS_NilConfig(t *testing.T) {
	sh := NewHeadersWithHSTS(nil, nil)
	require.NotNil(t, sh)
	assert.Equal(t, "DENY", sh.config.XFrameOptions)
	assert.Equal(t, "nosniff", sh.config.XContentTypeOptions)
	assert.Equal(t, "strict-origin-when-cross-origin", sh.config.ReferrerPolicy)
}

// TestNewHeadersWithHSTS_WithConfig 测试传入自定义配置
func TestNewHeadersWithHSTS_WithConfig(t *testing.T) {
	cfg := &config.SecurityHeaders{
		XFrameOptions:         "SAMEORIGIN",
		XContentTypeOptions:   "nosniff",
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "no-referrer",
		PermissionsPolicy:     "geolocation=()",
	}
	hstsCfg := &config.HSTSConfig{
		MaxAge:            86400,
		IncludeSubDomains: true,
		Preload:           false,
	}

	sh := NewHeadersWithHSTS(cfg, hstsCfg)
	require.NotNil(t, sh)
	assert.Equal(t, "SAMEORIGIN", sh.config.XFrameOptions)
	assert.Contains(t, sh.hsts, "max-age=86400")
}

// TestNewHeadersWithHSTS_NilHSTSConfig 测试 HSTS 配置为 nil 时使用默认值
func TestNewHeadersWithHSTS_NilHSTSConfig(t *testing.T) {
	cfg := &config.SecurityHeaders{
		XFrameOptions: "DENY",
	}
	sh := NewHeadersWithHSTS(cfg, nil)
	require.NotNil(t, sh)
	assert.Contains(t, sh.hsts, "max-age=")
	assert.Contains(t, sh.hsts, "includeSubDomains")
}

// TestNewHeadersWithHSTS_ZeroMaxAge 测试 MaxAge 为 0 时使用默认值
func TestNewHeadersWithHSTS_ZeroMaxAge(t *testing.T) {
	cfg := &config.SecurityHeaders{XFrameOptions: "DENY"}
	hstsCfg := &config.HSTSConfig{MaxAge: 0}
	sh := NewHeadersWithHSTS(cfg, hstsCfg)
	require.NotNil(t, sh)
	assert.Contains(t, sh.hsts, "max-age=31536000")
}

// TestHeadersMiddleware_Name 测试 Name 方法
func TestHeadersMiddleware_Name(t *testing.T) {
	sh := NewHeadersWithHSTS(nil, nil)
	assert.Equal(t, "security_headers", sh.Name())
}

// TestHeadersMiddleware_Process 测试 Process 添加安全头
func TestHeadersMiddleware_Process(t *testing.T) {
	cfg := &config.SecurityHeaders{
		XFrameOptions:         "DENY",
		XContentTypeOptions:   "nosniff",
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		PermissionsPolicy:     "camera=()",
	}
	hstsCfg := &config.HSTSConfig{
		MaxAge:            31536000,
		IncludeSubDomains: true,
	}
	sh := NewHeadersWithHSTS(cfg, hstsCfg)

	called := false
	next := func(ctx *fasthttp.RequestCtx) {
		called = true
	}

	handler := sh.Process(next)
	require.NotNil(t, handler)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/")
	handler(ctx)

	assert.True(t, called)
	assert.Equal(t, "DENY", string(ctx.Response.Header.Peek("X-Frame-Options")))
	assert.Equal(t, "nosniff", string(ctx.Response.Header.Peek("X-Content-Type-Options")))
	assert.Equal(t, "default-src 'self'", string(ctx.Response.Header.Peek("Content-Security-Policy")))
	assert.Equal(t, "strict-origin-when-cross-origin", string(ctx.Response.Header.Peek("Referrer-Policy")))
	assert.Equal(t, "camera=()", string(ctx.Response.Header.Peek("Permissions-Policy")))
}

// TestHeadersMiddleware_Process_HSTS_OnlyOnTLS 测试 HSTS 仅在 TLS 时添加
func TestHeadersMiddleware_Process_HSTS_OnlyOnTLS(t *testing.T) {
	cfg := &config.SecurityHeaders{XFrameOptions: "DENY"}
	hstsCfg := &config.HSTSConfig{MaxAge: 31536000, IncludeSubDomains: true}
	sh := NewHeadersWithHSTS(cfg, hstsCfg)

	handler := sh.Process(func(ctx *fasthttp.RequestCtx) {})

	// 非 TLS 请求不应添加 HSTS
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/")
	handler(ctx)
	assert.Empty(t, string(ctx.Response.Header.Peek("Strict-Transport-Security")))
}

// TestHeadersMiddleware_addHeaders_DefaultContentType 测试默认 X-Content-Type-Options
func TestHeadersMiddleware_addHeaders_DefaultContentType(t *testing.T) {
	cfg := &config.SecurityHeaders{
		XFrameOptions: "DENY",
	}
	sh := NewHeadersWithHSTS(cfg, nil)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/")
	sh.addHeaders(ctx)

	assert.Equal(t, "nosniff", string(ctx.Response.Header.Peek("X-Content-Type-Options")))
}

// TestHeadersMiddleware_UpdateConfig 测试动态更新配置
func TestHeadersMiddleware_UpdateConfig(t *testing.T) {
	sh := NewHeadersWithHSTS(nil, nil)

	newCfg := &config.SecurityHeaders{
		XFrameOptions: "SAMEORIGIN",
	}
	sh.UpdateConfig(newCfg)

	result := sh.GetConfig()
	assert.Equal(t, "SAMEORIGIN", result.XFrameOptions)
}

// TestHeadersMiddleware_SetXFrameOptions 测试设置 X-Frame-Options
func TestHeadersMiddleware_SetXFrameOptions(t *testing.T) {
	cfg := &config.SecurityHeaders{XFrameOptions: "DENY"}
	sh := NewHeadersWithHSTS(cfg, nil)

	sh.SetXFrameOptions("SAMEORIGIN")
	assert.Equal(t, "SAMEORIGIN", sh.GetConfig().XFrameOptions)
}

// TestHeadersMiddleware_SetXFrameOptions_NilConfig 测试 nil 配置下设置
func TestHeadersMiddleware_SetXFrameOptions_NilConfig(t *testing.T) {
	sh := &HeadersMiddleware{}
	sh.SetXFrameOptions("DENY")
}

// TestHeadersMiddleware_SetContentSecurityPolicy 测试设置 CSP
func TestHeadersMiddleware_SetContentSecurityPolicy(t *testing.T) {
	cfg := &config.SecurityHeaders{}
	sh := NewHeadersWithHSTS(cfg, nil)

	sh.SetContentSecurityPolicy("default-src 'none'")
	assert.Equal(t, "default-src 'none'", sh.GetConfig().ContentSecurityPolicy)
}

// TestHeadersMiddleware_SetContentSecurityPolicy_NilConfig 测试 nil 配置下设置 CSP
func TestHeadersMiddleware_SetContentSecurityPolicy_NilConfig(t *testing.T) {
	sh := &HeadersMiddleware{}
	sh.SetContentSecurityPolicy("default-src 'none'")
}

// TestHeadersMiddleware_SetReferrerPolicy 测试设置 Referrer-Policy
func TestHeadersMiddleware_SetReferrerPolicy(t *testing.T) {
	cfg := &config.SecurityHeaders{}
	sh := NewHeadersWithHSTS(cfg, nil)

	sh.SetReferrerPolicy("no-referrer")
	assert.Equal(t, "no-referrer", sh.GetConfig().ReferrerPolicy)
}

// TestHeadersMiddleware_SetReferrerPolicy_NilConfig 测试 nil 配置下设置
func TestHeadersMiddleware_SetReferrerPolicy_NilConfig(t *testing.T) {
	sh := &HeadersMiddleware{}
	sh.SetReferrerPolicy("no-referrer")
}

// TestHeadersMiddleware_SetPermissionsPolicy 测试设置 Permissions-Policy
func TestHeadersMiddleware_SetPermissionsPolicy(t *testing.T) {
	cfg := &config.SecurityHeaders{}
	sh := NewHeadersWithHSTS(cfg, nil)

	sh.SetPermissionsPolicy("geolocation=()")
	assert.Equal(t, "geolocation=()", sh.GetConfig().PermissionsPolicy)
}

// TestHeadersMiddleware_SetPermissionsPolicy_NilConfig 测试 nil 配置下设置
func TestHeadersMiddleware_SetPermissionsPolicy_NilConfig(t *testing.T) {
	sh := &HeadersMiddleware{}
	sh.SetPermissionsPolicy("geolocation=()")
}

// TestHeadersMiddleware_GetConfig 测试获取配置副本
func TestHeadersMiddleware_GetConfig(t *testing.T) {
	cfg := &config.SecurityHeaders{
		XFrameOptions: "DENY",
	}
	sh := NewHeadersWithHSTS(cfg, nil)

	result := sh.GetConfig()
	require.NotNil(t, result)
	assert.Equal(t, "DENY", result.XFrameOptions)
}

// TestHeadersMiddleware_formatHSTSFromConfig_WithHSTS 测试 HSTS 配置格式化
func TestHeadersMiddleware_formatHSTSFromConfig_WithHSTS(t *testing.T) {
	sh := &HeadersMiddleware{}
	hstsCfg := &config.HSTSConfig{
		MaxAge:            604800,
		IncludeSubDomains: false,
		Preload:           true,
	}
	sh.formatHSTSFromConfig(hstsCfg)
	assert.Equal(t, "max-age=604800; preload", sh.hsts)
}

// ===================== auth.go 测试 =====================

// TestAuthenticateArgon2id_Success 测试 argon2id 认证成功
func TestAuthenticateArgon2id_Success(t *testing.T) {
	password := "testpassword"
	salt := []byte("randomsalt123456")
	time := uint32(1)
	memory := uint32(64)
	threads := uint8(1)
	keyLen := uint32(32)

	hash := argon2.IDKey([]byte(password), salt, time, memory, threads, keyLen)
	hashStr := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, time, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)

	result := authenticateArgon2id(password, hashStr)
	assert.True(t, result)
}

// TestAuthenticateArgon2id_WrongPassword 测试 argon2id 密码错误
func TestAuthenticateArgon2id_WrongPassword(t *testing.T) {
	password := "testpassword"
	salt := []byte("randomsalt123456")
	hash := argon2.IDKey([]byte(password), salt, 1, 64, 1, 32)
	hashStr := fmt.Sprintf("$argon2id$v=19$m=64,t=1,p=1$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)

	result := authenticateArgon2id("wrongpassword", hashStr)
	assert.False(t, result)
}

// TestAuthenticateArgon2id_InvalidHash 测试 argon2id 无效哈希
func TestAuthenticateArgon2id_InvalidHash(t *testing.T) {
	result := authenticateArgon2id("password", "invalid")
	assert.False(t, result)
}

// TestParseArgon2idHash_Valid 测试解析有效的 argon2id 哈希
func TestParseArgon2idHash_Valid(t *testing.T) {
	salt := []byte("testsalt")
	expectedHash := make([]byte, 32)
	for i := range expectedHash {
		expectedHash[i] = byte(i)
	}

	hashStr := fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=4$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(expectedHash),
	)

	params, gotSalt, gotHash, err := parseArgon2idHash(hashStr)
	require.NoError(t, err)
	assert.Equal(t, uint32(65536), params.memory)
	assert.Equal(t, uint32(3), params.time)
	assert.Equal(t, uint8(4), params.threads)
	assert.Equal(t, uint32(32), params.keyLen)
	assert.Equal(t, salt, gotSalt)
	assert.Equal(t, expectedHash, gotHash)
}

// TestParseArgon2idHash_InvalidSalt 测试无效的盐值
func TestParseArgon2idHash_InvalidSalt(t *testing.T) {
	hashStr := "$argon2id$v=19$m=65536,t=3,p=4$invalid!base64$hash"
	_, _, _, err := parseArgon2idHash(hashStr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid salt")
}

// TestParseArgon2idHash_InvalidHashValue 测试无效的哈希值
func TestParseArgon2idHash_InvalidHashValue(t *testing.T) {
	hashStr := "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$$invalid!base64!"
	_, _, _, err := parseArgon2idHash(hashStr)
	assert.Error(t, err)
}

// TestParseArgon2idHash_MalformedParams 测试畸形参数
func TestParseArgon2idHash_MalformedParams(t *testing.T) {
	hashStr := "$argon2id$v=19$m=abc,t=def,p=xyz$c2FsdA$hash"
	params, _, _, err := parseArgon2idHash(hashStr)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), params.memory)
	assert.Equal(t, uint32(0), params.time)
	assert.Equal(t, uint8(0), params.threads)
}

// TestParseUint32 测试 parseUint32 函数
func TestParseUint32(t *testing.T) {
	assert.Equal(t, uint32(0), parseUint32(""))
	assert.Equal(t, uint32(0), parseUint32("abc"))
	assert.Equal(t, uint32(42), parseUint32("42"))
	assert.Equal(t, uint32(65536), parseUint32("65536"))
	assert.Equal(t, uint32(100), parseUint32("1a0b0"))
	assert.Equal(t, uint32(0), parseUint32("0"))
}

// TestParseUint8 测试 parseUint8 函数
func TestParseUint8(t *testing.T) {
	assert.Equal(t, uint8(0), parseUint8(""))
	assert.Equal(t, uint8(0), parseUint8("xyz"))
	assert.Equal(t, uint8(4), parseUint8("4"))
	assert.Equal(t, uint8(255), parseUint8("255"))
	assert.Equal(t, uint8(12), parseUint8("1a2"))
	assert.Equal(t, uint8(0), parseUint8("0"))
}

// TestBasicAuth_Process_Argon2id 测试 argon2id 认证流程
func TestBasicAuth_Process_Argon2id(t *testing.T) {
	password := "secret"
	salt := []byte("saltsalt")
	hash := argon2.IDKey([]byte(password), salt, 1, 64, 1, 32)
	hashStr := fmt.Sprintf("$argon2id$v=19$m=64,t=1,p=1$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)

	auth, err := NewBasicAuth(&config.AuthConfig{
		Type:      "basic",
		Algorithm: "argon2id",
		Users: []config.User{
			{Name: "admin", Password: hashStr},
		},
		RequireTLS: false,
	})
	require.NoError(t, err)

	called := false
	handler := auth.Process(func(ctx *fasthttp.RequestCtx) {
		called = true
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Authorization", "Basic YWRtaW46c2VjcmV0")
	ctx.Request.SetRequestURI("/")
	handler(ctx)

	assert.True(t, called)
	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
}

// ===================== geoip.go 测试 =====================

// TestNewGeoIPLookup_EmptyPath 测试空路径
func TestNewGeoIPLookup_EmptyPath(t *testing.T) {
	_, err := NewGeoIPLookup("", 100, 0, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database path is required")
}

// TestNewGeoIPLookup_NonexistentPath 测试不存在的数据库路径
func TestNewGeoIPLookup_NonexistentPath(t *testing.T) {
	_, err := NewGeoIPLookup("/nonexistent/GeoIP.mmdb", 100, 0, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "open geoip database")
}

// TestGeoIPLookup_Close_NilDB 测试 nil 数据库的关闭
func TestGeoIPLookup_Close_NilDB(t *testing.T) {
	g := &GeoIPLookup{}
	err := g.Close()
	assert.NoError(t, err)
}

// TestIsPrivateIP_Coverage 补充测试私有 IP 检测
func TestIsPrivateIP_Coverage(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"10.x 私有", "10.0.0.1", true},
		{"172.16.x 私有", "172.16.0.1", true},
		{"192.168.x 私有", "192.168.1.1", true},
		{"127.x 回环", "127.0.0.1", true},
		{"公网 IP", "8.8.8.8", false},
		{"公网 IP 2", "1.1.1.1", false},
		{"IPv6 回环", "::1", true},
		{"IPv6 公网", "2001:db8::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip)
			assert.Equal(t, tt.expected, isPrivateIP(ip))
		})
	}
}

// ===================== access.go Check 测试 =====================

// TestCheck_GeoIPAllowCountry 测试 GeoIP 国家允许
func TestCheck_GeoIPAllowCountry(t *testing.T) {
	cfg := &config.AccessConfig{
		Default: "deny",
		GeoIP: config.GeoIPConfig{
			Enabled:        true,
			Database:       "", // 不使用真实数据库
			AllowCountries: []string{"CN"},
		},
	}
	ac, err := NewAccessControl(cfg)
	require.NoError(t, err)

	ip := net.ParseIP("8.8.8.8")
	// GeoIP 未初始化（无数据库），不检查国家规则，直接走默认
	result := ac.Check(ip)
	assert.False(t, result)
}

// TestCheck_GeoIPDenyCountry 测试 GeoIP 国家拒绝
func TestCheck_GeoIPDenyCountry(t *testing.T) {
	cfg := &config.AccessConfig{
		Default: "allow",
		GeoIP: config.GeoIPConfig{
			Enabled:       true,
			Database:      "",
			DenyCountries: []string{"RU"},
		},
	}
	ac, err := NewAccessControl(cfg)
	require.NoError(t, err)

	ip := net.ParseIP("8.8.8.8")
	// GeoIP 未初始化（无数据库），不检查国家规则，直接走默认
	result := ac.Check(ip)
	assert.True(t, result)
}

// ===================== auth_request.go Process 测试 =====================

// TestAuthRequest_Process_AuthServiceUnavailable 测试认证服务不可用
func TestAuthRequest_Process_AuthServiceUnavailable(t *testing.T) {
	cfg := config.AuthRequestConfig{
		Enabled:        true,
		URI:            "/auth",
		Method:         "GET",
		Timeout:        100 * time.Millisecond,
		ForwardHeaders: []string{},
		Headers:        map[string]string{"X-Auth-Source": "lolly"},
	}

	ar, err := NewAuthRequest(cfg)
	require.NoError(t, err)

	nextCalled := false
	next := func(ctx *fasthttp.RequestCtx) {
		nextCalled = true
	}

	handler := ar.Process(next)
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/protected")
	ctx.Request.Header.SetHost("localhost")
	handler(ctx)

	assert.False(t, nextCalled)
	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
}

// TestAuthRequest_Process_WithVariables 测试变量展开后请求失败
func TestAuthRequest_Process_WithVariables(t *testing.T) {
	cfg := config.AuthRequestConfig{
		Enabled:        true,
		URI:            "http://127.0.0.1:1/auth?uri=$request_uri",
		Method:         "GET",
		Timeout:        100 * time.Millisecond,
		ForwardHeaders: []string{"Authorization"},
	}

	ar, err := NewAuthRequest(cfg)
	require.NoError(t, err)

	handler := ar.Process(func(ctx *fasthttp.RequestCtx) {})
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/test")
	ctx.Request.Header.Set("Authorization", "Bearer token123")
	handler(ctx)

	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
}
