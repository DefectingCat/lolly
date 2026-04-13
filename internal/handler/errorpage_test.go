// Package handler 提供错误页面管理器功能的测试。
//
// 该文件测试错误页面管理模块的各项功能，包括：
//   - 管理器构造函数
//   - 空配置处理
//   - 部分加载失败处理
//   - 全部加载失败处理
//   - 错误页面查找
//   - 默认页面 fallback
//   - 状态码检查
//   - 响应码覆盖
//   - 配置状态检查
//
// 作者：xfy
package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rua.plus/lolly/internal/config"
)

// TestNewErrorPageManager_EmptyConfig 测试空配置情况
func TestNewErrorPageManager_EmptyConfig(t *testing.T) {
	tests := []struct {
		want *ErrorPageManager
		name string
		cfg  config.ErrorPageConfig
	}{
		{
			name: "完全空配置",
			cfg:  config.ErrorPageConfig{},
		},
		{
			name: "空的 pages 和 default",
			cfg: config.ErrorPageConfig{
				Pages:   map[int]string{},
				Default: "",
			},
		},
		{
			name: "设置了 responseCode 但无页面",
			cfg: config.ErrorPageConfig{
				Pages:        map[int]string{},
				Default:      "",
				ResponseCode: 200,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewErrorPageManager(&tt.cfg)
			if err != nil {
				t.Errorf("NewErrorPageManager() 不应返回错误, got %v", err)
			}
			if manager == nil {
				t.Fatal("NewErrorPageManager() 返回 nil")
			}
			if manager.IsConfigured() {
				t.Error("IsConfigured() 应返回 false")
			}
			if manager.GetResponseCode() != tt.cfg.ResponseCode {
				t.Errorf("GetResponseCode() = %d, want %d", manager.GetResponseCode(), tt.cfg.ResponseCode)
			}
		})
	}
}

// TestNewErrorPageManager_PartialLoadFailure 测试部分加载失败
func TestNewErrorPageManager_PartialLoadFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建有效的错误页面文件
	validPage := filepath.Join(tmpDir, "404.html")
	if err := os.WriteFile(validPage, []byte("404 page"), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 不存在的文件路径
	nonExistent := filepath.Join(tmpDir, "nonexistent", "500.html")

	tests := []struct {
		wantPages      map[int]bool
		name           string
		cfg            config.ErrorPageConfig
		wantConfigured bool
		wantPartialErr bool
	}{
		{
			name: "一个成功一个失败",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{
					404: validPage,
					500: nonExistent,
				},
			},
			wantConfigured: true,
			wantPages:      map[int]bool{404: true},
			wantPartialErr: true,
		},
		{
			name: "特定页面成功默认失败",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{
					404: validPage,
				},
				Default: filepath.Join(tmpDir, "default.html"),
			},
			wantConfigured: true,
			wantPages:      map[int]bool{404: true},
			wantPartialErr: true,
		},
		{
			name: "默认成功特定页面失败",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{
					404: nonExistent,
				},
				Default: validPage,
			},
			wantConfigured: true,
			wantPages:      map[int]bool{},
			wantPartialErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewErrorPageManager(&tt.cfg)

			// 检查是否为部分错误
			if tt.wantPartialErr {
				if err == nil {
					t.Error("期望返回部分加载错误，但 got nil")
					return
				}
				if _, ok := err.(*PartialLoadError); !ok {
					t.Errorf("期望返回 *PartialLoadError，但 got %T", err)
					return
				}
			} else if err != nil {
				t.Errorf("NewErrorPageManager() 不应返回错误, got %v", err)
				return
			}

			if manager == nil {
				t.Fatal("NewErrorPageManager() 返回 nil")
			}

			if got := manager.IsConfigured(); got != tt.wantConfigured {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.wantConfigured)
			}

			// 检查特定页面是否存在
			for code, shouldExist := range tt.wantPages {
				if got := manager.HasPage(code); got != shouldExist {
					t.Errorf("HasPage(%d) = %v, want %v", code, got, shouldExist)
				}
			}
		})
	}
}

// TestNewErrorPageManager_AllLoadFailure 测试全部加载失败
func TestNewErrorPageManager_AllLoadFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// 不存在的文件路径
	nonExistent1 := filepath.Join(tmpDir, "nonexistent1.html")
	nonExistent2 := filepath.Join(tmpDir, "nonexistent2.html")

	tests := []struct {
		name    string
		cfg     config.ErrorPageConfig
		wantErr bool
	}{
		{
			name: "单个页面加载失败",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{
					404: nonExistent1,
				},
			},
			wantErr: true,
		},
		{
			name: "多个页面全部加载失败",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{
					404: nonExistent1,
					500: nonExistent2,
				},
			},
			wantErr: true,
		},
		{
			name: "默认页面加载失败",
			cfg: config.ErrorPageConfig{
				Default: nonExistent1,
			},
			wantErr: true,
		},
		{
			name: "页面和默认都加载失败",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{
					404: nonExistent1,
				},
				Default: nonExistent2,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewErrorPageManager(&tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("期望返回错误，但 got nil")
				}
				// 全部失败时不应返回 PartialLoadError
				if _, ok := err.(*PartialLoadError); ok {
					t.Error("全部失败时不应返回 *PartialLoadError")
				}
				if manager != nil {
					t.Error("全部失败时 manager 应为 nil")
				}
			} else {
				if err != nil {
					t.Errorf("NewErrorPageManager() 不应返回错误, got %v", err)
				}
			}
		})
	}
}

// TestPartialLoadError_Error 测试错误消息格式
func TestPartialLoadError_Error(t *testing.T) {
	tests := []struct {
		name   string
		errors map[int]error
		want   string
	}{
		{
			name:   "单个错误",
			errors: map[int]error{404: os.ErrNotExist},
			want:   "部分错误页面加载失败: 1 个错误",
		},
		{
			name:   "多个错误",
			errors: map[int]error{404: os.ErrNotExist, 500: os.ErrPermission},
			want:   "部分错误页面加载失败: 2 个错误",
		},
		{
			name:   "包含默认页面错误",
			errors: map[int]error{0: os.ErrNotExist, 404: os.ErrNotExist},
			want:   "部分错误页面加载失败: 2 个错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &PartialLoadError{Errors: tt.errors}
			got := err.Error()
			if !strings.Contains(got, tt.want) {
				t.Errorf("Error() = %q, want contain %q", got, tt.want)
			}
		})
	}
}

// TestErrorPageManager_GetPage 测试获取错误页面
func TestErrorPageManager_GetPage(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试页面文件
	page404 := filepath.Join(tmpDir, "404.html")
	page500 := filepath.Join(tmpDir, "500.html")
	pageDefault := filepath.Join(tmpDir, "default.html")

	if err := os.WriteFile(page404, []byte("404 page content"), 0o644); err != nil {
		t.Fatalf("创建 404 页面失败: %v", err)
	}
	if err := os.WriteFile(page500, []byte("500 page content"), 0o644); err != nil {
		t.Fatalf("创建 500 页面失败: %v", err)
	}
	if err := os.WriteFile(pageDefault, []byte("default page content"), 0o644); err != nil {
		t.Fatalf("创建默认页面失败: %v", err)
	}

	tests := []struct {
		name             string
		wantContent      string
		cfg              config.ErrorPageConfig
		requestCode      int
		wantResponseCode int
		wantFound        bool
	}{
		{
			name: "找到特定状态码页面",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{
					404: page404,
					500: page500,
				},
			},
			requestCode:      404,
			wantContent:      "404 page content",
			wantFound:        true,
			wantResponseCode: 404,
		},
		{
			name: "找到另一个状态码页面",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{
					404: page404,
					500: page500,
				},
			},
			requestCode:      500,
			wantContent:      "500 page content",
			wantFound:        true,
			wantResponseCode: 500,
		},
		{
			name: "未找到特定页面，使用默认页面",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{
					404: page404,
				},
				Default: pageDefault,
			},
			requestCode:      500,
			wantContent:      "default page content",
			wantFound:        true,
			wantResponseCode: 500,
		},
		{
			name: "未找到页面且无默认页面",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{
					404: page404,
				},
			},
			requestCode:      500,
			wantContent:      "",
			wantFound:        false,
			wantResponseCode: 500,
		},
		{
			name: "有默认页面但请求特定页面",
			cfg: config.ErrorPageConfig{
				Default: pageDefault,
			},
			requestCode:      503,
			wantContent:      "default page content",
			wantFound:        true,
			wantResponseCode: 503,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewErrorPageManager(&tt.cfg)
			if err != nil {
				t.Fatalf("NewErrorPageManager() 失败: %v", err)
			}

			content, found, responseCode := manager.GetPage(tt.requestCode)

			if found != tt.wantFound {
				t.Errorf("GetPage() found = %v, want %v", found, tt.wantFound)
			}
			if string(content) != tt.wantContent {
				t.Errorf("GetPage() content = %q, want %q", string(content), tt.wantContent)
			}
			if responseCode != tt.wantResponseCode {
				t.Errorf("GetPage() responseCode = %d, want %d", responseCode, tt.wantResponseCode)
			}
		})
	}
}

// TestErrorPageManager_GetPage_WithResponseCodeOverride 测试响应码覆盖
func TestErrorPageManager_GetPage_WithResponseCodeOverride(t *testing.T) {
	tmpDir := t.TempDir()

	page404 := filepath.Join(tmpDir, "404.html")
	if err := os.WriteFile(page404, []byte("404 page"), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	cfg := config.ErrorPageConfig{
		Pages: map[int]string{
			404: page404,
		},
		ResponseCode: 200, // 覆盖响应码
	}

	manager, err := NewErrorPageManager(&cfg)
	if err != nil {
		t.Fatalf("NewErrorPageManager() 失败: %v", err)
	}

	_, _, responseCode := manager.GetPage(404)
	if responseCode != 200 {
		t.Errorf("GetPage() responseCode = %d, want 200", responseCode)
	}

	// 测试默认页面也受覆盖影响
	cfg2 := config.ErrorPageConfig{
		Default:      page404,
		ResponseCode: 418,
	}

	manager2, err := NewErrorPageManager(&cfg2)
	if err != nil {
		t.Fatalf("NewErrorPageManager() 失败: %v", err)
	}

	_, _, responseCode2 := manager2.GetPage(500)
	if responseCode2 != 418 {
		t.Errorf("GetPage() responseCode = %d, want 418", responseCode2)
	}
}

// TestErrorPageManager_HasPage 测试页面存在检查
func TestErrorPageManager_HasPage(t *testing.T) {
	tmpDir := t.TempDir()

	page404 := filepath.Join(tmpDir, "404.html")
	pageDefault := filepath.Join(tmpDir, "default.html")

	if err := os.WriteFile(page404, []byte("404"), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	if err := os.WriteFile(pageDefault, []byte("default"), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	tests := []struct {
		name     string
		cfg      config.ErrorPageConfig
		code     int
		expected bool
	}{
		{
			name: "配置的特定页面存在",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{404: page404},
			},
			code:     404,
			expected: true,
		},
		{
			name: "未配置的页面但有默认页面",
			cfg: config.ErrorPageConfig{
				Pages:   map[int]string{404: page404},
				Default: pageDefault,
			},
			code:     500,
			expected: true, // 因为有默认页面
		},
		{
			name: "只有默认页面",
			cfg: config.ErrorPageConfig{
				Default: pageDefault,
			},
			code:     404,
			expected: true,
		},
		{
			name: "页面不存在且无默认页面",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{404: page404},
			},
			code:     500,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewErrorPageManager(&tt.cfg)
			if err != nil {
				t.Fatalf("NewErrorPageManager() 失败: %v", err)
			}

			if got := manager.HasPage(tt.code); got != tt.expected {
				t.Errorf("HasPage(%d) = %v, want %v", tt.code, got, tt.expected)
			}
		})
	}
}

// TestErrorPageManager_GetResponseCode 测试获取响应码覆盖值
func TestErrorPageManager_GetResponseCode(t *testing.T) {
	tests := []struct {
		name string
		code int
		want int
	}{
		{
			name: "无覆盖",
			code: 0,
			want: 0,
		},
		{
			name: "覆盖为 200",
			code: 200,
			want: 200,
		},
		{
			name: "覆盖为 418",
			code: 418,
			want: 418,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ErrorPageConfig{
				ResponseCode: tt.code,
			}
			manager, err := NewErrorPageManager(&cfg)
			if err != nil {
				t.Fatalf("NewErrorPageManager() 失败: %v", err)
			}

			if got := manager.GetResponseCode(); got != tt.want {
				t.Errorf("GetResponseCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestErrorPageManager_IsConfigured 测试配置状态检查
func TestErrorPageManager_IsConfigured(t *testing.T) {
	tmpDir := t.TempDir()

	page404 := filepath.Join(tmpDir, "404.html")
	pageDefault := filepath.Join(tmpDir, "default.html")

	if err := os.WriteFile(page404, []byte("404"), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	if err := os.WriteFile(pageDefault, []byte("default"), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	tests := []struct {
		name     string
		cfg      config.ErrorPageConfig
		expected bool
	}{
		{
			name:     "空配置",
			cfg:      config.ErrorPageConfig{},
			expected: false,
		},
		{
			name: "配置特定页面",
			cfg: config.ErrorPageConfig{
				Pages: map[int]string{404: page404},
			},
			expected: true,
		},
		{
			name: "配置默认页面",
			cfg: config.ErrorPageConfig{
				Default: pageDefault,
			},
			expected: true,
		},
		{
			name: "配置两者",
			cfg: config.ErrorPageConfig{
				Pages:   map[int]string{404: page404},
				Default: pageDefault,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewErrorPageManager(&tt.cfg)
			if err != nil {
				t.Fatalf("NewErrorPageManager() 失败: %v", err)
			}

			if got := manager.IsConfigured(); got != tt.expected {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestErrorPageManager_SuccessfulLoad 测试成功加载场景
func TestErrorPageManager_SuccessfulLoad(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建多个测试页面
	pages := map[int]string{
		404: filepath.Join(tmpDir, "404.html"),
		500: filepath.Join(tmpDir, "500.html"),
		403: filepath.Join(tmpDir, "403.html"),
	}
	defaultPage := filepath.Join(tmpDir, "default.html")

	for code, path := range pages {
		content := []byte(fmt.Sprintf("Error %d page", code))
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("创建页面 %d 失败: %v", code, err)
		}
	}
	if err := os.WriteFile(defaultPage, []byte("Default error page"), 0o644); err != nil {
		t.Fatalf("创建默认页面失败: %v", err)
	}

	cfg := config.ErrorPageConfig{
		Pages: map[int]string{
			404: pages[404],
			500: pages[500],
			403: pages[403],
		},
		Default: defaultPage,
	}

	manager, err := NewErrorPageManager(&cfg)
	if err != nil {
		t.Fatalf("NewErrorPageManager() 失败: %v", err)
	}

	// 验证所有页面都能正常访问
	for code := range pages {
		content, found, responseCode := manager.GetPage(code)
		if !found {
			t.Errorf("GetPage(%d) 应返回 found=true", code)
		}
		if responseCode != code {
			t.Errorf("GetPage(%d) responseCode = %d, want %d", code, responseCode, code)
		}
		wantContent := fmt.Sprintf("Error %d page", code)
		if string(content) != wantContent {
			t.Errorf("GetPage(%d) content = %q, want %q", code, string(content), wantContent)
		}
	}

	// 验证未配置的状态码返回默认页面
	content, found, responseCode := manager.GetPage(503)
	if !found {
		t.Error("GetPage(503) 应返回 found=true (默认页面)")
	}
	if responseCode != 503 {
		t.Errorf("GetPage(503) responseCode = %d, want 503", responseCode)
	}
	if string(content) != "Default error page" {
		t.Errorf("GetPage(503) content = %q, want default page", string(content))
	}
}
