// Package lua 提供 ngx.req API 测试
package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	glua "github.com/yuin/gopher-lua"

	"rua.plus/lolly/internal/testutil"
)

// 创建测试用的 fasthttp.RequestCtx
func createTestRequestCtx(method, uri string, headers map[string]string, body []byte) *fasthttp.RequestCtx {
	ctx := testutil.NewRequestCtxWithHeader(method, uri, headers)

	// 设置请求体
	if len(body) > 0 {
		ctx.Request.SetBody(body)
	}

	return ctx
}

// TestNgxReqGetMethod 测试 ngx.req.get_method()
func TestNgxReqGetMethod(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建测试请求
	reqCtx := createTestRequestCtx("POST", "/test", nil, nil)

	// 创建协程
	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	// 设置沙箱（这会自动注册 ngx API）
	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试获取请求方法
	err = coro.Execute(`
		local method = ngx.req.get_method()
		if method ~= "POST" then
			error("expected POST, got " .. tostring(method))
		end
	`)
	assert.NoError(t, err)
}

// TestNgxReqGetURI 测试 ngx.req.get_uri()
func TestNgxReqGetURI(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	reqCtx := createTestRequestCtx("GET", "/path/to/resource?key=value", nil, nil)

	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	err = coro.Execute(`
		local uri = ngx.req.get_uri()
		if uri ~= "/path/to/resource" then
			error("expected /path/to/resource, got " .. tostring(uri))
		end
	`)
	assert.NoError(t, err)
}

// TestNgxReqSetURI 测试 ngx.req.set_uri()
func TestNgxReqSetURI(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 测试设置 URI（不带 jump）
	reqCtx := createTestRequestCtx("GET", "/original", nil, nil)
	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)

	err = coro.SetupSandbox()
	require.NoError(t, err)

	err = coro.Execute(`
		ngx.req.set_uri("/new/path")
	`)
	assert.NoError(t, err)
	coro.Close()

	// 验证 URI 已修改
	assert.Equal(t, "/new/path", string(reqCtx.URI().Path()))

	// 测试设置 URI（带 jump）
	reqCtx2 := createTestRequestCtx("GET", "/original", nil, nil)
	coro2, err := engine.NewCoroutine(reqCtx2)
	require.NoError(t, err)

	err = coro2.SetupSandbox()
	require.NoError(t, err)

	err = coro2.Execute(`
		ngx.req.set_uri("/redirect/path", true)
	`)
	assert.NoError(t, err)
	coro2.Close()

	assert.Equal(t, "/redirect/path", string(reqCtx2.URI().Path()))
	// 验证 jump 标记已设置
	jumpFlag := reqCtx2.UserValue("_ngx_req_internal_jump")
	assert.Equal(t, true, jumpFlag)
}

// TestNgxReqGetURIArgs 测试 ngx.req.get_uri_args()
func TestNgxReqGetURIArgs(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	reqCtx := createTestRequestCtx("GET", "/test?foo=bar&baz=qux&arr=1&arr=2", nil, nil)

	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	err = coro.Execute(`
		local args = ngx.req.get_uri_args()

		if args.foo ~= "bar" then
			error("expected foo=bar, got " .. tostring(args.foo))
		end

		if args.baz ~= "qux" then
			error("expected baz=qux, got " .. tostring(args.baz))
		end

		-- 多值参数应该返回数组
		if type(args.arr) ~= "table" then
			error("expected arr to be table, got " .. type(args.arr))
		end

		if args.arr[1] ~= "1" or args.arr[2] ~= "2" then
			error("expected arr = {1, 2}")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxReqSetURIArgs 测试 ngx.req.set_uri_args()
func TestNgxReqSetURIArgs(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	reqCtx := createTestRequestCtx("GET", "/test", nil, nil)

	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试使用 table 设置参数
	err = coro.Execute(`
		ngx.req.set_uri_args({ key = "value", num = 123 })
	`)
	assert.NoError(t, err)

	queryStr := string(reqCtx.URI().QueryString())
	assert.Contains(t, queryStr, "key=value")
	assert.Contains(t, queryStr, "num=123")

	// 测试使用字符串设置参数
	reqCtx2 := createTestRequestCtx("GET", "/test", nil, nil)
	coro2, err := engine.NewCoroutine(reqCtx2)
	require.NoError(t, err)
	defer coro2.Close()

	err = coro2.SetupSandbox()
	require.NoError(t, err)

	err = coro2.Execute(`
		ngx.req.set_uri_args("foo=bar&baz=qux")
	`)
	assert.NoError(t, err)

	assert.Equal(t, "foo=bar&baz=qux", string(reqCtx2.URI().QueryString()))
}

// TestNgxReqGetHeaders 测试 ngx.req.get_headers()
func TestNgxReqGetHeaders(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	reqCtx := createTestRequestCtx("GET", "/test", map[string]string{
		"Host":         "example.com",
		"X-Custom":     "custom-value",
		"Content-Type": "application/json",
	}, nil)

	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	err = coro.Execute(`
		local headers = ngx.req.get_headers()

		if headers.Host ~= "example.com" then
			error("expected Host=example.com, got " .. tostring(headers.Host))
		end

		if headers["X-Custom"] ~= "custom-value" then
			error("expected X-Custom=custom-value")
		end

		if headers["Content-Type"] ~= "application/json" then
			error("expected Content-Type=application/json")
		end
	`)
	assert.NoError(t, err)
}

// TestNgxReqSetHeader 测试 ngx.req.set_header()
func TestNgxReqSetHeader(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 测试设置请求头
	reqCtx := createTestRequestCtx("GET", "/test", nil, nil)
	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)

	err = coro.SetupSandbox()
	require.NoError(t, err)

	err = coro.Execute(`
		ngx.req.set_header("X-Custom-Header", "custom-value")
	`)
	assert.NoError(t, err)
	coro.Close()

	assert.Equal(t, "custom-value", string(reqCtx.Request.Header.Peek("X-Custom-Header")))

	// 测试使用 nil 清除请求头
	reqCtx2 := createTestRequestCtx("GET", "/test", map[string]string{
		"X-Custom-Header": "custom-value",
	}, nil)
	coro2, err := engine.NewCoroutine(reqCtx2)
	require.NoError(t, err)

	err = coro2.SetupSandbox()
	require.NoError(t, err)

	err = coro2.Execute(`
		ngx.req.set_header("X-Custom-Header", nil)
	`)
	assert.NoError(t, err)
	coro2.Close()

	assert.Equal(t, "", string(reqCtx2.Request.Header.Peek("X-Custom-Header")))
}

// TestNgxReqClearHeader 测试 ngx.req.clear_header()
func TestNgxReqClearHeader(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	reqCtx := createTestRequestCtx("GET", "/test", map[string]string{
		"X-To-Clear": "value",
	}, nil)

	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 先验证头存在
	assert.Equal(t, "value", string(reqCtx.Request.Header.Peek("X-To-Clear")))

	// 清除头
	err = coro.Execute(`
		ngx.req.clear_header("X-To-Clear")
	`)
	assert.NoError(t, err)

	assert.Equal(t, "", string(reqCtx.Request.Header.Peek("X-To-Clear")))
}

// TestNgxReqGetBodyData 测试 ngx.req.get_body_data()
func TestNgxReqGetBodyData(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	reqCtx := createTestRequestCtx("POST", "/test", nil, []byte("test body data"))

	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	err = coro.Execute(`
		local body = ngx.req.get_body_data()
		if body ~= "test body data" then
			error("expected 'test body data', got " .. tostring(body))
		end
	`)
	assert.NoError(t, err)
}

// TestNgxReqGetBodyDataEmpty 测试 ngx.req.get_body_data() 空请求体
func TestNgxReqGetBodyDataEmpty(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	reqCtx := createTestRequestCtx("GET", "/test", nil, nil)

	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	err = coro.Execute(`
		local body = ngx.req.get_body_data()
		if body ~= nil then
			error("expected nil for empty body, got " .. tostring(body))
		end
	`)
	assert.NoError(t, err)
}

// TestNgxReqReadBody 测试 ngx.req.read_body()
func TestNgxReqReadBody(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	reqCtx := createTestRequestCtx("POST", "/test", map[string]string{
		"Content-Length": "14",
	}, []byte("test body data"))

	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// read_body 应该成功执行
	err = coro.Execute(`
		ngx.req.read_body()
	`)
	assert.NoError(t, err)

	// 验证请求体仍可访问
	body := reqCtx.Request.Body()
	assert.Equal(t, "test body data", string(body))
}

// TestNgxReqAPIIntegration 测试 ngx.req API 集成场景
func TestNgxReqAPIIntegration(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	reqCtx := createTestRequestCtx("POST", "/api/users?limit=10&offset=20", map[string]string{
		"Content-Type": "application/json",
		"X-API-Key":    "secret123",
	}, []byte(`{"name":"test"}`))

	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 复杂场景：获取各种请求信息并修改
	err = coro.Execute(`
		-- 获取请求信息
		local method = ngx.req.get_method()
		local uri = ngx.req.get_uri()
		local args = ngx.req.get_uri_args()
		local headers = ngx.req.get_headers()

		-- 验证获取的信息
		if method ~= "POST" then
			error("method should be POST")
		end

		if uri ~= "/api/users" then
			error("uri should be /api/users, got " .. tostring(uri))
		end

		if args.limit ~= "10" or args.offset ~= "20" then
			error("args incorrect")
		end

		-- 注意：fasthttp 会标准化 header 名称，所以需要使用实际的 key
		if headers["Content-Type"] ~= "application/json" and headers["content-type"] ~= "application/json" then
			error("Content-Type header incorrect: " .. tostring(headers["Content-Type"]))
		end

		-- 修改请求
		ngx.req.set_header("X-Request-ID", "req-12345")
		ngx.req.set_uri("/api/v2/users")
	`)
	assert.NoError(t, err)

	// 在 Go 层验证修改
	assert.Equal(t, "/api/v2/users", string(reqCtx.URI().Path()))
	assert.Equal(t, "req-12345", string(reqCtx.Request.Header.Peek("X-Request-ID")))
}

// TestNgxReqMetrics 测试 ngx.req API 性能指标
func TestNgxReqMetrics(t *testing.T) {
	reqCtx := createTestRequestCtx("GET", "/test?a=1&b=2", nil, nil)
	api := newNgxReqAPI(reqCtx)

	L := glua.NewState()
	defer L.Close()

	// 创建 ngx 表
	ngx := L.NewTable()

	// 注册 API
	RegisterNgxReqAPI(L, api, ngx)

	// 将 ngx 设置到全局
	L.SetGlobal("ngx", ngx)

	// 调用各种 API
	L.DoString(`
		ngx.req.get_method()
		ngx.req.get_uri()
		ngx.req.get_uri_args()
	`)

	// 验证指标
	metrics := api.GetMetrics()
	assert.Greater(t, metrics.DirectCallCount, uint64(0), "应该有直接层调用")
	assert.Greater(t, metrics.CompatibleCallCount, uint64(0), "应该有兼容层调用")

	// 验证平均延迟
	directAvg := api.GetDirectLayerAvgNs()
	compatibleAvg := api.GetCompatibleLayerAvgNs()
	assert.GreaterOrEqual(t, directAvg, float64(0))
	assert.GreaterOrEqual(t, compatibleAvg, float64(0))

	// 验证性能比率
	ratio := api.GetPerformanceRatio()
	assert.GreaterOrEqual(t, ratio, float64(0))

	// 重置指标
	api.ResetMetrics()
	metrics = api.GetMetrics()
	assert.Equal(t, uint64(0), metrics.DirectCallCount)
	assert.Equal(t, uint64(0), metrics.CompatibleCallCount)
}

// TestNgxReqGetHeadersWithMaxHeaders 测试 ngx.req.get_headers(max_headers)
func TestNgxReqGetHeadersWithMaxHeaders(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	// 创建带有多个头的请求
	reqCtx := &fasthttp.RequestCtx{}
	reqCtx.Request.Header.SetMethod("GET")
	reqCtx.Request.SetRequestURI("/test")
	reqCtx.Request.Header.Set("Header1", "value1")
	reqCtx.Request.Header.Set("Header2", "value2")
	reqCtx.Request.Header.Set("Header3", "value3")

	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试限制头数
	err = coro.Execute(`
		local headers = ngx.req.get_headers(2)
		local count = 0
		for k, v in pairs(headers) do
			count = count + 1
		end
		-- 应该最多返回 2 个头
		if count > 2 then
			error("expected at most 2 headers, got " .. count)
		end
	`)
	assert.NoError(t, err)
}

// TestNgxReqSetURIArgsWithArray 测试 ngx.req.set_uri_args() 使用数组值
func TestNgxReqSetURIArgsWithArray(t *testing.T) {
	engine, err := NewEngine(DefaultConfig())
	require.NoError(t, err)
	defer engine.Close()

	reqCtx := createTestRequestCtx("GET", "/test", nil, nil)

	coro, err := engine.NewCoroutine(reqCtx)
	require.NoError(t, err)
	defer coro.Close()

	err = coro.SetupSandbox()
	require.NoError(t, err)

	// 测试使用包含数组的 table
	err = coro.Execute(`
		ngx.req.set_uri_args({ tags = { "a", "b", "c" }, page = 1 })
	`)
	assert.NoError(t, err)

	queryStr := string(reqCtx.URI().QueryString())
	assert.Contains(t, queryStr, "tags=a")
	assert.Contains(t, queryStr, "tags=b")
	assert.Contains(t, queryStr, "tags=c")
	assert.Contains(t, queryStr, "page=1")
}
