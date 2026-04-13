//go:build !linux

// Package handler 提供 Sendfile 功能的测试（非 Linux 平台）。
//
// 该文件测试非 Linux 平台的 Sendfile 功能。
//
// 作者：xfy
package handler

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestMinSendfileSize(t *testing.T) {
	if MinSendfileSize != 8*1024 {
		t.Errorf("Expected MinSendfileSize 8KB, got %d", MinSendfileSize)
	}
}

// TestPlatformSendfile_NonLinux 测试非 Linux 平台的 sendfile 行为
func TestPlatformSendfile_NonLinux(t *testing.T) {
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

	err = platformSendfile(nil, file, 0, int64(len(content)))
	if err != syscall.ENOTSUP {
		t.Errorf("expected ENOTSUP on non-Linux, got: %v", err)
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
