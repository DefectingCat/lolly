package requestid

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestRequestID_GeneratesUUID(t *testing.T) {
	m := New()
	var capturedID string

	next := func(ctx *fasthttp.RequestCtx) {
		capturedID = GetRequestID(ctx)
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	handler := m.Process(next)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")

	handler(ctx)

	assert.NotEmpty(t, capturedID, "request ID should be generated")
	_, err := uuid.Parse(capturedID)
	assert.NoError(t, err, "generated ID should be valid UUID")

	assert.Equal(t, capturedID, string(ctx.Response.Header.Peek("X-Request-ID")))
}

func TestRequestID_ReusesIncoming(t *testing.T) {
	m := New()
	incomingID := "existing-id-12345"
	var capturedID string

	next := func(ctx *fasthttp.RequestCtx) {
		capturedID = GetRequestID(ctx)
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	handler := m.Process(next)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")
	ctx.Request.Header.Set("X-Request-ID", incomingID)

	handler(ctx)

	assert.Equal(t, incomingID, capturedID)
	assert.Equal(t, incomingID, string(ctx.Response.Header.Peek("X-Request-ID")))
}

func TestRequestID_EmptyHeaderGeneratesNew(t *testing.T) {
	m := New()
	var capturedID string

	next := func(ctx *fasthttp.RequestCtx) {
		capturedID = GetRequestID(ctx)
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	handler := m.Process(next)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")
	ctx.Request.Header.Set("X-Request-ID", "   ")

	handler(ctx)

	assert.NotEmpty(t, capturedID, "empty header should generate new UUID")
	_, err := uuid.Parse(capturedID)
	assert.NoError(t, err)
}

func TestRequestID_UserValueAccessible(t *testing.T) {
	m := New()

	next := func(ctx *fasthttp.RequestCtx) {
		val := ctx.UserValue("request_id")
		assert.NotNil(t, val)
		s, ok := val.(string)
		assert.True(t, ok)
		assert.NotEmpty(t, s)
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	handler := m.Process(next)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")

	handler(ctx)
}

func TestRequestID_ResponseHeaderSet(t *testing.T) {
	m := New()

	next := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	handler := m.Process(next)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")

	handler(ctx)

	respHeader := string(ctx.Response.Header.Peek("X-Request-ID"))
	assert.NotEmpty(t, respHeader)
}

func TestRequestID_GeneratedUUIDValid(t *testing.T) {
	m := New()

	next := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	}

	handler := m.Process(next)

	for range 10 {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/test")

		handler(ctx)

		respHeader := string(ctx.Response.Header.Peek("X-Request-ID"))
		_, err := uuid.Parse(respHeader)
		assert.NoError(t, err, "generated UUID should be valid: %s", respHeader)
	}
}

func TestRequestID_Name(t *testing.T) {
	m := New()
	assert.Equal(t, "request_id", m.Name())
}

func TestGetRequestID_Empty(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	assert.Equal(t, "", GetRequestID(ctx))
}
