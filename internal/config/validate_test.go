// Package config 提供 YAML 配置文件的解析、验证和默认配置生成功能。
//
// 该文件测试配置验证模块的各项功能，包括：
//   - 服务器配置验证
//   - 代理配置验证
//   - SSL 配置验证
//   - 认证配置验证
//   - 速率限制验证
//   - 压缩配置验证
//   - 访问控制验证
//   - Stream 配置验证
//   - 性能配置验证
//
// 作者：xfy
package config

import (
	"strings"
	"testing"
)

func TestValidateServer(t *testing.T) {
	// TestValidateServer 测试服务器配置验证。
	tests := []struct {
		name      string
		config    ServerConfig
		isDefault bool
		wantErr   bool
		errMsg    string
	}{
		{
			name: "有效配置",
			config: ServerConfig{
				Listen: ":8080",
				Static: []StaticConfig{{Path: "/", Root: "/var/www"}},
				Proxy: []ProxyConfig{
					{Path: "/api", Targets: []ProxyTarget{{URL: "http://backend:8080"}}},
				},
			},
			isDefault: false,
			wantErr:   false,
		},
		{
			name: "默认服务器可省略Listen",
			config: ServerConfig{
				Static: []StaticConfig{{Path: "/", Root: "/var/www"}},
			},
			isDefault: true,
			wantErr:   false,
		},
		{
			name: "非默认服务器Listen缺失",
			config: ServerConfig{
				Static: []StaticConfig{{Path: "/", Root: "/var/www"}},
			},
			isDefault: false,
			wantErr:   true,
			errMsg:    "listen 地址必填",
		},
		{
			name: "无效Listen地址",
			config: ServerConfig{
				Listen: "invalid:address:format",
			},
			isDefault: false,
			wantErr:   true,
			errMsg:    "无效的监听地址",
		},
		{
			name: "静态根目录含..",
			config: ServerConfig{
				Listen: ":8080",
				Static: []StaticConfig{{Path: "/", Root: "/var/../www"}},
			},
			isDefault: false,
			wantErr:   true,
			errMsg:    "根目录路径不能包含 '..'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServer(&tt.config, tt.isDefault)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateServer() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateServer() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validateServer() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}

func TestValidateProxy(t *testing.T) {
	// TestValidateProxy 测试代理配置验证。
	tests := []struct {
		name    string
		config  ProxyConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "有效代理配置",
			config: ProxyConfig{
				Path:    "/api",
				Targets: []ProxyTarget{{URL: "http://backend:8080"}},
			},
			wantErr: false,
		},
		{
			name: "有效代理带负载均衡",
			config: ProxyConfig{
				Path:        "/api",
				Targets:     []ProxyTarget{{URL: "http://backend:8080"}},
				LoadBalance: "round_robin",
			},
			wantErr: false,
		},
		{
			name: "Path缺失",
			config: ProxyConfig{
				Targets: []ProxyTarget{{URL: "http://backend:8080"}},
			},
			wantErr: true,
			errMsg:  "path 必填",
		},
		{
			name: "Targets空",
			config: ProxyConfig{
				Path:    "/api",
				Targets: []ProxyTarget{},
			},
			wantErr: true,
			errMsg:  "targets 至少需要一个目标地址",
		},
		{
			name: "URL格式错误-无协议",
			config: ProxyConfig{
				Path:    "/api",
				Targets: []ProxyTarget{{URL: "backend:8080"}},
			},
			wantErr: true,
			errMsg:  "必须以 http:// 或 https:// 开头",
		},
		{
			name: "URL格式错误-空URL",
			config: ProxyConfig{
				Path:    "/api",
				Targets: []ProxyTarget{{URL: ""}},
			},
			wantErr: true,
			errMsg:  "url 必填",
		},
		{
			name: "无效负载均衡算法",
			config: ProxyConfig{
				Path:        "/api",
				Targets:     []ProxyTarget{{URL: "http://backend:8080"}},
				LoadBalance: "invalid_algorithm",
			},
			wantErr: true,
			errMsg:  "无效的负载均衡算法",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProxy(&tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateProxy() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateProxy() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validateProxy() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}

func TestValidateSSL(t *testing.T) {
	// TestValidateSSL 测试 SSL 配置验证。
	tests := []struct {
		name    string
		config  SSLConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "未配置SSL",
			config:  SSLConfig{},
			wantErr: false,
		},
		{
			name: "有效SSL配置",
			config: SSLConfig{
				Cert:      "/path/to/cert.pem",
				Key:       "/path/to/key.pem",
				Protocols: []string{"TLSv1.2", "TLSv1.3"},
			},
			wantErr: false,
		},
		{
			name: "仅Cert配置",
			config: SSLConfig{
				Cert: "/path/to/cert.pem",
			},
			wantErr: true,
			errMsg:  "cert 和 key 必须同时配置",
		},
		{
			name: "仅Key配置",
			config: SSLConfig{
				Key: "/path/to/key.pem",
			},
			wantErr: true,
			errMsg:  "cert 和 key 必须同时配置",
		},
		{
			name: "TLSv1.0不安全",
			config: SSLConfig{
				Cert:      "/path/to/cert.pem",
				Key:       "/path/to/key.pem",
				Protocols: []string{"TLSv1.0"},
			},
			wantErr: true,
			errMsg:  "不安全的 TLS 版本: TLSv1.0",
		},
		{
			name: "TLSv1.1不安全",
			config: SSLConfig{
				Cert:      "/path/to/cert.pem",
				Key:       "/path/to/key.pem",
				Protocols: []string{"TLSv1.1"},
			},
			wantErr: true,
			errMsg:  "不安全的 TLS 版本: TLSv1.1",
		},
		{
			name: "不安全加密套件RC4",
			config: SSLConfig{
				Cert:      "/path/to/cert.pem",
				Key:       "/path/to/key.pem",
				Protocols: []string{"TLSv1.2"},
				Ciphers:   []string{"RC4-SHA"},
			},
			wantErr: true,
			errMsg:  "不安全的加密套件",
		},
		{
			name: "不安全加密套件DES",
			config: SSLConfig{
				Cert:      "/path/to/cert.pem",
				Key:       "/path/to/key.pem",
				Protocols: []string{"TLSv1.2"},
				Ciphers:   []string{"DES-CBC3-SHA"},
			},
			wantErr: true,
			errMsg:  "不安全的加密套件",
		},
		{
			name: "未知TLS版本",
			config: SSLConfig{
				Cert:      "/path/to/cert.pem",
				Key:       "/path/to/key.pem",
				Protocols: []string{"TLSv1.4"},
			},
			wantErr: true,
			errMsg:  "未知的 TLS 版本",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSSL(&tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateSSL() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateSSL() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validateSSL() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}

func TestValidateAuth(t *testing.T) {
	// TestValidateAuth 测试认证配置验证。
	tests := []struct {
		name    string
		config  AuthConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "未配置认证",
			config:  AuthConfig{},
			wantErr: false,
		},
		{
			name: "有效Basic认证配置",
			config: AuthConfig{
				Type:      "basic",
				Algorithm: "bcrypt",
				Users:     []User{{Name: "admin", Password: "hashed_password"}},
			},
			wantErr: false,
		},
		{
			name: "无效认证类型",
			config: AuthConfig{
				Type:  "oauth",
				Users: []User{{Name: "admin", Password: "hashed_password"}},
			},
			wantErr: true,
			errMsg:  "不支持的认证类型",
		},
		{
			name: "启用认证但无用户",
			config: AuthConfig{
				Type:      "basic",
				Algorithm: "bcrypt",
				Users:     []User{},
			},
			wantErr: true,
			errMsg:  "启用认证时至少需要一个用户",
		},
		{
			name: "用户名缺失",
			config: AuthConfig{
				Type:      "basic",
				Algorithm: "bcrypt",
				Users:     []User{{Name: "", Password: "hashed_password"}},
			},
			wantErr: true,
			errMsg:  "name 必填",
		},
		{
			name: "密码缺失",
			config: AuthConfig{
				Type:      "basic",
				Algorithm: "bcrypt",
				Users:     []User{{Name: "admin", Password: ""}},
			},
			wantErr: true,
			errMsg:  "password 必填",
		},
		{
			name: "无效哈希算法",
			config: AuthConfig{
				Type:      "basic",
				Algorithm: "md5",
				Users:     []User{{Name: "admin", Password: "hashed_password"}},
			},
			wantErr: true,
			errMsg:  "不支持的哈希算法",
		},
		{
			name: "空算法默认有效",
			config: AuthConfig{
				Type:  "basic",
				Users: []User{{Name: "admin", Password: "hashed_password"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAuth(&tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateAuth() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateAuth() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validateAuth() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}

func TestValidateRateLimit(t *testing.T) {
	// TestValidateRateLimit 测试速率限制配置验证。
	tests := []struct {
		name    string
		config  RateLimitConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "未配置速率限制",
			config:  RateLimitConfig{},
			wantErr: false,
		},
		{
			name: "有效速率限制配置",
			config: RateLimitConfig{
				RequestRate: 100,
				Burst:       20,
				Key:         "ip",
			},
			wantErr: false,
		},
		{
			name: "负数RequestRate",
			config: RateLimitConfig{
				RequestRate: -1,
			},
			wantErr: true,
			errMsg:  "request_rate 不能为负数",
		},
		{
			name: "负数Burst",
			config: RateLimitConfig{
				RequestRate: 100,
				Burst:       -1,
			},
			wantErr: true,
			errMsg:  "burst 不能为负数",
		},
		{
			name: "负数ConnLimit",
			config: RateLimitConfig{
				ConnLimit: -1,
			},
			wantErr: true,
			errMsg:  "conn_limit 不能为负数",
		},
		{
			name: "无效Key来源",
			config: RateLimitConfig{
				RequestRate: 100,
				Key:         "invalid_key",
			},
			wantErr: true,
			errMsg:  "无效的 key 来源",
		},
		{
			name: "仅ConnLimit配置",
			config: RateLimitConfig{
				ConnLimit: 10,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRateLimit(&tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateRateLimit() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateRateLimit() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validateRateLimit() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}

func TestValidateCompression(t *testing.T) {
	// TestValidateCompression 测试压缩配置验证。
	tests := []struct {
		name    string
		config  CompressionConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "未配置压缩",
			config:  CompressionConfig{},
			wantErr: false,
		},
		{
			name: "有效gzip压缩配置",
			config: CompressionConfig{
				Type:    "gzip",
				Level:   6,
				MinSize: 1024,
			},
			wantErr: false,
		},
		{
			name: "有效brotli压缩配置",
			config: CompressionConfig{
				Type:    "brotli",
				Level:   4,
				MinSize: 512,
			},
			wantErr: false,
		},
		{
			name: "无效压缩类型",
			config: CompressionConfig{
				Type: "lz4",
			},
			wantErr: true,
			errMsg:  "无效的压缩类型",
		},
		{
			name: "级别过低",
			config: CompressionConfig{
				Type:  "gzip",
				Level: -1,
			},
			wantErr: true,
			errMsg:  "无效的压缩级别",
		},
		{
			name: "级别过高",
			config: CompressionConfig{
				Type:  "gzip",
				Level: 10,
			},
			wantErr: true,
			errMsg:  "无效的压缩级别",
		},
		{
			name: "负数MinSize",
			config: CompressionConfig{
				Type:    "gzip",
				MinSize: -100,
			},
			wantErr: true,
			errMsg:  "min_size 不能为负数",
		},
		{
			name: "级别0有效",
			config: CompressionConfig{
				Type:  "gzip",
				Level: 0,
			},
			wantErr: false,
		},
		{
			name: "级别9有效",
			config: CompressionConfig{
				Type:  "gzip",
				Level: 9,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCompression(&tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateCompression() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateCompression() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validateCompression() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}

func TestValidateAccess(t *testing.T) {
	// TestValidateAccess 测试访问控制配置验证。
	tests := []struct {
		name    string
		config  AccessConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "空配置有效",
			config:  AccessConfig{},
			wantErr: false,
		},
		{
			name: "有效CIDR",
			config: AccessConfig{
				Allow: []string{"192.168.1.0/24", "10.0.0.0/8"},
			},
			wantErr: false,
		},
		{
			name: "有效单个IP",
			config: AccessConfig{
				Allow: []string{"192.168.1.100"},
				Deny:  []string{"10.0.0.1"},
			},
			wantErr: false,
		},
		{
			name: "有效IPv6 CIDR",
			config: AccessConfig{
				Allow: []string{"2001:db8::/32"},
			},
			wantErr: false,
		},
		{
			name: "有效IPv6地址",
			config: AccessConfig{
				Allow: []string{"::1", "2001:db8::1"},
			},
			wantErr: false,
		},
		{
			name: "无效CIDR格式",
			config: AccessConfig{
				Allow: []string{"invalid-cidr"},
			},
			wantErr: true,
			errMsg:  "无效的 allow CIDR/IP",
		},
		{
			name: "无效Deny CIDR",
			config: AccessConfig{
				Deny: []string{"not-a-cidr"},
			},
			wantErr: true,
			errMsg:  "无效的 deny CIDR/IP",
		},
		{
			name: "有效默认动作allow",
			config: AccessConfig{
				Default: "allow",
			},
			wantErr: false,
		},
		{
			name: "有效默认动作deny",
			config: AccessConfig{
				Default: "deny",
			},
			wantErr: false,
		},
		{
			name: "无效默认动作",
			config: AccessConfig{
				Default: "reject",
			},
			wantErr: true,
			errMsg:  "无效的 default 动作",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAccess(&tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateAccess() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateAccess() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validateAccess() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}

func TestValidateStatic(t *testing.T) {
	// TestValidateStatic 测试静态文件配置验证。
	tests := []struct {
		name    string
		config  StaticConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "空配置有效",
			config:  StaticConfig{},
			wantErr: false,
		},
		{
			name: "有效根目录",
			config: StaticConfig{
				Root: "/var/www/html",
			},
			wantErr: false,
		},
		{
			name: "根目录含..路径遍历",
			config: StaticConfig{
				Root: "/var/www/../etc",
			},
			wantErr: true,
			errMsg:  "根目录路径不能包含 '..'",
		},
		{
			name: "根目录含多个..",
			config: StaticConfig{
				Root: "/var/../www/../html",
			},
			wantErr: true,
			errMsg:  "根目录路径不能包含 '..'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStatic(&tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateStatic() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateStatic() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validateStatic() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}

func TestValidateSecurity(t *testing.T) {
	// TestValidateSecurity 测试安全配置验证。
	tests := []struct {
		name    string
		config  SecurityConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "空配置有效",
			config:  SecurityConfig{},
			wantErr: false,
		},
		{
			name: "有效安全配置",
			config: SecurityConfig{
				Access: AccessConfig{
					Allow: []string{"192.168.1.0/24"},
				},
				Auth: AuthConfig{
					Type:  "basic",
					Users: []User{{Name: "admin", Password: "hashed"}},
				},
				RateLimit: RateLimitConfig{
					RequestRate: 100,
				},
			},
			wantErr: false,
		},
		{
			name: "无效Access配置",
			config: SecurityConfig{
				Access: AccessConfig{
					Allow: []string{"invalid-ip"},
				},
			},
			wantErr: true,
			errMsg:  "无效的 allow CIDR/IP",
		},
		{
			name: "无效Auth配置",
			config: SecurityConfig{
				Auth: AuthConfig{
					Type:  "invalid",
					Users: []User{{Name: "admin", Password: "hashed"}},
				},
			},
			wantErr: true,
			errMsg:  "不支持的认证类型",
		},
		{
			name: "无效RateLimit配置",
			config: SecurityConfig{
				RateLimit: RateLimitConfig{
					RequestRate: -1,
				},
			},
			wantErr: true,
			errMsg:  "request_rate 不能为负数",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSecurity(&tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateSecurity() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateSecurity() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validateSecurity() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}

func TestValidateStream(t *testing.T) {
	// TestValidateStream 测试 Stream 代理配置验证。
	tests := []struct {
		name    string
		config  StreamConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "有效 TCP Stream",
			config: StreamConfig{
				Listen:   ":3306",
				Protocol: "tcp",
				Upstream: StreamUpstream{
					Targets:     []StreamTarget{{Addr: "db1:3306"}},
					LoadBalance: "round_robin",
				},
			},
			wantErr: false,
		},
		{
			name: "有效 UDP Stream",
			config: StreamConfig{
				Listen:   ":53",
				Protocol: "udp",
				Upstream: StreamUpstream{
					Targets:     []StreamTarget{{Addr: "dns1:53"}},
					LoadBalance: "least_conn",
				},
			},
			wantErr: false,
		},
		{
			name: "监听地址为空",
			config: StreamConfig{
				Listen:   "",
				Protocol: "tcp",
			},
			wantErr: true,
			errMsg:  "listen 地址必填",
		},
		{
			name: "无效协议类型",
			config: StreamConfig{
				Listen:   ":3306",
				Protocol: "http",
			},
			wantErr: true,
			errMsg:  "无效的协议类型",
		},
		{
			name: "无目标地址",
			config: StreamConfig{
				Listen:   ":3306",
				Protocol: "tcp",
				Upstream: StreamUpstream{
					Targets: []StreamTarget{},
				},
			},
			wantErr: true,
			errMsg:  "upstream.targets 至少需要一个目标地址",
		},
		{
			name: "目标地址为空",
			config: StreamConfig{
				Listen:   ":3306",
				Protocol: "tcp",
				Upstream: StreamUpstream{
					Targets: []StreamTarget{{Addr: ""}},
				},
			},
			wantErr: true,
			errMsg:  "addr 必填",
		},
		{
			name: "无效负载均衡算法",
			config: StreamConfig{
				Listen:   ":3306",
				Protocol: "tcp",
				Upstream: StreamUpstream{
					Targets:     []StreamTarget{{Addr: "db1:3306"}},
					LoadBalance: "invalid_algorithm",
				},
			},
			wantErr: true,
			errMsg:  "无效的负载均衡算法",
		},
		{
			name: "有效加权轮询算法",
			config: StreamConfig{
				Listen:   ":3306",
				Protocol: "tcp",
				Upstream: StreamUpstream{
					Targets:     []StreamTarget{{Addr: "db1:3306"}},
					LoadBalance: "weighted_round_robin",
				},
			},
			wantErr: false,
		},
		{
			name: "有效 IP 哈希算法",
			config: StreamConfig{
				Listen:   ":3306",
				Protocol: "tcp",
				Upstream: StreamUpstream{
					Targets:     []StreamTarget{{Addr: "db1:3306"}},
					LoadBalance: "ip_hash",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStream(&tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateStream() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateStream() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validateStream() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}

func TestValidatePerformance(t *testing.T) {
	// TestValidatePerformance 测试性能配置验证。
	tests := []struct {
		name    string
		config  PerformanceConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "空配置有效",
			config: PerformanceConfig{
				FileCache: FileCacheConfig{},
				Transport: TransportConfig{},
			},
			wantErr: false,
		},
		{
			name: "有效的 file_cache 配置",
			config: PerformanceConfig{
				FileCache: FileCacheConfig{
					MaxEntries: 1000,
					MaxSize:    1024 * 1024 * 100,
				},
				GoroutinePool: GoroutinePoolConfig{
					Enabled: true,
				},
			},
			wantErr: false,
		},
		{
			name: "有效的 transport 配置（零值）",
			config: PerformanceConfig{
				Transport: TransportConfig{
					MaxIdleConns:        0,
					MaxIdleConnsPerHost: 0,
					MaxConnsPerHost:     0,
				},
			},
			wantErr: false,
		},
		{
			name: "有效的 transport 配置（正值）",
			config: PerformanceConfig{
				Transport: TransportConfig{
					MaxIdleConns:        100,
					MaxIdleConnsPerHost: 10,
					MaxConnsPerHost:     50,
				},
			},
			wantErr: false,
		},
		{
			name: "LRUEviction=true（废弃警告）",
			config: PerformanceConfig{
				FileCache: FileCacheConfig{
					LRUEviction: true,
				},
			},
			wantErr: false,
		},
		{
			name: "MaxIdleConns 负数",
			config: PerformanceConfig{
				Transport: TransportConfig{
					MaxIdleConns: -1,
				},
			},
			wantErr: true,
			errMsg:  "transport.max_idle_conns 不能为负数",
		},
		{
			name: "MaxIdleConnsPerHost 负数",
			config: PerformanceConfig{
				Transport: TransportConfig{
					MaxIdleConnsPerHost: -1,
				},
			},
			wantErr: true,
			errMsg:  "transport.max_idle_conns_per_host 不能为负数",
		},
		{
			name: "MaxConnsPerHost 负数",
			config: PerformanceConfig{
				Transport: TransportConfig{
					MaxConnsPerHost: -1,
				},
			},
			wantErr: true,
			errMsg:  "transport.max_conns_per_host 不能为负数",
		},
		{
			name: "多个 transport 字段为负",
			config: PerformanceConfig{
				Transport: TransportConfig{
					MaxIdleConns:        -1,
					MaxIdleConnsPerHost: -2,
					MaxConnsPerHost:     -3,
				},
			},
			wantErr: true,
			errMsg:  "transport.max_idle_conns 不能为负数",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePerformance(&tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validatePerformance() 期望返回错误，但返回 nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validatePerformance() 错误消息不匹配，期望包含 %q，实际 %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("validatePerformance() 期望返回 nil，但返回错误: %v", err)
				}
			}
		})
	}
}
