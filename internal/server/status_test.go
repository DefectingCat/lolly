// Package server 提供状态处理器功能的测试。
//
// 该文件测试状态处理器模块的各项功能，包括：
//   - 状态处理器创建
//   - CIDR 和单 IP 白名单配置
//   - 无效 IP 格式处理
//   - 访问控制逻辑
//   - 客户端 IP 提取
//   - 状态信息收集
//   - Goroutine 池统计
//   - 文件缓存统计
//
// 作者：xfy
package server

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/netutil"
)

func TestNewStatusHandler_CIDR(t *testing.T) {
	tests := []struct {
		name    string
		allow   []string
		wantErr bool
	}{
		{
			name:    "valid CIDR IPv4",
			allow:   []string{"192.168.1.0/24"},
			wantErr: false,
		},
		{
			name:    "valid CIDR IPv6",
			allow:   []string{"2001:db8::/32"},
			wantErr: false,
		},
		{
			name:    "multiple CIDRs",
			allow:   []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			wantErr: false,
		},
		{
			name:    "empty allow list",
			allow:   []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.StatusConfig{
				Path:  "/_status",
				Allow: tt.allow,
			}

			h, err := NewStatusHandler(nil, cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if h == nil {
					t.Fatal("expected non-nil handler")
				}
			}
		})
	}
}

func TestNewStatusHandler_SingleIP(t *testing.T) {
	tests := []struct {
		name    string
		allow   []string
		wantErr bool
	}{
		{
			name:    "single IPv4",
			allow:   []string{"192.168.1.100"},
			wantErr: false,
		},
		{
			name:    "single IPv6",
			allow:   []string{"2001:db8::1"},
			wantErr: false,
		},
		{
			name:    "mixed CIDR and single IP",
			allow:   []string{"10.0.0.0/8", "192.168.1.100"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.StatusConfig{
				Path:  "/_status",
				Allow: tt.allow,
			}

			h, err := NewStatusHandler(nil, cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if h == nil {
					t.Fatal("expected non-nil handler")
				}
				if len(h.allowed) != len(tt.allow) {
					t.Errorf("expected %d allowed networks, got %d", len(tt.allow), len(h.allowed))
				}
			}
		})
	}
}

func TestNewStatusHandler_InvalidIP(t *testing.T) {
	tests := []struct {
		name  string
		allow []string
	}{
		{
			name:  "invalid CIDR",
			allow: []string{"invalid-cidr"},
		},
		{
			name:  "invalid IP format",
			allow: []string{"not-an-ip"},
		},
		{
			name:  "CIDR with invalid mask",
			allow: []string{"192.168.1.0/33"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.StatusConfig{
				Path:  "/_status",
				Allow: tt.allow,
			}

			_, err := NewStatusHandler(nil, cfg)
			if err == nil {
				t.Error("expected error for invalid IP/CIDR, got nil")
			}
		})
	}
}

func TestStatusHandler_Path(t *testing.T) {
	tests := []struct {
		name     string
		cfgPath  string
		wantPath string
	}{
		{
			name:     "default path",
			cfgPath:  "",
			wantPath: "/_status",
		},
		{
			name:     "custom path",
			cfgPath:  "/health",
			wantPath: "/health",
		},
		{
			name:     "custom path with prefix",
			cfgPath:  "/api/v1/status",
			wantPath: "/api/v1/status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.StatusConfig{
				Path:  tt.cfgPath,
				Allow: []string{},
			}

			h, err := NewStatusHandler(nil, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if h.Path() != tt.wantPath {
				t.Errorf("expected path %s, got %s", tt.wantPath, h.Path())
			}
		})
	}
}

func TestStatusHandler_checkAccess(t *testing.T) {
	tests := []struct {
		name       string
		clientIP   string
		allow      []string
		wantAccess bool
	}{
		{
			name:       "no allow list - open access",
			allow:      []string{},
			clientIP:   "1.2.3.4",
			wantAccess: true,
		},
		{
			name:       "CIDR match",
			allow:      []string{"192.168.0.0/16"},
			clientIP:   "192.168.1.100",
			wantAccess: true,
		},
		{
			name:       "CIDR no match",
			allow:      []string{"10.0.0.0/8"},
			clientIP:   "192.168.1.100",
			wantAccess: false,
		},
		{
			name:       "single IP match",
			allow:      []string{"127.0.0.1"},
			clientIP:   "127.0.0.1",
			wantAccess: true,
		},
		{
			name:       "single IP no match",
			allow:      []string{"127.0.0.1"},
			clientIP:   "127.0.0.2",
			wantAccess: false,
		},
		{
			name:       "IPv6 match",
			allow:      []string{"2001:db8::/32"},
			clientIP:   "2001:db8::1",
			wantAccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.StatusConfig{
				Path:  "/_status",
				Allow: tt.allow,
			}

			h, err := NewStatusHandler(nil, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// 直接测试 checkAccess 内部逻辑
			// 由于无法轻松设置 RemoteAddr，我们直接测试 IP 是否在 allowed 列表中
			if len(h.allowed) > 0 {
				ip := net.ParseIP(tt.clientIP)
				if ip == nil {
					t.Fatalf("failed to parse client IP: %s", tt.clientIP)
				}

				found := false
				for _, network := range h.allowed {
					if network.Contains(ip) {
						found = true
						break
					}
				}

				if found != tt.wantAccess {
					t.Errorf("expected access %v, got %v", tt.wantAccess, found)
				}
			} else {
				// 无白名单时应允许所有访问
				if !tt.wantAccess {
					t.Error("expected access to be true when no allow list configured")
				}
			}
		})
	}
}

func TestStatusHandler_ServeHTTP_NoAllowList(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:  "/_status",
		Allow: []string{},
	}

	// 创建带有有效 server 的 handler
	srv := New(nil)
	srv.startTime = time.Now()

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")

	h.ServeHTTP(ctx)

	// 无白名单时应允许所有访问
	if ctx.Response.StatusCode() != 200 {
		t.Errorf("expected status 200 (open access), got %d", ctx.Response.StatusCode())
	}
}

func TestGetClientIPForStatus_XForwardedFor(t *testing.T) {
	tests := []struct {
		name   string
		xff    string
		wantIP string
	}{
		{
			name:   "single IP",
			xff:    "10.0.0.1",
			wantIP: "10.0.0.1",
		},
		{
			name:   "multiple IPs - first is client",
			xff:    "10.0.0.1, 192.168.1.1, 172.16.0.1",
			wantIP: "10.0.0.1",
		},
		{
			name:   "IPv6 address",
			xff:    "2001:db8::1",
			wantIP: "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.Set("X-Forwarded-For", tt.xff)

			gotIP := netutil.ExtractClientIPNet(ctx)
			if gotIP == nil {
				t.Errorf("expected IP %s, got nil", tt.wantIP)
			} else if gotIP.String() != tt.wantIP {
				t.Errorf("expected IP %s, got %s", tt.wantIP, gotIP.String())
			}
		})
	}
}

// TestGetClientIPForStatus_InvalidIPs 测试无效 IP 场景
// 注意：当头部解析失败时，函数会回退到 RemoteAddr
// 在没有初始化连接的情况下，行为取决于 fasthttp 的默认值

func TestGetClientIPForStatus_XRealIP(t *testing.T) {
	tests := []struct {
		name   string
		xri    string
		wantIP string
	}{
		{
			name:   "valid IPv4",
			xri:    "10.0.0.2",
			wantIP: "10.0.0.2",
		},
		{
			name:   "valid IPv6",
			xri:    "2001:db8::2",
			wantIP: "2001:db8::2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.Set("X-Real-IP", tt.xri)

			gotIP := netutil.ExtractClientIPNet(ctx)
			if gotIP == nil {
				t.Errorf("expected IP %s, got nil", tt.wantIP)
			} else if gotIP.String() != tt.wantIP {
				t.Errorf("expected IP %s, got %s", tt.wantIP, gotIP.String())
			}
		})
	}
}

func TestGetClientIPForStatus_Priority(t *testing.T) {
	// X-Forwarded-For 优先级高于 X-Real-IP
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("X-Forwarded-For", "10.0.0.1")
	ctx.Request.Header.Set("X-Real-IP", "10.0.0.2")

	gotIP := netutil.ExtractClientIPNet(ctx)
	if gotIP == nil {
		t.Error("expected IP, got nil")
	} else if gotIP.String() != "10.0.0.1" {
		t.Errorf("expected X-Forwarded-For IP 10.0.0.1, got %s", gotIP.String())
	}
}

func TestCollectStatus(t *testing.T) {
	// 创建服务器实例用于测试
	srv := New(nil)
	srv.startTime = time.Now()
	srv.connections.Store(5)
	srv.requests.Store(100)
	srv.bytesSent.Store(1024 * 1024)
	srv.bytesReceived.Store(512 * 1024)

	h := &StatusHandler{
		server: srv,
		path:   "/_status",
	}

	status := h.collectStatus()

	if status == nil {
		t.Error("expected non-nil status")
		return
	}

	// 验证基本字段
	if status.Connections != 5 {
		t.Errorf("expected Connections 5, got %d", status.Connections)
	}
	if status.Requests != 100 {
		t.Errorf("expected Requests 100, got %d", status.Requests)
	}
	if status.BytesSent != 1024*1024 {
		t.Errorf("expected BytesSent %d, got %d", 1024*1024, status.BytesSent)
	}
	if status.BytesReceived != 512*1024 {
		t.Errorf("expected BytesReceived %d, got %d", 512*1024, status.BytesReceived)
	}

	// 验证运行时间合理
	if status.Uptime < 0 {
		t.Errorf("expected positive Uptime, got %v", status.Uptime)
	}
}

func TestCollectStatus_WithPool(t *testing.T) {
	srv := New(nil)
	srv.startTime = time.Now()
	srv.pool = NewGoroutinePool(PoolConfig{
		MinWorkers:  2,
		MaxWorkers:  10,
		QueueSize:   100,
		IdleTimeout: 30 * time.Second,
	})
	srv.pool.Start()

	h := &StatusHandler{
		server: srv,
		path:   "/_status",
	}

	status := h.collectStatus()

	if status.Pool == nil {
		t.Error("expected Pool stats to be populated")
	} else {
		if status.Pool.MinWorkers != 2 {
			t.Errorf("expected MinWorkers 2, got %d", status.Pool.MinWorkers)
		}
		if status.Pool.MaxWorkers != 10 {
			t.Errorf("expected MaxWorkers 10, got %d", status.Pool.MaxWorkers)
		}
	}

	srv.pool.Stop()
}

func TestCollectStatus_WithFileCache(t *testing.T) {
	srv := New(nil)
	srv.startTime = time.Now()

	// 创建文件缓存需要有效配置，这里跳过复杂的缓存测试
	// 仅测试 nil cache 情况
	h := &StatusHandler{
		server: srv,
		path:   "/_status",
	}

	status := h.collectStatus()

	// 无缓存时 Cache 应为 nil
	if status.Cache != nil {
		t.Error("expected Cache to be nil when no fileCache configured")
	}
}

// ---------------------------------------------------------------------------
// Format output tests
// ---------------------------------------------------------------------------

func TestStatusHandler_ServeHTTP_JSONFormat(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "json",
		Allow:  []string{},
	}

	srv := New(nil)
	srv.startTime = time.Now()
	srv.connections.Store(10)
	srv.requests.Store(500)
	srv.bytesSent.Store(2048)
	srv.bytesReceived.Store(1024)

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")

	h.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Errorf("expected status 200, got %d", ctx.Response.StatusCode())
	}

	ct := string(ctx.Response.Header.ContentType())
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected content-type application/json, got %s", ct)
	}

	// 验证 JSON 可解析
	var status Status
	if err := json.Unmarshal(ctx.Response.Body(), &status); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if status.Connections != 10 {
		t.Errorf("expected Connections 10, got %d", status.Connections)
	}
	if status.Requests != 500 {
		t.Errorf("expected Requests 500, got %d", status.Requests)
	}
	if status.BytesSent != 2048 {
		t.Errorf("expected BytesSent 2048, got %d", status.BytesSent)
	}
	if status.BytesReceived != 1024 {
		t.Errorf("expected BytesReceived 1024, got %d", status.BytesReceived)
	}
}

func TestStatusHandler_ServeHTTP_HTMLFormat(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "html",
		Allow:  []string{},
	}

	srv := New(nil)
	srv.startTime = time.Now()

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")

	h.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Errorf("expected status 200, got %d", ctx.Response.StatusCode())
	}

	ct := string(ctx.Response.Header.ContentType())
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected content-type text/html, got %s", ct)
	}

	body := string(ctx.Response.Body())
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("expected HTML output to start with DOCTYPE")
	}
	if !strings.Contains(body, "<title>Lolly Status</title>") {
		t.Error("expected HTML output to contain title")
	}
	if !strings.Contains(body, "<h1>Lolly Status</h1>") {
		t.Error("expected HTML output to contain h1 heading")
	}
	if !strings.Contains(body, "<table>") {
		t.Error("expected HTML output to contain table")
	}
	if !strings.Contains(body, "<th>Version</th>") {
		t.Error("expected HTML output to contain Version row")
	}
}

func TestStatusHandler_ServeHTTP_TextFormat(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "text",
		Allow:  []string{},
	}

	srv := New(nil)
	srv.startTime = time.Now()

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")

	h.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Errorf("expected status 200, got %d", ctx.Response.StatusCode())
	}

	ct := string(ctx.Response.Header.ContentType())
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected content-type text/plain, got %s", ct)
	}

	body := string(ctx.Response.Body())
	if !strings.Contains(body, "Lolly Status") {
		t.Error("expected text output to contain 'Lolly Status'")
	}
	if !strings.Contains(body, "Version:") {
		t.Error("expected text output to contain Version")
	}
	if !strings.Contains(body, "Uptime:") {
		t.Error("expected text output to contain Uptime")
	}
	if !strings.Contains(body, "Connections:") {
		t.Error("expected text output to contain Connections")
	}
	if !strings.Contains(body, "Requests:") {
		t.Error("expected text output to contain Requests")
	}
	if !strings.Contains(body, "Bytes Sent:") {
		t.Error("expected text output to contain Bytes Sent")
	}
	if !strings.Contains(body, "Bytes Received:") {
		t.Error("expected text output to contain Bytes Received")
	}
}

func TestStatusHandler_ServeHTTP_PrometheusFormat(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "prometheus",
		Allow:  []string{},
	}

	srv := New(nil)
	srv.startTime = time.Now()
	srv.connections.Store(10)
	srv.requests.Store(500)
	srv.bytesSent.Store(2048)
	srv.bytesReceived.Store(1024)

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")

	h.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Errorf("expected status 200, got %d", ctx.Response.StatusCode())
	}

	ct := string(ctx.Response.Header.ContentType())
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected content-type text/plain, got %s", ct)
	}

	body := string(ctx.Response.Body())
	if !strings.Contains(body, "# HELP lolly_version") {
		t.Error("expected prometheus output to contain lolly_version HELP")
	}
	if !strings.Contains(body, "# TYPE lolly_version gauge") {
		t.Error("expected prometheus output to contain lolly_version TYPE")
	}
	if !strings.Contains(body, "lolly_uptime_seconds") {
		t.Error("expected prometheus output to contain lolly_uptime_seconds")
	}
	if !strings.Contains(body, "lolly_connections 10") {
		t.Error("expected prometheus output to contain lolly_connections 10")
	}
	if !strings.Contains(body, "lolly_requests_total 500") {
		t.Error("expected prometheus output to contain lolly_requests_total 500")
	}
	if !strings.Contains(body, "lolly_bytes_sent_total 2048") {
		t.Error("expected prometheus output to contain lolly_bytes_sent_total 2048")
	}
	if !strings.Contains(body, "lolly_bytes_received_total 1024") {
		t.Error("expected prometheus output to contain lolly_bytes_received_total 1024")
	}
}

func TestStatusHandler_ServeHTTP_DefaultFormat(t *testing.T) {
	// Format 为空时应回退到 JSON
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "",
		Allow:  []string{},
	}

	srv := New(nil)
	srv.startTime = time.Now()

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证 format 默认为 json
	if h.format != "json" {
		t.Errorf("expected default format 'json', got '%s'", h.format)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")

	h.ServeHTTP(ctx)

	ct := string(ctx.Response.Header.ContentType())
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected content-type application/json for default format, got %s", ct)
	}
}

// ---------------------------------------------------------------------------
// Access control tests via ServeHTTP
// ---------------------------------------------------------------------------

func TestStatusHandler_ServeHTTP_AccessDenied(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "json",
		Allow:  []string{"10.0.0.0/8"},
	}

	srv := New(nil)
	srv.startTime = time.Now()

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")
	// RemoteAddr 默认为 127.0.0.1，不在 10.0.0.0/8 范围内
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345})

	h.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != 403 {
		t.Errorf("expected status 403 for denied access, got %d", ctx.Response.StatusCode())
	}
}

func TestStatusHandler_ServeHTTP_AccessAllowed_ByCIDR(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "json",
		Allow:  []string{"192.168.0.0/16"},
	}

	srv := New(nil)
	srv.startTime = time.Now()

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("192.168.1.100"), Port: 12345})

	h.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Errorf("expected status 200 for allowed access, got %d", ctx.Response.StatusCode())
	}
}

func TestStatusHandler_New_localhost(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:  "/_status",
		Allow: []string{"localhost"},
	}

	h, err := NewStatusHandler(nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// localhost 应解析为两个网络：127.0.0.1/32 和 ::1/128
	if len(h.allowed) != 2 {
		t.Errorf("expected 2 allowed networks for localhost, got %d", len(h.allowed))
	}

	// 验证包含 127.0.0.1
	ip := net.ParseIP("127.0.0.1")
	found := false
	for _, n := range h.allowed {
		if n.Contains(ip) {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected localhost to include 127.0.0.1")
	}

	// 验证包含 ::1
	ipv6 := net.ParseIP("::1")
	found = false
	for _, n := range h.allowed {
		if n.Contains(ipv6) {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected localhost to include ::1")
	}
}

func TestStatusHandler_checkAccess_AllowList(t *testing.T) {
	tests := []struct {
		name       string
		allow      []string
		remoteIP   net.IP
		wantAccess bool
	}{
		{
			name:       "no allow list allows all",
			allow:      []string{},
			remoteIP:   net.ParseIP("1.2.3.4"),
			wantAccess: true,
		},
		{
			name:       "CIDR match",
			allow:      []string{"192.168.0.0/16"},
			remoteIP:   net.ParseIP("192.168.5.10"),
			wantAccess: true,
		},
		{
			name:       "CIDR no match",
			allow:      []string{"10.0.0.0/8"},
			remoteIP:   net.ParseIP("192.168.1.1"),
			wantAccess: false,
		},
		{
			name:       "single IP match",
			allow:      []string{"127.0.0.1"},
			remoteIP:   net.ParseIP("127.0.0.1"),
			wantAccess: true,
		},
		{
			name:       "IPv6 match",
			allow:      []string{"::1/128"},
			remoteIP:   net.ParseIP("::1"),
			wantAccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.StatusConfig{
				Path:  "/_status",
				Allow: tt.allow,
			}
			h, err := NewStatusHandler(nil, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			ctx := &fasthttp.RequestCtx{}
			ctx.SetRemoteAddr(&net.TCPAddr{IP: tt.remoteIP, Port: 12345})

			got := h.checkAccess(ctx)
			if got != tt.wantAccess {
				t.Errorf("expected access %v, got %v", tt.wantAccess, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// collectStatus with full data
// ---------------------------------------------------------------------------

func TestCollectStatus_WithFullData(t *testing.T) {
	srv := New(nil)
	srv.startTime = time.Now()
	srv.connections.Store(42)
	srv.requests.Store(999)
	srv.bytesSent.Store(50000)
	srv.bytesReceived.Store(25000)

	// 设置文件缓存
	fc := cache.NewFileCache(100, 1024*1024, 5*time.Minute)
	srv.fileCache = fc

	// 设置 pool
	srv.pool = NewGoroutinePool(PoolConfig{
		MinWorkers:  2,
		MaxWorkers:  10,
		QueueSize:   50,
		IdleTimeout: 30 * time.Second,
	})
	srv.pool.Start()
	defer srv.pool.Stop()

	h := &StatusHandler{
		server: srv,
		path:   "/_status",
	}

	status := h.collectStatus()

	if status == nil {
		t.Fatal("expected non-nil status")
	}

	// 基本字段
	if status.Connections != 42 {
		t.Errorf("expected Connections 42, got %d", status.Connections)
	}
	if status.Requests != 999 {
		t.Errorf("expected Requests 999, got %d", status.Requests)
	}
	if status.BytesSent != 50000 {
		t.Errorf("expected BytesSent 50000, got %d", status.BytesSent)
	}
	if status.BytesReceived != 25000 {
		t.Errorf("expected BytesReceived 25000, got %d", status.BytesReceived)
	}

	// Cache 应有数据
	if status.Cache == nil {
		t.Fatal("expected non-nil Cache")
	}
	if status.Cache.FileCache.MaxEntries != 100 {
		t.Errorf("expected FileCache MaxEntries 100, got %d", status.Cache.FileCache.MaxEntries)
	}
	if status.Cache.FileCache.MaxSize != 1024*1024 {
		t.Errorf("expected FileCache MaxSize %d, got %d", 1024*1024, status.Cache.FileCache.MaxSize)
	}

	// Pool 应有数据
	if status.Pool == nil {
		t.Error("expected non-nil Pool")
	} else {
		if status.Pool.MinWorkers != 2 {
			t.Errorf("expected MinWorkers 2, got %d", status.Pool.MinWorkers)
		}
		if status.Pool.MaxWorkers != 10 {
			t.Errorf("expected MaxWorkers 10, got %d", status.Pool.MaxWorkers)
		}
	}
}

func TestCollectStatus_NilServerFields(t *testing.T) {
	srv := New(nil)
	srv.startTime = time.Now()
	// fileCache 和 pool 都为 nil

	h := &StatusHandler{
		server: srv,
		path:   "/_status",
	}

	status := h.collectStatus()

	if status.Cache != nil {
		t.Error("expected Cache to be nil when fileCache is nil")
	}
	if status.Pool != nil {
		t.Error("expected Pool to be nil when pool is nil")
	}
}

// ---------------------------------------------------------------------------
// HTML format with all sections
// ---------------------------------------------------------------------------

func TestStatusHandler_ServeHTTP_HTMLWithAllSections(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "html",
		Allow:  []string{},
	}

	srv := New(nil)
	srv.startTime = time.Now()
	srv.connections.Store(5)
	srv.requests.Store(100)
	srv.bytesSent.Store(1024)
	srv.bytesReceived.Store(512)

	// 设置文件缓存
	fc := cache.NewFileCache(100, 1024*1024, 5*time.Minute)
	srv.fileCache = fc

	// 设置 pool
	srv.pool = NewGoroutinePool(PoolConfig{
		MinWorkers:  2,
		MaxWorkers:  10,
		QueueSize:   50,
		IdleTimeout: 30 * time.Second,
	})
	srv.pool.Start()
	defer srv.pool.Stop()

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")
	h.ServeHTTP(ctx)

	body := string(ctx.Response.Body())

	// 验证所有 section 标题
	if !strings.Contains(body, "<h2>Cache</h2>") {
		t.Error("expected HTML to contain Cache section")
	}
	if !strings.Contains(body, "<h2>Goroutine Pool</h2>") {
		t.Error("expected HTML to contain Goroutine Pool section")
	}

	// 验证 pool 数据
	if !strings.Contains(body, "Queue</th>") {
		t.Error("expected HTML to contain Pool Queue column")
	}
}

// ---------------------------------------------------------------------------
// Text format with all sections
// ---------------------------------------------------------------------------

func TestStatusHandler_ServeHTTP_TextWithPool(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "text",
		Allow:  []string{},
	}

	srv := New(nil)
	srv.startTime = time.Now()
	srv.pool = NewGoroutinePool(PoolConfig{
		MinWorkers:  2,
		MaxWorkers:  10,
		QueueSize:   50,
		IdleTimeout: 30 * time.Second,
	})
	srv.pool.Start()
	defer srv.pool.Stop()

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")
	h.ServeHTTP(ctx)

	body := string(ctx.Response.Body())
	if !strings.Contains(body, "Goroutine Pool:") {
		t.Error("expected text output to contain Goroutine Pool section")
	}
	if !strings.Contains(body, "Workers:") {
		t.Error("expected text output to contain Workers")
	}
	if !strings.Contains(body, "Queue:") {
		t.Error("expected text output to contain Queue")
	}
}

// ---------------------------------------------------------------------------
// Prometheus format with cache metrics
// ---------------------------------------------------------------------------

func TestStatusHandler_ServeHTTP_PrometheusWithCache(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "prometheus",
		Allow:  []string{},
	}

	srv := New(nil)
	srv.startTime = time.Now()
	srv.connections.Store(5)
	srv.requests.Store(100)
	srv.bytesSent.Store(1024)
	srv.bytesReceived.Store(512)

	fc := cache.NewFileCache(100, 1024*1024, 5*time.Minute)
	srv.fileCache = fc

	srv.pool = NewGoroutinePool(PoolConfig{
		MinWorkers:  2,
		MaxWorkers:  10,
		QueueSize:   50,
		IdleTimeout: 30 * time.Second,
	})
	srv.pool.Start()
	defer srv.pool.Stop()

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")
	h.ServeHTTP(ctx)

	body := string(ctx.Response.Body())

	// 验证缓存指标
	if !strings.Contains(body, "lolly_cache_entries") {
		t.Error("expected prometheus output to contain lolly_cache_entries")
	}
	if !strings.Contains(body, "lolly_cache_size_bytes") {
		t.Error("expected prometheus output to contain lolly_cache_size_bytes")
	}
	if !strings.Contains(body, "lolly_pool_workers") {
		t.Error("expected prometheus output to contain lolly_pool_workers")
	}
	if !strings.Contains(body, "lolly_pool_queue_length") {
		t.Error("expected prometheus output to contain lolly_pool_queue_length")
	}
}

// ---------------------------------------------------------------------------
// Access via X-Forwarded-For header
// ---------------------------------------------------------------------------

func TestStatusHandler_ServeHTTP_AccessDenied_XForwardedFor(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "json",
		Allow:  []string{"10.0.0.0/8"},
	}

	srv := New(nil)
	srv.startTime = time.Now()

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")
	// 设置 X-Forwarded-For 为不在白名单中的 IP
	ctx.Request.Header.Set("X-Forwarded-For", "192.168.1.1")

	h.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != 403 {
		t.Errorf("expected status 403 for denied access via X-Forwarded-For, got %d", ctx.Response.StatusCode())
	}
}

func TestStatusHandler_ServeHTTP_AccessAllowed_XForwardedFor(t *testing.T) {
	cfg := &config.StatusConfig{
		Path:   "/_status",
		Format: "json",
		Allow:  []string{"10.0.0.0/8"},
	}

	srv := New(nil)
	srv.startTime = time.Now()

	h, err := NewStatusHandler(srv, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/_status")
	ctx.Request.Header.Set("X-Forwarded-For", "10.5.5.5")

	h.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != 200 {
		t.Errorf("expected status 200 for allowed access via X-Forwarded-For, got %d", ctx.Response.StatusCode())
	}
}
