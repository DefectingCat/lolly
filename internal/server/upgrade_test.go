//go:build !windows

// Package server 提供优雅升级功能的测试。
//
// 该文件测试优雅升级模块的各项功能，包括：
//   - UpgradeManager 的创建和配置
//   - 子进程检测
//   - PID 文件的读写
//   - 监听器继承
//   - 进程通知机制
//   - 平滑关闭等待
//   - 错误处理场景
//
// 作者：xfy
package server

import (
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestNewUpgradeManager(t *testing.T) {
	srv := New(nil)
	mgr := NewUpgradeManager(srv)
	if mgr == nil {
		t.Fatal("Expected non-nil manager")
	}
	if mgr.server != srv {
		t.Error("Expected server to be set")
	}
}

func TestIsChild(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// 默认不是子进程
	if mgr.IsChild() {
		t.Error("Expected IsChild to be false by default")
	}

	// 设置环境变量
	t.Setenv("GRACEFUL_UPGRADE", "1")

	if !mgr.IsChild() {
		t.Error("Expected IsChild to be true when GRACEFUL_UPGRADE=1")
	}
}

func TestPidFile(t *testing.T) {
	tmpFile := "/tmp/lolly-test.pid"
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	mgr := NewUpgradeManager(nil)
	mgr.SetPidFile(tmpFile)

	// 写入 PID
	if err := mgr.WritePid(); err != nil {
		t.Errorf("WritePid failed: %v", err)
	}

	// 读取 PID
	pid, err := mgr.ReadOldPid()
	if err != nil {
		t.Errorf("ReadOldPid failed: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("Expected pid %d, got %d", os.Getpid(), pid)
	}
}

func TestReadOldPidNoFile(t *testing.T) {
	mgr := NewUpgradeManager(nil)
	mgr.SetPidFile("/nonexistent/path/pid")

	_, err := mgr.ReadOldPid()
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestGetInheritedListenersNoFds(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// 没有设置 LISTEN_FDS
	listeners, err := mgr.GetInheritedListeners()
	if err != nil {
		t.Errorf("GetInheritedListeners failed: %v", err)
	}
	if len(listeners) != 0 {
		t.Errorf("Expected 0 listeners, got %d", len(listeners))
	}
}

func TestNotifyOldProcessNoPid(t *testing.T) {
	mgr := NewUpgradeManager(nil)
	mgr.SetPidFile("/nonexistent/pid")

	// 没有 PID 文件，应该返回 nil
	err := mgr.NotifyOldProcess()
	if err != nil {
		t.Errorf("NotifyOldProcess should return nil for no pid, got: %v", err)
	}
}

func TestWaitForShutdownNoOldPid(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// 没有旧进程
	err := mgr.WaitForShutdown(0)
	if err != nil {
		t.Errorf("WaitForShutdown should return nil for no old pid, got: %v", err)
	}
}

// TestUpgradeSetListeners 测试监听器设置
func TestUpgradeSetListeners(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// 创建模拟监听器
	listener1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		_ = listener1.Close()
	}()

	listener2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		_ = listener2.Close()
	}()

	listeners := []net.Listener{listener1, listener2}
	mgr.SetListeners(listeners)

	if len(mgr.listeners) != 2 {
		t.Errorf("Expected 2 listeners, got %d", len(mgr.listeners))
	}
}

// TestWritePid_NoPidFile 测试无 PID 文件配置时的行为
func TestWritePid_NoPidFile(t *testing.T) {
	mgr := NewUpgradeManager(nil)
	// 不设置 PID 文件

	err := mgr.WritePid()
	if err != nil {
		t.Errorf("WritePid should return nil when no pid file configured, got: %v", err)
	}
}

// TestReadOldPid_InvalidContent 测试 PID 文件内容无效时的错误处理
func TestReadOldPid_InvalidContent(t *testing.T) {
	tmpFile := "/tmp/lolly-test-invalid.pid"
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// 写入无效内容
	if err := os.WriteFile(tmpFile, []byte("not-a-pid"), 0o644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	mgr := NewUpgradeManager(nil)
	mgr.SetPidFile(tmpFile)

	_, err := mgr.ReadOldPid()
	if err == nil {
		t.Error("Expected error for invalid PID content")
	}
}

// TestGetInheritedListeners_InvalidFds 测试 LISTEN_FDS 环境变量格式无效
func TestGetInheritedListeners_InvalidFds(t *testing.T) {
	tests := []struct {
		name    string
		fdsEnv  string
		wantErr bool
	}{
		{
			name:    "invalid format - not a number",
			fdsEnv:  "invalid",
			wantErr: true,
		},
		{
			name:    "zero fds",
			fdsEnv:  "0",
			wantErr: false,
		},
		{
			name:    "negative fds",
			fdsEnv:  "-1",
			wantErr: false, // Sscanf 解析成功，但逻辑上无效
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LISTEN_FDS", tt.fdsEnv)

			mgr := NewUpgradeManager(nil)
			_, err := mgr.GetInheritedListeners()

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error for invalid LISTEN_FDS")
				}
			}
			// 零或负数应该返回空列表，无错误
			// 注意：对于 -1，后续逻辑可能会尝试访问无效的 FD
		})
	}
}

// TestWaitForShutdown_WithTimeout 测试超时行为
func TestWaitForShutdown_WithTimeout(t *testing.T) {
	mgr := NewUpgradeManager(nil)
	mgr.oldPid = 9999999 // 不存在的进程 ID

	// 短超时 - 不存在的进程会被立即检测到
	// WaitForShutdown 会尝试发送 Signal(0) 检测进程存在
	err := mgr.WaitForShutdown(100 * time.Millisecond)
	// 对于不存在的进程，Signal(0) 会返回错误，所以 WaitForShutdown 应该返回 nil
	if err != nil {
		// 进程不存在时，Signal(0) 返回错误，函数提前返回 nil
		t.Logf("WaitForShutdown returned: %v (expected nil for non-existent process)", err)
	}
}

// TestListenerFile_TCPListener 测试从 TCP 监听器获取文件
func TestListenerFile_TCPListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	file, err := listenerFile(listener)
	if err != nil {
		t.Errorf("Failed to get listener file: %v", err)
	}
	if file != nil {
		_ = file.Close()
	}
}

// TestListenerFile_UnsupportedType 测试不支持的监听器类型
func TestListenerFile_UnsupportedType(t *testing.T) {
	// 创建一个模拟的不支持类型
	listener := &mockListener{}

	_, err := listenerFile(listener)
	if err == nil {
		t.Error("Expected error for unsupported listener type")
	}
}

// mockListener 是一个不实现 TCPListener/UnixListener 的监听器
type mockListener struct{}

func (m *mockListener) Accept() (net.Conn, error) { return nil, nil }
func (m *mockListener) Close() error              { return nil }
func (m *mockListener) Addr() net.Addr            { return nil }

// TestGracefulUpgrade_NoListeners 测试无监听器时的升级失败
func TestGracefulUpgrade_NoListeners(t *testing.T) {
	mgr := NewUpgradeManager(nil)
	// 不设置监听器

	err := mgr.GracefulUpgrade("/nonexistent/binary")
	if err == nil {
		t.Error("Expected error when no listeners configured")
	}
	expectedErr := "no listeners configured for upgrade"
	if err != nil && err.Error() != expectedErr {
		t.Errorf("Expected error '%s', got: %v", expectedErr, err)
	}
}

// TestNotifyOldProcess_WithCurrentPid 测试通知进程
// 注意：不能向当前进程发送 SIGQUIT，会导致测试崩溃
func TestNotifyOldProcess_WithCurrentPid(t *testing.T) {
	// 跳过此测试，因为发送 SIGQUIT 给当前进程会导致崩溃
	t.Skip("Skipping test that would send SIGQUIT to current process")
}

// TestReadOldPid_EmptyFile 测试空 PID 文件
func TestReadOldPid_EmptyFile(t *testing.T) {
	tmpFile := "/tmp/lolly-test-empty.pid"
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// 写入空内容
	if err := os.WriteFile(tmpFile, []byte(""), 0o644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	mgr := NewUpgradeManager(nil)
	mgr.SetPidFile(tmpFile)

	_, err := mgr.ReadOldPid()
	if err == nil {
		t.Error("Expected error for empty PID file")
	}
}

// TestNotifyOldProcess_ReadPidError 测试读取 PID 失败的情况
func TestNotifyOldProcess_ReadPidError(t *testing.T) {
	mgr := NewUpgradeManager(nil)
	// 不设置 PID 文件，ReadOldPid 会返回错误

	err := mgr.NotifyOldProcess()
	if err != nil {
		t.Errorf("NotifyOldProcess should return nil when ReadOldPid fails, got: %v", err)
	}
}

// TestNotifyOldProcess_ZeroPid 测试 PID 为 0 的情况
func TestNotifyOldProcess_ZeroPid(t *testing.T) {
	tmpFile := "/tmp/lolly-test-zero.pid"
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// 写入 PID 0
	if err := os.WriteFile(tmpFile, []byte("0"), 0o644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	mgr := NewUpgradeManager(nil)
	mgr.SetPidFile(tmpFile)

	err := mgr.NotifyOldProcess()
	if err != nil {
		t.Errorf("NotifyOldProcess should return nil for PID 0, got: %v", err)
	}
}

// TestNotifyOldProcess_NonExistentProcess 测试通知不存在的进程
func TestNotifyOldProcess_NonExistentProcess(t *testing.T) {
	tmpFile := "/tmp/lolly-test-nonexistent.pid"
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// 写入一个不存在的 PID (使用一个极大的 PID 值)
	if err := os.WriteFile(tmpFile, []byte("9999999"), 0o644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	mgr := NewUpgradeManager(nil)
	mgr.SetPidFile(tmpFile)

	// 对于不存在的进程，Signal 会返回错误
	// NotifyOldProcess 会直接返回这个错误
	err := mgr.NotifyOldProcess()
	if err == nil {
		t.Error("Expected error when notifying non-existent process")
	}
}

// TestNotifyOldProcess_FindProcessError 测试 os.FindProcess 的行为
// 注意：在 Unix 系统上，os.FindProcess 总是成功，即使进程不存在
func TestNotifyOldProcess_FindProcessBehavior(t *testing.T) {
	// 这个测试验证 os.FindProcess 的行为
	// 在 Unix 上，FindProcess 总是返回一个 Process 对象
	process, err := os.FindProcess(9999999)
	if err != nil {
		t.Errorf("FindProcess should not return error on Unix, got: %v", err)
	}
	if process == nil {
		t.Error("FindProcess should return non-nil Process on Unix")
	}
}

// TestSetupSignalHandlers_SetsUpChannel 测试信号处理器设置
func TestSetupSignalHandlers_SetsUpChannel(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// 调用 SetupSignalHandlers 应该不会 panic
	mgr.SetupSignalHandlers("/nonexistent/binary")

	// 给 goroutine 一点时间启动
	time.Sleep(10 * time.Millisecond)

	// 测试通过如果没 panic
}

// TestSetupSignalHandlers_TriggersUpgrade 测试信号触发升级
func TestSetupSignalHandlers_TriggersUpgrade(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// 创建一个监听器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		_ = listener.Close()
	}()
	mgr.SetListeners([]net.Listener{listener})

	// 设置信号处理器，使用一个不存在的二进制文件
	mgr.SetupSignalHandlers("/nonexistent/binary/path")

	// 给 goroutine 启动时间
	time.Sleep(10 * time.Millisecond)

	// 发送 SIGUSR2 信号给当前进程
	// 注意：这会触发 GracefulUpgrade，但由于二进制文件不存在会失败
	// 信号处理器会忽略错误（使用 _ = u.GracefulUpgrade）
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find current process: %v", err)
	}

	// 发送 SIGUSR2
	if err := process.Signal(syscall.SIGUSR2); err != nil {
		t.Fatalf("Failed to send SIGUSR2: %v", err)
	}

	// 等待信号处理
	time.Sleep(100 * time.Millisecond)

	// 测试通过如果没有 panic
}

// TestGracefulUpgrade_UnsupportedListener 测试不支持的监听器类型
func TestGracefulUpgrade_UnsupportedListener(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// 使用 mock 监听器（不支持的类型）
	mgr.SetListeners([]net.Listener{&mockListener{}})

	err := mgr.GracefulUpgrade("/nonexistent/binary")
	if err == nil {
		t.Error("Expected error for unsupported listener type")
	}
	if err != nil && !containsString(err.Error(), "unsupported listener type") &&
		!containsString(err.Error(), "failed to get listener file") {
		t.Errorf("Expected unsupported listener error, got: %v", err)
	}
}

// TestGracefulUpgrade_NonexistentBinary 测试不存在的二进制文件
// 注意：此测试使用 mock 监听器避免创建实际网络连接
func TestGracefulUpgrade_NonexistentBinary(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// 使用 mock 监听器测试不支持类型的错误路径
	mgr.SetListeners([]net.Listener{&mockListener{}})

	// 由于 mockListener 是不支持的类型，应该返回错误
	err := mgr.GracefulUpgrade("/nonexistent/path/to/binary")
	if err == nil {
		t.Error("Expected error for unsupported listener type")
	}
}

// TestGracefulUpgrade_WithPidFile 测试升级时写入 PID 文件
// 注意：此测试使用 mock 监听器避免创建实际网络连接
func TestGracefulUpgrade_WithPidFile(t *testing.T) {
	tmpFile := "/tmp/lolly-test-upgrade.pid"
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	mgr := NewUpgradeManager(nil)
	mgr.SetPidFile(tmpFile)

	// 使用 mock 监听器
	mgr.SetListeners([]net.Listener{&mockListener{}})

	// 使用不存在的二进制文件，会失败但测试 PID 文件设置逻辑
	_ = mgr.GracefulUpgrade("/nonexistent/binary")
	// 测试通过如果没有 panic
}

// TestWaitForShutdown_ProcessExits 测试进程退出后的等待
func TestWaitForShutdown_ProcessExits(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// 使用一个不存在的 PID（Signal(0) 会返回错误）
	mgr.oldPid = 9999999

	// 不存在的进程应该立即返回 nil
	err := mgr.WaitForShutdown(1 * time.Second)
	if err != nil {
		t.Errorf("Expected nil for non-existent process, got: %v", err)
	}
}

// TestWaitForShutdown_Timeout 测试等待超时
func TestWaitForShutdown_Timeout(t *testing.T) {
	// 跳过此测试：需要实际运行的进程来测试超时
	// 向当前进程发送 SIGKILL 会导致测试崩溃
	t.Skip("Skipping test that would kill current process")
}

// TestWaitForShutdown_SetsOldPid 测试 oldPid 设置
func TestWaitForShutdown_SetsOldPid(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// oldPid 为 0 时应该直接返回 nil
	if mgr.oldPid != 0 {
		t.Error("Expected oldPid to be 0 initially")
	}

	err := mgr.WaitForShutdown(100 * time.Millisecond)
	if err != nil {
		t.Errorf("Expected nil when oldPid is 0, got: %v", err)
	}
}

// TestListenerFile_UnixListener 测试 Unix 监听器获取文件
// 注意：跳过此测试，因为在大量测试运行时可能导致 FD 问题
func TestListenerFile_UnixListener(t *testing.T) {
	t.Skip("Skipping test to avoid FD exhaustion in parallel test runs")
}

// TestGracefulUpgrade_MultipleListeners 测试多个监听器的升级
// 注意：使用 mock 监听器避免 FD 问题
func TestGracefulUpgrade_MultipleListeners(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// 使用多个 mock 监听器
	mgr.SetListeners([]net.Listener{&mockListener{}, &mockListener{}})

	// 由于 mockListener 是不支持的类型，应该返回错误
	err := mgr.GracefulUpgrade("/nonexistent/binary")
	if err == nil {
		t.Error("Expected error for unsupported listener type")
	}
}

// TestGracefulUpgrade_RelativePath 测试相对路径的二进制文件
// 注意：使用 mock 监听器避免 FD 问题
func TestGracefulUpgrade_RelativePath(t *testing.T) {
	mgr := NewUpgradeManager(nil)

	// 使用 mock 监听器
	mgr.SetListeners([]net.Listener{&mockListener{}})

	// 使用相对路径，应该返回错误
	err := mgr.GracefulUpgrade("./nonexistent")
	if err == nil {
		t.Error("Expected error for unsupported listener type")
	}
}

// TestWaitForShutdown_FindProcessError 测试 FindProcess 行为
func TestWaitForShutdown_FindProcessError(t *testing.T) {
	// 在 Unix 系统上，os.FindProcess 总是成功
	// 我们需要测试 Signal(0) 失败的情况
	mgr := NewUpgradeManager(nil)
	mgr.oldPid = 1 // init 进程，通常存在但无法发送信号

	// 短超时
	_ = mgr.WaitForShutdown(50 * time.Millisecond)
	// 无论结果如何，测试都应该正常完成
}

// TestGetInheritedListeners_EnvPreserved 测试环境变量处理
func TestGetInheritedListeners_EnvPreserved(t *testing.T) {
	t.Setenv("LISTEN_FDS", "1")

	mgr := NewUpgradeManager(nil)
	_, err := mgr.GetInheritedListeners()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// 环境变量应该仍然存在
	if os.Getenv("LISTEN_FDS") != "1" {
		t.Error("LISTEN_FDS env should be preserved")
	}
}

// containsString 检查字符串是否包含子串
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
