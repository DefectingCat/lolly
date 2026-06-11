// Package lua 提供 ngx.log 和输出控制 API 实现。
//
// 该文件实现与 OpenResty/ngx_lua 兼容的日志和输出控制 API，包括：
//   - ngx.log：日志输出（兼容 OpenResty 日志级别常量）
//   - ngx.say/print：内容输出（追加到响应缓冲区）
//   - ngx.flush：刷新输出缓冲区
//   - ngx.exit：终止请求处理
//   - ngx.redirect：HTTP 重定向
//   - HTTP 状态码常量（如 ngx.HTTP_OK、ngx.HTTP_NOT_FOUND 等）
//
// 注意事项：
//   - ngx.exit/ngx.redirect 通过 RaiseError 终止 Lua 执行
//   - Scheduler 模式下 ngx.log 不依赖 RequestCtx，仅输出到标准日志
//
// 作者：xfy
package lua

import (
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// 日志级别常量（与 OpenResty/ngx_lua 兼容）
const (
	// LogStderr 标准错误日志级别。
	LogStderr = 0
	// LogEmerg 紧急日志级别。
	LogEmerg = 1
	// LogAlert 警报日志级别。
	LogAlert = 2
	// LogCrit 严重日志级别。
	LogCrit = 3
	// LogErr 错误日志级别。
	LogErr = 4
	// LogWarn 警告日志级别。
	LogWarn = 5
	// LogNotice 通知日志级别。
	LogNotice = 6
	// LogInfo 信息日志级别。
	LogInfo = 7
	// LogDebug 调试日志级别。
	LogDebug = 8
)

// HTTP 状态码常量
const (
	// HTTPContinue HTTP 100 继续状态码。
	HTTPContinue = 100
	// HTTPSwitchingProtocols HTTP 101 切换协议状态码。
	HTTPSwitchingProtocols = 101
	// HTTPOK HTTP 200 成功状态码。
	HTTPOK = 200
	// HTTPCreated HTTP 201 已创建状态码。
	HTTPCreated = 201
	// HTTPAccepted HTTP 202 已接受状态码。
	HTTPAccepted = 202
	// HTTPNoContent HTTP 204 无内容状态码。
	HTTPNoContent = 204
	// HTTPPartialContent HTTP 206 部分内容状态码。
	HTTPPartialContent = 206
	// HTTPMovedPermanently HTTP 301 永久重定向状态码。
	HTTPMovedPermanently = 301
	// HTTPFound HTTP 302 找到状态码。
	HTTPFound = 302
	// HTTPSeeOther HTTP 303 查看其他状态码。
	HTTPSeeOther = 303
	// HTTPNotModified HTTP 304 未修改状态码。
	HTTPNotModified = 304
	// HTTPTemporaryRedirect HTTP 307 临时重定向状态码。
	HTTPTemporaryRedirect = 307
	// HTTPPermanentRedirect HTTP 308 永久重定向状态码。
	HTTPPermanentRedirect = 308
	// HTTPBadRequest HTTP 400 错误请求状态码。
	HTTPBadRequest = 400
	// HTTPUnauthorized HTTP 401 未授权状态码。
	HTTPUnauthorized = 401
	// HTTPForbidden HTTP 403 禁止访问状态码。
	HTTPForbidden = 403
	// HTTPNotFound HTTP 404 未找到状态码。
	HTTPNotFound = 404
	// HTTPMethodNotAllowed HTTP 405 方法不允许状态码。
	HTTPMethodNotAllowed = 405
	// HTTPRequestTimeout HTTP 408 请求超时状态码。
	HTTPRequestTimeout = 408
	// HTTPConflict HTTP 409 冲突状态码。
	HTTPConflict = 409
	// HTTPGone HTTP 410 已移除状态码。
	HTTPGone = 410
	// HTTPLengthRequired HTTP 411 需要长度状态码。
	HTTPLengthRequired = 411
	// HTTPPayloadTooLarge HTTP 413 请求实体过大状态码。
	HTTPPayloadTooLarge = 413
	// HTTPURITooLong HTTP 414 URI 过长状态码。
	HTTPURITooLong = 414
	// HTTPUnsupportedMedia HTTP 415 不支持的媒体类型状态码。
	HTTPUnsupportedMedia = 415
	// HTTPRangeNotSatisfiable HTTP 416 范围不可满足状态码。
	HTTPRangeNotSatisfiable = 416
	// HTTPTooManyRequests HTTP 429 请求过多状态码。
	HTTPTooManyRequests = 429
	// HTTPInternalServerError HTTP 500 内部服务器错误状态码。
	HTTPInternalServerError = 500
	// HTTPNotImplemented HTTP 501 未实现状态码。
	HTTPNotImplemented = 501
	// HTTPBadGateway HTTP 502 错误网关状态码。
	HTTPBadGateway = 502
	// HTTPServiceUnavailable HTTP 503 服务不可用状态码。
	HTTPServiceUnavailable = 503
	// HTTPGatewayTimeout HTTP 504 网关超时状态码。
	HTTPGatewayTimeout = 504
	// HTTPHTTPVersionNotSupported HTTP 505 HTTP 版本不支持状态码。
	HTTPHTTPVersionNotSupported = 505
)

// ngxLogAPI 封装 ngx.log 和输出控制相关的 API。
//
// 包含请求上下文、Lua 上下文和日志记录器，用于：
//   - 将 Lua 日志消息转发到 zerolog 记录器
//   - 通过 ngx.say/print 写入响应缓冲区
//   - 通过 ngx.exit/redirect 终止请求处理
type ngxLogAPI struct {
	// ctx 关联的 fasthttp 请求上下文，用于直接写入响应
	ctx *fasthttp.RequestCtx

	// luaCtx Lua 上下文，用于访问输出缓冲区
	luaCtx *LuaContext

	// logger zerolog 日志记录器，用于结构化日志输出
	logger *zerolog.Logger
}

// newNgxLogAPI 创建 ngx.log API 实例。
//
// 参数：
//   - ctx: fasthttp 请求上下文，用于直接写入响应
//   - luaCtx: Lua 上下文，用于访问输出缓冲区
//   - logger: zerolog 日志记录器，为 nil 时禁用结构化日志
//
// 返回值：
//   - *ngxLogAPI: 初始化的 API 实例
func newNgxLogAPI(ctx *fasthttp.RequestCtx, luaCtx *LuaContext, logger *zerolog.Logger) *ngxLogAPI {
	return &ngxLogAPI{
		ctx:    ctx,
		luaCtx: luaCtx,
		logger: logger,
	}
}

// RegisterNgxLogAPI 在 Lua 状态机中注册 ngx.log 和输出控制 API。
//
// 常量（日志级别、HTTP状态码等）只在首次注册时写入，避免并发写入冲突。
// 每次请求都会重新注册请求特定的函数（log, say, print, flush, exit, redirect）。
func RegisterNgxLogAPI(L *glua.LState, api *ngxLogAPI) {
	// 获取或创建 ngx 表
	ngx := GetOrCreateNgxTable(L)

	// 检查常量是否已注册（通过 STDERR 常量判断）
	// 如果已注册，跳过常量写入，避免并发写入全局表
	if ngx.RawGetString("STDERR") == glua.LNil {
		// 注册日志级别常量
		ngx.RawSetString("STDERR", glua.LNumber(LogStderr))
		ngx.RawSetString("EMERG", glua.LNumber(LogEmerg))
		ngx.RawSetString("ALERT", glua.LNumber(LogAlert))
		ngx.RawSetString("CRIT", glua.LNumber(LogCrit))
		ngx.RawSetString("ERR", glua.LNumber(LogErr))
		ngx.RawSetString("WARN", glua.LNumber(LogWarn))
		ngx.RawSetString("NOTICE", glua.LNumber(LogNotice))
		ngx.RawSetString("INFO", glua.LNumber(LogInfo))
		ngx.RawSetString("DEBUG", glua.LNumber(LogDebug))

		// 注册 HTTP 状态码常量
		ngx.RawSetString("HTTP_CONTINUE", glua.LNumber(HTTPContinue))
		ngx.RawSetString("HTTP_SWITCHING_PROTOCOLS", glua.LNumber(HTTPSwitchingProtocols))
		ngx.RawSetString("HTTP_OK", glua.LNumber(HTTPOK))
		ngx.RawSetString("HTTP_CREATED", glua.LNumber(HTTPCreated))
		ngx.RawSetString("HTTP_ACCEPTED", glua.LNumber(HTTPAccepted))
		ngx.RawSetString("HTTP_NO_CONTENT", glua.LNumber(HTTPNoContent))
		ngx.RawSetString("HTTP_PARTIAL_CONTENT", glua.LNumber(HTTPPartialContent))
		ngx.RawSetString("HTTP_MOVED_PERMANENTLY", glua.LNumber(HTTPMovedPermanently))
		ngx.RawSetString("HTTP_FOUND", glua.LNumber(HTTPFound))
		ngx.RawSetString("HTTP_SEE_OTHER", glua.LNumber(HTTPSeeOther))
		ngx.RawSetString("HTTP_NOT_MODIFIED", glua.LNumber(HTTPNotModified))
		ngx.RawSetString("HTTP_TEMPORARY_REDIRECT", glua.LNumber(HTTPTemporaryRedirect))
		ngx.RawSetString("HTTP_PERMANENT_REDIRECT", glua.LNumber(HTTPPermanentRedirect))
		ngx.RawSetString("HTTP_BAD_REQUEST", glua.LNumber(HTTPBadRequest))
		ngx.RawSetString("HTTP_UNAUTHORIZED", glua.LNumber(HTTPUnauthorized))
		ngx.RawSetString("HTTP_FORBIDDEN", glua.LNumber(HTTPForbidden))
		ngx.RawSetString("HTTP_NOT_FOUND", glua.LNumber(HTTPNotFound))
		ngx.RawSetString("HTTP_METHOD_NOT_ALLOWED", glua.LNumber(HTTPMethodNotAllowed))
		ngx.RawSetString("HTTP_REQUEST_TIMEOUT", glua.LNumber(HTTPRequestTimeout))
		ngx.RawSetString("HTTP_CONFLICT", glua.LNumber(HTTPConflict))
		ngx.RawSetString("HTTP_GONE", glua.LNumber(HTTPGone))
		ngx.RawSetString("HTTP_LENGTH_REQUIRED", glua.LNumber(HTTPLengthRequired))
		ngx.RawSetString("HTTP_PAYLOAD_TOO_LARGE", glua.LNumber(HTTPPayloadTooLarge))
		ngx.RawSetString("HTTP_URI_TOO_LONG", glua.LNumber(HTTPURITooLong))
		ngx.RawSetString("HTTP_UNSUPPORTED_MEDIA_TYPE", glua.LNumber(HTTPUnsupportedMedia))
		ngx.RawSetString("HTTP_RANGE_NOT_SATISFIABLE", glua.LNumber(HTTPRangeNotSatisfiable))
		ngx.RawSetString("HTTP_TOO_MANY_REQUESTS", glua.LNumber(HTTPTooManyRequests))
		ngx.RawSetString("HTTP_INTERNAL_SERVER_ERROR", glua.LNumber(HTTPInternalServerError))
		ngx.RawSetString("HTTP_NOT_IMPLEMENTED", glua.LNumber(HTTPNotImplemented))
		ngx.RawSetString("HTTP_BAD_GATEWAY", glua.LNumber(HTTPBadGateway))
		ngx.RawSetString("HTTP_SERVICE_UNAVAILABLE", glua.LNumber(HTTPServiceUnavailable))
		ngx.RawSetString("HTTP_GATEWAY_TIMEOUT", glua.LNumber(HTTPGatewayTimeout))
		ngx.RawSetString("HTTP_VERSION_NOT_SUPPORTED", glua.LNumber(HTTPHTTPVersionNotSupported))

		// 特殊常量
		ngx.RawSetString("ERROR", glua.LNumber(-1))
		ngx.RawSetString("OK", glua.LNumber(0))
		ngx.RawSetString("AGAIN", glua.LNumber(-2))
		ngx.RawSetString("DONE", glua.LNumber(-4))
		ngx.RawSetString("DECLINED", glua.LNumber(-5))
	}

	// 注册 ngx.log 函数（每次请求重新注册以绑定正确的 ctx）
	ngx.RawSetString("log", L.NewFunction(api.luaLog))

	// 注册输出控制函数
	ngx.RawSetString("say", L.NewFunction(api.luaSay))
	ngx.RawSetString("print", L.NewFunction(api.luaPrint))
	ngx.RawSetString("flush", L.NewFunction(api.luaFlush))
	ngx.RawSetString("exit", L.NewFunction(api.luaExit))
	ngx.RawSetString("redirect", L.NewFunction(api.luaRedirect))

	// 注册 ngx 全局变量
	L.SetGlobal("ngx", ngx)
}

// luaLog 实现 ngx.log(level, ...) - 日志输出
// Lua 调用: ngx.log(ngx.ERR, "error message")
// 返回: 无
func (api *ngxLogAPI) luaLog(L *glua.LState) int {
	// 获取日志级别
	level := L.CheckInt(1)

	// 收集所有参数并拼接
	var parts []string
	n := L.GetTop()
	for i := 2; i <= n; i++ {
		parts = append(parts, L.ToString(i))
	}
	msg := strings.Join(parts, " ")

	// 根据级别映射到 zerolog
	if api.logger != nil {
		switch level {
		case LogEmerg, LogAlert, LogCrit:
			api.logger.Error().Str("lua_level", "critical").Msg(msg)
		case LogErr:
			api.logger.Error().Msg(msg)
		case LogWarn:
			api.logger.Warn().Msg(msg)
		case LogNotice:
			api.logger.Info().Msg(msg)
		case LogInfo:
			api.logger.Info().Msg(msg)
		case LogDebug:
			api.logger.Debug().Msg(msg)
		default:
			api.logger.Info().Msg(msg)
		}
	}

	return 0
}

// luaSay 实现 ngx.say(...) - 输出内容并附加换行符
// Lua 调用: ngx.say("hello", "world")
// 返回: 无
func (api *ngxLogAPI) luaSay(L *glua.LState) int {
	// 收集所有参数
	var parts []string
	n := L.GetTop()
	for i := 1; i <= n; i++ {
		parts = append(parts, L.ToString(i))
	}
	msg := strings.Join(parts, "") + "\n"

	// 写入到 LuaContext 的输出缓冲
	if api.luaCtx != nil {
		api.luaCtx.Write([]byte(msg))
	} else if api.ctx != nil {
		// 直接写入响应
		_, _ = api.ctx.Write([]byte(msg))
	}

	return 0
}

// luaPrint 实现 ngx.print(...) - 输出内容不附加换行符
// Lua 调用: ngx.print("hello", "world")
// 返回: 无
func (api *ngxLogAPI) luaPrint(L *glua.LState) int {
	// 收集所有参数
	var parts []string
	n := L.GetTop()
	for i := 1; i <= n; i++ {
		parts = append(parts, L.ToString(i))
	}
	msg := strings.Join(parts, "")

	// 写入到 LuaContext 的输出缓冲
	if api.luaCtx != nil {
		api.luaCtx.Write([]byte(msg))
	} else if api.ctx != nil {
		// 直接写入响应
		_, _ = api.ctx.Write([]byte(msg))
	}

	return 0
}

// luaFlush 实现 ngx.flush(wait?) - 刷新输出缓冲区
// Lua 调用: ngx.flush() 或 ngx.flush(true)
// 返回: 1 (boolean，表示是否成功)
func (api *ngxLogAPI) luaFlush(L *glua.LState) int {
	// 可选的 wait 参数
	wait := false
	if L.GetTop() >= 1 {
		wait = L.ToBool(1)
	}

	// 刷新输出缓冲
	if api.luaCtx != nil {
		api.luaCtx.FlushOutput()
	}

	// fasthttp 没有显式的 flush 方法，数据会自动发送
	// wait 参数在此实现中被忽略（阻塞式 flush）
	_ = wait

	L.Push(glua.LTrue)
	return 1
}

// luaExit 实现 ngx.exit(status) - 结束请求处理
// Lua 调用: ngx.exit(ngx.HTTP_OK) 或 ngx.exit(200)
// 返回: 无（抛出错误以终止执行）
func (api *ngxLogAPI) luaExit(L *glua.LState) int {
	status := L.CheckInt(1)

	// 设置退出状态
	if api.luaCtx != nil {
		api.luaCtx.Exit(status)
	} else if api.ctx != nil {
		api.ctx.SetStatusCode(status)
	}

	// 抛出错误以终止 Lua 执行
	L.RaiseError("%s", "ngx.exit: "+strconv.Itoa(status))
	return 0
}

// luaRedirect 实现 ngx.redirect(uri, status?) - HTTP 重定向
// Lua 调用: ngx.redirect("/new/path") 或 ngx.redirect("/new/path", 301)
// 返回: 无（抛出错误以终止执行）
func (api *ngxLogAPI) luaRedirect(L *glua.LState) int {
	uri := L.CheckString(1)

	// 默认状态码为 302 (HTTPFound)
	status := HTTPFound
	if L.GetTop() >= 2 {
		status = L.CheckInt(2)
	}

	// 验证重定向状态码
	if status != HTTPMovedPermanently &&
		status != HTTPFound &&
		status != HTTPSeeOther &&
		status != HTTPTemporaryRedirect &&
		status != HTTPPermanentRedirect {
		L.ArgError(2, "invalid redirect status code")
		return 0
	}

	// 设置重定向头
	if api.ctx != nil {
		api.ctx.Response.Header.Set("Location", uri)
		api.ctx.SetStatusCode(status)
	}

	if api.luaCtx != nil {
		api.luaCtx.Exited = true
	}

	// 抛出错误以终止 Lua 执行
	L.RaiseError("%s", "ngx.redirect: "+uri)
	return 0
}
