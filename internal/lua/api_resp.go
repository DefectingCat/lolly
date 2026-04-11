// Package lua 提供 ngx.resp API 实现
// 本文件实现 nginx 风格的响应 API，用于操作 HTTP 响应
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
	// 获取已存在的 ngx 表，如果不存在则创建
	ngx := L.GetGlobal("ngx")
	if ngx == nil || ngx.Type() != glua.LTTable {
		ngx = L.NewTable()
		L.SetGlobal("ngx", ngx)
	}

	// 创建 ngx.resp 子表
	ngxResp := L.NewTable()

	// ngx.resp.get_status() - 获取响应状态码
	ngxResp.RawSetString("get_status", L.NewFunction(api.luaGetStatus))

	// ngx.resp.set_status(code) - 设置响应状态码
	ngxResp.RawSetString("set_status", L.NewFunction(api.luaSetStatus))

	// ngx.resp.get_headers(max_headers?) - 获取响应头表
	ngxResp.RawSetString("get_headers", L.NewFunction(api.luaGetHeaders))

	// ngx.resp.set_header(key, value) - 设置响应头
	ngxResp.RawSetString("set_header", L.NewFunction(api.luaSetHeader))

	// ngx.resp.clear_header(key) - 清除响应头
	ngxResp.RawSetString("clear_header", L.NewFunction(api.luaClearHeader))

	// 将 ngx.resp 添加到 ngx
	ngx.(*glua.LTable).RawSetString("resp", ngxResp)
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

	// 遍历所有响应头
	api.ctx.Response.Header.VisitAll(func(key, value []byte) {
		keyStr := string(key)
		valueStr := string(value)

		if existing, ok := result[keyStr]; ok {
			result[keyStr] = append(existing, valueStr)
		} else {
			result[keyStr] = []string{valueStr}
		}
	})

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
