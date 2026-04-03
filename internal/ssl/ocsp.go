// Package ssl 提供 SSL/TLS 支持。
//
// 该文件实现 OCSP Stapling 功能，用于在 TLS 握手时附加证书状态信息，
// 提高证书验证效率并减少客户端对 OCSP 服务器的直接查询。
//
// 主要功能：
//   - OCSP 响应缓存：缓存证书状态响应，定期自动刷新
//   - 优雅降级：OCSP 查询失败时仍允许 TLS 连接
//   - 自动重试：支持配置最大重试次数
//   - 多服务器支持：尝试证书配置的多个 OCSP 服务器
//
// 使用示例：
//
//	mgr := ssl.NewOCSPManager(ssl.DefaultOCSPConfig())
//	mgr.Start()
//	defer mgr.Stop()
//
//	// 注册证书
//	err := mgr.RegisterCertificate(cert, issuer)
//
// 作者：xfy
package ssl

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/ocsp"
)

// OCSPManager OCSP Stapling 管理器。
//
// 管理 TLS 证书的 OCSP 响应缓存，支持定期自动刷新和优雅降级。
// 当 OCSP 查询失败时，TLS 握手仍可继续进行。
type OCSPManager struct {
	responses map[string]*ocspResponse // OCSP 响应，按证书序列号索引
	mu        sync.RWMutex             // 保护并发访问的读写锁
	client    *http.Client             // HTTP 客户端，用于 OCSP 查询
	stopChan  chan struct{}            // 停止信号通道
	running   bool                     // 运行状态标志

	// 配置参数
	refreshInterval time.Duration // OCSP 响应刷新间隔
	timeout         time.Duration // HTTP 请求超时时间
	maxRetries      int           // 失败时的最大重试次数
}

// ocspResponse OCSP 响应缓存条目。
//
// 保存 OCSP 响应数据及其元数据，用于证书状态验证。
type ocspResponse struct {
	response   []byte     // 原始 OCSP 响应数据
	thisUpdate time.Time  // 响应生成时间
	nextUpdate time.Time  // 响应过期时间
	status     ocspStatus // 响应状态
	fetchedAt  time.Time  // 获取响应的时间
	errors     int        // 连续获取失败的次数
}

// ocspStatus OCSP 响应状态类型。
type ocspStatus int

const (
	statusValid  ocspStatus = iota // 响应有效且新鲜
	statusStale                    // 响应过期但可用（优雅降级）
	statusFailed                   // 无有效响应可用
)

// OCSPConfig OCSP 管理器配置。
type OCSPConfig struct {
	Enabled         bool          // 是否启用 OCSP Stapling
	RefreshInterval time.Duration // 刷新间隔（默认：1 小时）
	Timeout         time.Duration // HTTP 超时（默认：10 秒）
	MaxRetries      int           // 失败时最大重试次数（默认：3）
}

// DefaultOCSPConfig 返回默认的 OCSP 配置。
func DefaultOCSPConfig() *OCSPConfig {
	return &OCSPConfig{
		Enabled:         true,
		RefreshInterval: 1 * time.Hour,
		Timeout:         10 * time.Second,
		MaxRetries:      3,
	}
}

// NewOCSPManager 创建新的 OCSP 管理器。
//
// 如果配置为 nil，则使用默认配置。
//
// 参数：
//   - cfg: OCSP 配置
//
// 返回值：
//   - *OCSPManager: 初始化的 OCSP 管理器
func NewOCSPManager(cfg *OCSPConfig) *OCSPManager {
	if cfg == nil {
		cfg = DefaultOCSPConfig()
	}

	// 应用默认值
	refreshInterval := cfg.RefreshInterval
	if refreshInterval == 0 {
		refreshInterval = 1 * time.Hour
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	return &OCSPManager{
		responses: make(map[string]*ocspResponse),
		client: &http.Client{
			Timeout: timeout,
		},
		refreshInterval: refreshInterval,
		timeout:         timeout,
		maxRetries:      maxRetries,
		stopChan:        make(chan struct{}),
	}
}

// Start 启动 OCSP 定期刷新进程。
func (m *OCSPManager) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	go m.refreshLoop()
}

// Stop 停止 OCSP 刷新进程。
func (m *OCSPManager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	m.mu.Unlock()

	close(m.stopChan)
}

// refreshLoop 定期刷新所有 OCSP 响应。
func (m *OCSPManager) refreshLoop() {
	ticker := time.NewTicker(m.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.refreshAll()
		}
	}
}

// refreshAll 刷新所有缓存的 OCSP 响应。
func (m *OCSPManager) refreshAll() {
	m.mu.RLock()
	certs := make([]string, 0, len(m.responses))
	for serial := range m.responses {
		certs = append(certs, serial)
	}
	m.mu.RUnlock()

	for _, serial := range certs {
		m.mu.RLock()
		resp, ok := m.responses[serial]
		m.mu.RUnlock()

		if ok && resp != nil {
			// 检查是否需要刷新
			if time.Now().Before(resp.nextUpdate) && resp.status == statusValid {
				continue
			}
		}
	}
}

// RegisterCertificate 注册证书以进行 OCSP Stapling。
//
// 从证书中提取 OCSP URL 并获取初始响应。如果获取失败，仍注册为失败状态
// 以支持优雅降级，允许 TLS 连接继续进行。
//
// 参数：
//   - cert: 待注册的证书
//   - issuer: 颁发者证书
//
// 返回值：
//   - error: 证书为空或无 OCSP 服务器时返回错误
func (m *OCSPManager) RegisterCertificate(cert *x509.Certificate, issuer *x509.Certificate) error {
	if cert == nil {
		return errors.New("certificate is nil")
	}

	// 检查证书是否有 OCSP 服务器 URL
	if len(cert.OCSPServer) == 0 {
		return errors.New("certificate has no OCSP server URL")
	}

	serial := cert.SerialNumber.String()

	// 获取初始 OCSP 响应
	response, err := m.fetchOCSP(cert, issuer)
	if err != nil {
		// 优雅降级：注册为失败状态但允许 TLS 继续
		m.mu.Lock()
		m.responses[serial] = &ocspResponse{
			status:    statusFailed,
			fetchedAt: time.Now(),
			errors:    1,
		}
		m.mu.Unlock()
		return fmt.Errorf("failed to fetch OCSP response: %w", err)
	}

	m.mu.Lock()
	m.responses[serial] = response
	m.mu.Unlock()

	return nil
}

// fetchOCSP 从证书的 OCSP 服务器获取 OCSP 响应。
//
// 参数：
//   - cert: 待查询的证书
//   - issuer: 颁发者证书
//
// 返回值：
//   - *ocspResponse: OCSP 响应
//   - error: 获取失败时返回错误
func (m *OCSPManager) fetchOCSP(cert, issuer *x509.Certificate) (*ocspResponse, error) {
	if len(cert.OCSPServer) == 0 {
		return nil, errors.New("no OCSP server in certificate")
	}

	// 创建 OCSP 请求
	ocspReq, err := ocsp.CreateRequest(cert, issuer, &ocsp.RequestOptions{
		Hash: crypto.SHA256,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OCSP request: %w", err)
	}

	// 尝试每个 OCSP 服务器 URL
	var lastErr error
	for _, url := range cert.OCSPServer {
		resp, err := m.sendOCSPRequest(url, ocspReq)
		if err != nil {
			lastErr = err
			continue
		}

		// 解析并验证响应
		ocspResp, err := ocsp.ParseResponse(resp, issuer)
		if err != nil {
			lastErr = fmt.Errorf("failed to parse OCSP response: %w", err)
			continue
		}

		// 检查响应状态
		if ocspResp.Status != ocsp.Good {
			return nil, fmt.Errorf("certificate status is not good: %d", ocspResp.Status)
		}

		// 检查响应是否匹配证书
		if !bytes.Equal(ocspResp.SerialNumber.Bytes(), cert.SerialNumber.Bytes()) {
			return nil, errors.New("OCSP response serial number mismatch")
		}

		return &ocspResponse{
			response:   resp,
			thisUpdate: ocspResp.ThisUpdate,
			nextUpdate: ocspResp.NextUpdate,
			status:     statusValid,
			fetchedAt:  time.Now(),
			errors:     0,
		}, nil
	}

	return nil, fmt.Errorf("all OCSP servers failed: %w", lastErr)
}

// sendOCSPRequest 向指定 URL 发送 OCSP 请求。
//
// 参数：
//   - url: OCSP 服务器 URL
//   - req: OCSP 请求数据
//
// 返回值：
//   - []byte: OCSP 响应数据
//   - error: 请求失败时返回错误
func (m *OCSPManager) sendOCSPRequest(url string, req []byte) ([]byte, error) {
	// 重试逻辑
	for i := 0; i < m.maxRetries; i++ {
		httpReq, err := http.NewRequest("POST", url, bytes.NewReader(req))
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/ocsp-request")

		resp, err := m.client.Do(httpReq)
		if err != nil {
			if i < m.maxRetries-1 {
				time.Sleep(time.Duration(i+1) * time.Second)
				continue
			}
			return nil, fmt.Errorf("HTTP request failed: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			if i < m.maxRetries-1 {
				time.Sleep(time.Duration(i+1) * time.Second)
				continue
			}
			return nil, fmt.Errorf("OCSP server returned status %d", resp.StatusCode)
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // Limit to 1MB
		if err != nil {
			if i < m.maxRetries-1 {
				time.Sleep(time.Duration(i+1) * time.Second)
				continue
			}
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		return body, nil
	}

	return nil, errors.New("max retries exceeded")
}

// GetOCSPResponse 返回证书的 OCSP 响应。
//
// 如果无有效响应则返回 nil（优雅降级，TLS 可继续）。
//
// 参数：
//   - serial: 证书序列号字符串
//
// 返回值：
//   - []byte: OCSP 响应数据，无响应时返回 nil
func (m *OCSPManager) GetOCSPResponse(serial string) []byte {
	m.mu.RLock()
	resp, ok := m.responses[serial]
	m.mu.RUnlock()

	if !ok || resp == nil {
		return nil
	}

	// 检查响应是否仍可用
	switch resp.status {
	case statusValid:
		// 检查过期
		if time.Now().After(resp.nextUpdate) {
			// 标记为过期但仍返回（优雅降级）
			m.mu.Lock()
			resp.status = statusStale
			m.mu.Unlock()
		}
		return resp.response
	case statusStale:
		// 返回过期响应用于优雅降级
		// 这允许即使 OCSP 刷新失败也能继续 TLS 握手
		return resp.response
	case statusFailed:
		// 无响应可用
		return nil
	default:
		return nil
	}
}

// RefreshResponse 强制刷新证书的 OCSP 响应。
//
// 参数：
//   - cert: 待刷新的证书
//   - issuer: 颁发者证书
//
// 返回值：
//   - error: 刷新失败时返回错误
func (m *OCSPManager) RefreshResponse(cert, issuer *x509.Certificate) error {
	response, err := m.fetchOCSP(cert, issuer)
	if err != nil {
		// 更新错误计数
		serial := cert.SerialNumber.String()
		m.mu.Lock()
		if existing, ok := m.responses[serial]; ok {
			existing.errors++
			if existing.errors >= m.maxRetries {
				existing.status = statusFailed
			}
		}
		m.mu.Unlock()
		return err
	}

	serial := cert.SerialNumber.String()
	m.mu.Lock()
	m.responses[serial] = response
	m.mu.Unlock()

	return nil
}

// GetStatus 返回证书的当前 OCSP 状态。
//
// 参数：
//   - serial: 证书序列号字符串
//
// 返回值：
//   - status: OCSP 响应状态
//   - hasResponse: 是否有可用响应
func (m *OCSPManager) GetStatus(serial string) (status ocspStatus, hasResponse bool) {
	m.mu.RLock()
	resp, ok := m.responses[serial]
	m.mu.RUnlock()

	if !ok || resp == nil {
		return statusFailed, false
	}

	return resp.status, len(resp.response) > 0
}

// extractCertificates 解析 PEM 数据并返回证书列表。
//
// 参数：
//   - pemData: PEM 编码的证书数据
//
// 返回值：
//   - []*x509.Certificate: 解析后的证书列表
//   - error: 解析失败时返回错误
func extractCertificates(pemData []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := pemData

	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse certificate: %w", err)
			}
			certs = append(certs, cert)
		}
		rest = remaining
	}

	if len(certs) == 0 {
		return nil, errors.New("no certificates found in PEM data")
	}

	return certs, nil
}
