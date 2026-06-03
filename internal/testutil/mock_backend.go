package testutil

import (
	"net"
	"time"

	"github.com/valyala/fasthttp"
)

// 预定义的测试数据大小常量
const (
	Size100B  = 100
	Size1KB   = 1024
	Size10KB  = 10 * 1024
	Size100KB = 100 * 1024
	Size1MB   = 1024 * 1024
)

// MockBackendConfig Mock 后端配置
type MockBackendConfig struct {
	Mode         string
	StatusCode   int
	ResponseBody []byte
	ErrorRate    float64
	Delay        time.Duration
}

// Mock 后端运行模式
const (
	ModeNormalResponse  = "normal"
	ModeRandomResponse  = "random"
	ModeErrorResponse   = "error"
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

// DelayedMockBackend 创建一个有延迟的 Mock HTTP 后端。
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

// StartMockFasthttpBackend 创建一个可配置的 Mock HTTP 后端。
func StartMockFasthttpBackend(cfg MockBackendConfig) (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	var requestCount int
	handler := func(ctx *fasthttp.RequestCtx) {
		requestCount++

		if cfg.Delay > 0 {
			time.Sleep(cfg.Delay)
		}

		switch cfg.Mode {
		case ModeErrorResponse:
			if float64(requestCount%100)/100 < cfg.ErrorRate {
				ctx.SetStatusCode(fasthttp.StatusInternalServerError)
				ctx.SetBody(cfg.ResponseBody)
				return
			}
		case ModeDelayedResponse:
			time.Sleep(cfg.Delay)
		case ModeRandomResponse:
			if requestCount%2 == 0 {
				ctx.SetStatusCode(fasthttp.StatusOK)
			} else {
				ctx.SetStatusCode(fasthttp.StatusInternalServerError)
			}
			ctx.SetBody(cfg.ResponseBody)
			return
		}

		ctx.SetStatusCode(cfg.StatusCode)
		ctx.SetBody(cfg.ResponseBody)
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
