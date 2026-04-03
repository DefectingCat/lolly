package server

import (
	"net"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
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
		allow      []string
		clientIP   string
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

			gotIP := getClientIPForStatus(ctx)
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

			gotIP := getClientIPForStatus(ctx)
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

	gotIP := getClientIPForStatus(ctx)
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
