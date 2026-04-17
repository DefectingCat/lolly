package matcher

import "fmt"

// ConflictDetector 冲突检测
type ConflictDetector struct {
	registeredPaths map[string]string // path -> location type
}

// NewConflictDetector 创建冲突检测器
func NewConflictDetector() *ConflictDetector {
	return &ConflictDetector{
		registeredPaths: make(map[string]string),
	}
}

// Register 注册路径，返回冲突错误
func (cd *ConflictDetector) Register(path, locationType string) error {
	if existing, ok := cd.registeredPaths[path]; ok {
		return fmt.Errorf("path conflict: '%s' already registered as '%s', trying to register as '%s'",
			path, existing, locationType)
	}
	cd.registeredPaths[path] = locationType
	return nil
}

// Exists 检查路径是否已注册
func (cd *ConflictDetector) Exists(path string) bool {
	_, ok := cd.registeredPaths[path]
	return ok
}

// GetRegisteredPaths 返回所有已注册路径
func (cd *ConflictDetector) GetRegisteredPaths() map[string]string {
	result := make(map[string]string, len(cd.registeredPaths))
	for k, v := range cd.registeredPaths {
		result[k] = v
	}
	return result
}

// Remove 移除已注册路径
func (cd *ConflictDetector) Remove(path string) {
	delete(cd.registeredPaths, path)
}

// Clear 清空所有注册路径
func (cd *ConflictDetector) Clear() {
	cd.registeredPaths = make(map[string]string)
}
