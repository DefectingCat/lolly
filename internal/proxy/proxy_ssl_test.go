// Package proxy 反向代理包，为 Lolly HTTP 服务器提供反向代理功能。
package proxy

import (
	"crypto/tls"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
)

func TestCreateTLSConfig_NilConfig(t *testing.T) {
	tlsCfg, err := CreateTLSConfig(nil, "example.com")
	if err != nil {
		t.Errorf("CreateTLSConfig(nil) returned error: %v", err)
	}
	if tlsCfg != nil {
		t.Error("CreateTLSConfig(nil) should return nil")
	}
}

func TestCreateTLSConfig_Disabled(t *testing.T) {
	cfg := &config.ProxySSLConfig{Enabled: false}
	tlsCfg, err := CreateTLSConfig(cfg, "example.com")
	if err != nil {
		t.Errorf("CreateTLSConfig(disabled) returned error: %v", err)
	}
	if tlsCfg != nil {
		t.Error("CreateTLSConfig(disabled) should return nil")
	}
}

func TestCreateTLSConfig_ServerName(t *testing.T) {
	tests := []struct {
		name             string
		cfg              *config.ProxySSLConfig
		defaultServerName string
		wantServerName   string
	}{
		{
			name:             "custom server name",
			cfg:              &config.ProxySSLConfig{Enabled: true, ServerName: "custom.example.com"},
			defaultServerName: "default.example.com",
			wantServerName:   "custom.example.com",
		},
		{
			name:             "default server name",
			cfg:              &config.ProxySSLConfig{Enabled: true},
			defaultServerName: "default.example.com",
			wantServerName:   "default.example.com",
		},
		{
			name:             "empty default",
			cfg:              &config.ProxySSLConfig{Enabled: true},
			defaultServerName: "",
			wantServerName:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlsCfg, err := CreateTLSConfig(tt.cfg, tt.defaultServerName)
			if err != nil {
				t.Errorf("CreateTLSConfig returned error: %v", err)
				return
			}
			if tlsCfg == nil {
				t.Error("CreateTLSConfig returned nil")
				return
			}
			if tlsCfg.ServerName != tt.wantServerName {
				t.Errorf("ServerName = %q, want %q", tlsCfg.ServerName, tt.wantServerName)
			}
		})
	}
}

func TestCreateTLSConfig_InsecureSkipVerify(t *testing.T) {
	cfg := &config.ProxySSLConfig{
		Enabled:            true,
		InsecureSkipVerify: true,
	}
	tlsCfg, err := CreateTLSConfig(cfg, "example.com")
	if err != nil {
		t.Errorf("CreateTLSConfig returned error: %v", err)
		return
	}
	if tlsCfg == nil {
		t.Error("CreateTLSConfig returned nil")
		return
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestCreateTLSConfig_TLSVersions(t *testing.T) {
	tests := []struct {
		name        string
		minVersion  string
		maxVersion  string
		wantMin     uint16
		wantMax     uint16
	}{
		{
			name:       "TLSV1.2 min",
			minVersion: "TLSV1.2",
			wantMin:    tls.VersionTLS12,
		},
		{
			name:       "TLSV1.3 min",
			minVersion: "TLSV1.3",
			wantMin:    tls.VersionTLS13,
		},
		{
			name:       "TLSV1.2 max",
			maxVersion: "TLSV1.2",
			wantMax:    tls.VersionTLS12,
		},
		{
			name:        "both versions",
			minVersion:  "TLSV1.2",
			maxVersion:  "TLSV1.3",
			wantMin:     tls.VersionTLS12,
			wantMax:     tls.VersionTLS13,
		},
		{
			name:       "mixed case TLSv1.2",
			minVersion: "TLSv1.2", // 测试大小写不敏感
			wantMin:    tls.VersionTLS12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ProxySSLConfig{
				Enabled:     true,
				MinVersion:  tt.minVersion,
				MaxVersion:  tt.maxVersion,
			}
			tlsCfg, err := CreateTLSConfig(cfg, "example.com")
			if err != nil {
				t.Errorf("CreateTLSConfig returned error: %v", err)
				return
			}
			if tlsCfg == nil {
				t.Error("CreateTLSConfig returned nil")
				return
			}
			if tt.wantMin != 0 && tlsCfg.MinVersion != tt.wantMin {
				t.Errorf("MinVersion = %d, want %d", tlsCfg.MinVersion, tt.wantMin)
			}
			if tt.wantMax != 0 && tlsCfg.MaxVersion != tt.wantMax {
				t.Errorf("MaxVersion = %d, want %d", tlsCfg.MaxVersion, tt.wantMax)
			}
		})
	}
}

func TestCreateTLSConfig_InvalidTLSVersion(t *testing.T) {
	cfg := &config.ProxySSLConfig{
		Enabled:    true,
		MinVersion: "TLSv9.9",
	}
	_, err := CreateTLSConfig(cfg, "example.com")
	if err == nil {
		t.Error("CreateTLSConfig should return error for invalid TLS version")
	}
}

func TestCreateTLSConfig_TrustedCA(t *testing.T) {
	// 跳过这个测试，因为需要有效的 CA 证书文件
	// 实际集成测试会使用真实证书
	t.Skip("需要有效的 CA 证书文件，在集成测试中验证")
}

func TestCreateTLSConfig_TrustedCANotFound(t *testing.T) {
	cfg := &config.ProxySSLConfig{
		Enabled:   true,
		TrustedCA: "/nonexistent/ca.crt",
	}
	_, err := CreateTLSConfig(cfg, "example.com")
	if err == nil {
		t.Error("CreateTLSConfig should return error for nonexistent CA file")
	}
}

func TestCreateTLSConfig_ClientCert(t *testing.T) {
	// 跳过这个测试，因为需要有效的证书文件
	t.Skip("需要有效的客户端证书文件，在集成测试中验证")
}

func TestCreateTLSConfig_ClientCertNotFound(t *testing.T) {
	cfg := &config.ProxySSLConfig{
		Enabled:    true,
		ClientCert: "/nonexistent/client.crt",
		ClientKey:  "/nonexistent/client.key",
	}
	_, err := CreateTLSConfig(cfg, "example.com")
	if err == nil {
		t.Error("CreateTLSConfig should return error for nonexistent cert files")
	}
}

// 缓存分段有效期测试（US-007）
func TestGetCacheDuration_NoCacheValid(t *testing.T) {
	cfg := &config.ProxyConfig{
		Cache: config.ProxyCacheConfig{
			MaxAge: 5 * time.Minute,
		},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	// 无 CacheValid 配置时，所有状态码应使用 MaxAge
	tests := []struct {
		statusCode int
		want       time.Duration
	}{
		{200, 5 * time.Minute},
		{301, 5 * time.Minute},
		{404, 5 * time.Minute},
		{500, 5 * time.Minute},
	}

	for _, tt := range tests {
		got := p.getCacheDuration(tt.statusCode)
		if got != tt.want {
			t.Errorf("getCacheDuration(%d) = %v, want %v", tt.statusCode, got, tt.want)
		}
	}
}

func TestGetCacheDuration_CacheValidOKInheritsMaxAge(t *testing.T) {
	cfg := &config.ProxyConfig{
		Cache: config.ProxyCacheConfig{
			MaxAge: 10 * time.Minute,
		},
		CacheValid: &config.ProxyCacheValidConfig{
			OK: 0, // 0 表示继承 MaxAge
		},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	got := p.getCacheDuration(200)
	want := 10 * time.Minute
	if got != want {
		t.Errorf("getCacheDuration(200) with OK=0 = %v, want %v (MaxAge)", got, want)
	}
}

func TestGetCacheDuration_StatusCodeMapping(t *testing.T) {
	cfg := &config.ProxyConfig{
		Cache: config.ProxyCacheConfig{
			MaxAge: 1 * time.Minute,
		},
		CacheValid: &config.ProxyCacheValidConfig{
			OK:           10 * time.Minute,
			Redirect:     1 * time.Hour,
			NotFound:     1 * time.Minute,
			ClientError:  30 * time.Second,
			ServerError:  0, // 不缓存
		},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	tests := []struct {
		name       string
		statusCode int
		want       time.Duration
	}{
		{"200 OK", 200, 10 * time.Minute},
		{"201 Created", 201, 10 * time.Minute},
		{"299 OK boundary", 299, 10 * time.Minute},
		{"301 Moved", 301, 1 * time.Hour},
		{"302 Found", 302, 1 * time.Hour},
		{"304 Not Modified", 304, 0}, // 不在 Redirect 范围内
		{"404 Not Found", 404, 1 * time.Minute},
		{"400 Bad Request", 400, 30 * time.Second},
		{"403 Forbidden", 403, 30 * time.Second},
		{"500 Internal Error", 500, 0},
		{"503 Service Unavailable", 503, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.getCacheDuration(tt.statusCode)
			if got != tt.want {
				t.Errorf("getCacheDuration(%d) = %v, want %v", tt.statusCode, got, tt.want)
			}
		})
	}
}

func TestGetCacheDuration_ZeroValuesNoCache(t *testing.T) {
	cfg := &config.ProxyConfig{
		Cache: config.ProxyCacheConfig{
			MaxAge: 5 * time.Minute,
		},
		CacheValid: &config.ProxyCacheValidConfig{
			OK:           10 * time.Minute, // OK 有值
			Redirect:     0,                // 不缓存
			NotFound:     0,                // 不缓存
			ClientError:  0,                // 不缓存
			ServerError:  0,                // 不缓存
		},
	}
	targets := []*loadbalance.Target{{URL: "http://localhost:8080"}}
	p, err := NewProxy(cfg, targets, nil, nil)
	if err != nil {
		t.Fatalf("NewProxy() error: %v", err)
	}

	tests := []struct {
		statusCode int
		want       time.Duration
	}{
		{200, 10 * time.Minute}, // OK 有值
		{301, 0},                // Redirect=0 不缓存
		{404, 0},                // NotFound=0 不缓存
		{500, 0},                // ServerError=0 不缓存
	}

	for _, tt := range tests {
		got := p.getCacheDuration(tt.statusCode)
		if got != tt.want {
			t.Errorf("getCacheDuration(%d) = %v, want %v", tt.statusCode, got, tt.want)
		}
	}
}