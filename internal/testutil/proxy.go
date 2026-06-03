package testutil

import (
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
)

// NewTestProxyConfig 创建测试用的代理配置
//
// 参数：
//   - path: 代理路径
//   - targetURLs: 后端目标 URL 列表
//
// 返回值：
//   - *config.ProxyConfig: 配置好的代理配置
func NewTestProxyConfig(path string, targetURLs ...string) *config.ProxyConfig {
	cfg := &config.ProxyConfig{
		Path:        path,
		LoadBalance: "round_robin",
		Timeout: config.ProxyTimeout{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
		},
	}

	if len(targetURLs) > 0 {
		cfg.Targets = make([]config.ProxyTarget, len(targetURLs))
		for i, url := range targetURLs {
			cfg.Targets[i] = config.ProxyTarget{URL: url}
		}
	}

	return cfg
}

// NewTestProxyConfigWithCache 创建带缓存的测试代理配置
func NewTestProxyConfigWithCache(path string, maxAge time.Duration, targetURLs ...string) *config.ProxyConfig {
	cfg := NewTestProxyConfig(path, targetURLs...)
	cfg.Cache = config.ProxyCacheConfig{
		Enabled: true,
		MaxAge:  maxAge,
	}
	return cfg
}

// NewTestTarget 创建测试用的代理目标
//
// 参数：
//   - url: 目标 URL
//
// 返回值：
//   - *loadbalance.Target: 测试目标
func NewTestTarget(url string) *loadbalance.Target {
	return &loadbalance.Target{URL: url}
}

// NewTestTargets 批量创建测试目标
func NewTestTargets(urls ...string) []*loadbalance.Target {
	targets := make([]*loadbalance.Target, len(urls))
	for i, url := range urls {
		targets[i] = NewTestTarget(url)
	}
	return targets
}

// NewTestHealthyTarget 创建已标记为健康的测试目标
//
// 参数：
//   - url: 目标 URL
//
// 返回值：
//   - *loadbalance.Target: 已标记为健康的测试目标
func NewTestHealthyTarget(url string) *loadbalance.Target {
	t := NewTestTarget(url)
	t.Healthy.Store(true)
	return t
}

// NewTestHealthyTargets 批量创建健康测试目标
func NewTestHealthyTargets(urls ...string) []*loadbalance.Target {
	targets := make([]*loadbalance.Target, len(urls))
	for i, url := range urls {
		targets[i] = NewTestHealthyTarget(url)
	}
	return targets
}
