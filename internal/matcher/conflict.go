// Package matcher 提供 nginx 风格的 location 匹配引擎实现。
//
// 该文件实现路径冲突检测器，防止同一路径被重复注册为不同类型。
//
// 作者：xfy
package matcher

import "maps"

import "fmt"

// ConflictDetector 路径冲突检测器。
//
// 维护已注册路径的映射，在添加新 location 时检查是否存在冲突。
// 同一路径不能同时注册为多种匹配类型（如同时作为 exact 和 prefix）。
type ConflictDetector struct {
	// registeredPaths 已注册路径映射，key 为路径，value 为 location 类型
	registeredPaths map[string]string
}

// NewConflictDetector 创建冲突检测器。
//
// 返回值：
//   - *ConflictDetector: 冲突检测器实例
func NewConflictDetector() *ConflictDetector {
	return &ConflictDetector{
		registeredPaths: make(map[string]string),
	}
}

// Register 注册路径，如果已存在则返回冲突错误。
//
// 参数：
//   - path: 待注册的路径
//   - locationType: location 类型（exact/prefix/regex 等）
//
// 返回值：
//   - error: 路径已注册时返回冲突错误
func (cd *ConflictDetector) Register(path, locationType string) error {
	if existing, ok := cd.registeredPaths[path]; ok {
		return fmt.Errorf("path conflict: '%s' already registered as '%s', trying to register as '%s'",
			path, existing, locationType)
	}
	cd.registeredPaths[path] = locationType
	return nil
}

// Exists 检查路径是否已注册。
//
// 参数：
//   - path: 待检查的路径
//
// 返回值：
//   - bool: 已注册返回 true
func (cd *ConflictDetector) Exists(path string) bool {
	_, ok := cd.registeredPaths[path]
	return ok
}

// GetRegisteredPaths 返回所有已注册路径的副本。
//
// 返回值：
//   - map[string]string: 路径到 location 类型的映射
func (cd *ConflictDetector) GetRegisteredPaths() map[string]string {
	result := make(map[string]string, len(cd.registeredPaths))
	maps.Copy(result, cd.registeredPaths)
	return result
}

// Remove 移除已注册的路径。
//
// 参数：
//   - path: 要移除的路径
func (cd *ConflictDetector) Remove(path string) {
	delete(cd.registeredPaths, path)
}

// Clear 清空所有已注册路径。
func (cd *ConflictDetector) Clear() {
	cd.registeredPaths = make(map[string]string)
}
