package config

import "time"

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
