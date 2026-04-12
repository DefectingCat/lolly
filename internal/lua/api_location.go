// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"strings"
	"sync"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// LocationCaptureResult 子请求结果
type LocationCaptureResult struct {
	Status  int
	Body    []byte
	Headers map[string]string
}

// LocationManager location 管理（用于子请求）
type LocationManager struct {
	mu       sync.Mutex
	handlers map[string]fasthttp.RequestHandler // location -> handler
}

// NewLocationManager 创建 location 管理器
func NewLocationManager() *LocationManager {
	return &LocationManager{
		handlers: make(map[string]fasthttp.RequestHandler),
	}
}

// Register 注册 location handler
func (m *LocationManager) Register(location string, handler fasthttp.RequestHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[location] = handler
}

// Capture 执行子请求
func (m *LocationManager) Capture(parentCtx *fasthttp.RequestCtx, location string, opts map[string]interface{}) (*LocationCaptureResult, error) {
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

	// 复制父请求作为基础
	parentCtx.Request.CopyTo(&subCtx.Request)

	// 设置子请求的 URI
	subCtx.Request.SetRequestURI(location)

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
	}

	// 执行 handler
	handler(subCtx)

	// 收集结果
	result := &LocationCaptureResult{
		Status:  subCtx.Response.StatusCode(),
		Body:    subCtx.Response.Body(),
		Headers: make(map[string]string),
	}

	// 收集响应头（使用 VisitAll）
	subCtx.Response.Header.VisitAll(func(key, value []byte) {
		result.Headers[string(key)] = string(value)
	})

	return result, nil
}

// RegisterLocationAPI 注册 ngx.location API
func RegisterLocationAPI(L *glua.LState, manager *LocationManager, ngx *glua.LTable) {
	// 创建 ngx.location 表
	location := L.NewTable()

	// ngx.location.capture(uri, options?)
	L.SetField(location, "capture", L.NewFunction(func(L *glua.LState) int {
		uri := L.CheckString(1)

		// 解析选项
		opts := make(map[string]interface{})
		if L.GetTop() >= 2 {
			optionsTable := L.CheckTable(2)
			optionsTable.ForEach(func(key, value glua.LValue) {
				keyStr := glua.LVAsString(key)
				switch value.Type() {
				case glua.LTString:
					opts[keyStr] = glua.LVAsString(value)
				case glua.LTNumber:
					opts[keyStr] = float64(glua.LVAsNumber(value))
				case glua.LTTable:
					// 处理 headers 表
					if keyStr == "headers" {
						headers := make(map[string]string)
						value.(*glua.LTable).ForEach(func(hKey, hValue glua.LValue) {
							headers[glua.LVAsString(hKey)] = glua.LVAsString(hValue)
						})
						opts[keyStr] = headers
					}
				}
			})
		}

		// 创建结果表
		result := L.NewTable()

		// 尝试执行子请求
		// 注意：由于无法直接获取 RequestCtx，这里使用模拟的上下文
		// 在完整实现中，应该通过 coroutine 传递 RequestCtx
		if manager != nil {
			// 创建模拟请求上下文用于子请求执行
			mockCtx := &fasthttp.RequestCtx{}
			mockCtx.Request.SetRequestURI(uri)

			captureResult, err := manager.Capture(mockCtx, uri, opts)
			if err == nil && captureResult != nil {
				L.SetField(result, "status", glua.LNumber(captureResult.Status))
				L.SetField(result, "body", glua.LString(string(captureResult.Body)))

				// 设置 headers
				headersTable := headersToLuaTable(L, captureResult.Headers)
				L.SetField(result, "headers", headersTable)
			} else {
				// 执行失败
				L.SetField(result, "status", glua.LNumber(500))
				L.SetField(result, "body", glua.LString("subrequest failed"))
			}
		} else {
			// manager 未初始化
			L.SetField(result, "status", glua.LNumber(404))
			L.SetField(result, "body", glua.LString("location manager not initialized"))
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
