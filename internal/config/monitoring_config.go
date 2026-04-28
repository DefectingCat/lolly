package config

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
