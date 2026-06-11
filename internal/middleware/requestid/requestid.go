package requestid

import (
	"bytes"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"

	"rua.plus/lolly/internal/middleware"
)

var requestIDHeader = []byte("X-Request-ID")

// RequestIDMiddleware generates or propagates X-Request-ID for request tracing.
type RequestIDMiddleware struct{}

var _ middleware.Middleware = (*RequestIDMiddleware)(nil)

// New creates a new Request-ID middleware.
func New() *RequestIDMiddleware {
	return &RequestIDMiddleware{}
}

// Name returns the middleware name.
func (m *RequestIDMiddleware) Name() string { return "request_id" }

// Process implements the middleware.Middleware interface.
func (m *RequestIDMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		var id string

		incoming := ctx.Request.Header.PeekBytes(requestIDHeader)
		if len(incoming) > 0 {
			trimmed := bytes.TrimSpace(incoming)
			if len(trimmed) > 0 {
				id = string(trimmed)
			}
		}

		if id == "" {
			id = uuid.New().String()
		}

		ctx.SetUserValue("request_id", id)
		ctx.Response.Header.SetBytesKV(requestIDHeader, []byte(id))

		next(ctx)
	}
}

// GetRequestID extracts the request ID from the request context.
func GetRequestID(ctx *fasthttp.RequestCtx) string {
	if v := ctx.UserValue("request_id"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
