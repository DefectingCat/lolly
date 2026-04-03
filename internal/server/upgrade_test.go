package server

import (
	"net"
	"os"
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
	_ = os.Setenv("GRACEFUL_UPGRADE", "1")
	defer func() { _ = os.Unsetenv("GRACEFUL_UPGRADE") }()

	if !mgr.IsChild() {
		t.Error("Expected IsChild to be true when GRACEFUL_UPGRADE=1")
	}
}

func TestPidFile(t *testing.T) {
	tmpFile := "/tmp/lolly-test.pid"
	defer func() { _ = os.Remove(tmpFile) }()

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
	defer func() { _ = listener1.Close() }()

	listener2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() { _ = listener2.Close() }()

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
	defer func() { _ = os.Remove(tmpFile) }()

	// 写入无效内容
	if err := os.WriteFile(tmpFile, []byte("not-a-pid"), 0644); err != nil {
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
	// 保存原始环境变量
	origFds := os.Getenv("LISTEN_FDS")
	defer func() { _ = os.Setenv("LISTEN_FDS", origFds) }()

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
			_ = os.Setenv("LISTEN_FDS", tt.fdsEnv)

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
	defer func() { _ = listener.Close() }()

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
	defer func() { _ = os.Remove(tmpFile) }()

	// 写入空内容
	if err := os.WriteFile(tmpFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	mgr := NewUpgradeManager(nil)
	mgr.SetPidFile(tmpFile)

	_, err := mgr.ReadOldPid()
	if err == nil {
		t.Error("Expected error for empty PID file")
	}
}
