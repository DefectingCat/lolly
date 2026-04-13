// Package server 提供服务器初始化函数。
//
// 该文件包含 Start() 方法中分离出的可测试初始化函数：
//   - Goroutine 池初始化
//   - 文件缓存初始化
//   - Lua 引擎初始化
//   - 错误页面管理器初始化
//
// 主要用途：
//
//	将 Start() 方法中的初始化逻辑分离，便于单元测试
//
// 注意事项：
//   - 这些函数仅在 Server.Start() 内部调用
//   - 分离后保持原逻辑不变，仅提取为独立函数
//
// 作者：xfy
package server

import (
	"fmt"
	"time"

	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/handler"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/lua"
)

// initGoroutinePool 初始化 Goroutine 池。
//
// 根据配置创建并启动 Goroutine 池，用于优化请求处理性能。
//
// 参数：
//   - cfg: 性能配置，包含 GoroutinePool 配置
//
// 返回值：
//   - *GoroutinePool: 初始化的 Goroutine 池，未启用时返回 nil
func initGoroutinePool(cfg *config.PerformanceConfig) *GoroutinePool {
	if cfg == nil || !cfg.GoroutinePool.Enabled {
		return nil
	}

	pool := NewGoroutinePool(PoolConfig{
		MaxWorkers:  cfg.GoroutinePool.MaxWorkers,
		MinWorkers:  cfg.GoroutinePool.MinWorkers,
		IdleTimeout: cfg.GoroutinePool.IdleTimeout,
	})
	pool.Start()

	logging.Info().
		Int("maxWorkers", cfg.GoroutinePool.MaxWorkers).
		Int("minWorkers", cfg.GoroutinePool.MinWorkers).
		Msg("Goroutine 池已启动")

	return pool
}

// initFileCache 初始化文件缓存。
//
// 根据配置创建文件缓存实例，用于缓存静态文件内容。
//
// 参数：
//   - cfg: 性能配置，包含 FileCache 配置
//
// 返回值：
//   - *cache.FileCache: 初始化的文件缓存，未启用时返回 nil
func initFileCache(cfg *config.PerformanceConfig) *cache.FileCache {
	// 检查是否配置了缓存
	if cfg == nil || (cfg.FileCache.MaxEntries <= 0 && cfg.FileCache.MaxSize <= 0) {
		return nil
	}

	fileCache := cache.NewFileCache(
		cfg.FileCache.MaxEntries,
		cfg.FileCache.MaxSize,
		cfg.FileCache.Inactive,
	)

	logging.Info().
		Int64("maxEntries", cfg.FileCache.MaxEntries).
		Int64("maxSize", cfg.FileCache.MaxSize).
		Msg("文件缓存已启动")

	return fileCache
}

// initLuaEngine 初始化 Lua 引擎。
//
// 根据配置创建 Lua 引擎实例，用于执行 Lua 脚本。
//
// 参数：
//   - luaCfg: Lua 中间件配置
//
// 返回值：
//   - *lua.LuaEngine: 初始化的 Lua 引擎，未启用时返回 nil
//   - error: 初始化过程中遇到的错误
func initLuaEngine(luaCfg *config.LuaMiddlewareConfig) (*lua.LuaEngine, error) {
	if luaCfg == nil || !luaCfg.Enabled {
		return nil, nil
	}

	engineCfg := &lua.Config{
		MaxConcurrentCoroutines: luaCfg.GlobalSettings.MaxConcurrentCoroutines,
		CoroutineTimeout:        luaCfg.GlobalSettings.CoroutineTimeout,
		CodeCacheSize:           luaCfg.GlobalSettings.CodeCacheSize,
		CodeCacheTTL:            time.Hour, // 默认值
		EnableFileWatch:         luaCfg.GlobalSettings.EnableFileWatch,
		MaxExecutionTime:        luaCfg.GlobalSettings.MaxExecutionTime,
		EnableOSLib:             false, // 安全默认值
		EnableIOLib:             false,
		EnableLoadLib:           false,
	}

	// 设置默认值
	if engineCfg.MaxConcurrentCoroutines == 0 {
		engineCfg.MaxConcurrentCoroutines = 1000
	}
	if engineCfg.CoroutineTimeout == 0 {
		engineCfg.CoroutineTimeout = 30 * time.Second
	}
	if engineCfg.CodeCacheSize == 0 {
		engineCfg.CodeCacheSize = 1000
	}
	if engineCfg.MaxExecutionTime == 0 {
		engineCfg.MaxExecutionTime = 30 * time.Second
	}

	engine, err := lua.NewEngine(engineCfg)
	if err != nil {
		return nil, fmt.Errorf("初始化 Lua 引擎失败: %w", err)
	}

	logging.Info().Msg("Lua 引擎已启动")
	return engine, nil
}

// initErrorPageManager 初始化错误页面管理器。
//
// 根据配置创建错误页面管理器，用于加载和提供自定义错误页面。
//
// 参数：
//   - errorPageCfg: 错误页面配置
//
// 返回值：
//   - *handler.ErrorPageManager: 初始化的错误页面管理器，未配置时返回 nil
//   - error: 初始化过程中遇到的错误
func initErrorPageManager(errorPageCfg *config.ErrorPageConfig) (*handler.ErrorPageManager, error) {
	// 检查是否配置了错误页面
	if errorPageCfg == nil || (len(errorPageCfg.Pages) == 0 && errorPageCfg.Default == "") {
		return nil, nil
	}

	manager, err := handler.NewErrorPageManager(errorPageCfg)
	if err != nil {
		// 检查是否是部分加载失败
		if _, ok := err.(*handler.PartialLoadError); ok {
			logging.Warn().Msg("部分错误页面加载失败: " + err.Error())
			// 返回部分加载的管理器
			return manager, nil
		}
		// 全部加载失败
		return nil, fmt.Errorf("加载错误页面失败: %w", err)
	}

	logging.Info().Msg("错误页面管理器已启动")
	return manager, nil
}
