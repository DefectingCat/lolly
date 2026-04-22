//go:build integration

// proxy_integration_test.go - 代理集成测试（L2 层，进程内）
//
// 测试反向代理的配置和创建逻辑。
// 实际的网络转发测试在 L3 E2E 测试中进行。
//
// 作者：xfy
package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/proxy"
)

// TestProxyCreation 测试代理创建
func TestProxyCreation(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    10 * time.Second,
			Write:   10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{
			URL:      "http://127.0.0.1:8081",
			Weight:   1,
			MaxFails: 3,
		},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err, "failed to create proxy")
	require.NotNil(t, p)

	// 验证配置
	assert.Equal(t, "round_robin", cfg.LoadBalance)
	assert.Equal(t, 5*time.Second, cfg.Timeout.Connect)
	assert.Equal(t, 10*time.Second, cfg.Timeout.Read)
}

// TestProxyRequestHeaders 测试请求头修改配置
func TestProxyRequestHeaders(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "round_robin",
		Headers: config.ProxyHeaders{
			SetRequest: map[string]string{
				"X-Custom-Header":   "custom-value",
				"X-Forwarded-Proto": "https",
			},
		},
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:8081", Weight: 1},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	// 验证代理配置已设置
	assert.NotNil(t, cfg.Headers.SetRequest)
	assert.Equal(t, "custom-value", cfg.Headers.SetRequest["X-Custom-Header"])
	assert.Equal(t, "https", cfg.Headers.SetRequest["X-Forwarded-Proto"])
}

// TestProxyResponseHeaders 测试响应头修改配置
func TestProxyResponseHeaders(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "round_robin",
		Headers: config.ProxyHeaders{
			SetResponse: map[string]string{
				"X-Server": "lolly",
			},
			Remove: []string{"X-Powered-By"},
		},
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:8081", Weight: 1},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	// 验证响应头配置
	assert.Equal(t, "lolly", cfg.Headers.SetResponse["X-Server"])
	assert.Contains(t, cfg.Headers.Remove, "X-Powered-By")
}

// TestProxyTimeout 测试代理超时配置
func TestProxyTimeout(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 1 * time.Second,
			Read:    50 * time.Millisecond,
			Write:   1 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:8081", Weight: 1},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	// 验证超时配置
	assert.Equal(t, 1*time.Second, cfg.Timeout.Connect)
	assert.Equal(t, 50*time.Millisecond, cfg.Timeout.Read)
	assert.Equal(t, 1*time.Second, cfg.Timeout.Write)
}

// TestProxyLoadBalanceRoundRobin 测试轮询负载均衡配置
func TestProxyLoadBalanceRoundRobin(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:8081", Weight: 1},
		{URL: "http://127.0.0.1:8082", Weight: 1},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	// 验证负载均衡器类型
	assert.Equal(t, "round_robin", cfg.LoadBalance)
	assert.Len(t, targets, 2)
}

// TestProxyWeightedRoundRobin 测试加权轮询配置
func TestProxyWeightedRoundRobin(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "weighted_round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:8081", Weight: 3},
		{URL: "http://127.0.0.1:8082", Weight: 1},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	// 验证权重配置
	assert.Equal(t, 3, targets[0].Weight)
	assert.Equal(t, 1, targets[1].Weight)
}

// TestProxyLeastConn 测试最少连接负载均衡配置
func TestProxyLeastConn(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "least_conn",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:8081", Weight: 1},
		{URL: "http://127.0.0.1:8082", Weight: 1},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	assert.Equal(t, "least_conn", cfg.LoadBalance)
}

// TestProxyIPHash 测试 IP 哈希负载均衡配置
func TestProxyIPHash(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "ip_hash",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:8081", Weight: 1},
		{URL: "http://127.0.0.1:8082", Weight: 1},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	assert.Equal(t, "ip_hash", cfg.LoadBalance)
}

// TestProxyConsistentHash 测试一致性哈希负载均衡配置
func TestProxyConsistentHash(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance:  "consistent_hash",
		HashKey:      "uri",
		VirtualNodes: 150,
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:8081", Weight: 1},
		{URL: "http://127.0.0.1:8082", Weight: 1},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	assert.Equal(t, "consistent_hash", cfg.LoadBalance)
	assert.Equal(t, "uri", cfg.HashKey)
	assert.Equal(t, 150, cfg.VirtualNodes)
}

// TestProxyErrorHandling 测试错误处理配置
func TestProxyErrorHandling(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{
			URL:         "http://127.0.0.1:8081",
			Weight:      1,
			MaxFails:    3,
			FailTimeout: 10 * time.Second,
		},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	// 验证 MaxFails 配置 (int64 类型)
	assert.Equal(t, int64(3), targets[0].MaxFails)
	assert.Equal(t, 10*time.Second, targets[0].FailTimeout)
}

// TestProxyCacheConfig 测试缓存配置
func TestProxyCacheConfig(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "round_robin",
		Cache: config.ProxyCacheConfig{
			Enabled:              true,
			MaxAge:               60 * time.Second,
			Methods:              []string{"GET", "HEAD"},
			MinUses:              1,
			CacheLock:            true,
			CacheLockTimeout:     5 * time.Second,
			StaleWhileRevalidate: 30 * time.Second,
		},
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:8081", Weight: 1},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	// 验证缓存配置
	assert.True(t, cfg.Cache.Enabled)
	assert.Equal(t, 60*time.Second, cfg.Cache.MaxAge)
	assert.Contains(t, cfg.Cache.Methods, "GET")
	assert.True(t, cfg.Cache.CacheLock)
}

// TestProxyNextUpstream 测试故障转移配置
func TestProxyNextUpstream(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "round_robin",
		NextUpstream: config.NextUpstreamConfig{
			Tries:     3,
			HTTPCodes: []int{502, 503, 504},
		},
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:8081", Weight: 1},
		{URL: "http://127.0.0.1:8082", Weight: 1},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	// 验证故障转移配置
	assert.Equal(t, 3, cfg.NextUpstream.Tries)
	assert.Contains(t, cfg.NextUpstream.HTTPCodes, 502)
	assert.Contains(t, cfg.NextUpstream.HTTPCodes, 503)
	assert.Contains(t, cfg.NextUpstream.HTTPCodes, 504)
}

// TestProxyHealthCheck 测试健康检查配置
func TestProxyHealthCheck(t *testing.T) {
	cfg := &config.ProxyConfig{
		LoadBalance: "round_robin",
		HealthCheck: config.HealthCheckConfig{
			Interval: 10 * time.Second,
			Path:     "/health",
			Timeout:  5 * time.Second,
		},
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    10 * time.Second,
		},
	}

	targets := []*loadbalance.Target{
		{URL: "http://127.0.0.1:8081", Weight: 1},
	}

	p, err := proxy.NewProxy(cfg, targets, nil, nil)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	defer p.Stop()

	// 验证健康检查配置
	assert.Equal(t, 10*time.Second, cfg.HealthCheck.Interval)
	assert.Equal(t, "/health", cfg.HealthCheck.Path)
	assert.Equal(t, 5*time.Second, cfg.HealthCheck.Timeout)
}
