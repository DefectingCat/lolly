// Package utils 提供工具函数的测试。
//
// 该文件测试工具模块的各项功能，包括：
//   - HTTPError 结构体和预定义错误
//   - SendError 函数
//   - SendErrorWithDetail 函数
//   - 内部重定向相关函数
//
// 作者：xfy
package utils

import (
	"testing"

	"github.com/valyala/fasthttp"
)

// TestHTTPErrorPredefined 测试预定义的 HTTP 错误
func TestHTTPErrorPredefined(t *testing.T) {
	tests := []struct {
		name       string
		err        HTTPError
		wantMsg    string
		wantStatus int
	}{
		{"NotFound", ErrNotFound, "Not Found", fasthttp.StatusNotFound},
		{"Forbidden", ErrForbidden, "Forbidden", fasthttp.StatusForbidden},
		{"Unauthorized", ErrUnauthorized, "Unauthorized", fasthttp.StatusUnauthorized},
		{"BadGateway", ErrBadGateway, "Bad Gateway", fasthttp.StatusBadGateway},
		{"GatewayTimeout", ErrGatewayTimeout, "Gateway Timeout", fasthttp.StatusGatewayTimeout},
		{"InternalError", ErrInternalError, "Internal Server Error", fasthttp.StatusInternalServerError},
		{"TooManyRequests", ErrTooManyRequests, "Too Many Requests", fasthttp.StatusTooManyRequests},
		{"ServiceUnavailable", ErrServiceUnavailable, "Service Unavailable", fasthttp.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", tt.err.Message, tt.wantMsg)
			}
			if tt.err.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", tt.err.StatusCode, tt.wantStatus)
			}
		})
	}
}

// TestSendError 测试 SendError 函数
func TestSendError(t *testing.T) {
	tests := []struct {
		name       string
		err        HTTPError
		wantBody   string
		wantStatus int
	}{
		{
			name:       "not_found",
			err:        ErrNotFound,
			wantBody:   "Not Found",
			wantStatus: fasthttp.StatusNotFound,
		},
		{
			name:       "internal_error",
			err:        ErrInternalError,
			wantBody:   "Internal Server Error",
			wantStatus: fasthttp.StatusInternalServerError,
		},
		{
			name:       "custom_error",
			err:        HTTPError{Message: "Custom Error", StatusCode: 418},
			wantBody:   "Custom Error",
			wantStatus: 418,
		},
		{
			name:       "empty_message",
			err:        HTTPError{Message: "", StatusCode: 200},
			wantBody:   "",
			wantStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/test")

			SendError(ctx, tt.err)

			if ctx.Response.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", ctx.Response.StatusCode(), tt.wantStatus)
			}
			if body := string(ctx.Response.Body()); body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

// TestSendErrorWithDetail 测试 SendErrorWithDetail 函数
func TestSendErrorWithDetail(t *testing.T) {
	tests := []struct {
		name       string
		err        HTTPError
		detail     string
		wantBody   string
		wantStatus int
	}{
		{
			name:       "with_detail",
			err:        ErrNotFound,
			detail:     "resource missing",
			wantBody:   "Not Found: resource missing",
			wantStatus: fasthttp.StatusNotFound,
		},
		{
			name:       "empty_detail",
			err:        ErrForbidden,
			detail:     "",
			wantBody:   "Forbidden",
			wantStatus: fasthttp.StatusForbidden,
		},
		{
			name:       "detail_with_special_chars",
			err:        ErrBadGateway,
			detail:     "upstream: http://backend:8080 (connection refused)",
			wantBody:   "Bad Gateway: upstream: http://backend:8080 (connection refused)",
			wantStatus: fasthttp.StatusBadGateway,
		},
		{
			name:       "detail_with_chinese",
			err:        ErrForbidden,
			detail:     "权限不足",
			wantBody:   "Forbidden: 权限不足",
			wantStatus: fasthttp.StatusForbidden,
		},
		{
			name:       "custom_error_with_detail",
			err:        HTTPError{Message: "Custom", StatusCode: 418},
			detail:     "I'm a teapot",
			wantBody:   "Custom: I'm a teapot",
			wantStatus: 418,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/test")

			SendErrorWithDetail(ctx, tt.err, tt.detail)

			if ctx.Response.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", ctx.Response.StatusCode(), tt.wantStatus)
			}
			if body := string(ctx.Response.Body()); body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

// TestInternalRedirectKey 测试内部重定向常量
func TestInternalRedirectKey(t *testing.T) {
	if InternalRedirectKey != "__internal_redirect__" {
		t.Errorf("InternalRedirectKey = %q, want %q", InternalRedirectKey, "__internal_redirect__")
	}
}

// TestInternalRedirect 测试内部重定向相关函数
func TestInternalRedirect(t *testing.T) {
	t.Run("SetAndGet", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		SetInternalRedirect(ctx, "/new/path")

		if !IsInternalRedirect(ctx) {
			t.Error("expected internal redirect to be set")
		}
		if path := GetInternalRedirectPath(ctx); path != "/new/path" {
			t.Errorf("path = %q, want %q", path, "/new/path")
		}
	})

	t.Run("NotSet", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}

		if IsInternalRedirect(ctx) {
			t.Error("expected no internal redirect")
		}
		if path := GetInternalRedirectPath(ctx); path != "" {
			t.Errorf("path = %q, want empty", path)
		}
	})

	t.Run("WrongType", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		ctx.SetUserValue(InternalRedirectKey, 123)

		if !IsInternalRedirect(ctx) {
			t.Error("expected internal redirect to be set")
		}
		if path := GetInternalRedirectPath(ctx); path != "" {
			t.Errorf("path = %q, want empty for wrong type", path)
		}
	})

	t.Run("EmptyPath", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		SetInternalRedirect(ctx, "")

		if !IsInternalRedirect(ctx) {
			t.Error("expected internal redirect to be set even with empty path")
		}
		if path := GetInternalRedirectPath(ctx); path != "" {
			t.Errorf("path = %q, want empty", path)
		}
	})

	t.Run("RootPath", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		SetInternalRedirect(ctx, "/")

		if !IsInternalRedirect(ctx) {
			t.Error("expected internal redirect to be set")
		}
		if path := GetInternalRedirectPath(ctx); path != "/" {
			t.Errorf("path = %q, want %q", path, "/")
		}
	})

	t.Run("PathWithQuery", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		SetInternalRedirect(ctx, "/api/health?check=all")

		if path := GetInternalRedirectPath(ctx); path != "/api/health?check=all" {
			t.Errorf("path = %q, want %q", path, "/api/health?check=all")
		}
	})

	t.Run("PathWithChinese", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		SetInternalRedirect(ctx, "/内部/健康检查")

		if path := GetInternalRedirectPath(ctx); path != "/内部/健康检查" {
			t.Errorf("path = %q, want %q", path, "/内部/健康检查")
		}
	})

	t.Run("NilUserValue", func(t *testing.T) {
		ctx := &fasthttp.RequestCtx{}
		ctx.SetUserValue(InternalRedirectKey, nil)

		if IsInternalRedirect(ctx) {
			t.Error("expected no internal redirect for nil value")
		}
		if path := GetInternalRedirectPath(ctx); path != "" {
			t.Errorf("path = %q, want empty", path)
		}
	})
}

// TestHTTPError_CustomErrors 测试自定义 HTTPError
func TestHTTPError_CustomErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        HTTPError
		wantMsg    string
		wantStatus int
	}{
		{
			name:       "teapot",
			err:        HTTPError{Message: "I'm a teapot", StatusCode: 418},
			wantMsg:    "I'm a teapot",
			wantStatus: 418,
		},
		{
			name:       "rate_limit",
			err:        HTTPError{Message: "Rate limit exceeded", StatusCode: 429},
			wantMsg:    "Rate limit exceeded",
			wantStatus: 429,
		},
		{
			name:       "empty_message",
			err:        HTTPError{Message: "", StatusCode: 500},
			wantMsg:    "",
			wantStatus: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", tt.err.Message, tt.wantMsg)
			}
			if tt.err.StatusCode != tt.wantStatus {
				t.Errorf("StatusCode = %d, want %d", tt.err.StatusCode, tt.wantStatus)
			}
		})
	}
}
