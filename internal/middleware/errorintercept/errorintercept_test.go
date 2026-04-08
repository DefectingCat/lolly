// Package errorintercept 提供 HTTP 错误拦截中间件的测试。
//
// 该文件测试错误拦截中间件的各项功能，包括：
//   - 中间件实例创建
//   - 中间件名称获取
//   - 错误响应拦截
//   - 4xx/5xx 状态码检测
//   - 错误页面替换
//   - 边界值测试
//
// 作者：xfy
package errorintercept

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/handler"
)

// TestNew 测试创建错误拦截中间件。
func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		manager *handler.ErrorPageManager
	}{
		{
			name:    "创建带有 manager 的实例",
			manager: &handler.ErrorPageManager{},
		},
		{
			name:    "创建带有 nil manager 的实例",
			manager: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ei := New(tt.manager)

			if ei == nil {
				t.Fatal("New() returned nil")
			}

			if ei.manager != tt.manager {
				t.Errorf("expected manager %v, got %v", tt.manager, ei.manager)
			}
		})
	}
}

// TestErrorIntercept_Name 测试获取中间件名称。
func TestErrorIntercept_Name(t *testing.T) {
	ei := New(nil)

	name := ei.Name()

	if name != "ErrorIntercept" {
		t.Errorf("expected name 'ErrorIntercept', got '%s'", name)
	}
}

// TestErrorIntercept_Process_NilManager 测试 nil manager 情况下的 Process。
func TestErrorIntercept_Process_NilManager(t *testing.T) {
	ei := New(nil)

	called := false
	next := func(ctx *fasthttp.RequestCtx) {
		called = true
		ctx.SetStatusCode(200)
		ctx.SetBodyString("OK")
	}

	wrapped := ei.Process(next)

	if wrapped == nil {
		t.Fatal("Process() returned nil")
	}

	var ctx fasthttp.RequestCtx
	ctx.Init(&fasthttp.Request{}, nil, nil)
	wrapped(&ctx)

	if !called {
		t.Error("next handler was not called")
	}
}

// TestErrorIntercept_Process_NotConfigured 测试未配置错误页面的情况。
func TestErrorIntercept_Process_NotConfigured(t *testing.T) {
	// 创建一个空 manager（没有配置任何页面）
	manager, err := handler.NewErrorPageManager(&config.ErrorPageConfig{
		Pages: make(map[int]string),
	})
	if err != nil {
		t.Skipf("跳过测试：无法创建 manager: %v", err)
	}
	ei := New(manager)

	called := false
	next := func(ctx *fasthttp.RequestCtx) {
		called = true
		ctx.SetStatusCode(200)
		ctx.SetBodyString("OK")
	}

	wrapped := ei.Process(next)

	// 未配置时应该返回 next 本身（通过行为验证）
	// 当 manager 未配置时，Process 直接返回 next，不会包装
	// 验证方式是确认 wrapped 调用后会直接执行 next 且不做额外操作
	if wrapped == nil {
		t.Fatal("Process() returned nil")
	}

	var ctx fasthttp.RequestCtx
	ctx.Init(&fasthttp.Request{}, nil, nil)
	wrapped(&ctx)

	if !called {
		t.Error("next handler was not called")
	}
}

// TestErrorIntercept_Process_SuccessStatus 测试成功状态码不拦截。
func TestErrorIntercept_Process_SuccessStatus(t *testing.T) {
	manager := createConfiguredManager(t)
	if manager == nil {
		t.Skip("跳过测试：无法创建配置好的 manager")
	}
	ei := New(manager)

	tests := []int{200, 201, 299, 301, 304, 399}

	for _, status := range tests {
		t.Run("状态码_"+string(rune('0'+status/100)), func(t *testing.T) {
			next := func(ctx *fasthttp.RequestCtx) {
				ctx.SetStatusCode(status)
				ctx.SetBodyString("success")
			}

			wrapped := ei.Process(next)

			var ctx fasthttp.RequestCtx
			ctx.Init(&fasthttp.Request{}, nil, nil)

			wrapped(&ctx)

			// 验证状态码未被修改
			if ctx.Response.StatusCode() != status {
				t.Errorf("expected status %d, got %d", status, ctx.Response.StatusCode())
			}

			// 验证 body 未被修改
			if string(ctx.Response.Body()) != "success" {
				t.Errorf("expected body 'success', got '%s'", string(ctx.Response.Body()))
			}
		})
	}
}

// TestErrorIntercept_Process_ErrorStatus_Intercepted 测试错误状态码被拦截并替换。
func TestErrorIntercept_Process_ErrorStatus_Intercepted(t *testing.T) {
	tempDir := t.TempDir()

	// 创建测试用的错误页面文件
	page404 := filepath.Join(tempDir, "404.html")
	page500 := filepath.Join(tempDir, "500.html")
	pageDefault := filepath.Join(tempDir, "default.html")

	if err := os.WriteFile(page404, []byte("<html>404 Not Found</html>"), 0644); err != nil {
		t.Fatalf("创建 404.html 失败: %v", err)
	}
	if err := os.WriteFile(page500, []byte("<html>500 Error</html>"), 0644); err != nil {
		t.Fatalf("创建 500.html 失败: %v", err)
	}
	if err := os.WriteFile(pageDefault, []byte("<html>Default Error</html>"), 0644); err != nil {
		t.Fatalf("创建 default.html 失败: %v", err)
	}

	manager, err := handler.NewErrorPageManager(&config.ErrorPageConfig{
		Pages: map[int]string{
			404: page404,
			500: page500,
		},
		Default: pageDefault,
	})
	if err != nil {
		t.Fatalf("创建 ErrorPageManager 失败: %v", err)
	}

	ei := New(manager)

	tests := []struct {
		name           string
		statusCode     int
		expectedBody   string
		expectedStatus int
	}{
		{
			name:           "拦截 404 错误",
			statusCode:     404,
			expectedBody:   "<html>404 Not Found</html>",
			expectedStatus: 404,
		},
		{
			name:           "拦截 500 错误",
			statusCode:     500,
			expectedBody:   "<html>500 Error</html>",
			expectedStatus: 500,
		},
		{
			name:           "拦截 403 错误（使用默认页面）",
			statusCode:     403,
			expectedBody:   "<html>Default Error</html>",
			expectedStatus: 403,
		},
		{
			name:           "拦截 502 错误（使用默认页面）",
			statusCode:     502,
			expectedBody:   "<html>Default Error</html>",
			expectedStatus: 502,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := func(ctx *fasthttp.RequestCtx) {
				ctx.SetStatusCode(tt.statusCode)
				ctx.SetBodyString("original error response")
			}

			wrapped := ei.Process(next)

			var ctx fasthttp.RequestCtx
			ctx.Init(&fasthttp.Request{}, nil, nil)

			wrapped(&ctx)

			// 验证 body 被替换
			if string(ctx.Response.Body()) != tt.expectedBody {
				t.Errorf("expected body '%s', got '%s'", tt.expectedBody, string(ctx.Response.Body()))
			}

			// 验证状态码
			if ctx.Response.StatusCode() != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, ctx.Response.StatusCode())
			}

			// 验证 content-type
			contentType := string(ctx.Response.Header.ContentType())
			if contentType != "text/html; charset=utf-8" {
				t.Errorf("expected content-type 'text/html; charset=utf-8', got '%s'", contentType)
			}
		})
	}
}

// TestErrorIntercept_Process_WithResponseCodeOverride 测试响应状态码覆盖。
func TestErrorIntercept_Process_WithResponseCodeOverride(t *testing.T) {
	tempDir := t.TempDir()

	page404 := filepath.Join(tempDir, "404.html")
	if err := os.WriteFile(page404, []byte("<html>404 Not Found</html>"), 0644); err != nil {
		t.Fatalf("创建 404.html 失败: %v", err)
	}

	manager, err := handler.NewErrorPageManager(&config.ErrorPageConfig{
		Pages: map[int]string{
			404: page404,
		},
		ResponseCode: 200, // 覆盖状态码为 200
	})
	if err != nil {
		t.Fatalf("创建 ErrorPageManager 失败: %v", err)
	}

	ei := New(manager)

	next := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(404)
		ctx.SetBodyString("not found")
	}

	wrapped := ei.Process(next)

	var ctx fasthttp.RequestCtx
	ctx.Init(&fasthttp.Request{}, nil, nil)

	wrapped(&ctx)

	// 状态码应该被覆盖为 200
	if ctx.Response.StatusCode() != 200 {
		t.Errorf("expected status 200 (overridden), got %d", ctx.Response.StatusCode())
	}

	// body 应该被替换
	if string(ctx.Response.Body()) != "<html>404 Not Found</html>" {
		t.Errorf("expected custom error page, got '%s'", string(ctx.Response.Body()))
	}
}

// TestErrorIntercept_Process_NoMatchingPage 测试没有匹配错误页面的情况。
func TestErrorIntercept_Process_NoMatchingPage(t *testing.T) {
	tempDir := t.TempDir()

	// 只创建 404 页面，没有默认页面
	page404 := filepath.Join(tempDir, "404.html")
	if err := os.WriteFile(page404, []byte("<html>404 Not Found</html>"), 0644); err != nil {
		t.Fatalf("创建 404.html 失败: %v", err)
	}

	manager, err := handler.NewErrorPageManager(&config.ErrorPageConfig{
		Pages: map[int]string{
			404: page404,
		},
		// 没有配置默认页面
	})
	if err != nil {
		t.Fatalf("创建 ErrorPageManager 失败: %v", err)
	}

	ei := New(manager)

	// 请求 500，但没有配置 500 页面和默认页面
	next := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(500)
		ctx.SetBodyString("original 500 error")
	}

	wrapped := ei.Process(next)

	var ctx fasthttp.RequestCtx
	ctx.Init(&fasthttp.Request{}, nil, nil)

	wrapped(&ctx)

	// 没有匹配的错误页面，应该保持原样
	if ctx.Response.StatusCode() != 500 {
		t.Errorf("expected status 500, got %d", ctx.Response.StatusCode())
	}

	// body 不应该被修改（因为没有找到匹配页面）
	if string(ctx.Response.Body()) != "original 500 error" {
		t.Errorf("expected body unchanged, got '%s'", string(ctx.Response.Body()))
	}
}

// TestErrorIntercept_Process_4xxErrors 测试所有 4xx 错误被拦截。
func TestErrorIntercept_Process_4xxErrors(t *testing.T) {
	tempDir := t.TempDir()

	// 创建默认错误页面
	pageDefault := filepath.Join(tempDir, "default.html")
	if err := os.WriteFile(pageDefault, []byte("<html>Error</html>"), 0644); err != nil {
		t.Fatalf("创建 default.html 失败: %v", err)
	}

	manager, err := handler.NewErrorPageManager(&config.ErrorPageConfig{
		Default: pageDefault,
	})
	if err != nil {
		t.Fatalf("创建 ErrorPageManager 失败: %v", err)
	}

	ei := New(manager)

	// 测试不同的 4xx 错误码
	codes := []int{400, 401, 403, 404, 405, 408, 429, 499}

	for _, code := range codes {
		t.Run("状态码_"+string(rune('0'+code/100)), func(t *testing.T) {
			next := func(ctx *fasthttp.RequestCtx) {
				ctx.SetStatusCode(code)
				ctx.SetBodyString("error")
			}

			wrapped := ei.Process(next)

			var ctx fasthttp.RequestCtx
			ctx.Init(&fasthttp.Request{}, nil, nil)

			wrapped(&ctx)

			// 验证 body 被替换为默认页面
			if string(ctx.Response.Body()) != "<html>Error</html>" {
				t.Errorf("expected custom error page, got '%s'", string(ctx.Response.Body()))
			}

			// 验证状态码
			if ctx.Response.StatusCode() != code {
				t.Errorf("expected status %d, got %d", code, ctx.Response.StatusCode())
			}
		})
	}
}

// TestErrorIntercept_Process_5xxErrors 测试所有 5xx 错误被拦截。
func TestErrorIntercept_Process_5xxErrors(t *testing.T) {
	tempDir := t.TempDir()

	// 创建默认错误页面
	pageDefault := filepath.Join(tempDir, "default.html")
	if err := os.WriteFile(pageDefault, []byte("<html>Server Error</html>"), 0644); err != nil {
		t.Fatalf("创建 default.html 失败: %v", err)
	}

	manager, err := handler.NewErrorPageManager(&config.ErrorPageConfig{
		Default: pageDefault,
	})
	if err != nil {
		t.Fatalf("创建 ErrorPageManager 失败: %v", err)
	}

	ei := New(manager)

	// 测试不同的 5xx 错误码
	codes := []int{500, 501, 502, 503, 504, 505, 599}

	for _, code := range codes {
		t.Run("状态码_"+string(rune('0'+code/100)), func(t *testing.T) {
			next := func(ctx *fasthttp.RequestCtx) {
				ctx.SetStatusCode(code)
				ctx.SetBodyString("server error")
			}

			wrapped := ei.Process(next)

			var ctx fasthttp.RequestCtx
			ctx.Init(&fasthttp.Request{}, nil, nil)

			wrapped(&ctx)

			// 验证 body 被替换
			if string(ctx.Response.Body()) != "<html>Server Error</html>" {
				t.Errorf("expected custom error page, got '%s'", string(ctx.Response.Body()))
			}

			// 验证状态码
			if ctx.Response.StatusCode() != code {
				t.Errorf("expected status %d, got %d", code, ctx.Response.StatusCode())
			}
		})
	}
}

// TestIsErrorStatusCode 测试错误状态码检测。
func TestIsErrorStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected bool
	}{
		// 边界测试
		{"边界 399", 399, false},
		{"边界 400", 400, true},
		{"边界 599", 599, true},
		{"边界 600", 600, false},

		// 4xx 错误
		{"400 Bad Request", 400, true},
		{"401 Unauthorized", 401, true},
		{"403 Forbidden", 403, true},
		{"404 Not Found", 404, true},
		{"429 Too Many Requests", 429, true},
		{"499", 499, true},

		// 5xx 错误
		{"500 Internal Server Error", 500, true},
		{"502 Bad Gateway", 502, true},
		{"503 Service Unavailable", 503, true},
		{"504 Gateway Timeout", 504, true},

		// 非错误状态码
		{"200 OK", 200, false},
		{"201 Created", 201, false},
		{"301 Redirect", 301, false},
		{"304 Not Modified", 304, false},

		// 边缘值
		{"0", 0, false},
		{"负值 -1", -1, false},
		{"极大值 999", 999, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isErrorStatusCode(tt.code)
			if result != tt.expected {
				t.Errorf("isErrorStatusCode(%d) = %v, expected %v", tt.code, result, tt.expected)
			}
		})
	}
}

// TestErrorIntercept_GetManager 测试获取 manager。
func TestErrorIntercept_GetManager(t *testing.T) {
	manager := &handler.ErrorPageManager{}
	ei := New(manager)

	got := ei.GetManager()
	if got != manager {
		t.Error("GetManager() did not return the expected manager")
	}
}

// createConfiguredManager 创建一个已配置的 ErrorPageManager 用于测试。
func createConfiguredManager(t *testing.T) *handler.ErrorPageManager {
	tempDir := t.TempDir()

	// 创建一个简单的错误页面
	pageDefault := filepath.Join(tempDir, "default.html")
	if err := os.WriteFile(pageDefault, []byte("<html>Error</html>"), 0644); err != nil {
		t.Fatalf("创建 default.html 失败: %v", err)
	}

	manager, err := handler.NewErrorPageManager(&config.ErrorPageConfig{
		Default: pageDefault,
	})
	if err != nil {
		t.Fatalf("创建 ErrorPageManager 失败: %v", err)
	}

	return manager
}
