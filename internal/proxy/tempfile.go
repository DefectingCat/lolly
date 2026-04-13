// Package proxy 提供反向代理功能的临时文件处理。
//
// 该文件包含代理响应的临时文件存储功能，用于保护内存：
//   - 响应大小检测
//   - 超过阈值时写入临时文件
//   - 超过最大值时返回 502
//   - 响应完成后自动清理
//
// 主要用途：
//
//	大响应场景下避免内存溢出。
//
// 注意事项：
//   - 使用 bodylimit.ParseSize 解析大小字符串
//   - 临时文件在响应完成后删除
//   - 无 Content-Length 时动态检测
//
// 作者：xfy
package proxy

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/middleware/bodylimit"
)

// TempFileManager 临时文件管理器。
//
// 管理代理响应的临时文件存储，保护内存免受大响应影响。
//
// 注意事项：
//   - 阈值和最大值的解析在初始化时完成
//   - 临时文件在响应后自动删除
//   - 并发安全
type TempFileManager struct {
	activeFiles map[string]*TempFile
	tempPath    string
	threshold   int64
	maxSize     int64
	mu          sync.RWMutex
}

// TempFile 临时文件包装器。
//
// 包装临时文件，提供便捷的读写和自动清理功能。
type TempFile struct {
	// file 底层文件句柄
	file *os.File

	// path 文件路径
	path string

	// size 当前写入大小
	size int64

	// maxSize 最大允许大小
	maxSize int64

	// exceeded 是否超过最大大小
	exceeded bool
}

// NewTempFileManager 创建临时文件管理器。
//
// 参数：
//   - tempPath: 临时文件存储目录
//   - threshold: 触发阈值字符串（如 "1m"）
//   - maxSize: 最大大小字符串（如 "1024m"）
//
// 返回值：
//   - *TempFileManager: 临时文件管理器实例
//   - error: 解析大小失败时的错误
func NewTempFileManager(tempPath, threshold, maxSize string) (*TempFileManager, error) {
	// 解析阈值
	thresholdBytes, err := bodylimit.ParseSize(threshold)
	if err != nil {
		return nil, fmt.Errorf("解析 temp_file_threshold 失败: %w", err)
	}

	// 解析最大大小
	maxSizeBytes, err := bodylimit.ParseSize(maxSize)
	if err != nil {
		return nil, fmt.Errorf("解析 max_temp_file_size 失败: %w", err)
	}

	// 使用默认临时目录
	if tempPath == "" {
		tempPath = os.TempDir()
	}

	// 确保临时目录存在
	if err := os.MkdirAll(tempPath, 0755); err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}

	return &TempFileManager{
		tempPath:    tempPath,
		threshold:   thresholdBytes,
		maxSize:     maxSizeBytes,
		activeFiles: make(map[string]*TempFile),
	}, nil
}

// ShouldUseTempFile 检查是否应该使用临时文件。
//
// 参数：
//   - contentLength: Content-Length 值（-1 表示未知）
//
// 返回值：
//   - bool: 是否应使用临时文件
func (m *TempFileManager) ShouldUseTempFile(contentLength int64) bool {
	// 如果已知大小且超过阈值，使用临时文件
	if contentLength >= 0 && contentLength >= m.threshold {
		return true
	}
	// 未知大小时由调用方决定（动态检测）
	return false
}

// CreateTempFile 创建临时文件。
//
// 返回值：
//   - *TempFile: 临时文件实例
//   - error: 创建失败时的错误
func (m *TempFileManager) CreateTempFile() (*TempFile, error) {
	// 创建临时文件
	file, err := os.CreateTemp(m.tempPath, "lolly-proxy-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时文件失败: %w", err)
	}

	tf := &TempFile{
		file:    file,
		path:    file.Name(),
		maxSize: m.maxSize,
	}

	// 记录活动文件
	m.mu.Lock()
	m.activeFiles[tf.path] = tf
	m.mu.Unlock()

	return tf, nil
}

// GetThreshold 获取触发阈值。
func (m *TempFileManager) GetThreshold() int64 {
	return m.threshold
}

// GetMaxSize 获取最大大小限制。
func (m *TempFileManager) GetMaxSize() int64 {
	return m.maxSize
}

// RemoveTempFile 移除临时文件记录。
//
// 参数：
//   - path: 临时文件路径
func (m *TempFileManager) RemoveTempFile(path string) {
	m.mu.Lock()
	delete(m.activeFiles, path)
	m.mu.Unlock()
}

// GetActiveCount 获取活动临时文件数量。
func (m *TempFileManager) GetActiveCount() int {
	m.mu.RLock()
	count := len(m.activeFiles)
	m.mu.RUnlock()
	return count
}

// Write 写入数据到临时文件。
//
// 参数：
//   - data: 要写入的数据
//
// 返回值：
//   - int: 写入的字节数
//   - error: 写入失败或超过最大大小时的错误
func (tf *TempFile) Write(data []byte) (int, error) {
	if tf.exceeded {
		return 0, fmt.Errorf("response exceeds max_temp_file_size")
	}

	// 检查是否超过最大大小
	if tf.size+int64(len(data)) > tf.maxSize {
		tf.exceeded = true
		return 0, fmt.Errorf("response exceeds max_temp_file_size")
	}

	n, err := tf.file.Write(data)
	tf.size += int64(n)
	return n, err
}

// WriteTo 将临时文件内容写入响应。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - statusCode: HTTP 状态码
//
// 返回值：
//   - error: 写入失败时的错误
func (tf *TempFile) WriteTo(ctx *fasthttp.RequestCtx, statusCode int) error {
	// 关闭文件以便读取
	if err := tf.file.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}

	// 重新打开文件用于读取
	file, err := os.Open(tf.path)
	if err != nil {
		return fmt.Errorf("打开临时文件失败: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	// 设置状态码
	ctx.Response.SetStatusCode(statusCode)

	// 流式传输文件内容
	buf := make([]byte, 64*1024) // 64KB 缓冲区
	for {
		n, err := file.Read(buf)
		if n > 0 {
			ctx.Response.AppendBody(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取临时文件失败: %w", err)
		}
	}

	return nil
}

// Close 关闭并删除临时文件。
func (tf *TempFile) Close() error {
	if tf.file != nil {
		_ = tf.file.Close()
	}
	if tf.path != "" {
		_ = os.Remove(tf.path)
	}
	return nil
}

// IsExceeded 检查是否超过最大大小。
func (tf *TempFile) IsExceeded() bool {
	return tf.exceeded
}

// GetSize 获取当前写入大小。
func (tf *TempFile) GetSize() int64 {
	return tf.size
}

// GetPath 获取临时文件路径。
func (tf *TempFile) GetPath() string {
	return tf.path
}

// DynamicTempFileWriter 动态检测临时文件写入器。
//
// 用于未知 Content-Length 时的动态阈值检测。
type DynamicTempFileWriter struct {
	// manager 临时文件管理器
	manager *TempFileManager

	// tempFile 当前使用的临时文件（如果已切换）
	tempFile *TempFile

	// buffer 缓冲区（在未超过阈值前使用）
	buffer []byte

	// totalSize 已接收的总大小
	totalSize int64

	// threshold 触发阈值
	threshold int64

	// maxSize 最大大小限制
	maxSize int64

	// switched 是否已切换到临时文件
	switched bool

	// exceeded 是否超过最大大小
	exceeded bool
}

// NewDynamicTempFileWriter 创建动态临时文件写入器。
//
// 参数：
//   - manager: 临时文件管理器
//
// 返回值：
//   - *DynamicTempFileWriter: 动态写入器实例
func NewDynamicTempFileWriter(manager *TempFileManager) *DynamicTempFileWriter {
	return &DynamicTempFileWriter{
		manager:   manager,
		buffer:    make([]byte, 0, manager.GetThreshold()),
		threshold: manager.GetThreshold(),
		maxSize:   manager.GetMaxSize(),
	}
}

// Write 写入数据。
//
// 参数：
//   - data: 要写入的数据
//
// 返回值：
//   - error: 写入失败或超过最大大小时的错误
func (w *DynamicTempFileWriter) Write(data []byte) error {
	if w.exceeded {
		return fmt.Errorf("response exceeds max_temp_file_size")
	}

	dataLen := int64(len(data))

	// 检查是否超过最大大小
	if w.totalSize+dataLen > w.maxSize {
		w.exceeded = true
		return fmt.Errorf("response exceeds max_temp_file_size")
	}

	// 如果已经切换到临时文件，直接写入
	if w.switched {
		_, err := w.tempFile.Write(data)
		return err
	}

	// 检查是否需要切换到临时文件
	if w.totalSize+dataLen >= w.threshold {
		// 创建临时文件
		tf, err := w.manager.CreateTempFile()
		if err != nil {
			return err
		}
		w.tempFile = tf
		w.switched = true

		// 将缓冲区数据写入临时文件
		if len(w.buffer) > 0 {
			_, writeErr := w.tempFile.Write(w.buffer)
			if writeErr != nil {
				return writeErr
			}
			w.buffer = nil // 释放缓冲区
		}

		// 写入新数据
		_, err = w.tempFile.Write(data)
		if err != nil {
			return err
		}
	} else {
		// 继续累积到缓冲区
		w.buffer = append(w.buffer, data...)
	}

	w.totalSize += dataLen
	return nil
}

// Finalize 完成写入并返回结果。
//
// 参数：
//   - ctx: fasthttp 请求上下文
//   - statusCode: HTTP 状态码
//
// 返回值：
//   - error: 处理失败时的错误
func (w *DynamicTempFileWriter) Finalize(ctx *fasthttp.RequestCtx, statusCode int) error {
	if w.exceeded {
		return fmt.Errorf("response exceeds max_temp_file_size")
	}

	// 设置状态码
	ctx.Response.SetStatusCode(statusCode)

	// 如果使用了临时文件，流式传输
	if w.switched && w.tempFile != nil {
		// 关闭文件以便读取
		if err := w.tempFile.file.Close(); err != nil {
			return fmt.Errorf("关闭临时文件失败: %w", err)
		}

		// 重新打开文件用于读取
		file, err := os.Open(w.tempFile.path)
		if err != nil {
			return fmt.Errorf("打开临时文件失败: %w", err)
		}
		defer func() {
			_ = file.Close()
		}()

		// 流式传输文件内容
		buf := make([]byte, 64*1024) // 64KB 缓冲区
		for {
			n, err := file.Read(buf)
			if n > 0 {
				ctx.Response.AppendBody(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = file.Close()
				return fmt.Errorf("读取临时文件失败: %w", err)
			}
		}
		_ = file.Close()

		// 删除临时文件
		_ = os.Remove(w.tempFile.path)
		w.manager.RemoveTempFile(w.tempFile.path)
		return nil
	}

	// 未使用临时文件，直接返回缓冲区内容
	ctx.Response.SetBody(w.buffer)
	return nil
}

// IsExceeded 检查是否超过最大大小。
func (w *DynamicTempFileWriter) IsExceeded() bool {
	return w.exceeded
}

// GetTotalSize 获取总大小。
func (w *DynamicTempFileWriter) GetTotalSize() int64 {
	return w.totalSize
}

// Cleanup 清理资源。
func (w *DynamicTempFileWriter) Cleanup() {
	if w.tempFile != nil {
		_ = w.tempFile.Close()
		if w.tempFile.path != "" {
			w.manager.RemoveTempFile(w.tempFile.path)
		}
		w.tempFile = nil
	}
	w.buffer = nil
}

// defaultTempFileManager 默认临时文件管理器（未配置时使用）。
var defaultTempFileManager *TempFileManager
var defaultTempFileManagerOnce sync.Once

// GetDefaultTempFileManager 获取默认临时文件管理器。
//
// 默认配置：
//   - temp_path: 系统临时目录
//   - temp_file_threshold: 1m
//   - max_temp_file_size: 1024m
//
// 返回值：
//   - *TempFileManager: 默认临时文件管理器
func GetDefaultTempFileManager() *TempFileManager {
	defaultTempFileManagerOnce.Do(func() {
		manager, err := NewTempFileManager("", "1m", "1024m")
		if err != nil {
			// 默认配置不应该出错，但如果出错则创建一个最小配置
			manager = &TempFileManager{
				tempPath:    os.TempDir(),
				threshold:   1 << 20, // 1MB
				maxSize:     1 << 30, // 1GB
				activeFiles: make(map[string]*TempFile),
			}
		}
		defaultTempFileManager = manager
	})
	return defaultTempFileManager
}
