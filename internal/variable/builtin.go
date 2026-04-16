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
)

// 内置变量常量
const (
	VarHost          = "host"
	VarRemoteAddr    = "remote_addr"
	VarRemotePort    = "remote_port"
	VarRequestURI    = "request_uri"
	VarURI           = "uri"
	VarArgs          = "args"
	VarRequestMethod = "request_method"
	VarScheme        = "scheme"
	VarServerName    = "server_name"
	VarServerPort    = "server_port"
	VarStatus        = "status"
	VarBodyBytesSent = "body_bytes_sent"
	VarRequestTime   = "request_time"
	VarTimeLocal     = "time_local"
	VarTimeISO8601   = "time_iso8601"
	VarRequestID     = "request_id"
	// 上游变量
	VarUpstreamAddr         = "upstream_addr"
	VarUpstreamStatus       = "upstream_status"
	VarUpstreamResponseTime = "upstream_response_time"
	VarUpstreamConnectTime  = "upstream_connect_time"
	VarUpstreamHeaderTime   = "upstream_header_time"
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
			addr := ctx.RemoteAddr()
			if addr == nil {
				return "-"
			}
			return addr.String()
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
			// 解析地址获取端口
			s := addr.String()
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

// GetArgVariable 获取查询参数变量（动态变量 $arg_name）
func GetArgVariable(ctx *fasthttp.RequestCtx, name string) string {
	return string(ctx.URI().QueryArgs().Peek(name))
}

// GetHTTPVariable 获取 HTTP 头变量（动态变量 $http_name）
func GetHTTPVariable(ctx *fasthttp.RequestCtx, name string) string {
	// 将下划线转换为连字符，并规范化头名
	headerName := normalizeHeaderName(name)
	return string(ctx.Request.Header.Peek(headerName))
}

// GetCookieVariable 获取 Cookie 变量（动态变量 $cookie_name）
func GetCookieVariable(ctx *fasthttp.RequestCtx, name string) string {
	return string(ctx.Request.Header.Cookie(name))
}

// normalizeHeaderName 规范化 HTTP 头名
func normalizeHeaderName(name string) string {
	// 简单处理：将 _ 替换为 -，并首字母大写
	if name == "" {
		return name
	}

	var result []byte
	upper := true
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' {
			result = append(result, '-')
			upper = true
		} else if upper {
			if c >= 'a' && c <= 'z' {
				result = append(result, c-'a'+'A')
			} else {
				result = append(result, c)
			}
			upper = false
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}

// SetResponseInfoInContext 在 fasthttp.RequestCtx 中设置响应信息
// 用于在 builtin getter 中获取 status、body_bytes_sent、request_time
func SetResponseInfoInContext(ctx *fasthttp.RequestCtx, status int, bodySize int64, durationNs int64) {
	ctx.SetUserValue(VarStatus, status)
	ctx.SetUserValue(VarBodyBytesSent, bodySize)
	ctx.SetUserValue(VarRequestTime, durationNs)
}

// SetServerNameInContext 在 fasthttp.RequestCtx 中设置服务器名称
func SetServerNameInContext(ctx *fasthttp.RequestCtx, name string) {
	ctx.SetUserValue(VarServerName, name)
}

// SetRequestIDInContext 在 fasthttp.RequestCtx 中设置请求 ID
func SetRequestIDInContext(ctx *fasthttp.RequestCtx, id string) {
	ctx.SetUserValue(VarRequestID, id)
}

// BuiltinVarNames 返回所有内置变量名称列表
func BuiltinVarNames() []string {
	names := make([]string, 0, len(builtinVars))
	for name := range builtinVars {
		names = append(names, name)
	}
	return names
}
