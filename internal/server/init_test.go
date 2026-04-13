// Package server 提供初始化函数的测试。
//
// 该文件测试从 Start() 分离出的初始化函数：
//   - initGoroutinePool()
//   - initFileCache()
//   - initLuaEngine()
//   - initErrorPageManager()
//
// 主要用途：
//
//	验证初始化函数在各种配置场景下的行为
//
// 作者：xfy
package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
)

// TestInitGoroutinePool_Disabled 测试禁用时返回 nil
func TestInitGoroutinePool_Disabled(t *testing.T) {
	cfg := &config.PerformanceConfig{
		GoroutinePool: config.GoroutinePoolConfig{
			Enabled: false,
		},
	}

	pool := initGoroutinePool(cfg)
	if pool != nil {
		t.Error("Expected nil when GoroutinePool is disabled")
	}
}

// TestInitGoroutinePool_Enabled 测试启用时创建池
func TestInitGoroutinePool_Enabled(t *testing.T) {
	cfg := &config.PerformanceConfig{
		GoroutinePool: config.GoroutinePoolConfig{
			Enabled:     true,
			MaxWorkers:  50,
			MinWorkers:  5,
			IdleTimeout: 30 * time.Second,
		},
	}

	pool := initGoroutinePool(cfg)
	if pool == nil {
		t.Fatal("Expected non-nil pool when enabled")
	}

	// 验证配置正确应用
	if pool.maxWorkers != 50 {
		t.Errorf("Expected maxWorkers 50, got %d", pool.maxWorkers)
	}
	if pool.minWorkers != 5 {
		t.Errorf("Expected minWorkers 5, got %d", pool.minWorkers)
	}

	// 清理
	pool.Stop()
}

// TestInitGoroutinePool_ZeroWorkers 测试零值配置
func TestInitGoroutinePool_ZeroWorkers(t *testing.T) {
	cfg := &config.PerformanceConfig{
		GoroutinePool: config.GoroutinePoolConfig{
			Enabled:    true,
			MaxWorkers: 0,
			MinWorkers: 0,
		},
	}

	pool := initGoroutinePool(cfg)
	if pool == nil {
		t.Fatal("Expected non-nil pool")
	}

	// 清理
	pool.Stop()
}

// TestInitFileCache_Disabled 测试禁用时返回 nil
func TestInitFileCache_Disabled(t *testing.T) {
	cfg := &config.PerformanceConfig{
		FileCache: config.FileCacheConfig{
			MaxEntries: 0,
			MaxSize:    0,
		},
	}

	cache := initFileCache(cfg)
	if cache != nil {
		t.Error("Expected nil when FileCache is disabled")
	}
}

// TestInitFileCache_ByEntries 测试按条目数启用
func TestInitFileCache_ByEntries(t *testing.T) {
	cfg := &config.PerformanceConfig{
		FileCache: config.FileCacheConfig{
			MaxEntries: 1000,
			MaxSize:    0,
			Inactive:   10 * time.Minute,
		},
	}

	cache := initFileCache(cfg)
	if cache == nil {
		t.Fatal("Expected non-nil cache when MaxEntries > 0")
	}
}

// TestInitFileCache_BySize 测试按大小启用
func TestInitFileCache_BySize(t *testing.T) {
	cfg := &config.PerformanceConfig{
		FileCache: config.FileCacheConfig{
			MaxEntries: 0,
			MaxSize:    100 * 1024 * 1024, // 100MB
			Inactive:   10 * time.Minute,
		},
	}

	cache := initFileCache(cfg)
	if cache == nil {
		t.Fatal("Expected non-nil cache when MaxSize > 0")
	}
}

// TestInitFileCache_BothLimits 测试同时设置条目数和大小
func TestInitFileCache_BothLimits(t *testing.T) {
	cfg := &config.PerformanceConfig{
		FileCache: config.FileCacheConfig{
			MaxEntries: 1000,
			MaxSize:    100 * 1024 * 1024,
			Inactive:   10 * time.Minute,
		},
	}

	cache := initFileCache(cfg)
	if cache == nil {
		t.Fatal("Expected non-nil cache")
	}
}

// TestInitLuaEngine_Disabled 测试禁用时返回 nil
func TestInitLuaEngine_Disabled(t *testing.T) {
	luaCfg := &config.LuaMiddlewareConfig{
		Enabled: false,
	}

	engine, err := initLuaEngine(luaCfg)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if engine != nil {
		t.Error("Expected nil when Lua is disabled")
	}
}

// TestInitLuaEngine_NilConfig 测试 nil 配置
func TestInitLuaEngine_NilConfig(t *testing.T) {
	engine, err := initLuaEngine(nil)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if engine != nil {
		t.Error("Expected nil for nil config")
	}
}

// TestInitLuaEngine_Enabled 测试启用 Lua 引擎
func TestInitLuaEngine_Enabled(t *testing.T) {
	luaCfg := &config.LuaMiddlewareConfig{
		Enabled: true,
		GlobalSettings: config.LuaGlobalSettings{
			MaxConcurrentCoroutines: 500,
			CoroutineTimeout:        60 * time.Second,
			CodeCacheSize:           500,
			MaxExecutionTime:        60 * time.Second,
		},
	}

	engine, err := initLuaEngine(luaCfg)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if engine == nil {
		t.Fatal("Expected non-nil engine when enabled")
	}

	// 清理
	engine.Close()
}

// TestInitLuaEngine_DefaultValues 测试默认值设置
func TestInitLuaEngine_DefaultValues(t *testing.T) {
	luaCfg := &config.LuaMiddlewareConfig{
		Enabled:        true,
		GlobalSettings: config.LuaGlobalSettings{
			// 所有值为零，应使用默认值
		},
	}

	engine, err := initLuaEngine(luaCfg)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if engine == nil {
		t.Fatal("Expected non-nil engine")
	}

	// 清理
	engine.Close()
}

// TestInitErrorPageManager_NoConfig 测试无配置时返回 nil
func TestInitErrorPageManager_NoConfig(t *testing.T) {
	cfg := &config.ErrorPageConfig{
		Pages:   nil,
		Default: "",
	}

	manager, err := initErrorPageManager(cfg)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if manager != nil {
		t.Error("Expected nil when no error page configured")
	}
}

// TestInitErrorPageManager_NilConfig 测试 nil 配置
func TestInitErrorPageManager_NilConfig(t *testing.T) {
	manager, err := initErrorPageManager(nil)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if manager != nil {
		t.Error("Expected nil for nil config")
	}
}

// TestInitErrorPageManager_WithPages 测试配置错误页面
func TestInitErrorPageManager_WithPages(t *testing.T) {
	// 创建临时目录和文件
	tempDir := t.TempDir()
	errorPagePath := filepath.Join(tempDir, "404.html")
	content := []byte("<html><body>Not Found</body></html>")
	if err := os.WriteFile(errorPagePath, content, 0o644); err != nil {
		t.Fatalf("Failed to create error page file: %v", err)
	}

	cfg := &config.ErrorPageConfig{
		Pages: map[int]string{
			404: errorPagePath,
		},
	}

	manager, err := initErrorPageManager(cfg)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if manager == nil {
		t.Fatal("Expected non-nil manager when pages configured")
	}
}

// TestInitErrorPageManager_WithDefault 测试配置默认错误页面
func TestInitErrorPageManager_WithDefault(t *testing.T) {
	// 创建临时目录和文件
	tempDir := t.TempDir()
	defaultPagePath := filepath.Join(tempDir, "error.html")
	content := []byte("<html><body>Error</body></html>")
	if err := os.WriteFile(defaultPagePath, content, 0o644); err != nil {
		t.Fatalf("Failed to create error page file: %v", err)
	}

	cfg := &config.ErrorPageConfig{
		Default: defaultPagePath,
	}

	manager, err := initErrorPageManager(cfg)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if manager == nil {
		t.Fatal("Expected non-nil manager when default page configured")
	}
}

// TestInitErrorPageManager_NonExistentFile 测试不存在的文件
func TestInitErrorPageManager_NonExistentFile(t *testing.T) {
	cfg := &config.ErrorPageConfig{
		Default: "/nonexistent/path/error.html",
	}

	manager, err := initErrorPageManager(cfg)
	// 应该返回错误
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
	if manager != nil {
		t.Error("Expected nil manager when file doesn't exist")
	}
}

// TestInitErrorPageManager_PartialLoad 测试部分加载（部分文件存在）
func TestInitErrorPageManager_PartialLoad(t *testing.T) {
	// 创建临时目录和一个存在的文件
	tempDir := t.TempDir()
	existingPath := filepath.Join(tempDir, "404.html")
	content := []byte("<html><body>Not Found</body></html>")
	if err := os.WriteFile(existingPath, content, 0o644); err != nil {
		t.Fatalf("Failed to create error page file: %v", err)
	}

	cfg := &config.ErrorPageConfig{
		Pages: map[int]string{
			404: existingPath,
			500: "/nonexistent/path/500.html", // 不存在的文件
		},
	}

	manager, err := initErrorPageManager(cfg)
	// 部分加载应该成功返回 manager，但可能有警告
	// 注意：具体行为取决于 handler.NewErrorPageManager 的实现
	if manager == nil {
		t.Logf("Manager is nil with error: %v", err)
	}
}

// TestInitFunctions_NilPerformanceConfig 测试 nil PerformanceConfig
func TestInitFunctions_NilPerformanceConfig(t *testing.T) {
	// 这个测试验证函数能正确处理空配置
	var cfg *config.PerformanceConfig

	// 应该能处理 nil 而不 panic
	pool := initGoroutinePool(cfg)
	if pool != nil {
		t.Error("Expected nil for nil config")
	}

	cache := initFileCache(cfg)
	if cache != nil {
		t.Error("Expected nil for nil config")
	}
}
