package config

import "time"

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
