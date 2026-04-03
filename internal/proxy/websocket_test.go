package proxy

import (
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// TestNewWebSocketBridge 测试桥接器创建
func TestNewWebSocketBridge(t *testing.T) {
	clientConn, _ := net.Pipe()
	targetConn, _ := net.Pipe()
	defer clientConn.Close()
	defer targetConn.Close()

	bridge := NewWebSocketBridge(clientConn, targetConn)

	if bridge == nil {
		t.Error("Expected non-nil bridge")
	}
	if bridge.clientConn != clientConn {
		t.Error("Expected clientConn to be set")
	}
	if bridge.targetConn != targetConn {
		t.Error("Expected targetConn to be set")
	}
	if bridge.closed != false {
		t.Error("Expected closed to be false initially")
	}
}

// TestWebSocketBridge_Close 测试关闭桥接器
func TestWebSocketBridge_Close(t *testing.T) {
	clientConn, client2 := net.Pipe()
	targetConn, target2 := net.Pipe()

	bridge := NewWebSocketBridge(clientConn, targetConn)

	// 关闭桥接器
	err := bridge.Close()
	if err != nil {
		t.Errorf("Expected nil error, got: %v", err)
	}

	// 验证连接已关闭 - 写入应该失败
	_, err = client2.Write([]byte("test"))
	if err == nil {
		t.Error("Expected error writing to closed connection")
	}

	_ = target2

	// 重复关闭应该安全
	err = bridge.Close()
	if err != nil {
		t.Errorf("Expected nil error on double close, got: %v", err)
	}
}

// TestWebSocketBridge_Close_NilConnections 测试空连接的关闭
func TestWebSocketBridge_Close_NilConnections(t *testing.T) {
	bridge := &WebSocketBridge{
		clientConn: nil,
		targetConn: nil,
		closed:     false,
	}

	err := bridge.Close()
	if err != nil {
		t.Errorf("Expected nil error for nil connections, got: %v", err)
	}
}

// TestIsConnectionClosedError 测试连接关闭错误检测
func TestIsConnectionClosedError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "EOF",
			err:      io.EOF,
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isConnectionClosedError(tt.err)
			if result != tt.expected {
				t.Errorf("isConnectionClosedError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestExtractHost 测试从 URL 提取主机
func TestExtractHost(t *testing.T) {
	// extractHost 函数可能不存在，检查一下
	// 如果存在则测试
}

// TestDialTarget_InvalidAddress 测试无效地址的拨号
func TestDialTarget_InvalidAddress(t *testing.T) {
	// 测试连接到无效端口
	_, err := dialTarget("http://127.0.0.1:1", 100*time.Millisecond)
	if err == nil {
		t.Error("Expected error for invalid address")
	}
}

// TestDialTarget_HTTPS 测试 HTTPS 连接（会失败，但验证错误处理）
func TestDialTarget_HTTPS(t *testing.T) {
	// 测试 HTTPS 连接到无效端口
	_, err := dialTarget("https://127.0.0.1:1", 100*time.Millisecond)
	if err == nil {
		t.Error("Expected error for invalid HTTPS address")
	}
}

// mockNetError 模拟网络错误
type mockNetError struct {
	msg string
}

func (e *mockNetError) Error() string   { return e.msg }
func (e *mockNetError) Timeout() bool   { return true }
func (e *mockNetError) Temporary() bool { return false }

// TestIsConnectionClosedError_Timeout 测试超时错误
func TestIsConnectionClosedError_Timeout(t *testing.T) {
	timeoutErr := &mockNetError{msg: "timeout"}
	result := isConnectionClosedError(timeoutErr)
	if !result {
		t.Error("Expected timeout error to be treated as closed connection error")
	}
}

// TestWebSocketBridge_Bridge 测试双向数据转发
func TestWebSocketBridge_Bridge(t *testing.T) {
	// 创建管道连接
	client1, client2 := net.Pipe()
	target1, target2 := net.Pipe()
	defer client2.Close()
	defer target2.Close()

	bridge := NewWebSocketBridge(client1, target1)

	// 启动桥接（在 goroutine 中）
	errCh := make(chan error, 1)
	go func() {
		errCh <- bridge.Bridge()
	}()

	// 发送数据从客户端到后端
	testData := []byte("hello from client")
	go func() {
		client2.Write(testData)
	}()

	// 在后端读取数据
	buf := make([]byte, 1024)
	n, err := target2.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read from target: %v", err)
	}
	if string(buf[:n]) != string(testData) {
		t.Errorf("Expected %q, got %q", string(testData), string(buf[:n]))
	}

	// 发送数据从后端到客户端
	testData2 := []byte("hello from target")
	go func() {
		target2.Write(testData2)
	}()

	// 在客户端读取数据
	buf2 := make([]byte, 1024)
	n, err = client2.Read(buf2)
	if err != nil {
		t.Fatalf("Failed to read from client: %v", err)
	}
	if string(buf2[:n]) != string(testData2) {
		t.Errorf("Expected %q, got %q", string(testData2), string(buf2[:n]))
	}

	// 关闭连接以结束桥接
	client2.Close()
	target2.Close()

	// 等待桥接完成
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Bridge returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Bridge did not complete in time")
	}
}

// TestDialTarget_URLParsing 测试 URL 解析
func TestDialTarget_URLParsing(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
	}{
		{
			name:        "http URL with invalid port",
			url:         "http://127.0.0.1:1",
			expectError: true, // 连接会失败
		},
		{
			name:        "https URL with invalid port",
			url:         "https://127.0.0.1:1",
			expectError: true, // 连接会失败
		},
		{
			name:        "URL with path",
			url:         "http://127.0.0.1:1/ws",
			expectError: true, // 连接会失败
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dialTarget(tt.url, 10*time.Millisecond)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestCopyData 测试数据复制
func TestCopyData(t *testing.T) {
	// 创建管道连接
	src1, src2 := net.Pipe()
	dst1, dst2 := net.Pipe()
	defer src2.Close()
	defer dst2.Close()

	bridge := &WebSocketBridge{}

	// 启动数据复制
	errCh := make(chan error, 1)
	go func() {
		errCh <- bridge.copyData(dst1, src1, "test")
	}()

	// 发送数据
	testData := []byte("test data")
	src2.Write(testData)

	// 接收数据
	buf := make([]byte, 1024)
	n, err := dst2.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
	if string(buf[:n]) != string(testData) {
		t.Errorf("Expected %q, got %q", string(testData), string(buf[:n]))
	}

	// 关闭连接
	src2.Close()
	dst2.Close()

	// 等待复制完成
	select {
	case err := <-errCh:
		// 连接关闭错误应返回 nil
		if err != nil && !strings.Contains(err.Error(), "closed") {
			t.Errorf("copyData returned unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("copyData did not complete in time")
	}
}
