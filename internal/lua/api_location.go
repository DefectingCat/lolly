// Package lua 提供 ngx.location API 实现。
//
// 该文件实现 ngx.location.capture 子请求功能，兼容 OpenResty/ngx_lua 语义。
// 支持：
//   - 注册 location handler 映射
//   - 执行子请求并捕获响应（状态码、响应头、响应体）
//   - 选项控制：方法、请求体、自定义头、查询参数
//
// 注意事项：
//   - 子请求复用父请求的数据（深拷贝请求体）
//   - Scheduler 模式下 ngx.location.capture 不可用
//
// 作者：xfy
package lua

import (
	"strings"
	"sync"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// LocationCaptureResult 子请求捕获结果。
//
// 包含子请求的响应状态码、响应头和响应体数据。
type LocationCaptureResult struct {
	// Headers 响应头映射
	Headers map[string]string

	// Body 响应体数据
	Body []byte

	// Status HTTP 状态码
	Status int
}

// LocationManager location 管理器，用于注册和管理子请求 handler。
//
// 维护 location 路径到 fasthttp.RequestHandler 的映射，
// 支持并发安全的注册和捕获操作。
type LocationManager struct {
	// handlers 路径到 handler 的映射
	handlers map[string]fasthttp.RequestHandler

	// mu 读写锁
	mu sync.Mutex
}

// NewLocationManager 创建 location 管理器实例。
//
// 返回值：
//   - *LocationManager: 初始化的管理器实例
func NewLocationManager() *LocationManager {
	return &LocationManager{
		handlers: make(map[string]fasthttp.RequestHandler),
	}
}

// Register 注册 location handler。
//
// 参数：
//   - location: 路径标识（如 "/api/internal"）
//   - handler: fasthttp 请求处理器
func (m *LocationManager) Register(location string, handler fasthttp.RequestHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[location] = handler
}

// Capture 执行子请求。
//
// 查找指定 location 的 handler，创建子请求上下文，复制父请求数据，
// 应用选项（方法、请求体、头、查询参数），执行 handler 并收集响应。
//
// 参数：
//   - parentCtx: 父请求上下文，用于数据复制
//   - location: 目标 location 路径
//   - opts: 可选参数映射（method、body、headers、args）
//
// 返回值：
//   - *LocationCaptureResult: 子请求响应结果
//   - error: 当前实现始终返回 nil
func (m *LocationManager) Capture(parentCtx *fasthttp.RequestCtx, location string, opts map[string]any) (*LocationCaptureResult, error) {
	m.mu.Lock()
	handler, ok := m.handlers[location]
	m.mu.Unlock()

	if !ok {
		// location 不存在，返回 404
		return &LocationCaptureResult{
			Status:  404,
			Body:    []byte("location not found"),
			Headers: map[string]string{},
		}, nil
	}

	// 创建子请求上下文（不设置 Conn）
	subCtx := &fasthttp.RequestCtx{}

	// 复制父请求作为基础（深拷贝）
	parentCtx.Request.CopyTo(&subCtx.Request)

	// 设置子请求的路径，保留父请求的查询参数
	// 解析 location，分离路径和查询参数
	uri := subCtx.URI()
	uri.SetPath(location)

	// 应用选项
	if opts != nil {
		if method, ok := opts["method"].(string); ok {
			subCtx.Request.Header.SetMethod(method)
		}
		if body, ok := opts["body"].(string); ok {
			subCtx.Request.SetBodyString(body)
		}
		if headers, ok := opts["headers"].(map[string]string); ok {
			for k, v := range headers {
				subCtx.Request.Header.Set(k, v)
			}
		}
		// 如果选项中显式指定了 args，则覆盖父请求的查询参数
		if args, ok := opts["args"].(map[string]string); ok {
			uri.QueryArgs().Reset()
			for k, v := range args {
				uri.QueryArgs().Add(k, v)
			}
		}
	}

	// 执行 handler
	handler(subCtx)

	// 收集结果
	result := &LocationCaptureResult{
		Status:  subCtx.Response.StatusCode(),
		Body:    subCtx.Response.Body(),
		Headers: make(map[string]string),
	}

	// 收集响应头（使用 All 替代已弃用的 VisitAll）
	for key, value := range subCtx.Response.Header.All() {
		result.Headers[string(key)] = string(value)
	}

	return result, nil
}

// getRequestCtx 从当前 Lua 协程的 UserData 中获取 RequestCtx
// 通过协程关联的 RequestCtx 实现子请求对父请求数据的访问
func getRequestCtx(L *glua.LState) *fasthttp.RequestCtx {
	// 获取当前协程的上下文（在创建协程时通过 SetContext 设置）
	if ctx := L.Context(); ctx != nil {
		if reqCtx, ok := ctx.(*fasthttp.RequestCtx); ok {
			return reqCtx
		}
	}
	return nil
}

// RegisterLocationAPI 注册 ngx.location API 到 Lua 状态机。
//
// 在 ngx 表下创建 location 子表，注册 capture 方法用于执行子请求。
//
// 参数：
//   - L: Lua 状态
//   - manager: location 管理器实例
//   - ngx: ngx 全局表
func RegisterLocationAPI(L *glua.LState, manager *LocationManager, ngx *glua.LTable) {
	// 创建 ngx.location 表
	location := L.NewTable()

	// ngx.location.capture(uri, options?)
	L.SetField(location, "capture", L.NewFunction(func(L *glua.LState) int {
		uri := L.CheckString(1)

		// 解析选项
		opts := make(map[string]any)
		if L.GetTop() >= 2 {
			optionsTable := L.CheckTable(2)
			optionsTable.ForEach(func(key, value glua.LValue) {
				keyStr := glua.LVAsString(key)
				//nolint:exhaustive // 只处理特定类型
				switch value.Type() {
				case glua.LTString:
					opts[keyStr] = glua.LVAsString(value)
				case glua.LTNumber:
					opts[keyStr] = float64(glua.LVAsNumber(value))
				case glua.LTTable:
					// 处理 headers 表
					if keyStr == "headers" {
						headers := make(map[string]string)
						// ForEach 不返回错误，但在类型断言前需要检查
						tbl, ok := value.(*glua.LTable)
						if !ok {
							return // 跳过当前 ForEach 回调
						}
						tbl.ForEach(func(hKey, hValue glua.LValue) {
							headers[glua.LVAsString(hKey)] = glua.LVAsString(hValue)
						})
						opts[keyStr] = headers
					}
				default:
					// 其他类型不处理
				}
			})
		}

		// 创建结果表
		result := L.NewTable()

		if manager == nil {
			// manager 未初始化
			L.SetField(result, "status", glua.LNumber(500))
			L.SetField(result, "body", glua.LString("location manager not initialized"))
			L.Push(result)
			return 1
		}

		// 获取父请求上下文（从当前协程）
		parentCtx := getRequestCtx(L)
		if parentCtx == nil {
			// 没有父请求上下文，使用模拟上下文
			mockCtx := &fasthttp.RequestCtx{}
			mockCtx.Request.SetRequestURI(uri)
			parentCtx = mockCtx
		}

		// 执行子请求，传递父请求上下文用于数据复制
		captureResult, err := manager.Capture(parentCtx, uri, opts)
		if err == nil && captureResult != nil {
			L.SetField(result, "status", glua.LNumber(captureResult.Status))
			L.SetField(result, "body", glua.LString(string(captureResult.Body)))

			// 设置 headers
			headersTable := headersToLuaTable(L, captureResult.Headers)
			L.SetField(result, "headers", headersTable)
		} else {
			// 执行失败
			L.SetField(result, "status", glua.LNumber(500))
			L.SetField(result, "body", glua.LString("subrequest failed: "+err.Error()))
		}

		L.Push(result)
		return 1
	}))

	L.SetField(ngx, "location", location)
}

// headersToLuaTable 将 headers 转为 Lua 表
func headersToLuaTable(L *glua.LState, headers map[string]string) *glua.LTable {
	table := L.NewTable()
	for k, v := range headers {
		// 转换为小写键名（nginx 风格）
		table.RawSetString(strings.ToLower(k), glua.LString(v))
	}
	return table
}

// RegisterSchedulerUnsafeLocationAPI 为 Scheduler LState 注册不安全的 ngx.location API
// 这些 API 在 scheduler 模式下会返回错误
func RegisterSchedulerUnsafeLocationAPI(L *glua.LState, ngx *glua.LTable) {
	// 创建 ngx.location 表
	location := L.NewTable()

	// ngx.location.capture 在 scheduler 模式下不可用
	L.SetField(location, "capture", L.NewFunction(luaSchedulerUnsafeLocation))

	L.SetField(ngx, "location", location)
}

// luaSchedulerUnsafeLocation 返回 scheduler 模式下不可用的错误
func luaSchedulerUnsafeLocation(L *glua.LState) int {
	L.RaiseError("API ngx.location.capture not available in timer callback context")
	return 0
}
