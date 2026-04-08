// Package config 提供 YAML 配置文件的解析、验证和默认配置生成功能。
//
// 该文件包含配置验证相关的核心逻辑，包括：
//   - 服务器配置验证（监听地址、静态文件、代理）
//   - SSL/TLS 配置验证（证书、协议、加密套件）
//   - 安全配置验证（访问控制、认证、速率限制）
//   - 压缩配置验证（类型、级别、最小大小）
//
// 主要用途：
//
//	用于验证用户提供的配置是否符合要求，确保服务器启动前配置有效。
//
// 注意事项：
//   - 验证失败时返回详细的错误信息
//   - 支持默认服务器和虚拟主机两种模式的验证
//
// 作者：xfy
package config

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"rua.plus/lolly/internal/loadbalance"
)

// validateServer 验证服务器配置。
//
// 检查服务器配置的各项参数是否符合要求，包括监听地址、
// 静态文件、代理、SSL、安全和压缩等配置。
//
// 参数：
//   - s: 服务器配置对象
//   - isDefault: 是否为默认服务器，默认服务器可省略部分配置
//
// 返回值：
//   - error: 验证失败时返回具体错误信息，成功返回 nil
//
// 注意事项：
//   - 默认服务器可省略监听地址
//   - 验证错误信息包含字段路径，便于定位问题
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
	if err := validateStatics(s.Static); err != nil {
		return fmt.Errorf("static: %w", err)
	}

	// 验证代理配置
	for i := range s.Proxy {
		if err := validateProxy(&s.Proxy[i]); err != nil {
			return fmt.Errorf("proxy[%d]: %w", i, err)
		}
	}

	// 检查 static 和 proxy 路径冲突
	if err := validatePathConflicts(s); err != nil {
		return err
	}

	// 验证重写规则
	for i := range s.Rewrite {
		if err := validateRewrite(&s.Rewrite[i]); err != nil {
			return fmt.Errorf("rewrite[%d]: %w", i, err)
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

// validateStatics 验证静态文件配置数组。
//
// 检查静态文件配置的路径重复和根目录路径安全性。
//
// 参数：
//   - statics: 静态文件配置数组
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
func validateStatics(statics []StaticConfig) error {
	if len(statics) == 0 {
		return nil
	}

	paths := make(map[string]int)
	for i, s := range statics {
		// Path 默认为 "/"
		path := s.Path
		if path == "" {
			path = "/"
		}

		// 检查路径重复
		if idx, exists := paths[path]; exists {
			return fmt.Errorf("路径 %s 重复定义 (static[%d] 和 static[%d])", path, idx, i)
		}
		paths[path] = i

		// 验证根目录路径安全
		if s.Root != "" && strings.Contains(s.Root, "..") {
			return fmt.Errorf("static[%d]: 根目录路径不能包含 '..'", i)
		}
	}
	return nil
}

// validatePathConflicts 检查 static 和 proxy 路径冲突。
//
// 确保 static 和 proxy 没有相同的 path 前缀。
//
// 参数：
//   - s: 服务器配置对象
//
// 返回值：
//   - error: 发现冲突时返回错误信息，成功返回 nil
func validatePathConflicts(s *ServerConfig) error {
	staticPaths := make(map[string]int)
	for i, st := range s.Static {
		path := st.Path
		if path == "" {
			path = "/"
		}
		staticPaths[path] = i
	}

	for i, p := range s.Proxy {
		if idx, exists := staticPaths[p.Path]; exists {
			return fmt.Errorf("路径 %s 同时定义在 static[%d] 和 proxy[%d]", p.Path, idx, i)
		}
	}
	return nil
}

// validateStatic 验证静态文件配置。
//
// 检查静态文件根目录路径的安全性，防止路径遍历攻击。
//
// 参数：
//   - s: 静态文件配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
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
//
// 检查代理路径、目标地址和负载均衡算法的有效性。
//
// 参数：
//   - p: 代理配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
//
// 验证规则：
//   - path 必填
//   - targets 至少需要一个目标
//   - 目标 URL 必须以 http:// 或 https:// 开头
//   - load_balance 必须是有效的负载均衡算法
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
	if !loadbalance.IsValidAlgorithm(p.LoadBalance) {
		return fmt.Errorf("无效的负载均衡算法：%s", p.LoadBalance)
	}

	// 验证故障转移配置
	if err := validateNextUpstream(&p.NextUpstream); err != nil {
		return fmt.Errorf("next_upstream: %w", err)
	}

	// 验证一致性哈希键格式
	if p.HashKey != "" {
		validHashKeys := []string{"ip", "uri"}
		valid := false
		for _, k := range validHashKeys {
			if p.HashKey == k || strings.HasPrefix(p.HashKey, "header:") {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("无效的 hash_key: %s（仅支持 ip, uri 或 header:X-Name 格式）", p.HashKey)
		}
	}

	return nil
}

// validateSSL 验证 SSL 配置。
//
// 检查 SSL 证书、私钥、TLS 协议版本和加密套件的有效性。
//
// 参数：
//   - s: SSL 配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
//
// 验证规则：
//   - cert 和 key 必须同时配置或同时为空
//   - TLS 协议仅允许 TLSv1.2 和 TLSv1.3
//   - 拒绝不安全的加密套件（RC4、DES、3DES、CBC）
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
//
// 验证访问控制、认证、速率限制和安全头部的有效性。
//
// 参数：
//   - s: 安全配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
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

	// 验证安全头部配置
	if err := validateSecurityHeaders(&s.Headers); err != nil {
		return fmt.Errorf("headers: %w", err)
	}

	return nil
}

// validateAccess 验证访问控制配置。
//
// 检查允许和拒绝列表中的 CIDR/IP 格式，以及默认动作的有效性。
//
// 参数：
//   - a: 访问控制配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
//
// 验证规则：
//   - allow 和 deny 列表中的项必须是有效的 CIDR 或 IP 地址
//   - default 动作仅允许 "allow" 或 "deny"
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
//
// 检查认证类型、哈希算法和用户列表的有效性。
//
// 参数：
//   - a: 认证配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
//
// 验证规则：
//   - type 目前仅支持 "basic"
//   - algorithm 仅支持 bcrypt 或 argon2id
//   - 启用认证时至少需要一个用户
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
	// 注意：SSL 配置在 ServerConfig 中，这里无法直接检查
	// 需要在上层验证中检查 SSL 与 Auth 的关联
	_ = a.RequireTLS // 避免空分支警告

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
//
// 检查请求速率、突发容量和连接限制的有效性。
//
// 参数：
//   - r: 速率限制配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
//
// 验证规则：
//   - request_rate、burst、conn_limit 不能为负数
//   - key 仅支持 "ip" 或 "header"
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

	// 验证限流算法
	validAlgorithms := []string{"", "token_bucket", "sliding_window"}
	valid = false
	for _, alg := range validAlgorithms {
		if r.Algorithm == alg {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("无效的限流算法: %s（仅支持 token_bucket 或 sliding_window）", r.Algorithm)
	}

	// 验证滑动窗口模式
	validModes := []string{"", "approximate", "precise"}
	valid = false
	for _, mode := range validModes {
		if r.SlidingWindowMode == mode {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("无效的滑动窗口模式: %s（仅支持 approximate 或 precise）", r.SlidingWindowMode)
	}

	return nil
}

// validateCompression 验证压缩配置。
//
// 检查压缩类型、压缩级别和最小压缩大小的有效性。
//
// 参数：
//   - c: 压缩配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
//
// 验证规则：
//   - type 仅支持 gzip、brotli 或 both
//   - level 范围为 0-9
//   - min_size 不能为负数
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

// validateRewrite 验证 URL 重写规则。
//
// 检查重写模式、替换目标和标志的有效性。
//
// 参数：
//   - r: 重写规则配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
//
// 验证规则：
//   - pattern 必填
//   - flag 仅允许 last, redirect, permanent, break
func validateRewrite(r *RewriteRule) error {
	// 模式必填
	if r.Pattern == "" {
		return errors.New("pattern 必填")
	}

	// 验证标志
	validFlags := []string{"", "last", "redirect", "permanent", "break"}
	valid := false
	for _, f := range validFlags {
		if r.Flag == f {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("无效的 flag: %s（仅支持 last, redirect, permanent, break）", r.Flag)
	}

	return nil
}

// validateLogging 验证日志配置。
//
// 检查日志格式和级别的有效性。
//
// 参数：
//   - l: 日志配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
//
// 验证规则：
//   - format 仅允许 text 或 json
//   - level 仅允许 debug, info, warn, error
func validateLogging(l *LoggingConfig) error {
	// 验证日志格式
	validFormats := []string{"", "text", "json"}
	valid := false
	for _, f := range validFormats {
		if l.Format == f {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("无效的日志格式: %s（仅支持 text 或 json）", l.Format)
	}

	// 验证错误日志级别
	validLevels := []string{"", "debug", "info", "warn", "error"}
	valid = false
	for _, lvl := range validLevels {
		if l.Error.Level == lvl {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("无效的日志级别: %s（仅支持 debug, info, warn, error）", l.Error.Level)
	}

	return nil
}

// validateSecurityHeaders 验证安全头部配置。
//
// 检查各安全头部值的有效性。
//
// 参数：
//   - h: 安全头部配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
//
// 验证规则：
//   - x_frame_options 仅允许 DENY, SAMEORIGIN 或空
//   - referrer_policy 仅允许标准 RFC 值
func validateSecurityHeaders(h *SecurityHeaders) error {
	// 验证 X-Frame-Options
	validFrameOptions := []string{"", "DENY", "SAMEORIGIN"}
	valid := false
	for _, opt := range validFrameOptions {
		if h.XFrameOptions == opt {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("无效的 x_frame_options: %s（仅支持 DENY, SAMEORIGIN 或空）", h.XFrameOptions)
	}

	// 验证 Referrer-Policy
	validReferrerPolicies := []string{
		"", "no-referrer", "no-referrer-when-downgrade", "origin",
		"origin-when-cross-origin", "same-origin", "strict-origin",
		"strict-origin-when-cross-origin", "unsafe-url",
	}
	valid = false
	for _, policy := range validReferrerPolicies {
		if h.ReferrerPolicy == policy {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("无效的 referrer_policy: %s", h.ReferrerPolicy)
	}

	return nil
}

// validateStream 验证 Stream 代理配置。
//
// 检查监听地址、协议类型和上游配置的有效性。
//
// 参数：
//   - s: Stream 配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
//
// 验证规则：
//   - listen 必填
//   - protocol 仅允许 tcp 或 udp
//   - upstream.targets 至少需要一个目标
func validateStream(s *StreamConfig) error {
	// 监听地址必填
	if s.Listen == "" {
		return errors.New("listen 地址必填")
	}

	// 验证协议类型
	if s.Protocol != "tcp" && s.Protocol != "udp" {
		return fmt.Errorf("无效的协议类型: %s（仅允许 tcp 或 udp）", s.Protocol)
	}

	// 验证上游目标
	if len(s.Upstream.Targets) == 0 {
		return errors.New("upstream.targets 至少需要一个目标地址")
	}

	// 验证每个目标地址
	for i, t := range s.Upstream.Targets {
		if t.Addr == "" {
			return fmt.Errorf("upstream.targets[%d].addr 必填", i)
		}
	}

	// 验证负载均衡算法
	if !loadbalance.IsValidAlgorithm(s.Upstream.LoadBalance) {
		return fmt.Errorf("无效的负载均衡算法：%s", s.Upstream.LoadBalance)
	}

	return nil
}

// validatePerformance 验证性能配置。
//
// 检查性能配置中的废弃选项和潜在问题，输出警告信息。
//
// 参数：
//   - p: 性能配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
func validatePerformance(p *PerformanceConfig) error {
	// 检查 Transport 配置（可能导致性能问题）
	if p.Transport.MaxIdleConnsPerHost < 0 {
		return errors.New("transport.max_idle_conns_per_host 不能为负数")
	}
	if p.Transport.MaxConnsPerHost < 0 {
		return errors.New("transport.max_conns_per_host 不能为负数")
	}

	return nil
}

// validateNextUpstream 验证故障转移配置。
//
// 检查重试次数和 HTTP 状态码的有效性。
//
// 参数：
//   - n: 故障转移配置对象
//
// 返回值：
//   - error: 验证失败时返回错误信息，成功返回 nil
//
// 验证规则：
//   - tries 不能为负数，建议不超过后端数量
//   - http_codes 应包含有效的 HTTP 状态码
func validateNextUpstream(n *NextUpstreamConfig) error {
	// 未配置时跳过
	if n.Tries == 0 && len(n.HTTPCodes) == 0 {
		return nil
	}

	// 验证重试次数
	if n.Tries < 0 {
		return errors.New("tries 不能为负数")
	}

	// 验证 HTTP 状态码
	for i, code := range n.HTTPCodes {
		if code < 100 || code > 599 {
			return fmt.Errorf("http_codes[%d]: 无效的 HTTP 状态码 %d", i, code)
		}
	}

	return nil
}
