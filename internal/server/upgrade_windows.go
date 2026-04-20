//go:build windows

// Package server 提供 Windows 平台的空实现 stub。
//
// Windows 不支持优雅升级（热升级）功能，该文件提供空的 stub
// 以满足编译依赖。
//
// 作者：xfy
package server

import (
	"net"
)

// UpgradeManager 空的升级管理器 stub（Windows 不支持）。
type UpgradeManager struct{}

// NewUpgradeManager 创建空的升级管理器 stub。
//
// Windows 平台不支持优雅升级（热升级）功能，该函数返回
// 一个空的 UpgradeManager 实例，所有方法均为空操作。
//
// 参数：
//   - server: 服务器实例（Windows 上不使用）
//
// 返回值：
//   - *UpgradeManager: 空操作的管理器实例
func NewUpgradeManager(server *Server) *UpgradeManager {
	return &UpgradeManager{}
}

// SetPidFile stub。
func (u *UpgradeManager) SetPidFile(path string) {}

// SetListeners stub。
func (u *UpgradeManager) SetListeners(listeners []net.Listener) {}

// WritePid stub。
func (u *UpgradeManager) WritePid() error { return nil }

// IsChild stub。
func (u *UpgradeManager) IsChild() bool { return false }

// GetInheritedListeners stub。
func (u *UpgradeManager) GetInheritedListeners() ([]net.Listener, error) {
	return nil, nil
}

// GracefulUpgrade stub（Windows 不支持）。
func (u *UpgradeManager) GracefulUpgrade(newBinary string) error {
	return nil // Windows 不支持热升级，静默忽略
}

// SetupSignalHandlers stub（Windows 不支持 SIGUSR2）。
func (u *UpgradeManager) SetupSignalHandlers(newBinary string) {}
