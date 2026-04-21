package mimeutil

import (
	"sync"
	"testing"
)

// TestDetectContentType 测试 MIME 类型检测
func TestDetectContentType(t *testing.T) {
	tests := []struct {
		filePath string
		want     string
	}{
		// 标准库已知类型 (验证回退)
		{"test.html", "text/html; charset=utf-8"},
		{"test.css", "text/css; charset=utf-8"},
		{"test.js", "text/javascript; charset=utf-8"},
		{"test.json", "application/json"},
		{"test.mjs", "text/javascript; charset=utf-8"}, // Go 1.26.2+ 已支持

		// 包本地补充类型 - 缺失
		{"test.eot", "application/vnd.ms-fontobject"},
		{"test.webmanifest", "application/manifest+json"},
		{"test.map", "application/json"},

		// 包本地覆盖类型 - Go 返回错误类型
		{"test.otf", "font/otf"},    // Go 返回 OpenDocument 公式模板
		{"test.webm", "video/webm"}, // Go 返回 audio/webm

		// 大小写处理 - 验证 strings.ToLower
		{"test.EOT", "application/vnd.ms-fontobject"}, // 不在 Go 表中
		{"test.WEBMANIFEST", "application/manifest+json"},
		{"test.JPG", "image/jpeg"}, // Go 已知，也处理大小写

		// 未知类型
		{"test.unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := DetectContentType(tt.filePath)
			if got != tt.want {
				t.Errorf("DetectContentType(%q) = %q, want %q", tt.filePath, got, tt.want)
			}
		})
	}
}

// TestAddTypes 测试自定义 MIME 类型添加
func TestAddTypes(t *testing.T) {
	tests := []struct {
		name     string
		types    map[string]string
		filePath string
		want     string
	}{
		{
			name:     "添加新扩展名",
			types:    map[string]string{".xyz": "application/x-xyz"},
			filePath: "test.xyz",
			want:     "application/x-xyz",
		},
		{
			name:     "覆盖已有映射",
			types:    map[string]string{".otf": "application/x-font-otf"},
			filePath: "test.otf",
			want:     "application/x-font-otf",
		},
		{
			name:     "大写扩展名自动转小写",
			types:    map[string]string{".ABC": "application/x-abc"},
			filePath: "test.abc",
			want:     "application/x-abc",
		},
		{
			name:     "多个映射同时添加",
			types:    map[string]string{".foo": "application/x-foo", ".bar": "application/x-bar"},
			filePath: "test.foo",
			want:     "application/x-foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 保存原始值以便恢复
			original := make(map[string]string)
			for ext := range tt.types {
				original[ext] = mimeOverrides[ext]
			}

			AddTypes(tt.types)
			got := DetectContentType(tt.filePath)

			if got != tt.want {
				t.Errorf("AddTypes: DetectContentType(%q) = %q, want %q", tt.filePath, got, tt.want)
			}

			// 恢复原始值
			mimeMutex.Lock()
			for ext, orig := range original {
				if orig == "" {
					delete(mimeOverrides, ext)
				} else {
					mimeOverrides[ext] = orig
				}
			}
			mimeMutex.Unlock()
		})
	}
}

// TestSetDefaultType 测试设置默认 MIME 类型
func TestSetDefaultType(t *testing.T) {
	// 保存原始默认值
	original := GetDefaultType()

	tests := []struct {
		name string
		want string
	}{
		{"设置为 text/plain", "text/plain"},
		{"设置为 application/json", "application/json"},
		{"设置为空字符串", ""},
		{"恢复默认值", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetDefaultType(tt.want)
			got := GetDefaultType()
			if got != tt.want {
				t.Errorf("SetDefaultType(%q): GetDefaultType() = %q, want %q", tt.want, got, tt.want)
			}
		})
	}

	// 恢复原始值
	SetDefaultType(original)
}

// TestGetDefaultType 测试获取默认 MIME 类型
func TestGetDefaultType(t *testing.T) {
	// 保存原始默认值
	original := GetDefaultType()

	// 验证默认值
	if original != "application/octet-stream" {
		t.Errorf("GetDefaultType() = %q, want %q", original, "application/octet-stream")
	}

	// 测试设置后获取
	SetDefaultType("text/plain")
	if got := GetDefaultType(); got != "text/plain" {
		t.Errorf("GetDefaultType() = %q, want %q", got, "text/plain")
	}

	// 恢复原始值
	SetDefaultType(original)
}

// TestConcurrentAccess 测试并发访问安全性
func TestConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	numGoroutines := 100

	// 并发调用 DetectContentType
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			DetectContentType("test.html")
		}()
	}

	// 并发调用 AddTypes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(i int) {
			defer wg.Done()
			AddTypes(map[string]string{".test": "application/x-test"})
		}(i)
	}

	// 并发调用 SetDefaultType 和 GetDefaultType
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			SetDefaultType("text/plain")
			_ = GetDefaultType()
		}()
	}

	wg.Wait()

	// 恢复默认值
	SetDefaultType("application/octet-stream")
}
