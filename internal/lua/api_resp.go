// Package lua 提供 ngx.resp API 实现。
//
// 该文件实现 nginx 风格的响应操作 Lua API，用于操作 HTTP 响应。
// 兼容 OpenResty/ngx_lua 的 ngx.resp 语义。
//
// 主要功能：
//   - ngx.resp.get_status()：获取响应状态码
//   - ngx.resp.set_status(code)：设置响应状态码
//   - ngx.resp.get_headers()：获取响应头表
//   - ngx.resp.set_header(key, value)：设置响应头
//   - ngx.resp.clear_header(key)：清除响应头
//
// 注意事项：
//   - 响应头缓存使用 sync.Once 延迟初始化
//   - 修改响应头后会自动清除缓存
//   - Scheduler 模式下 ngx.resp 不可用
//
// 作者：xfy
package lua

import (
	"sync"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// ngxRespAPI ngx.resp API 实现
type ngxRespAPI struct {
	// 请求上下文（包含 fasthttp.Response）
	ctx *fasthttp.RequestCtx

	// 缓存：响应头表
	headersCache     map[string][]string
	headersCacheOnce sync.Once
}

// newNgxRespAPI 创建 ngx.resp API 实例
func newNgxRespAPI(ctx *fasthttp.RequestCtx) *ngxRespAPI {
	return &ngxRespAPI{
		ctx:          ctx,
		headersCache: nil, // 延迟初始化
	}
}

// RegisterNgxRespAPI 在 Lua 状态机中注册 ngx.resp API
// 这是主入口函数，由 LuaEngine 在初始化时调用
func RegisterNgxRespAPI(L *glua.LState, api *ngxRespAPI) {
	// 获取已存在的 ngx 表（必须已设置全局）
	ngx := L.GetGlobal("ngx")
	if ngx == nil || ngx.Type() != glua.LTTable {
		// 如果不存在，创建新表并设置全局
		ngx = L.NewTable()
		L.SetGlobal("ngx", ngx)
	}

	// 类型断言检查
	ngxTable, ok := ngx.(*glua.LTable)
	if !ok {
		return
	}

	// 检查 ngx.resp 是否已存在，避免并发写入
	var ngxResp *glua.LTable
	if existingResp := ngxTable.RawGetString("resp"); existingResp == glua.LNil {
		// 首次创建 ngx.resp 子表
		ngxResp = L.NewTable()
		ngxTable.RawSetString("resp", ngxResp)
	} else {
		ngxResp = existingResp.(*glua.LTable)
	}

	// 每次请求更新函数以绑定正确的 ctx
	ngxResp.RawSetString("get_status", L.NewFunction(api.luaGetStatus))
	ngxResp.RawSetString("set_status", L.NewFunction(api.luaSetStatus))
	ngxResp.RawSetString("get_headers", L.NewFunction(api.luaGetHeaders))
	ngxResp.RawSetString("set_header", L.NewFunction(api.luaSetHeader))
	ngxResp.RawSetString("clear_header", L.NewFunction(api.luaClearHeader))
}

// ==================== API 实现 ====================

// luaGetStatus 实现 ngx.resp.get_status()
// Lua 调用: local status = ngx.resp.get_status()
// 返回: number (HTTP 状态码，如 200, 404, 500 等)
func (api *ngxRespAPI) luaGetStatus(L *glua.LState) int {
	status := api.ctx.Response.StatusCode()
	L.Push(glua.LNumber(status))
	return 1
}

// luaSetStatus 实现 ngx.resp.set_status(code)
// Lua 调用: ngx.resp.set_status(404)
// 参数: code (number) - HTTP 状态码
// 返回: 无
func (api *ngxRespAPI) luaSetStatus(L *glua.LState) int {
	code := L.CheckInt(1)
	api.ctx.Response.SetStatusCode(code)
	return 0
}

// luaGetHeaders 实现 ngx.resp.get_headers(max_headers?)
// Lua 调用: local headers = ngx.resp.get_headers() 或 ngx.resp.get_headers(100)
// 参数: max_headers (number, 可选) - 最大返回头数量，默认为 100
// 返回: table (响应头表，如 { ["Content-Type"] = "text/html", ... })
func (api *ngxRespAPI) luaGetHeaders(L *glua.LState) int {
	// 获取可选的 max_headers 参数
	maxHeaders := 100
	if L.GetTop() >= 1 {
		maxHeaders = L.CheckInt(1)
		if maxHeaders <= 0 {
			maxHeaders = 100
		}
	}

	// 延迟初始化缓存
	api.headersCacheOnce.Do(func() {
		api.headersCache = api.parseHeaders()
	})

	// 构建 Lua 表
	result := L.NewTable()
	count := 0

	for key, values := range api.headersCache {
		if count >= maxHeaders {
			break
		}

		if len(values) == 1 {
			// 单值：直接存储为字符串
			result.RawSetString(key, glua.LString(values[0]))
		} else if len(values) > 1 {
			// 多值：存储为数组（table）
			arr := L.NewTable()
			for i, v := range values {
				arr.RawSetInt(i+1, glua.LString(v)) // Lua 数组从 1 开始
			}
			result.RawSetString(key, arr)
		}
		count++
	}

	L.Push(result)
	return 1
}

// luaSetHeader 实现 ngx.resp.set_header(key, value)
// Lua 调用: ngx.resp.set_header("Content-Type", "application/json")
// 参数:
//   - key (string) - 头名称
//   - value (string) - 头值
//
// 返回: 无
func (api *ngxRespAPI) luaSetHeader(L *glua.LState) int {
	key := L.CheckString(1)
	value := L.CheckString(2)

	api.ctx.Response.Header.Set(key, value)

	// 清除缓存，下次 get_headers 会重新解析
	api.headersCache = nil
	api.headersCacheOnce = sync.Once{}

	return 0
}

// luaClearHeader 实现 ngx.resp.clear_header(key)
// Lua 调用: ngx.resp.clear_header("X-Custom-Header")
// 参数: key (string) - 要清除的头名称
// 返回: 无
func (api *ngxRespAPI) luaClearHeader(L *glua.LState) int {
	key := L.CheckString(1)

	api.ctx.Response.Header.Del(key)

	// 清除缓存，下次 get_headers 会重新解析
	api.headersCache = nil
	api.headersCacheOnce = sync.Once{}

	return 0
}

// ==================== 辅助函数 ====================

// parseHeaders 解析响应头为 map
func (api *ngxRespAPI) parseHeaders() map[string][]string {
	result := make(map[string][]string)

	// 遍历所有响应头（使用 All 替代已弃用的 VisitAll）
	for key, value := range api.ctx.Response.Header.All() {
		keyStr := string(key)
		valueStr := string(value)

		if existing, ok := result[keyStr]; ok {
			result[keyStr] = append(existing, valueStr)
		} else {
			result[keyStr] = []string{valueStr}
		}
	}

	return result
}

// GetHeader 获取单个响应头值（辅助函数，供外部调用）
func (api *ngxRespAPI) GetHeader(name string) string {
	return string(api.ctx.Response.Header.Peek(name))
}

// SetHeader 设置单个响应头（辅助函数，供外部调用）
func (api *ngxRespAPI) SetHeader(name, value string) {
	api.ctx.Response.Header.Set(name, value)

	// 清除缓存
	api.headersCache = nil
	api.headersCacheOnce = sync.Once{}
}

// RegisterSchedulerUnsafeRespAPI 为 Scheduler LState 注册不安全的 ngx.resp API
func RegisterSchedulerUnsafeRespAPI(L *glua.LState, ngx *glua.LTable) {
	methods := []string{
		"get_status",
		"set_status",
		"get_headers",
		"set_header",
		"clear_header",
	}
	RegisterUnsafeAPI(L, ngx, "ngx.resp", methods)
}
