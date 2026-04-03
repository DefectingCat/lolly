package handler

import (
	"os"
	"path/filepath"
	"testing"
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
