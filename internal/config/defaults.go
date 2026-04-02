// Package config 提供 YAML 配置文件的解析、验证和默认配置生成功能。
package config

import (
	"bytes"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultConfig 返回带默认值的配置结构体。
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Listen: ":8080",
			Name:   "localhost",
			Static: StaticConfig{
				Root:  "/var/www/html",
				Index: []string{"index.html", "index.htm"},
			},
			SSL: SSLConfig{
				Protocols: []string{"TLSv1.2", "TLSv1.3"},
				HSTS: HSTSConfig{
					MaxAge:            31536000,
					IncludeSubDomains: true,
					Preload:           false,
				},
			},
			Security: SecurityConfig{
				Headers: SecurityHeaders{
					XFrameOptions:       "DENY",
					XContentTypeOptions: "nosniff",
					ReferrerPolicy:      "strict-origin-when-cross-origin",
				},
				Auth: AuthConfig{
					RequireTLS: true,
					Algorithm:  "bcrypt",
					Realm:      "Restricted Area",
				},
			},
			Compression: CompressionConfig{
				Type:    "gzip",
				Level:   6,
				MinSize: 1024,
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
			Access: AccessLogConfig{
				Format: "$remote_addr - $request - $status - $body_bytes_sent",
			},
			Error: ErrorLogConfig{
				Level: "info",
			},
		},
		Performance: PerformanceConfig{
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
			},
		},
		Monitoring: MonitoringConfig{
			Status: StatusConfig{
				Path:  "/_status",
				Allow: []string{"127.0.0.1"},
			},
		},
	}
}

// GenerateConfigYAML 生成带注释的默认配置 YAML。
func GenerateConfigYAML(cfg *Config) ([]byte, error) {
	// 手动构建带注释的 YAML
	var buf bytes.Buffer

	buf.WriteString("# Lolly 配置文件\n")
	buf.WriteString("# 文档: https://github.com/xfy/lolly\n")
	buf.WriteString("\n")

	// server 配置
	buf.WriteString("# 服务器配置（单服务器模式）\n")
	buf.WriteString("server:\n")
	buf.WriteString(fmt.Sprintf("  listen: \"%s\"           # 监听地址\n", cfg.Server.Listen))
	buf.WriteString(fmt.Sprintf("  name: \"%s\"             # 服务器名称（虚拟主机匹配）\n", cfg.Server.Name))
	buf.WriteString("\n")

	// static 配置
	buf.WriteString("  # 静态文件服务配置\n")
	buf.WriteString("  static:\n")
	buf.WriteString(fmt.Sprintf("    root: \"%s\"   # 静态文件根目录\n", cfg.Server.Static.Root))
	buf.WriteString("    index:                  # 索引文件\n")
	for _, idx := range cfg.Server.Static.Index {
		buf.WriteString(fmt.Sprintf("      - \"%s\"\n", idx))
	}
	buf.WriteString("\n")

	// proxy 配置示例
	buf.WriteString("  # 反向代理配置（示例）\n")
	buf.WriteString("  # proxy:\n")
	buf.WriteString("  #   - path: /api         # 匹配路径前缀\n")
	buf.WriteString("  #     targets:           # 后端目标列表\n")
	buf.WriteString("  #       - url: http://backend1:8080\n")
	buf.WriteString("  #         weight: 3      # 权重（加权轮询）\n")
	buf.WriteString("  #       - url: http://backend2:8080\n")
	buf.WriteString("  #         weight: 1\n")
	buf.WriteString("  #     load_balance: weighted_round_robin  # 负载均衡算法\n")
	buf.WriteString("  #     health_check:      # 健康检查\n")
	buf.WriteString("  #       interval: 10s\n")
	buf.WriteString("  #       path: /health\n")
	buf.WriteString("  #       timeout: 5s\n")
	buf.WriteString("\n")

	// SSL 配置
	buf.WriteString("  # SSL/TLS 配置\n")
	buf.WriteString("  # ssl:\n")
	buf.WriteString("  #   cert: /path/to/cert.pem      # 证书文件\n")
	buf.WriteString("  #   key: /path/to/key.pem        # 私钥文件\n")
	buf.WriteString("  #   protocols:                   # TLS 版本（默认安全）\n")
	for _, proto := range cfg.Server.SSL.Protocols {
		buf.WriteString(fmt.Sprintf("  #     - \"%s\"\n", proto))
	}
	buf.WriteString("  #   hsts:                        # HTTP Strict Transport Security\n")
	buf.WriteString(fmt.Sprintf("  #     max_age: %d             # 过期时间（秒）\n", cfg.Server.SSL.HSTS.MaxAge))
	buf.WriteString(fmt.Sprintf("  #     include_sub_domains: %v  # 包含子域名\n", cfg.Server.SSL.HSTS.IncludeSubDomains))
	buf.WriteString("\n")

	// security 配置
	buf.WriteString("  # 安全配置\n")
	buf.WriteString("  security:\n")
	buf.WriteString("    # IP 访问控制\n")
	buf.WriteString("    # access:\n")
	buf.WriteString("    #   allow: [192.168.1.0/24]   # 允许的 IP/CIDR\n")
	buf.WriteString("    #   deny: [192.168.2.100/32]  # 拒绝的 IP/CIDR\n")
	buf.WriteString("    #   default: deny             # 默认动作\n")
	buf.WriteString("\n")
	buf.WriteString("    # 速率限制\n")
	buf.WriteString("    # rate_limit:\n")
	buf.WriteString("    #   request_rate: 100         # 每秒请求数\n")
	buf.WriteString("    #   burst: 200                # 突发上限\n")
	buf.WriteString("    #   conn_limit: 1000          # 连接数限制\n")
	buf.WriteString("\n")
	buf.WriteString("    # 认证配置（启用时强制 HTTPS）\n")
	buf.WriteString("    # auth:\n")
	buf.WriteString("    #   type: basic\n")
	buf.WriteString("    #   users:\n")
	buf.WriteString("    #     - name: admin\n")
	buf.WriteString("    #       password: $2b$12$...  # bcrypt 哈希\n")
	buf.WriteString("    #   require_tls: true\n")
	buf.WriteString("\n")
	buf.WriteString("    # 安全头部（默认值）\n")
	buf.WriteString("    headers:\n")
	buf.WriteString(fmt.Sprintf("      x_frame_options: \"%s\"        # 防止点击劫持\n", cfg.Server.Security.Headers.XFrameOptions))
	buf.WriteString(fmt.Sprintf("      x_content_type_options: \"%s\" # 防止 MIME 嗅探\n", cfg.Server.Security.Headers.XContentTypeOptions))
	buf.WriteString(fmt.Sprintf("      referrer_policy: \"%s\"        # 引用策略\n", cfg.Server.Security.Headers.ReferrerPolicy))
	buf.WriteString("      # content_security_policy: \"default-src 'self'\"  # CSP（推荐配置）\n")
	buf.WriteString("\n")

	// rewrite 配置示例
	buf.WriteString("  # URL 重写规则（示例）\n")
	buf.WriteString("  # rewrite:\n")
	buf.WriteString("  #   - pattern: \"^/old/(.*)$\"\n")
	buf.WriteString("  #     replacement: /new/$1\n")
	buf.WriteString("  #     flag: permanent  # 301 重定向\n")
	buf.WriteString("\n")

	// compression 配置
	buf.WriteString("  # 响应压缩配置\n")
	buf.WriteString("  compression:\n")
	buf.WriteString(fmt.Sprintf("    type: \"%s\"            # 压缩类型: gzip, brotli, both\n", cfg.Server.Compression.Type))
	buf.WriteString(fmt.Sprintf("    level: %d              # 压缩级别 (1-9)\n", cfg.Server.Compression.Level))
	buf.WriteString(fmt.Sprintf("    min_size: %d        # 最小压缩大小（字节）\n", cfg.Server.Compression.MinSize))
	buf.WriteString("    types:                 # 可压缩的 MIME 类型\n")
	for _, t := range cfg.Server.Compression.Types {
		buf.WriteString(fmt.Sprintf("      - \"%s\"\n", t))
	}
	buf.WriteString("\n")

	// servers 配置说明
	buf.WriteString("# 多虚拟主机模式（可选）\n")
	buf.WriteString("# servers:\n")
	buf.WriteString("#   - listen: \":8080\"\n")
	buf.WriteString("#     name: \"api.example.com\"\n")
	buf.WriteString("#     proxy:\n")
	buf.WriteString("#       - path: /api\n")
	buf.WriteString("#         targets: [http://backend:8080]\n")
	buf.WriteString("#   - listen: \":8443\"\n")
	buf.WriteString("#     name: \"static.example.com\"\n")
	buf.WriteString("#     static:\n")
	buf.WriteString("#       root: /var/www/static\n")
	buf.WriteString("\n")

	// logging 配置
	buf.WriteString("# 日志配置\n")
	buf.WriteString("logging:\n")
	buf.WriteString("  access:\n")
	buf.WriteString(fmt.Sprintf("    format: \"%s\"  # 日志格式\n", cfg.Logging.Access.Format))
	buf.WriteString("    # path: /var/log/lolly/access.log  # 日志文件路径\n")
	buf.WriteString("  error:\n")
	buf.WriteString(fmt.Sprintf("    level: \"%s\"           # 日志级别: debug, info, warn, error\n", cfg.Logging.Error.Level))
	buf.WriteString("    # path: /var/log/lolly/error.log\n")
	buf.WriteString("\n")

	// performance 配置
	buf.WriteString("# 性能配置\n")
	buf.WriteString("performance:\n")
	buf.WriteString("  file_cache:             # 静态文件缓存\n")
	buf.WriteString(fmt.Sprintf("    max_entries: %d    # 最大缓存条目\n", cfg.Performance.FileCache.MaxEntries))
	buf.WriteString(fmt.Sprintf("    max_size: %dMB      # 内存上限\n", cfg.Performance.FileCache.MaxSize/1024/1024))
	buf.WriteString(fmt.Sprintf("    inactive: %ds         # 未访问淘汰时间\n", int(cfg.Performance.FileCache.Inactive.Seconds())))
	buf.WriteString(fmt.Sprintf("    lru_eviction: %v     # 启用 LRU 淘汰\n", cfg.Performance.FileCache.LRUEviction))
	buf.WriteString("  transport:             # HTTP Transport 连接池\n")
	buf.WriteString(fmt.Sprintf("    max_idle_conns: %d         # 最大空闲连接\n", cfg.Performance.Transport.MaxIdleConns))
	buf.WriteString(fmt.Sprintf("    max_idle_conns_per_host: %d # 每主机空闲连接\n", cfg.Performance.Transport.MaxIdleConnsPerHost))
	buf.WriteString(fmt.Sprintf("    idle_conn_timeout: %ds     # 空闲超时\n", int(cfg.Performance.Transport.IdleConnTimeout.Seconds())))
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

// GenerateSimpleYAML 生成简洁的 YAML（不带注释），用于程序内部使用。
func GenerateSimpleYAML(cfg *Config) ([]byte, error) {
	return yaml.Marshal(cfg)
}