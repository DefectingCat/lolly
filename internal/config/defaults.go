// Package config 提供 YAML 配置文件的解析、验证和默认配置生成功能。
//
// 该文件包含默认配置生成功能，包括：
//   - DefaultConfig 函数：返回带合理默认值的配置结构体
//   - GenerateConfigYAML 函数：生成带注释的示例配置文件
//
// 主要用途：
//
//	用于生成默认配置和示例配置文件，便于用户快速上手。
//
// 注意事项：
//   - 默认值经过优化，适合大多数常见场景
//   - 生成的 YAML 包含详细注释说明
//
// 作者：xfy
package config

import (
	"bytes"
	"fmt"
	"time"
)

// DefaultConfig 返回带默认值的配置结构体。
//
// 返回一个预填充了合理默认值的配置对象，包含服务器、
// SSL、安全、压缩、日志、性能和监控等各模块的默认设置。
//
// 返回值：
//   - *Config: 带默认值的配置对象
//
// 默认值说明：
//   - 监听地址: :8080
//   - 超时: 读取/写入 30s，空闲 120s
//   - TLS: 仅支持 TLSv1.2 和 TLSv1.3
//   - 安全头部: X-Frame-Options=DENY, X-Content-Type-Options=nosniff
//   - 压缩: gzip，级别 6，最小 1024 字节
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:             ":8080",
			Name:               "localhost",
			ReadTimeout:        30 * time.Second,
			WriteTimeout:       30 * time.Second,
			IdleTimeout:        120 * time.Second,
			MaxConnsPerIP:      1000,
			MaxRequestsPerConn: 10000,
			CacheAPI: &CacheAPIConfig{
				Enabled: false,
				Path:    "/_cache/purge",
				Allow:   []string{"127.0.0.1"},
				Auth: CacheAPIAuthConfig{
					Type:  "none",
					Token: "",
				},
			},
			Static: []StaticConfig{{
				Path:  "/",
				Root:  "/var/www/html",
				Index: []string{"index.html", "index.htm"},
			}},
			SSL: SSLConfig{
				Protocols:    []string{"TLSv1.2", "TLSv1.3"},
				OCSPStapling: false,
				HSTS: HSTSConfig{
					MaxAge:            31536000,
					IncludeSubDomains: true,
					Preload:           false,
				},
				HTTP2: HTTP2Config{
					Enabled:              true,
					MaxConcurrentStreams: 128,
					MaxHeaderListSize:    1048576, // 1MB
					IdleTimeout:          120 * time.Second,
					PushEnabled:          false,
					H2CEnabled:           false,
				},
				SessionTickets: SessionTicketsConfig{
					Enabled:        false,
					KeyFile:        "",
					RotateInterval: 1 * time.Hour,
					RetainKeys:     3,
				},
				ClientVerify: ClientVerifyConfig{
					Enabled:     false,
					Mode:        "none",
					ClientCA:    "",
					VerifyDepth: 1,
					CRL:         "",
				},
			},
			Security: SecurityConfig{
				Access: AccessConfig{
					Allow:   []string{},
					Deny:    []string{},
					Default: "allow",
				},
				RateLimit: RateLimitConfig{
					RequestRate:       0,
					Burst:             0,
					ConnLimit:         0,
					Key:               "ip",
					Algorithm:         "token_bucket",
					SlidingWindowMode: "approximate",
					SlidingWindow:     60,
				},
				Auth: AuthConfig{
					RequireTLS:        true,
					Algorithm:         "bcrypt",
					Realm:             "Restricted Area",
					MinPasswordLength: 8,
				},
				Headers: SecurityHeaders{
					XFrameOptions:       "DENY",
					XContentTypeOptions: "nosniff",
					ReferrerPolicy:      "strict-origin-when-cross-origin",
				},
			},
			Compression: CompressionConfig{
				Type:                 "gzip",
				Level:                6,
				MinSize:              1024,
				GzipStatic:           false,
				GzipStaticExtensions: []string{".br", ".gz"},
				Types: []string{
					"text/html",
					"text/css",
					"text/javascript",
					"application/json",
					"application/javascript",
				},
			},
		},
		Logging: LoggingConfig{
			Format: "text",
			Access: AccessLogConfig{
				// 近似 nginx combined 格式
				// nginx: $remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"
				// lolly: $request 不含 HTTP 版本，$time 为 RFC3339 格式
				Format: "$remote_addr - $remote_user [$time] \"$request\" $status $body_bytes_sent \"$http_referer\" \"$http_user_agent\"",
			},
			Error: ErrorLogConfig{
				Level: "info",
			},
		},
		Performance: PerformanceConfig{
			GoroutinePool: GoroutinePoolConfig{
				Enabled:     false,
				MaxWorkers:  1000,
				MinWorkers:  10,
				IdleTimeout: 60 * time.Second,
			},
			FileCache: FileCacheConfig{
				MaxEntries: 10000,
				MaxSize:    256 * 1024 * 1024, // 256MB
				Inactive:   20 * time.Second,
			},
			Transport: TransportConfig{
				MaxIdleConnsPerHost: 32,
				IdleConnTimeout:     90 * time.Second,
				MaxConnsPerHost:     0, // 0 表示不限制
			},
		},
		Monitoring: MonitoringConfig{
			Status: StatusConfig{
				Path:  "/_status",
				Allow: []string{"127.0.0.1"},
			},
			Pprof: PprofConfig{
				Enabled: false,
				Path:    DefaultPprofPath,
				Allow:   []string{"127.0.0.1"},
			},
		},
		HTTP3: HTTP3Config{
			Enabled:     false,
			Listen:      ":443",
			MaxStreams:  100,
			IdleTimeout: 60 * time.Second,
			Enable0RTT:  false,
		},
		Resolver: ResolverConfig{
			Enabled:   false,
			Addresses: []string{"8.8.8.8:53", "8.8.4.4:53"},
			Valid:     30 * time.Second,
			Timeout:   5 * time.Second,
			IPv4:      true,
			IPv6:      false,
			CacheSize: 1024,
		},
		Variables: VariablesConfig{
			Set: map[string]string{},
		},
	}
}

// GenerateConfigYAML 生成带注释的默认配置 YAML。
//
// 根据配置对象生成带详细注释的 YAML 配置文件内容，
// 包含各配置项的说明和有效值提示。
//
// 参数：
//   - cfg: 配置对象，用于填充 YAML 中的默认值
//
// 返回值：
//   - []byte: 生成的 YAML 内容（包含注释）
//   - error: 当前实现始终返回 nil
//
// 注意事项：
//   - 生成的 YAML 包含中文注释，便于理解各配置项用途
//   - 注释中包含有效值范围和默认值说明
func GenerateConfigYAML(cfg *Config) ([]byte, error) {
	// 手动构建带注释的 YAML
	var buf bytes.Buffer

	buf.WriteString("# Lolly 配置文件\n")
	// buf.WriteString("# 文档: https://github.com/xfy/lolly\n")
	buf.WriteString("\n")

	// server 配置
	buf.WriteString("# 服务器配置（单服务器模式）\n")
	buf.WriteString("server:\n")
	fmt.Fprintf(&buf, "  listen: \"%s\"           # 监听地址\n", cfg.Server.Listen)
	fmt.Fprintf(&buf, "  name: \"%s\"             # 服务器名称（虚拟主机匹配）\n", cfg.Server.Name)
	fmt.Fprintf(&buf, "  read_timeout: %ds            # 读取超时（0 表示不限制）\n", int(cfg.Server.ReadTimeout.Seconds()))
	fmt.Fprintf(&buf, "  write_timeout: %ds           # 写入超时（0 表示不限制）\n", int(cfg.Server.WriteTimeout.Seconds()))
	fmt.Fprintf(&buf, "  idle_timeout: %ds            # 空闲超时（0 表示不限制）\n", int(cfg.Server.IdleTimeout.Seconds()))
	fmt.Fprintf(&buf, "  max_conns_per_ip: %d         # 每 IP 最大连接数（0 表示不限制）\n", cfg.Server.MaxConnsPerIP)
	fmt.Fprintf(&buf, "  max_requests_per_conn: %d    # 每连接最大请求数（0 表示不限制）\n", cfg.Server.MaxRequestsPerConn)
	fmt.Fprintf(&buf, "  client_max_body_size: \"1MB\"  # 请求体大小限制（支持单位: b, kb, mb, gb）\n")
	buf.WriteString("\n")

	// static 配置
	buf.WriteString("  # 静态文件服务配置（支持多个目录）\n")
	buf.WriteString("  static:\n")
	for _, st := range cfg.Server.Static {
		buf.WriteString("    - path: \"/\"              # 匹配路径前缀\n")
		fmt.Fprintf(&buf, "      root: \"%s\"  # 静态文件根目录\n", st.Root)
		buf.WriteString("      index:                 # 索引文件\n")
		for _, idx := range st.Index {
			fmt.Fprintf(&buf, "        - \"%s\"\n", idx)
		}
		buf.WriteString("      try_files: []             # SPA 部署示例: [\"$uri\", \"$uri/\", \"/index.html\"]\n")
		buf.WriteString("      try_files_pass: false     # 内部重定向是否触发中间件\n")
	}
	buf.WriteString("  # 示例：额外的静态目录\n")
	buf.WriteString("  # - path: \"/assets/\"\n")
	buf.WriteString("  #   root: \"/var/www/assets\"\n")
	buf.WriteString("  #   index: [\"index.html\"]\n")
	buf.WriteString("\n")

	// proxy 配置示例
	buf.WriteString("  # 反向代理配置\n")
	buf.WriteString("  # proxy:\n")
	buf.WriteString("  #   - path: /api                  # 匹配路径前缀\n")
	buf.WriteString("  #     targets:                    # 后端目标列表\n")
	buf.WriteString("  #       - url: http://backend1:8080\n")
	buf.WriteString("  #         weight: 3               # 权重（加权轮询时有效）\n")
	buf.WriteString("  #       - url: http://backend2:8080\n")
	buf.WriteString("  #         weight: 1\n")
	buf.WriteString("  #     load_balance: round_robin   # 负载均衡算法（有效值: round_robin, weighted_round_robin, least_conn, ip_hash, consistent_hash）\n")
	buf.WriteString("  #     hash_key: ip                # 一致性哈希键（仅 load_balance=consistent_hash 时有效，有效值: ip, uri, header:X-Name）\n")
	buf.WriteString("  #     virtual_nodes: 150          # 一致性哈希虚拟节点数（仅 load_balance=consistent_hash 时有效）\n")
	buf.WriteString("  #     health_check:               # 健康检查\n")
	buf.WriteString("  #       interval: 10s\n")
	buf.WriteString("  #       path: /health\n")
	buf.WriteString("  #       timeout: 5s\n")
	buf.WriteString("  #     timeout:                    # 超时配置\n")
	buf.WriteString("  #       connect: 5s               # 连接超时\n")
	buf.WriteString("  #       read: 30s                 # 读取超时\n")
	buf.WriteString("  #       write: 30s                # 写入超时\n")
	buf.WriteString("  #     headers:                    # 头部修改\n")
	buf.WriteString("  #       set_request: {X-Custom: value}\n")
	buf.WriteString("  #       set_response: {X-Server: lolly}\n")
	buf.WriteString("  #       remove: [X-Powered-By]\n")
	buf.WriteString("  #     cache:                      # 代理缓存\n")
	buf.WriteString("  #       enabled: false\n")
	buf.WriteString("  #       max_age: 60s\n")
	buf.WriteString("  #       cache_lock: true          # 防止缓存击穿\n")
	buf.WriteString("  #       stale_while_revalidate: 30s\n")
	buf.WriteString("  #     client_max_body_size: \"50MB\"  # 此代理路径的请求体限制\n")
	buf.WriteString("  #     next_upstream:              # 故障转移配置\n")
	buf.WriteString("  #       tries: 1                  # 最大尝试次数（1 表示禁用故障转移）\n")
	buf.WriteString("  #       http_codes: [502, 503, 504]  # 触发重试的 HTTP 状态码\n")
	buf.WriteString("\n")

	// SSL 配置
	buf.WriteString("  # SSL/TLS 配置\n")
	buf.WriteString("  # ssl:\n")
	buf.WriteString("  #   cert: /path/to/cert.pem        # 证书文件\n")
	buf.WriteString("  #   key: /path/to/key.pem          # 私钥文件\n")
	buf.WriteString("  #   cert_chain: /path/to/chain.pem # 证书链文件\n")
	buf.WriteString("  #   protocols:                     # TLS 版本（有效值: TLSv1.2, TLSv1.3）\n")
	for _, proto := range cfg.Server.SSL.Protocols {
		fmt.Fprintf(&buf, "  #     - \"%s\"\n", proto)
	}
	buf.WriteString("  #   ciphers:                       # 加密套件（仅 TLS 1.2 有效，TLS 1.3 使用内置套件）\n")
	buf.WriteString("  #     - ECDHE-ECDSA-AES256-GCM-SHA384\n")
	buf.WriteString("  #     - ECDHE-RSA-AES256-GCM-SHA384\n")
	buf.WriteString("  #     - ECDHE-ECDSA-CHACHA20-POLY1305\n")
	buf.WriteString("  #     - ECDHE-RSA-CHACHA20-POLY1305\n")
	buf.WriteString("  #   # 拒绝不安全套件：含 RC4、DES、3DES、CBC 的配置将报错\n")
	fmt.Fprintf(&buf, "  #   ocsp_stapling: %v              # OCSP Stapling\n", cfg.Server.SSL.OCSPStapling)
	buf.WriteString("  #   hsts:                          # HTTP Strict Transport Security\n")
	fmt.Fprintf(&buf, "  #     max_age: %d                  # 过期时间（秒）\n", cfg.Server.SSL.HSTS.MaxAge)
	fmt.Fprintf(&buf, "  #     include_sub_domains: %v      # 包含子域名\n", cfg.Server.SSL.HSTS.IncludeSubDomains)
	fmt.Fprintf(&buf, "  #     preload: %v                  # 加入 HSTS 预加载列表\n", cfg.Server.SSL.HSTS.Preload)
	buf.WriteString("  #   session_tickets:               # TLS Session Tickets 配置（TLS 1.3 会话恢复）\n")
	fmt.Fprintf(&buf, "  #     enabled: %v                  # 是否启用 Session Tickets\n", cfg.Server.SSL.SessionTickets.Enabled)
	buf.WriteString("  #     key_file: \"\"                # 密钥存储文件路径（用于持久化密钥）\n")
	fmt.Fprintf(&buf, "  #     rotate_interval: %d          # 密钥轮换间隔（秒），建议 1-24 小时\n", int(cfg.Server.SSL.SessionTickets.RotateInterval.Seconds()))
	fmt.Fprintf(&buf, "  #     retain_keys: %d              # 保留的历史密钥数量，建议 3-5 个\n", cfg.Server.SSL.SessionTickets.RetainKeys)
	buf.WriteString("  #   client_verify:                 # mTLS 客户端证书验证配置\n")
	buf.WriteString("  #     enabled: false               # 是否启用客户端证书验证\n")
	buf.WriteString("  #     mode: \"none\"                # 验证模式（有效值: none, request, require, optional_no_ca）\n")
	buf.WriteString("  #     client_ca: \"\"               # 客户端 CA 证书文件路径\n")
	buf.WriteString("  #     verify_depth: 1              # 证书链验证深度\n")
	buf.WriteString("  #     crl: \"\"                     # 证书撤销列表文件路径（可选）\n")
	buf.WriteString("  #   http2:                         # HTTP/2 配置（需 SSL 证书）\n")
	fmt.Fprintf(&buf, "  #     enabled: %v                  # 是否启用 HTTP/2\n", cfg.Server.SSL.HTTP2.Enabled)
	fmt.Fprintf(&buf, "  #     max_concurrent_streams: %d   # 最大并发流数\n", cfg.Server.SSL.HTTP2.MaxConcurrentStreams)
	fmt.Fprintf(&buf, "  #     max_header_list_size: %d     # 最大头部列表大小（字节）\n", cfg.Server.SSL.HTTP2.MaxHeaderListSize)
	fmt.Fprintf(&buf, "  #     idle_timeout: %ds            # 空闲超时\n", int(cfg.Server.SSL.HTTP2.IdleTimeout.Seconds()))
	fmt.Fprintf(&buf, "  #     push_enabled: %v             # 是否启用 Server Push\n", cfg.Server.SSL.HTTP2.PushEnabled)
	fmt.Fprintf(&buf, "  #     h2c_enabled: %v              # 是否启用 H2C（明文 HTTP/2）\n", cfg.Server.SSL.HTTP2.H2CEnabled)
	buf.WriteString("\n")

	// security 配置
	buf.WriteString("  # 安全配置\n")
	buf.WriteString("  security:\n")
	buf.WriteString("    # IP 访问控制\n")
	buf.WriteString("    access:\n")
	buf.WriteString("      allow: []                   # 允许的 IP/CIDR 列表\n")
	buf.WriteString("      deny: []                    # 拒绝的 IP/CIDR 列表\n")
	fmt.Fprintf(&buf, "      default: \"%s\"             # 默认动作（有效值: allow, deny）\n", cfg.Server.Security.Access.Default)
	buf.WriteString("      trusted_proxies: []         # 可信代理 CIDR 列表，用于 X-Forwarded-For 解析\n")
	buf.WriteString("\n")
	buf.WriteString("    # 速率限制\n")
	buf.WriteString("    rate_limit:\n")
	fmt.Fprintf(&buf, "      request_rate: %d            # 每秒请求数（0 表示不限制）\n", cfg.Server.Security.RateLimit.RequestRate)
	fmt.Fprintf(&buf, "      burst: %d                   # 突发上限\n", cfg.Server.Security.RateLimit.Burst)
	fmt.Fprintf(&buf, "      conn_limit: %d              # 连接数限制\n", cfg.Server.Security.RateLimit.ConnLimit)
	fmt.Fprintf(&buf, "      key: \"%s\"                  # 限流 key 来源（有效值: ip, header）\n", cfg.Server.Security.RateLimit.Key)
	fmt.Fprintf(&buf, "      algorithm: \"%s\"             # 限流算法（有效值: token_bucket, sliding_window）\n", cfg.Server.Security.RateLimit.Algorithm)
	fmt.Fprintf(&buf, "      sliding_window_mode: \"%s\"   # 滑动窗口模式（有效值: approximate, precise，仅 algorithm=sliding_window 时有效）\n", cfg.Server.Security.RateLimit.SlidingWindowMode)
	fmt.Fprintf(&buf, "      sliding_window: %d          # 滑动窗口大小（秒，仅 algorithm=sliding_window 时有效）\n", cfg.Server.Security.RateLimit.SlidingWindow)
	buf.WriteString("\n")
	buf.WriteString("    # 认证配置（type 为空时禁用）\n")
	buf.WriteString("    auth:\n")
	buf.WriteString("      type: \"\"                    # 认证类型（有效值: basic，空表示禁用）\n")
	fmt.Fprintf(&buf, "      require_tls: %v             # 启用时强制 HTTPS\n", cfg.Server.Security.Auth.RequireTLS)
	fmt.Fprintf(&buf, "      algorithm: \"%s\"            # 密码哈希算法（有效值: bcrypt, argon2id）\n", cfg.Server.Security.Auth.Algorithm)
	buf.WriteString("      users: []                   # 用户列表\n")
	fmt.Fprintf(&buf, "      realm: \"%s\"                # 认证域\n", cfg.Server.Security.Auth.Realm)
	fmt.Fprintf(&buf, "      min_password_length: %d     # 密码最小长度\n", cfg.Server.Security.Auth.MinPasswordLength)
	buf.WriteString("\n")
	buf.WriteString("    # 安全头部\n")
	buf.WriteString("    headers:\n")
	fmt.Fprintf(&buf, "      x_frame_options: \"%s\"        # 防止点击劫持（有效值: DENY, SAMEORIGIN, 空表示禁用）\n", cfg.Server.Security.Headers.XFrameOptions)
	fmt.Fprintf(&buf, "      x_content_type_options: \"%s\" # 防止 MIME 嗅探（有效值：nosniff，空表示禁用）\n", cfg.Server.Security.Headers.XContentTypeOptions)
	fmt.Fprintf(&buf, "      referrer_policy: \"%s\"        # 引用策略（有效值: no-referrer, no-referrer-when-downgrade, origin, origin-when-cross-origin, same-origin, strict-origin, strict-origin-when-cross-origin, unsafe-url）\n", cfg.Server.Security.Headers.ReferrerPolicy)
	buf.WriteString("      content_security_policy: \"\"  # 内容安全策略 CSP（空表示禁用）\n")
	buf.WriteString("      permissions_policy: \"\"       # 权限策略（空表示禁用）\n")
	buf.WriteString("\n")
	buf.WriteString("    # 自定义错误页面\n")
	buf.WriteString("    error_page:\n")
	buf.WriteString("      pages: {}                  # 状态码到页面映射，如 {404: \"/errors/404.html\"}\n")
	buf.WriteString("      default: \"\"                # 默认错误页面\n")
	buf.WriteString("      response_code: 0          # 响应状态码覆盖（0 表示使用原始状态码）\n")
	buf.WriteString("\n")
	buf.WriteString("    # 外部认证子请求配置（将认证委托给外部服务）\n")
	buf.WriteString("    auth_request:\n")
	buf.WriteString("      enabled: false              # 是否启用外部认证\n")
	buf.WriteString("      uri: \"\"                     # 认证服务地址（支持相对路径或完整 URL）\n")
	buf.WriteString("      method: \"GET\"               # 认证请求方法（有效值: GET, POST, HEAD）\n")
	buf.WriteString("      auth_timeout: 5s            # 认证请求超时时间\n")
	buf.WriteString("      headers: {}                 # 自定义认证请求头，如 {X-Original-Uri: \"$request_uri\"}\n")
	buf.WriteString("      forward_headers: []         # 需要转发的原请求头，默认包含 Cookie, Authorization, X-Forwarded-For\n")
	buf.WriteString("\n")

	// rewrite 配置示例
	buf.WriteString("  # URL 重写规则\n")
	buf.WriteString("  # rewrite:\n")
	buf.WriteString("  #   - pattern: \"^/old/(.*)$\"     # 匹配模式（正则表达式）\n")
	buf.WriteString("  #     replacement: /new/$1       # 替换目标\n")
	buf.WriteString("  #     flag: last                 # 标志（有效值: last, redirect, permanent, break）\n")
	buf.WriteString("\n")

	// compression 配置
	buf.WriteString("  # 响应压缩配置\n")
	buf.WriteString("  compression:\n")
	fmt.Fprintf(&buf, "    type: \"%s\"            # 压缩类型（有效值: gzip, brotli, both，空表示禁用）\n", cfg.Server.Compression.Type)
	fmt.Fprintf(&buf, "    level: %d              # 压缩级别（范围 0-9，0=不压缩，1=最快，9=最高压缩率）\n", cfg.Server.Compression.Level)
	fmt.Fprintf(&buf, "    min_size: %d        # 最小压缩大小（字节，小于此值不压缩）\n", cfg.Server.Compression.MinSize)
	fmt.Fprintf(&buf, "    gzip_static: %v        # 启用预压缩文件支持（自动查找 .gz/.br 文件）\n", cfg.Server.Compression.GzipStatic)
	buf.WriteString("    gzip_static_extensions:  # 预压缩文件扩展名\n")
	for _, ext := range cfg.Server.Compression.GzipStaticExtensions {
		fmt.Fprintf(&buf, "      - \"%s\"\n", ext)
	}
	buf.WriteString("    types:                 # 可压缩的 MIME 类型\n")
	for _, t := range cfg.Server.Compression.Types {
		fmt.Fprintf(&buf, "      - \"%s\"\n", t)
	}
	buf.WriteString("\n")

	// servers 配置说明 - 完整示例
	buf.WriteString("# 多虚拟主机模式（可选，每个虚拟主机支持完整的 server 配置）\n")
	buf.WriteString("# servers:\n")
	buf.WriteString("#   - listen: \":8080\"              # 监听地址\n")
	buf.WriteString("#     name: \"api.example.com\"        # 服务器名称（用于虚拟主机匹配）\n")
	buf.WriteString("#     read_timeout: 30s               # 读取超时（0 表示不限制）\n")
	buf.WriteString("#     write_timeout: 30s              # 写入超时（0 表示不限制）\n")
	buf.WriteString("#     idle_timeout: 120s              # 空闲超时（0 表示不限制）\n")
	buf.WriteString("#     max_conns_per_ip: 1000          # 每 IP 最大连接数（0 表示不限制）\n")
	buf.WriteString("#     max_requests_per_conn: 10000   # 每连接最大请求数（0 表示不限制）\n")
	buf.WriteString("#     static:                         # 静态文件配置（支持多个目录）\n")
	buf.WriteString("#       - path: /\n")
	buf.WriteString("#         root: /var/www/api\n")
	buf.WriteString("#         index: [index.html]\n")
	buf.WriteString("#     proxy:                          # 反向代理配置\n")
	buf.WriteString("#       - path: /api\n")
	buf.WriteString("#         targets:\n")
	buf.WriteString("#           - url: http://backend:8080\n")
	buf.WriteString("#         load_balance: round_robin\n")
	buf.WriteString("#     ssl:                            # SSL/TLS 配置\n")
	buf.WriteString("#       cert: /path/to/api.cert.pem\n")
	buf.WriteString("#       key: /path/to/api.key.pem\n")
	buf.WriteString("#       protocols: [TLSv1.2, TLSv1.3]\n")
	buf.WriteString("#       hsts:\n")
	buf.WriteString("#         max_age: 31536000\n")
	buf.WriteString("#         include_sub_domains: true\n")
	buf.WriteString("#     security:                       # 安全配置\n")
	buf.WriteString("#       access:\n")
	buf.WriteString("#         default: allow\n")
	buf.WriteString("#       rate_limit:\n")
	buf.WriteString("#         request_rate: 100\n")
	buf.WriteString("#       headers:\n")
	buf.WriteString("#         x_frame_options: DENY\n")
	buf.WriteString("#     compression:                    # 响应压缩配置\n")
	buf.WriteString("#       type: gzip\n")
	buf.WriteString("#       level: 6\n")
	buf.WriteString("#   - listen: \":8443\"              # 另一个虚拟主机\n")
	buf.WriteString("#     name: \"static.example.com\"\n")
	buf.WriteString("#     static:\n")
	buf.WriteString("#       - path: /\n")
	buf.WriteString("#         root: /var/www/static\n")
	buf.WriteString("#         index: [index.html, index.htm]\n")
	buf.WriteString("#     ssl:\n")
	buf.WriteString("#       cert: /path/to/static.cert.pem\n")
	buf.WriteString("#       key: /path/to/static.key.pem\n")
	buf.WriteString("#     compression:\n")
	buf.WriteString("#       type: gzip\n")
	buf.WriteString("\n")

	// SSL 默认值说明（即使不启用也展示默认配置）
	buf.WriteString("# SSL/TLS 默认配置说明（未配置证书时不启用）\n")
	buf.WriteString("# 默认 TLS 协议: TLSv1.2, TLSv1.3（不支持 TLSv1.0/1.1）\n")
	buf.WriteString("# 默认 HSTS 配置: max_age=31536000（1年）, include_sub_domains=true\n")
	buf.WriteString("\n")

	// stream 配置
	buf.WriteString("# TCP/UDP Stream 代理配置（可选）\n")
	buf.WriteString("# stream:\n")
	buf.WriteString("#   - listen: \"3306\"              # 监听地址\n")
	buf.WriteString("#     protocol: \"tcp\"             # 协议类型（有效值: tcp, udp）\n")
	buf.WriteString("#     upstream:\n")
	buf.WriteString("#       targets:                   # 上游目标列表\n")
	buf.WriteString("#         - addr: \"mysql1:3306\"   # 目标地址\n")
	buf.WriteString("#           weight: 3              # 权重（加权轮询时有效）\n")
	buf.WriteString("#         - addr: \"mysql2:3306\"\n")
	buf.WriteString("#           weight: 1\n")
	buf.WriteString("#       load_balance: \"round_robin\"  # 负载均衡算法（有效值: round_robin, weighted_round_robin, least_conn, ip_hash）\n")
	buf.WriteString("#     ssl:                        # 服务端 SSL 配置（仅 TCP 支持）\n")
	buf.WriteString("#       enabled: false            # 是否启用 TLS 终端\n")
	buf.WriteString("#       cert: \"/path/to/cert.pem\" # 服务器证书文件\n")
	buf.WriteString("#       key: \"/path/to/key.pem\"   # 服务器私钥文件\n")
	buf.WriteString("#       protocols: [\"TLSv1.2\", \"TLSv1.3\"]  # TLS 协议版本\n")
	buf.WriteString("#       ciphers: []               # 加密套件（仅 TLS 1.2 有效）\n")
	buf.WriteString("#       client_ca: \"\"             # 客户端 CA 证书（mTLS）\n")
	buf.WriteString("#       verify_depth: 1           # 证书链验证深度\n")
	buf.WriteString("#     proxy_ssl:                  # 上游 SSL 配置（加密到后端的连接）\n")
	buf.WriteString("#       enabled: false            # 是否启用上游 TLS\n")
	buf.WriteString("#       verify: false             # 是否验证上游证书\n")
	buf.WriteString("#       trusted_ca: \"\"            # 信任的 CA 证书\n")
	buf.WriteString("#       server_name: \"\"           # SNI 服务器名称\n")
	buf.WriteString("#       cert: \"\"                  # 客户端证书（mTLS）\n")
	buf.WriteString("#       key: \"\"                   # 客户端私钥（mTLS）\n")
	buf.WriteString("#       protocols: []             # TLS 协议版本\n")
	buf.WriteString("#       session_reuse: false      # 是否复用 SSL 会话\n")
	buf.WriteString("\n")

	// logging 配置
	buf.WriteString("# 日志配置\n")
	buf.WriteString("logging:\n")
	fmt.Fprintf(&buf, "  format: \"%s\"           # 全局日志格式（有效值: text, json），控制启动/停止日志格式\n", cfg.Logging.Format)
	buf.WriteString("  access:\n")
	buf.WriteString("    path: \"\"                   # 日志文件路径（空表示输出到 stdout）\n")
	fmt.Fprintf(&buf, "    format: '%s'  # 访问日志格式，近似 nginx combined\n", cfg.Logging.Access.Format)
	buf.WriteString("    # 支持变量: $remote_addr, $remote_user, $request, $status, $body_bytes_sent, $request_time, $http_referer, $http_user_agent, $time\n")
	buf.WriteString("    # 特殊值 \"json\" 输出结构化 JSON\n")
	buf.WriteString("  error:\n")
	buf.WriteString("    path: \"\"                   # 日志文件路径（空表示输出到 stderr）\n")
	fmt.Fprintf(&buf, "    level: \"%s\"           # 日志级别（有效值: debug, info, warn, error，级别越高日志越少）\n", cfg.Logging.Error.Level)
	buf.WriteString("\n")

	// performance 配置
	buf.WriteString("# 性能配置\n")
	buf.WriteString("performance:\n")
	buf.WriteString("  goroutine_pool:              # Goroutine 池（处理并发请求）\n")
	fmt.Fprintf(&buf, "    enabled: %v             # 是否启用\n", cfg.Performance.GoroutinePool.Enabled)
	fmt.Fprintf(&buf, "    max_workers: %d          # 最大 worker 数\n", cfg.Performance.GoroutinePool.MaxWorkers)
	fmt.Fprintf(&buf, "    min_workers: %d          # 最小 worker 数（预热）\n", cfg.Performance.GoroutinePool.MinWorkers)
	fmt.Fprintf(&buf, "    idle_timeout: %ds        # 空闲超时\n", int(cfg.Performance.GoroutinePool.IdleTimeout.Seconds()))
	buf.WriteString("  file_cache:                  # 静态文件缓存\n")
	fmt.Fprintf(&buf, "    max_entries: %d          # 最大缓存条目\n", cfg.Performance.FileCache.MaxEntries)
	fmt.Fprintf(&buf, "    max_size: %d              # 内存上限（字节，%dMB）\n", cfg.Performance.FileCache.MaxSize, cfg.Performance.FileCache.MaxSize/1024/1024)
	fmt.Fprintf(&buf, "    inactive: %ds             # 未访问淘汰时间\n", int(cfg.Performance.FileCache.Inactive.Seconds()))
	buf.WriteString("  transport:                   # HTTP Transport 连接池\n")
	fmt.Fprintf(&buf, "    max_idle_conns_per_host: %d   # 每主机空闲连接\n", cfg.Performance.Transport.MaxIdleConnsPerHost)
	fmt.Fprintf(&buf, "    idle_conn_timeout: %ds        # 空闲超时\n", int(cfg.Performance.Transport.IdleConnTimeout.Seconds()))
	fmt.Fprintf(&buf, "    max_conns_per_host: %d        # 每主机最大连接（0 表示不限制）\n", cfg.Performance.Transport.MaxConnsPerHost)
	buf.WriteString("\n")

	// HTTP3 配置
	buf.WriteString("# HTTP/3 (QUIC) 配置（需要 SSL 证书）\n")
	buf.WriteString("http3:\n")
	fmt.Fprintf(&buf, "  enabled: %v              # 是否启用 HTTP/3\n", cfg.HTTP3.Enabled)
	fmt.Fprintf(&buf, "  listen: \"%s\"             # UDP 监听地址\n", cfg.HTTP3.Listen)
	fmt.Fprintf(&buf, "  max_streams: %d          # 最大并发流\n", cfg.HTTP3.MaxStreams)
	fmt.Fprintf(&buf, "  idle_timeout: %ds        # 空闲超时\n", int(cfg.HTTP3.IdleTimeout.Seconds()))
	fmt.Fprintf(&buf, "  enable_0rtt: %v          # 启用 0-RTT（早期数据，可能存在安全风险）\n", cfg.HTTP3.Enable0RTT)
	buf.WriteString("\n")

	// monitoring 配置
	buf.WriteString("# 监控配置\n")
	buf.WriteString("monitoring:\n")
	buf.WriteString("  status:\n")
	fmt.Fprintf(&buf, "    path: \"%s\"        # 状态端点路径\n", cfg.Monitoring.Status.Path)
	buf.WriteString("    allow:                 # 允许访问的 IP\n")
	for _, ip := range cfg.Monitoring.Status.Allow {
		fmt.Fprintf(&buf, "      - \"%s\"\n", ip)
	}
	buf.WriteString("  pprof:                  # pprof 性能分析端点（用于 PGO 优化）\n")
	fmt.Fprintf(&buf, "    enabled: %v           # 是否启用（生产环境仅在收集 profile 时启用）\n", cfg.Monitoring.Pprof.Enabled)
	fmt.Fprintf(&buf, "    path: \"%s\"      # 端点路径前缀\n", cfg.Monitoring.Pprof.Path)
	buf.WriteString("    allow:                 # 允许访问的 IP\n")
	for _, ip := range cfg.Monitoring.Pprof.Allow {
		fmt.Fprintf(&buf, "      - \"%s\"\n", ip)
	}
	buf.WriteString("\n")

	// resolver 配置
	buf.WriteString("# DNS 解析器配置（用于后端域名动态解析）\n")
	buf.WriteString("resolver:\n")
	fmt.Fprintf(&buf, "  enabled: %v              # 是否启用 DNS 解析器\n", cfg.Resolver.Enabled)
	buf.WriteString("  addresses:               # DNS 服务器地址列表\n")
	buf.WriteString("    - \"8.8.8.8:53\"\n")
	buf.WriteString("    - \"8.8.4.4:53\"\n")
	fmt.Fprintf(&buf, "  valid: %ds               # 缓存有效期（TTL），建议 30s-300s\n", int(cfg.Resolver.Valid.Seconds()))
	fmt.Fprintf(&buf, "  timeout: %ds             # DNS 查询超时\n", int(cfg.Resolver.Timeout.Seconds()))
	fmt.Fprintf(&buf, "  ipv4: %v                 # 是否查询 IPv4 地址\n", cfg.Resolver.IPv4)
	fmt.Fprintf(&buf, "  ipv6: %v                 # 是否查询 IPv6 地址\n", cfg.Resolver.IPv6)
	fmt.Fprintf(&buf, "  cache_size: %d           # 缓存最大条目数（0 表示不限制）\n", cfg.Resolver.CacheSize)
	buf.WriteString("\n")

	// variables 配置
	buf.WriteString("# 自定义变量配置（全局变量，应用于所有虚拟主机）\n")
	buf.WriteString("variables:\n")
	buf.WriteString("  set: {}                  # 自定义变量集合，如 {app_name: \"lolly\"}\n")
	buf.WriteString("  # 注意：变量名只允许字母、数字、下划线，不能与内置变量冲突\n")
	buf.WriteString("  # 不能以 arg_、http_、cookie_ 开头（这些是动态变量前缀）\n")

	return buf.Bytes(), nil
}
