//go:build !windows

// Package app 提供 Lolly 应用程序的生命周期管理和命令行入口。
//
// 包含测试工具函数，用于测试场景。
//
// 作者：xfy
package app

import (
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
)

// setupTestLogger 创建一个测试用的日志记录器。
// 返回一个使用默认配置的 AppLogger，适用于测试场景。
func setupTestLogger() *logging.AppLogger {
	return logging.NewAppLogger(&config.LoggingConfig{})
}
