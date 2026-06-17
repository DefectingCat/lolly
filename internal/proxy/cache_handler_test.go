package proxy

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestBuildCacheKeyIncludesHostAndVary(t *testing.T) {
	p := &Proxy{}
	ctx1 := &fasthttp.RequestCtx{}
	ctx1.Request.Header.SetMethod("GET")
	ctx1.Request.Header.SetHost("a.example.com")
	ctx1.Request.SetRequestURI("/api/data")

	ctx2 := &fasthttp.RequestCtx{}
	ctx2.Request.Header.SetMethod("GET")
	ctx2.Request.Header.SetHost("b.example.com")
	ctx2.Request.SetRequestURI("/api/data")

	vary := []string{"Accept-Encoding"}
	h1, key1 := p.buildCacheKeyHash(ctx1, vary)
	h2, key2 := p.buildCacheKeyHash(ctx2, vary)

	if h1 == h2 || key1 == key2 {
		t.Fatal("cache keys must differ by Host")
	}
}
