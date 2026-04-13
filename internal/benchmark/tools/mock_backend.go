// Package tools provides testing utilities and mock infrastructure for benchmark tests.
package tools

import (
	"math/rand"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
)

// BackendMode defines the response behavior of the mock backend.
type BackendMode int

const (
	// ModeFixed returns a fixed response with configurable status and body.
	ModeFixed BackendMode = iota
	// ModeDelay adds artificial delay before responding.
	ModeDelay
	// ModeError returns errors for a percentage of requests.
	ModeError
	// ModeRandomResponse returns random status codes.
	ModeRandomResponse
)

// MockBackendConfig configures the mock backend behavior.
type MockBackendConfig struct {
	Body       []byte
	Mode       BackendMode
	StatusCode int
	Delay      time.Duration
	ErrorRate  float64 // 0.0 to 1.0, for ModeError
}

// MockBackend represents a mock fasthttp server for testing.
type MockBackend struct {
	server *fasthttp.Server
	config MockBackendConfig
	mu     sync.RWMutex
}

// StartMockFasthttpBackend starts a mock fasthttp backend server.
// Returns the server address and a cleanup function.
func StartMockFasthttpBackend(config MockBackendConfig) (string, func()) {
	mb := &MockBackend{
		config: config,
	}

	mb.server = &fasthttp.Server{
		Handler: mb.handler,
	}

	// Use in-memory listener for testing
	ln := fasthttputil.NewInmemoryListener()

	// Start server in background
	go func() {
		_ = mb.server.Serve(ln)
	}()

	addr := "127.0.0.1:0" // In-memory listener address

	cleanup := func() {
		_ = mb.server.Shutdown()
		_ = ln.Close()
	}

	return addr, cleanup
}

// handler processes incoming requests based on the configured mode.
func (mb *MockBackend) handler(ctx *fasthttp.RequestCtx) {
	mb.mu.RLock()
	config := mb.config
	mb.mu.RUnlock()

	switch config.Mode {
	case ModeFixed:
		ctx.SetStatusCode(config.StatusCode)
		_, _ = ctx.Write(config.Body)

	case ModeDelay:
		time.Sleep(config.Delay)
		ctx.SetStatusCode(config.StatusCode)
		_, _ = ctx.Write(config.Body)

	case ModeError:
		if rand.Float64() < config.ErrorRate {
			ctx.SetStatusCode(fasthttp.StatusInternalServerError)
			_, _ = ctx.WriteString("internal server error")
			return
		}
		ctx.SetStatusCode(config.StatusCode)
		_, _ = ctx.Write(config.Body)

	case ModeRandomResponse:
		codes := []int{
			fasthttp.StatusOK,
			fasthttp.StatusCreated,
			fasthttp.StatusNoContent,
			fasthttp.StatusBadRequest,
			fasthttp.StatusNotFound,
		}
		ctx.SetStatusCode(codes[rand.Intn(len(codes))])
		_, _ = ctx.Write(config.Body)

	default: // ModeFixed
		ctx.SetStatusCode(config.StatusCode)
		_, _ = ctx.Write(config.Body)
	}
}

// SetConfig updates the backend configuration at runtime.
func (mb *MockBackend) SetConfig(config MockBackendConfig) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.config = config
}

// SimpleMockBackend creates a simple backend with fixed response.
// Returns address and cleanup function.
func SimpleMockBackend(statusCode int, body []byte) (string, func()) {
	return StartMockFasthttpBackend(MockBackendConfig{
		Mode:       ModeFixed,
		StatusCode: statusCode,
		Body:       body,
	})
}

// DelayedMockBackend creates a backend with delayed responses.
// Returns address and cleanup function.
func DelayedMockBackend(delay time.Duration, body []byte) (string, func()) {
	return StartMockFasthttpBackend(MockBackendConfig{
		Mode:       ModeDelay,
		StatusCode: fasthttp.StatusOK,
		Body:       body,
		Delay:      delay,
	})
}

// ErrorMockBackend creates a backend that returns errors at the specified rate.
// Returns address and cleanup function.
func ErrorMockBackend(errorRate float64, body []byte) (string, func()) {
	return StartMockFasthttpBackend(MockBackendConfig{
		Mode:       ModeError,
		StatusCode: fasthttp.StatusOK,
		Body:       body,
		ErrorRate:  errorRate,
	})
}
