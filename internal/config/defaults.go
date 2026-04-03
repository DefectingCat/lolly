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
			Static: StaticConfig{
				Root:  "/var/www/html",
				Index: []string{"index.html", "index.htm"},
			},
			SSL: SSLConfig{
				Protocols:    []string{"TLSv1.2", "TLSv1.3"},
				OCSPStapling: false,
				HSTS: HSTSConfig{
					MaxAge:            31536000,
					IncludeSubDomains: true,
					Preload:           false,
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
				GzipStaticExtensions: []string{".gz", ".br"},
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
				MaxEntries:  10000,
				MaxSize:     256 * 1024 * 1024, // 256MB
				Inactive:    20 * time.Second,
				LRUEviction: true,
			},
			Transport: TransportConfig{
				MaxIdleConns:        100,
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
		},
		HTTP3: HTTP3Config{
			Enabled:     false,
			Listen:      ":443",
			MaxStreams:  100,
			IdleTimeout: 60 * time.Second,
			Enable0RTT:  false,
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
	buf.WriteString("\n")

	// static 配置
	buf.WriteString("  # 静态文件服务配置\n")
	buf.WriteString("  static:\n")
	fmt.Fprintf(&buf, "    root: \"%s\"   # 静态文件根目录\n", cfg.Server.Static.Root)
	buf.WriteString("    index:                  # 索引文件\n")
	for _, idx := range cfg.Server.Static.Index {
		fmt.Fprintf(&buf, "      - \"%s\"\n", idx)
	}
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
	buf.WriteString("  #   ciphers: []                    # 加密套件（仅 TLS 1.2 有效）\n")
	fmt.Fprintf(&buf, "  #   ocsp_stapling: %v              # OCSP Stapling\n", cfg.Server.SSL.OCSPStapling)
	buf.WriteString("  #   hsts:                          # HTTP Strict Transport Security\n")
	fmt.Fprintf(&buf, "  #     max_age: %d                  # 过期时间（秒）\n", cfg.Server.SSL.HSTS.MaxAge)
	fmt.Fprintf(&buf, "  #     include_sub_domains: %v      # 包含子域名\n", cfg.Server.SSL.HSTS.IncludeSubDomains)
	fmt.Fprintf(&buf, "  #     preload: %v                  # 加入 HSTS 预加载列表\n", cfg.Server.SSL.HSTS.Preload)
	buf.WriteString("\n")

	// security 配置
	buf.WriteString("  # 安全配置\n")
	buf.WriteString("  security:\n")
	buf.WriteString("    # IP 访问控制\n")
	buf.WriteString("    access:\n")
	buf.WriteString("      allow: []                   # 允许的 IP/CIDR 列表\n")
	buf.WriteString("      deny: []                    # 拒绝的 IP/CIDR 列表\n")
	fmt.Fprintf(&buf, "      default: \"%s\"             # 默认动作（有效值: allow, deny）\n", cfg.Server.Security.Access.Default)
	buf.WriteString("\n")
	buf.WriteString("    # 速率限制\n")
	buf.WriteString("    rate_limit:\n")
	buf.WriteString(fmt.Sprintf("      request_rate: %d            # 每秒请求数（0 表示不限制）\n", cfg.Server.Security.RateLimit.RequestRate))
	buf.WriteString(fmt.Sprintf("      burst: %d                   # 突发上限\n", cfg.Server.Security.RateLimit.Burst))
	buf.WriteString(fmt.Sprintf("      conn_limit: %d              # 连接数限制\n", cfg.Server.Security.RateLimit.ConnLimit))
	buf.WriteString(fmt.Sprintf("      key: \"%s\"                  # 限流 key 来源（有效值: ip, header）\n", cfg.Server.Security.RateLimit.Key))
	buf.WriteString(fmt.Sprintf("      algorithm: \"%s\"             # 限流算法（有效值: token_bucket, sliding_window）\n", cfg.Server.Security.RateLimit.Algorithm))
	buf.WriteString(fmt.Sprintf("      sliding_window_mode: \"%s\"   # 滑动窗口模式（有效值: approximate, precise，仅 algorithm=sliding_window 时有效）\n", cfg.Server.Security.RateLimit.SlidingWindowMode))
	buf.WriteString(fmt.Sprintf("      sliding_window: %d          # 滑动窗口大小（秒，仅 algorithm=sliding_window 时有效）\n", cfg.Server.Security.RateLimit.SlidingWindow))
	buf.WriteString("\n")
	buf.WriteString("    # 认证配置（type 为空时禁用）\n")
	buf.WriteString("    auth:\n")
	buf.WriteString("      type: \"\"                    # 认证类型（有效值: basic，空表示禁用）\n")
	buf.WriteString(fmt.Sprintf("      require_tls: %v             # 启用时强制 HTTPS\n", cfg.Server.Security.Auth.RequireTLS))
	buf.WriteString(fmt.Sprintf("      algorithm: \"%s\"            # 密码哈希算法（有效值: bcrypt, argon2id）\n", cfg.Server.Security.Auth.Algorithm))
	buf.WriteString("      users: []                   # 用户列表\n")
	buf.WriteString(fmt.Sprintf("      realm: \"%s\"                # 认证域\n", cfg.Server.Security.Auth.Realm))
	buf.WriteString(fmt.Sprintf("      min_password_length: %d     # 密码最小长度\n", cfg.Server.Security.Auth.MinPasswordLength))
	buf.WriteString("\n")
	buf.WriteString("    # 安全头部\n")
	buf.WriteString("    headers:\n")
	buf.WriteString(fmt.Sprintf("      x_frame_options: \"%s\"        # 防止点击劫持（有效值: DENY, SAMEORIGIN, 空表示禁用）\n", cfg.Server.Security.Headers.XFrameOptions))
	buf.WriteString(fmt.Sprintf("      x_content_type_options: \"%s\" # 防止 MIME 嗅探\n", cfg.Server.Security.Headers.XContentTypeOptions))
	buf.WriteString(fmt.Sprintf("      referrer_policy: \"%s\"        # 引用策略（有效值: no-referrer, no-referrer-when-downgrade, origin, origin-when-cross-origin, same-origin, strict-origin, strict-origin-when-cross-origin, unsafe-url）\n", cfg.Server.Security.Headers.ReferrerPolicy))
	buf.WriteString("      # content_security_policy: \"default-src 'self'\"  # 内容安全策略 CSP\n")
	buf.WriteString("      # permissions_policy: \"geolocation=(), microphone=()\"  # 权限策略\n")
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
	buf.WriteString(fmt.Sprintf("    type: \"%s\"            # 压缩类型（有效值: gzip, brotli, both，空表示禁用）\n", cfg.Server.Compression.Type))
	buf.WriteString(fmt.Sprintf("    level: %d              # 压缩级别（范围 1-9，值越大压缩率越高但速度越慢）\n", cfg.Server.Compression.Level))
	buf.WriteString(fmt.Sprintf("    min_size: %d        # 最小压缩大小（字节，小于此值不压缩）\n", cfg.Server.Compression.MinSize))
	buf.WriteString(fmt.Sprintf("    gzip_static: %v        # 启用预压缩文件支持（自动查找 .gz/.br 文件）\n", cfg.Server.Compression.GzipStatic))
	buf.WriteString("    gzip_static_extensions:  # 预压缩文件扩展名\n")
	for _, ext := range cfg.Server.Compression.GzipStaticExtensions {
		buf.WriteString(fmt.Sprintf("      - \"%s\"\n", ext))
	}
	buf.WriteString("    types:                 # 可压缩的 MIME 类型\n")
	for _, t := range cfg.Server.Compression.Types {
		buf.WriteString(fmt.Sprintf("      - \"%s\"\n", t))
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
	buf.WriteString("#     static:                         # 静态文件配置\n")
	buf.WriteString("#       root: /var/www/api\n")
	buf.WriteString("#       index: [index.html]\n")
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
	buf.WriteString("#       root: /var/www/static\n")
	buf.WriteString("#       index: [index.html, index.htm]\n")
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
	buf.WriteString("\n")

	// logging 配置
	buf.WriteString("# 日志配置\n")
	buf.WriteString("logging:\n")
	buf.WriteString(fmt.Sprintf("  format: \"%s\"           # 全局日志格式（有效值: text, json），控制启动/停止日志格式\n", cfg.Logging.Format))
	buf.WriteString("  access:\n")
	buf.WriteString("    path: \"\"                   # 日志文件路径（空表示输出到 stdout）\n")
	buf.WriteString(fmt.Sprintf("    format: '%s'  # 访问日志格式，近似 nginx combined\n", cfg.Logging.Access.Format))
	buf.WriteString("    # 支持变量: $remote_addr, $remote_user, $request, $status, $body_bytes_sent, $request_time, $http_referer, $http_user_agent, $time\n")
	buf.WriteString("    # 特殊值 \"json\" 输出结构化 JSON\n")
	buf.WriteString("  error:\n")
	buf.WriteString("    path: \"\"                   # 日志文件路径（空表示输出到 stderr）\n")
	buf.WriteString(fmt.Sprintf("    level: \"%s\"           # 日志级别（有效值: debug, info, warn, error，级别越高日志越少）\n", cfg.Logging.Error.Level))
	buf.WriteString("\n")

	// performance 配置
	buf.WriteString("# 性能配置\n")
	buf.WriteString("performance:\n")
	buf.WriteString("  goroutine_pool:              # Goroutine 池（处理并发请求）\n")
	buf.WriteString(fmt.Sprintf("    enabled: %v             # 是否启用\n", cfg.Performance.GoroutinePool.Enabled))
	buf.WriteString(fmt.Sprintf("    max_workers: %d          # 最大 worker 数\n", cfg.Performance.GoroutinePool.MaxWorkers))
	buf.WriteString(fmt.Sprintf("    min_workers: %d          # 最小 worker 数（预热）\n", cfg.Performance.GoroutinePool.MinWorkers))
	buf.WriteString(fmt.Sprintf("    idle_timeout: %ds        # 空闲超时\n", int(cfg.Performance.GoroutinePool.IdleTimeout.Seconds())))
	buf.WriteString("  file_cache:                  # 静态文件缓存\n")
	buf.WriteString(fmt.Sprintf("    max_entries: %d          # 最大缓存条目\n", cfg.Performance.FileCache.MaxEntries))
	buf.WriteString(fmt.Sprintf("    max_size: %d              # 内存上限（字节，%dMB）\n", cfg.Performance.FileCache.MaxSize, cfg.Performance.FileCache.MaxSize/1024/1024))
	buf.WriteString(fmt.Sprintf("    inactive: %ds             # 未访问淘汰时间\n", int(cfg.Performance.FileCache.Inactive.Seconds())))
	buf.WriteString(fmt.Sprintf("    lru_eviction: %v          # 启用 LRU 淘汰\n", cfg.Performance.FileCache.LRUEviction))
	buf.WriteString("  transport:                   # HTTP Transport 连接池\n")
	buf.WriteString(fmt.Sprintf("    max_idle_conns: %d            # 最大空闲连接\n", cfg.Performance.Transport.MaxIdleConns))
	buf.WriteString(fmt.Sprintf("    max_idle_conns_per_host: %d   # 每主机空闲连接\n", cfg.Performance.Transport.MaxIdleConnsPerHost))
	buf.WriteString(fmt.Sprintf("    idle_conn_timeout: %ds        # 空闲超时\n", int(cfg.Performance.Transport.IdleConnTimeout.Seconds())))
	buf.WriteString(fmt.Sprintf("    max_conns_per_host: %d        # 每主机最大连接（0 表示不限制）\n", cfg.Performance.Transport.MaxConnsPerHost))
	buf.WriteString("\n")

	// HTTP3 配置
	buf.WriteString("# HTTP/3 (QUIC) 配置（需要 SSL 证书）\n")
	buf.WriteString("http3:\n")
	buf.WriteString(fmt.Sprintf("  enabled: %v              # 是否启用 HTTP/3\n", cfg.HTTP3.Enabled))
	buf.WriteString(fmt.Sprintf("  listen: \"%s\"             # UDP 监听地址\n", cfg.HTTP3.Listen))
	buf.WriteString(fmt.Sprintf("  max_streams: %d          # 最大并发流\n", cfg.HTTP3.MaxStreams))
	buf.WriteString(fmt.Sprintf("  idle_timeout: %ds        # 空闲超时\n", int(cfg.HTTP3.IdleTimeout.Seconds())))
	buf.WriteString(fmt.Sprintf("  enable_0rtt: %v          # 启用 0-RTT（早期数据，可能存在安全风险）\n", cfg.HTTP3.Enable0RTT))
	buf.WriteString("\n")

	// monitoring 配置
	buf.WriteString("# 监控配置\n")
	buf.WriteString("monitoring:\n")
	buf.WriteString("  status:\n")
	buf.WriteString(fmt.Sprintf("    path: \"%s\"        # 状态端点路径\n", cfg.Monitoring.Status.Path))
	buf.WriteString("    allow:                 # 允许访问的 IP\n")
	for _, ip := range cfg.Monitoring.Status.Allow {
		buf.WriteString(fmt.Sprintf("      - \"%s\"\n", ip))
	}

	return buf.Bytes(), nil
}
