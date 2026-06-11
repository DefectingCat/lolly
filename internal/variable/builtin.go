// Package variable 提供 nginx 风格的内置变量支持，用于日志、代理和重写规则。
//
// 包含 18 个内置变量常量、动态变量获取函数（$arg_name、$http_name、$cookie_name）
// 以及 Context 池管理。
//
// 作者：xfy
package variable

import (
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/netutil"
)

// 内置变量常量
const (
	// VarHost 请求主机名变量。
	VarHost = "host"
	// VarRemoteAddr 客户端地址变量。
	VarRemoteAddr = "remote_addr"
	// VarRemotePort 客户端端口变量。
	VarRemotePort = "remote_port"
	// VarRequestURI 请求 URI 变量。
	VarRequestURI = "request_uri"
	// VarURI URI 变量。
	VarURI = "uri"
	// VarArgs 查询参数变量。
	VarArgs = "args"
	// VarRequestMethod 请求方法变量。
	VarRequestMethod = "request_method"
	// VarScheme 协议方案变量。
	VarScheme = "scheme"
	// VarServerName 服务器名称变量。
	VarServerName = "server_name"
	// VarServerPort 服务器端口变量。
	VarServerPort = "server_port"
	// VarStatus HTTP 状态码变量。
	VarStatus = "status"
	// VarBodyBytesSent 发送字节数变量。
	VarBodyBytesSent = "body_bytes_sent"
	// VarRequestTime 请求处理时间变量。
	VarRequestTime = "request_time"
	// VarTimeLocal 本地时间变量。
	VarTimeLocal = "time_local"
	// VarTimeISO8601 ISO8601 时间变量。
	VarTimeISO8601 = "time_iso8601"
	// VarRequestID 请求 ID 变量。
	VarRequestID = "request_id"
	// 上游变量
	// VarUpstreamAddr 上游地址变量。
	VarUpstreamAddr = "upstream_addr"
	// VarUpstreamStatus 上游状态码变量。
	VarUpstreamStatus = "upstream_status"
	// VarUpstreamResponseTime 上游响应时间变量。
	VarUpstreamResponseTime = "upstream_response_time"
	// VarUpstreamConnectTime 上游连接时间变量。
	VarUpstreamConnectTime = "upstream_connect_time"
	// VarUpstreamHeaderTime 上游响应头时间变量。
	VarUpstreamHeaderTime = "upstream_header_time"
)

// init 注册所有内置变量
func init() {
	// 1. $host - 请求 Host 头
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarHost,
		Description: "请求的主机名（Host 头）",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return string(ctx.Host())
		},
		GetterBytes: func(ctx *fasthttp.RequestCtx) []byte {
			// SAFETY: ctx.Host() returns []byte valid within request scope
			return ctx.Host()
		},
	})

	// 2. $remote_addr - 客户端 IP
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarRemoteAddr,
		Description: "客户端 IP 地址",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return netutil.FormatRemoteAddr(ctx)
		},
	})

	// 3. $remote_port - 客户端端口
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarRemotePort,
		Description: "客户端端口",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			addr := ctx.RemoteAddr()
			if addr == nil {
				return "-"
			}
			// 解析地址获取端口，优先用 FormatRemoteAddr 缓存的结果
			s := netutil.FormatRemoteAddr(ctx)
			for i := len(s) - 1; i >= 0; i-- {
				if s[i] == ':' {
					return s[i+1:]
				}
			}
			return "-"
		},
	})

	// 4. $request_uri - 原始请求 URI
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarRequestURI,
		Description: "原始请求 URI（包含查询参数）",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return string(ctx.RequestURI())
		},
		GetterBytes: func(ctx *fasthttp.RequestCtx) []byte {
			// SAFETY: ctx.RequestURI() returns []byte valid within request scope
			return ctx.RequestURI()
		},
	})

	// 5. $uri - 解码后的 URI 路径
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarURI,
		Description: "URI 路径（不包含查询参数）",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return string(ctx.Path())
		},
		GetterBytes: func(ctx *fasthttp.RequestCtx) []byte {
			// SAFETY: ctx.Path() returns []byte valid within request scope
			return ctx.Path()
		},
	})

	// 6. $args - 查询参数字符串
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarArgs,
		Description: "查询参数字符串",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return string(ctx.QueryArgs().QueryString())
		},
		GetterBytes: func(ctx *fasthttp.RequestCtx) []byte {
			// SAFETY: ctx.QueryArgs().QueryString() returns []byte valid within request scope
			return ctx.QueryArgs().QueryString()
		},
	})

	// 7. $request_method - 请求方法
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarRequestMethod,
		Description: "HTTP 请求方法",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			return string(ctx.Method())
		},
		GetterBytes: func(ctx *fasthttp.RequestCtx) []byte {
			// SAFETY: ctx.Method() returns []byte valid within request scope
			return ctx.Method()
		},
	})

	// 8. $scheme - 协议
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarScheme,
		Description: "协议（http 或 https）",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			if ctx.IsTLS() {
				return "https"
			}
			return "http"
		},
	})

	// 9. $server_name - 服务器名称
	// 注意：这个变量需要从 Context 获取
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarServerName,
		Description: "服务器名称",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			// 从 UserValue 获取，由外部设置
			if v := ctx.UserValue(VarServerName); v != nil {
				if s, ok := v.(string); ok {
					return s
				}
			}
			return "-"
		},
	})

	// 10. $server_port - 服务器端口
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarServerPort,
		Description: "服务器端口",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			addr := ctx.LocalAddr()
			if addr == nil {
				return "-"
			}
			s := addr.String()
			for i := len(s) - 1; i >= 0; i-- {
				if s[i] == ':' {
					return s[i+1:]
				}
			}
			return "-"
		},
	})

	// 11. $status - HTTP 状态码
	// 需要从 Context 获取
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarStatus,
		Description: "HTTP 响应状态码",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			if v := ctx.UserValue(VarStatus); v != nil {
				if i, ok := v.(int); ok {
					return strconv.Itoa(i)
				}
			}
			return "-"
		},
	})

	// 12. $body_bytes_sent - 响应体大小
	// 需要从 Context 获取
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarBodyBytesSent,
		Description: "发送的响应体字节数",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			if v := ctx.UserValue(VarBodyBytesSent); v != nil {
				if i, ok := v.(int64); ok {
					return strconv.FormatInt(i, 10)
				}
			}
			return "0"
		},
	})

	// 13. $request_time - 请求处理时间
	// 需要从 Context 获取
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarRequestTime,
		Description: "请求处理时间（秒）",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			if v := ctx.UserValue(VarRequestTime); v != nil {
				if i, ok := v.(int64); ok {
					return formatRequestTime(i)
				}
			}
			return "0.000"
		},
	})

	// 14. $time_local - 本地时间
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarTimeLocal,
		Description: "本地时间（格式：02/Jan/2024:15:04:05 +0800）",
		Getter: func(_ *fasthttp.RequestCtx) string {
			return time.Now().Format("02/Jan/2006:15:04:05 +0800")
		},
	})

	// 15. $time_iso8601 - ISO8601 时间
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarTimeISO8601,
		Description: "ISO8601 格式时间",
		Getter: func(_ *fasthttp.RequestCtx) string {
			return time.Now().Format(time.RFC3339)
		},
	})

	// 16. $request_id - 唯一请求 ID
	RegisterBuiltin(&BuiltinVariable{
		Name:        VarRequestID,
		Description: "唯一请求标识符",
		Getter: func(ctx *fasthttp.RequestCtx) string {
			// 先从 UserValue 获取，如果没有则生成
			if v := ctx.UserValue(VarRequestID); v != nil {
				if s, ok := v.(string); ok {
					return s
				}
			}
			return uuid.New().String()
		},
	})
}

// formatRequestTime 格式化请求处理时间
func formatRequestTime(ns int64) string {
	// 转换为秒，保留3位小数
	sec := float64(ns) / 1e9
	return strconv.FormatFloat(sec, 'f', 3, 64)
}

// SetResponseInfoInContext 在 fasthttp.RequestCtx 中设置响应信息
// 用于在 builtin getter 中获取 status、body_bytes_sent、request_time
func SetResponseInfoInContext(ctx *fasthttp.RequestCtx, status int, bodySize int64, durationNs int64) {
	ctx.SetUserValue(VarStatus, status)
	ctx.SetUserValue(VarBodyBytesSent, bodySize)
	ctx.SetUserValue(VarRequestTime, durationNs)
}
