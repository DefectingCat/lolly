package server

import "github.com/valyala/fasthttp"

// HealthzHandler returns a liveness probe that always responds 200 {"status":"ok"}.
func HealthzHandler(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(200)
	ctx.SetBodyString(`{"status":"ok"}`)
}

// NewReadyzHandler creates a readiness probe using the provided checker function.
func NewReadyzHandler(checker func() (bool, []string)) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		ctx.SetContentType("application/json")
		ready, reasons := checker()
		if ready {
			ctx.SetStatusCode(200)
			ctx.SetBodyString(`{"status":"ready"}`)
		} else {
			ctx.SetStatusCode(503)
			ctx.SetBodyString(buildReasonsJSON(reasons))
		}
	}
}

func buildReasonsJSON(reasons []string) string {
	if len(reasons) == 0 {
		return `{"status":"not ready"}`
	}
	var buf []byte
	buf = append(buf, `{"status":"not ready","reasons":[`...)
	for i, r := range reasons {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '"')
		buf = append(buf, r...)
		buf = append(buf, '"')
	}
	buf = append(buf, "]}"...)
	return string(buf)
}

// DefaultReadyzChecker returns a readiness checker for the Server.
func DefaultReadyzChecker(s *Server) func() (bool, []string) {
	return func() (bool, []string) {
		if !s.running.Load() {
			return false, []string{"server not running"}
		}
		s.proxiesMu.RLock()
		n := len(s.proxies)
		s.proxiesMu.RUnlock()
		if n == 0 {
			return true, nil
		}
		return true, nil
	}
}
