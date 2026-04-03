package handler

import (
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

func TestBufferPool(t *testing.T) {
	// 获取缓冲区
	buf := BufferPool.Get()
	if buf == nil {
		t.Error("Expected non-nil buffer")
	}
	if len(buf) != 32*1024 {
		t.Errorf("Expected buffer size 32KB, got %d", len(buf))
	}

	// 放回缓冲区
	BufferPool.Put(buf)

	// 再次获取（可能是同一个）
	buf2 := BufferPool.Get()
	if buf2 == nil {
		t.Error("Expected non-nil buffer")
	}
}

func TestRealBufferPool(t *testing.T) {
	buf := GetBuffer()
	if buf == nil {
		t.Error("Expected non-nil buffer")
	}
	if len(buf) != 32*1024 {
		t.Errorf("Expected buffer size 32KB, got %d", len(buf))
	}

	PutBuffer(buf)
}

func TestMinSendfileSize(t *testing.T) {
	if MinSendfileSize != 8*1024 {
		t.Errorf("Expected MinSendfileSize 8KB, got %d", MinSendfileSize)
	}
}

func TestGetBuffer(t *testing.T) {
	buf := GetBuffer()
	if buf == nil {
		t.Error("Expected non-nil buffer")
		return
	}
	if len(buf) != 32*1024 {
		t.Errorf("Expected buffer size 32KB, got %d", len(buf))
	}

	// 测试写入
	copy(buf, []byte("test"))
	if string(buf[:4]) != "test" {
		t.Error("Expected to write 'test' to buffer")
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
	defer file.Close()

	// 测试平台 sendfile（小文件会 fallback 到 copyFile）
	// 由于没有真实的网络连接，这个测试主要验证不会崩溃
	_ = platformSendfile(nil, file, 0, int64(len(content)))
}

func TestBufferPoolConcurrent(t *testing.T) {
	const iterations = 100

	done := make(chan bool)

	for i := 0; i < iterations; i++ {
		go func() {
			buf := GetBuffer()
			PutBuffer(buf)
			done <- true
		}()
	}

	for i := 0; i < iterations; i++ {
		<-done
	}
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
	defer file.Close()

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
			file.Seek(0, io.SeekStart)

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
	if runtime.GOOS == "linux" {
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
	defer file.Close()

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

func (m *mockConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (m *mockConn) Write(b []byte) (n int, err error)  { return 0, nil }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }