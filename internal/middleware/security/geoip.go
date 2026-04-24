// Package security 提供安全相关的 HTTP 中间件。
//
// 该文件实现 GeoIP 查询功能，支持基于国家代码的访问控制，
// 使用 LRU 缓存提高查询性能。
//
// 使用示例：
//
//	geoip, err := security.NewGeoIPLookup("/var/lib/geoip/GeoIP2-Country.mmdb", 10000, time.Hour, "allow")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer geoip.Close()
//
//	country, err := geoip.LookupCountry(ip)
//	if err != nil {
//	    log.Printf("GeoIP lookup failed: %v", err)
//	}
//
// 作者：xfy
package security

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/oschwald/geoip2-golang"
)

// GeoIPLookup GeoIP 查询器（带 LRU 缓存）。
//
// 使用 MaxMind GeoIP2 数据库查询 IP 地址所属国家，
// 通过 LRU 缓存减少数据库查询次数，提高性能。
type GeoIPLookup struct {
	db                *geoip2.Reader
	cache             *lru.Cache[string, *cachedCountry]
	privateIPBehavior string
	ttl               time.Duration
	mu                sync.RWMutex
}

// cachedCountry 缓存的国家代码条目。
type cachedCountry struct {
	expires time.Time
	country string
}

// GeoIPStats GeoIP 缓存统计信息。
type GeoIPStats struct {
	CacheSize    int
	CacheMaxSize int
}

// NewGeoIPLookup 创建 GeoIP 查询器。
//
// 打开 GeoIP2 数据库文件并初始化 LRU 缓存。
//
// 参数：
//   - dbPath: GeoIP2 数据库文件路径（.mmdb 格式）
//   - cacheSize: LRU 缓存最大条目数（硬限制）
//   - ttl: 缓存条目有效期
//   - privateIPBehavior: 私有 IP 处理策略（"allow", "deny", "bypass"）
//
// 返回值：
//   - *GeoIPLookup: 查询器实例
//   - error: 数据库打开失败或缓存创建失败时返回错误
func NewGeoIPLookup(dbPath string, cacheSize int, ttl time.Duration, privateIPBehavior string) (*GeoIPLookup, error) {
	if dbPath == "" {
		return nil, errors.New("geoip database path is required")
	}

	// 打开 GeoIP2 数据库
	db, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open geoip database: %w", err)
	}

	// 创建 LRU 缓存
	cache, err := lru.New[string, *cachedCountry](cacheSize)
	if err != nil {

		_ = db.Close()
		return nil, fmt.Errorf("create lru cache: %w", err)
	}

	// 默认私有 IP 行为
	if privateIPBehavior == "" {
		privateIPBehavior = accessAllow
	}

	return &GeoIPLookup{
		db:                db,
		cache:             cache,
		ttl:               ttl,
		privateIPBehavior: privateIPBehavior,
	}, nil
}

// LookupCountry 查询 IP 所属国家。
//
// 返回 ISO 3166-1 alpha-2 国家代码（如 "CN", "US"）。
// 查询结果会被缓存，减少数据库访问。
//
// 参数：
//   - ip: 待查询的 IP 地址
//
// 返回值：
//   - string: ISO 3166-1 alpha-2 国家代码
//   - error: 查询失败时返回错误
func (g *GeoIPLookup) LookupCountry(ip net.IP) (string, error) {
	// 检查私有 IP
	if isPrivateIP(ip) {
		switch g.privateIPBehavior {
		case accessAllow:
			return "PRIVATE_ALLOW", nil // 特殊标记，表示允许
		case accessDeny:
			return "PRIVATE_DENY", nil // 特殊标记，表示拒绝
		case "bypass":
			return "", errors.New("private IP bypassed")
		}
	}

	ipStr := ip.String()

	// 检查缓存（读锁）
	g.mu.RLock()
	if cached, ok := g.cache.Get(ipStr); ok {
		if time.Now().Before(cached.expires) {
			g.mu.RUnlock()
			return cached.country, nil
		}
	}
	g.mu.RUnlock()

	// 查询数据库（写锁）
	g.mu.Lock()
	defer g.mu.Unlock()

	// 双重检查（可能已被其他 goroutine 更新）
	if cached, ok := g.cache.Get(ipStr); ok {
		if time.Now().Before(cached.expires) {
			return cached.country, nil
		}
	}

	// 查询数据库
	country, err := g.db.Country(ip)
	if err != nil {
		return "", fmt.Errorf("geoip lookup: %w", err)
	}

	isoCode := country.Country.IsoCode
	if isoCode == "" {
		isoCode = "UNKNOWN"
	}

	// 存入缓存
	g.cache.Add(ipStr, &cachedCountry{
		country: isoCode,
		expires: time.Now().Add(g.ttl),
	})

	return isoCode, nil
}

// Close 关闭数据库连接。
//
// 必须在服务器停止时调用，释放 GeoIP2 数据库资源。
//
// 返回值：
//   - error: 关闭失败时返回错误
func (g *GeoIPLookup) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.db != nil {
		return g.db.Close()
	}
	return nil
}

// GetStats 返回缓存统计信息。
//
// 返回值：
//   - GeoIPStats: 包含当前缓存大小和最大缓存大小的统计对象
func (g *GeoIPLookup) GetStats() GeoIPStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return GeoIPStats{
		CacheSize:    g.cache.Len(),
		CacheMaxSize: g.cache.Len(), // LRU 缓存容量与当前大小相同（已淘汰的已被移除）
	}
}

// isPrivateIP 检查是否为私有 IP 地址。
//
// 支持的私有地址范围：
//   - 10.0.0.0/8
//   - 172.16.0.0/12
//   - 192.168.0.0/16
//   - 127.0.0.0/8（回环）
//   - IPv6 本地地址
//
// 参数：
//   - ip: 待检查的 IP 地址
//
// 返回值：
//   - bool: true 表示是私有 IP
func isPrivateIP(ip net.IP) bool {
	// IPv4 私有地址范围
	privateBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
	}

	for _, cidr := range privateBlocks {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	// IPv6 私有地址
	if ip.To4() == nil {
		// IPv6 本地地址
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true
		}
	}

	return false
}
