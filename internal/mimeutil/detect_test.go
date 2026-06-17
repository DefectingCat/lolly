package mimeutil

import (
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

		// 未知类型 - 回退到 defaultMIME
		{"test.unknown", "application/octet-stream"},
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

// TestDetectContentTypeUnknownCached 测试未知扩展名多次查询均回退到默认 MIME 类型。
// 防止缓存中写入空字符串导致后续查询返回错误值。
func TestDetectContentTypeUnknownCached(t *testing.T) {
	const unknownFile = "test.unknownxyz"

	for range 3 {
		got := DetectContentType(unknownFile)
		if got != defaultMIME {
			t.Errorf("DetectContentType(%q) = %q, want %q", unknownFile, got, defaultMIME)
		}
	}
}
