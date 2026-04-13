// Package lua 提供 ngx.req API 实现
// 本文件实现双层 API 边界验证原型，用于测量直接映射层 vs 兼容层的性能差异
package lua

import (
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"
)

// ngxReqAPILayer 定义 API 层级类型
type ngxReqAPILayer int

const (
	// APILayerDirect 直接映射层：fasthttp -> Lua，无中间层
	// 性能最优，延迟最低
	APILayerDirect ngxReqAPILayer = 1

	// APILayerCompatible 兼容层：需要模拟 nginx 语义
	// 增加了少量转换开销
	APILayerCompatible ngxReqAPILayer = 2

	// APILayerPseudoNonBlocking 伪非阻塞层：yield/resume
	// 支持在 Lua 中调用异步操作
	APILayerPseudoNonBlocking ngxReqAPILayer = 3
)

const (
	// layerStringDirect 直接映射层字符串
	layerStringDirect = "direct"
	// layerStringCompatible 兼容层字符串
	layerStringCompatible = "compatible"
	// layerStringPseudoNonBlocking 伪非阻塞层字符串
	layerStringPseudoNonBlocking = "pseudo_non_blocking"
	// layerStringUnknown 未知层字符串
	layerStringUnknown = "unknown"
)

func (l ngxReqAPILayer) String() string {
	switch l {
	case APILayerDirect:
		return layerStringDirect
	case APILayerCompatible:
		return layerStringCompatible
	case APILayerPseudoNonBlocking:
		return layerStringPseudoNonBlocking
	default:
		return layerStringUnknown
	}
}

// ngxReqMetrics 收集 API 调用指标
type ngxReqMetrics struct {
	// 调用计数
	DirectCallCount         uint64
	CompatibleCallCount     uint64
	PseudoBlockingCallCount uint64

	// 累积延迟（纳秒）
	DirectTotalNs         uint64
	CompatibleTotalNs     uint64
	PseudoBlockingTotalNs uint64

	// 最大值（用于识别异常）
	DirectMaxNs         uint64
	CompatibleMaxNs     uint64
	PseudoBlockingMaxNs uint64
}

// ngxReqAPI ngx.req API 实现
type ngxReqAPI struct {
	ctx              *fasthttp.RequestCtx
	uriArgsCache     map[string][]string
	metrics          ngxReqMetrics
	uriArgsCacheOnce sync.Once
}

// newNgxReqAPI 创建 ngx.req API 实例
func newNgxReqAPI(ctx *fasthttp.RequestCtx) *ngxReqAPI {
	return &ngxReqAPI{
		ctx:          ctx,
		uriArgsCache: nil, // 延迟初始化
	}
}

// RegisterNgxReqAPI 在 Lua 状态机中注册 ngx.req API
// 这是主入口函数，由 LuaEngine 在初始化时调用
func RegisterNgxReqAPI(L *glua.LState, api *ngxReqAPI, ngxTable *glua.LTable) {
	// 创建 ngx.req 子表
	ngxReq := L.NewTable()

	// 直接映射层 API：get_method
	// 特点：直接访问 fasthttp.RequestCtx，零拷贝，最小开销
	ngxReq.RawSetString("get_method", L.NewFunction(api.luaGetMethod))

	// 直接映射层 API：get_uri
	// 特点：直接返回请求的 URI 路径（不含 query string）
	ngxReq.RawSetString("get_uri", L.NewFunction(api.luaGetURI))

	// 直接映射层 API：set_uri
	// 特点：直接修改请求的 URI 路径，支持可选的内部跳转标记
	ngxReq.RawSetString("set_uri", L.NewFunction(api.luaSetURI))

	// 兼容层 API：get_uri_args
	// 特点：需要解析 query string 为 nginx 兼容的表结构
	// 增加了解析开销，但保持 API 兼容性
	ngxReq.RawSetString("get_uri_args", L.NewFunction(api.luaGetURIArgs))

	// 兼容层 API：set_uri_args
	// 特点：支持 table 或 string 参数设置查询参数
	ngxReq.RawSetString("set_uri_args", L.NewFunction(api.luaSetURIArgs))

	// 兼容层 API：get_headers
	// 特点：需要遍历所有请求头，模拟 nginx 的头表结构
	ngxReq.RawSetString("get_headers", L.NewFunction(api.luaGetHeaders))

	// 直接映射层 API：set_header
	// 特点：直接操作 fasthttp 请求头，支持设置和清除
	ngxReq.RawSetString("set_header", L.NewFunction(api.luaSetHeader))

	// 直接映射层 API：clear_header
	// 特点：直接删除 fasthttp 请求头
	ngxReq.RawSetString("clear_header", L.NewFunction(api.luaClearHeader))

	// 兼容层 API：get_body_data
	// 特点：获取请求体内容
	ngxReq.RawSetString("get_body_data", L.NewFunction(api.luaGetBodyData))

	// 伪非阻塞层 API：read_body
	// 特点：确保请求体已被读取（fasthttp 已预读）
	ngxReq.RawSetString("read_body", L.NewFunction(api.luaReadBody))

	// 将 ngx.req 添加到 ngx 表
	ngxTable.RawSetString("req", ngxReq)
}

// ==================== 直接映射层 API ====================

// luaGetMethod 实现 ngx.req.get_method() - 直接映射层
// Lua 调用: local method = ngx.req.get_method()
// 返回: string (如 "GET", "POST", "PUT" 等)
func (api *ngxReqAPI) luaGetMethod(L *glua.LState) int {
	start := time.Now()

	// 直接访问 fasthttp：零拷贝，最小开销
	method := string(api.ctx.Method())

	// 记录指标
	elapsed := uint64(time.Since(start).Nanoseconds())
	api.metrics.DirectCallCount++
	api.metrics.DirectTotalNs += elapsed
	if elapsed > api.metrics.DirectMaxNs {
		api.metrics.DirectMaxNs = elapsed
	}

	L.Push(glua.LString(method))
	return 1
}

// luaSetURI 实现 ngx.req.set_uri(uri, jump?) - 直接映射层
// Lua 调用: ngx.req.set_uri("/new/path") 或 ngx.req.set_uri("/new/path", true)
// 参数:
//   - uri: 新的 URI 路径
//   - jump: 是否触发内部跳转（可选，默认为 false）
func (api *ngxReqAPI) luaSetURI(L *glua.LState) int {
	start := time.Now()

	// 获取 uri 参数
	uri := L.CheckString(1)

	// 获取可选的 jump 参数
	jump := false
	if L.GetTop() >= 2 {
		jump = L.ToBool(2)
	}

	// 设置新的 URI
	api.ctx.Request.URI().SetPath(uri)

	// 如果 jump 为 true，记录内部跳转标记（供后续处理使用）
	if jump {
		// 在请求上下文中存储跳转标记
		api.ctx.SetUserValue("_ngx_req_internal_jump", true)
	}

	// 记录指标
	elapsed := uint64(time.Since(start).Nanoseconds())
	api.metrics.DirectCallCount++
	api.metrics.DirectTotalNs += elapsed
	if elapsed > api.metrics.DirectMaxNs {
		api.metrics.DirectMaxNs = elapsed
	}

	return 0
}

// luaGetURI 实现 ngx.req.get_uri() - 直接映射层
// Lua 调用: local uri = ngx.req.get_uri()
// 返回: string (如 "/path/to/resource")
func (api *ngxReqAPI) luaGetURI(L *glua.LState) int {
	start := time.Now()

	// 直接访问 fasthttp URI 路径
	uri := string(api.ctx.Request.URI().Path())

	// 记录指标
	elapsed := uint64(time.Since(start).Nanoseconds())
	api.metrics.DirectCallCount++
	api.metrics.DirectTotalNs += elapsed
	if elapsed > api.metrics.DirectMaxNs {
		api.metrics.DirectMaxNs = elapsed
	}

	L.Push(glua.LString(uri))
	return 1
}

// ==================== 兼容层 API ====================

// luaGetURIArgs 实现 ngx.req.get_uri_args() - 兼容层
// Lua 调用: local args = ngx.req.get_uri_args()
// 返回: table (如 { name = "value", arr = { "v1", "v2" } })
// 注意：兼容层需要解析 query string，模拟 nginx 的参数表结构
func (api *ngxReqAPI) luaGetURIArgs(L *glua.LState) int {
	start := time.Now()

	// 延迟初始化缓存
	api.uriArgsCacheOnce.Do(func() {
		api.uriArgsCache = api.parseURIArgs()
	})

	// 构建 Lua 表（兼容 nginx 的 ngx.req.get_uri_args 格式）
	result := L.NewTable()

	for key, values := range api.uriArgsCache {
		if len(values) == 1 {
			// 单值：直接存储为字符串
			result.RawSetString(key, glua.LString(values[0]))
		} else {
			// 多值：存储为数组（table）
			arr := L.NewTable()
			for i, v := range values {
				arr.RawSetInt(i+1, glua.LString(v)) // Lua 数组从 1 开始
			}
			result.RawSetString(key, arr)
		}
	}

	// 记录指标
	elapsed := uint64(time.Since(start).Nanoseconds())
	api.metrics.CompatibleCallCount++
	api.metrics.CompatibleTotalNs += elapsed
	if elapsed > api.metrics.CompatibleMaxNs {
		api.metrics.CompatibleMaxNs = elapsed
	}

	L.Push(result)
	return 1
}

// parseURIArgs 解析 URI query string 为 map
// 这是兼容层的核心转换逻辑，模拟 nginx 的参数解析
func (api *ngxReqAPI) parseURIArgs() map[string][]string {
	args := make(map[string][]string)

	// 获取 query string
	query := api.ctx.QueryArgs()

	// 遍历所有参数 - 使用 All() 替代已弃用的 VisitAll()
	for key, value := range query.All() {
		keyStr := string(key)
		valueStr := string(value)

		if existing, ok := args[keyStr]; ok {
			args[keyStr] = append(existing, valueStr)
		} else {
			args[keyStr] = []string{valueStr}
		}
	}

	return args
}

// luaSetURIArgs 实现 ngx.req.set_uri_args(args) - 兼容层
// Lua 调用: ngx.req.set_uri_args({ key = "value" }) 或 ngx.req.set_uri_args("key=value&foo=bar")
// 参数:
//   - args: table 或 string 类型的查询参数
func (api *ngxReqAPI) luaSetURIArgs(L *glua.LState) int {
	start := time.Now()

	// 获取参数类型
	argType := L.Get(1)

	//nolint:exhaustive // 只处理特定类型
	switch argType.Type() {
	case glua.LTString:
		// 如果是字符串，直接解析并设置
		// 类型断言检查
		queryStr, ok := argType.(glua.LString)
		if !ok {
			return 0
		}
		api.ctx.Request.URI().SetQueryString(string(queryStr))

	case glua.LTTable:
		// 如果是 table，构建查询字符串
		// 类型断言检查
		table, ok := argType.(*glua.LTable)
		if !ok {
			return 0
		}
		args := make(map[string][]string)

		table.ForEach(func(key, value glua.LValue) {
			keyStr := glua.LVAsString(key)
			//nolint:exhaustive // 只处理特定类型
			switch value.Type() {
			case glua.LTString:
				// 类型断言检查
				if strVal, ok := value.(glua.LString); ok {
					args[keyStr] = []string{string(strVal)}
				}
			case glua.LTNumber:
				args[keyStr] = []string{glua.LVAsString(value)}
			case glua.LTTable:
				// 数组形式的多值
				// 类型断言检查
				arr, ok := value.(*glua.LTable)
				if !ok {
					return // 跳过当前回调
				}
				values := []string{}
				arr.ForEach(func(_, v glua.LValue) {
					values = append(values, glua.LVAsString(v))
				})
				args[keyStr] = values
			default:
				// 其他类型不处理
			}
		})

		// 构建查询字符串
		if len(args) > 0 {
			query := fasthttp.Args{}
			for key, values := range args {
				for _, v := range values {
					query.Add(key, v)
				}
			}
			api.ctx.Request.URI().SetQueryString(query.String())
		}

	default:
		L.RaiseError("set_uri_args expects table or string, got %s", argType.Type().String())
		return 0
	}

	// 记录指标
	elapsed := uint64(time.Since(start).Nanoseconds())
	api.metrics.CompatibleCallCount++
	api.metrics.CompatibleTotalNs += elapsed
	if elapsed > api.metrics.CompatibleMaxNs {
		api.metrics.CompatibleMaxNs = elapsed
	}

	return 0
}

// ==================== 请求头 API ====================

// luaGetHeaders 实现 ngx.req.get_headers(max_headers?) - 兼容层
// Lua 调用: local headers = ngx.req.get_headers() 或 ngx.req.get_headers(50)
// 返回: table (如 { ["host"] = "example.com", ["cookie"] = { "a=1", "b=2" } })
// 注意：兼容层需要遍历所有请求头，模拟 nginx 的头表结构
func (api *ngxReqAPI) luaGetHeaders(L *glua.LState) int {
	start := time.Now()

	// 获取可选的 max_headers 参数
	maxHeaders := 100 // 默认最大头数
	if L.GetTop() >= 1 {
		maxHeaders = L.ToInt(1)
		if maxHeaders <= 0 {
			maxHeaders = 100
		}
	}

	// 构建 Lua 表
	result := L.NewTable()
	headers := &api.ctx.Request.Header

	count := 0
	// 使用 All 遍历所有请求头（已弃用的 VisitAll 的替代方法）
	for key, value := range headers.All() {
		if count >= maxHeaders {
			break
		}
		keyStr := string(key)
		valueStr := string(value)

		// 检查是否已存在同名头（多值头）
		existing := result.RawGetString(keyStr)
		if existing == glua.LNil {
			// 第一次遇到这个头
			result.RawSetString(keyStr, glua.LString(valueStr))
		} else if existingStr, ok := existing.(glua.LString); ok {
			// 第二次遇到，需要转换为数组
			arr := L.NewTable()
			arr.Append(existingStr)
			arr.Append(glua.LString(valueStr))
			result.RawSetString(keyStr, arr)
		} else if existingArr, ok := existing.(*glua.LTable); ok {
			// 已经是数组，追加
			existingArr.Append(glua.LString(valueStr))
		}
		count++
	}

	// 记录指标
	elapsed := uint64(time.Since(start).Nanoseconds())
	api.metrics.CompatibleCallCount++
	api.metrics.CompatibleTotalNs += elapsed
	if elapsed > api.metrics.CompatibleMaxNs {
		api.metrics.CompatibleMaxNs = elapsed
	}

	L.Push(result)
	return 1
}

// luaSetHeader 实现 ngx.req.set_header(key, value) - 直接映射层
// Lua 调用: ngx.req.set_header("X-Custom", "value") 或 ngx.req.set_header("X-Custom", nil) 清除
// 参数:
//   - key: 头名称
//   - value: 头值，如果为 nil 则清除该头
func (api *ngxReqAPI) luaSetHeader(L *glua.LState) int {
	start := time.Now()

	// 获取参数
	key := L.CheckString(1)
	value := L.Get(2)

	if value == glua.LNil {
		// 值为 nil，删除头
		api.ctx.Request.Header.Del(key)
	} else {
		// 设置头值
		valueStr := glua.LVAsString(value)
		api.ctx.Request.Header.Set(key, valueStr)
	}

	// 记录指标
	elapsed := uint64(time.Since(start).Nanoseconds())
	api.metrics.DirectCallCount++
	api.metrics.DirectTotalNs += elapsed
	if elapsed > api.metrics.DirectMaxNs {
		api.metrics.DirectMaxNs = elapsed
	}

	return 0
}

// luaClearHeader 实现 ngx.req.clear_header(key) - 直接映射层
// Lua 调用: ngx.req.clear_header("X-Custom")
// 参数:
//   - key: 要清除的头名称
func (api *ngxReqAPI) luaClearHeader(L *glua.LState) int {
	start := time.Now()

	// 获取参数
	key := L.CheckString(1)

	// 删除头
	api.ctx.Request.Header.Del(key)

	// 记录指标
	elapsed := uint64(time.Since(start).Nanoseconds())
	api.metrics.DirectCallCount++
	api.metrics.DirectTotalNs += elapsed
	if elapsed > api.metrics.DirectMaxNs {
		api.metrics.DirectMaxNs = elapsed
	}

	return 0
}

// ==================== 请求体 API ====================

// luaGetBodyData 实现 ngx.req.get_body_data() - 兼容层
// Lua 调用: local body = ngx.req.get_body_data()
// 返回: string 或 nil（如果没有请求体）
func (api *ngxReqAPI) luaGetBodyData(L *glua.LState) int {
	start := time.Now()

	// 获取请求体
	body := api.ctx.Request.Body()

	if len(body) == 0 {
		L.Push(glua.LNil)
	} else {
		L.Push(glua.LString(body))
	}

	// 记录指标
	elapsed := uint64(time.Since(start).Nanoseconds())
	api.metrics.CompatibleCallCount++
	api.metrics.CompatibleTotalNs += elapsed
	if elapsed > api.metrics.CompatibleMaxNs {
		api.metrics.CompatibleMaxNs = elapsed
	}

	return 1
}

// luaReadBody 实现 ngx.req.read_body() - 伪非阻塞层
// Lua 调用: ngx.req.read_body() -- 完成后返回
// 注意：fasthttp 已经预读取了请求体，这里主要是确保请求体已被读取
func (api *ngxReqAPI) luaReadBody(_ *glua.LState) int {
	start := time.Now()

	// fasthttp 默认会预读取请求体到内存中
	// 这里我们只需要确保请求体已被读取（对于 POST/PUT 等请求）
	// 如果请求体未读取，触发读取
	if api.ctx.Request.Header.ContentLength() > 0 {
		// 访问 Body() 会确保请求体已被读取
		_ = api.ctx.Request.Body()
	}

	// 记录指标（使用伪非阻塞层指标）
	elapsed := uint64(time.Since(start).Nanoseconds())
	api.metrics.PseudoBlockingCallCount++
	api.metrics.PseudoBlockingTotalNs += elapsed
	if elapsed > api.metrics.PseudoBlockingMaxNs {
		api.metrics.PseudoBlockingMaxNs = elapsed
	}

	return 0
}

// ==================== 辅助函数 ====================

// GetMetrics 返回 API 调用指标
// 用于基准测试和性能监控
func (api *ngxReqAPI) GetMetrics() ngxReqMetrics {
	return api.metrics
}

// GetDirectLayerAvgNs 返回直接映射层平均延迟（纳秒）
func (api *ngxReqAPI) GetDirectLayerAvgNs() float64 {
	if api.metrics.DirectCallCount == 0 {
		return 0
	}
	return float64(api.metrics.DirectTotalNs) / float64(api.metrics.DirectCallCount)
}

// GetCompatibleLayerAvgNs 返回兼容层平均延迟（纳秒）
func (api *ngxReqAPI) GetCompatibleLayerAvgNs() float64 {
	if api.metrics.CompatibleCallCount == 0 {
		return 0
	}
	return float64(api.metrics.CompatibleTotalNs) / float64(api.metrics.CompatibleCallCount)
}

// GetPerformanceRatio 返回兼容层/直接映射层的性能比率
// ratio > 1.2 表示兼容层比直接映射层慢 20% 以上
func (api *ngxReqAPI) GetPerformanceRatio() float64 {
	directAvg := api.GetDirectLayerAvgNs()
	compatibleAvg := api.GetCompatibleLayerAvgNs()

	if directAvg == 0 {
		return 0
	}
	return compatibleAvg / directAvg
}

// ResetMetrics 重置指标（用于基准测试）
func (api *ngxReqAPI) ResetMetrics() {
	api.metrics = ngxReqMetrics{}
}

// RegisterSchedulerUnsafeReqAPI 为 Scheduler LState 注册不安全的 ngx.req API
func RegisterSchedulerUnsafeReqAPI(L *glua.LState, ngx *glua.LTable) {
	methods := []string{
		"get_method",
		"get_uri",
		"set_uri",
		"get_uri_args",
		"set_uri_args",
		"get_headers",
		"set_header",
		"clear_header",
		"get_body_data",
		"read_body",
	}
	RegisterUnsafeAPI(L, ngx, "ngx.req", methods)
}
