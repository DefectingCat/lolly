// Package proxy 反向代理包，为 Lolly HTTP 服务器提供反向代理功能。
//
// 该文件实现 DNS 动态解析和刷新功能，支持后端目标的域名自动解析、
// IP 缓存、定时刷新和故障恢复。
//
// 主要功能：
//   - DNS 解析器集成：支持自定义 resolver 实现域名解析
//   - 定时刷新循环：根据 TTL 自动刷新已解析目标的 IP 地址
//   - 连接池同步更新：DNS 解析结果自动同步到 HostClient 连接池
//   - 统计信息查询：暴露 DNS 解析器的运行统计数据
//
// 注意事项：
//   - 所有公开方法均为并发安全
//   - DNS 刷新在后台 goroutine 中运行，通过 stopCh 控制生命周期
//
// 作者：xfy
package proxy

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"rua.plus/lolly/internal/loadbalance"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/resolver"
)

// SetResolver 设置 DNS 解析器。
//
// 该方法为代理实例配置自定义的 DNS 解析器，用于动态解析后端目标的域名。
// 必须在调用 Start() 之前设置，否则 DNS 刷新循环不会启动。
//
// 参数：
//   - r: DNS 解析器实例，需实现 resolver.Resolver 接口
func (p *Proxy) SetResolver(r resolver.Resolver) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resolver = r
}

// Start 启动代理服务，包括 DNS 刷新循环。
//
// 该方法标记代理为已启动状态，如果配置了 resolver，则启动解析器并
// 在后台 goroutine 中启动 DNS 定时刷新循环。该方法是幂等的，
// 重复调用不会重复启动。
//
// 返回值：
//   - error: 启动 resolver 失败时返回非 nil 错误
func (p *Proxy) Start() error {
	if p.started.Load() {
		return nil
	}

	p.started.Store(true)

	// 启动 DNS 刷新循环（如果配置了 resolver）
	if p.resolver != nil {
		if err := p.resolver.Start(); err != nil {
			return fmt.Errorf("failed to start resolver: %w", err)
		}
		go p.startDNSRefreshLoop()
	}

	return nil
}

// Stop 停止代理服务，包括关闭 DNS 刷新循环。
//
// 该方法标记代理为已停止状态，关闭 stopCh 通知所有后台协程退出，
// 并停止 resolver。该方法是幂等的，重复调用不会产生副作用。
//
// 返回值：
//   - error: 停止 resolver 失败时返回非 nil 错误
func (p *Proxy) Stop() error {
	if !p.started.Load() {
		return nil
	}

	p.started.Store(false)

	// 关闭 stopCh 通知所有后台协程退出
	close(p.stopCh)

	// 停止 resolver
	if p.resolver != nil {
		if err := p.resolver.Stop(); err != nil {
			return fmt.Errorf("failed to stop resolver: %w", err)
		}
	}

	return nil
}

// startDNSRefreshLoop 启动 DNS 刷新后台循环。
//
// 根据 resolver 的 TTL 计算刷新间隔（TTL / 2，最小 1 秒），
// 定时调用 refreshDNS 刷新所有需要解析的目标。
// 该方法阻塞运行，直到收到 stopCh 信号。
func (p *Proxy) startDNSRefreshLoop() {
	if p.resolver == nil {
		return
	}

	ttl := p.getResolverTTL()
	if ttl == 0 {
		ttl = 30 * time.Second
	}

	// 刷新间隔为 TTL / 2
	interval := ttl / 2
	if interval < time.Second {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.refreshDNS()
		case <-p.stopCh:
			return
		}
	}
}

// refreshDNS 刷新所有需要解析的目标。
//
// 遍历所有后端目标，对超过 TTL 的域名执行 DNS 查询，
// 更新目标的已解析 IP 列表，并同步更新对应 HostClient 的地址。
func (p *Proxy) refreshDNS() {
	if p.resolver == nil {
		return
	}

	ttl := p.getResolverTTL()

	p.mu.RLock()
	targets := p.targets
	p.mu.RUnlock()

	for _, target := range targets {
		if !target.NeedsResolve(ttl) {
			continue
		}

		hostname := target.Hostname()
		if hostname == "" {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ips, err := p.resolver.LookupHostWithCache(ctx, hostname)
		cancel()

		if err != nil {
			logging.Debug().Msgf("DNS refresh failed for %s: %v", hostname, err)
			continue
		}

		if len(ips) > 0 {
			target.SetResolvedIPs(ips)
			p.updateHostClientAddr(target, ips[0])
		}
	}
}

// updateHostClientAddr 更新 HostClient 的连接地址。
//
// 从目标 URL 中解析出端口，与新的 IP 地址组合后更新对应
// HostClient 的 Addr 字段。旧连接不受影响，新连接将使用新地址。
//
// 参数：
//   - target: 负载均衡目标，包含原始 URL
//   - ip: 新解析出的 IP 地址
func (p *Proxy) updateHostClientAddr(target *loadbalance.Target, ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 从 URL 解析出端口
	u, err := url.Parse(target.URL)
	if err != nil {
		return
	}

	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		// 没有端口，使用默认端口
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	newAddr := net.JoinHostPort(ip, port)

	// 更新 HostClient 的 Addr
	// 注意：新连接将使用新 IP，旧连接继续使用旧 IP 直到超时
	if client, ok := p.clients[target.URL]; ok {
		client.Addr = newAddr
		logging.Debug().Msgf("Updated HostClient addr for %s to %s", target.URL, newAddr)
	}
}

// getResolverTTL 获取 DNS 解析记录的过期时间。
//
// 返回 resolver 的 TTL 配置，默认值为 30 秒。
// 该方法当前返回固定值，未来可从 resolver stats 中动态推断。
//
// 返回值：
//   - time.Duration: DNS 记录的有效期
func (p *Proxy) getResolverTTL() time.Duration {
	if p.resolver == nil {
		return 0
	}

	// 从 stats 中推断 TTL（如果实现了相应接口）
	// 这里返回默认值
	return 30 * time.Second
}

// GetResolverStats 返回 DNS 解析器的统计信息。
//
// 获取当前 resolver 的运行统计数据，包括查询次数、
// 缓存命中率、错误次数等。如果未配置 resolver，返回空的 Stats。
//
// 返回值：
//   - resolver.Stats: DNS 解析器统计数据
func (p *Proxy) GetResolverStats() resolver.Stats {
	p.mu.RLock()
	r := p.resolver
	p.mu.RUnlock()

	if r == nil {
		return resolver.Stats{}
	}
	return r.Stats()
}
