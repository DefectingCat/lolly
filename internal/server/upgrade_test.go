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
	"testing"
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


// TestReadOldPid_EmptyFile 测试空 PID 文件


// TestNotifyOldProcess_ReadPidError 测试读取 PID 失败的情况


// TestNotifyOldProcess_ZeroPid 测试 PID 为 0 的情况


// TestNotifyOldProcess_NonExistentProcess 测试通知不存在的进程


// TestNotifyOldProcess_FindProcessError 测试 os.FindProcess 的行为
// 注意：在 Unix 系统上，os.FindProcess 总是成功，即使进程不存在


// TestSetupSignalHandlers_SetsUpChannel 测试信号处理器设置


// TestSetupSignalHandlers_TriggersUpgrade 测试信号触发升级


// TestGracefulUpgrade_UnsupportedListener 测试不支持的监听器类型


// TestGracefulUpgrade_NonexistentBinary 测试不存在的二进制文件
// 注意：此测试使用 mock 监听器避免创建实际网络连接


// TestGracefulUpgrade_WithPidFile 测试升级时写入 PID 文件
// 注意：此测试使用 mock 监听器避免创建实际网络连接


// TestWaitForShutdown_ProcessExits 测试进程退出后的等待


// TestWaitForShutdown_Timeout 测试等待超时


// TestWaitForShutdown_SetsOldPid 测试 oldPid 设置


// TestListenerFile_UnixListener 测试 Unix 监听器获取文件
// 注意：跳过此测试，因为在大量测试运行时可能导致 FD 问题
func TestListenerFile_UnixListener(t *testing.T) {
	t.Skip("Skipping test to avoid FD exhaustion in parallel test runs")
}

// TestGracefulUpgrade_MultipleListeners 测试多个监听器的升级
// 注意：使用 mock 监听器避免 FD 问题


// TestGracefulUpgrade_RelativePath 测试相对路径的二进制文件
// 注意：使用 mock 监听器避免 FD 问题


// TestWaitForShutdown_FindProcessError 测试 FindProcess 行为


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
