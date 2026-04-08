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
func (p *Proxy) SetResolver(r resolver.Resolver) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resolver = r
}

// Start 启动代理，包括 DNS 刷新循环。
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

// Stop 停止代理，包括关闭 DNS 刷新循环。
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

// updateHostClientAddr 更新 HostClient 的 Addr。
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

// getResolverTTL 获取 resolver 的 TTL。
func (p *Proxy) getResolverTTL() time.Duration {
	if p.resolver == nil {
		return 0
	}

	// 从 stats 中推断 TTL（如果实现了相应接口）
	// 这里返回默认值
	return 30 * time.Second
}

// GetResolverStats 返回 DNS 解析器的统计信息。
func (p *Proxy) GetResolverStats() resolver.ResolverStats {
	p.mu.RLock()
	r := p.resolver
	p.mu.RUnlock()

	if r == nil {
		return resolver.ResolverStats{}
	}
	return r.Stats()
}
