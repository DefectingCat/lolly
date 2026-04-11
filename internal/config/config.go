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
	"time"

	"gopkg.in/yaml.v3"
)

// 默认配置常量。
const (
	// DefaultPprofPath pprof 端点的默认路径。
	DefaultPprofPath = "/debug/pprof"
)

// Config 根配置结构，支持单服务器和多虚拟主机两种模式。
//
// 包含服务器配置、日志配置、性能配置和监控配置等模块。
// 是配置文件的顶级结构体，所有其他配置都作为其子结构。
//
// 注意事项：
//   - 必须配置 server 或 servers 中的至少一个
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
//	server := cfg.Server
//	// 或使用多虚拟主机
//	for _, s := range cfg.Servers {
//	    // 处理每个服务器配置
//	}
type Config struct {
	// Server 单服务器模式配置
	// 用于单一服务监听场景，与 Servers 二选一配置
	Server ServerConfig `yaml:"server"`

	// Servers 多虚拟主机模式配置
	// 用于同时监听多个地址或提供不同服务场景
	Servers []ServerConfig `yaml:"servers"`

	// Stream TCP/UDP Stream 代理配置
	// 用于四层网络代理，如数据库、缓存等 TCP 服务
	Stream []StreamConfig `yaml:"stream"`

	// HTTP3 HTTP/3 (QUIC) 配置
	// 启用 HTTP/3 协议支持，需要配合 SSL 配置使用
	HTTP3 HTTP3Config `yaml:"http3"`

	// Logging 日志配置
	// 控制访问日志和错误日志的输出格式与位置
	Logging LoggingConfig `yaml:"logging"`

	// Performance 性能配置
	// 包含 Goroutine 池、文件缓存、连接池等性能优化选项
	Performance PerformanceConfig `yaml:"performance"`

	// Monitoring 监控配置
	// 包含状态端点等监控相关配置
	Monitoring MonitoringConfig `yaml:"monitoring"`

	// Resolver DNS 解析器配置
	// 启用动态 DNS 解析和缓存
	Resolver ResolverConfig `yaml:"resolver"`

	// Variables 自定义变量配置
	// 全局变量定义，应用于所有虚拟主机
	Variables VariablesConfig `yaml:"variables"`
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
	// Enabled 是否启用 HTTP/2
	// 默认为 true，但仅在配置了 SSL 时生效
	Enabled bool `yaml:"enabled"`

	// MaxConcurrentStreams 最大并发流
	// 控制单个连接允许的最大并发流数量，默认 128
	MaxConcurrentStreams int `yaml:"max_concurrent_streams"`

	// MaxHeaderListSize 最大头部列表大小（字节）
	// 限制请求和响应头部的大小，默认 1MB (1048576)
	MaxHeaderListSize int `yaml:"max_header_list_size"`

	// IdleTimeout 空闲超时
	// 连接无活动时的最大保持时间，默认 120s
	IdleTimeout time.Duration `yaml:"idle_timeout"`

	// PushEnabled 是否启用 Server Push
	// 默认 false
	PushEnabled bool `yaml:"push_enabled"`

	// H2CEnabled 是否启用 H2C（明文 HTTP/2）
	// 默认 false，需要 Enabled 为 true 才生效
	H2CEnabled bool `yaml:"h2c_enabled"`
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
	// Enabled 是否启用 HTTP/3
	Enabled bool `yaml:"enabled"`

	// Listen UDP 监听地址，如 ":443"
	// 通常与 HTTPS 端口一致
	Listen string `yaml:"listen"`

	// MaxStreams 最大并发流
	// 控制单个连接允许的最大并发流数量
	MaxStreams int `yaml:"max_streams"`

	// IdleTimeout 空闲超时
	// 连接无活动时的最大保持时间
	IdleTimeout time.Duration `yaml:"idle_timeout"`

	// Enable0RTT 启用 0-RTT 特性
	// 允许在首次握手时发送数据，降低延迟但可能存在安全风险
	Enable0RTT bool `yaml:"enable_0rtt"`
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
	// Listen 监听地址，如 ":8080" 或 "127.0.0.1:8080"
	// 必填字段，决定服务器在哪个地址和端口接收请求
	Listen string `yaml:"listen"`

	// Name 服务器名称，用于虚拟主机匹配
	// 多个服务器可通过 Name 区分不同域名或服务
	Name string `yaml:"name"`

	// Static 静态文件服务配置列表
	// 支持多个静态目录，按 path 前缀匹配
	Static []StaticConfig `yaml:"static"`

	// Proxy 反向代理规则列表
	// 按顺序匹配，首个匹配的规则生效
	Proxy []ProxyConfig `yaml:"proxy"`

	// SSL SSL/TLS 配置
	// HTTPS 必需配置，包含证书和加密设置
	SSL SSLConfig `yaml:"ssl"`

	// Security 安全配置
	// 包含访问控制、限流、认证等安全功能
	Security SecurityConfig `yaml:"security"`

	// Rewrite URL 重写规则
	// 在代理或静态文件服务前执行 URL 转换
	Rewrite []RewriteRule `yaml:"rewrite"`

	// Compression 响应压缩配置
	// 控制 gzip/brotli 压缩行为
	Compression CompressionConfig `yaml:"compression"`

	// ReadTimeout 读取超时
	// 读取完整请求（包括 body）的最大时间
	ReadTimeout time.Duration `yaml:"read_timeout"`

	// WriteTimeout 写入超时
	// 写入响应的最大时间
	WriteTimeout time.Duration `yaml:"write_timeout"`

	// IdleTimeout 空闲超时
	// Keep-Alive 连接的最大空闲时间
	IdleTimeout time.Duration `yaml:"idle_timeout"`

	// MaxConnsPerIP 每 IP 最大连接数
	// 防止单个 IP 占用过多连接资源
	MaxConnsPerIP int `yaml:"max_conns_per_ip"`

	// MaxRequestsPerConn 每连接最大请求数
	// 达到后连接将被优雅关闭
	MaxRequestsPerConn int `yaml:"max_requests_per_conn"`

	// ClientMaxBodySize 客户端请求体大小限制
	// 限制请求体最大字节数，超过返回 413 错误
	// 支持单位：b, kb, mb, gb 或纯数字表示字节
	// 默认值为 1MB
	ClientMaxBodySize string `yaml:"client_max_body_size"`

	// CacheAPI 缓存 API 配置
	// 用于主动清理代理缓存
	CacheAPI *CacheAPIConfig `yaml:"cache_api"`

	// Lua Lua 中间件配置
	// 用于嵌入 Lua 脚本处理请求
	Lua *LuaMiddlewareConfig `yaml:"lua"`
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
}

// ProxyConfig 反向代理配置，支持负载均衡和健康检查。
//
// 用于将请求转发到后端服务器，支持多种负载均衡算法
// 和健康检查机制。
//
// 注意事项：
//   - Path 使用前缀匹配，较长路径优先匹配
//   - 至少配置一个 Target 才能正常工作
//   - 负载均衡算法支持：round_robin、weighted_round_robin、least_conn、ip_hash、consistent_hash
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
	// Path 匹配路径前缀
	// 以此前缀开头的请求将被转发到该代理
	Path string `yaml:"path"`

	// Targets 后端目标列表
	// 支持配置多个后端服务器实现负载均衡
	Targets []ProxyTarget `yaml:"targets"`

	// LoadBalance 负载均衡算法
	// 可选值：round_robin、weighted_round_robin、least_conn、ip_hash、consistent_hash
	LoadBalance string `yaml:"load_balance"`

	// HashKey 一致性哈希键
	// 可选值：ip、uri、header:X-Name
	HashKey string `yaml:"hash_key"`

	// VirtualNodes 一致性哈希虚拟节点数
	// 影响哈希分布的均匀性，默认为 150
	VirtualNodes int `yaml:"virtual_nodes"`

	// HealthCheck 健康检查配置
	// 定期检查后端服务健康状态
	HealthCheck HealthCheckConfig `yaml:"health_check"`

	// Timeout 超时配置
	// 控制代理连接和读写超时
	Timeout ProxyTimeout `yaml:"timeout"`

	// Headers 请求/响应头修改
	// 可以在转发前后添加、修改或删除 HTTP 头
	Headers ProxyHeaders `yaml:"headers"`

	// Cache 代理缓存配置
	// 启用后缓存后端响应减少重复请求
	Cache ProxyCacheConfig `yaml:"cache"`

	// ClientMaxBodySize 请求体大小限制
	// 限制此代理路径的请求体最大字节数，覆盖全局配置
	// 支持单位：b, kb, mb, gb 或纯数字表示字节
	ClientMaxBodySize string `yaml:"client_max_body_size"`

	// NextUpstream 故障转移配置
	// 配置后端故障时的自动重试行为
	NextUpstream NextUpstreamConfig `yaml:"next_upstream"`
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
	// Interval 检查间隔
	// 每次健康检查之间的时间间隔
	Interval time.Duration `yaml:"interval"`

	// Path 健康检查路径
	// 发送 HTTP GET 请求的路径
	Path string `yaml:"path"`

	// Timeout 检查超时时间
	// 超过此时间未响应视为不健康
	Timeout time.Duration `yaml:"timeout"`
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
	// Enabled 是否启用缓存
	Enabled bool `yaml:"enabled"`

	// MaxAge 缓存有效期
	// 缓存内容的最大存活时间
	MaxAge time.Duration `yaml:"max_age"`

	// CacheLock 缓存锁
	// 防止缓存击穿，同一资源同时只有一个后端请求
	CacheLock bool `yaml:"cache_lock"`

	// StaleWhileRevalidate 过期缓存复用时间
	// 在重新验证期间返回过期缓存的最大时间
	StaleWhileRevalidate time.Duration `yaml:"stale_while_revalidate"`
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
	// Tries 最大尝试次数
	// 包括第一次尝试在内的总请求次数
	Tries int `yaml:"tries"`

	// HTTPCodes 触发重试的 HTTP 状态码列表
	// 后端返回这些状态码时自动尝试下一个
	HTTPCodes []int `yaml:"http_codes"`
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
	// Cert 证书文件路径
	// PEM 格式的服务器证书文件
	Cert string `yaml:"cert"`

	// Key 私钥文件路径
	// PEM 格式的私钥文件
	Key string `yaml:"key"`

	// CertChain 证书链文件路径
	// 可选，包含中间证书以支持完整证书链验证
	CertChain string `yaml:"cert_chain"`

	// Protocols TLS 版本列表
	// 默认 ["TLSv1.2", "TLSv1.3"]
	Protocols []string `yaml:"protocols"`

	// Ciphers 加密套件列表
	// 仅对 TLS 1.2 有效，建议使用默认值
	Ciphers []string `yaml:"ciphers"`

	// OCSPStapling OCSP Stapling 支持
	// 启用后可在 TLS 握手时提供证书状态信息
	OCSPStapling bool `yaml:"ocsp_stapling"`

	// HSTS HSTS 配置
	// HTTP Strict Transport Security 安全策略
	HSTS HSTSConfig `yaml:"hsts"`

	// SessionTickets Session Tickets 配置
	// 启用 TLS 1.3 会话恢复以提升握手性能
	SessionTickets SessionTicketsConfig `yaml:"session_tickets"`

	// HTTP2 HTTP/2 配置
	// 启用 HTTP/2 支持，仅在配置了 SSL/TLS 时生效
	HTTP2 HTTP2Config `yaml:"http2"`

	// ClientVerify 客户端证书验证配置
	// 启用 mTLS 双向认证
	ClientVerify ClientVerifyConfig `yaml:"client_verify"`
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
	// Enabled 是否启用 Session Tickets
	Enabled bool `yaml:"enabled"`

	// KeyFile 密钥存储文件路径
	// 用于持久化密钥，确保重启后旧票据仍可解密
	KeyFile string `yaml:"key_file"`

	// RotateInterval 密钥轮换间隔
	// 定期生成新密钥，增强安全性
	RotateInterval time.Duration `yaml:"rotate_interval"`

	// RetainKeys 保留的历史密钥数量
	// 旧密钥用于解密已发放的票据，建议 3-5 个
	RetainKeys int `yaml:"retain_keys"`
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
	// Enabled 是否启用客户端证书验证
	Enabled bool `yaml:"enabled"`

	// Mode 验证模式
	// 可选值：
	//   - none: 不请求客户端证书（默认）
	//   - request: 请求证书但不验证
	//   - require: 要求并验证客户端证书
	//   - optional_no_ca: 请求证书但不强制验证
	Mode string `yaml:"mode"`

	// ClientCA 客户端 CA 证书文件路径
	// 用于验证客户端证书的信任链
	ClientCA string `yaml:"client_ca"`

	// VerifyDepth 证书链验证深度
	// 限制证书链的最大层数，默认 1
	VerifyDepth int `yaml:"verify_depth"`

	// CRL 证书撤销列表文件路径
	// 用于检查客户端证书是否被撤销（可选）
	CRL string `yaml:"crl"`
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
	// Access IP 访问控制
	// 配置允许或拒绝的 IP 地址/CIDR 范围
	Access AccessConfig `yaml:"access"`

	// RateLimit 速率限制
	// 控制请求频率防止滥用
	RateLimit RateLimitConfig `yaml:"rate_limit"`

	// Auth 认证配置
	// HTTP Basic 认证设置
	Auth AuthConfig `yaml:"auth"`

	// AuthRequest 外部认证子请求配置
	// 将认证委托给外部服务，根据响应状态码决定是否允许请求继续
	AuthRequest AuthRequestConfig `yaml:"auth_request"`

	// Headers 安全头部
	// 添加安全相关的 HTTP 响应头
	Headers SecurityHeaders `yaml:"headers"`

	// ErrorPage 自定义错误页面配置
	// 允许为特定 HTTP 状态码配置自定义错误页面
	ErrorPage ErrorPageConfig `yaml:"error_page"`
}

// AccessConfig IP 访问控制配置。
//
// 通过 IP 地址或 CIDR 范围控制访问权限。
//
// 注意事项：
//   - Allow 和 Deny 列表按配置顺序匹配
//   - Default 指定未匹配时的默认动作
//   - TrustedProxies 用于正确获取客户端真实 IP
//   - 支持 IPv4 和 IPv6 地址格式
//
// 使用示例：
//
//	access:
//	  allow: ["192.168.1.0/24", "10.0.0.0/8"]
//	  deny: ["192.168.1.100"]
//	  default: "deny"
//	  trusted_proxies: ["172.16.0.0/16"]
type AccessConfig struct {
	// Allow 允许的 IP/CIDR 列表
	// 配置允许访问的 IP 地址或网段
	Allow []string `yaml:"allow"`

	// Deny 拒绝的 IP/CIDR 列表
	// 配置拒绝访问的 IP 地址或网段
	Deny []string `yaml:"deny"`

	// Default 默认动作
	// 未匹配任何规则时的处理方式：allow 或 deny
	Default string `yaml:"default"`

	// TrustedProxies 可信代理 CIDR 列表
	// 用于正确解析 X-Forwarded-For 头部获取真实客户端 IP
	TrustedProxies []string `yaml:"trusted_proxies"`
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
	// RequestRate 每秒请求数限制
	// 超过此速率的请求将被拒绝
	RequestRate int `yaml:"request_rate"`

	// Burst 突发流量上限
	// 允许短时间内超出 RequestRate 的最大请求数
	Burst int `yaml:"burst"`

	// ConnLimit 连接数限制
	// 单个 IP 的最大并发连接数
	ConnLimit int `yaml:"conn_limit"`

	// Key 限流 key 来源
	// 可选值：ip、header，决定限流键的生成方式
	Key string `yaml:"key"`

	// Algorithm 限流算法
	// 可选值：token_bucket、sliding_window
	Algorithm string `yaml:"algorithm"`

	// SlidingWindowMode 滑动窗口模式
	// 可选值：approximate（近似）、precise（精确）
	SlidingWindowMode string `yaml:"sliding_window_mode"`

	// SlidingWindow 滑动窗口大小（秒）
	// 滑动窗口算法的时间窗口大小
	SlidingWindow int `yaml:"sliding_window"`
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
	// Type 认证类型
	// 目前仅支持 basic
	Type string `yaml:"type"`

	// RequireTLS 强制 HTTPS
	// 为 true 时只有通过 HTTPS 的请求才允许认证
	RequireTLS bool `yaml:"require_tls"`

	// Algorithm 哈希算法
	// 可选值：bcrypt、argon2id
	Algorithm string `yaml:"algorithm"`

	// Users 用户列表
	// 配置允许访问的用户及其密码哈希
	Users []User `yaml:"users"`

	// Realm 认证域
	// 显示在浏览器认证对话框中的描述信息
	Realm string `yaml:"realm"`
	// MinPasswordLength 密码最小长度
	// 用于验证密码哈希对应的原始密码长度（仅提示性验证）
	// 建议值：8-128，默认 8
	MinPasswordLength int `yaml:"min_password_length"`
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
	// Enabled 是否启用外部认证子请求
	Enabled bool `yaml:"enabled"`

	// URI 认证服务地址
	// 可以是相对路径（如 /auth）或完整 URL（如 http://auth-service:8080/verify）
	URI string `yaml:"uri"`

	// Method 认证请求方法
	// 默认为 GET，支持 GET、POST、HEAD 等
	Method string `yaml:"method"`

	// Timeout 认证请求超时时间
	// 默认 5 秒，超过此时间视为认证失败
	Timeout time.Duration `yaml:"auth_timeout"`

	// Headers 自定义认证请求头
	// 支持变量展开，用于向认证服务传递原请求信息
	Headers map[string]string `yaml:"headers"`

	// ForwardHeaders 需要转发到认证服务的原请求头
	// 默认包含：Cookie, Authorization, X-Forwarded-For
	ForwardHeaders []string `yaml:"forward_headers"`
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
	// Type 压缩类型
	// 可选值：gzip、brotli、both
	Type string `yaml:"type"`

	// Level 压缩级别
	// 范围 1-9，1 最快，9 压缩率最高
	Level int `yaml:"level"`

	// MinSize 最小压缩大小（字节）
	// 低于此大小的响应不进行压缩
	MinSize int `yaml:"min_size"`

	// Types 可压缩的 MIME 类型
	// 只有这些类型的响应会被压缩
	Types []string `yaml:"types"`

	// GzipStatic 启用预压缩文件支持
	// 为 true 时优先返回 .gz 预压缩文件
	GzipStatic bool `yaml:"gzip_static"`

	// GzipStaticExtensions 预压缩文件扩展名
	// 查找预压缩文件时附加的扩展名列表
	GzipStaticExtensions []string `yaml:"gzip_static_extensions"`
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
//   - MaxIdleConnsPerHost 控制每个后端主机的空闲连接
//   - IdleConnTimeout 控制空闲连接的保持时间
//   - MaxConnsPerHost 限制每个后端主机的总连接数
//
// 使用示例：
//
//	transport:
//	  max_idle_conns_per_host: 10
//	  idle_conn_timeout: 90s
//	  max_conns_per_host: 100
type TransportConfig struct {
	// MaxIdleConnsPerHost 每主机最大空闲连接
	// 单个后端主机的最大空闲连接数
	MaxIdleConnsPerHost int `yaml:"max_idle_conns_per_host"`

	// IdleConnTimeout 空闲连接超时
	// 空闲连接的最大存活时间
	IdleConnTimeout time.Duration `yaml:"idle_conn_timeout"`

	// MaxConnsPerHost 每主机最大连接数
	// 单个后端主机的总连接数上限
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
	// Enabled 是否启用 pprof 端点
	Enabled bool `yaml:"enabled"`

	// Path 端点路径前缀
	// 默认为 "/debug/pprof"
	Path string `yaml:"path"`

	// Allow 允许访问的 IP 列表
	// 可访问 pprof 端点的 IP 地址或 CIDR
	Allow []string `yaml:"allow"`
}

// StatusConfig 状态监控端点配置。
//
// 配置服务状态检查端点的路径和访问控制。
//
// 注意事项：
//   - Path 为状态端点的 URL 路径
//   - Allow 限制可访问的 IP 地址列表
//   - 生产环境建议严格限制访问来源
//
// 使用示例：
//
//	status:
//	  path: "/status"
//	  allow: ["127.0.0.1", "192.168.0.0/16"]
type StatusConfig struct {
	// Path 端点路径
	// 状态检查端点的 URL 路径
	Path string `yaml:"path"`

	// Allow 允许访问的 IP 列表
	// 可访问状态端点的 IP 地址或 CIDR
	Allow []string `yaml:"allow"`

	// Format 输出格式
	// 支持 "json" 和 "prometheus" 两种格式
	// 默认为 "json"
	Format string `yaml:"format"`
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
	// Enabled 是否启用缓存 API
	// 默认为 false
	Enabled bool `yaml:"enabled"`

	// Path API 端点路径
	// 默认为 "/_cache/purge"
	Path string `yaml:"path"`

	// Allow 允许访问的 IP 列表
	// 可访问缓存 API 的 IP 地址或 CIDR
	Allow []string `yaml:"allow"`

	// Auth 认证配置
	Auth CacheAPIAuthConfig `yaml:"auth"`
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
	// Enabled 是否启用 Lua 中间件
	Enabled bool `yaml:"enabled"`

	// Scripts 脚本配置列表
	Scripts []LuaScriptConfig `yaml:"scripts"`

	// GlobalSettings 全局设置
	GlobalSettings LuaGlobalSettings `yaml:"global_settings"`
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
//
// 使用示例：
//
//	global_settings:
//	  max_concurrent_coroutines: 1000
//	  coroutine_timeout: 30s
//	  code_cache_size: 1000
//	  enable_file_watch: true
//	  max_execution_time: 30s
type LuaGlobalSettings struct {
	// MaxConcurrentCoroutines 最大并发协程数
	MaxConcurrentCoroutines int `yaml:"max_concurrent_coroutines"`

	// CoroutineTimeout 协程执行超时
	CoroutineTimeout time.Duration `yaml:"coroutine_timeout"`

	// CodeCacheSize 字节码缓存条目数
	CodeCacheSize int `yaml:"code_cache_size"`

	// EnableFileWatch 启用文件变更检测
	EnableFileWatch bool `yaml:"enable_file_watch"`

	// MaxExecutionTime 单脚本最大执行时间
	MaxExecutionTime time.Duration `yaml:"max_execution_time"`
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
	// Listen 监听地址
	// TCP/UDP 监听地址，如 ":3306" 或 "0.0.0.0:6379"
	Listen string `yaml:"listen"`

	// Protocol 协议类型
	// 可选值：tcp、udp
	Protocol string `yaml:"protocol"`

	// Upstream 上游配置
	// 后端服务器列表和负载均衡设置
	Upstream StreamUpstream `yaml:"upstream"`

	// SSL SSL/TLS 配置
	// 启用 TLS 终端，支持加密的 TCP 连接
	SSL StreamSSLConfig `yaml:"ssl"`

	// ProxySSL 上游 SSL 配置
	// 启用到上游服务器的 TLS 连接
	ProxySSL StreamProxySSLConfig `yaml:"proxy_ssl"`
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
	// Targets 目标列表
	// 后端服务器地址列表
	Targets []StreamTarget `yaml:"targets"`

	// LoadBalance 负载均衡算法
	// 可选值：round_robin、least_conn
	LoadBalance string `yaml:"load_balance"`
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
	// Enabled 是否启用 SSL/TLS
	Enabled bool `yaml:"enabled"`

	// Cert 证书文件路径
	// PEM 格式的服务器证书
	Cert string `yaml:"cert"`

	// Key 私钥文件路径
	// PEM 格式的私钥
	Key string `yaml:"key"`

	// Protocols TLS 协议版本
	// 默认 ["TLSv1.2", "TLSv1.3"]
	Protocols []string `yaml:"protocols"`

	// Ciphers 加密套件
	// 仅对 TLS 1.2 有效
	Ciphers []string `yaml:"ciphers"`

	// ClientCA 客户端 CA 证书
	// 用于 mTLS 客户端证书验证
	ClientCA string `yaml:"client_ca"`

	// VerifyDepth 证书链验证深度
	// 默认 1
	VerifyDepth int `yaml:"verify_depth"`
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
	// Enabled 是否启用上游 SSL
	Enabled bool `yaml:"enabled"`

	// Verify 是否验证上游证书
	// 为 true 时验证证书链
	Verify bool `yaml:"verify"`

	// TrustedCA 信任的 CA 证书
	// 用于验证上游服务器证书
	TrustedCA string `yaml:"trusted_ca"`

	// ServerName 服务器名称
	// 用于 SNI 和证书验证
	ServerName string `yaml:"server_name"`

	// Cert 客户端证书
	// 用于 mTLS 客户端认证
	Cert string `yaml:"cert"`

	// Key 客户端私钥
	// 用于 mTLS 客户端认证
	Key string `yaml:"key"`

	// Protocols TLS 协议版本
	Protocols []string `yaml:"protocols"`

	// SessionReuse 是否复用 SSL 会话
	// 启用后可提升连接性能
	SessionReuse bool `yaml:"session_reuse"`
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
	// Enabled 是否启用 DNS 解析器
	Enabled bool `yaml:"enabled"`

	// Addresses DNS 服务器地址列表
	// 格式为 "ip:port"，如 "8.8.8.8:53"
	Addresses []string `yaml:"addresses"`

	// Valid 缓存有效期（TTL）
	// 解析结果的缓存时间
	Valid time.Duration `yaml:"valid"`

	// Timeout DNS 查询超时
	// 单次 DNS 查询的最大等待时间
	Timeout time.Duration `yaml:"timeout"`

	// IPv4 是否查询 IPv4 地址
	IPv4 bool `yaml:"ipv4"`

	// IPv6 是否查询 IPv6 地址
	IPv6 bool `yaml:"ipv6"`

	// CacheSize 缓存最大条目数
	// 0 表示无限制
	CacheSize int `yaml:"cache_size"`
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
