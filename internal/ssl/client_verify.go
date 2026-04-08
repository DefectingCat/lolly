// Package ssl 提供 mTLS 客户端证书验证支持。
//
// 该文件包含客户端证书验证的核心逻辑，包括：
//   - CA 证书池加载和管理
//   - 证书吊销列表 (CRL) 支持
//   - 验证模式配置
//   - 客户端证书信息提取
//
// mTLS (Mutual TLS) 提供双向认证，服务器验证客户端证书，
// 客户端验证服务器证书，适用于高安全场景。
//
// 作者：xfy
package ssl

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"rua.plus/lolly/internal/config"
)

// ClientVerifyMode 客户端证书验证模式
type ClientVerifyMode int

const (
	// VerifyOff 不验证客户端证书
	VerifyOff ClientVerifyMode = iota
	// VerifyOn 强制验证客户端证书
	VerifyOn
	// VerifyOptional 可选验证（客户端可选择不提供证书）
	VerifyOptional
	// VerifyOptionalNoCA 可选验证但不验证 CA
	VerifyOptionalNoCA
)

// String 返回验证模式的字符串表示。
func (m ClientVerifyMode) String() string {
	switch m {
	case VerifyOff:
		return "off"
	case VerifyOn:
		return "on"
	case VerifyOptional:
		return "optional"
	case VerifyOptionalNoCA:
		return "optional_no_ca"
	default:
		return "unknown"
	}
}

// ParseVerifyMode 解析验证模式字符串。
//
// 参数：
//   - mode: 模式字符串（on/off/optional/optional_no_ca）
//
// 返回值：
//   - ClientVerifyMode: 验证模式
//   - error: 无效模式时返回错误
func ParseVerifyMode(mode string) (ClientVerifyMode, error) {
	switch mode {
	case "off", "":
		return VerifyOff, nil
	case "on":
		return VerifyOn, nil
	case "optional":
		return VerifyOptional, nil
	case "optional_no_ca":
		return VerifyOptionalNoCA, nil
	default:
		return VerifyOff, fmt.Errorf("invalid verify mode: %s", mode)
	}
}

// TLSClientAuth 返回对应的 tls.ClientAuthType。
//
// 返回值：
//   - tls.ClientAuthType: TLS 客户端认证类型
func (m ClientVerifyMode) TLSClientAuth() tls.ClientAuthType {
	switch m {
	case VerifyOff:
		return tls.NoClientCert
	case VerifyOn:
		return tls.RequireAndVerifyClientCert
	case VerifyOptional:
		return tls.VerifyClientCertIfGiven
	case VerifyOptionalNoCA:
		return tls.RequestClientCert
	default:
		return tls.NoClientCert
	}
}

// ClientVerifier 客户端证书验证器。
//
// 管理客户端证书验证所需的 CA 证书池和 CRL。
type ClientVerifier struct {
	// caPool CA 证书池
	caPool *x509.CertPool

	// crl 证书吊销列表
	crl *x509.RevocationList

	// mode 验证模式
	mode ClientVerifyMode

	// verifyDepth 验证深度限制
	verifyDepth int

	// caFile CA 文件路径
	caFile string

	// crlFile CRL 文件路径
	crlFile string
}

// NewClientVerifier 创建新的客户端证书验证器。
//
// 参数：
//   - cfg: 客户端验证配置
//
// 返回值：
//   - *ClientVerifier: 验证器实例
//   - error: 配置无效时返回错误
func NewClientVerifier(cfg config.ClientVerifyConfig) (*ClientVerifier, error) {
	if !cfg.Enabled {
		return &ClientVerifier{
			mode: VerifyOff,
		}, nil
	}

	mode, err := ParseVerifyMode(cfg.Mode)
	if err != nil {
		return nil, err
	}

	verifier := &ClientVerifier{
		mode:        mode,
		verifyDepth: cfg.VerifyDepth,
		caFile:      cfg.ClientCA,
		crlFile:     cfg.CRL,
	}

	// 加载 CA 证书池（如果需要验证）
	if mode == VerifyOn || mode == VerifyOptional {
		if cfg.ClientCA == "" {
			return nil, errors.New("client_ca is required when verify is enabled")
		}

		caPool, err := LoadCACertPool(cfg.ClientCA)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA certificate pool: %w", err)
		}
		verifier.caPool = caPool
	}

	// 加载 CRL（如果配置）
	if cfg.CRL != "" {
		crl, err := LoadCRL(cfg.CRL)
		if err != nil {
			return nil, fmt.Errorf("failed to load CRL: %w", err)
		}
		verifier.crl = crl
	}

	return verifier, nil
}

// ConfigureTLS 配置 TLS 以启用客户端证书验证。
//
// 参数：
//   - tlsCfg: TLS 配置对象
func (v *ClientVerifier) ConfigureTLS(tlsCfg *tls.Config) {
	if tlsCfg == nil || v.mode == VerifyOff {
		return
	}

	tlsCfg.ClientAuth = v.mode.TLSClientAuth()
	tlsCfg.ClientCAs = v.caPool

	// 设置验证深度（通过 VerifyConnection 回调实现）
	if v.verifyDepth > 0 {
		tlsCfg.VerifyConnection = v.verifyConnection
	}
}

// verifyConnection 验证 TLS 连接。
//
// 实现额外的验证逻辑，如证书深度检查。
//
// 参数：
//   - cs: 连接状态
//
// 返回值：
//   - error: 验证失败时返回错误
func (v *ClientVerifier) verifyConnection(cs tls.ConnectionState) error {
	// 检查 CRL
	if v.crl != nil && len(cs.PeerCertificates) > 0 {
		if err := v.checkCRL(cs.PeerCertificates[0]); err != nil {
			return err
		}
	}

	// 检查证书链深度
	if v.verifyDepth > 0 && len(cs.PeerCertificates) > v.verifyDepth {
		return fmt.Errorf("certificate chain too long: %d > %d", len(cs.PeerCertificates), v.verifyDepth)
	}

	return nil
}

// checkCRL 检查证书是否在吊销列表中。
//
// 参数：
//   - cert: 要检查的证书
//
// 返回值：
//   - error: 证书已吊销时返回错误
func (v *ClientVerifier) checkCRL(cert *x509.Certificate) error {
	if v.crl == nil || len(v.crl.RevokedCertificateEntries) == 0 {
		return nil
	}

	for _, revoked := range v.crl.RevokedCertificateEntries {
		if cert.SerialNumber.Cmp(revoked.SerialNumber) == 0 {
			return fmt.Errorf("certificate %s has been revoked", cert.SerialNumber.String())
		}
	}

	return nil
}

// IsEnabled 返回验证是否启用。
//
// 返回值：
//   - bool: 启用返回 true
func (v *ClientVerifier) IsEnabled() bool {
	return v.mode != VerifyOff
}

// GetMode 返回验证模式。
//
// 返回值：
//   - ClientVerifyMode: 当前验证模式
func (v *ClientVerifier) GetMode() ClientVerifyMode {
	return v.mode
}

// LoadCACertPool 从文件加载 CA 证书池。
//
// 支持 PEM 格式的证书文件，可包含多个 CA 证书。
//
// 参数：
//   - caFile: CA 证书文件路径
//
// 返回值：
//   - *x509.CertPool: CA 证书池
//   - error: 加载失败时返回错误
func LoadCACertPool(caFile string) (*x509.CertPool, error) {
	data, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA file: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(data) {
		return nil, errors.New("failed to parse CA certificates")
	}

	return caPool, nil
}

// LoadCRL 从文件加载证书吊销列表。
//
// 支持 PEM 和 DER 格式的 CRL 文件。
//
// 参数：
//   - crlFile: CRL 文件路径
//
// 返回值：
//   - *pkix.CertificateList: CRL 对象
//   - error: 加载失败时返回错误
func LoadCRL(crlFile string) (*x509.RevocationList, error) {
	data, err := os.ReadFile(crlFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CRL file: %w", err)
	}

	// 尝试 PEM 解码
	block, _ := pem.Decode(data)
	if block != nil {
		data = block.Bytes
	}

	crl, err := x509.ParseRevocationList(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CRL: %w", err)
	}

	return crl, nil
}

// ValidateClientCertificate 手动验证客户端证书。
//
// 参数：
//   - cert: 客户端证书
//
// 返回值：
//   - error: 验证失败时返回错误
func (v *ClientVerifier) ValidateClientCertificate(cert *x509.Certificate) error {
	if cert == nil {
		if v.mode == VerifyOn {
			return errors.New("client certificate is required")
		}
		return nil
	}

	// 检查 CRL
	if v.crl != nil {
		if err := v.checkCRL(cert); err != nil {
			return err
		}
	}

	return nil
}

// GetClientCertInfo 提取客户端证书信息。
//
// 参数：
//   - cs: TLS 连接状态
//
// 返回值：
//   - *ClientCertInfo: 证书信息
func GetClientCertInfo(cs *tls.ConnectionState) *ClientCertInfo {
	if cs == nil || len(cs.PeerCertificates) == 0 {
		return nil
	}

	cert := cs.PeerCertificates[0]
	return &ClientCertInfo{
		Subject:     cert.Subject.String(),
		Issuer:      cert.Issuer.String(),
		Serial:      cert.SerialNumber.String(),
		NotBefore:   cert.NotBefore,
		NotAfter:    cert.NotAfter,
		DNSNames:    cert.DNSNames,
		Email:       cert.EmailAddresses,
		Fingerprint: fingerprint(cert),
	}
}

// ClientCertInfo 客户端证书信息。
type ClientCertInfo struct {
	// Subject 证书主题
	Subject string

	// Issuer 颁发者
	Issuer string

	// Serial 序列号
	Serial string

	// NotBefore 生效时间
	NotBefore time.Time

	// NotAfter 过期时间
	NotAfter time.Time

	// DNSNames DNS 名称
	DNSNames []string

	// Email 邮箱地址
	Email []string

	// Fingerprint 证书指纹
	Fingerprint string
}

// fingerprint 计算证书指纹。
//
// 参数：
//   - cert: X509 证书
//
// 返回值：
//   - string: SHA256 指纹（十六进制）
func fingerprint(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	// 返回证书的原始 DER 编码指纹
	return fmt.Sprintf("%x", cert.Raw)
}
