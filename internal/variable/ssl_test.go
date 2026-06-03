// ssl_test.go - SSL/TLS 客户端证书变量测试
//
// 测试覆盖：
//   - mTLS 客户端证书变量获取
//   - SetSSLClientInfoInContext 设置功能
//   - calculateFingerprint 指纹计算
//
// 作者：xfy
package variable

import (
	"testing"

	"github.com/valyala/fasthttp"
)

// TestGetSSLClientVerify_NilContext 测试 nil 上下文
func TestGetSSLClientVerify_NilContext(t *testing.T) {
	result := GetSSLClientVerify(nil)
	if result != "NONE" {
		t.Errorf("GetSSLClientVerify(nil) = %q, want NONE", result)
	}
}

// TestGetSSLClientVerify_NoTLS 测试非 TLS 连接
func TestGetSSLClientVerify_NoTLS(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	// 默认情况下 IsTLS() 返回 false
	result := GetSSLClientVerify(ctx)
	if result != "NONE" {
		t.Errorf("GetSSLClientVerify(non-TLS) = %q, want NONE", result)
	}
}

// TestGetSSLClientVerify_NonTLSWithUserValue 测试非 TLS 连接即使设置了 UserValue 也返回 NONE
// 注意：GetSSLClientVerify 会先检查 ctx.IsTLS()，非 TLS 连接直接返回 NONE
// 这是正确的行为，SSL 客户端变量只在 TLS 连接中有效
func TestGetSSLClientVerify_NonTLSWithUserValue(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue(VarSSLClientVerify, "SUCCESS")

	// 非 TLS 连接，即使设置了 UserValue 也应该返回 NONE
	result := GetSSLClientVerify(ctx)
	if result != "NONE" {
		t.Errorf("GetSSLClientVerify(non-TLS with value) = %q, want NONE", result)
	}
}

// TestGetSSLClientVerify_PeerCertPresent_NonTLS 测试非 TLS 下 peer_cert_present 不生效
func TestGetSSLClientVerify_PeerCertPresent_NonTLS(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue("tls_peer_cert_present", true)

	// 非 TLS 连接，peer_cert_present 不应该改变结果
	result := GetSSLClientVerify(ctx)
	if result != "NONE" {
		t.Errorf("GetSSLClientVerify(non-TLS with peer_cert) = %q, want NONE", result)
	}
}

// TestGetSSLClientSerial 测试获取序列号
func TestGetSSLClientSerial(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*fasthttp.RequestCtx)
		expected string
	}{
		{
			name:     "no value",
			setup:    func(_ *fasthttp.RequestCtx) {},
			expected: "",
		},
		{
			name: "with serial",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.SetUserValue(VarSSLClientSerial, "1234567890ABCDEF")
			},
			expected: "1234567890ABCDEF",
		},
		{
			name: "invalid type",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.SetUserValue(VarSSLClientSerial, 12345)
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			tt.setup(ctx)
			result := GetSSLClientSerial(ctx)
			if result != tt.expected {
				t.Errorf("GetSSLClientSerial() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestGetSSLClientSubject 测试获取主题
func TestGetSSLClientSubject(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*fasthttp.RequestCtx)
		expected string
	}{
		{
			name:     "no value",
			setup:    func(_ *fasthttp.RequestCtx) {},
			expected: "",
		},
		{
			name: "with subject",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.SetUserValue(VarSSLClientSubject, "CN=test.example.com,O=Test Org")
			},
			expected: "CN=test.example.com,O=Test Org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			tt.setup(ctx)
			result := GetSSLClientSubject(ctx)
			if result != tt.expected {
				t.Errorf("GetSSLClientSubject() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestGetSSLClientIssuer 测试获取颁发者
func TestGetSSLClientIssuer(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*fasthttp.RequestCtx)
		expected string
	}{
		{
			name:     "no value",
			setup:    func(_ *fasthttp.RequestCtx) {},
			expected: "",
		},
		{
			name: "with issuer",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.SetUserValue(VarSSLClientIssuer, "CN=Test CA,O=Test Org")
			},
			expected: "CN=Test CA,O=Test Org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			tt.setup(ctx)
			result := GetSSLClientIssuer(ctx)
			if result != tt.expected {
				t.Errorf("GetSSLClientIssuer() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestGetSSLClientFingerprint 测试获取指纹
func TestGetSSLClientFingerprint(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*fasthttp.RequestCtx)
		expected string
	}{
		{
			name:     "no value",
			setup:    func(_ *fasthttp.RequestCtx) {},
			expected: "",
		},
		{
			name: "with fingerprint",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.SetUserValue(VarSSLClientFingerprint, "A1B2C3D4E5F6")
			},
			expected: "A1B2C3D4E5F6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			tt.setup(ctx)
			result := GetSSLClientFingerprint(ctx)
			if result != tt.expected {
				t.Errorf("GetSSLClientFingerprint() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestGetSSLClientNotBefore 测试获取生效时间
func TestGetSSLClientNotBefore(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*fasthttp.RequestCtx)
		expected string
	}{
		{
			name:     "no value",
			setup:    func(_ *fasthttp.RequestCtx) {},
			expected: "",
		},
		{
			name: "with notbefore",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.SetUserValue(VarSSLClientNotBefore, "2025-01-01T00:00:00Z")
			},
			expected: "2025-01-01T00:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			tt.setup(ctx)
			result := GetSSLClientNotBefore(ctx)
			if result != tt.expected {
				t.Errorf("GetSSLClientNotBefore() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestGetSSLClientNotAfter 测试获取过期时间
func TestGetSSLClientNotAfter(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*fasthttp.RequestCtx)
		expected string
	}{
		{
			name:     "no value",
			setup:    func(_ *fasthttp.RequestCtx) {},
			expected: "",
		},
		{
			name: "with notafter",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.SetUserValue(VarSSLClientNotAfter, "2026-01-01T00:00:00Z")
			},
			expected: "2026-01-01T00:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			tt.setup(ctx)
			result := GetSSLClientNotAfter(ctx)
			if result != tt.expected {
				t.Errorf("GetSSLClientNotAfter() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestGetSSLClientEmail 测试获取邮箱
func TestGetSSLClientEmail(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*fasthttp.RequestCtx)
		expected string
	}{
		{
			name:     "no value",
			setup:    func(_ *fasthttp.RequestCtx) {},
			expected: "",
		},
		{
			name: "with email",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.SetUserValue(VarSSLClientEmail, "test@example.com")
			},
			expected: "test@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			tt.setup(ctx)
			result := GetSSLClientEmail(ctx)
			if result != tt.expected {
				t.Errorf("GetSSLClientEmail() = %q, want %q", result, tt.expected)
			}
		})
	}
}















// TestSSLVariablesInContext 测试通过 VariableContext 访问 SSL 变量
// 注意：ssl_client_verify 在非 TLS 连接下会返回 NONE（因为 GetSSLClientVerify 检查 ctx.IsTLS()）
func TestSSLVariablesInContext(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}

	// 设置 SSL 客户端信息
	ctx.SetUserValue(VarSSLClientSerial, "ABC123")
	ctx.SetUserValue(VarSSLClientSubject, "CN=test")
	ctx.SetUserValue(VarSSLClientIssuer, "CN=CA")
	ctx.SetUserValue(VarSSLClientFingerprint, "FINGERPRINT")
	ctx.SetUserValue(VarSSLClientNotBefore, "2025-01-01T00:00:00Z")
	ctx.SetUserValue(VarSSLClientNotAfter, "2026-01-01T00:00:00Z")
	ctx.SetUserValue(VarSSLClientEmail, "test@example.com")

	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	tests := []struct {
		varName  string
		expected string
	}{
		{VarSSLClientSerial, "ABC123"},
		{VarSSLClientSubject, "CN=test"},
		{VarSSLClientIssuer, "CN=CA"},
		{VarSSLClientFingerprint, "FINGERPRINT"},
		{VarSSLClientNotBefore, "2025-01-01T00:00:00Z"},
		{VarSSLClientNotAfter, "2026-01-01T00:00:00Z"},
		{VarSSLClientEmail, "test@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.varName, func(t *testing.T) {
			value, ok := vc.Get(tt.varName)
			if !ok {
				t.Errorf("variable %s not found", tt.varName)
				return
			}
			if value != tt.expected {
				t.Errorf("%s = %q, want %q", tt.varName, value, tt.expected)
			}
		})
	}
}

// TestSSLVariablesInContext_VerifyNonTLS 测试 ssl_client_verify 在非 TLS 下返回 NONE
func TestSSLVariablesInContext_VerifyNonTLS(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue(VarSSLClientVerify, "SUCCESS")

	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 非 TLS 连接，ssl_client_verify 应该返回 NONE
	value, ok := vc.Get(VarSSLClientVerify)
	if !ok {
		t.Error("ssl_client_verify not found")
		return
	}
	if value != "NONE" {
		t.Errorf("ssl_client_verify = %q, want NONE (non-TLS context)", value)
	}
}

// TestSSLVariablesExpand 测试在模板中展开 SSL 变量
// 注意：ssl_client_verify 在非 TLS 连接下会返回 NONE
func TestSSLVariablesExpand(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}

	ctx.SetUserValue(VarSSLClientSerial, "12345")
	ctx.SetUserValue(VarSSLClientSubject, "CN=test")

	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	tests := []struct {
		template string
		expected string
	}{
		{"$ssl_client_serial", "12345"},
		{"$ssl_client_subject", "CN=test"},
		{"serial=$ssl_client_serial subject=$ssl_client_subject", "serial=12345 subject=CN=test"},
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

// TestSSLVariablesExpand_VerifyNonTLS 测试 ssl_client_verify 在非 TLS 下展开为 NONE
func TestSSLVariablesExpand_VerifyNonTLS(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue(VarSSLClientVerify, "SUCCESS")

	vc := NewContext(ctx)
	defer ReleaseContext(vc)

	// 非 TLS 连接，ssl_client_verify 应该展开为 NONE
	result := vc.Expand("$ssl_client_verify")
	if result != "NONE" {
		t.Errorf("Expand($ssl_client_verify) = %q, want NONE (non-TLS context)", result)
	}
}
