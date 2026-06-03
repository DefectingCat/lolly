// Package server 提供测试工具函数的测试。
package server

import (
	"testing"
	"time"
)

// TestMockFastServer_Serve 测试 MockFastServer.Serve 方法


// TestMockFastServer_ServeTLS 测试 MockFastServer.ServeTLS 方法


// TestMockFastServer_Shutdown 测试 MockFastServer.Shutdown 方法


// TestNewServerForTesting 测试 NewServerForTesting 函数


// TestNewTestServerWithOptions 测试 NewTestServerWithOptions 函数


// TestMustStartTestServer 测试 MustStartTestServer 函数


// TestTestDependencies 测试 TestDependencies 结构体


// TestTestServerOptions 测试 TestServerOptions 结构体


// TestMockFastServer_Fields 测试 MockFastServer 字段
func TestMockFastServer_Fields(t *testing.T) {
	mock := &MockFastServer{
		Name:               "test-server",
		ReadTimeout:        10 * time.Second,
		WriteTimeout:       20 * time.Second,
		IdleTimeout:        30 * time.Second,
		MaxConnsPerIP:      100,
		MaxRequestsPerConn: 1000,
		CloseOnShutdown:    true,
	}

	if mock.Name != "test-server" {
		t.Errorf("expected Name test-server, got %s", mock.Name)
	}
	if mock.ReadTimeout != 10*time.Second {
		t.Errorf("expected ReadTimeout 10s, got %v", mock.ReadTimeout)
	}
	if mock.WriteTimeout != 20*time.Second {
		t.Errorf("expected WriteTimeout 20s, got %v", mock.WriteTimeout)
	}
	if mock.IdleTimeout != 30*time.Second {
		t.Errorf("expected IdleTimeout 30s, got %v", mock.IdleTimeout)
	}
	if mock.MaxConnsPerIP != 100 {
		t.Errorf("expected MaxConnsPerIP 100, got %d", mock.MaxConnsPerIP)
	}
	if mock.MaxRequestsPerConn != 1000 {
		t.Errorf("expected MaxRequestsPerConn 1000, got %d", mock.MaxRequestsPerConn)
	}
	if !mock.CloseOnShutdown {
		t.Error("CloseOnShutdown should be true")
	}
}
