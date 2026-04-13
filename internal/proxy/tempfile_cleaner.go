// Package proxy 提供反向代理功能的临时文件清理。
//
// 该文件包含孤儿临时文件的清理功能：
//   - 定期扫描临时目录
//   - 删除过期的临时文件
//   - 后台 goroutine 运行
//
// 主要用途：
//
//	清理异常退出时遗留的临时文件。
//
// 注意事项：
//   - 只清理以 "lolly-proxy-" 前缀开头的文件
//   - 清理超过 1 小时的文件
//   - 可通过 stopCh 停止清理器
//
// 作者：xfy
package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TempFileCleaner 临时文件清理器。
//
// 定期清理临时目录中的孤儿文件。
//
// 注意事项：
//   - 清理器在后台运行
//   - 可通过 Stop 方法停止
//   - 只清理以 "lolly-proxy-" 前缀开头的文件
type TempFileCleaner struct {
	stopCh   chan struct{}
	tempPath string
	prefix   string
	interval time.Duration
	maxAge   time.Duration
	mu       sync.RWMutex
	stopped  bool
}

// DefaultCleanupInterval 默认清理间隔（5 分钟）。
const DefaultCleanupInterval = 5 * time.Minute

// DefaultMaxFileAge 默认文件最大存活时间（1 小时）。
const DefaultMaxFileAge = time.Hour

// TempFilePrefix 临时文件前缀。
const TempFilePrefix = "lolly-proxy-"

// NewTempFileCleaner 创建临时文件清理器。
//
// 参数：
//   - tempPath: 临时文件目录
//   - interval: 清理间隔（0 使用默认值 5 分钟）
//   - maxAge: 文件最大存活时间（0 使用默认值 1 小时）
//
// 返回值：
//   - *TempFileCleaner: 清理器实例
func NewTempFileCleaner(tempPath string, interval, maxAge time.Duration) *TempFileCleaner {
	if interval <= 0 {
		interval = DefaultCleanupInterval
	}
	if maxAge <= 0 {
		maxAge = DefaultMaxFileAge
	}
	if tempPath == "" {
		tempPath = os.TempDir()
	}

	return &TempFileCleaner{
		tempPath: tempPath,
		interval: interval,
		maxAge:   maxAge,
		prefix:   TempFilePrefix,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动清理器。
//
// 在后台启动一个 goroutine 定期清理临时文件。
func (c *TempFileCleaner) Start() {
	go c.run()
}

// Stop 停止清理器。
//
// 发送停止信号并等待清理器退出。
func (c *TempFileCleaner) Stop() {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	c.mu.Unlock()

	close(c.stopCh)
}

// IsStopped 检查清理器是否已停止。
func (c *TempFileCleaner) IsStopped() bool {
	c.mu.RLock()
	stopped := c.stopped
	c.mu.RUnlock()
	return stopped
}

// run 清理循环。
func (c *TempFileCleaner) run() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// 立即执行一次清理
	c.cleanup()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

// cleanup 执行一次清理。
func (c *TempFileCleaner) cleanup() {
	// 读取目录
	entries, err := os.ReadDir(c.tempPath)
	if err != nil {
		// 目录读取失败，跳过本次清理
		return
	}

	cutoff := time.Now().Add(-c.maxAge)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// 检查文件名前缀
		if !strings.HasPrefix(name, c.prefix) {
			continue
		}

		// 获取文件信息
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// 检查文件年龄
		if info.ModTime().After(cutoff) {
			continue
		}

		// 删除过期文件
		fullPath := filepath.Join(c.tempPath, name)
		_ = os.Remove(fullPath) //nolint:errcheck
	}
}

// GetTempPath 获取临时目录路径。
func (c *TempFileCleaner) GetTempPath() string {
	return c.tempPath
}

// GetInterval 获取清理间隔。
func (c *TempFileCleaner) GetInterval() time.Duration {
	return c.interval
}

// GetMaxAge 获取文件最大存活时间。
func (c *TempFileCleaner) GetMaxAge() time.Duration {
	return c.maxAge
}

// CleanupNow 立即执行一次清理（用于测试）。
func (c *TempFileCleaner) CleanupNow() {
	c.cleanup()
}

// CountOrphanFiles 统计孤儿临时文件数量。
//
// 返回值：
//   - int: 孤儿文件数量
func (c *TempFileCleaner) CountOrphanFiles() int {
	entries, err := os.ReadDir(c.tempPath)
	if err != nil {
		return 0
	}

	cutoff := time.Now().Add(-c.maxAge)
	count := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasPrefix(entry.Name(), c.prefix) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			count++
		}
	}

	return count
}

// globalCleaner 全局清理器实例。
var globalCleaner *TempFileCleaner
var globalCleanerMu sync.RWMutex

// StartGlobalTempFileCleaner 启动全局临时文件清理器。
//
// 参数：
//   - tempPath: 临时文件目录
func StartGlobalTempFileCleaner(tempPath string) {
	globalCleanerMu.Lock()
	defer globalCleanerMu.Unlock()

	if globalCleaner != nil {
		globalCleaner.Stop()
	}

	globalCleaner = NewTempFileCleaner(tempPath, 0, 0)
	globalCleaner.Start()
}

// StopGlobalTempFileCleaner 停止全局临时文件清理器。
func StopGlobalTempFileCleaner() {
	globalCleanerMu.Lock()
	defer globalCleanerMu.Unlock()

	if globalCleaner != nil {
		globalCleaner.Stop()
		globalCleaner = nil
	}
}

// GetGlobalTempFileCleaner 获取全局临时文件清理器。
//
// 返回值：
//   - *TempFileCleaner: 全局清理器实例（可能为 nil）
func GetGlobalTempFileCleaner() *TempFileCleaner {
	globalCleanerMu.RLock()
	defer globalCleanerMu.RUnlock()
	return globalCleaner
}
