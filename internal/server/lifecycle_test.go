// Package server 提供服务器生命周期相关功能的测试。
//
// 该文件测试生命周期管理中的并发安全场景，包括：
//   - 代理缓存统计与代理创建/清理的并发访问
//
// 作者：xfy
package server

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/proxy"
	"rua.plus/lolly/internal/testutil"
)

// TestGetProxyCacheStats_Concurrent 验证并发统计代理缓存与修改 proxies 切片不产生竞态。
func TestGetProxyCacheStats_Concurrent(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.ServerConfig{{Listen: ":0"}},
	}
	s := New(cfg)

	proxyCfg := &config.ProxyConfig{
		Path:        "/api",
		LoadBalance: "round_robin",
		Timeout:     config.ProxyTimeout{Connect: 5 * time.Second},
		Cache: config.ProxyCacheConfig{
			Enabled: true,
			MaxAge:  10 * time.Second,
		},
	}
	targets := testutil.NewTestTargets("http://localhost:8080")
	p, err := proxy.NewProxy(proxyCfg, targets, nil, nil)
	require.NoError(t, err)

	s.proxies = []*proxy.Proxy{p}

	purgeHandler := &PurgeHandler{server: s}

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(3)

		go func() {
			defer wg.Done()
			_ = s.getProxyCacheStats()
		}()

		go func() {
			defer wg.Done()
			_ = purgeHandler.purgeByPath("/api/test", "GET")
		}()

		go func() {
			defer wg.Done()
			s.proxiesMu.Lock()
			s.proxies = append(s.proxies, p)
			s.proxiesMu.Unlock()
		}()
	}
	wg.Wait()
}
