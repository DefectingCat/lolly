//go:build !windows

// Package server 提供了带中间件支持、虚拟主机和状态监控功能的 HTTP 服务器。
//
// 该文件实现了优雅升级（热升级）功能，支持在不中断服务的情况下
// 更新服务器二进制文件，实现零停机部署。
//
// 主要功能：
//   - 文件描述符继承：子进程继承父进程的监听套接字
//   - PID 文件管理：记录当前运行进程的 PID
//   - 信号处理：响应 SIGUSR2 触发升级，SIGQUIT 优雅关闭
//   - 进程协调：等待旧进程处理完现有请求后关闭
//
// 使用示例：
//
//	mgr := server.NewUpgradeManager(srv)
//	mgr.SetPidFile("/var/run/lolly.pid")
//	mgr.SetListeners(listeners)
//	mgr.SetupSignalHandlers("/path/to/new/binary")
//
// 作者：xfy
package server

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// UpgradeManager 优雅升级管理器。
//
// 管理服务器热升级的生命周期，包括 PID 文件管理、
// 监听器继承、新进程启动和旧进程关闭协调。
//
// 注意事项：
//   - 仅支持 Linux 和 Unix 系统
//   - 需要正确配置 PID 文件路径
//   - 监听器必须在升级前设置
type UpgradeManager struct {
	server    *Server
	pidFile   string
	listeners []net.Listener
	oldPid    int
}

// NewUpgradeManager 创建升级管理器。
//
// 参数：
//   - server: 服务器实例
//
// 返回值：
//   - *UpgradeManager: 初始化的升级管理器
func NewUpgradeManager(server *Server) *UpgradeManager {
	return &UpgradeManager{
		server: server,
	}
}

// SetPidFile 设置 PID 文件路径。
//
// PID 文件用于记录当前运行进程的 PID，支持升级时识别旧进程。
//
// 参数：
//   - path: PID 文件的完整路径（如 "/var/run/lolly.pid"）
func (u *UpgradeManager) SetPidFile(path string) {
	u.pidFile = path
}

// SetListeners 设置监听器列表。
//
// 设置将在升级时传递给子进程的监听器。
// 子进程通过继承文件描述符无缝接管这些监听器。
//
// 参数：
//   - listeners: 待继承的网络监听器列表
func (u *UpgradeManager) SetListeners(listeners []net.Listener) {
	u.listeners = listeners
}

// WritePid 写入当前进程 PID 到文件。
//
// 将当前进程的 PID 写入配置的 PID 文件，用于后续升级时识别。
//
// 返回值：
//   - error: 写入失败时返回错误，未配置 PID 文件时返回 nil
func (u *UpgradeManager) WritePid() error {
	if u.pidFile == "" {
		return nil
	}

	pid := os.Getpid()
	return os.WriteFile(u.pidFile, []byte(fmt.Sprintf("%d", pid)), 0o644)
}

// ReadOldPid 读取旧进程 PID。
//
// 从 PID 文件读取当前运行的服务器进程 PID。
//
// 返回值：
//   - int: 旧进程的 PID
//   - error: 文件不存在或解析失败时返回错误
func (u *UpgradeManager) ReadOldPid() (int, error) {
	if u.pidFile == "" {
		return 0, fmt.Errorf("pid file not configured")
	}

	data, err := os.ReadFile(u.pidFile)
	if err != nil {
		return 0, err
	}

	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	return pid, err
}

// IsChild 检查当前进程是否是升级启动的子进程。
//
// 通过检查 GRACEFUL_UPGRADE 环境变量判断。
// 子进程应从继承的监听器启动，而非创建新监听器。
//
// 返回值：
//   - bool: true 表示是升级启动的子进程
func (u *UpgradeManager) IsChild() bool {
	return os.Getenv("GRACEFUL_UPGRADE") == "1"
}

// GetInheritedListeners 获取从父进程继承的监听器。
//
// 从 LISTEN_FDS 环境变量读取继承的文件描述符数量，
// 并重建监听器对象。
//
// 返回值：
//   - []net.Listener: 继承的监听器列表
//   - error: 解析或重建失败时返回错误
func (u *UpgradeManager) GetInheritedListeners() ([]net.Listener, error) {
	fdsStr := os.Getenv("LISTEN_FDS")
	if fdsStr == "" {
		return nil, nil // 不是升级启动
	}

	var fdCount int
	_, err := fmt.Sscanf(fdsStr, "%d", &fdCount)
	if err != nil {
		return nil, err
	}

	var listeners []net.Listener

	// 文件描述符从 3 开始（0=stdin, 1=stdout, 2=stderr）
	for i := 3; i < 3+fdCount; i++ {
		file := os.NewFile(uintptr(i), fmt.Sprintf("listener-%d", i))
		if file == nil {
			continue
		}

		listener, err := net.FileListener(file)
		if err != nil {

			_ = file.Close()
			continue
		}

		listeners = append(listeners, listener)
	}

	return listeners, nil
}

// GracefulUpgrade 执行优雅升级。
//
// 启动新的服务器进程，传递监听器文件描述符。
// 新进程启动后，旧进程可以优雅关闭。
//
// 参数：
//   - newBinary: 新服务器二进制文件的路径
//
// 返回值：
//   - error: 启动新进程失败时返回错误
func (u *UpgradeManager) GracefulUpgrade(newBinary string) error {
	if len(u.listeners) == 0 {
		return fmt.Errorf("no listeners configured for upgrade")
	}

	// 准备环境变量
	env := os.Environ()
	env = append(env, "GRACEFUL_UPGRADE=1")
	env = append(env, fmt.Sprintf("LISTEN_FDS=%d", len(u.listeners)))

	// 获取监听器的文件描述符
	files := make([]*os.File, 0, len(u.listeners))
	for _, listener := range u.listeners {
		file, err := listenerFile(listener)
		if err != nil {
			return fmt.Errorf("failed to get listener file: %w", err)
		}
		files = append(files, file)
	}

	// 启动新进程
	execPath, err := filepath.Abs(newBinary)
	if err != nil {
		return err
	}

	cmd := exec.Command(execPath)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = files // 传递监听器文件描述符

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start new process: %w", err)
	}

	newPid := cmd.Process.Pid
	u.oldPid = os.Getpid()

	// 写入新 PID 到文件
	if u.pidFile != "" {

		_ = os.WriteFile(u.pidFile, []byte(fmt.Sprintf("%d", newPid)), 0o644)
	}

	// 启动 goroutine 等待子进程结束，避免产生僵尸进程
	// cmd.Wait() 会回收子进程资源，确保不会产生 defunct 进程
	go func() {

		_ = cmd.Wait()
	}()

	// 关闭父进程中的文件描述符副本（子进程已继承）
	// 避免文件描述符泄漏
	for _, file := range files {

		_ = file.Close()
	}

	return nil
}

// listenerFile 从 net.Listener 获取底层的文件描述符。
//
// 支持 TCP 和 Unix 套接字监听器。
//
// 参数：
//   - listener: 网络监听器
//
// 返回值：
//   - *os.File: 监听器对应的文件对象
//   - error: 不支持的监听器类型时返回错误
func listenerFile(listener net.Listener) (*os.File, error) {
	switch l := listener.(type) {
	case *net.TCPListener:
		return l.File()
	case *net.UnixListener:
		return l.File()
	default:
		return nil, fmt.Errorf("unsupported listener type: %T", listener)
	}
}

// WaitForShutdown 等待旧进程关闭。
//
// 轮询检查旧进程是否已退出，超时后强制发送 SIGKILL。
//
// 参数：
//   - timeout: 最大等待时间
//
// 返回值：
//   - error: 超时时返回错误，旧进程正常退出返回 nil
func (u *UpgradeManager) WaitForShutdown(timeout time.Duration) error {
	if u.oldPid == 0 {
		return nil
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		process, err := os.FindProcess(u.oldPid)
		if err != nil {
			return nil
		}

		err = process.Signal(syscall.Signal(0))
		if err != nil {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	process, _ := os.FindProcess(u.oldPid)

	_ = process.Signal(syscall.SIGKILL)
	return fmt.Errorf("old process did not shutdown gracefully")
}

// NotifyOldProcess 通知旧进程关闭。
//
// 向旧进程发送 SIGQUIT 信号，触发优雅关闭。
//
// 返回值：
//   - error: 发送信号失败时返回错误
func (u *UpgradeManager) NotifyOldProcess() error {
	oldPid, err := u.ReadOldPid()
	if err != nil || oldPid == 0 {
		return nil
	}

	process, err := os.FindProcess(oldPid)
	if err != nil {
		return nil
	}

	return process.Signal(syscall.SIGQUIT)
}

// SetupSignalHandlers 设置升级相关的信号处理器。
//
// 注册 SIGUSR2 信号处理器，收到信号时执行优雅升级。
// 可以通过 kill -USR2 <pid> 触发升级。
//
// 参数：
//   - newBinary: 新服务器二进制文件的路径
func (u *UpgradeManager) SetupSignalHandlers(newBinary string) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR2)

	go func() {
		for sig := range sigCh {
			if sig == syscall.SIGUSR2 {

				_ = u.GracefulUpgrade(newBinary)
			}
		}
	}()
}
