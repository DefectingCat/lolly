// Package handler 提供 HTTP 请求处理器，包括路由、静态文件服务和零拷贝传输。
//
// 该文件包含自定义错误页面相关的核心逻辑，包括：
//   - 错误页面预加载
//   - 错误页面内容管理
//   - 错误页面查找
//
// 主要用途：
//
//	用于在服务器启动时预加载自定义错误页面文件到内存中，运行时不进行文件 I/O。
//
// 注意事项：
//   - 所有错误页面在启动时预加载
//   - 全部加载失败时会阻止服务器启动
//   - 部分加载失败会记录警告但允许启动
//
// 作者：xfy
package handler

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"rua.plus/lolly/internal/config"
)

// ErrorPageManager 自定义错误页面管理器。
//
// 负责在服务器启动时预加载错误页面文件到内存中，
// 并在运行时提供错误页面内容。
type ErrorPageManager struct {
	// pages 预加载的错误页面内容
	// key 为 HTTP 状态码，value 为页面内容
	pages map[int][]byte

	// defaultPage 默认错误页面内容
	defaultPage []byte

	// responseCode 响应状态码覆盖
	responseCode int

	// mu 保护 pages 的读写锁
	mu sync.RWMutex
}

// NewErrorPageManager 创建错误页面管理器。
//
// 根据配置预加载错误页面文件到内存中。
//
// 参数：
//   - cfg: 错误页面配置
//
// 返回值：
//   - *ErrorPageManager: 创建的错误页面管理器
//   - error: 预加载失败时的错误，全部失败时返回错误
//
// 使用示例：
//
//	manager, err := handler.NewErrorPageManager(&cfg.ErrorPage)
//	if err != nil {
//	    log.Fatal("加载错误页面失败:", err)
//	}
func NewErrorPageManager(cfg *config.ErrorPageConfig) (*ErrorPageManager, error) {
	if len(cfg.Pages) == 0 && cfg.Default == "" {
		// 没有配置错误页面，返回空管理器
		return &ErrorPageManager{
			pages:        make(map[int][]byte),
			responseCode: cfg.ResponseCode,
		}, nil
	}

	manager := &ErrorPageManager{
		pages:        make(map[int][]byte),
		responseCode: cfg.ResponseCode,
	}

	// 预加载特定状态码的错误页面
	loadErrors := make(map[int]error)
	for code, path := range cfg.Pages {
		content, err := os.ReadFile(path)
		if err != nil {
			loadErrors[code] = err
			continue
		}
		manager.pages[code] = content
	}

	// 预加载默认错误页面
	if cfg.Default != "" {
		content, err := os.ReadFile(cfg.Default)
		if err != nil {
			loadErrors[0] = err // 使用 0 表示默认页面错误
		} else {
			manager.defaultPage = content
		}
	}

	// 检查加载结果
	if len(loadErrors) > 0 {
		// 部分或全部加载失败
		totalPages := len(cfg.Pages)
		if cfg.Default != "" {
			totalPages++
		}

		if len(loadErrors) == totalPages {
			// 全部加载失败，返回错误
			errs := make([]error, 0, len(loadErrors))
			for _, e := range loadErrors {
				errs = append(errs, e)
			}
			return nil, fmt.Errorf("所有错误页面加载失败: %w", errors.Join(errs...))
		}
		// 部分失败，记录警告（由调用者处理）
		return manager, &PartialLoadError{Errors: loadErrors}
	}

	return manager, nil
}

// PartialLoadError 部分错误页面加载失败错误。
type PartialLoadError struct {
	Errors map[int]error
}

// Error 实现 error 接口。
//
// 返回值：
//   - string: 格式化的错误消息，包含失败的数量
func (e *PartialLoadError) Error() string {
	return fmt.Sprintf("部分错误页面加载失败: %d 个错误", len(e.Errors))
}

// GetPage 获取指定状态码的错误页面内容。
//
// 参数：
//   - code: HTTP 状态码
//
// 返回值：
//   - []byte: 错误页面内容，如未找到返回 nil
//   - bool: 是否找到
//   - int: 响应状态码（可能被覆盖）
func (m *ErrorPageManager) GetPage(code int) ([]byte, bool, int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 查找特定状态码的页面
	if content, ok := m.pages[code]; ok {
		responseCode := code
		if m.responseCode > 0 {
			responseCode = m.responseCode
		}
		return content, true, responseCode
	}

	// 使用默认页面
	if m.defaultPage != nil {
		responseCode := code
		if m.responseCode > 0 {
			responseCode = m.responseCode
		}
		return m.defaultPage, true, responseCode
	}

	return nil, false, code
}

// HasPage 检查是否有指定状态码的错误页面。
//
// 参数：
//   - code: HTTP 状态码
//
// 返回值：
//   - bool: 是否有该状态码的错误页面（包括默认页面）
func (m *ErrorPageManager) HasPage(code int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.pages[code]; ok {
		return true
	}
	return m.defaultPage != nil
}

// GetResponseCode 获取响应状态码覆盖值。
//
// 返回值：
//   - int: 响应状态码覆盖值，0 表示不覆盖
func (m *ErrorPageManager) GetResponseCode() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.responseCode
}

// IsConfigured 检查是否配置了错误页面。
//
// 返回值：
//   - bool: 是否配置了任何错误页面
func (m *ErrorPageManager) IsConfigured() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pages) > 0 || m.defaultPage != nil
}
