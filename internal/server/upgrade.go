// Package server 提供优雅升级（热升级）功能，实现零停机部署。
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
type UpgradeManager struct {
	server    *Server
	pidFile   string // PID 文件路径
	oldPid    int    // 旧进程 PID
	listeners []net.Listener
}

// NewUpgradeManager 创建升级管理器。
func NewUpgradeManager(server *Server) *UpgradeManager {
	return &UpgradeManager{
		server: server,
	}
}

// SetPidFile 设置 PID 文件路径。
func (u *UpgradeManager) SetPidFile(path string) {
	u.pidFile = path
}

// SetListeners 设置监听器列表（用于升级时继承）。
func (u *UpgradeManager) SetListeners(listeners []net.Listener) {
	u.listeners = listeners
}

// WritePid 写入当前进程 PID。
func (u *UpgradeManager) WritePid() error {
	if u.pidFile == "" {
		return nil
	}

	pid := os.Getpid()
	return os.WriteFile(u.pidFile, []byte(fmt.Sprintf("%d", pid)), 0644)
}

// ReadOldPid 读取旧进程 PID。
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

// IsChild 检查是否是子进程（从升级启动）。
func (u *UpgradeManager) IsChild() bool {
	return os.Getenv("GRACEFUL_UPGRADE") == "1"
}

// GetInheritedListeners 获取继承的监听器。
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

	for i := 3; i < 3+fdCount; i++ {
		file := os.NewFile(uintptr(i), fmt.Sprintf("listener-%d", i))
		if file == nil {
			continue
		}

		listener, err := net.FileListener(file)
		if err != nil {
			file.Close()
			continue
		}

		listeners = append(listeners, listener)
	}

	return listeners, nil
}

// GracefulUpgrade 执行优雅升级。
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
	cmd.ExtraFiles = files

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start new process: %w", err)
	}

	newPid := cmd.Process.Pid
	u.oldPid = os.Getpid()

	// 写入新 PID
	if u.pidFile != "" {
		os.WriteFile(u.pidFile, []byte(fmt.Sprintf("%d", newPid)), 0644)
	}

	return nil
}

// listenerFile 从 net.Listener 获取文件描述符。
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
	process.Signal(syscall.SIGKILL)
	return fmt.Errorf("old process did not shutdown gracefully")
}

// NotifyOldProcess 通知旧进程关闭。
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

// SetupSignalHandlers 设置升级相关信号处理。
func (u *UpgradeManager) SetupSignalHandlers(newBinary string) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR2)

	go func() {
		for sig := range sigCh {
			if sig == syscall.SIGUSR2 {
				u.GracefulUpgrade(newBinary)
			}
		}
	}()
}
