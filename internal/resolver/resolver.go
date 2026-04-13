// Package resolver 提供 DNS 解析功能，支持缓存和后台刷新。
//
// 该包实现了带缓存的 DNS 解析器，用于动态解析后端服务域名。
// 支持 UDP DNS 查询、TTL 缓存、后台刷新等特性。
//
// 主要用途：
//
//	用于代理模块动态解析 upstream 域名，支持域名变更自动感知
//
// 注意事项：
//   - 解析器使用 sync.Map 实现并发安全的缓存
//   - 后台刷新协程需要调用 Start() 启动
//   - 停止使用时应调用 Stop() 释放资源
//
// 作者：xfy
package resolver

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"rua.plus/lolly/internal/config"
)

// Resolver DNS 解析器接口。
type Resolver interface {
	// LookupHost 解析主机名，返回 IP 地址列表
	LookupHost(ctx context.Context, host string) ([]string, error)

	// LookupHostWithCache 带缓存的解析，优先返回缓存结果
	LookupHostWithCache(ctx context.Context, host string) ([]string, error)

	// Refresh 刷新指定主机的缓存
	Refresh(host string) error

	// Start 启动后台刷新协程
	Start() error

	// Stop 停止解析器
	Stop() error

	// Stats 返回统计信息
	Stats() Stats
}

// Stats 解析器统计信息。
type Stats struct {
	CacheHits      int64         // 缓存命中次数
	CacheMisses    int64         // 缓存未命中次数
	CacheEntries   int           // 当前缓存条目数
	ResolveErrors  int64         // 解析错误次数
	AverageLatency time.Duration // 平均解析延迟
}

// DNSResolver 实现 Resolver 接口的 DNS 解析器。
type DNSResolver struct {
	config       *config.ResolverConfig
	stopCh       chan struct{}
	refreshHosts map[string]struct{}
	cache        sync.Map
	hits         atomic.Int64
	misses       atomic.Int64
	errors       atomic.Int64
	latencyNs    atomic.Int64
	count        atomic.Int64
	mu           sync.RWMutex
	serverIdx    atomic.Uint32
	started      atomic.Bool
}

// DNSCacheEntry DNS 缓存条目。
type DNSCacheEntry struct {
	ExpiresAt  time.Time
	LastLookup time.Time
	Error      error
	IPs        []string
	mu         sync.RWMutex
}

// New 创建新的 DNS 解析器。
func New(cfg *config.ResolverConfig) Resolver {
	if !cfg.Enabled {
		return &noopResolver{}
	}

	// 设置默认值
	valid := cfg.Valid
	if valid == 0 {
		valid = 30 * time.Second
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	// 创建新配置副本，应用默认值
	configCopy := *cfg
	configCopy.Valid = valid
	configCopy.Timeout = timeout
	if !configCopy.IPv4 && !configCopy.IPv6 {
		configCopy.IPv4 = true
	}

	return &DNSResolver{
		config:       &configCopy,
		stopCh:       make(chan struct{}),
		refreshHosts: make(map[string]struct{}),
	}
}

// LookupHost 解析主机名，返回 IP 地址列表。
func (r *DNSResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	return r.lookup(ctx, host, false)
}

// LookupHostWithCache 带缓存的解析，优先返回缓存结果。
func (r *DNSResolver) LookupHostWithCache(ctx context.Context, host string) ([]string, error) {
	return r.lookup(ctx, host, true)
}

// lookup 内部解析方法。
func (r *DNSResolver) lookup(ctx context.Context, host string, useCache bool) ([]string, error) {
	// 如果 host 已经是 IP 地址，直接返回
	if ip := net.ParseIP(host); ip != nil {
		return []string{host}, nil
	}

	// 尝试从缓存获取
	if useCache {
		if entry, ok := r.cache.Load(host); ok {
			cacheEntry := entry.(*DNSCacheEntry) //nolint:errcheck // 类型断言
			cacheEntry.mu.RLock()
			ips := cacheEntry.IPs
			expiresAt := cacheEntry.ExpiresAt
			cacheErr := cacheEntry.Error
			cacheEntry.mu.RUnlock()

			// 缓存未过期，返回缓存结果
			if time.Now().Before(expiresAt) {
				r.hits.Add(1)
				if cacheErr != nil {
					return nil, cacheErr
				}
				return ips, nil
			}
		}
	}

	r.misses.Add(1)

	// 执行 DNS 查询
	start := time.Now()
	ips, err := r.queryDNS(ctx, host)
	latency := time.Since(start)

	r.latencyNs.Add(latency.Nanoseconds())
	r.count.Add(1)

	if err != nil {
		r.errors.Add(1)
	}

	// 更新缓存
	entry := &DNSCacheEntry{
		IPs:        ips,
		ExpiresAt:  time.Now().Add(r.config.TTL()),
		LastLookup: time.Now(),
		Error:      err,
	}
	r.cache.Store(host, entry)

	// 添加到刷新列表
	r.mu.Lock()
	r.refreshHosts[host] = struct{}{}
	r.mu.Unlock()

	if err != nil {
		return nil, err
	}
	return ips, nil
}

// queryDNS 执行实际的 DNS 查询。
func (r *DNSResolver) queryDNS(ctx context.Context, host string) ([]string, error) {
	if len(r.config.Addresses) == 0 {
		// 使用系统默认 DNS
		return r.queryWithResolver(ctx, host, "")
	}

	// 轮询选择 DNS 服务器
	idx := r.serverIdx.Add(1) % uint32(len(r.config.Addresses))
	dnsServer := r.config.Addresses[idx]

	return r.queryWithResolver(ctx, host, dnsServer)
}

// queryWithResolver 使用指定的 DNS 服务器查询。
func (r *DNSResolver) queryWithResolver(ctx context.Context, host, server string) ([]string, error) {
	var ips []string

	// 创建带超时的 context
	if r.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.config.Timeout)
		defer cancel()
	}

	// 创建自定义 resolver
	var resolver *net.Resolver
	if server != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, "udp", server)
			},
		}
	}

	// 查询 IPv4
	if r.config.IPv4 {
		var ipAddrs []net.IPAddr
		var err error
		if resolver != nil {
			ipAddrs, err = resolver.LookupIPAddr(ctx, host)
		} else {
			ipList, lookupErr := net.LookupIP(host)
			if lookupErr != nil {
				err = lookupErr
			} else {
				ipAddrs = make([]net.IPAddr, len(ipList))
				for i, ip := range ipList {
					ipAddrs[i] = net.IPAddr{IP: ip}
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("DNS lookup failed for %s: %w", host, err)
		}

		for _, addr := range ipAddrs {
			if ip4 := addr.IP.To4(); ip4 != nil {
				ips = append(ips, ip4.String())
			}
		}
	}

	// 查询 IPv6
	if r.config.IPv6 {
		var ipAddrs []net.IPAddr
		var err error
		if resolver != nil {
			ipAddrs, err = resolver.LookupIPAddr(ctx, host)
		} else {
			ipList, lookupErr := net.LookupIP(host)
			if lookupErr != nil {
				err = lookupErr
			} else {
				ipAddrs = make([]net.IPAddr, len(ipList))
				for i, ip := range ipList {
					ipAddrs[i] = net.IPAddr{IP: ip}
				}
			}
		}
		if err != nil {
			// IPv6 查询失败不返回错误，继续使用 IPv4 结果
			_ = err
		} else {
			for _, addr := range ipAddrs {
				if ip := addr.IP.To16(); ip != nil && ip.To4() == nil {
					ips = append(ips, ip.String())
				}
			}
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses found for %s", host)
	}

	return ips, nil
}

// Refresh 刷新指定主机的缓存。
func (r *DNSResolver) Refresh(host string) error {
	_, err := r.LookupHost(context.Background(), host)
	return err
}

// Start 启动后台刷新协程。
func (r *DNSResolver) Start() error {
	if !r.config.Enabled {
		return nil
	}

	if r.started.Load() {
		return nil
	}

	r.started.Store(true)

	// 启动后台刷新协程
	go r.refreshLoop()

	return nil
}

// refreshLoop 后台刷新循环。
func (r *DNSResolver) refreshLoop() {
	// 刷新间隔为 TTL / 2
	interval := r.config.TTL() / 2
	if interval < time.Second {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.doRefresh()
		case <-r.stopCh:
			return
		}
	}
}

// doRefresh 执行刷新操作。
func (r *DNSResolver) doRefresh() {
	r.mu.RLock()
	hosts := make([]string, 0, len(r.refreshHosts))
	for host := range r.refreshHosts {
		hosts = append(hosts, host)
	}
	r.mu.RUnlock()

	for _, host := range hosts {
		ctx, cancel := context.WithTimeout(context.Background(), r.config.Timeout)
		_, _ = r.LookupHost(ctx, host) //nolint:errcheck // 刷新缓存
		cancel()
	}
}

// Stop 停止解析器。
func (r *DNSResolver) Stop() error {
	if !r.started.Load() {
		return nil
	}

	close(r.stopCh)
	r.started.Store(false)
	return nil
}

// Stats 返回统计信息。
func (r *DNSResolver) Stats() Stats {
	hits := r.hits.Load()
	misses := r.misses.Load()

	// 统计缓存条目数
	var entries int
	r.cache.Range(func(_, _ interface{}) bool {
		entries++
		return true
	})

	// 计算平均延迟
	var avgLatency time.Duration
	count := r.count.Load()
	if count > 0 {
		avgLatency = time.Duration(r.latencyNs.Load() / count)
	}

	return Stats{
		CacheHits:      hits,
		CacheMisses:    misses,
		CacheEntries:   entries,
		ResolveErrors:  r.errors.Load(),
		AverageLatency: avgLatency,
	}
}

// noopResolver 是禁用状态下的空实现。
type noopResolver struct{}

func (n *noopResolver) LookupHost(_ context.Context, _ string) ([]string, error) {
	return nil, fmt.Errorf("resolver is disabled")
}

func (n *noopResolver) LookupHostWithCache(ctx context.Context, host string) ([]string, error) {
	return n.LookupHost(ctx, host)
}

func (n *noopResolver) Refresh(_ string) error {
	return nil
}

func (n *noopResolver) Start() error {
	return nil
}

func (n *noopResolver) Stop() error {
	return nil
}

func (n *noopResolver) Stats() Stats {
	return Stats{}
}
