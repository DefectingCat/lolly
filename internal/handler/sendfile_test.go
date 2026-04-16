//go:build linux

// Package handler 提供 Sendfile 功能的 Linux 平台测试。
//
// 该文件测试 Linux 平台特有的 Sendfile 功能，包括：
//   - Linux sendfile 系统调用
//   - 套接字文件描述符获取
//   - 小文件发送 fallback
//
// 作者：xfy
package handler

import (
	"bytes"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

func TestMinSendfileSize(t *testing.T) {
	if MinSendfileSize != 8*1024 {
		t.Errorf("Expected MinSendfileSize 8KB, got %d", MinSendfileSize)
	}
}

// TestCopyFile 测试 copyFile fallback 函数
func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	content := []byte("Hello, World! This is test content for copyFile.")
	if err := os.WriteFile(tmpFile, content, 0o644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	tests := []struct {
		name    string
		offset  int64
		length  int64
		wantLen int
		wantErr bool
	}{
		{
			name:    "full file",
			offset:  0,
			length:  0,
			wantLen: len(content),
			wantErr: false,
		},
		{
			name:    "with length",
			offset:  0,
			length:  10,
			wantLen: 10,
			wantErr: false,
		},
		{
			name:    "with offset",
			offset:  7,
			length:  5,
			wantLen: 5,
			wantErr: false,
		},
		{
			name:    "offset beyond file",
			offset:  1000,
			length:  10,
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _ = file.Seek(0, io.SeekStart)
			ctx := &fasthttp.RequestCtx{}

			err := copyFile(ctx, file, tt.offset, tt.length)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				body := ctx.Response.Body()
				if len(body) != tt.wantLen {
					t.Errorf("expected body length %d, got %d", tt.wantLen, len(body))
				}
				if tt.wantLen > 0 && tt.length == 0 {
					if string(body) != string(content[tt.offset:]) {
						t.Errorf("body content mismatch")
					}
				}
			}
		})
	}
}

// TestGetSocketFd_NilConn 测试 nil 连接的情况
func TestGetSocketFd_NilConn(t *testing.T) {
	_, err := getSocketFd(nil)
	if err == nil {
		t.Error("expected error for nil connection")
	}
}

// TestGetSocketFd_UnsupportedType 测试不支持的连接类型
func TestGetSocketFd_UnsupportedType(t *testing.T) {
	conn := &mockConn{}
	_, err := getSocketFd(conn)
	if err != syscall.ENOTSUP {
		t.Errorf("expected ENOTSUP for unsupported conn type, got: %v", err)
	}
}

// mockConn 是一个不实现 TCPConn/UnixConn 的连接
type mockConn struct{}

func (m *mockConn) Read([]byte) (n int, err error)   { return 0, nil }
func (m *mockConn) Write([]byte) (n int, err error)  { return 0, nil }
func (m *mockConn) Close() error                     { return nil }
func (m *mockConn) LocalAddr() net.Addr              { return nil }
func (m *mockConn) RemoteAddr() net.Addr             { return nil }
func (m *mockConn) SetDeadline(time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(time.Time) error { return nil }

// TestSendFile_SmallFile 测试小文件发送（使用 fallback）
func TestSendFile_SmallFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "small.txt")

	content := []byte("small file content")
	if err := os.WriteFile(tmpFile, content, 0o644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	err = SendFile(ctx, file, 0, int64(len(content)))
	if err != nil {
		t.Errorf("SendFile failed: %v", err)
	}

	if !bytes.Equal(ctx.Response.Body(), content) {
		t.Errorf("Expected body %s, got %s", content, ctx.Response.Body())
	}
}

// TestSendFile_WithOffset 测试带偏移量的文件发送
func TestSendFile_WithOffset(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	content := []byte("0123456789ABCDEF")
	if err := os.WriteFile(tmpFile, content, 0o644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	err = SendFile(ctx, file, 5, 5)
	if err != nil {
		t.Errorf("SendFile failed: %v", err)
	}

	expected := content[5:10]
	if !bytes.Equal(ctx.Response.Body(), expected) {
		t.Errorf("Expected body %s, got %s", expected, ctx.Response.Body())
	}
}

// TestSendFile_ZeroLength 测试零长度文件
func TestSendFile_ZeroLength(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "empty.txt")

	if err := os.WriteFile(tmpFile, []byte{}, 0o644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	err = SendFile(ctx, file, 0, 0)
	if err != nil {
		t.Errorf("SendFile failed: %v", err)
	}

	if len(ctx.Response.Body()) != 0 {
		t.Errorf("Expected empty body, got %s", ctx.Response.Body())
	}
}

// TestGetNetConn 测试获取底层连接
func TestGetNetConn(_ *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	conn := getNetConn(ctx)
	_ = conn
}

// TestCopyFile_Error 测试 copyFile 错误情况
func TestCopyFile_Error(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	content := []byte("test content")
	if err := os.WriteFile(tmpFile, content, 0o644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	err = copyFile(ctx, file, 1000, 10)
	if err == nil {
		t.Error("Expected error for offset beyond file size")
	}
}

// TestLinuxSendfile_NilConn 测试 linuxSendfile 空连接
func TestLinuxSendfile_NilConn(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test")
	_ = os.WriteFile(tmpFile, content, 0o644)

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	err = linuxSendfile(nil, file.Fd(), 0, int64(len(content)))
	if err == nil {
		t.Error("Expected error for nil connection")
	}
}

// TestSendFile_LargeFile 测试大文件使用 sendfile 调用
func TestSendFile_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "large.bin")

	// 创建超过 MinSendfileSize (8KB) 的文件
	content := make([]byte, 16*1024) // 16KB
	_, _ = rand.Read(content)

	if err := os.WriteFile(tmpFile, content, 0o644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	// 创建真正的 TCP 连接用于 sendfile
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer ln.Close()

	// 启动 goroutine 接收连接
	var serverConn net.Conn
	go func() {
		serverConn, _ = ln.Accept()
	}()

	// 客户端连接
	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer clientConn.Close()

	// 等待服务器接受
	time.Sleep(100 * time.Millisecond)

	// 将客户端连接设置为非阻塞以便测试 sendfile
	if err := clientConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("Failed to set deadline: %v", err)
	}

	// 构造 RequestCtx
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/test")

	// 发送大文件（应使用 sendfile）
	err = SendFile(ctx, file, 0, int64(len(content)))
	if err != nil {
		t.Logf("SendFile returned: %v", err)
		// EPIPE 是可接受的，因为服务器可能在读取后关闭连接
		if err != syscall.EPIPE && err != syscall.ECONNRESET {
			t.Errorf("SendFile unexpected error: %v", err)
		}
	}

	// 关闭服务器连接
	if serverConn != nil {
		serverConn.Close()
	}
}

// TestSendFile_FullRange 测试传输完整文件范围
func TestSendFile_FullRange(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "range.txt")

	content := []byte("0123456789ABCDEF")
	if err := os.WriteFile(tmpFile, content, 0o644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	// 传输整个文件
	err = SendFile(ctx, file, 0, -1)
	if err != nil {
		t.Errorf("SendFile failed: %v", err)
	}

	if !bytes.Equal(ctx.Response.Body(), content) {
		t.Errorf("Expected body %s, got %s", content, ctx.Response.Body())
	}
}

// TestSendFile_FileNotFound 测试文件不存在的情况
func TestSendFile_FileNotFound(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	// 打开不存在的文件
	file, err := os.Open("/nonexistent/file/test.txt")
	if err != nil {
		t.Skip("Skipping: file not found")
	}
	defer file.Close()

	err = SendFile(ctx, file, 0, 100)
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

// TestCopyFile_EmptyFile 测试空文件拷贝
func TestCopyFile_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "empty.txt")

	if err := os.WriteFile(tmpFile, []byte{}, 0o644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open dir: %v", err)
	}
	defer file.Close()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	// 尝试拷贝目录（应失败）
	err = copyFile(ctx, file, 0, 0)
	// 目录不可读，应返回错误
	if err == nil {
		t.Error("Expected error when copying directory")
	}
}

// TestGetSocketFd_UnixConn 测试 UnixConn 获取 socket fd
func TestGetSocketFd_UnixConn(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to listen on unix socket: %v", err)
	}
	defer ln.Close()
	defer os.Remove(socketPath)

	// 启动 goroutine 接收连接
	var serverConn net.Conn
	go func() {
		serverConn, _ = ln.Accept()
	}()

	// 客户端连接
	clientConn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to dial unix socket: %v", err)
	}
	defer clientConn.Close()

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	// 测试获取 socket fd
	fd, err := getSocketFd(clientConn)
	if err != nil {
		t.Errorf("getSocketFd failed: %v", err)
	}
	if fd == 0 {
		t.Error("Expected non-zero fd")
	}

	if serverConn != nil {
		serverConn.Close()
	}
}

// TestSendFile_OffsetBeyondFile 测试偏移量超出文件大小
func TestSendFile_OffsetBeyondFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("short content")
	_ = os.WriteFile(tmpFile, content, 0o644)

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	// 偏移量超出文件大小
	err = SendFile(ctx, file, 1000, 10)
	if err == nil {
		t.Error("Expected error when offset beyond file size")
	}
}

// TestSendFile_LengthOutOfRange 测试长度超出文件范围
func TestSendFile_LengthOutOfRange(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("short")
	_ = os.WriteFile(tmpFile, content, 0o644)

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	// 请求长度超出文件大小
	err = SendFile(ctx, file, 0, 1000)
	if err != nil {
		// 小文件会使用 copyFile，可能返回错误
		t.Logf("SendFile returned: %v", err)
	}
}

// TestSendFile_AtMinBoundary 测试刚好等于 MinSendfileSize 的文件
func TestSendFile_AtMinBoundary(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "boundary.bin")

	// 创建刚好等于 MinSendfileSize 的文件
	content := make([]byte, MinSendfileSize)
	_, _ = rand.Read(content)
	_ = os.WriteFile(tmpFile, content, 0o644)

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	// 创建监听器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer ln.Close()

	// 启动 goroutine 接收连接
	var serverConn net.Conn
	go func() {
		serverConn, _ = ln.Accept()
		buf := make([]byte, MinSendfileSize)
		serverConn.Read(buf)
		serverConn.Close()
	}()

	// 客户端连接
	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer clientConn.Close()

	time.Sleep(100 * time.Millisecond)

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/test")

	err = SendFile(ctx, file, 0, int64(len(content)))
	if err != nil {
		if err != syscall.EPIPE && err != syscall.ECONNRESET {
			t.Logf("SendFile returned: %v", err)
		}
	}

	if serverConn != nil {
		serverConn.Close()
	}
}

// TestSendFile_JustBelowMin 测试刚好小于 MinSendfileSize 的文件（使用 fallback）
func TestSendFile_JustBelowMin(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "below.bin")

	// 创建略小于 MinSendfileSize 的文件
	content := make([]byte, MinSendfileSize-1)
	_, _ = rand.Read(content)
	_ = os.WriteFile(tmpFile, content, 0o644)

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	err = SendFile(ctx, file, 0, int64(len(content)))
	if err != nil {
		t.Errorf("SendFile failed: %v", err)
	}

	if !bytes.Equal(ctx.Response.Body(), content) {
		t.Errorf("Body mismatch")
	}
}
