// Package server 提供 pprof 实现的测试。
package server

import (
	"bufio"
	"bytes"
	"sync"
	"testing"
)

// TestStartCPUProfile 测试 startCPUProfile 函数
func TestStartCPUProfile(t *testing.T) {
	t.Run("start and stop CPU profile", func(t *testing.T) {
		var buf bytes.Buffer

		err := startCPUProfile(&buf)
		if err != nil {
			t.Errorf("unexpected error starting CPU profile: %v", err)
		}

		// 停止 CPU profile
		stopCPUProfile()
	})

	t.Run("start twice should not error", func(t *testing.T) {
		var buf1, buf2 bytes.Buffer

		err1 := startCPUProfile(&buf1)
		if err1 != nil {
			t.Errorf("unexpected error on first start: %v", err1)
		}

		// 第二次启动应该被忽略（已在采集）
		err2 := startCPUProfile(&buf2)
		if err2 != nil {
			t.Errorf("unexpected error on second start: %v", err2)
		}

		stopCPUProfile()
	})

	t.Run("stop without start should not panic", func(t *testing.T) {
		// 确保停止状态
		stopCPUProfile()
		// 再次停止应该安全
		stopCPUProfile()
	})
}

// TestStopCPUProfile 测试 stopCPUProfile 函数
func TestStopCPUProfile(t *testing.T) {
	t.Run("stop when not active", func(t *testing.T) {
		// 确保停止状态
		stopCPUProfile()
		// 再次停止应该安全
		stopCPUProfile()
	})

	t.Run("stop when active", func(t *testing.T) {
		var buf bytes.Buffer
		err := startCPUProfile(&buf)
		if err != nil {
			t.Fatalf("failed to start CPU profile: %v", err)
		}

		stopCPUProfile()

		// 验证可以再次启动
		err = startCPUProfile(&buf)
		if err != nil {
			t.Errorf("failed to restart CPU profile after stop: %v", err)
		}
		stopCPUProfile()
	})
}

// TestWriteHeapProfile 测试 writeHeapProfile 函数
func TestWriteHeapProfile(t *testing.T) {
	t.Run("write heap profile", func(t *testing.T) {
		var buf bytes.Buffer

		// 写入 heap profile
		writeHeapProfile(&buf)

		// 验证有输出
		if buf.Len() == 0 {
			t.Error("expected heap profile output, got empty buffer")
		}
	})

	t.Run("write heap profile multiple times", func(t *testing.T) {
		var buf1, buf2 bytes.Buffer

		writeHeapProfile(&buf1)
		writeHeapProfile(&buf2)

		// 两次都应该有输出
		if buf1.Len() == 0 {
			t.Error("expected heap profile output on first call")
		}
		if buf2.Len() == 0 {
			t.Error("expected heap profile output on second call")
		}
	})
}

// TestWriteGoroutineProfile 测试 writeGoroutineProfile 函数
func TestWriteGoroutineProfile(t *testing.T) {
	t.Run("write goroutine profile", func(t *testing.T) {
		var buf bytes.Buffer

		writeGoroutineProfile(&buf)

		// 验证有输出
		if buf.Len() == 0 {
			t.Error("expected goroutine profile output, got empty buffer")
		}
	})

	t.Run("write goroutine profile with spawned goroutines", func(t *testing.T) {
		var buf bytes.Buffer

		// 启动一些 goroutine
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {}
			}()
		}

		writeGoroutineProfile(&buf)

		// 应该有输出
		if buf.Len() == 0 {
			t.Error("expected goroutine profile output")
		}
	})
}

// TestWriteBlockProfile 测试 writeBlockProfile 函数
func TestWriteBlockProfile(t *testing.T) {
	t.Run("write block profile", func(t *testing.T) {
		var buf bytes.Buffer

		writeBlockProfile(&buf)

		// block profile 可能为空（如果没有阻塞操作）
		// 所以我们只验证函数不会 panic
	})
}

// TestWriteMutexProfile 测试 writeMutexProfile 函数
func TestWriteMutexProfile(t *testing.T) {
	t.Run("write mutex profile", func(t *testing.T) {
		var buf bytes.Buffer

		writeMutexProfile(&buf)

		// mutex profile 可能为空（如果没有锁竞争）
		// 所以我们只验证函数不会 panic
	})
}

// TestBufioWriterAdapter 测试 bufioWriterAdapter 结构体
func TestBufioWriterAdapter(t *testing.T) {
	t.Run("write and flush", func(t *testing.T) {
		var buf bytes.Buffer
		bw := bufio.NewWriter(&buf)
		writer := wrapBufioWriter(bw)

		data := []byte("test data")
		n, err := writer.Write(data)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if n != len(data) {
			t.Errorf("expected %d bytes written, got %d", len(data), n)
		}
	})

	t.Run("write multiple times", func(t *testing.T) {
		var buf bytes.Buffer
		bw := bufio.NewWriter(&buf)
		writer := wrapBufioWriter(bw)

		data1 := []byte("first")
		data2 := []byte("second")

		n1, err1 := writer.Write(data1)
		if err1 != nil {
			t.Errorf("unexpected error on first write: %v", err1)
		}
		if n1 != len(data1) {
			t.Errorf("expected %d bytes on first write, got %d", len(data1), n1)
		}

		n2, err2 := writer.Write(data2)
		if err2 != nil {
			t.Errorf("unexpected error on second write: %v", err2)
		}
		if n2 != len(data2) {
			t.Errorf("expected %d bytes on second write, got %d", len(data2), n2)
		}
	})
}

// TestWrapBufioWriter 测试 wrapBufioWriter 函数
func TestWrapBufioWriter(t *testing.T) {
	t.Run("wrap returns non-nil", func(t *testing.T) {
		var buf bytes.Buffer
		bw := bufio.NewWriter(&buf)
		writer := wrapBufioWriter(bw)

		if writer == nil {
			t.Error("expected non-nil writer")
		}
	})

	t.Run("wrapped writer implements io.Writer", func(t *testing.T) {
		var buf bytes.Buffer
		bw := bufio.NewWriter(&buf)
		writer := wrapBufioWriter(bw)

		// 测试写入
		data := []byte("hello world")
		n, err := writer.Write(data)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if n != len(data) {
			t.Errorf("expected %d bytes, got %d", len(data), n)
		}
	})
}

// TestCPUProfileMutex 测试 CPU profile 的并发安全性
func TestCPUProfileMutex(t *testing.T) {
	t.Run("concurrent start/stop", func(t *testing.T) {
		var wg sync.WaitGroup
		var buf bytes.Buffer

		// 启动多个 goroutine 同时操作
		for i := 0; i < 10; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				_ = startCPUProfile(&buf)
			}()
			go func() {
				defer wg.Done()
				stopCPUProfile()
			}()
		}

		wg.Wait()

		// 确保最终状态一致
		stopCPUProfile()
	})
}
