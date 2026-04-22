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
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// 默认配置常量。
const (
	// DefaultPprofPath pprof 端点的默认路径。
	DefaultPprofPath = "/debug/pprof"
)

// ServerMode 服务器运行模式类型。
//
// 定义服务器的工作模式，支持显式配置或自动推断。
type ServerMode string

// ServerMode 枚举值。
const (
	// ServerModeSingle 单服务器模式 - 只运行一个服务器实例。
	ServerModeSingle ServerMode = "single"
	// ServerModeVHost 虚拟主机模式 - 多个服务器共享相同的监听地址。
	ServerModeVHost ServerMode = "vhost"
	// ServerModeMultiServer 多服务器模式 - 多个服务器监听不同的地址。
	ServerModeMultiServer ServerMode = "multi_server"
	// ServerModeAuto 自动模式 - 根据配置自动推断运行模式。
	ServerModeAuto ServerMode = "auto"
)

// Config 根配置结构，支持单服务器和多虚拟主机两种模式。
//
// 包含服务器配置、日志配置、性能配置和监控配置等模块。
// 是配置文件的顶级结构体，所有其他配置都作为其子结构。
//
// 注意事项：
//   - 必须配置 servers 列表中的至少一个
//   - 加载后会自动进行配置验证
//   - Stream 配置为可选，用于 TCP/UDP 层代理
//   - HTTP/3 配置为可选，需 SSL 配置配合才能生效
//
// 使用示例：
//
//	cfg, err := config.Load("config.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// 使用多虚拟主机模式
//	for _, s := range cfg.Servers {
//	    // 处理每个服务器配置
//	}
type Config struct {
	Mode        ServerMode            `yaml:"mode"`
	Variables   VariablesConfig       `yaml:"variables"`
	Logging     LoggingConfig         `yaml:"logging"`
	Servers     []ServerConfig        `yaml:"servers"`
	Stream      []StreamConfig        `yaml:"stream"`
	Monitoring  MonitoringConfig      `yaml:"monitoring"`
	HTTP3       HTTP3Config           `yaml:"http3"`
	Resolver    ResolverConfig        `yaml:"resolver"`
	Performance PerformanceConfig     `yaml:"performance"`
	Shutdown    ShutdownConfig        `yaml:"shutdown"`
	Include     []IncludeConfig       `yaml:"include"`    // 配置引入，支持从其他文件引入配置片段
	CachePath   *ProxyCachePathConfig `yaml:"cache_path"` // 缓存路径配置（磁盘持久化）
}

// IncludeConfig 配置引入配置。
//
// 用于从其他文件加载配置片段并合并到当前配置。
// 支持 glob 模式展开多个文件。
//
// 使用示例：
//
//	include:
//	  - path: "conf.d/*.yaml"
type IncludeConfig struct {
	Path string `yaml:"path"`
}

// VariablesConfig 自定义变量配置。
//
// 用于定义全局自定义变量，可在日志格式和请求头中引用。
// 变量作用于所有虚拟主机。
//
// 注意事项：
//   - 变量名只允许字母、数字、下划线
//   - 变量名不能与内置变量冲突
//   - 变量名不能以 arg_、http_、cookie_ 开头（动态变量前缀）
//
// 使用示例：
//
//	variables:
//	  set:
//	    app_name: "lolly"
//	    version: "1.0.0"
type VariablesConfig struct {
	// Set 自定义变量集合
	// 键值对形式，可在日志格式和请求头模板中使用 $var_name 引用
	Set map[string]string `yaml:"set"`
}

// HTTP2Config HTTP/2 配置。
//
// HTTP/2 提供多路复用、头部压缩和服务器推送等功能，
// 需要服务器配置 SSL/TLS 证书才能正常工作。
//
// 注意事项：
//   - 必须配置有效的 SSL 证书（TLS 1.2 或更高版本）
//   - http2.enabled 仅在配置了 SSL/TLS 时生效
//   - 客户端可以通过 ALPN 协商使用 HTTP/2 或 HTTP/1.1
//
// 使用示例：
//
//	server:
//	  ssl:
//	    cert: "/etc/ssl/server.crt"
//	    key: "/etc/ssl/server.key"
//	    http2:
//	      enabled: true
//	      max_concurrent_streams: 128
//	      max_header_list_size: "16KB"
type HTTP2Config struct {
	MaxConcurrentStreams    int           `yaml:"max_concurrent_streams"`
	MaxHeaderListSize       int           `yaml:"max_header_list_size"`
	IdleTimeout             time.Duration `yaml:"idle_timeout"`
	Enabled                 bool          `yaml:"enabled"`
	PushEnabled             bool          `yaml:"push_enabled"`
	H2CEnabled              bool          `yaml:"h2c_enabled"`
	GracefulShutdownTimeout time.Duration `yaml:"graceful_shutdown_timeout"`
}

// HTTP3Config HTTP/3 (QUIC) 配置。
//
// HTTP/3 基于 QUIC 协议，提供更快的连接建立和更低的延迟。
// 需要服务器配置 SSL/TLS 证书才能正常工作。
//
// 注意事项：
//   - 必须配置有效的 SSL 证书
//   - UDP 监听地址不能与 HTTP/1.1 或 HTTP/2 冲突
//   - 0-RTT 特性可能带来重放攻击风险，需评估安全性
//   - 部分网络环境可能限制 UDP 流量
//
// 使用示例：
//
//	http3:
//	  enabled: true
//	  listen: ":443"
//	  max_streams: 1000
//	  idle_timeout: 30s
//	  enable_0rtt: true
type HTTP3Config struct {
	Listen      string        `yaml:"listen"`
	MaxStreams  int           `yaml:"max_streams"`
	IdleTimeout time.Duration `yaml:"idle_timeout"`
	Enabled     bool          `yaml:"enabled"`
	Enable0RTT  bool          `yaml:"enable_0rtt"`
}

// ServerConfig 服务器配置，包含监听地址、静态文件、代理、SSL 等设置。
//
// 用于定义单个服务器的完整行为，包括网络监听、请求处理、
// 安全防护和性能控制等方面。
//
// 注意事项：
//   - Listen 字段为必填项，格式为 "host:port" 或 ":port"
//   - Name 字段用于虚拟主机匹配，多服务器模式下建议配置
//   - SSL 配置为可选，但生产环境强烈建议启用
//   - 超时设置需根据实际业务场景调整
//
// 使用示例：
//
//	server:
//	  listen: ":8080"
//	  name: "api.example.com"
//	  read_timeout: 30s
//	  write_timeout: 30s
type ServerConfig struct {
	// 指针类型字段（按大小排列，减少 padding）
	CacheAPI *CacheAPIConfig      `yaml:"cache_api"`
	Lua      *LuaMiddlewareConfig `yaml:"lua"`
	// 切片字段
	Static  []StaticConfig `yaml:"static"`
	Proxy   []ProxyConfig  `yaml:"proxy"`
	Rewrite []RewriteRule  `yaml:"rewrite"`
	// 字符串字段
	ClientMaxBodySize string `yaml:"client_max_body_size"`
	Name              string `yaml:"name"`
	Listen            string `yaml:"listen"`
	// 结构体字段（嵌入类型）
	Security    SecurityConfig    `yaml:"security"`
	Compression CompressionConfig `yaml:"compression"`
	SSL         SSLConfig         `yaml:"ssl"`
	UnixSocket  UnixSocketConfig  `yaml:"unix_socket"` // Unix socket 配置
	LimitRate   LimitRateConfig   `yaml:"limit_rate"`  // 响应速率限制配置
	Types       TypesConfig       `yaml:"types"`       // MIME 类型配置
	// 切片字段
	ServerNames []string `yaml:"server_names"` // 支持多个 server_name
	// time.Duration 字段（int64）
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	// 基本类型字段（int 按大小排列）
	MaxRequestsPerConn int `yaml:"max_requests_per_conn"`
	MaxConnsPerIP      int `yaml:"max_conns_per_ip"`
	Concurrency        int `yaml:"concurrency"`       // 最大并发连接数（默认 256 * 1024）
	ReadBufferSize     int `yaml:"read_buffer_size"`  // 读缓冲区大小（字节，默认 16KB）
	WriteBufferSize    int `yaml:"write_buffer_size"` // 写缓冲区大小（字节，默认 16KB）
	// 布尔字段（放在一起减少 padding）
	Default           bool `yaml:"default,omitempty"`   // VHost 默认主机标记
	ReduceMemoryUsage bool `yaml:"reduce_memory_usage"` // 是否优先减少内存使用（默认 false，优先性能）
	ServerTokens      bool `yaml:"server_tokens"`       // false 隐藏版本号，默认 true（零值表示显示版本）
}

// StaticConfig 静态文件服务配置。
//
// 用于配置静态文件服务器的行为，包括路径匹配、根目录和索引文件。
//
// 注意事项：
//   - Path 为路径前缀，匹配的请求将被该静态处理器处理
//   - Root 路径可以是相对路径或绝对路径
//   - 索引文件按顺序查找，第一个存在的文件将被使用
//   - 目录路径需要确保有读取权限
//
// 使用示例：
//
//	static:
//	  - path: "/"
//	    root: "/var/www/html"
//	    index: ["index.html", "index.htm"]
//	  - path: "/assets/"
//	    root: "/var/www/assets"
type StaticConfig struct {
	// Path 匹配路径前缀
	// 以此前缀开头的请求将被该静态处理器处理
	// 默认为 "/"，匹配所有路径
	Path string `yaml:"path"`

	// Root 静态文件根目录
	// 所有静态文件请求都将以此目录为基础解析
	Root string `yaml:"root"`

	// Index 索引文件列表
	// 访问目录时依次查找这些文件作为默认页面
	// 默认为 ["index.html", "index.htm"]
	Index []string `yaml:"index"`

	// TryFiles 按顺序尝试查找的文件列表
	// 支持以下模式：
	//   - $uri: 请求路径
	//   - $uri/: 请求路径加斜杠（目录）
	//   - $uri.<ext>: 请求路径加扩展名（如 $uri.html, $uri.json）
	//   - /path: 绝对路径回退（如 /index.html）
	//   - filename: 相对路径回退（如 fallback.html）
	//
	// nginx 兼容性：
	//   - $uri 变量语义与 nginx try_files 指令一致
	//   - 配置语法可从 nginx 直接迁移
	//
	// 安全限制（附加于 nginx 基础）：
	//   - 扩展名仅允许字母、数字、点、下划线、连字符
	//   - 禁止危险后缀（.php, .exe, .bat 等）
	//   - 禁止 null byte 和路径分隔符
	//
	// 根路径边界情况：
	//   - 当 relPath="/" 且模式为 "$uri.<ext>" 时，返回空字符串
	//   - 此设计避免生成 "/.html" 这样的隐藏文件名
	//   - 建议使用绝对路径回退（如 /index.html）处理根路径
	//
	// 示例:
	//   try_files: ["$uri", "$uri.html", "/index.html"]
	//   try_files: ["$uri", "$uri/", "/app.html"]
	TryFiles []string `yaml:"try_files"`

	// TryFilesPass 内部重定向是否触发中间件
	// 默认为 false，内部重定向不触发中间件
	// 设置为 true 时，try_files 回退会重新进入中间件链
	TryFilesPass bool `yaml:"try_files_pass"`

	// SymlinkCheck 是否启用符号链接安全检查
	// 默认为 false，启用后会验证符号链接指向的文件是否在允许的路径范围内
	// 防止通过符号链接访问敏感文件（如 /etc/passwd）
	SymlinkCheck bool `yaml:"symlink_check"`

	// LocationType 位置匹配类型
	// 可选值：exact、prefix、regex、regex_caseless、prefix_priority、named
	LocationType string `yaml:"location_type"`

	// Internal 仅允许内部访问
	// 设置为 true 时，该位置仅允许内部重定向访问
	Internal bool `yaml:"internal"`
}

// ProxyConfig 反向代理配置，支持负载均衡和健康检查。
//
// 用于将请求转发到后端服务器，支持多种负载均衡算法
// 和健康检查机制。
//
// 注意事项：
//   - Path 使用前缀匹配，较长路径优先匹配
//   - 至少配置一个 Target 才能正常工作
//   - 负载均衡算法支持：round_robin、weighted_round_robin、least_conn、ip_hash、consistent_hash、random
//   - 一致性哈希需要配置 HashKey
//
// 使用示例：
//
//	proxy:
//	  - path: "/api/"
//	    targets:
//	      - url: "http://backend1:8080"
//	        weight: 3
//	      - url: "http://backend2:8080"
//	        weight: 1
//	    load_balance: "weighted_round_robin"
//	    health_check:
//	      interval: 10s
//	      path: "/health"
type ProxyConfig struct {
	// 指针类型字段（按大小排列）
	RedirectRewrite *RedirectRewriteConfig `yaml:"redirect_rewrite"`
	ProxySSL        *ProxySSLConfig        `yaml:"proxy_ssl"`
	CacheValid      *ProxyCacheValidConfig `yaml:"cache_valid"`
	Buffering       *ProxyBufferingConfig  `yaml:"buffering"`
	// 切片字段
	Targets []ProxyTarget `yaml:"targets"`
	// 字符串字段
	Path              string `yaml:"path"`
	LoadBalance       string `yaml:"load_balance"`
	HashKey           string `yaml:"hash_key"`
	ClientMaxBodySize string `yaml:"client_max_body_size"`
	ProxyBind         string `yaml:"proxy_bind"`
	// 结构体字段
	Headers       ProxyHeaders        `yaml:"headers"`
	BalancerByLua BalancerByLuaConfig `yaml:"balancer_by_lua"`
	HealthCheck   HealthCheckConfig   `yaml:"health_check"`
	NextUpstream  NextUpstreamConfig  `yaml:"next_upstream"`
	Cache         ProxyCacheConfig    `yaml:"cache"`
	Timeout       ProxyTimeout        `yaml:"timeout"`
	// 基本类型字段
	VirtualNodes int `yaml:"virtual_nodes"`

	// LocationType 位置匹配类型
	// 可选值：exact、prefix_priority、regex、regex_caseless、prefix、named
	LocationType string `yaml:"location_type"`

	// LocationName 位置名称
	// 仅当 LocationType 为 named 时使用，用于命名位置块
	LocationName string `yaml:"location_name"`

	// Internal 仅允许内部访问
	// 设置为 true 时，该位置仅允许内部重定向访问
	Internal bool `yaml:"internal"`
}

// ProxyBufferingConfig 代理缓冲配置。
//
// 控制代理响应的缓冲行为：
//   - "default" 或 "on": 缓冲响应到内存/临时文件
//   - "off": 流式转发响应，不缓冲
//
// 使用示例：
//
//	buffering:
//	  mode: "off"
type ProxyBufferingConfig struct {
	// Mode 缓冲模式
	// 可选值："default"（默认缓冲）, "on"（强制缓冲）, "off"（关闭缓冲）
	Mode string `yaml:"mode"`

	// BufferSize 响应缓冲区大小（字节）
	// 0 表示使用默认值
	BufferSize int `yaml:"buffer_size"`

	// Buffers 多缓冲区配置字符串
	// 格式："数量 大小" 或 "数量1 大小1 数量2 大小2 ..."
	// 例如："8 16k" 表示 8 个 16KB 缓冲区
	// 例如："4 4k 8 16k" 表示 4 个 4KB + 8 个 16KB 缓冲区
	Buffers string `yaml:"buffers"`

	// BufferCount 缓冲区数量（解析后）
	BufferCount int `yaml:"-"`

	// BufferSizeEach 每个缓冲区大小（字节，解析后）
	BufferSizeEach int `yaml:"-"`
}

// ParseBuffers 解析 Buffers 配置字符串。
//
// 支持格式：
//   - "8 16k" → 8 个 16KB 缓冲区
//   - "4 4k" → 4 个 4KB 缓冲区
//
// 大小单位：
//   - k 或 K: KB (1024 字节)
//   - m 或 M: MB (1024 * 1024 字节)
//   - 无单位: 字节
func (c *ProxyBufferingConfig) ParseBuffers() {
	if c.Buffers == "" {
		// 向后兼容：使用 BufferSize
		if c.BufferSize > 0 {
			c.BufferCount = 1
			c.BufferSizeEach = c.BufferSize
		}
		return
	}

	parts := strings.Fields(c.Buffers)
	if len(parts) < 2 {
		return // 无效格式
	}

	count, err := strconv.Atoi(parts[0])
	if err != nil || count <= 0 {
		return // 无效数量
	}

	sizeEach, err := parseSize(parts[1])
	if err != nil || sizeEach <= 0 {
		return // 无效大小
	}

	c.BufferCount = count
	c.BufferSizeEach = sizeEach
}

// parseSize 解析大小字符串（支持 k, m 单位）。
func parseSize(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, strconv.ErrSyntax
	}

	// 提取单位
	unit := strings.ToLower(s[len(s)-1:])
	multiplier := 1
	numStr := s

	if unit == "k" {
		multiplier = 1024
		numStr = s[:len(s)-1]
	} else if unit == "m" {
		multiplier = 1024 * 1024
		numStr = s[:len(s)-1]
	}

	value, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, err
	}

	return value * multiplier, nil
}

// BalancerByLuaConfig Lua 负载均衡配置
//
// 使用 Lua 脚本动态选择后端目标，支持自定义负载均衡逻辑。
//
// 注意事项：
//   - Script 为 Lua 脚本文件路径
//   - Timeout 控制脚本执行超时
//   - Fallback 指定 Lua 失败时的备用算法
//
// 使用示例：
//
//	balancer_by_lua:
//	  enabled: true
//	  script: "/etc/lolly/scripts/balancer.lua"
//	  timeout: 100ms
//	  fallback: "round_robin"
type BalancerByLuaConfig struct {
	// Script Lua 脚本路径
	Script string `yaml:"script"`

	// Fallback 失败时使用的默认负载均衡算法
	// 默认值: "round_robin"
	Fallback string `yaml:"fallback"`

	// Timeout 执行超时
	// 默认值: 100ms
	Timeout time.Duration `yaml:"timeout"`

	// Enabled 是否启用
	Enabled bool `yaml:"enabled"`
}

// ProxyTarget 后端目标配置。
//
// 定义单个后端服务器的地址和权重。
//
// 注意事项：
//   - URL 必须包含协议（http:// 或 https://）
//   - Weight 仅在 weighted_round_robin 算法下生效
//
// 使用示例：
//
//	targets:
//	  - url: "http://backend1:8080"
//	    weight: 3
//	  - url: "http://backend2:8080"
//	    weight: 1
type ProxyTarget struct {
	// URL 后端地址
	// 格式："http://host:port" 或 "https://host:port"
	URL string `yaml:"url"`

	// Weight 权重
	// 用于加权轮询算法，值越大分配的请求越多
	Weight int `yaml:"weight"`

	// MaxConns 最大并发连接数
	// 0 表示不限制
	MaxConns int `yaml:"max_conns"`

	// MaxFails 最大失败次数
	// 在 FailTimeout 期间失败次数达到此值后标记为不可用
	// 0 表示不进行被动失败检测
	MaxFails int `yaml:"max_fails"`

	// FailTimeout 失败超时时间
	// 达到 MaxFails 后，目标在此时间内被视为不可用
	FailTimeout time.Duration `yaml:"fail_timeout"`

	// Backup 备份服务器
	// 仅当所有非备份服务器不可用时才使用
	Backup bool `yaml:"backup"`

	// Down 标记服务器为永久不可用
	Down bool `yaml:"down"`

	// ProxyURI 代理传递的 URI 路径
	// 设置后替换请求路径，支持 nginx proxy_pass URI 语义
	ProxyURI string `yaml:"proxy_uri"`
}

// HealthCheckConfig 健康检查配置。
//
// 定期检查后端服务器的健康状态，自动剔除不健康的节点。
//
// 注意事项：
//   - Interval 不宜设置过小，避免增加后端负担
//   - Path 应该是轻量级的健康检查端点
//   - 超时时间应小于检查间隔
//
// 使用示例：
//
//	health_check:
//	  interval: 10s
//	  path: "/health"
//	  timeout: 5s
type HealthCheckConfig struct {
	Path      string             `yaml:"path"`
	Interval  time.Duration      `yaml:"interval"`
	Timeout   time.Duration      `yaml:"timeout"`
	Match     *HealthMatchConfig `yaml:"match"`      // 健康检查匹配配置
	SlowStart time.Duration      `yaml:"slow_start"` // 慢启动时间
}

// HealthMatchConfig 健康检查匹配配置。
type HealthMatchConfig struct {
	Status  []string          `yaml:"status"`  // 状态码范围列表
	Body    string            `yaml:"body"`    // 响应体正则表达式
	Headers map[string]string `yaml:"headers"` // 响应头匹配
}

// ProxyTimeout 代理超时配置。
//
// 控制代理请求的各个阶段超时。
//
// 注意事项：
//   - Connect 超时包括 DNS 解析和 TCP 连接建立
//   - Read 和 Write 超时分别控制响应读取和请求发送
//   - 超时时间需要根据后端服务响应时间调整
//
// 使用示例：
//
//	timeout:
//	  connect: 5s
//	  read: 30s
//	  write: 30s
type ProxyTimeout struct {
	// Connect 连接超时
	// 建立到后端服务器的连接超时
	Connect time.Duration `yaml:"connect"`

	// Read 读取超时
	// 从后端读取响应的超时
	Read time.Duration `yaml:"read"`

	// Write 写入超时
	// 向后端发送请求的超时
	Write time.Duration `yaml:"write"`
}

// ProxyHeaders 代理请求/响应头配置。
//
// 在代理转发过程中修改 HTTP 头部。
//
// 注意事项：
//   - SetRequest 添加/修改发送到后端的请求头
//   - SetResponse 添加/修改返回给客户端的响应头
//   - Remove 会删除指定的请求头（在发送到后端之前）
//
// 使用示例：
//
//	headers:
//	  set_request:
//	    X-Forwarded-For: "$remote_addr"
//	    X-Real-IP: "$remote_addr"
//	  set_response:
//	    X-Proxy-By: "lolly"
//	  remove:
//	    - "X-Internal-Header"
type ProxyHeaders struct {
	// SetRequest 设置请求头
	// 发送到后端的请求中添加或覆盖的头部
	SetRequest map[string]string `yaml:"set_request"`

	// SetResponse 设置响应头
	// 返回给客户端的响应中添加或覆盖的头部
	SetResponse map[string]string `yaml:"set_response"`

	// Remove 移除的头部
	// 从发送到后端的请求中移除的头部列表
	Remove []string `yaml:"remove"`

	// HideResponse 隐藏的响应头
	// 从返回给客户端的响应中移除的头部列表
	HideResponse []string `yaml:"hide_response"`

	// PassResponse 允许传递的响应头
	// 仅传递列出的头部，其他全部隐藏（白名单模式）
	PassResponse []string `yaml:"pass_response"`

	// IgnoreHeaders 忽略的头部
	// 代理时完全忽略这些头部，不转发到后端也不返回给客户端
	IgnoreHeaders []string `yaml:"ignore_headers"`

	// CookieDomain Cookie 域重写
	// 将响应中 Set-Cookie 的 domain 替换为此值
	CookieDomain string `yaml:"cookie_domain"`

	// CookiePath Cookie 路径重写
	// 将响应中 Set-Cookie 的 path 替换为此值
	CookiePath string `yaml:"cookie_path"`
}

// ProxyCachePathConfig 缓存路径配置（磁盘持久化）。
//
// 配置磁盘缓存路径和相关参数，支持 L1/L2 分层缓存架构。
// 配置后，代理缓存将持久化到磁盘，服务重启后可恢复。
//
// 注意事项：
//   - Path 为必填项，指定缓存根目录
//   - Levels 支持最多 3 级目录（如 "1:2:2"）
//   - MaxSize 为 0 表示不限制大小
//   - L1MaxEntries/L1MaxSize 为 0 时使用默认值
//
// 使用示例：
//
//	cache_path:
//	  path: "/var/cache/lolly"
//	  levels: "1:2"
//	  max_size: "1GB"
//	  inactive: "60m"
//	  l1_max_entries: 10000
type ProxyCachePathConfig struct {
	// Path 缓存根目录
	Path string `yaml:"path"`

	// Levels 目录层级，如 "1:2" 表示两级目录
	Levels string `yaml:"levels"`

	// MaxSize 最大缓存大小（字节）
	MaxSize int64 `yaml:"max_size"`

	// Inactive 未访问淘汰时间
	Inactive time.Duration `yaml:"inactive"`

	// Purger 是否启用后台清理
	Purger bool `yaml:"purger"`

	// PurgerInterval 清理间隔
	PurgerInterval time.Duration `yaml:"purger_interval"`

	// L1MaxEntries L1 最大条目数
	L1MaxEntries int64 `yaml:"l1_max_entries"`

	// L1MaxSize L1 最大内存大小
	L1MaxSize int64 `yaml:"l1_max_size"`

	// PromoteThreshold 提升到 L1 的访问阈值
	PromoteThreshold int `yaml:"promote_threshold"`
}

// ProxyCacheConfig 代理缓存配置。
//
// 缓存后端响应，减少重复请求，提高响应速度。
//
// 注意事项：
//   - 仅缓存 GET 和 HEAD 请求
//   - 后端响应中 Cache-Control 头会覆盖 MaxAge 设置
//   - CacheLock 可防止缓存击穿，但会增加首次请求延迟
//   - 谨慎缓存动态内容，避免返回过期数据
//
// 使用示例：
//
//	cache:
//	  enabled: true
//	  max_age: 5m
//	  cache_lock: true
//	  stale_while_revalidate: 1m
type ProxyCacheConfig struct {
	MaxAge                  time.Duration `yaml:"max_age"`
	StaleWhileRevalidate    time.Duration `yaml:"stale_while_revalidate"`
	StaleIfError            time.Duration `yaml:"stale_if_error"`   // 错误时使用过期缓存
	StaleIfTimeout          time.Duration `yaml:"stale_if_timeout"` // 超时时使用过期缓存
	Enabled                 bool          `yaml:"enabled"`
	CacheLock               bool          `yaml:"cache_lock"`
	Methods                 []string      `yaml:"methods"`
	MinUses                 int           `yaml:"min_uses"`                  // 缓存阈值，请求次数达到此值才缓存
	CacheLockTimeout        time.Duration `yaml:"cache_lock_timeout"`        // 缓存锁超时时间
	BackgroundUpdateDisable bool          `yaml:"background_update_disable"` // 禁用后台更新（默认 false = 启用后台更新）
	CacheIgnoreHeaders      []string      `yaml:"cache_ignore_headers"`      // 缓存时忽略的响应头
	Revalidate              bool          `yaml:"revalidate"`                // 启用条件请求（If-Modified-Since/If-None-Match）
}

// ProxyCacheValidConfig 缓存有效期分段配置。
//
// 按 HTTP 状态码配置不同的缓存有效期，提供更精细的缓存控制。
// 未配置 CacheValid 时，使用 ProxyCacheConfig.MaxAge 作为统一缓存时间。
//
// 注意事项：
//   - OK=0 时继承 MaxAge（向后兼容）
//   - 其他字段为 0 表示不缓存该类响应
//   - NotFound 缓存需谨慎，避免缓存错误页面
//
// 使用示例：
//
//	cache_valid:
//	  ok: 10m        # 200-299 缓存 10 分钟
//	  redirect: 1h   # 301/302 缓存 1 小时
//	  not_found: 1m  # 404 缓存 1 分钟
//	  client_error: 0  # 其他客户端错误不缓存
//	  server_error: 0  # 服务端错误不缓存
type ProxyCacheValidConfig struct {
	// OK 200-299 状态码缓存时间
	// 0 表示继承 MaxAge
	OK time.Duration `yaml:"ok"`

	// Redirect 301/302 重定向缓存时间
	// 0 表示不缓存
	Redirect time.Duration `yaml:"redirect"`

	// NotFound 404 缓存时间
	// 0 表示不缓存
	NotFound time.Duration `yaml:"not_found"`

	// ClientError 400-499（除 404）缓存时间
	// 0 表示不缓存
	ClientError time.Duration `yaml:"client_error"`

	// ServerError 500-599 缓存时间
	// 0 表示不缓存
	ServerError time.Duration `yaml:"server_error"`
}

// ProxySSLConfig 上游 SSL/TLS 配置。
//
// 配置代理连接上游服务器时的 TLS 行为，支持自定义 CA、客户端证书（mTLS）、
// SNI 和 TLS 版本控制。
//
// 注意事项：
//   - Enabled 为 true 时启用自定义 TLS 配置
//   - TrustedCA 用于验证上游服务器证书
//   - ClientCert + ClientKey 用于 mTLS 客户端认证
//   - InsecureSkipVerify 仅用于测试，生产环境禁用
//
// 使用示例：
//
//	proxy_ssl:
//	  enabled: true
//	  server_name: "api.internal"
//	  trusted_ca: "/etc/ssl/ca/upstream-ca.crt"
//	  client_cert: "/etc/ssl/client.crt"
//	  client_key: "/etc/ssl/client.key"
//	  min_version: "TLSv1.2"
type ProxySSLConfig struct {
	// 字符串字段
	ServerName string `yaml:"server_name"`
	TrustedCA  string `yaml:"trusted_ca"`
	ClientCert string `yaml:"client_cert"`
	ClientKey  string `yaml:"client_key"`
	MinVersion string `yaml:"min_version"`
	MaxVersion string `yaml:"max_version"`
	// 布尔字段
	Enabled            bool `yaml:"enabled"`
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

// RedirectRewriteConfig Location/Refresh 头改写配置
//
// 用于配置代理响应中 Location 和 Refresh 头的改写行为。
//
// 注意事项：
//   - Mode 支持 "default"、"off"、"custom" 三种模式
//   - 未配置或空字符串时默认为 "default" 模式
//   - "custom" 模式必须配置至少一条规则
//
// 使用示例：
//
//	redirect_rewrite:
//	  mode: "default"  # 或 "off" 或 "custom"
//	  rules:
//	    - pattern: "http://backend:8000/"
//	      replacement: "$scheme://$host:$server_port/"
type RedirectRewriteConfig struct {
	// Mode 运行模式: "default" | "off" | "custom"
	// default: 自动从选中的 target URL 生成规则（运行时）
	// off: 禁用改写
	// custom: 使用 Rules 列表（预编译）
	// 未配置或空字符串时默认为 "default"
	Mode string `yaml:"mode"`

	// Rules 改写规则列表，仅在 Mode="custom" 时使用
	Rules []RedirectRewriteRule `yaml:"rules"`
}

// RedirectRewriteRule 单条改写规则
//
// 定义 Location/Refresh 头改写的匹配模式和替换目标。
//
// 注意事项：
//   - Pattern 以 ~ 开头表示正则，~* 表示大小写不敏感
//   - 无 ~ 前缀时使用前缀匹配语义
//   - Replacement 支持变量展开（$host, $scheme, $server_port 等）
//
// 使用示例：
//
//	rules:
//	  - pattern: "http://backend:8000/"
//	    replacement: "$scheme://$host:$server_port/"
//	  - pattern: "~^http://[^/]+:8000/(.*)$"
//	    replacement: "$scheme://$host/$1"
type RedirectRewriteRule struct {
	// Pattern 匹配模式，支持正则（以 ~ 开头）或精确匹配
	// 示例: "http://localhost:8000/" 或 "~^http://[^/]+:8000/"
	Pattern string `yaml:"pattern"`

	// Replacement 替换目标，支持变量展开
	// 示例: "$scheme://$host:$server_port/" 或 "/"
	Replacement string `yaml:"replacement"`
}

// NextUpstreamConfig 故障转移配置，定义后端失败时的自动重试行为。
//
// 当后端返回特定错误状态码或连接失败时，自动尝试下一个可用后端。
//
// 注意事项：
//   - Tries 为 1 时禁用故障转移
//   - 空 NextUpstream 使用默认值（Tries=1，禁用故障转移）
//   - 建议根据后端数量合理设置 Tries 值
//
// 使用示例：
//
//	next_upstream:
//	  tries: 3
//	  http_codes: [502, 503, 504]
type NextUpstreamConfig struct {
	HTTPCodes []int `yaml:"http_codes"`
	Tries     int   `yaml:"tries"`
}

// SSLConfig SSL/TLS 配置。
//
// 用于配置 HTTPS 服务所需的证书和加密参数。
// 支持 TLS 1.2 和 TLS 1.3 协议，可自定义加密套件。
//
// 注意事项：
//   - Cert 和 Key 为必需字段，分别指向证书和私钥文件
//   - CertChain 可选，用于配置完整的证书链
//   - Protocols 建议使用默认值，避免使用不安全的 TLS 1.0/1.1
//   - Ciphers 仅对 TLS 1.2 有效，TLS 1.3 有固定加密套件
//   - 启用 OCSPStapling 可提升握手性能
//
// 使用示例：
//
//	ssl:
//	  cert: "/etc/ssl/certs/server.crt"
//	  key: "/etc/ssl/private/server.key"
//	  cert_chain: "/etc/ssl/certs/chain.crt"
//	  protocols: ["TLSv1.2", "TLSv1.3"]
//	  ocsp_stapling: true
//	  hsts:
//	    max_age: 31536000
//	    include_sub_domains: true
type SSLConfig struct {
	ClientVerify   ClientVerifyConfig   `yaml:"client_verify"`
	Cert           string               `yaml:"cert"`
	Key            string               `yaml:"key"`
	CertChain      string               `yaml:"cert_chain"`
	Protocols      []string             `yaml:"protocols"`
	Ciphers        []string             `yaml:"ciphers"`
	SessionTickets SessionTicketsConfig `yaml:"session_tickets"`
	HTTP2          HTTP2Config          `yaml:"http2"`
	HSTS           HSTSConfig           `yaml:"hsts"`
	OCSPStapling   bool                 `yaml:"ocsp_stapling"`
}

// HSTSConfig HTTP Strict Transport Security 配置。
//
// 强制浏览器使用 HTTPS 访问，防止中间人攻击和协议降级攻击。
//
// 注意事项：
//   - MaxAge 单位为秒，建议至少设置为 1 年（31536000）
//   - IncludeSubDomains 为 true 时策略应用于所有子域名
//   - Preload 为 true 表示申请加入浏览器预加载列表
//   - 启用前确保所有站点资源都支持 HTTPS
//
// 使用示例：
//
//	hsts:
//	  max_age: 31536000
//	  include_sub_domains: true
//	  preload: false
type HSTSConfig struct {
	// MaxAge 过期时间（秒）
	// 默认 31536000（1年），建议至少 6 个月
	MaxAge int `yaml:"max_age"`

	// IncludeSubDomains 包含子域名
	// 为 true 时策略应用于当前域名及其所有子域名
	IncludeSubDomains bool `yaml:"include_sub_domains"`

	// Preload 加入 HSTS 预加载列表
	// 申请加入浏览器内置的 HSTS 列表
	Preload bool `yaml:"preload"`
}

// SessionTicketsConfig TLS Session Ticket 配置。
//
// Session Tickets 允许 TLS 1.3 会话恢复，避免完整握手，显著提升性能。
// 密钥定期轮换增强安全性，同时保留旧密钥确保已发放的票据仍可解密。
//
// 注意事项：
//   - KeyFile 为密钥存储文件路径，用于持久化密钥
//   - RotateInterval 为密钥轮换间隔，建议 1-24 小时
//   - RetainKeys 为保留的历史密钥数量，至少保留 2 个
//   - 密钥文件权限应为 0600（仅所有者可读写）
//
// 使用示例：
//
//	ssl:
//	  session_tickets:
//	    enabled: true
//	    key_file: "/var/lib/lolly/session_tickets.key"
//	    rotate_interval: 1h
//	    retain_keys: 3
type SessionTicketsConfig struct {
	KeyFile        string        `yaml:"key_file"`
	RotateInterval time.Duration `yaml:"rotate_interval"`
	RetainKeys     int           `yaml:"retain_keys"`
	Enabled        bool          `yaml:"enabled"`
}

// ClientVerifyConfig mTLS 客户端证书验证配置。
//
// 配置双向 TLS 认证，要求客户端提供有效证书才能建立连接。
// 适用于需要强身份验证的场景，如 API 服务、内部系统通信。
//
// 注意事项：
//   - Mode 可选值：none、request、require、optional_no_ca
//   - ClientCA 为客户端 CA 证书文件路径（必需）
//   - VerifyDepth 为证书链验证深度，默认 1
//   - CRL 为证书撤销列表文件路径（可选）
//
// 使用示例：
//
//	ssl:
//	  client_verify:
//	    enabled: true
//	    mode: "require"
//	    client_ca: "/etc/ssl/ca/client-ca.crt"
//	    verify_depth: 2
//	    crl: "/etc/ssl/ca/client-ca.crl"
type ClientVerifyConfig struct {
	Mode        string `yaml:"mode"`
	ClientCA    string `yaml:"client_ca"`
	CRL         string `yaml:"crl"`
	VerifyDepth int    `yaml:"verify_depth"`
	Enabled     bool   `yaml:"enabled"`
}

// SecurityConfig 安全配置，包含访问控制、限流、认证和安全头部。
//
// 用于保护服务器免受各种网络攻击和滥用。
//
// 注意事项：
//   - Access 配置 IP 黑白名单控制访问来源
//   - RateLimit 配置请求频率限制防止 DDoS 攻击
//   - Auth 配置 HTTP Basic 认证保护敏感资源
//   - Headers 配置安全响应头部增强浏览器安全
//   - 各项配置可以组合使用，增强安全性
//
// 使用示例：
//
//	security:
//	  access:
//	    allow: ["192.168.1.0/24"]
//	    deny: ["10.0.0.0/8"]
//	  rate_limit:
//	    request_rate: 100
//	    burst: 150
//	  auth:
//	    type: "basic"
//	    users:
//	      - name: "admin"
//	        password: "$2y$10$..."
//	  headers:
//	    x_frame_options: "DENY"
type SecurityConfig struct {
	Headers     SecurityHeaders   `yaml:"headers"`
	Access      AccessConfig      `yaml:"access"`
	ErrorPage   ErrorPageConfig   `yaml:"error_page"`
	Auth        AuthConfig        `yaml:"auth"`
	AuthRequest AuthRequestConfig `yaml:"auth_request"`
	RateLimit   RateLimitConfig   `yaml:"rate_limit"`
}

// AccessConfig IP 访问控制配置。
//
// 通过 IP 地址或 CIDR 范围控制访问权限，支持基于 GeoIP 的国家代码访问控制。
//
// 注意事项：
//   - Allow 和 Deny 列表按配置顺序匹配
//   - Default 指定未匹配时的默认动作
//   - TrustedProxies 用于正确获取客户端真实 IP
//   - GeoIP 配置启用后，会基于国家代码进行二次检查
//   - 支持 IPv4 和 IPv6 地址格式
//
// 使用示例：
//
//	access:
//	  allow: ["192.168.1.0/24", "10.0.0.0/8"]
//	  deny: ["192.168.1.100"]
//	  default: "deny"
//	  trusted_proxies: ["172.16.0.0/16"]
//	  geoip:
//	    enabled: true
//	    database: "/var/lib/geoip/GeoIP2-Country.mmdb"
//	    allow_countries: ["US", "JP", "GB"]
//	    deny_countries: ["CN", "RU"]
//	    default: "deny"
//	    cache_size: 10000
//	    cache_ttl: 1h
//	    private_ip_behavior: "allow"
type AccessConfig struct {
	// Allow 允许的 IP/CIDR 列表
	// 配置允许访问的 IP 地址或网段
	Allow []string `yaml:"allow"`

	// Deny 拒绝的 IP/CIDR 列表
	// 配置拒绝访问的 IP 地址或网段
	Deny []string `yaml:"deny"`

	// TrustedProxies 可信代理 CIDR 列表
	// 用于正确解析 X-Forwarded-For 头部获取真实客户端 IP
	TrustedProxies []string `yaml:"trusted_proxies"`

	// Default 默认动作
	// 未匹配任何规则时的处理方式：allow 或 deny
	Default string `yaml:"default"`

	// GeoIP GeoIP 国家代码访问控制配置
	GeoIP GeoIPConfig `yaml:"geoip"`
}

// GeoIPConfig GeoIP 访问控制配置。
//
// 通过 MaxMind GeoIP2 数据库查询 IP 所属国家，实现基于国家代码的访问控制。
//
// 注意事项：
//   - Database 为 GeoIP2 数据库文件路径（.mmdb 格式）
//   - AllowCountries 和 DenyCountries 使用 ISO 3166-1 alpha-2 国家代码
//   - CacheSize 设置 LRU 缓存最大条目数，0 表示使用默认值 10000
//   - CacheTTL 设置缓存有效期，0 表示使用默认值 1 小时
//   - PrivateIPBehavior 控制私有 IP 的处理策略
//
// 使用示例：
//
//	geoip:
//	  enabled: true
//	  database: "/var/lib/geoip/GeoIP2-Country.mmdb"
//	  allow_countries: ["US", "JP", "GB"]
//	  deny_countries: ["CN", "RU"]
//	  default: "deny"
//	  cache_size: 10000
//	  cache_ttl: 1h
//	  private_ip_behavior: "allow"
type GeoIPConfig struct {
	Database          string        `yaml:"database"`
	Default           string        `yaml:"default"`
	PrivateIPBehavior string        `yaml:"private_ip_behavior"`
	AllowCountries    []string      `yaml:"allow_countries"`
	DenyCountries     []string      `yaml:"deny_countries"`
	CacheSize         int           `yaml:"cache_size"`
	CacheTTL          time.Duration `yaml:"cache_ttl"`
	Enabled           bool          `yaml:"enabled"`
}

// RateLimitConfig 速率限制配置。
//
// 限制请求频率防止 DDoS 攻击和资源滥用。
//
// 注意事项：
//   - RequestRate 为每秒允许的最大请求数
//   - Burst 为突发流量允许的最大请求数
//   - ConnLimit 为单个 IP 的最大并发连接数
//   - Algorithm 支持 token_bucket 和 sliding_window 两种算法
//   - SlidingWindow 仅在 sliding_window 算法下生效
//
// 使用示例：
//
//	rate_limit:
//	  request_rate: 100
//	  burst: 150
//	  conn_limit: 50
//	  algorithm: "token_bucket"
//	  key: "ip"
type RateLimitConfig struct {
	Key               string `yaml:"key"`
	Algorithm         string `yaml:"algorithm"`
	SlidingWindowMode string `yaml:"sliding_window_mode"`
	RequestRate       int    `yaml:"request_rate"`
	Burst             int    `yaml:"burst"`
	ConnLimit         int    `yaml:"conn_limit"`
	SlidingWindow     int    `yaml:"sliding_window"`
}

// LimitRateConfig 响应速率限制配置。
//
// 控制响应数据的发送速率，防止单个连接占用过多带宽。
//
// 注意事项：
//   - Rate 为每秒发送的字节数，0 表示不限速
//   - Burst 为突发流量允许的字节数
//   - LargeFileThreshold 为大文件阈值，超过此大小的文件采用特殊策略
//   - LargeFileStrategy 为大文件策略：skip（跳过限速）或 coarse（粗粒度限速）
//
// 使用示例：
//
//	limit_rate:
//	  rate: 1048576        # 1MB/s
//	  burst: 524288        # 512KB 突发
//	  large_file_threshold: 10485760  # 10MB
//	  large_file_strategy: "skip"
type LimitRateConfig struct {
	// Rate 字节/秒，0 表示不限速
	Rate int64 `yaml:"rate"`

	// Burst 突发流量字节数
	Burst int64 `yaml:"burst"`

	// LargeFileThreshold 大文件阈值（字节），默认 10MB
	LargeFileThreshold int64 `yaml:"large_file_threshold"`

	// LargeFileStrategy 大文件策略：skip（跳过限速）或 coarse（粗粒度限速）
	LargeFileStrategy string `yaml:"large_file_strategy"`
}

// AuthConfig 认证配置。
//
// 配置 HTTP Basic 认证保护敏感资源。
//
// 注意事项：
//   - Type 目前仅支持 basic
//   - RequireTLS 默认为 true，强制 HTTPS 传输
//   - Algorithm 支持 bcrypt 和 argon2id
//   - Users 中 Password 字段存储的是密码哈希而非明文
//   - MinPasswordLength 控制密码最小长度要求
//
// 使用示例：
//
//	auth:
//	  type: "basic"
//	  require_tls: true
//	  algorithm: "bcrypt"
//	  realm: "Secure Area"
//	  min_password_length: 8
//	  users:
//	    - name: "admin"
//	      password: "$2y$10$..."
type AuthConfig struct {
	Type              string `yaml:"type"`
	Algorithm         string `yaml:"algorithm"`
	Realm             string `yaml:"realm"`
	Users             []User `yaml:"users"`
	MinPasswordLength int    `yaml:"min_password_length"`
	RequireTLS        bool   `yaml:"require_tls"`
}

// User 认证用户配置。
//
// 定义单个认证用户的凭据。
//
// 注意事项：
//   - Name 为用户标识，区分大小写
//   - Password 存储的是哈希值而非明文密码
//   - 支持的哈希格式取决于 Algorithm 设置
//
// 使用示例：
//
//	users:
//	  - name: "admin"
//	    password: "$2y$10$N9qo8uLOickgx2ZMRZoMy..."
type User struct {
	// Name 用户名
	// 认证时使用的用户标识
	Name string `yaml:"name"`

	// Password 密码哈希
	// bcrypt 或 argon2id 哈希值，非明文密码
	Password string `yaml:"password"`
}

// SecurityHeaders 安全头部配置。
//
// 配置 HTTP 安全响应头部增强浏览器安全。
//
// 注意事项：
//   - XFrameOptions 防止点击劫持攻击
//   - XContentTypeOptions 防止 MIME 类型嗅探
//   - ContentSecurityPolicy 控制资源加载策略
//   - ReferrerPolicy 控制 Referer 头发送策略
//   - PermissionsPolicy 控制浏览器功能权限
//
// 使用示例：
//
//	headers:
//	  x_frame_options: "DENY"
//	  x_content_type_options: "nosniff"
//	  content_security_policy: "default-src 'self'"
//	  referrer_policy: "strict-origin-when-cross-origin"
type SecurityHeaders struct {
	// XFrameOptions X-Frame-Options 头部
	// 可选值：DENY、SAMEORIGIN，防止页面被嵌入 iframe
	XFrameOptions string `yaml:"x_frame_options"`

	// XContentTypeOptions X-Content-Type-Options 头部
	// 建议值：nosniff，防止浏览器 MIME 类型嗅探
	XContentTypeOptions string `yaml:"x_content_type_options"`

	// ContentSecurityPolicy Content-Security-Policy 头部
	// 控制页面可以加载的资源来源
	ContentSecurityPolicy string `yaml:"content_security_policy"`

	// ReferrerPolicy Referrer-Policy 头部
	// 控制 Referer 头的发送策略
	ReferrerPolicy string `yaml:"referrer_policy"`

	// PermissionsPolicy Permissions-Policy 头部
	// 控制浏览器功能权限（原 Feature-Policy）
	PermissionsPolicy string `yaml:"permissions_policy"`
}

// ErrorPageConfig 自定义错误页面配置。
//
// 允许为特定 HTTP 状态码配置自定义错误页面。
// 错误页面文件在启动时预加载到内存中，运行时不进行文件 I/O。
//
// 注意事项：
//   - 错误页面文件路径可以是相对路径或绝对路径
//   - 所有错误页面加载失败时会阻止服务器启动
//   - 部分错误页面加载失败会记录警告但允许启动
//   - 支持可选的响应状态码覆盖
//
// 使用示例：
//
//	error_page:
//	  pages:
//	    404: "/var/www/errors/404.html"
//	    500: "/var/www/errors/500.html"
//	    503: "/var/www/errors/503.html"
//	  default: "/var/www/errors/error.html"
//	  response_code: 200  # 可选：覆盖响应状态码
type ErrorPageConfig struct {
	// Pages 状态码到错误页面文件的映射
	// key 为 HTTP 状态码（如 404, 500），value 为文件路径
	Pages map[int]string `yaml:"pages"`

	// Default 默认错误页面
	// 当特定状态码没有配置时使用
	Default string `yaml:"default"`

	// ResponseCode 响应状态码覆盖
	// 如果不为 0，所有错误页面响应将使用此状态码
	// 例如设置为 200 时，即使发生错误也返回 200 OK
	ResponseCode int `yaml:"response_code"`
}

// AuthRequestConfig 外部认证子请求配置。
//
// 将认证委托给外部服务，根据子请求的响应状态码决定是否允许原请求继续。
// 适用于需要复杂认证逻辑或与现有认证系统集成的场景。
//
// 行为规则：
//   - 2xx 响应：认证通过，原请求继续处理
//   - 401/403 响应：认证失败，返回相应状态码
//   - 其他响应或超时：返回 500 内部服务器错误
//   - 认证服务不可用时：返回 500 内部服务器错误
//
// 注意事项：
//   - 认证请求使用独立的连接池，避免影响主服务
//   - 支持变量展开（如 $host, $uri, $request_uri）
//   - 建议配置合理的超时时间，避免长时间阻塞
//   - 认证请求会携带原请求的头信息（如 Cookie, Authorization）
//
// 使用示例：
//
//	security:
//	  auth_request:
//	    uri: /auth
//	    method: GET
//	    auth_timeout: 5s
//	    headers:
//	      X-Original-Uri: $request_uri
//	      X-Original-Host: $host
type AuthRequestConfig struct {
	Headers        map[string]string `yaml:"headers"`
	URI            string            `yaml:"uri"`
	Method         string            `yaml:"method"`
	ForwardHeaders []string          `yaml:"forward_headers"`
	Timeout        time.Duration     `yaml:"auth_timeout"`
	Enabled        bool              `yaml:"enabled"`
}

// RewriteRule URL 重写规则。
//
// 用于在代理或静态文件服务前修改请求 URL。
//
// 注意事项：
//   - Pattern 为正则表达式，用于匹配原始 URL
//   - Replacement 为替换后的目标 URL，支持捕获组
//   - Flag 控制重写行为：last、redirect、permanent、break
//   - 规则按顺序执行，匹配后根据 Flag 决定是否继续
//
// 使用示例：
//
//	rewrite:
//	  - pattern: "^/old/(.*)$"
//	    replacement: "/new/$1"
//	    flag: "permanent"
//	  - pattern: "^/api/(.*)$"
//	    replacement: "/v1/$1"
//	    flag: "last"
type RewriteRule struct {
	// Pattern 匹配模式
	// 正则表达式，用于匹配请求 URL
	Pattern string `yaml:"pattern"`

	// Replacement 替换目标
	// 替换后的 URL 路径，支持 $1、$2 等捕获组引用
	Replacement string `yaml:"replacement"`

	// Flag 标志
	// 可选值：
	//   - last：停止后续规则匹配
	//   - redirect：返回 302 临时重定向
	//   - permanent：返回 301 永久重定向
	//   - break：停止规则匹配但继续处理
	Flag string `yaml:"flag"`
}

// CompressionConfig 响应压缩配置。
//
// 配置响应内容压缩，减少传输数据量。
//
// 注意事项：
//   - Type 支持 gzip、brotli 或 both（同时使用两种）
//   - Level 压缩级别 1-9，越高压缩率越好但 CPU 消耗越大
//   - MinSize 低于此大小的响应不压缩
//   - Types 指定哪些 MIME 类型进行压缩
//   - GzipStatic 启用后优先使用预压缩文件
//
// 使用示例：
//
//	compression:
//	  type: "gzip"
//	  level: 6
//	  min_size: 1024
//	  types: ["text/html", "text/css", "application/json"]
//	  gzip_static: true
//	  gzip_static_extensions: [".gz"]
type CompressionConfig struct {
	Type                 string   `yaml:"type"`
	Types                []string `yaml:"types"`
	GzipStaticExtensions []string `yaml:"gzip_static_extensions"`
	Level                int      `yaml:"level"`
	MinSize              int      `yaml:"min_size"`
	GzipStatic           bool     `yaml:"gzip_static"`
}

// LoggingConfig 日志配置。
//
// 配置访问日志和错误日志的输出行为。
//
// 注意事项：
//   - Format 控制日志格式：text 或 json
//   - Access 配置访问日志（记录每个请求）
//   - Error 配置错误日志（记录错误信息）
//   - Path 为空时日志输出到标准输出/标准错误
//
// 使用示例：
//
//	logging:
//	  format: "json"
//	  access:
//	    path: "/var/log/lolly/access.log"
//	    format: "combined"
//	  error:
//	    path: "/var/log/lolly/error.log"
//	    level: "warn"
type LoggingConfig struct {
	// Format 全局格式
	// 可选值：text（默认）、json
	Format string `yaml:"format"`

	// Access 访问日志配置
	Access AccessLogConfig `yaml:"access"`

	// Error 错误日志配置
	Error ErrorLogConfig `yaml:"error"`
}

// ShutdownConfig 服务器关闭配置。
//
// 用于配置服务器在接收到不同信号时的关闭超时行为。
// 优雅停止会等待正在处理的请求完成，快速停止会立即中断连接。
//
// 注意事项：
//   - graceful_timeout = 0 表示使用默认值（30s）
//   - fast_timeout = 0 表示使用默认值（5s）
//   - graceful_timeout 应显著大于 fast_timeout
//   - 两个值都必须 >= 0，负数在验证时会报错
//
// 使用示例：
//
//	shutdown:
//	  graceful_timeout: 30s   # SIGQUIT 优雅停止超时
//	  fast_timeout: 5s        # SIGINT/SIGTERM 快速停止超时
type ShutdownConfig struct {
	// GracefulTimeout 优雅停止超时（SIGQUIT）
	// 接收到 SIGQUIT 信号后，等待活跃请求完成的最大时间
	// 默认: 30s（当值为 0 时使用默认值）
	GracefulTimeout time.Duration `yaml:"graceful_timeout"`

	// FastTimeout 快速停止超时（SIGINT/SIGTERM）
	// 接收到 SIGINT 或 SIGTERM 信号后，等待服务器关闭的最大时间
	// 默认: 5s（当值为 0 时使用默认值）
	FastTimeout time.Duration `yaml:"fast_timeout"`
}

// AccessLogConfig 访问日志配置。
//
// 配置访问日志的输出位置和格式。
//
// 注意事项：
//   - Path 为日志文件路径，为空则输出到 stdout
//   - Format 支持预设格式或自定义格式
//   - 常用预设格式：common、combined
//
// 使用示例：
//
//	access:
//	  path: "/var/log/lolly/access.log"
//	  format: "combined"
type AccessLogConfig struct {
	// Path 日志文件路径
	// 访问日志的输出文件，为空则输出到标准输出
	Path string `yaml:"path"`

	// Format 日志格式
	// 预设格式或自定义日志格式字符串
	Format string `yaml:"format"`
}

// ErrorLogConfig 错误日志配置。
//
// 配置错误日志的输出位置和级别。
//
// 注意事项：
//   - Path 为日志文件路径，为空则输出到 stderr
//   - Level 控制记录的日志级别阈值
//   - 可选级别：debug、info、warn、error
//
// 使用示例：
//
//	error:
//	  path: "/var/log/lolly/error.log"
//	  level: "error"
type ErrorLogConfig struct {
	// Path 日志文件路径
	// 错误日志的输出文件，为空则输出到标准错误
	Path string `yaml:"path"`

	// Level 日志级别
	// 可选值：debug、info、warn、error
	Level string `yaml:"level"`
}

// PerformanceConfig 性能配置。
//
// 配置服务器性能优化相关参数。
//
// 注意事项：
//   - GoroutinePool 复用 goroutine 减少创建开销
//   - FileCache 缓存静态文件内容提升响应速度
//   - Transport 配置代理连接的连接池参数
//
// 使用示例：
//
//	performance:
//	  goroutine_pool:
//	    enabled: true
//	    max_workers: 1000
//	  file_cache:
//	    max_entries: 10000
//	    max_size: 1073741824
//	  transport:
//	    max_idle_conns: 100
type PerformanceConfig struct {
	// GoroutinePool Goroutine 池配置
	// 控制 worker goroutine 的复用行为
	GoroutinePool GoroutinePoolConfig `yaml:"goroutine_pool"`

	// FileCache 文件缓存配置
	// 缓存静态文件内容避免重复磁盘 IO
	FileCache FileCacheConfig `yaml:"file_cache"`

	// Transport HTTP Transport 配置
	// 代理连接池的参数设置
	Transport TransportConfig `yaml:"transport"`
}

// GoroutinePoolConfig Goroutine 池配置。
//
// 复用 goroutine 减少创建和销毁开销。
//
// 注意事项：
//   - Enabled 为 true 时启用 goroutine 池
//   - MaxWorkers 限制最大并发 worker 数
//   - MinWorkers 预热 worker 数量
//   - IdleTimeout 空闲 worker 回收时间
//
// 使用示例：
//
//	goroutine_pool:
//	  enabled: true
//	  max_workers: 1000
//	  min_workers: 100
//	  idle_timeout: 60s
type GoroutinePoolConfig struct {
	// Enabled 是否启用
	Enabled bool `yaml:"enabled"`

	// MaxWorkers 最大 worker 数
	// 限制同时运行的最大 goroutine 数量
	MaxWorkers int `yaml:"max_workers"`

	// MinWorkers 最小 worker 数（预热）
	// 启动时预创建的 goroutine 数量
	MinWorkers int `yaml:"min_workers"`

	// IdleTimeout 空闲超时
	// 空闲 worker 超过此时间将被回收
	IdleTimeout time.Duration `yaml:"idle_timeout"`
}

// FileCacheConfig 文件缓存配置。
//
// 缓存静态文件内容减少磁盘 IO。
//
// 注意事项：
//   - MaxEntries 限制最大缓存文件数量
//   - MaxSize 限制缓存总内存使用量（字节）
//   - Inactive 超过此时间未访问的文件将被淘汰
//
// 使用示例：
//
//	file_cache:
//	  max_entries: 10000
//	  max_size: 1073741824
//	  inactive: 60s
type FileCacheConfig struct {
	// MaxEntries 最大缓存条目数
	// 缓存文件的最大数量限制
	MaxEntries int64 `yaml:"max_entries"`

	// MaxSize 内存上限（字节）
	// 缓存占用的最大内存限制
	MaxSize int64 `yaml:"max_size"`

	// Inactive 未访问淘汰时间
	// 超过此时间未被访问的缓存将被清除
	Inactive time.Duration `yaml:"inactive"`
}

// TransportConfig HTTP Transport 配置。
//
// 配置代理后端连接的连接池参数。
//
// 注意事项：
//   - IdleConnTimeout 控制空闲连接的保持时间
//   - MaxConnsPerHost 限制每个后端主机的总连接数（含活跃和空闲）
//
// 使用示例：
//
//	transport:
//	  idle_conn_timeout: 90s
//	  max_conns_per_host: 100
type TransportConfig struct {
	// IdleConnTimeout 空闲连接超时
	// 空闲连接的最大存活时间
	IdleConnTimeout time.Duration `yaml:"idle_conn_timeout"`

	// MaxConnsPerHost 每主机最大连接数
	// 单个后端主机的总连接数上限（包括活跃连接和空闲连接）
	MaxConnsPerHost int `yaml:"max_conns_per_host"`
}

// MonitoringConfig 监控配置。
//
// 配置服务状态监控和健康检查端点。
//
// 注意事项：
//   - Status 配置状态检查端点
//   - 监控端点建议限制访问 IP 防止信息泄露
//
// 使用示例：
//
//	monitoring:
//	  status:
//	    path: "/status"
//	    allow: ["127.0.0.1", "10.0.0.0/8"]
type MonitoringConfig struct {
	// Status 状态端点配置
	// 服务健康状态检查端点
	Status StatusConfig `yaml:"status"`

	// Pprof pprof 性能分析端点配置
	// 用于收集 CPU、内存等性能数据，支持 PGO 优化
	Pprof PprofConfig `yaml:"pprof"`
}

// PprofConfig pprof 性能分析端点配置。
//
// 配置 pprof 端点用于收集运行时性能数据。
// 收集的 profile 可用于 PGO (Profile-Guided Optimization) 构建。
//
// 注意事项：
//   - 生产环境仅在收集 profile 时启用，完成后关闭
//   - 建议严格限制访问 IP，防止性能数据泄露
//   - CPU profile 收集需要代表性 workload
//
// 使用示例：
//
//	pprof:
//	  enabled: true
//	  path: "/debug/pprof"
//	  allow: ["127.0.0.1"]
type PprofConfig struct {
	Path    string   `yaml:"path"`
	Allow   []string `yaml:"allow"`
	Enabled bool     `yaml:"enabled"`
}

// StatusConfig 状态监控端点配置。
//
// 配置服务状态检查端点的路径和访问控制。
//
// 注意事项：
//   - Enabled 默认为 false，需显式启用
//   - Path 为状态端点的 URL 路径
//   - Format 支持 json、text、html、prometheus 格式
//   - Allow 限制可访问的 IP 地址列表
//   - 生产环境建议严格限制访问来源
//
// 使用示例：
//
//	status:
//	  enabled: true
//	  path: "/_status"
//	  format: "json"
//	  allow: ["127.0.0.1", "192.168.0.0/16"]
type StatusConfig struct {
	Path    string   `yaml:"path"`
	Format  string   `yaml:"format"`
	Allow   []string `yaml:"allow"`
	Enabled bool     `yaml:"enabled"`
}

// CacheAPIConfig 缓存 API 配置。
//
// 配置缓存清理 API 端点，支持主动清理代理缓存。
//
// 注意事项：
//   - Enabled 默认为 false，需显式启用
//   - Allow 限制可访问的 IP 地址列表
//   - Auth 配置认证方式，推荐使用 token 认证
//
// 使用示例：
//
//	cache_api:
//	  enabled: true
//	  path: "/_cache/purge"
//	  allow: ["127.0.0.1", "10.0.0.0/8"]
//	  auth:
//	    type: "token"
//	    token: "${CACHE_API_TOKEN}"
type CacheAPIConfig struct {
	Auth    CacheAPIAuthConfig `yaml:"auth"`
	Path    string             `yaml:"path"`
	Allow   []string           `yaml:"allow"`
	Enabled bool               `yaml:"enabled"`
}

// CacheAPIAuthConfig 缓存 API 认证配置。
type CacheAPIAuthConfig struct {
	// Type 认证类型
	// 支持 "none" 和 "token" 两种类型
	// 默认为 "none"
	Type string `yaml:"type"`

	// Token 认证令牌
	// 当 Type 为 "token" 时使用
	// 支持环境变量替换，如 "${CACHE_API_TOKEN}"
	Token string `yaml:"token"`
}

// LuaMiddlewareConfig Lua 中间件配置（配置文件格式）
//
// 用于配置 Lua 中间件的行为，包括脚本路径、执行阶段和全局设置。
//
// 注意事项：
//   - Enabled 为 true 时启用 Lua 中间件
//   - Scripts 配置要执行的脚本列表
//   - GlobalSettings 控制 Lua 引擎的全局行为
//
// 使用示例：
//
//	lua:
//	  enabled: true
//	  scripts:
//	    - path: "/scripts/auth.lua"
//	      phase: "access"
//	      timeout: 10s
//	  global_settings:
//	    max_concurrent_coroutines: 1000
//	    coroutine_timeout: 30s
type LuaMiddlewareConfig struct {
	Scripts        []LuaScriptConfig `yaml:"scripts"`
	GlobalSettings LuaGlobalSettings `yaml:"global_settings"`
	Enabled        bool              `yaml:"enabled"`
}

// LuaScriptConfig 单个脚本配置
//
// 定义单个 Lua 脚本的执行参数。
//
// 注意事项：
//   - Path 为脚本文件路径，必需字段
//   - Phase 为执行阶段，必需字段
//   - Timeout 控制脚本执行超时
//
// 使用示例：
//
//	scripts:
//	  - path: "/scripts/auth.lua"
//	    phase: "access"
//	    timeout: 10s
//	    enabled: true
type LuaScriptConfig struct {
	// Path 脚本路径
	Path string `yaml:"path"`

	// Phase 执行阶段
	// 可选值：rewrite、access、content、log、header_filter、body_filter
	Phase string `yaml:"phase"`

	// Timeout 执行超时
	Timeout time.Duration `yaml:"timeout"`

	// Enabled 是否启用此脚本（默认 true）
	Enabled bool `yaml:"enabled"`
}

// LuaGlobalSettings 全局 Lua 设置
//
// 控制 Lua 引擎的全局行为。
//
// 注意事项：
//   - MaxConcurrentCoroutines 控制最大并发协程数
//   - CoroutineTimeout 控制协程执行超时
//   - CodeCacheSize 控制字节码缓存大小
//   - CoroutineStackSize 控制协程栈大小（默认64）
//   - MinimizeStackMemory 启用栈内存自动收缩
//   - CoroutinePoolWarmup 协程池预热数量
//
// 使用示例：
//
//	global_settings:
//	  max_concurrent_coroutines: 1000
//	  coroutine_timeout: 30s
//	  code_cache_size: 1000
//	  enable_file_watch: true
//	  max_execution_time: 30s
//	  coroutine_stack_size: 64
//	  minimize_stack_memory: true
//	  coroutine_pool_warmup: 4
type LuaGlobalSettings struct {
	// MaxConcurrentCoroutines 最大并发协程数
	MaxConcurrentCoroutines int `yaml:"max_concurrent_coroutines"`

	// CoroutineTimeout 协程执行超时
	CoroutineTimeout time.Duration `yaml:"coroutine_timeout"`

	// CodeCacheSize 字节码缓存条目数
	CodeCacheSize int `yaml:"code_cache_size"`

	// MaxExecutionTime 单脚本最大执行时间
	MaxExecutionTime time.Duration `yaml:"max_execution_time"`

	// CoroutineStackSize 协程栈大小（默认64，最大256）
	// 较小的栈减少内存分配，适用于简单脚本
	CoroutineStackSize int `yaml:"coroutine_stack_size"`

	// CoroutinePoolWarmup 协程池预热数量，启动时预创建
	CoroutinePoolWarmup int `yaml:"coroutine_pool_warmup"`

	// EnableFileWatch 启用文件变更检测
	EnableFileWatch bool `yaml:"enable_file_watch"`

	// MinimizeStackMemory 启用栈内存自动收缩以减少内存占用
	MinimizeStackMemory bool `yaml:"minimize_stack_memory"`
}

// StreamConfig TCP/UDP Stream 代理配置。
//
// 用于四层网络代理，如数据库、Redis 等 TCP/UDP 服务。
//
// 注意事项：
//   - Listen 配置监听地址
//   - Protocol 支持 tcp 或 udp
//   - Upstream 配置后端目标列表
//   - Stream 代理工作在传输层，不解析应用层协议
//
// 使用示例：
//
//	stream:
//	  - listen: ":3306"
//	    protocol: "tcp"
//	    upstream:
//	      targets:
//	        - addr: "mysql1:3306"
//	          weight: 3
//	        - addr: "mysql2:3306"
//	          weight: 1
//	      load_balance: "round_robin"
type StreamConfig struct {
	Listen   string               `yaml:"listen"`
	Protocol string               `yaml:"protocol"`
	Upstream StreamUpstream       `yaml:"upstream"`
	ProxySSL StreamProxySSLConfig `yaml:"proxy_ssl"`
	SSL      StreamSSLConfig      `yaml:"ssl"`
}

// StreamUpstream Stream 上游配置。
//
// 配置 Stream 代理的后端服务器列表。
//
// 注意事项：
//   - Targets 配置后端服务器地址
//   - LoadBalance 配置负载均衡算法
//
// 使用示例：
//
//	upstream:
//	  targets:
//	    - addr: "backend1:3306"
//	      weight: 3
//	  load_balance: "round_robin"
type StreamUpstream struct {
	LoadBalance string         `yaml:"load_balance"`
	Targets     []StreamTarget `yaml:"targets"`
}

// StreamTarget Stream 目标配置。
//
// 定义单个 Stream 后端服务器。
//
// 注意事项：
//   - Addr 为后端服务器地址
//   - Weight 在加权轮询算法下生效
//
// 使用示例：
//
//	targets:
//	  - addr: "mysql1:3306"
//	    weight: 3
//	  - addr: "mysql2:3306"
//	    weight: 1
type StreamTarget struct {
	// Addr 目标地址
	// 后端服务器地址，如 "host:port"
	Addr string `yaml:"addr"`

	// Weight 权重
	// 用于加权轮询负载均衡
	Weight int `yaml:"weight"`
}

// StreamSSLConfig Stream SSL 服务端配置。
//
// 配置 Stream 模块的 TLS 终端功能，用于加密 TCP 流量。
//
// 注意事项：
//   - 仅对 TCP 协议有效，UDP 不支持 TLS
//   - 证书文件需要 PEM 格式
//   - 支持配置客户端证书验证（mTLS）
//
// 使用示例：
//
//	stream:
//	  - listen: ":3306"
//	    protocol: "tcp"
//	    ssl:
//	      enabled: true
//	      cert: "/etc/ssl/server.crt"
//	      key: "/etc/ssl/server.key"
//	    upstream:
//	      targets:
//	        - addr: "mysql:3306"
type StreamSSLConfig struct {
	Cert        string   `yaml:"cert"`
	Key         string   `yaml:"key"`
	ClientCA    string   `yaml:"client_ca"`
	Protocols   []string `yaml:"protocols"`
	Ciphers     []string `yaml:"ciphers"`
	VerifyDepth int      `yaml:"verify_depth"`
	Enabled     bool     `yaml:"enabled"`
}

// StreamProxySSLConfig Stream 上游 SSL 配置。
//
// 配置到上游服务器的 TLS 连接，用于加密代理到后端的流量。
//
// 注意事项：
//   - 启用后，代理将使用 TLS 连接到上游
//   - 支持客户端证书（mTLS）和服务器证书验证
//   - ServerName 用于 SNI 和证书验证
//
// 使用示例：
//
//	stream:
//	  - listen: ":3306"
//	    protocol: "tcp"
//	    proxy_ssl:
//	      enabled: true
//	      verify: true
//	      trusted_ca: "/etc/ssl/ca.crt"
//	      server_name: "mysql.internal"
//	    upstream:
//	      targets:
//	        - addr: "mysql:3306"
type StreamProxySSLConfig struct {
	TrustedCA    string   `yaml:"trusted_ca"`
	ServerName   string   `yaml:"server_name"`
	Cert         string   `yaml:"cert"`
	Key          string   `yaml:"key"`
	Protocols    []string `yaml:"protocols"`
	Enabled      bool     `yaml:"enabled"`
	Verify       bool     `yaml:"verify"`
	SessionReuse bool     `yaml:"session_reuse"`
}

// TypesConfig MIME 类型配置
//
// 用于配置静态文件的 MIME 类型映射。
//
// 注意事项：
//   - DefaultType 为默认 MIME 类型
//   - Map 为扩展名到 MIME 类型的映射
//
// 使用示例：
//
//	types:
//	  default_type: "application/octet-stream"
//	  map:
//	    ".html": "text/html"
//	    ".css": "text/css"
//	    ".js": "application/javascript"
type TypesConfig struct {
	// DefaultType 默认 MIME 类型
	// 当无法识别文件扩展名时使用
	DefaultType string `yaml:"default_type"`

	// Map 扩展名到 MIME 类型的映射
	// 键为文件扩展名（如 ".html"），值为 MIME 类型
	Map map[string]string `yaml:"map"`
}

// UnixSocketConfig Unix socket 特定配置。
//
// 用于配置服务器监听 Unix domain socket 时的文件权限和所有权。
//
// 注意事项：
//   - Mode 为 socket 文件权限，默认 0666
//   - User 为 socket 文件所有者用户名
//   - Group 为 socket 文件所属用户组
//
// 使用示例：
//
//	unix_socket:
//	  mode: 0660
//	  user: "www-data"
//	  group: "www-data"
type UnixSocketConfig struct {
	// Mode 文件权限
	// Unix socket 文件的访问权限，默认 0666
	Mode int `yaml:"mode"`

	// User 文件所有者
	// Unix socket 文件的所有者用户名
	User string `yaml:"user"`

	// Group 文件组
	// Unix socket 文件的所属用户组
	Group string `yaml:"group"`
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

	if err := os.WriteFile(path, data, 0o644); err != nil {
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

// GetDefaultServerFromList 从 servers 列表中获取默认服务器配置。
//
// 遍历 servers 列表，返回第一个 Default 标记为 true 的服务器。
// 用于在虚拟主机模式下获取默认服务器的配置作为 fallback。
//
// 返回值：
//   - *ServerConfig: 默认服务器配置，如无则返回 nil
func (c *Config) GetDefaultServerFromList() *ServerConfig {
	for i := range c.Servers {
		if c.Servers[i].Default {
			return &c.Servers[i]
		}
	}
	return nil
}

// GetMode 获取服务器运行模式。
//
// 如果 Mode 显式设置（非 auto），返回设置的值。
// 如果 Mode 是 auto 或未设置，根据配置自动推断：
//   - servers 数量 == 1 → single
//   - servers 数量 > 1 且所有 listen 地址相同 → vhost
//   - servers 数量 > 1 且 listen 地址不同 → multi_server
//
// 返回值：
//   - ServerMode: 推断后的服务器运行模式
func (c *Config) GetMode() ServerMode {
	// 如果显式设置了非 auto 模式，直接返回
	if c.Mode != "" && c.Mode != ServerModeAuto {
		return c.Mode
	}

	// 自动推断模式
	serverCount := len(c.Servers)

	// servers 为空 → auto（配置验证会确保至少有一个服务器）
	if serverCount == 0 {
		return ServerModeAuto
	}

	// servers 数量 == 1 → single
	if serverCount == 1 {
		return ServerModeSingle
	}

	// servers 数量 > 1，检查 listen 地址
	firstListen := c.Servers[0].Listen
	allSameListen := true
	for i := 1; i < serverCount; i++ {
		if c.Servers[i].Listen != firstListen {
			allSameListen = false
			break
		}
	}

	// 所有 listen 地址相同 → vhost，否则 → multi_server
	if allSameListen {
		return ServerModeVHost
	}
	return ServerModeMultiServer
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
//   - 必须配置 servers 数组且至少包含一个服务器
//   - 所有服务器配置必须通过 validateServer 验证
func Validate(cfg *Config) error {
	// 必须配置 servers 且至少包含一个服务器
	if !cfg.HasServers() {
		return errors.New("必须配置 servers 且至少包含一个服务器")
	}

	// 验证模式
	if err := validateMode(cfg.Mode); err != nil {
		return err
	}

	// 验证监听地址冲突（multi_server 模式）
	if err := validateListenConflicts(cfg.Servers, cfg.GetMode()); err != nil {
		return err
	}

	// 验证 default 服务器唯一性
	if err := validateDefaultServer(cfg.Servers); err != nil {
		return err
	}

	// 验证所有服务器
	for i := range cfg.Servers {
		if err := validateServer(&cfg.Servers[i], false); err != nil {
			return fmt.Errorf("servers[%d]: %w", i, err)
		}
	}

	// 验证 Stream 配置
	for i := range cfg.Stream {
		if err := validateStream(&cfg.Stream[i]); err != nil {
			return fmt.Errorf("stream[%d]: %w", i, err)
		}
	}

	// 验证日志配置
	if err := validateLogging(&cfg.Logging); err != nil {
		return err
	}

	// 验证性能配置
	if err := validatePerformance(&cfg.Performance); err != nil {
		return fmt.Errorf("performance: %w", err)
	}

	// 验证 Resolver 配置
	if err := cfg.Resolver.Validate(); err != nil {
		return fmt.Errorf("resolver: %w", err)
	}

	// 验证变量配置
	if err := validateVariables(&cfg.Variables); err != nil {
		return fmt.Errorf("variables: %w", err)
	}

	// 验证关闭配置
	if err := validateShutdown(&cfg.Shutdown); err != nil {
		return err
	}

	return nil
}

// validateShutdown 验证关闭配置。
func validateShutdown(cfg *ShutdownConfig) error {
	if cfg.GracefulTimeout < 0 {
		return errors.New("shutdown.graceful_timeout 不能为负数")
	}
	if cfg.FastTimeout < 0 {
		return errors.New("shutdown.fast_timeout 不能为负数")
	}
	// 0 值表示使用默认值，在应用层处理
	return nil
}

// ResolverConfig DNS 解析器配置。
//
// 配置 DNS 解析器的行为，包括服务器地址、缓存 TTL、超时等。
// 启用后可实现动态 DNS 解析和缓存，支持后端域名的动态解析。
//
// 注意事项：
//   - Enabled 为 true 时启用 DNS 解析器
//   - Addresses 配置 DNS 服务器地址，如 "8.8.8.8:53"
//   - Valid 为缓存有效期（TTL），建议 30s-300s
//   - Timeout 为单次查询超时时间
//
// 使用示例：
//
//	resolver:
//	  enabled: true
//	  addresses:
//	    - "8.8.8.8:53"
//	    - "8.8.4.4:53"
//	  valid: 30s
//	  timeout: 5s
//	  ipv4: true
//	  ipv6: false
//	  cache_size: 1024
type ResolverConfig struct {
	Addresses []string      `yaml:"addresses"`
	Valid     time.Duration `yaml:"valid"`
	Timeout   time.Duration `yaml:"timeout"`
	CacheSize int           `yaml:"cache_size"`
	Enabled   bool          `yaml:"enabled"`
	IPv4      bool          `yaml:"ipv4"`
	IPv6      bool          `yaml:"ipv6"`
}

// TTL 返回缓存有效期（Valid 的别名，便于代码理解）。
func (c *ResolverConfig) TTL() time.Duration {
	return c.Valid
}

// Validate 验证 Resolver 配置。
//
// 检查 DNS 服务器地址格式、TTL 和超时设置的有效性。
//
// 返回值：
//   - error: 验证失败时的错误信息
func (c *ResolverConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	if len(c.Addresses) == 0 {
		return errors.New("resolver.addresses is required when enabled")
	}

	for _, addr := range c.Addresses {
		if _, err := net.ResolveUDPAddr("udp", addr); err != nil {
			return fmt.Errorf("invalid DNS address %s: %w", addr, err)
		}
	}

	if c.Valid > 0 && c.Valid < time.Second {
		return errors.New("resolver.valid must be at least 1s")
	}

	if c.Timeout > 0 && c.Timeout < time.Second {
		return errors.New("resolver.timeout must be at least 1s")
	}

	if !c.IPv4 && !c.IPv6 {
		return errors.New("at least one of ipv4 or ipv6 must be enabled")
	}

	return nil
}
