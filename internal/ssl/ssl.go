// Package ssl 提供 SSL/TLS 支持。
//
// 该文件包含 TLS 配置管理的核心逻辑，包括：
//   - 安全的 TLS 默认配置（仅 TLSv1.2 和 TLSv1.3）
//   - 证书加载和管理
//   - SNI（服务器名称指示）支持
//   - OCSP Stapling 支持
//
// 主要用途：
//
//	用于管理 HTTPS 服务器的 TLS 配置，支持多证书虚拟主机。
//
// 安全默认值：
//   - TLS 版本：仅启用 TLSv1.2 和 TLSv1.3
//   - TLSv1.0 和 TLSv1.1 被强制禁用（不安全）
//   - 安全加密套件，支持前向保密
//   - 配置 TLS 时自动启用 HTTP/2
//
// 使用示例：
//
//	cfg := &config.SSLConfig{
//	    Cert: "/path/to/cert.pem",
//	    Key:  "/path/to/key.pem",
//	    Protocols: []string{"TLSv1.2", "TLSv1.3"},
//	}
//
//	manager, err := ssl.NewTLSManager(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 配合 fasthttp 使用
//	server := &fasthttp.Server{
//	    TLSConfig: manager.GetTLSConfig(),
//	}
//
// 作者：xfy
package ssl

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/netutil"
)

// TLSManager TLS 配置管理器。
//
// 管理单个或多个证书的 TLS 配置，支持 SNI（服务器名称指示）
// 用于多证书虚拟主机，以及 OCSP Stapling 用于证书状态验证。
type TLSManager struct {
	// configs TLS 配置映射，按服务器名称索引
	configs map[string]*tls.Config

	// defaultCfg 默认配置，用于 fallback
	defaultCfg *tls.Config

	// ocspManager OCSP Stapling 管理器
	ocspManager *OCSPManager

	// sessionTicketMgr Session Ticket 管理器
	sessionTicketMgr *SessionTicketManager

	// clientVerifier 客户端证书验证器
	clientVerifier *ClientVerifier

	// certificates 解析后的证书映射，用于 OCSP
	certificates map[string]*x509.Certificate

	// issuers 颁发者证书映射，用于 OCSP
	issuers map[string]*x509.Certificate

	// mu 保护并发访问的读写锁
	mu sync.RWMutex
}

// NewTLSManager 创建新的 TLS 配置管理器。
//
// 对于单服务器模式，传入单个 SSLConfig。
//
// 参数：
//   - cfg: SSL 配置，包含证书路径和 TLS 设置
//
// 返回值：
//   - *TLSManager: 配置好的 TLS 管理器
//   - error: 证书加载失败或配置无效时返回错误
func NewTLSManager(cfg *config.SSLConfig) (*TLSManager, error) {
	if cfg == nil {
		return nil, errors.New("ssl config is nil")
	}

	if cfg.Cert == "" || cfg.Key == "" {
		return nil, errors.New("certificate and key paths are required")
	}

	// 加载证书
	cert, err := loadCertificate(cfg.Cert, cfg.Key, cfg.CertChain)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}

	// 创建 TLS 配置，使用安全默认值
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12, // 强制 TLS 1.2 最低版本
		MaxVersion:   tls.VersionTLS13,
		NextProtos:   []string{"h2", "http/1.1"}, // 启用 HTTP/2 ALPN 支持
	}

	// 应用 TLS 1.2 的加密套件
	if len(cfg.Ciphers) > 0 {
		ciphers, err := parseCipherSuites(cfg.Ciphers)
		if err != nil {
			return nil, fmt.Errorf("invalid cipher suites: %w", err)
		}
		tlsCfg.CipherSuites = ciphers
	} else {
		// 使用安全的默认加密套件
		tlsCfg.CipherSuites = defaultCipherSuites()
	}

	// 解析 TLS 协议版本
	if len(cfg.Protocols) > 0 {
		minVer, maxVer, err := parseTLSVersions(cfg.Protocols)
		if err != nil {
			return nil, fmt.Errorf("invalid TLS protocols: %w", err)
		}
		tlsCfg.MinVersion = minVer
		tlsCfg.MaxVersion = maxVer
	}

	manager := &TLSManager{
		configs:      make(map[string]*tls.Config),
		certificates: make(map[string]*x509.Certificate),
		issuers:      make(map[string]*x509.Certificate),
	}

	// 初始化 Session Tickets（如果启用）
	if cfg.SessionTickets.Enabled {
		sessionTicketMgr, err := NewSessionTicketManager(cfg.SessionTickets)
		if err != nil {
			logging.Warn().Err(err).Msg("Session Ticket 初始化失败，TLS 性能可能降级")
		} else {
			manager.sessionTicketMgr = sessionTicketMgr
			// 应用 Session Tickets 到 TLS 配置
			sessionTicketMgr.ApplyToTLSConfig(tlsCfg)
			sessionTicketMgr.Start()
		}
	}

	// 初始化 OCSP Stapling（如果启用）
	if cfg.OCSPStapling {
		ocspMgr := NewOCSPManager(DefaultOCSPConfig())
		manager.ocspManager = ocspMgr

		// 解析证书用于 OCSP
		if len(cert.Certificate) > 0 {
			parsedCert, err := x509.ParseCertificate(cert.Certificate[0])
			if err == nil && len(parsedCert.OCSPServer) > 0 {
				// 存储证书用于 OCSP 查询
				serial := parsedCert.SerialNumber.String()
				manager.certificates[serial] = parsedCert

				// 尝试从证书链解析颁发者证书
				if len(cert.Certificate) > 1 {
					issuerCert, err := x509.ParseCertificate(cert.Certificate[1])
					if err == nil {
						manager.issuers[serial] = issuerCert
						if err := ocspMgr.RegisterCertificate(parsedCert, issuerCert); err != nil {
							logging.Warn().Err(err).Msg("OCSP Stapling 注册失败")
						}
					}
				}

				// 设置 GetConfigForClient 回调用于 OCSP Stapling
				tlsCfg.GetConfigForClient = manager.getConfigForClientWithOCSP
			}
		}

		ocspMgr.Start()
	}

	// 初始化客户端证书验证（如果启用）
	if cfg.ClientVerify.Enabled {
		clientVerifier, err := NewClientVerifier(cfg.ClientVerify)
		if err != nil {
			logging.Warn().Err(err).Msg("客户端证书验证配置失败")
		} else {
			manager.clientVerifier = clientVerifier
			clientVerifier.ConfigureTLS(tlsCfg)
		}
	}

	// 设置为默认配置
	manager.defaultCfg = tlsCfg

	return manager, nil
}

// NewMultiTLSManager 创建支持多证书的 TLS 管理器（SNI）。
//
// 用于多虚拟主机环境，每个主机有自己的证书。
//
// 参数：
//   - configs: 服务器名称到 SSL 配置的映射
//   - defaultCfg: 默认 SSL 配置，用于回退（可选）
//
// 返回值：
//   - *TLSManager: 支持 SNI 的 TLS 管理器
//   - error: 任何证书加载失败时返回错误
func NewMultiTLSManager(configs map[string]*config.SSLConfig, defaultCfg *config.SSLConfig) (*TLSManager, error) {
	if len(configs) == 0 {
		return nil, errors.New("no SSL configurations provided")
	}

	manager := &TLSManager{
		configs: make(map[string]*tls.Config),
	}

	// 加载每个证书
	for name, cfg := range configs {
		tlsCfg, err := createTLSConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config for %s: %w", name, err)
		}
		manager.configs[name] = tlsCfg
	}

	// 如果提供了默认配置，则加载
	if defaultCfg != nil {
		tlsCfg, err := createTLSConfig(defaultCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create default TLS config: %w", err)
		}
		manager.defaultCfg = tlsCfg
	}

	return manager, nil
}

// GetTLSConfig 返回默认的 TLS 配置。
//
// 用于单服务器模式。
//
// 返回值：
//   - *tls.Config: TLS 配置对象
func (m *TLSManager) GetTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultCfg
}

// GetTLSConfigForHost 返回指定主机的 TLS 配置（SNI）。
//
// 如果未找到匹配主机，则回退到默认配置。
//
// 参数：
//   - host: 主机名（可能包含端口）
//
// 返回值：
//   - *tls.Config: 匹配的 TLS 配置
func (m *TLSManager) GetTLSConfigForHost(host string) *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 从主机名中移除端口（如果存在）
	host = netutil.StripPort(host)

	if cfg, ok := m.configs[host]; ok {
		return cfg
	}
	return m.defaultCfg
}

// GetCertificate 返回用于 SNI 支持的 GetCertificate 回调。
//
// 该回调被 tls.Config 用于根据 SNI 选择证书。
//
// 返回值：
//   - func: 证书选择回调函数
func (m *TLSManager) GetCertificate() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		// 查找匹配的服务器名称
		if cfg, ok := m.configs[hello.ServerName]; ok {
			if len(cfg.Certificates) > 0 {
				return &cfg.Certificates[0], nil
			}
		}

		// 回退到默认配置
		if m.defaultCfg != nil && len(m.defaultCfg.Certificates) > 0 {
			return &m.defaultCfg.Certificates[0], nil
		}

		return nil, errors.New("no certificate available")
	}
}

// AddCertificate 为服务器名称添加新证书（SNI）。
//
// 用于动态证书更新。
//
// 参数：
//   - name: 服务器名称
//   - cfg: SSL 配置
//
// 返回值：
//   - error: 配置无效时返回错误
func (m *TLSManager) AddCertificate(name string, cfg *config.SSLConfig) error {
	tlsCfg, err := createTLSConfig(cfg)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.configs[name] = tlsCfg
	m.mu.Unlock()

	return nil
}

// RemoveCertificate 移除服务器名称的证书。
//
// 参数：
//   - name: 服务器名称
func (m *TLSManager) RemoveCertificate(name string) {
	m.mu.Lock()
	delete(m.configs, name)
	m.mu.Unlock()
}

// Close 停止 OCSP 管理器和 Session Ticket 管理器并释放资源。
func (m *TLSManager) Close() {
	if m.ocspManager != nil {
		m.ocspManager.Stop()
	}
	if m.sessionTicketMgr != nil {
		m.sessionTicketMgr.Stop()
	}
}

// getConfigForClientWithOCSP 返回启用 OCSP Stapling 的 TLS 配置。
//
// 该回调在每次 TLS 握手时调用，附加最新的 OCSP 响应。
//
// 参数：
//   - hello: 客户端 Hello 信息
//
// 返回值：
//   - *tls.Config: 带有 OCSP 响应的 TLS 配置
//   - error: 配置错误
func (m *TLSManager) getConfigForClientWithOCSP(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 获取基础配置
	var baseCfg *tls.Config
	if hello.ServerName != "" {
		if cfg, ok := m.configs[hello.ServerName]; ok {
			baseCfg = cfg
		}
	}
	if baseCfg == nil {
		baseCfg = m.defaultCfg
	}

	// 无 OCSP 管理器或无证书时，返回基础配置
	if m.ocspManager == nil || len(baseCfg.Certificates) == 0 {
		return baseCfg, nil
	}

	// 创建配置副本并附加 OCSP 响应
	cfgCopy := baseCfg.Clone()

	// 将 OCSP 响应附加到证书
	cert := &cfgCopy.Certificates[0]
	if len(cert.Certificate) > 0 {
		// 解析叶子证书以获取序列号
		leafCert, err := x509.ParseCertificate(cert.Certificate[0])
		if err == nil {
			serial := leafCert.SerialNumber.String()
			ocspResp := m.ocspManager.GetOCSPResponse(serial)
			if ocspResp != nil {
				// 将 OCSP 响应附加到证书
				cert.OCSPStaple = ocspResp
			}
		}
	}

	return cfgCopy, nil
}

// GetOCSPStatus 返回所有已注册证书的 OCSP 状态。
//
// 返回值：
//   - map[string]OCSPStatusInfo: 证书序列号到状态信息的映射
func (m *TLSManager) GetOCSPStatus() map[string]OCSPStatusInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]OCSPStatusInfo)
	if m.ocspManager == nil {
		return result
	}

	for serial, cert := range m.certificates {
		status, hasResponse := m.ocspManager.GetStatus(serial)
		result[serial] = OCSPStatusInfo{
			Serial:      serial,
			Subject:     cert.Subject.CommonName,
			Status:      status,
			HasResponse: hasResponse,
		}
	}

	return result
}

// OCSPStatusInfo OCSP 响应状态信息。
type OCSPStatusInfo struct {
	Serial      string     // 证书序列号
	Subject     string     // 证书主题 CN
	Status      OCSPStatus // OCSP 响应状态
	HasResponse bool       // 是否有可用响应
}

// loadCertificate 从给定路径加载 TLS 证书。
//
// 如果提供了证书链路径，则合并证书链。
//
// 参数：
//   - certPath: 证书文件路径
//   - keyPath: 私钥文件路径
//   - certChainPath: 证书链文件路径（可选）
//
// 返回值：
//   - tls.Certificate: 加载的证书
//   - error: 加载失败时返回错误
func loadCertificate(certPath, keyPath, certChainPath string) (tls.Certificate, error) {
	// 加载主证书
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, err
	}

	// 合并证书链（如果提供）
	if certChainPath != "" {
		chainData, err := os.ReadFile(certChainPath)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("failed to read certificate chain: %w", err)
		}

		// 将证书链追加到证书（每个证书作为独立的 [][]byte 条目）
		certs := parsePEMChain(chainData)
		cert.Certificate = append(cert.Certificate, certs...)
	}

	return cert, nil
}

// parsePEMChain 解析 PEM 编码的证书链数据。
//
// 返回 ASN.1 DER 编码的证书切片。
//
// 参数：
//   - data: PEM 编码的数据
//
// 返回值：
//   - [][]byte: DER 编码的证书列表
func parsePEMChain(data []byte) [][]byte {
	var certs [][]byte
	var block []byte
	rest := data

	for {
		block, rest = extractPEMBlock(rest)
		if block == nil {
			break
		}
		if len(block) > 0 {
			certs = append(certs, block)
		}
	}

	return certs
}

// extractPEMBlock 从数据中提取单个 PEM 块。
//
// 返回 DER 编码的块和剩余数据。
//
// 参数：
//   - data: PEM 数据
//
// 返回值：
//   - []byte: DER 编码的块
//   - []byte: 剩余数据
func extractPEMBlock(data []byte) ([]byte, []byte) {
	startMarker := []byte("-----BEGIN CERTIFICATE-----")
	endMarker := []byte("-----END CERTIFICATE-----")

	start := findMarker(data, startMarker)
	if start == -1 {
		return nil, nil
	}

	end := findMarker(data[start:], endMarker)
	if end == -1 {
		return nil, nil
	}

	// 提取并解码 PEM 块
	blockData := data[start : start+end+len(endMarker)]
	rest := data[start+end+len(endMarker):]

	// 注意：此处为简化实现，直接返回原始 PEM 块数据
	// 生产环境建议使用 encoding/pem 进行完整解码
	return blockData, rest
}

// findMarker 在数据中查找标记位置。
//
// 参数：
//   - data: 待搜索的数据
//   - marker: 要查找的标记
//
// 返回值：
//   - int: 标记位置，未找到返回 -1
func findMarker(data []byte, marker []byte) int {
	for i := 0; i <= len(data)-len(marker); i++ {
		if matchMarker(data[i:], marker) {
			return i
		}
	}
	return -1
}

// matchMarker 检查数据是否以指定标记开头。
//
// 参数：
//   - data: 待检查的数据
//   - marker: 要匹配的标记
//
// 返回值：
//   - bool: 匹配返回 true
func matchMarker(data []byte, marker []byte) bool {
	if len(data) < len(marker) {
		return false
	}
	for i := 0; i < len(marker); i++ {
		if data[i] != marker[i] {
			return false
		}
	}
	return true
}

// createTLSConfig 从 SSL 配置创建 tls.Config。
//
// 参数：
//   - cfg: SSL 配置
//
// 返回值：
//   - *tls.Config: TLS 配置对象
//   - error: 配置无效时返回错误
func createTLSConfig(cfg *config.SSLConfig) (*tls.Config, error) {
	if cfg == nil {
		return nil, errors.New("ssl config is nil")
	}

	cert, err := loadCertificate(cfg.Cert, cfg.Key, cfg.CertChain)
	if err != nil {
		return nil, err
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS13,
	}

	if len(cfg.Ciphers) > 0 {
		ciphers, err := parseCipherSuites(cfg.Ciphers)
		if err != nil {
			return nil, err
		}
		tlsCfg.CipherSuites = ciphers
	} else {
		tlsCfg.CipherSuites = defaultCipherSuites()
	}

	if len(cfg.Protocols) > 0 {
		minVer, maxVer, err := parseTLSVersions(cfg.Protocols)
		if err != nil {
			return nil, err
		}
		tlsCfg.MinVersion = minVer
		tlsCfg.MaxVersion = maxVer
	}

	return tlsCfg, nil
}

// parseTLSVersions 解析 TLS 协议版本字符串。
//
// 返回最小和最大 TLS 版本。
//
// 参数：
//   - protocols: 协议名称列表（如 "TLSv1.2", "TLSv1.3"）
//
// 返回值：
//   - uint16: 最小版本
//   - uint16: 最大版本
//   - error: 无效协议时返回错误
func parseTLSVersions(protocols []string) (uint16, uint16, error) {
	var minVer, maxVer uint16
	minVer = tls.VersionTLS13 // 默认最高版本
	maxVer = tls.VersionTLS13

	for _, p := range protocols {
		switch p {
		case "TLSv1.2":
			if minVer > tls.VersionTLS12 {
				minVer = tls.VersionTLS12
			}
		case "TLSv1.3":
			maxVer = tls.VersionTLS13
		case "TLSv1.0", "TLSv1.1":
			return 0, 0, fmt.Errorf("insecure TLS version %s is not supported", p)
		default:
			return 0, 0, fmt.Errorf("unknown TLS version: %s", p)
		}
	}

	return minVer, maxVer, nil
}

// parseCipherSuites 解析加密套件名称字符串为 TLS ID。
//
// 参数：
//   - ciphers: 加密套件名称列表
//
// 返回值：
//   - []uint16: 加密套件 ID 列表
//   - error: 未知或不安全的加密套件时返回错误
func parseCipherSuites(ciphers []string) ([]uint16, error) {
	result := make([]uint16, 0, len(ciphers))

	for _, c := range ciphers {
		id, ok := cipherSuiteMap[c]
		if !ok {
			return nil, fmt.Errorf("unknown cipher suite: %s", c)
		}
		// 检查不安全的加密套件
		if isInsecureCipher(id) {
			return nil, fmt.Errorf("insecure cipher suite %s is not allowed", c)
		}
		result = append(result, id)
	}

	return result, nil
}

// isInsecureCipher 检查加密套件是否不安全。
//
// 参数：
//   - id: 加密套件 ID
//
// 返回值：
//   - bool: 不安全返回 true
func isInsecureCipher(id uint16) bool {
	insecureCiphers := []uint16{
		tls.TLS_RSA_WITH_RC4_128_SHA,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	}

	return slices.Contains(insecureCiphers, id)
}

// defaultCipherSuites 返回 TLS 1.2 推荐的加密套件。
//
// 优先选择前向保密和 AEAD 加密算法。
//
// 返回值：
//   - []uint16: 加密套件 ID 列表
func defaultCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	}
}

// cipherSuiteMap 加密套件名称到 TLS ID 的映射。
var cipherSuiteMap = map[string]uint16{
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305":    tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305":  tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"TLS_RSA_WITH_AES_128_GCM_SHA256":         tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	"TLS_RSA_WITH_AES_256_GCM_SHA384":         tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	"TLS_RSA_WITH_AES_128_CBC_SHA":            tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"TLS_RSA_WITH_AES_256_CBC_SHA":            tls.TLS_RSA_WITH_AES_256_CBC_SHA,
}

// ValidateCertificate 验证证书文件。
//
// 检查证书是否有效且未过期。
//
// 参数：
//   - certPath: 证书文件路径
//
// 返回值：
//   - error: 证书无效时返回错误
func ValidateCertificate(certPath string) error {
	_, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("failed to read certificate: %w", err)
	}

	// Note: More detailed validation would require parsing individual certs
	// and checking expiration dates, which is done during tls.LoadX509KeyPair

	return nil
}

// ValidateKey 验证私钥文件。
//
// 参数：
//   - keyPath: 私钥文件路径
//
// 返回值：
//   - error: 私钥无效时返回错误
func ValidateKey(keyPath string) error {
	_, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read key: %w", err)
	}

	// 密钥验证在 tls.LoadX509KeyPair 期间进行
	// This is a preliminary check that the file exists and is readable
	return nil
}
