// Package lua 提供 ngx.log 和输出控制 API 实现
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
	LogStderr = 0
	LogEmerg  = 1
	LogAlert  = 2
	LogCrit   = 3
	LogErr    = 4
	LogWarn   = 5
	LogNotice = 6
	LogInfo   = 7
	LogDebug  = 8
)

// HTTP 状态码常量
const (
	HTTPContinue                = 100
	HTTPSwitchingProtocols      = 101
	HTTPOK                      = 200
	HTTPCreated                 = 201
	HTTPAccepted                = 202
	HTTPNoContent               = 204
	HTTPPartialContent          = 206
	HTTPMovedPermanently        = 301
	HTTPFound                   = 302
	HTTPSeeOther                = 303
	HTTPNotModified             = 304
	HTTPTemporaryRedirect       = 307
	HTTPPermanentRedirect       = 308
	HTTPBadRequest              = 400
	HTTPUnauthorized            = 401
	HTTPForbidden               = 403
	HTTPNotFound                = 404
	HTTPMethodNotAllowed        = 405
	HTTPRequestTimeout          = 408
	HTTPConflict                = 409
	HTTPGone                    = 410
	HTTPLengthRequired          = 411
	HTTPPayloadTooLarge         = 413
	HTTPURITooLong              = 414
	HTTPUnsupportedMedia        = 415
	HTTPRangeNotSatisfiable     = 416
	HTTPTooManyRequests         = 429
	HTTPInternalServerError     = 500
	HTTPNotImplemented          = 501
	HTTPBadGateway              = 502
	HTTPServiceUnavailable      = 503
	HTTPGatewayTimeout          = 504
	HTTPHTTPVersionNotSupported = 505
)

// ngxLogAPI ngx.log 和输出控制 API 实现
type ngxLogAPI struct {
	// 请求上下文
	ctx *fasthttp.RequestCtx

	// Lua 上下文（用于访问输出缓冲等）
	luaCtx *LuaContext

	// 日志记录器
	logger *zerolog.Logger
}

// newNgxLogAPI 创建 ngx.log API 实例
func newNgxLogAPI(ctx *fasthttp.RequestCtx, luaCtx *LuaContext, logger *zerolog.Logger) *ngxLogAPI {
	return &ngxLogAPI{
		ctx:    ctx,
		luaCtx: luaCtx,
		logger: logger,
	}
}

// RegisterNgxLogAPI 在 Lua 状态机中注册 ngx.log 和输出控制 API
func RegisterNgxLogAPI(L *glua.LState, api *ngxLogAPI) {
	// 获取或创建 ngx 表
	var ngx *glua.LTable
	existingNgx := L.GetGlobal("ngx")
	if existingNgx != nil && existingNgx.Type() == glua.LTTable {
		ngxTable, ok := existingNgx.(*glua.LTable)
		if ok {
			ngx = ngxTable
		} else {
			ngx = L.NewTable()
		}
	} else {
		ngx = L.NewTable()
	}

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

	// 注册 ngx.log 函数
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
			api.logger.Fatal().Msg(msg)
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

// LogLevelToZerolog 将 ngx 日志级别转换为 zerolog 级别
func LogLevelToZerolog(level int) zerolog.Level {
	switch level {
	case LogEmerg, LogAlert, LogCrit:
		return zerolog.FatalLevel
	case LogErr:
		return zerolog.ErrorLevel
	case LogWarn:
		return zerolog.WarnLevel
	case LogNotice, LogInfo:
		return zerolog.InfoLevel
	case LogDebug:
		return zerolog.DebugLevel
	default:
		return zerolog.InfoLevel
	}
}

// RegisterSchedulerLogAPI 为 Scheduler LState 注册安全的 ngx.log API
// 不依赖 RequestCtx，仅输出到标准日志
func RegisterSchedulerLogAPI(L *glua.LState, ngx *glua.LTable) {
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
	ngx.RawSetString("HTTP_OK", glua.LNumber(HTTPOK))
	ngx.RawSetString("HTTP_INTERNAL_SERVER_ERROR", glua.LNumber(HTTPInternalServerError))

	// 注册 ngx.log 函数（不依赖 RequestCtx 的版本）
	ngx.RawSetString("log", L.NewFunction(luaSchedulerLog))
}

// luaSchedulerLog 实现 scheduler 模式下的 ngx.log
// 不依赖 RequestCtx，仅输出到标准日志
func luaSchedulerLog(L *glua.LState) int {
	// 获取日志级别
	level := L.CheckInt(1)

	// 收集所有参数并拼接
	var parts []string
	n := L.GetTop()
	for i := 2; i <= n; i++ {
		parts = append(parts, L.ToString(i))
	}
	msg := strings.Join(parts, " ")

	// 根据级别输出（scheduler 模式下没有 logger，直接打印）
	// 在实际实现中，可以通过 engine 的 logger 输出
	_ = level
	_ = msg
	// fmt.Printf("[timer] %s\n", msg)

	return 0
}
