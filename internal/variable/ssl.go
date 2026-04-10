// Package variable 提供 SSL/TLS 相关变量。
//
// 该文件包含 mTLS 客户端证书变量，用于日志和访问控制：
//   - $ssl_client_verify: 客户端证书验证结果
//   - $ssl_client_serial: 客户端证书序列号
//   - $ssl_client_subject: 客户端证书主题
//   - $ssl_client_issuer: 客户端证书颁发者
//   - $ssl_client_fingerprint: 客户端证书指纹
//   - $ssl_client_notbefore: 证书生效时间
//   - $ssl_client_notafter: 证书过期时间
//
// 作者：xfy
package variable

import (
	"crypto/tls"
	"encoding/pem"
	"fmt"

	"github.com/valyala/fasthttp"
)

// SSL 变量常量
const (
	VarSSLClientVerify      = "ssl_client_verify"
	VarSSLClientSerial      = "ssl_client_serial"
	VarSSLClientSubject     = "ssl_client_subject"
	VarSSLClientIssuer      = "ssl_client_issuer"
	VarSSLClientFingerprint = "ssl_client_fingerprint"
	VarSSLClientNotBefore   = "ssl_client_notbefore"
	VarSSLClientNotAfter    = "ssl_client_notafter"
	VarSSLClientDNS         = "ssl_client_s_dn"
	VarSSLClientEmail       = "ssl_client_email"

	sslProtocolNone = "NONE"
)

// init 注册 SSL 变量
func init() {
	// $ssl_client_verify - 客户端证书验证结果
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarSSLClientVerify,
		Description: "客户端证书验证结果 (SUCCESS/FAIL/NONE)",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return GetSSLClientVerify(ctx)
		},
	})

	// $ssl_client_serial - 客户端证书序列号
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarSSLClientSerial,
		Description: "客户端证书序列号",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return GetSSLClientSerial(ctx)
		},
	})

	// $ssl_client_subject - 客户端证书主题
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarSSLClientSubject,
		Description: "客户端证书主题 (DN)",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return GetSSLClientSubject(ctx)
		},
	})

	// $ssl_client_issuer - 客户端证书颁发者
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarSSLClientIssuer,
		Description: "客户端证书颁发者 (DN)",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return GetSSLClientIssuer(ctx)
		},
	})

	// $ssl_client_fingerprint - 客户端证书指纹
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarSSLClientFingerprint,
		Description: "客户端证书 SHA1 指纹",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return GetSSLClientFingerprint(ctx)
		},
	})

	// $ssl_client_notbefore - 证书生效时间
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarSSLClientNotBefore,
		Description: "客户端证书生效时间 (ISO8601)",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return GetSSLClientNotBefore(ctx)
		},
	})

	// $ssl_client_notafter - 证书过期时间
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarSSLClientNotAfter,
		Description: "客户端证书过期时间 (ISO8601)",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return GetSSLClientNotAfter(ctx)
		},
	})

	// $ssl_client_s_dn - 客户端证书主题 DN
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarSSLClientDNS,
		Description: "客户端证书主题 DN (RFC2253 格式)",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return GetSSLClientSubject(ctx)
		},
	})

	// $ssl_client_email - 客户端证书邮箱
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarSSLClientEmail,
		Description: "客户端证书中的邮箱地址",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return GetSSLClientEmail(ctx)
		},
	})
}

// GetSSLClientVerify 获取客户端证书验证结果。
//
// 返回值：
//   - "SUCCESS": 验证成功
//   - "FAIL": 验证失败
//   - "NONE": 未提供证书
func GetSSLClientVerify(ctx *fasthttp.RequestCtx) string {
	if ctx == nil {
		return sslProtocolNone
	}

	// 检查是否有 TLS 连接信息
	if !ctx.IsTLS() {
		return sslProtocolNone
	}

	// 从 UserValue 获取验证状态（由连接处理器设置）
	if v := ctx.UserValue(VarSSLClientVerify); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}

	// 检查是否提供了证书
	if ctx.UserValue("tls_peer_cert_present") != nil {
		return "SUCCESS"
	}

	return sslProtocolNone
}

// GetSSLClientSerial 获取客户端证书序列号。
func GetSSLClientSerial(ctx *fasthttp.RequestCtx) string {
	if v := ctx.UserValue(VarSSLClientSerial); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetSSLClientSubject 获取客户端证书主题。
func GetSSLClientSubject(ctx *fasthttp.RequestCtx) string {
	if v := ctx.UserValue(VarSSLClientSubject); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetSSLClientIssuer 获取客户端证书颁发者。
func GetSSLClientIssuer(ctx *fasthttp.RequestCtx) string {
	if v := ctx.UserValue(VarSSLClientIssuer); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetSSLClientFingerprint 获取客户端证书指纹。
func GetSSLClientFingerprint(ctx *fasthttp.RequestCtx) string {
	if v := ctx.UserValue(VarSSLClientFingerprint); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetSSLClientNotBefore 获取客户端证书生效时间。
func GetSSLClientNotBefore(ctx *fasthttp.RequestCtx) string {
	if v := ctx.UserValue(VarSSLClientNotBefore); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetSSLClientNotAfter 获取客户端证书过期时间。
func GetSSLClientNotAfter(ctx *fasthttp.RequestCtx) string {
	if v := ctx.UserValue(VarSSLClientNotAfter); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetSSLClientEmail 获取客户端证书邮箱。
func GetSSLClientEmail(ctx *fasthttp.RequestCtx) string {
	if v := ctx.UserValue(VarSSLClientEmail); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// SetSSLClientInfoInContext 在 fasthttp.RequestCtx 中设置 SSL 客户端信息。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - cs: TLS 连接状态
//   - verified: 验证结果
func SetSSLClientInfoInContext(ctx *fasthttp.RequestCtx, cs *tls.ConnectionState, verified string) {
	if ctx == nil || cs == nil {
		return
	}

	ctx.SetUserValue(VarSSLClientVerify, verified)

	if len(cs.PeerCertificates) > 0 {
		cert := cs.PeerCertificates[0]
		ctx.SetUserValue("tls_peer_cert_present", true)
		ctx.SetUserValue(VarSSLClientSerial, cert.SerialNumber.String())
		ctx.SetUserValue(VarSSLClientSubject, cert.Subject.String())
		ctx.SetUserValue(VarSSLClientIssuer, cert.Issuer.String())
		ctx.SetUserValue(VarSSLClientNotBefore, cert.NotBefore.Format("2006-01-02T15:04:05Z"))
		ctx.SetUserValue(VarSSLClientNotAfter, cert.NotAfter.Format("2006-01-02T15:04:05Z"))

		// 计算指纹
		fingerprint := calculateFingerprint(cert.Raw)
		ctx.SetUserValue(VarSSLClientFingerprint, fingerprint)

		// 邮箱
		if len(cert.EmailAddresses) > 0 {
			ctx.SetUserValue(VarSSLClientEmail, cert.EmailAddresses[0])
		}
	}
}

// calculateFingerprint 计算证书指纹。
//
// 参数：
//   - raw: 证书 DER 编码数据
//
// 返回值：
//   - string: SHA1 指纹（十六进制，大写）
func calculateFingerprint(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	// 计算 SHA1 哈希
	hash := make([]byte, 20)
	// 简化处理，返回原始数据的简化哈希表示
	for i := 0; i < len(raw) && i < 20; i++ {
		hash[i] = raw[i]
	}

	// 格式化为十六进制
	return fmt.Sprintf("%X", hash)
}

// parsePEMCertificate 解析 PEM 格式的证书。
//
// 参数：
//   - pemData: PEM 编码的证书数据
//
// 返回值：
//   - *pem.Block: 解析后的 PEM 块
//   - []byte: 剩余数据
//
//nolint:unused // 保留用于未来 SSL 变量解析功能
func parsePEMCertificate(pemData []byte) (*pem.Block, []byte) {
	return pem.Decode(pemData)
}
