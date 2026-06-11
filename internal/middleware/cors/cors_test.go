package cors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func newTestHandler() (*fasthttp.RequestCtx, fasthttp.RequestHandler) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")
	return ctx, func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("ok")
	}
}

func TestDisabled_PassesThrough(t *testing.T) {
	cfg := &CORSConfig{Enabled: false}
	m := New(cfg)
	ctx, next := newTestHandler()
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, "ok", string(ctx.Response.Body()))
	assert.Empty(t, string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")))
}

func TestNilConfig_PassesThrough(t *testing.T) {
	m := New(nil)
	ctx, next := newTestHandler()
	ctx.Request.Header.Set("Origin", "https://example.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Empty(t, string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")))
}

func TestNoOrigin_PassesThrough(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://example.com"},
	}
	m := New(cfg)
	ctx, next := newTestHandler()
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Empty(t, string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")))
}

func TestNonMatchingOrigin_NoCORSHeaders(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://example.com"},
	}
	m := New(cfg)
	ctx, next := newTestHandler()
	ctx.Request.Header.Set("Origin", "https://evil.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Empty(t, string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")))
}

func TestMatchingOrigin_SetsCORSHeaders(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:          true,
		AllowedOrigins:   []string{"https://example.com"},
		AllowCredentials: true,
		ExposeHeaders:    []string{"X-Custom", "X-Another"},
	}
	m := New(cfg)
	ctx, next := newTestHandler()
	ctx.Request.Header.Set("Origin", "https://example.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, "https://example.com", string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")))
	assert.Equal(t, "true", string(ctx.Response.Header.Peek("Access-Control-Allow-Credentials")))
	assert.Equal(t, "X-Custom,X-Another", string(ctx.Response.Header.Peek("Access-Control-Expose-Headers")))
}

func TestPreflight_Returns204(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET", "POST", "PUT"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:         3600,
	}
	m := New(cfg)
	ctx, next := newTestHandler()
	ctx.Request.Header.SetMethod("OPTIONS")
	ctx.Request.Header.Set("Origin", "https://example.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, fasthttp.StatusNoContent, ctx.Response.StatusCode())
	assert.Equal(t, "https://example.com", string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")))
	assert.Equal(t, "GET,POST,PUT", string(ctx.Response.Header.Peek("Access-Control-Allow-Methods")))
	assert.Equal(t, "Content-Type,Authorization", string(ctx.Response.Header.Peek("Access-Control-Allow-Headers")))
	assert.Equal(t, "3600", string(ctx.Response.Header.Peek("Access-Control-Max-Age")))
}

func TestWildcardOrigin_MatchesAny(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
	}
	m := New(cfg)
	ctx, next := newTestHandler()
	ctx.Request.Header.Set("Origin", "https://anything.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, "https://anything.com", string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")))
}

func TestAllowCredentials_SetsHeader(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:          true,
		AllowedOrigins:   []string{"https://example.com"},
		AllowCredentials: true,
	}
	m := New(cfg)
	ctx, next := newTestHandler()
	ctx.Request.Header.Set("Origin", "https://example.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, "true", string(ctx.Response.Header.Peek("Access-Control-Allow-Credentials")))
}

func TestMaxAge_SetsHeaderWhenPositive(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET"},
		MaxAge:         7200,
	}
	m := New(cfg)
	ctx, next := newTestHandler()
	ctx.Request.Header.SetMethod("OPTIONS")
	ctx.Request.Header.Set("Origin", "https://example.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, "7200", string(ctx.Response.Header.Peek("Access-Control-Max-Age")))
}

func TestMaxAge_NotSetWhenZero(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET"},
		MaxAge:         0,
	}
	m := New(cfg)
	ctx, next := newTestHandler()
	ctx.Request.Header.SetMethod("OPTIONS")
	ctx.Request.Header.Set("Origin", "https://example.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Empty(t, string(ctx.Response.Header.Peek("Access-Control-Max-Age")))
}

func TestExposeHeaders_OnActualRequest(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://example.com"},
		ExposeHeaders:  []string{"X-Total-Count", "X-Page"},
	}
	m := New(cfg)
	ctx, next := newTestHandler()
	ctx.Request.Header.Set("Origin", "https://example.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, "X-Total-Count,X-Page", string(ctx.Response.Header.Peek("Access-Control-Expose-Headers")))
}

func TestMultipleOrigins_OnlyMatchingEchoedBack(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://a.com", "https://b.com", "https://c.com"},
	}
	m := New(cfg)

	for _, origin := range []string{"https://a.com", "https://b.com", "https://c.com"} {
		ctx, next := newTestHandler()
		ctx.Request.Header.Set("Origin", origin)
		handler := m.Process(next)
		handler(ctx)
		assert.Equal(t, origin, string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")))
	}

	ctx, next := newTestHandler()
	ctx.Request.Header.Set("Origin", "https://evil.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Empty(t, string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")))
}

func TestEmptyAllowedOrigins_PassesThrough(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{},
	}
	m := New(cfg)
	ctx, next := newTestHandler()
	ctx.Request.Header.Set("Origin", "https://example.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Empty(t, string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")))
}

func TestPreflight_WithCredentials(t *testing.T) {
	cfg := &CORSConfig{
		Enabled:          true,
		AllowedOrigins:   []string{"https://example.com"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowCredentials: true,
	}
	m := New(cfg)
	ctx, next := newTestHandler()
	ctx.Request.Header.SetMethod("OPTIONS")
	ctx.Request.Header.Set("Origin", "https://example.com")
	handler := m.Process(next)
	handler(ctx)
	assert.Equal(t, fasthttp.StatusNoContent, ctx.Response.StatusCode())
	assert.Equal(t, "true", string(ctx.Response.Header.Peek("Access-Control-Allow-Credentials")))
	assert.Equal(t, "https://example.com", string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")))
}

func TestName(t *testing.T) {
	m := New(nil)
	assert.Equal(t, "CORS", m.Name())
}

func TestPreflight_DoesNotCallNext(t *testing.T) {
	called := false
	cfg := &CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET"},
	}
	m := New(cfg)
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")
	ctx.Request.Header.SetMethod("OPTIONS")
	ctx.Request.Header.Set("Origin", "https://example.com")
	handler := m.Process(func(ctx *fasthttp.RequestCtx) {
		called = true
	})
	handler(ctx)
	assert.False(t, called, "next handler should not be called for preflight requests")
	assert.Equal(t, fasthttp.StatusNoContent, ctx.Response.StatusCode())
}
