// Package security 提供访问控制覆盖测试。
//
// 该文件补充测试 access.go 中未覆盖的方法：
//   - Name() 方法
//   - Process() 完整处理链（允许/拒绝路径）
//   - getClientIP() 通过 Process 间接测试
//   - Close() 方法
//   - actionToString 边缘情况
//   - trustedProxies 相关逻辑
//
// 作者：xfy
package security

import (
	"net"
	"testing"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// TestAccessControlName 测试 Name 方法
func TestAccessControlName(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Default: "allow",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	name := ac.Name()
	if name != "access_control" {
		t.Errorf("Name() = %q, want 'access_control'", name)
	}
}

// TestAccessControlProcess_AllowPath 测试 Process 允许路径
func TestAccessControlProcess_AllowPath(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Default: "allow",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
		_, _ = ctx.WriteString("allowed")
	}

	handler := ac.Process(nextHandler)
	if handler == nil {
		t.Fatal("Process() returned nil")
	}

	ctx := &fasthttp.RequestCtx{}
	handler(ctx)

	if !called {
		t.Error("Process() should call next handler when access allowed")
	}
	if string(ctx.Response.Body()) != "allowed" {
		t.Errorf("Process() body = %q, want 'allowed'", string(ctx.Response.Body()))
	}
}

// TestAccessControlProcess_DenyPath 测试 Process 拒绝路径
func TestAccessControlProcess_DenyPath(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Default: "deny",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
	}

	handler := ac.Process(nextHandler)
	ctx := &fasthttp.RequestCtx{}
	handler(ctx)

	if called {
		t.Error("Process() should NOT call next handler when access denied")
	}
	if ctx.Response.StatusCode() != fasthttp.StatusForbidden {
		t.Errorf("Process() status = %d, want 403", ctx.Response.StatusCode())
	}
}

// TestAccessControlProcess_ExplicitAllow 测试显式允许列表
func TestAccessControlProcess_ExplicitAllow(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Allow:   []string{"127.0.0.1"},
		Default: "deny",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
	}

	handler := ac.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345})
	handler(ctx)

	if !called {
		t.Error("Process() should call next handler for allowed IP")
	}
}

// TestAccessControlProcess_ExplicitDeny 测试显式拒绝列表
func TestAccessControlProcess_ExplicitDeny(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Deny:    []string{"10.0.0.1"},
		Default: "allow",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
	}

	handler := ac.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 12345})
	handler(ctx)

	if called {
		t.Error("Process() should NOT call next handler for denied IP")
	}
}

// TestAccessControlProcess_TrustedProxies_XFF 测试可信代理 XFF 解析
func TestAccessControlProcess_TrustedProxies_XFF(t *testing.T) {
	// 配置可信代理，10.0.0.0/8 为可信代理段
	ac, err := NewAccessControl(&config.AccessConfig{
		TrustedProxies: []string{"10.0.0.0/8"},
		Default:        "deny",
		Allow:          []string{"192.168.1.0/24"},
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
	}

	handler := ac.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	// 请求来自可信代理 10.0.0.1，XFF 中包含真实客户端 192.168.1.100
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 12345})
	ctx.Request.Header.Set("X-Forwarded-For", "192.168.1.100, 10.0.0.1")
	handler(ctx)

	if !called {
		t.Error("Process() should allow real client IP behind trusted proxy")
	}
}

// TestAccessControlProcess_TrustedProxies_UntrustedSource 测试不可信来源不解析 XFF
func TestAccessControlProcess_TrustedProxies_UntrustedSource(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		TrustedProxies: []string{"10.0.0.0/8"},
		Default:        "deny",
		Allow:          []string{"192.168.1.0/24"},
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
	}

	handler := ac.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	// 请求来自不可信地址，即使 XFF 包含允许列表 IP 也不应解析
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("203.0.113.1"), Port: 12345})
	ctx.Request.Header.Set("X-Forwarded-For", "192.168.1.100")
	handler(ctx)

	if called {
		t.Error("Process() should not trust XFF from untrusted source")
	}
}

// TestAccessControlProcess_TrustedProxies_XRealIP 测试可信代理 X-Real-IP
func TestAccessControlProcess_TrustedProxies_XRealIP(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		TrustedProxies: []string{"10.0.0.0/8"},
		Default:        "deny",
		Allow:          []string{"192.168.1.0/24"},
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	called := false
	nextHandler := func(ctx *fasthttp.RequestCtx) {
		called = true
	}

	handler := ac.Process(nextHandler)

	ctx := &fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("10.0.0.50"), Port: 12345})
	ctx.Request.Header.Set("X-Real-IP", "192.168.1.50")
	handler(ctx)

	if !called {
		t.Error("Process() should use X-Real-IP from trusted proxy")
	}
}

// TestAccessControlClose 测试 Close 方法
func TestAccessControlClose(t *testing.T) {
	// 无 GeoIP 的 Close
	ac, err := NewAccessControl(&config.AccessConfig{
		Default: "allow",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	err = ac.Close()
	if err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

// TestActionToString 测试 actionToString 边缘情况
func TestActionToString(t *testing.T) {
	// 测试 ActionAllow
	result := actionToString(ActionAllow)
	if result != "allow" {
		t.Errorf("actionToString(ActionAllow) = %q, want 'allow'", result)
	}

	// 测试 ActionDeny
	result = actionToString(ActionDeny)
	if result != "deny" {
		t.Errorf("actionToString(ActionDeny) = %q, want 'deny'", result)
	}

	// 测试未知值
	result = actionToString(Action(999))
	if result != "unknown" {
		t.Errorf("actionToString(999) = %q, want 'unknown'", result)
	}
}

// TestGetStatsWithEmpty 测试 GetStats 空列表
func TestGetStatsWithEmpty(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Default: "allow",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	stats := ac.GetStats()
	if stats.AllowCount != 0 {
		t.Errorf("GetStats().AllowCount = %d, want 0", stats.AllowCount)
	}
	if stats.DenyCount != 0 {
		t.Errorf("GetStats().DenyCount = %d, want 0", stats.DenyCount)
	}
	if stats.Default != "allow" {
		t.Errorf("GetStats().Default = %q, want 'allow'", stats.Default)
	}
}

// TestSetDefaultValidCases 测试 SetDefault 所有有效值
func TestSetDefaultValidCases(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Default: "allow",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	// 切换为 deny
	err = ac.SetDefault("deny")
	if err != nil {
		t.Errorf("SetDefault('deny') error: %v", err)
	}
	stats := ac.GetStats()
	if stats.Default != "deny" {
		t.Errorf("After SetDefault('deny'), Default = %q, want 'deny'", stats.Default)
	}

	// 切换回 allow
	err = ac.SetDefault("allow")
	if err != nil {
		t.Errorf("SetDefault('allow') error: %v", err)
	}
	stats = ac.GetStats()
	if stats.Default != "allow" {
		t.Errorf("After SetDefault('allow'), Default = %q, want 'allow'", stats.Default)
	}

	// 大小写不敏感
	err = ac.SetDefault("DENY")
	if err != nil {
		t.Errorf("SetDefault('DENY') error: %v", err)
	}
}

// TestUpdateDenyListError 测试 UpdateDenyList 错误路径
func TestUpdateDenyListError(t *testing.T) {
	ac, err := NewAccessControl(&config.AccessConfig{
		Default: "allow",
	})
	if err != nil {
		t.Fatalf("NewAccessControl() error: %v", err)
	}

	err = ac.UpdateDenyList([]string{"not-an-ip"})
	if err == nil {
		t.Error("UpdateDenyList() should return error for invalid CIDR")
	}
}
