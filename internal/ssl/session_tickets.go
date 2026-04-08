// Package ssl 提供 SSL Session Tickets 支持。
//
// 该文件包含 TLS Session Tickets 密钥管理和轮换逻辑，包括：
//   - Session Ticket 密钥生成和加载
//   - 自动密钥轮换机制
//   - 多密钥保留策略（支持旧票据解密）
//   - 与 TLS 配置的集成
//
// Session Tickets 允许 TLS 1.3 会话恢复，避免完整握手，显著提升性能。
// 密钥定期轮换增强安全性，同时保留旧密钥确保已发放的票据仍可解密。
//
// 作者：xfy
package ssl

import (
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"rua.plus/lolly/internal/config"
)

const (
	// ticketKeySize Session Ticket 密钥大小（字节）
	// TLS 1.3 使用 32 字节的 AES-256-GCM 密钥
	ticketKeySize = 32

	// defaultRotateInterval 默认密钥轮换间隔
	defaultRotateInterval = time.Hour

	// defaultRetainKeys 默认保留的密钥数量
	// 至少保留 2 个密钥（当前 + 上一个）
	defaultRetainKeys = 3

	// minRetainKeys 最小保留密钥数量
	minRetainKeys = 2
)

// SessionTicketManager Session Ticket 密钥管理器。
//
// 管理 Session Ticket 密钥的生命周期，包括生成、轮换、存储和加载。
// 密钥按时间顺序排列，最新的密钥用于加密，所有密钥都可用于解密。
type SessionTicketManager struct {
	// keys 密钥列表，按生成时间排序（最新的在最后）
	// [old1, old2, current] 或 [old1, old2, old3, current]
	keys [][]byte

	// config 配置
	config config.SessionTicketsConfig

	// rotateTimer 密钥轮换定时器
	rotateTimer *time.Timer

	// stopCh 停止信号通道
	stopCh chan struct{}

	// mu 保护并发访问的读写锁
	mu sync.RWMutex

	// started 是否已启动
	started bool
}

// NewSessionTicketManager 创建新的 Session Ticket 管理器。
//
// 根据配置创建管理器，如 key_file 存在则加载现有密钥，
// 否则自动生成新密钥。
//
// 参数：
//   - cfg: Session Tickets 配置
//
// 返回值：
//   - *SessionTicketManager: 配置好的管理器
//   - error: 密钥加载或生成失败时返回错误
func NewSessionTicketManager(cfg config.SessionTicketsConfig) (*SessionTicketManager, error) {
	if !cfg.Enabled {
		return nil, errors.New("session tickets are disabled")
	}

	// 使用默认值
	rotateInterval := cfg.RotateInterval
	if rotateInterval <= 0 {
		rotateInterval = defaultRotateInterval
	}

	retainKeys := cfg.RetainKeys
	if retainKeys < minRetainKeys {
		retainKeys = defaultRetainKeys
	}

	manager := &SessionTicketManager{
		config: config.SessionTicketsConfig{
			Enabled:        cfg.Enabled,
			KeyFile:        cfg.KeyFile,
			RotateInterval: rotateInterval,
			RetainKeys:     retainKeys,
		},
		keys:   make([][]byte, 0, retainKeys),
		stopCh: make(chan struct{}),
	}

	// 尝试加载或生成初始密钥
	if cfg.KeyFile != "" {
		if err := manager.loadOrGenerateKey(); err != nil {
			return nil, fmt.Errorf("failed to initialize session ticket key: %w", err)
		}
	} else {
		// 没有指定密钥文件，生成内存中的密钥
		key, err := generateTicketKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate session ticket key: %w", err)
		}
		manager.keys = append(manager.keys, key)
	}

	return manager, nil
}

// Start 启动密钥轮换定时器。
//
// 按照配置的 rotate_interval 定期生成新密钥。
// 必须在调用 GetKeys 之前启动。
func (m *SessionTicketManager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()

	// 启动轮换定时器
	m.scheduleRotation()
}

// Stop 停止密钥轮换定时器。
//
// 停止后不再进行自动密钥轮换，但现有密钥仍然有效。
func (m *SessionTicketManager) Stop() {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	m.started = false
	m.mu.Unlock()

	close(m.stopCh)

	if m.rotateTimer != nil {
		m.rotateTimer.Stop()
	}
}

// GetKeys 返回当前所有有效的 Session Ticket 密钥。
//
// 返回的密钥按时间顺序排列，最新的在最后。
// TLS 配置应该使用最新的密钥加密，所有密钥都可以解密。
//
// 返回值：
//   - [][]byte: 密钥列表，每个密钥 32 字节
func (m *SessionTicketManager) GetKeys() [][]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 返回副本以防止外部修改
	result := make([][]byte, len(m.keys))
	for i, key := range m.keys {
		result[i] = make([]byte, len(key))
		copy(result[i], key)
	}
	return result
}

// RotateKey 手动轮换 Session Ticket 密钥。
//
// 生成新密钥并添加到密钥列表，如果超过 retain_keys 数量则移除最旧的密钥。
// 新密钥用于加密新票据，旧密钥仍可用于解密已发放的票据。
//
// 返回值：
//   - error: 密钥生成失败时返回错误
func (m *SessionTicketManager) RotateKey() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 生成新密钥
	newKey, err := generateTicketKey()
	if err != nil {
		return fmt.Errorf("failed to generate new ticket key: %w", err)
	}

	// 添加新密钥
	m.keys = append(m.keys, newKey)

	// 如果超过保留数量，移除最旧的密钥
	if len(m.keys) > m.config.RetainKeys {
		m.keys = m.keys[len(m.keys)-m.config.RetainKeys:]
	}

	// 如果有密钥文件，保存所有密钥
	if m.config.KeyFile != "" {
		if err := m.saveKeys(); err != nil {
			// 保存失败不影响运行，记录错误即可
			// 这里可以考虑添加日志
			_ = err
		}
	}

	return nil
}

// ApplyToTLSConfig 将 Session Ticket 密钥应用到 TLS 配置。
//
// 设置 tls.Config 的 SetSessionTicketKeys 回调，用于动态提供密钥。
//
// 参数：
//   - tlsCfg: TLS 配置对象
func (m *SessionTicketManager) ApplyToTLSConfig(tlsCfg *tls.Config) {
	if tlsCfg == nil {
		return
	}

	// 设置会话票据密钥
	// Go 的 crypto/tls 使用 SetSessionTicketKeys 方法设置密钥
	// 需要转换为 [][32]byte 类型
	keys := m.GetKeys()
	ticketKeys := make([][32]byte, len(keys))
	for i, key := range keys {
		if len(key) >= 32 {
			copy(ticketKeys[i][:], key)
		}
	}
	tlsCfg.SetSessionTicketKeys(ticketKeys)
}

// scheduleRotation 调度密钥轮换。
//
// 使用定时器在指定间隔后执行密钥轮换。
func (m *SessionTicketManager) scheduleRotation() {
	if !m.started {
		return
	}

	m.rotateTimer = time.AfterFunc(m.config.RotateInterval, func() {
		select {
		case <-m.stopCh:
			return
		default:
			_ = m.RotateKey()
			m.scheduleRotation()
		}
	})
}

// loadOrGenerateKey 从文件加载密钥或生成新密钥。
//
// 如果密钥文件存在，加载所有密钥；否则生成新密钥并保存。
//
// 返回值：
//   - error: 加载或生成失败时返回错误
func (m *SessionTicketManager) loadOrGenerateKey() error {
	// 尝试加载现有密钥
	if _, err := os.Stat(m.config.KeyFile); err == nil {
		// 文件存在，加载密钥
		if err := m.loadKeys(); err != nil {
			// 加载失败，生成新密钥
			return m.generateAndSaveKey()
		}
		return nil
	}

	// 文件不存在，生成新密钥
	return m.generateAndSaveKey()
}

// loadKeys 从文件加载所有密钥。
//
// 密钥文件格式：每个密钥 32 字节，连续存储
//
// 返回值：
//   - error: 读取或解析失败时返回错误
func (m *SessionTicketManager) loadKeys() error {
	data, err := os.ReadFile(m.config.KeyFile)
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	// 解析密钥（每个 32 字节）
	if len(data) < ticketKeySize {
		return errors.New("key file too small")
	}

	m.keys = make([][]byte, 0, m.config.RetainKeys)
	for i := 0; i+ticketKeySize <= len(data); i += ticketKeySize {
		key := make([]byte, ticketKeySize)
		copy(key, data[i:i+ticketKeySize])
		m.keys = append(m.keys, key)
	}

	// 如果加载的密钥超过保留数量，只保留最新的
	if len(m.keys) > m.config.RetainKeys {
		m.keys = m.keys[len(m.keys)-m.config.RetainKeys:]
	}

	// 确保至少有一个密钥
	if len(m.keys) == 0 {
		return errors.New("no valid keys found in file")
	}

	return nil
}

// saveKeys 将所有密钥保存到文件。
//
// 密钥文件格式：每个密钥 32 字节，连续存储
// 文件权限设置为 0600（仅所有者可读写）
//
// 返回值：
//   - error: 写入失败时返回错误
func (m *SessionTicketManager) saveKeys() error {
	// 计算总大小
	totalSize := len(m.keys) * ticketKeySize
	data := make([]byte, 0, totalSize)

	for _, key := range m.keys {
		data = append(data, key...)
	}

	// 使用 0600 权限写入文件（敏感数据，限制访问）
	if err := os.WriteFile(m.config.KeyFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	return nil
}

// generateAndSaveKey 生成新密钥并保存。
//
// 返回值：
//   - error: 生成或保存失败时返回错误
func (m *SessionTicketManager) generateAndSaveKey() error {
	key, err := generateTicketKey()
	if err != nil {
		return err
	}

	m.keys = [][]byte{key}

	if m.config.KeyFile != "" {
		if err := m.saveKeys(); err != nil {
			return err
		}
	}

	return nil
}

// generateTicketKey 生成新的随机 Session Ticket 密钥。
//
// 使用 crypto/rand 生成加密安全的随机密钥。
//
// 返回值：
//   - []byte: 32 字节的随机密钥
//   - error: 随机数生成失败时返回错误
func generateTicketKey() ([]byte, error) {
	key := make([]byte, ticketKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// GetKeyStatus 返回当前密钥状态信息。
//
// 用于监控和调试，显示当前密钥数量和轮换状态。
//
// 返回值：
//   - SessionTicketStatus: 密钥状态信息
type SessionTicketStatus struct {
	// KeyCount 当前密钥数量
	KeyCount int

	// RetainKeys 配置的最大保留密钥数
	RetainKeys int

	// RotateInterval 配置的轮换间隔
	RotateInterval time.Duration

	// Started 管理器是否已启动
	Started bool
}

// GetStatus 返回当前密钥状态。
//
// 返回值：
//   - SessionTicketStatus: 密钥状态信息
func (m *SessionTicketManager) GetStatus() SessionTicketStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return SessionTicketStatus{
		KeyCount:       len(m.keys),
		RetainKeys:     m.config.RetainKeys,
		RotateInterval: m.config.RotateInterval,
		Started:        m.started,
	}
}
