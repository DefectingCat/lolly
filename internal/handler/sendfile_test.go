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
