// Package tools 提供基准测试和集成测试的辅助工具。
//
// 包含 Mock 后端创建、测试数据生成等工具函数。
//
// 作者：xfy
package tools

import (
	"net"
	"time"

	"github.com/valyala/fasthttp"
)

// 预定义的测试数据大小常量
const (
	// Size100B 100 字节测试数据。
	Size100B = 100
	// Size1KB 1KB 测试数据。
	Size1KB = 1024
	// Size10KB 10KB 测试数据。
	Size10KB = 10 * 1024
	// Size100KB 100KB 测试数据。
	Size100KB = 100 * 1024
	// Size1MB 1MB 测试数据。
	Size1MB = 1024 * 1024
)

// MockBackendConfig Mock 后端配置。
type MockBackendConfig struct {
	// Mode 运行模式
	Mode string
	// StatusCode 响应状态码
	StatusCode int
	// ResponseBody 响应体
	ResponseBody []byte
	// ErrorRate 错误率 (0.0 - 1.0)
	ErrorRate float64
	// Delay 响应延迟
	Delay time.Duration
}

// Mock 后端运行模式
const (
	// ModeNormalResponse 正常响应模式。
	ModeNormalResponse = "normal"
	// ModeRandomResponse 随机响应模式。
	ModeRandomResponse = "random"
	// ModeErrorResponse 错误响应模式。
	ModeErrorResponse = "error"
	// ModeDelayedResponse 延迟响应模式。
	ModeDelayedResponse = "delayed"
)

// GenerateTestData 生成指定大小的测试数据。
func GenerateTestData(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	return data
}

// SimpleMockBackend 创建一个简单的 Mock HTTP 后端。
// 返回监听地址和清理函数。
func SimpleMockBackend(statusCode int, responseBody []byte) (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(statusCode)
		ctx.SetBody(responseBody)
	}

	go func() {
		if serveErr := fasthttp.Serve(ln, handler); serveErr != nil {
			panic(serveErr)
		}
	}()

	return ln.Addr().String(), func() {
		_ = ln.Close()
	}
}

// ErrorMockBackend 创建一个返回错误的 Mock HTTP 后端。
func ErrorMockBackend(errorRate float64, errorBody []byte) (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	var requestCount int
	handler := func(ctx *fasthttp.RequestCtx) {
		requestCount++
		if float64(requestCount%100)/100 < errorRate {
			ctx.SetStatusCode(fasthttp.StatusInternalServerError)
			ctx.SetBody(errorBody)
			return
		}
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBody([]byte("OK"))
	}

	go func() {
		if serveErr := fasthttp.Serve(ln, handler); serveErr != nil {
			panic(serveErr)
		}
	}()

	return ln.Addr().String(), func() {
		_ = ln.Close()
	}
}

// DelayedMockBackend 创建一个带延迟的 Mock HTTP 后端。
func DelayedMockBackend(delay time.Duration, statusCode int, responseBody []byte) (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	handler := func(ctx *fasthttp.RequestCtx) {
		time.Sleep(delay)
		ctx.SetStatusCode(statusCode)
		ctx.SetBody(responseBody)
	}

	go func() {
		if serveErr := fasthttp.Serve(ln, handler); serveErr != nil {
			panic(serveErr)
		}
	}()

	return ln.Addr().String(), func() {
		_ = ln.Close()
	}
}

// StartMockFasthttpBackend 根据配置启动 Mock HTTP 后端。
func StartMockFasthttpBackend(config MockBackendConfig) (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	var requestCount int
	handler := func(ctx *fasthttp.RequestCtx) {
		requestCount++

		switch config.Mode {
		case ModeDelayedResponse:
			time.Sleep(config.Delay)
			ctx.SetStatusCode(config.StatusCode)
			ctx.SetBody(config.ResponseBody)

		case ModeErrorResponse:
			if float64(requestCount%100)/100 < config.ErrorRate {
				ctx.SetStatusCode(fasthttp.StatusInternalServerError)
				ctx.SetBody([]byte(`{"error": "internal error"}`))
				return
			}
			ctx.SetStatusCode(config.StatusCode)
			ctx.SetBody(config.ResponseBody)

		case ModeRandomResponse:
			if requestCount%2 == 0 {
				ctx.SetStatusCode(fasthttp.StatusOK)
			} else {
				ctx.SetStatusCode(fasthttp.StatusCreated)
			}
			ctx.SetBody(config.ResponseBody)

		default:
			ctx.SetStatusCode(config.StatusCode)
			ctx.SetBody(config.ResponseBody)
		}
	}

	go func() {
		if serveErr := fasthttp.Serve(ln, handler); serveErr != nil {
			panic(serveErr)
		}
	}()

	return ln.Addr().String(), func() {
		_ = ln.Close()
	}
}
