package server

import (
	"os"
	"testing"
)

func TestNewUpgradeManager(t *testing.T) {
	srv := New(nil)
	mgr := NewUpgradeManager(srv)

	if mgr == nil {
		t.Error("Expected non-nil manager")
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
	os.Setenv("GRACEFUL_UPGRADE", "1")
	defer os.Unsetenv("GRACEFUL_UPGRADE")

	if !mgr.IsChild() {
		t.Error("Expected IsChild to be true when GRACEFUL_UPGRADE=1")
	}
}

func TestPidFile(t *testing.T) {
	tmpFile := "/tmp/lolly-test.pid"
	defer os.Remove(tmpFile)

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
