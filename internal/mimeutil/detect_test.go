package mimeutil

import "testing"

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
