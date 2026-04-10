// Package lua 提供 ngx.req API 实现
// 本文件实现双层 API 边界验证原型，用于测量直接映射层 vs 兼容层的性能差异
package lua

import (
	"strconv"
	"strings"
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

func (l ngxReqAPILayer) String() string {
	switch l {
	case APILayerDirect:
		return "direct"
	case APILayerCompatible:
		return "compatible"
	case APILayerPseudoNonBlocking:
		return "pseudo_non_blocking"
	default:
		return "unknown"
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
	// 请求上下文
	ctx *fasthttp.RequestCtx

	// 指标收集
	metrics ngxReqMetrics

	// 缓存：URI args 解析结果（兼容层使用）
	uriArgsCache     map[string][]string
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
func RegisterNgxReqAPI(L *glua.LState, api *ngxReqAPI) {
	// 创建 ngx 表
	ngx := L.NewTable()

	// 创建 ngx.req 子表
	ngxReq := L.NewTable()

	// 直接映射层 API：get_method
	// 特点：直接访问 fasthttp.RequestCtx，零拷贝，最小开销
	ngxReq.RawSetString("get_method", L.NewFunction(api.luaGetMethod))

	// 直接映射层 API：get_uri
	// 特点：直接返回请求的 URI 路径（不含 query string）
	ngxReq.RawSetString("get_uri", L.NewFunction(api.luaGetURI))

	// 兼容层 API：get_uri_args
	// 特点：需要解析 query string 为 nginx 兼容的表结构
	// 增加了解析开销，但保持 API 兼容性
	ngxReq.RawSetString("get_uri_args", L.NewFunction(api.luaGetURIArgs))

	// 伪非阻塞层 API：read_body
	// 特点：使用 yield/resume 模式支持异步读取
	// 这是实验性 API，展示非阻塞调用模式
	ngxReq.RawSetString("read_body", L.NewFunction(api.luaReadBodyAsync))

	// 将 ngx.req 添加到 ngx
	ngx.RawSetString("req", ngxReq)

	// 注册 ngx 全局变量
	L.SetGlobal("ngx", ngx)
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

// ==================== 伪非阻塞层 API（实验性） ====================

// luaReadBodyAsync 实现 ngx.req.read_body() - 伪非阻塞层
// Lua 调用: ngx.req.read_body() -- 会 yield，完成后 resume
// 这是实验性 API，展示如何使用 yield/resume 实现非阻塞调用
func (api *ngxReqAPI) luaReadBodyAsync(L *glua.LState) int {
	// 伪非阻塞层：使用 yield 暂停协程，由引擎异步处理后 resume
	// 这种模式允许在 Lua 中编写看似同步的代码，实际是异步执行

	// 记录开始时间
	start := time.Now()

	// Yield 协程 - 控制权交回 Go 层
	// TODO: 实现真正的非阻塞 yield，目前使用同步模拟
	L.Push(glua.LString("read_body"))
	L.Push(glua.LString(strconv.FormatInt(start.UnixNano(), 10)))
	// Note: 在 gopher-lua v1.1.2 中，L.Yield 需要 LValue 参数，返回 int
	// 这里返回 2 表示有 2 个返回值已在栈上
	return 2 // 使用 return 代替 L.Yield
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

// ==================== 辅助方法 ====================

// getRequestHeader 获取请求头（辅助函数，供 Lua 绑定使用）
func (api *ngxReqAPI) getRequestHeader(name string) string {
	return string(api.ctx.Request.Header.Peek(name))
}

// setResponseHeader 设置响应头（辅助函数，供 Lua 绑定使用）
func (api *ngxReqAPI) setResponseHeader(name, value string) {
	api.ctx.Response.Header.Set(name, value)
}

// parseQueryString 手动解析 query string（用于对比测试）
// 这是纯 Go 实现，不依赖 fasthttp 的解析器
func parseQueryString(query []byte) map[string][]string {
	result := make(map[string][]string)
	if len(query) == 0 {
		return result
	}

	pairs := strings.Split(string(query), "&")
	for _, pair := range pairs {
		if len(pair) == 0 {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		key := parts[0]
		value := ""
		if len(parts) > 1 {
			value = parts[1]
		}

		if existing, ok := result[key]; ok {
			result[key] = append(existing, value)
		} else {
			result[key] = []string{value}
		}
	}

	return result
}
