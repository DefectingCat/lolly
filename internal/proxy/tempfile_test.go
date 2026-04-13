// Package proxy 提供临时文件处理功能的测试。
//
// 该文件测试临时文件模块的各项功能，包括：
//   - 临时文件管理器创建
//   - 阈值判定逻辑
//   - 临时文件写入
//   - 动态检测切换
//   - 超过最大大小处理
//   - 清理功能
//
// 作者：xfy
package proxy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

// TestNewTempFileManager 测试临时文件管理器创建
func TestNewTempFileManager(t *testing.T) {
	tests := []struct {
		name        string
		tempPath    string
		threshold   string
		maxSize     string
		errContains string
		wantErr     bool
	}{
		{
			name:      "正常创建",
			tempPath:  t.TempDir(),
			threshold: "1mb",
			maxSize:   "1024mb",
			wantErr:   false,
		},
		{
			name:      "使用默认临时目录",
			tempPath:  "",
			threshold: "1mb",
			maxSize:   "1024mb",
			wantErr:   false,
		},
		{
			name:        "无效阈值格式",
			tempPath:    t.TempDir(),
			threshold:   "invalid",
			maxSize:     "1024mb",
			wantErr:     true,
			errContains: "temp_file_threshold",
		},
		{
			name:        "无效最大大小格式",
			tempPath:    t.TempDir(),
			threshold:   "1mb",
			maxSize:     "invalid",
			wantErr:     true,
			errContains: "max_temp_file_size",
		},
		{
			name:      "使用字节单位",
			tempPath:  t.TempDir(),
			threshold: "1048576",
			maxSize:   "1073741824",
			wantErr:   false,
		},
		{
			name:      "使用 kb 单位",
			tempPath:  t.TempDir(),
			threshold: "1024kb",
			maxSize:   "1048576kb",
			wantErr:   false,
		},
		{
			name:      "使用 gb 单位",
			tempPath:  t.TempDir(),
			threshold: "0.001gb",
			maxSize:   "1gb",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewTempFileManager(tt.tempPath, tt.threshold, tt.maxSize)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewTempFileManager() 期望错误但未返回")
					return
				}
				if tt.errContains != "" && !strContains(err.Error(), tt.errContains) {
					t.Errorf("NewTempFileManager() 错误 = %v, 应包含 %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("NewTempFileManager() 意外错误 = %v", err)
				return
			}

			if manager == nil {
				t.Error("NewTempFileManager() 返回 nil")
				return
			}

			// 验证阈值和最大值
			if manager.GetThreshold() <= 0 {
				t.Error("GetThreshold() 应返回正数")
			}
			if manager.GetMaxSize() <= 0 {
				t.Error("GetMaxSize() 应返回正数")
			}
		})
	}
}

// TestTempFileManager_ShouldUseTempFile 测试阈值判定
func TestTempFileManager_ShouldUseTempFile(t *testing.T) {
	manager, err := NewTempFileManager(t.TempDir(), "1mb", "1024mb")
	if err != nil {
		t.Fatalf("创建管理器失败: %v", err)
	}

	tests := []struct {
		name          string
		contentLength int64
		want          bool
	}{
		{
			name:          "正好等于阈值",
			contentLength: 1 << 20, // 1MB
			want:          true,
		},
		{
			name:          "超过阈值",
			contentLength: 2 << 20, // 2MB
			want:          true,
		},
		{
			name:          "低于阈值",
			contentLength: 512 << 10, // 512KB
			want:          false,
		},
		{
			name:          "未知大小",
			contentLength: -1,
			want:          false,
		},
		{
			name:          "零大小",
			contentLength: 0,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manager.ShouldUseTempFile(tt.contentLength)
			if got != tt.want {
				t.Errorf("ShouldUseTempFile(%d) = %v, want %v", tt.contentLength, got, tt.want)
			}
		})
	}
}

// TestTempFile_Write 测试临时文件写入
func TestTempFile_Write(t *testing.T) {
	manager, err := NewTempFileManager(t.TempDir(), "1mb", "10mb")
	if err != nil {
		t.Fatalf("创建管理器失败: %v", err)
	}

	t.Run("正常写入", func(t *testing.T) {
		tf, err := manager.CreateTempFile()
		if err != nil {
			t.Fatalf("创建临时文件失败: %v", err)
		}
		defer func() { _ = tf.Close() }()

		data := []byte("test data")
		n, err := tf.Write(data)
		if err != nil {
			t.Errorf("Write() 错误 = %v", err)
			return
		}
		if n != len(data) {
			t.Errorf("Write() 写入字节数 = %d, want %d", n, len(data))
		}
		if tf.GetSize() != int64(len(data)) {
			t.Errorf("GetSize() = %d, want %d", tf.GetSize(), len(data))
		}
	})

	t.Run("超过最大大小", func(t *testing.T) {
		// 创建小阈值管理器
		smallManager, err := NewTempFileManager(t.TempDir(), "1mb", "100b")
		if err != nil {
			t.Fatalf("创建管理器失败: %v", err)
		}

		tf, err := smallManager.CreateTempFile()
		if err != nil {
			t.Fatalf("创建临时文件失败: %v", err)
		}
		defer func() { _ = tf.Close() }()

		data := make([]byte, 200) // 超过 100b
		_, err = tf.Write(data)
		if err == nil {
			t.Error("Write() 应返回错误（超过最大大小）")
		}
		if !tf.IsExceeded() {
			t.Error("IsExceeded() 应为 true")
		}
	})
}

// TestTempFile_WriteTo 测试临时文件写入响应
func TestTempFile_WriteTo(t *testing.T) {
	manager, err := NewTempFileManager(t.TempDir(), "1mb", "10mb")
	if err != nil {
		t.Fatalf("创建管理器失败: %v", err)
	}

	tf, err := manager.CreateTempFile()
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer func() { _ = tf.Close() }()

	// 写入测试数据
	data := []byte("response body content")
	_, err = tf.Write(data)
	if err != nil {
		t.Fatalf("写入数据失败: %v", err)
	}

	// 写入响应
	ctx := &fasthttp.RequestCtx{}
	err = tf.WriteTo(ctx, 200)
	if err != nil {
		t.Errorf("WriteTo() 错误 = %v", err)
		return
	}

	// 验证状态码
	if ctx.Response.StatusCode() != 200 {
		t.Errorf("StatusCode() = %d, want 200", ctx.Response.StatusCode())
	}

	// 验证内容
	body := string(ctx.Response.Body())
	if body != "response body content" {
		t.Errorf("Body() = %q, want %q", body, "response body content")
	}
}

// TestTempFile_Close 测试临时文件关闭和清理
func TestTempFile_Close(t *testing.T) {
	manager, err := NewTempFileManager(t.TempDir(), "1mb", "10mb")
	if err != nil {
		t.Fatalf("创建管理器失败: %v", err)
	}

	tf, err := manager.CreateTempFile()
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	path := tf.GetPath()

	// 关闭文件
	err = tf.Close()
	if err != nil {
		t.Errorf("Close() 错误 = %v", err)
	}

	// 验证文件已删除
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Close() 后文件应被删除")
	}
}

// TestDynamicTempFileWriter 测试动态临时文件写入器
func TestDynamicTempFileWriter(t *testing.T) {
	t.Run("小响应使用缓冲区", func(t *testing.T) {
		manager, err := NewTempFileManager(t.TempDir(), "1mb", "10mb")
		if err != nil {
			t.Fatalf("创建管理器失败: %v", err)
		}

		writer := NewDynamicTempFileWriter(manager)
		defer writer.Cleanup()

		// 写入小数据（低于阈值）
		data := []byte("small data")
		err = writer.Write(data)
		if err != nil {
			t.Errorf("Write() 错误 = %v", err)
			return
		}

		// 验证最终化
		ctx := &fasthttp.RequestCtx{}
		err = writer.Finalize(ctx, 200)
		if err != nil {
			t.Errorf("Finalize() 错误 = %v", err)
			return
		}

		// 验证内容
		body := string(ctx.Response.Body())
		if body != "small data" {
			t.Errorf("Body() = %q, want %q", body, "small data")
		}
	})

	t.Run("大响应切换到临时文件", func(t *testing.T) {
		manager, err := NewTempFileManager(t.TempDir(), "100b", "10mb")
		if err != nil {
			t.Fatalf("创建管理器失败: %v", err)
		}

		writer := NewDynamicTempFileWriter(manager)
		defer writer.Cleanup()

		// 写入大数据（超过阈值）
		data := make([]byte, 200)
		for i := range data {
			data[i] = byte(i % 256)
		}
		err = writer.Write(data)
		if err != nil {
			t.Errorf("Write() 错误 = %v", err)
			return
		}

		// 验证最终化
		ctx := &fasthttp.RequestCtx{}
		err = writer.Finalize(ctx, 200)
		if err != nil {
			t.Errorf("Finalize() 错误 = %v", err)
			return
		}

		// 验证内容
		body := ctx.Response.Body()
		if len(body) != 200 {
			t.Errorf("Body() 长度 = %d, want 200", len(body))
		}
	})

	t.Run("超过最大大小返回错误", func(t *testing.T) {
		manager, err := NewTempFileManager(t.TempDir(), "1mb", "100b")
		if err != nil {
			t.Fatalf("创建管理器失败: %v", err)
		}

		writer := NewDynamicTempFileWriter(manager)
		defer writer.Cleanup()

		// 写入超过最大值的数据
		data := make([]byte, 200)
		err = writer.Write(data)
		if err == nil {
			t.Error("Write() 应返回错误（超过最大大小）")
		}
		if !writer.IsExceeded() {
			t.Error("IsExceeded() 应为 true")
		}
	})
}

// TestGetDefaultTempFileManager 测试默认临时文件管理器
func TestGetDefaultTempFileManager(t *testing.T) {
	manager1 := GetDefaultTempFileManager()
	if manager1 == nil {
		t.Fatal("GetDefaultTempFileManager() 返回 nil")
	}

	// 验证单例
	manager2 := GetDefaultTempFileManager()
	if manager1 != manager2 {
		t.Error("GetDefaultTempFileManager() 应返回相同的实例")
	}

	// 验证默认配置
	if manager1.GetThreshold() != 1<<20 {
		t.Errorf("默认阈值 = %d, want %d", manager1.GetThreshold(), 1<<20)
	}
	if manager1.GetMaxSize() != 1<<30 {
		t.Errorf("默认最大大小 = %d, want %d", manager1.GetMaxSize(), 1<<30)
	}
}

// TestTempFileCleaner 测试临时文件清理器
func TestTempFileCleaner(t *testing.T) {
	t.Run("创建和启动", func(t *testing.T) {
		tempDir := t.TempDir()
		cleaner := NewTempFileCleaner(tempDir, time.Second, time.Second)

		if cleaner.GetTempPath() != tempDir {
			t.Errorf("GetTempPath() = %s, want %s", cleaner.GetTempPath(), tempDir)
		}

		cleaner.Start()
		time.Sleep(100 * time.Millisecond)

		if cleaner.IsStopped() {
			t.Error("Start() 后 IsStopped() 应为 false")
		}

		cleaner.Stop()

		if !cleaner.IsStopped() {
			t.Error("Stop() 后 IsStopped() 应为 true")
		}
	})

	t.Run("清理过期文件", func(t *testing.T) {
		tempDir := t.TempDir()

		// 创建一个过期的临时文件
		oldFile := filepath.Join(tempDir, TempFilePrefix+"old")
		if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}

		// 修改文件修改时间为过去
		oldTime := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
			t.Fatalf("修改文件时间失败: %v", err)
		}

		// 创建一个非过期的临时文件
		newFile := filepath.Join(tempDir, TempFilePrefix+"new")
		if err := os.WriteFile(newFile, []byte("new"), 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}

		// 执行清理（1 小时过期时间）
		cleaner := NewTempFileCleaner(tempDir, time.Hour, time.Hour)
		cleaner.CleanupNow()

		// 验证过期文件被删除
		if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
			t.Error("过期文件应被删除")
		}

		// 验证新文件保留
		if _, err := os.Stat(newFile); err != nil {
			t.Error("新文件应被保留")
		}
	})

	t.Run("不清理非 lolly 前缀文件", func(t *testing.T) {
		tempDir := t.TempDir()

		// 创建一个非 lolly 前缀的文件
		otherFile := filepath.Join(tempDir, "other-file")
		if err := os.WriteFile(otherFile, []byte("other"), 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}

		// 修改文件修改时间为过去
		oldTime := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(otherFile, oldTime, oldTime); err != nil {
			t.Fatalf("修改文件时间失败: %v", err)
		}

		// 执行清理
		cleaner := NewTempFileCleaner(tempDir, time.Hour, time.Hour)
		cleaner.CleanupNow()

		// 验证非 lolly 文件保留
		if _, err := os.Stat(otherFile); err != nil {
			t.Error("非 lolly 前缀文件应被保留")
		}
	})

	t.Run("统计孤儿文件", func(t *testing.T) {
		tempDir := t.TempDir()

		// 创建孤儿文件
		orphanFile := filepath.Join(tempDir, TempFilePrefix+"orphan")
		if err := os.WriteFile(orphanFile, []byte("orphan"), 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}

		// 修改文件修改时间为过去
		oldTime := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(orphanFile, oldTime, oldTime); err != nil {
			t.Fatalf("修改文件时间失败: %v", err)
		}

		cleaner := NewTempFileCleaner(tempDir, time.Hour, time.Hour)
		count := cleaner.CountOrphanFiles()
		if count != 1 {
			t.Errorf("CountOrphanFiles() = %d, want 1", count)
		}
	})
}

// TestGlobalTempFileCleaner 测试全局临时文件清理器
func TestGlobalTempFileCleaner(t *testing.T) {
	// 启动全局清理器
	StartGlobalTempFileCleaner(os.TempDir())

	// 验证已启动
	cleaner := GetGlobalTempFileCleaner()
	if cleaner == nil {
		t.Error("GetGlobalTempFileCleaner() 不应返回 nil")
	}

	// 停止全局清理器
	StopGlobalTempFileCleaner()

	// 验证已停止
	cleaner = GetGlobalTempFileCleaner()
	if cleaner != nil {
		t.Error("StopGlobalTempFileCleaner() 后 GetGlobalTempFileCleaner() 应返回 nil")
	}
}

// strContains 检查字符串是否包含子串
func strContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strContainsHelper(s, substr))
}

func strContainsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
