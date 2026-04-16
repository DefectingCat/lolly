// Package lua 提供 ngx.var API 实现
package lua

import (
	"strconv"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// ngxVarAPI ngx.var API 实现
type ngxVarAPI struct {
	// 请求上下文
	ctx *fasthttp.RequestCtx

	// 变量存储（用于自定义变量）
	store map[string]string
}

// newNgxVarAPI 创建 ngx.var API 实例
func newNgxVarAPI(ctx *fasthttp.RequestCtx) *ngxVarAPI {
	return &ngxVarAPI{
		ctx:   ctx,
		store: make(map[string]string),
	}
}

// RegisterNgxVarAPI 在 Lua 状态机中注册 ngx.var API
// 使用元表实现动态读写：ngx.var.key 和 ngx.var[key]
func RegisterNgxVarAPI(L *glua.LState, api *ngxVarAPI, ngxTable *glua.LTable) {
	// 创建 ngx.var 表（使用元表实现动态访问）
	ngxVar := L.NewTable()

	// 创建元表
	mt := L.NewTable()

	// __index 元方法：读取变量
	mt.RawSetString("__index", L.NewFunction(api.luaVarIndex))

	// __newindex 元方法：设置变量
	mt.RawSetString("__newindex", L.NewFunction(api.luaVarNewIndex))

	// 设置元表
	L.SetMetatable(ngxVar, mt)

	// 将 ngx.var 添加到 ngx 表
	ngxTable.RawSetString("var", ngxVar)
}

// luaVarIndex 实现 ngx.var[key] 读取
// Lua 调用: local value = ngx.var.key 或 ngx.var[key]
func (api *ngxVarAPI) luaVarIndex(L *glua.LState) int {
	// 第一个参数是表本身（ngx.var）
	// 第二个参数是键名
	key := L.CheckString(2)

	// 1. 先查自定义变量存储
	if value, ok := api.store[key]; ok {
		L.Push(glua.LString(value))
		return 1
	}

	// 2. 从 fasthttp RequestCtx 获取变量
	// 某些变量需要返回数值类型（如 request_length）
	lv := api.getVariableLua(key)
	if lv != nil {
		L.Push(lv)
		return 1
	}

	// 3. 未找到变量，返回 nil
	L.Push(glua.LNil)
	return 1
}

// luaVarNewIndex 实现 ngx.var[key] = value 写入
// Lua 调用: ngx.var.key = value 或 ngx.var[key] = value
// 注意：Lua 的 nil 会被转换为空字符串存储
func (api *ngxVarAPI) luaVarNewIndex(L *glua.LState) int {
	// 第一个参数是表本身（ngx.var）
	// 第二个参数是键名
	// 第三个参数是值
	key := L.CheckString(2)
	value := L.OptString(3, "")

	// 存储到自定义变量存储（nil 会转换为空字符串）
	api.store[key] = value

	return 0
}

// getVariableLua 从 fasthttp RequestCtx 获取变量值，返回 Lua 类型
// 支持常见的 nginx 变量，某些变量返回数值类型
func (api *ngxVarAPI) getVariableLua(name string) glua.LValue {
	if api.ctx == nil {
		return nil
	}

	switch name {
	// HTTP 请求相关 - 数值类型
	case "request_length":
		return glua.LNumber(api.ctx.Request.Header.ContentLength())

	// HTTP 请求相关 - 字符串类型
	case "request_method":
		return glua.LString(string(api.ctx.Method()))
	case "request_uri":
		return glua.LString(string(api.ctx.RequestURI()))
	case "uri":
		return glua.LString(string(api.ctx.URI().Path()))
	case "document_uri":
		return glua.LString(string(api.ctx.URI().Path()))
	case "query_string", "args":
		return glua.LString(string(api.ctx.URI().QueryString()))
	case "server_protocol", "protocol":
		return glua.LString(string(api.ctx.Request.Header.Protocol()))
	case "scheme":
		return glua.LString(string(api.ctx.URI().Scheme()))
	case "request_time":
		// 简化实现，返回空字符串
		return glua.LString("")

	// 请求头相关
	case "http_host":
		return glua.LString(string(api.ctx.Host()))
	case "http_user_agent", "http_user-agent":
		return glua.LString(string(api.ctx.UserAgent()))
	case "http_referer":
		return glua.LString(string(api.ctx.Referer()))
	case "http_accept":
		return glua.LString(string(api.ctx.Request.Header.Peek("Accept")))
	case "http_accept_encoding", "http_accept-encoding":
		return glua.LString(string(api.ctx.Request.Header.Peek("Accept-Encoding")))
	case "http_accept_language", "http_accept-language":
		return glua.LString(string(api.ctx.Request.Header.Peek("Accept-Language")))
	case "http_connection":
		return glua.LString(string(api.ctx.Request.Header.Peek("Connection")))
	case "http_content_type", "http_content-type":
		return glua.LString(string(api.ctx.Request.Header.ContentType()))
	case "http_content_length", "http_content-length":
		return glua.LString(string(api.ctx.Request.Header.Peek("Content-Length")))

	// 客户端信息
	case "remote_addr":
		return glua.LString(api.ctx.RemoteAddr().String())
	case "remote_port":
		addr := api.ctx.RemoteAddr()
		if addr != nil {
			// 简化处理，实际可能需要解析端口
			return glua.LString("")
		}
		return glua.LString("")
	case "binary_remote_addr":
		return glua.LString("")

	// 服务器信息
	case "server_addr":
		addr := api.ctx.LocalAddr()
		if addr != nil {
			return glua.LString(addr.String())
		}
		return glua.LString("")
	case "server_port":
		return glua.LString("")
	case "server_name":
		return glua.LString(string(api.ctx.Host()))

	// URI 参数
	case "arg_":
		// 获取所有参数
		return glua.LString(string(api.ctx.URI().QueryString()))
	default:
		// 检查是否是 arg_ 开头的参数
		if len(name) > 4 && name[:4] == "arg_" {
			paramName := name[4:]
			return glua.LString(string(api.ctx.QueryArgs().Peek(paramName)))
		}
		// 检查是否是 http_ 开头的请求头
		if len(name) > 5 && name[:5] == "http_" {
			headerName := name[5:]
			return glua.LString(string(api.ctx.Request.Header.Peek(headerName)))
		}
		return nil
	}
}

// getVariable 从 fasthttp RequestCtx 获取变量值（字符串形式）
// 用于 Go 层调用
func (api *ngxVarAPI) getVariable(name string) string {
	if api.ctx == nil {
		return ""
	}

	switch name {
	case "request_method":
		return string(api.ctx.Method())
	case "request_uri":
		return string(api.ctx.RequestURI())
	case "uri":
		return string(api.ctx.URI().Path())
	case "document_uri":
		return string(api.ctx.URI().Path())
	case "query_string", "args":
		return string(api.ctx.URI().QueryString())
	case "server_protocol", "protocol":
		return string(api.ctx.Request.Header.Protocol())
	case "scheme":
		return string(api.ctx.URI().Scheme())
	case "request_length":
		return strconv.Itoa(api.ctx.Request.Header.ContentLength())
	case "request_time":
		return ""
	case "http_host":
		return string(api.ctx.Host())
	case "http_user_agent", "http_user-agent":
		return string(api.ctx.UserAgent())
	case "http_referer":
		return string(api.ctx.Referer())
	case "http_accept":
		return string(api.ctx.Request.Header.Peek("Accept"))
	case "http_accept_encoding", "http_accept-encoding":
		return string(api.ctx.Request.Header.Peek("Accept-Encoding"))
	case "http_accept_language", "http_accept-language":
		return string(api.ctx.Request.Header.Peek("Accept-Language"))
	case "http_connection":
		return string(api.ctx.Request.Header.Peek("Connection"))
	case "http_content_type", "http_content-type":
		return string(api.ctx.Request.Header.ContentType())
	case "http_content_length", "http_content-length":
		return string(api.ctx.Request.Header.Peek("Content-Length"))
	case "remote_addr":
		return api.ctx.RemoteAddr().String()
	case "remote_port":
		return ""
	case "binary_remote_addr":
		return ""
	case "server_addr":
		addr := api.ctx.LocalAddr()
		if addr != nil {
			return addr.String()
		}
		return ""
	case "server_port":
		return ""
	case "server_name":
		return string(api.ctx.Host())
	case "arg_":
		return string(api.ctx.URI().QueryString())
	default:
		if len(name) > 4 && name[:4] == "arg_" {
			paramName := name[4:]
			return string(api.ctx.QueryArgs().Peek(paramName))
		}
		if len(name) > 5 && name[:5] == "http_" {
			headerName := name[5:]
			return string(api.ctx.Request.Header.Peek(headerName))
		}
		return ""
	}
}

// SetVariable 设置自定义变量（Go 层调用）
func (api *ngxVarAPI) SetVariable(name, value string) {
	api.store[name] = value
}

// GetVariable 获取变量值（Go 层调用）
func (api *ngxVarAPI) GetVariable(name string) (string, bool) {
	// 先查自定义变量
	if value, ok := api.store[name]; ok {
		return value, true
	}
	// 再查 fasthttp 变量
	value := api.getVariable(name)
	return value, value != ""
}

// RegisterSchedulerUnsafeVarAPI 为 Scheduler LState 注册不安全的 ngx.var API
func RegisterSchedulerUnsafeVarAPI(L *glua.LState, ngx *glua.LTable) {
	ngxVar := L.NewTable()
	mt := L.NewTable()
	mt.RawSetString("__index", L.NewFunction(luaSchedulerUnsafeVarIndex))
	mt.RawSetString("__newindex", L.NewFunction(luaSchedulerUnsafeVarNewIndex))
	L.SetMetatable(ngxVar, mt)
	ngx.RawSetString("var", ngxVar)
}

func luaSchedulerUnsafeVarIndex(L *glua.LState) int {
	L.RaiseError("API ngx.var not available in timer callback context")
	return 0
}

func luaSchedulerUnsafeVarNewIndex(L *glua.LState) int {
	L.RaiseError("API ngx.var not available in timer callback context")
	return 0
}
