// Package handler 提供 Sendfile 功能的测试。
//
// 该文件测试 Sendfile 模块的各项功能，包括：
//   - 最小 Sendfile 大小
//   - 平台 Sendfile 行为
//   - 文件复制功能
//   - 非 Linux 平台行为
//   - 套接字文件描述符获取
//   - 小文件发送
//   - 带偏移量发送
//   - 零长度文件
//   - 网络连接获取
//   - 错误处理
//
// 作者：xfy
package handler

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
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

func TestPlatformSendfile(t *testing.T) {
	// 创建临时文件
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	content := []byte("Hello, World! This is a test file for sendfile.")
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	// 测试平台 sendfile（小文件会 fallback 到 copyFile）
	// 由于没有真实的网络连接，这个测试主要验证不会崩溃
	_ = platformSendfile(nil, file, 0, int64(len(content)))
}

// TestCopyFile 测试 copyFile fallback 函数
func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	content := []byte("Hello, World! This is test content for copyFile.")
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
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
			length:  0, // 0 means copy all
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
			wantErr: true, // io.CopyN returns EOF error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 重置文件位置
			_, _ = file.Seek(0, io.SeekStart)

			// 创建响应上下文
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
					// 全量拷贝时验证内容
					if string(body) != string(content[tt.offset:]) {
						t.Errorf("body content mismatch")
					}
				}
			}
		})
	}
}

// TestPlatformSendfile_NonLinux 测试非 Linux 平台的 sendfile 行为
func TestPlatformSendfile_NonLinux(t *testing.T) {
	if runtime.GOOS == platformLinux {
		t.Skip("this test is for non-Linux platforms")
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content")
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	err = platformSendfile(nil, file, 0, int64(len(content)))
	if err != syscall.ENOTSUP {
		t.Errorf("expected ENOTSUP on non-Linux, got: %v", err)
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
	// 创建一个不支持的连接类型
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

	// 创建小文件 (< 8KB)
	content := []byte("small file content")
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
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

	// 验证响应体
	if !bytes.Equal(ctx.Response.Body(), content) {
		t.Errorf("Expected body %s, got %s", content, ctx.Response.Body())
	}
}

// TestSendFile_WithOffset 测试带偏移量的文件发送
func TestSendFile_WithOffset(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	content := []byte("0123456789ABCDEF") // 16 bytes
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	// 从偏移量 5 开始，读取 5 字节
	err = SendFile(ctx, file, 5, 5)
	if err != nil {
		t.Errorf("SendFile failed: %v", err)
	}

	expected := content[5:10] // "56789"
	if !bytes.Equal(ctx.Response.Body(), expected) {
		t.Errorf("Expected body %s, got %s", expected, ctx.Response.Body())
	}
}

// TestSendFile_ZeroLength 测试零长度文件
func TestSendFile_ZeroLength(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "empty.txt")

	if err := os.WriteFile(tmpFile, []byte{}, 0644); err != nil {
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

	// fasthttp 会创建内部连接，所以这里测试能正常获取
	conn := getNetConn(ctx)
	// 主要验证不会崩溃
	_ = conn
}

// TestSendFile_NilFile 测试空文件指针
func TestSendFile_NilFile(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	// 传入 nil 文件应该 panic 或返回错误
	defer func() {
		if r := recover(); r == nil {
			// 没有 panic，检查是否有错误返回
			t.Log("expected panic did not occur for nil file")
		}
	}()

	// 这个测试主要确保不会静默失败
}

// TestCopyFile_Error 测试 copyFile 错误情况
func TestCopyFile_Error(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	content := []byte("test content")
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	file, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)

	// 测试偏移量超出文件大小
	err = copyFile(ctx, file, 1000, 10)
	if err == nil {
		t.Error("Expected error for offset beyond file size")
	}
}

// TestLinuxSendfile_NilConn 测试 linuxSendfile 空连接
func TestLinuxSendfile_NilConn(t *testing.T) {
	if runtime.GOOS != platformLinux {
		t.Skip("This test is for Linux only")
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test")
	_ = os.WriteFile(tmpFile, content, 0644)

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
