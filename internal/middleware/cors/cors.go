package cors

import (
	"bytes"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/middleware"
)

// CORSConfig holds CORS middleware configuration.
type CORSConfig struct {
	Enabled          bool     `yaml:"enabled"`
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers"`
	ExposeHeaders    []string `yaml:"expose_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	MaxAge           int      `yaml:"max_age"`
}

// CORSMiddleware implements CORS (Cross-Origin Resource Sharing) handling.
type CORSMiddleware struct {
	cfg        *CORSConfig
	wildcard   bool
	originSet  map[string]struct{}
	methodsVal []byte
	headersVal []byte
	exposeVal  []byte
	maxAgeVal  []byte
}

var _ middleware.Middleware = (*CORSMiddleware)(nil)

// New creates a new CORS middleware from the given configuration.
func New(cfg *CORSConfig) *CORSMiddleware {
	if cfg == nil {
		return &CORSMiddleware{}
	}

	if !cfg.Enabled || len(cfg.AllowedOrigins) == 0 {
		return &CORSMiddleware{cfg: cfg}
	}

	m := &CORSMiddleware{
		cfg:       cfg,
		originSet: make(map[string]struct{}, len(cfg.AllowedOrigins)),
	}

	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			m.wildcard = true
			continue
		}
		m.originSet[o] = struct{}{}
	}

	if len(cfg.AllowedMethods) > 0 {
		m.methodsVal = []byte(joinStrings(cfg.AllowedMethods))
	}
	if len(cfg.AllowedHeaders) > 0 {
		m.headersVal = []byte(joinStrings(cfg.AllowedHeaders))
	}
	if len(cfg.ExposeHeaders) > 0 {
		m.exposeVal = []byte(joinStrings(cfg.ExposeHeaders))
	}
	if cfg.MaxAge > 0 {
		m.maxAgeVal = []byte(intToStr(cfg.MaxAge))
	}

	return m
}

// Name returns the middleware name.
func (c *CORSMiddleware) Name() string { return "CORS" }

// Process implements the middleware.Middleware interface.
func (c *CORSMiddleware) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		if c.cfg == nil || !c.cfg.Enabled || len(c.cfg.AllowedOrigins) == 0 {
			next(ctx)
			return
		}

		origin := ctx.Request.Header.Peek("Origin")
		if len(origin) == 0 {
			next(ctx)
			return
		}

		if !c.matchOrigin(origin) {
			next(ctx)
			return
		}

		if bytes.Equal(ctx.Request.Header.Method(), []byte("OPTIONS")) {
			c.handlePreflight(ctx, origin)
			return
		}

		next(ctx)
		c.setActualHeaders(ctx, origin)
	}
}

func (c *CORSMiddleware) matchOrigin(origin []byte) bool {
	if c.wildcard {
		return true
	}
	_, ok := c.originSet[string(origin)]
	return ok
}

func (c *CORSMiddleware) handlePreflight(ctx *fasthttp.RequestCtx, origin []byte) {
	h := &ctx.Response.Header
	h.SetBytesKV([]byte("Access-Control-Allow-Origin"), origin)

	if len(c.methodsVal) > 0 {
		h.SetBytesKV([]byte("Access-Control-Allow-Methods"), c.methodsVal)
	}
	if len(c.headersVal) > 0 {
		h.SetBytesKV([]byte("Access-Control-Allow-Headers"), c.headersVal)
	}
	if c.cfg.MaxAge > 0 {
		h.SetBytesKV([]byte("Access-Control-Max-Age"), c.maxAgeVal)
	}
	if c.cfg.AllowCredentials {
		h.SetBytesKV([]byte("Access-Control-Allow-Credentials"), []byte("true"))
	}

	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

func (c *CORSMiddleware) setActualHeaders(ctx *fasthttp.RequestCtx, origin []byte) {
	h := &ctx.Response.Header
	h.SetBytesKV([]byte("Access-Control-Allow-Origin"), origin)

	if len(c.exposeVal) > 0 {
		h.SetBytesKV([]byte("Access-Control-Expose-Headers"), c.exposeVal)
	}
	if c.cfg.AllowCredentials {
		h.SetBytesKV([]byte("Access-Control-Allow-Credentials"), []byte("true"))
	}
}

func joinStrings(ss []string) string {
	switch len(ss) {
	case 0:
		return ""
	case 1:
		return ss[0]
	default:
		var buf []byte
		for i, s := range ss {
			if i > 0 {
				buf = append(buf, ',')
			}
			buf = append(buf, s...)
		}
		return string(buf)
	}
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
