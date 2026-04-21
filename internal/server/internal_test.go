// Package server 提供内部重定向功能的测试。
package server

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestSetInternalRedirect(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	targetPath := "/internal/target"

	SetInternalRedirect(ctx, targetPath)

	// 验证值已设置
	v := ctx.UserValue(InternalRedirectKey)
	if v == nil {
		t.Error("expected user value to be set")
	}

	if v.(string) != targetPath {
		t.Errorf("expected %q, got %q", targetPath, v.(string))
	}
}

func TestIsInternalRedirect_True(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue(InternalRedirectKey, "/target")

	if !IsInternalRedirect(ctx) {
		t.Error("expected IsInternalRedirect to return true")
	}
}

func TestIsInternalRedirect_False(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}

	if IsInternalRedirect(ctx) {
		t.Error("expected IsInternalRedirect to return false")
	}
}

func TestGetInternalRedirectPath_Set(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	targetPath := "/internal/new-path"
	ctx.SetUserValue(InternalRedirectKey, targetPath)

	got := GetInternalRedirectPath(ctx)
	if got != targetPath {
		t.Errorf("expected %q, got %q", targetPath, got)
	}
}

func TestGetInternalRedirectPath_NotSet(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}

	got := GetInternalRedirectPath(ctx)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestGetInternalRedirectPath_WrongType(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue(InternalRedirectKey, 12345) // 设置非字符串值

	got := GetInternalRedirectPath(ctx)
	if got != "" {
		t.Errorf("expected empty string for wrong type, got %q", got)
	}
}

func TestInternalRedirectKey_Constant(t *testing.T) {
	// 验证常量值
	expectedKey := "__internal_redirect__"
	if InternalRedirectKey != expectedKey {
		t.Errorf("expected InternalRedirectKey to be %q, got %q", expectedKey, InternalRedirectKey)
	}
}

func TestInternalRedirect_RoundTrip(t *testing.T) {
	tests := []string{
		"/simple/path",
		"/path/with/query?foo=bar",
		"/path/with/special%20characters",
		"/路径/中文",
		"",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}

			SetInternalRedirect(ctx, tt)

			if !IsInternalRedirect(ctx) {
				t.Error("expected IsInternalRedirect to return true")
			}

			got := GetInternalRedirectPath(ctx)
			if got != tt {
				t.Errorf("expected %q, got %q", tt, got)
			}
		})
	}
}
