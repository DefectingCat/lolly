// Package config 提供 YAML 配置文件的解析、验证和默认配置生成功能。
//
// 包含缓存配置相关的结构体，用于控制代理缓存行为。
//
// 作者：xfy
package config

import "time"

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
	Vary                    []string      `yaml:"vary"`                      // 参与缓存键的 Vary 请求头
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
