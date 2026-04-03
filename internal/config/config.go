// Package config 提供 YAML 配置文件的解析、验证和默认配置生成功能。
//
// 该文件包含配置结构体定义和加载/保存功能，包括：
//   - 根配置和服务器配置结构体
//   - SSL、安全、代理、压缩等子配置结构体
//   - 配置文件的加载、保存和验证方法
//
// 主要用途：
//
//	用于定义和管理服务器的完整配置，支持单服务器和多虚拟主机两种模式。
//
// 注意事项：
//   - 配置文件使用 YAML 格式
//   - 所有配置项都有合理的默认值
//   - 配置加载后会自动验证
//
// 作者：xfy
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 根配置结构，支持单服务器和多虚拟主机两种模式。
type Config struct {
	Server      ServerConfig      `yaml:"server"`      // 单服务器模式配置
	Servers     []ServerConfig    `yaml:"servers"`     // 多虚拟主机模式配置
	Stream      []StreamConfig    `yaml:"stream"`      // TCP/UDP Stream 代理配置
	HTTP3       HTTP3Config       `yaml:"http3"`       // HTTP/3 (QUIC) 配置
	Logging     LoggingConfig     `yaml:"logging"`     // 日志配置
	Performance PerformanceConfig `yaml:"performance"` // 性能配置
	Monitoring  MonitoringConfig  `yaml:"monitoring"`  // 监控配置
}

// HTTP3Config HTTP/3 (QUIC) 配置。
type HTTP3Config struct {
	Enabled     bool          `yaml:"enabled"`      // 是否启用 HTTP/3
	Listen      string        `yaml:"listen"`       // UDP 监听地址，如 ":443"
	MaxStreams  int           `yaml:"max_streams"`  // 最大并发流
	IdleTimeout time.Duration `yaml:"idle_timeout"` // 空闲超时
	Enable0RTT  bool          `yaml:"enable_0rtt"`  // 启用 0-RTT
}

// ServerConfig 服务器配置，包含监听地址、静态文件、代理、SSL 等设置。
type ServerConfig struct {
	Listen      string            `yaml:"listen"`      // 监听地址，如 ":8080"
	Name        string            `yaml:"name"`        // 服务器名称，用于虚拟主机匹配
	Static      StaticConfig      `yaml:"static"`      // 静态文件服务配置
	Proxy       []ProxyConfig     `yaml:"proxy"`       // 反向代理规则列表
	SSL         SSLConfig         `yaml:"ssl"`         // SSL/TLS 配置
	Security    SecurityConfig    `yaml:"security"`    // 安全配置
	Rewrite     []RewriteRule     `yaml:"rewrite"`     // URL 重写规则
	Compression CompressionConfig `yaml:"compression"` // 响应压缩配置
	// 新增字段
	ReadTimeout        time.Duration `yaml:"read_timeout"`          // 读取超时
	WriteTimeout       time.Duration `yaml:"write_timeout"`         // 写入超时
	IdleTimeout        time.Duration `yaml:"idle_timeout"`          // 空闲超时
	MaxConnsPerIP      int           `yaml:"max_conns_per_ip"`      // 每 IP 最大连接数
	MaxRequestsPerConn int           `yaml:"max_requests_per_conn"` // 每连接最大请求数
}

// StaticConfig 静态文件服务配置。
type StaticConfig struct {
	Root  string   `yaml:"root"`  // 静态文件根目录
	Index []string `yaml:"index"` // 索引文件列表，默认 ["index.html", "index.htm"]
}

// ProxyConfig 反向代理配置，支持负载均衡和健康检查。
type ProxyConfig struct {
	Path          string            `yaml:"path"`           // 匹配路径前缀
	Targets       []ProxyTarget     `yaml:"targets"`        // 后端目标列表
	LoadBalance   string            `yaml:"load_balance"`   // 负载均衡算法：round_robin, weighted_round_robin, least_conn, ip_hash, consistent_hash
	HashKey       string            `yaml:"hash_key"`       // 一致性哈希键：ip, uri, header:X-Name
	VirtualNodes  int               `yaml:"virtual_nodes"`  // 一致性哈希虚拟节点数，默认 150
	HealthCheck   HealthCheckConfig `yaml:"health_check"`   // 健康检查配置
	Timeout       ProxyTimeout      `yaml:"timeout"`        // 超时配置
	Headers       ProxyHeaders      `yaml:"headers"`        // 请求/响应头修改
	Cache         ProxyCacheConfig  `yaml:"cache"`          // 代理缓存配置
}

// ProxyTarget 后端目标配置。
type ProxyTarget struct {
	URL    string `yaml:"url"`    // 后端地址，如 "http://backend1:8080"
	Weight int    `yaml:"weight"` // 权重，用于加权轮询算法
}

// HealthCheckConfig 健康检查配置。
type HealthCheckConfig struct {
	Interval time.Duration `yaml:"interval"` // 检查间隔
	Path     string        `yaml:"path"`     // 健康检查路径
	Timeout  time.Duration `yaml:"timeout"`  // 检查超时时间
}

// ProxyTimeout 代理超时配置。
type ProxyTimeout struct {
	Connect time.Duration `yaml:"connect"` // 连接超时
	Read    time.Duration `yaml:"read"`    // 读取超时
	Write   time.Duration `yaml:"write"`   // 写入超时
}

// ProxyHeaders 代理请求/响应头配置。
type ProxyHeaders struct {
	SetRequest  map[string]string `yaml:"set_request"`  // 设置请求头
	SetResponse map[string]string `yaml:"set_response"` // 设置响应头
	Remove      []string          `yaml:"remove"`       // 移除的头部
}

// ProxyCacheConfig 代理缓存配置。
type ProxyCacheConfig struct {
	Enabled              bool          `yaml:"enabled"`                // 是否启用缓存
	MaxAge               time.Duration `yaml:"max_age"`                // 缓存有效期
	CacheLock            bool          `yaml:"cache_lock"`             // 缓存锁，防止击穿
	StaleWhileRevalidate time.Duration `yaml:"stale_while_revalidate"` // 过期缓存复用时间
}

// SSLConfig SSL/TLS 配置。
type SSLConfig struct {
	Cert         string     `yaml:"cert"`          // 证书文件路径
	Key          string     `yaml:"key"`           // 私钥文件路径
	CertChain    string     `yaml:"cert_chain"`    // 证书链文件路径
	Protocols    []string   `yaml:"protocols"`     // TLS 版本，默认 ["TLSv1.2", "TLSv1.3"]
	Ciphers      []string   `yaml:"ciphers"`       // 加密套件（仅 TLS 1.2 有效）
	OCSPStapling bool       `yaml:"ocsp_stapling"` // OCSP Stapling 支持
	HSTS         HSTSConfig `yaml:"hsts"`          // HSTS 配置
}

// HSTSConfig HTTP Strict Transport Security 配置。
type HSTSConfig struct {
	MaxAge            int  `yaml:"max_age"`             // 过期时间（秒），默认 31536000（1年）
	IncludeSubDomains bool `yaml:"include_sub_domains"` // 包含子域名，默认 true
	Preload           bool `yaml:"preload"`             // 加入 HSTS 预加载列表
}

// SecurityConfig 安全配置，包含访问控制、限流、认证和安全头部。
type SecurityConfig struct {
	Access    AccessConfig    `yaml:"access"`     // IP 访问控制
	RateLimit RateLimitConfig `yaml:"rate_limit"` // 速率限制
	Auth      AuthConfig      `yaml:"auth"`       // 认证配置
	Headers   SecurityHeaders `yaml:"headers"`    // 安全头部
}

// AccessConfig IP 访问控制配置。
type AccessConfig struct {
	Allow   []string `yaml:"allow"`   // 允许的 IP/CIDR 列表
	Deny    []string `yaml:"deny"`    // 拒绝的 IP/CIDR 列表
	Default string   `yaml:"default"` // 默认动作：allow 或 deny
}

// RateLimitConfig 速率限制配置。
type RateLimitConfig struct {
	RequestRate        int    `yaml:"request_rate"`         // 每秒请求数限制
	Burst              int    `yaml:"burst"`                // 突发流量上限
	ConnLimit          int    `yaml:"conn_limit"`           // 连接数限制
	Key                string `yaml:"key"`                  // 限流 key 来源：ip, header
	Algorithm          string `yaml:"algorithm"`            // 限流算法：token_bucket, sliding_window
	SlidingWindowMode  string `yaml:"sliding_window_mode"`  // 滑动窗口模式：approximate, precise
	SlidingWindow      int    `yaml:"sliding_window"`       // 滑动窗口大小（秒）
}

// AuthConfig 认证配置。
type AuthConfig struct {
	Type              string `yaml:"type"`                // 认证类型：basic
	RequireTLS        bool   `yaml:"require_tls"`         // 强制 HTTPS，默认 true
	Algorithm         string `yaml:"algorithm"`           // 哈希算法：bcrypt, argon2id
	Users             []User `yaml:"users"`               // 用户列表
	Realm             string `yaml:"realm"`               // 认证域
	MinPasswordLength int    `yaml:"min_password_length"` // 密码最小长度
}

// User 认证用户配置。
type User struct {
	Name     string `yaml:"name"`     // 用户名
	Password string `yaml:"password"` // 密码哈希
}

// SecurityHeaders 安全头部配置。
type SecurityHeaders struct {
	XFrameOptions         string `yaml:"x_frame_options"`         // X-Frame-Options: DENY, SAMEORIGIN
	XContentTypeOptions   string `yaml:"x_content_type_options"`  // X-Content-Type-Options: nosniff
	ContentSecurityPolicy string `yaml:"content_security_policy"` // Content-Security-Policy
	ReferrerPolicy        string `yaml:"referrer_policy"`         // Referrer-Policy
	PermissionsPolicy     string `yaml:"permissions_policy"`      // Permissions-Policy
}

// RewriteRule URL 重写规则。
type RewriteRule struct {
	Pattern     string `yaml:"pattern"`     // 匹配模式（正则表达式）
	Replacement string `yaml:"replacement"` // 替换目标
	Flag        string `yaml:"flag"`        // 标志：last, redirect, permanent, break
}

// CompressionConfig 响应压缩配置。
type CompressionConfig struct {
	Type                string   `yaml:"type"`                  // 压缩类型：gzip, brotli, both
	Level               int      `yaml:"level"`                 // 压缩级别：1-9
	MinSize             int      `yaml:"min_size"`              // 最小压缩大小（字节）
	Types               []string `yaml:"types"`                 // 可压缩的 MIME 类型
	GzipStatic          bool     `yaml:"gzip_static"`           // 启用预压缩文件支持
	GzipStaticExtensions []string `yaml:"gzip_static_extensions"` // 预压缩文件扩展名
}

// LoggingConfig 日志配置。
type LoggingConfig struct {
	Format string           `yaml:"format"` // 全局格式：text（默认）或 json，控制启动/停止日志
	Access AccessLogConfig  `yaml:"access"` // 访问日志
	Error  ErrorLogConfig   `yaml:"error"`  // 错误日志
}

// AccessLogConfig 访问日志配置。
type AccessLogConfig struct {
	Path   string `yaml:"path"`   // 日志文件路径
	Format string `yaml:"format"` // 日志格式
}

// ErrorLogConfig 错误日志配置。
type ErrorLogConfig struct {
	Path  string `yaml:"path"`  // 日志文件路径
	Level string `yaml:"level"` // 日志级别：debug, info, warn, error
}

// PerformanceConfig 性能配置。
type PerformanceConfig struct {
	GoroutinePool GoroutinePoolConfig `yaml:"goroutine_pool"` // Goroutine 池
	FileCache     FileCacheConfig     `yaml:"file_cache"`     // 文件缓存
	Transport     TransportConfig     `yaml:"transport"`      // HTTP Transport
}

// GoroutinePoolConfig Goroutine 池配置。
type GoroutinePoolConfig struct {
	Enabled     bool          `yaml:"enabled"`      // 是否启用
	MaxWorkers  int           `yaml:"max_workers"`  // 最大 worker 数
	MinWorkers  int           `yaml:"min_workers"`  // 最小 worker 数（预热）
	IdleTimeout time.Duration `yaml:"idle_timeout"` // 空闲超时
}

// FileCacheConfig 文件缓存配置。
type FileCacheConfig struct {
	MaxEntries  int64         `yaml:"max_entries"`  // 最大缓存条目数
	MaxSize     int64         `yaml:"max_size"`     // 内存上限（字节）
	Inactive    time.Duration `yaml:"inactive"`     // 未访问淘汰时间
	LRUEviction bool          `yaml:"lru_eviction"` // 启用 LRU 淘汰
}

// TransportConfig HTTP Transport 配置。
type TransportConfig struct {
	MaxIdleConns        int           `yaml:"max_idle_conns"`          // 最大空闲连接数
	MaxIdleConnsPerHost int           `yaml:"max_idle_conns_per_host"` // 每主机最大空闲连接
	IdleConnTimeout     time.Duration `yaml:"idle_conn_timeout"`       // 空闲连接超时
	MaxConnsPerHost     int           `yaml:"max_conns_per_host"`      // 每主机最大连接数
}

// MonitoringConfig 监控配置。
type MonitoringConfig struct {
	Status StatusConfig `yaml:"status"` // 状态端点配置
}

// StatusConfig 状态监控端点配置。
type StatusConfig struct {
	Path  string   `yaml:"path"`  // 端点路径
	Allow []string `yaml:"allow"` // 允许访问的 IP 列表
}

// StreamConfig TCP/UDP Stream 代理配置。
type StreamConfig struct {
	Listen   string         `yaml:"listen"`   // 监听地址，如 ":3306"
	Protocol string         `yaml:"protocol"` // 协议：tcp 或 udp
	Upstream StreamUpstream `yaml:"upstream"` // 上游配置
}

// StreamUpstream Stream 上游配置。
type StreamUpstream struct {
	Targets     []StreamTarget `yaml:"targets"`      // 目标列表
	LoadBalance string         `yaml:"load_balance"` // 负载均衡算法
}

// StreamTarget Stream 目标配置。
type StreamTarget struct {
	Addr   string `yaml:"addr"`   // 目标地址，如 "mysql1:3306"
	Weight int    `yaml:"weight"` // 权重
}

// Load 从文件加载配置。
//
// 读取指定路径的 YAML 配置文件，解析并验证配置内容。
//
// 参数：
//   - path: 配置文件路径
//
// 返回值：
//   - *Config: 解析后的配置对象
//   - error: 读取、解析或验证失败时的错误信息
//
// 注意事项：
//   - 加载后会自动调用 Validate 进行配置验证
//   - 文件不存在或格式错误都会返回错误
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return &cfg, nil
}

// LoadFromString 从 YAML 字符串加载配置。
//
// 解析 YAML 格式的配置字符串，适用于从环境变量或命令行参数加载配置。
//
// 参数：
//   - yamlStr: YAML 格式的配置字符串
//
// 返回值：
//   - *Config: 解析后的配置对象
//   - error: 解析或验证失败时的错误信息
//
// 注意事项：
//   - 加载后会自动调用 Validate 进行配置验证
func LoadFromString(yamlStr string) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return &cfg, nil
}

// Save 保存配置到文件。
//
// 将配置对象序列化为 YAML 格式并写入指定文件。
//
// 参数：
//   - cfg: 配置对象
//   - path: 目标文件路径
//
// 返回值：
//   - error: 序列化或写入失败时的错误信息
//
// 注意事项：
//   - 文件权限设为 0644
func Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// HasServers 检查是否为多虚拟主机模式。
//
// 返回值：
//   - bool: 如果配置了 servers 列表且非空，返回 true
func (c *Config) HasServers() bool {
	return len(c.Servers) > 0
}

// HasDefaultServer 检查是否有默认服务器配置。
//
// 返回值：
//   - bool: 如果 server.listen 已配置，返回 true
func (c *Config) HasDefaultServer() bool {
	return c.Server.Listen != ""
}

// GetDefaultServer 获取默认服务器配置。
//
// 用于在虚拟主机模式下获取默认服务器的配置作为 fallback。
//
// 返回值：
//   - *ServerConfig: 默认服务器配置，如未配置则返回 nil
func (c *Config) GetDefaultServer() *ServerConfig {
	if c.HasDefaultServer() {
		return &c.Server
	}
	return nil
}

// Validate 配置验证入口。
//
// 验证配置的完整性和有效性，检查是否至少配置了一个服务器，
// 并递归验证所有服务器配置。
//
// 参数：
//   - cfg: 配置对象
//
// 返回值：
//   - error: 验证失败时的错误信息，包含具体字段路径
//
// 验证规则：
//   - 必须配置 server 或 servers 中的至少一个
//   - 所有服务器配置必须通过 validateServer 验证
func Validate(cfg *Config) error {
	// 至少需要一种服务器配置
	if !cfg.HasDefaultServer() && !cfg.HasServers() {
		return errors.New("至少需要配置 server 或 servers")
	}

	// 验证默认服务器
	if cfg.HasDefaultServer() {
		if err := validateServer(&cfg.Server, true); err != nil {
			return err
		}
	}

	// 验证所有虚拟主机
	for i := range cfg.Servers {
		if err := validateServer(&cfg.Servers[i], false); err != nil {
			return fmt.Errorf("servers[%d]: %w", i, err)
		}
	}

	return nil
}
