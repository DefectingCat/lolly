// Package config 提供 YAML 配置文件的解析、验证和默认配置生成功能。
package config

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// validateServer 验证服务器配置。
func validateServer(s *ServerConfig, isDefault bool) error {
	// 监听地址必填（默认服务器可省略，使用默认值）
	if s.Listen == "" && !isDefault {
		return errors.New("listen 地址必填")
	}

	// 验证监听地址格式
	if s.Listen != "" {
		if _, err := net.ResolveTCPAddr("tcp", s.Listen); err != nil {
			return fmt.Errorf("无效的监听地址 %s: %w", s.Listen, err)
		}
	}

	// 验证静态文件配置
	if err := validateStatic(&s.Static); err != nil {
		return fmt.Errorf("static: %w", err)
	}

	// 验证代理配置
	for i := range s.Proxy {
		if err := validateProxy(&s.Proxy[i]); err != nil {
			return fmt.Errorf("proxy[%d]: %w", i, err)
		}
	}

	// 验证 SSL 配置
	if err := validateSSL(&s.SSL); err != nil {
		return fmt.Errorf("ssl: %w", err)
	}

	// 验证安全配置
	if err := validateSecurity(&s.Security); err != nil {
		return fmt.Errorf("security: %w", err)
	}

	// 验证压缩配置
	if err := validateCompression(&s.Compression); err != nil {
		return fmt.Errorf("compression: %w", err)
	}

	return nil
}

// validateStatic 验证静态文件配置。
func validateStatic(s *StaticConfig) error {
	// 静态文件根目录非空时验证路径有效性
	if s.Root != "" {
		// 路径安全检查：不允许包含 ".."
		if strings.Contains(s.Root, "..") {
			return errors.New("根目录路径不能包含 '..'")
		}
	}
	return nil
}

// validateProxy 验证代理配置。
func validateProxy(p *ProxyConfig) error {
	// 路径必填
	if p.Path == "" {
		return errors.New("path 必填")
	}

	// 至少需要一个目标
	if len(p.Targets) == 0 {
		return errors.New("targets 至少需要一个目标地址")
	}

	// 验证每个目标地址
	for i, t := range p.Targets {
		if t.URL == "" {
			return fmt.Errorf("targets[%d].url 必填", i)
		}
		if !strings.HasPrefix(t.URL, "http://") && !strings.HasPrefix(t.URL, "https://") {
			return fmt.Errorf("targets[%d].url 必须以 http:// 或 https:// 开头", i)
		}
	}

	// 验证负载均衡算法
	validAlgorithms := []string{"", "round_robin", "weighted_round_robin", "least_conn", "ip_hash"}
	valid := false
	for _, alg := range validAlgorithms {
		if p.LoadBalance == alg {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("无效的负载均衡算法: %s", p.LoadBalance)
	}

	return nil
}

// validateSSL 验证 SSL 配置。
func validateSSL(s *SSLConfig) error {
	// 未配置 SSL 时跳过验证
	if s.Cert == "" && s.Key == "" {
		return nil
	}

	// 证书和私钥必须同时配置
	if s.Cert == "" || s.Key == "" {
		return errors.New("cert 和 key 必须同时配置")
	}

	// 验证 TLS 版本
	for _, proto := range s.Protocols {
		if proto == "TLSv1.0" || proto == "TLSv1.1" {
			return fmt.Errorf("不安全的 TLS 版本: %s（仅允许 TLSv1.2 和 TLSv1.3）", proto)
		}
		if proto != "TLSv1.2" && proto != "TLSv1.3" {
			return fmt.Errorf("未知的 TLS 版本: %s", proto)
		}
	}

	// 验证加密套件（拒绝不安全的）
	insecureCiphers := []string{"RC4", "DES", "3DES", "CBC"}
	for _, cipher := range s.Ciphers {
		for _, insecure := range insecureCiphers {
			if strings.Contains(cipher, insecure) {
				return fmt.Errorf("不安全的加密套件: %s", cipher)
			}
		}
	}

	return nil
}

// validateSecurity 验证安全配置。
func validateSecurity(s *SecurityConfig) error {
	// 验证访问控制配置
	if err := validateAccess(&s.Access); err != nil {
		return fmt.Errorf("access: %w", err)
	}

	// 验证认证配置
	if err := validateAuth(&s.Auth); err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	// 验证速率限制配置
	if err := validateRateLimit(&s.RateLimit); err != nil {
		return fmt.Errorf("rate_limit: %w", err)
	}

	return nil
}

// validateAccess 验证访问控制配置。
func validateAccess(a *AccessConfig) error {
	// 验证 CIDR 格式
	for _, cidr := range a.Allow {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			// 尝试作为单个 IP 解析
			if ip := net.ParseIP(cidr); ip == nil {
				return fmt.Errorf("无效的 allow CIDR/IP: %s", cidr)
			}
		}
	}

	for _, cidr := range a.Deny {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			if ip := net.ParseIP(cidr); ip == nil {
				return fmt.Errorf("无效的 deny CIDR/IP: %s", cidr)
			}
		}
	}

	// 验证默认动作
	if a.Default != "" && a.Default != "allow" && a.Default != "deny" {
		return fmt.Errorf("无效的 default 动作: %s（仅允许 allow 或 deny）", a.Default)
	}

	return nil
}

// validateAuth 验证认证配置。
func validateAuth(a *AuthConfig) error {
	// 未配置认证时跳过
	if a.Type == "" {
		return nil
	}

	// 仅支持 basic 认证
	if a.Type != "basic" {
		return fmt.Errorf("不支持的认证类型: %s（仅支持 basic）", a.Type)
	}

	// 启用 Basic Auth 时检查是否强制 HTTPS
	if a.RequireTLS {
		// 注意：SSL 配置在 ServerConfig 中，这里无法直接检查
		// 需要在上层验证中检查 SSL 与 Auth 的关联
	}

	// 验证哈希算法
	validAlgorithms := []string{"", "bcrypt", "argon2id"}
	valid := false
	for _, alg := range validAlgorithms {
		if a.Algorithm == alg {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("不支持的哈希算法: %s（仅支持 bcrypt 或 argon2id）", a.Algorithm)
	}

	// 至少需要一个用户
	if len(a.Users) == 0 {
		return errors.New("启用认证时至少需要一个用户")
	}

	// 验证每个用户
	for i, u := range a.Users {
		if u.Name == "" {
			return fmt.Errorf("users[%d].name 必填", i)
		}
		if u.Password == "" {
			return fmt.Errorf("users[%d].password 必填", i)
		}
	}

	return nil
}

// validateRateLimit 验证速率限制配置。
func validateRateLimit(r *RateLimitConfig) error {
	// 未配置时跳过
	if r.RequestRate == 0 && r.ConnLimit == 0 {
		return nil
	}

	// 验证速率限制值
	if r.RequestRate < 0 {
		return errors.New("request_rate 不能为负数")
	}
	if r.Burst < 0 {
		return errors.New("burst 不能为负数")
	}
	if r.ConnLimit < 0 {
		return errors.New("conn_limit 不能为负数")
	}

	// 验证 key 来源
	validKeys := []string{"", "ip", "header"}
	valid := false
	for _, k := range validKeys {
		if r.Key == k {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("无效的 key 来源: %s（仅支持 ip 或 header）", r.Key)
	}

	return nil
}

// validateCompression 验证压缩配置。
func validateCompression(c *CompressionConfig) error {
	// 未配置时跳过
	if c.Type == "" {
		return nil
	}

	// 验证压缩类型
	validTypes := []string{"gzip", "brotli", "both"}
	valid := false
	for _, t := range validTypes {
		if c.Type == t {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("无效的压缩类型: %s（仅支持 gzip, brotli 或 both）", c.Type)
	}

	// 验证压缩级别
	if c.Level < 0 || c.Level > 9 {
		return fmt.Errorf("无效的压缩级别: %d（范围 0-9）", c.Level)
	}

	// 验证最小压缩大小
	if c.MinSize < 0 {
		return errors.New("min_size 不能为负数")
	}

	return nil
}