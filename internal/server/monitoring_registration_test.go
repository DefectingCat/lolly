package server

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/matcher"
)

func TestMonitoringEndpoints_OnlyRegisteredWhenEnabled(t *testing.T) {
	// Case 1: monitoring 未启用时，/_status 不应注册
	cfg := &config.Config{
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Static: []config.StaticConfig{{
				Path: "/",
				Root: t.TempDir(),
			}},
		}},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	cfg.Servers[0].Listen = addr
	srv := New(cfg)
	srv.SetListeners([]net.Listener{ln})
	go srv.Start()
	defer srv.StopWithTimeout(5 * time.Second)

	client := &fasthttp.Client{}
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI("http://" + addr + "/_status")
	req.Header.SetMethod("GET")

	if err := client.Do(req, resp); err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// 未启用时应返回 404（被 static handler 处理，找不到文件）
	assert.Equal(t, fasthttp.StatusNotFound, resp.StatusCode(),
		"status endpoint should NOT be registered when monitoring is disabled")
}

func TestMonitoringEndpoints_ReachableWhenEnabled(t *testing.T) {
	cfg := &config.Config{
		Monitoring: config.MonitoringConfig{
			Status: config.StatusConfig{
				Enabled: true,
				Path:    "/_status",
				Allow:   []string{"127.0.0.1"},
			},
		},
		Servers: []config.ServerConfig{{
			Listen: "127.0.0.1:0",
			Static: []config.StaticConfig{{
				Path: "/",
				Root: t.TempDir(),
			}},
		}},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	cfg.Servers[0].Listen = addr
	srv := New(cfg)
	srv.SetListeners([]net.Listener{ln})
	go srv.Start()
	defer srv.StopWithTimeout(5 * time.Second)

	client := &fasthttp.Client{}
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI("http://" + addr + "/_status")
	req.Header.SetMethod("GET")

	if err := client.Do(req, resp); err != nil {
		t.Fatalf("request failed: %v", err)
	}

	assert.Equal(t, fasthttp.StatusOK, resp.StatusCode(),
		"status endpoint should be reachable when enabled, even with static handler on /")
}

func TestLocationEngine_StatusExactBeatsStaticPrefix(t *testing.T) {
	// 独立验证 location engine 的优先级：exact match 应该 beat prefix /
	engine := matcher.NewLocationEngine()

	engine.AddExact("/_status", func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	}, false)
	engine.AddPrefix("/", func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
	}, false)
	engine.MarkInitialized()

	result := engine.Match([]byte("/_status"))
	if result == nil {
		t.Fatal("expected match")
	}
	if result.LocationType != matcher.LocationTypeExact {
		t.Errorf("expected exact match, got %s", result.LocationType)
	}
}
