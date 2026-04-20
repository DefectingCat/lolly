// Package security 提供安全相关的 HTTP 中间件。
//
// 该文件实现 IP 访问控制中间件，支持基于 CIDR 的允许/拒绝列表，
// 兼容 IPv4 和 IPv6。
//
// 使用示例：
//
//	cfg := &config.AccessConfig{
//	    Allow: []string{"192.168.1.0/24", "10.0.0.0/8"},
//	    Deny:  []string{"192.168.2.100/32"},
//	    Default: "deny",
//	}
//
//	access, err := security.NewAccessControl(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 应用为中间件
//	chain := middleware.NewChain(access)
//	handler := chain.Apply(finalHandler)
//
// 作者：xfy
package security

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/middleware"
	"rua.plus/lolly/internal/netutil"
)

// Action 表示对 IP 的操作类型。
type Action int

const (
	// ActionAllow 允许请求通过
	ActionAllow Action = iota
	// ActionDeny 拒绝请求（返回 403 Forbidden）
	ActionDeny

	accessAllow     = "allow"
	accessDeny      = "deny"
	accessUnknown   = "unknown"
	geoPrivateAllow = "PRIVATE_ALLOW"
	geoPrivateDeny  = "PRIVATE_DENY"
)

// AccessControl 实现 IP 访问控制中间件。
//
// 根据配置的允许/拒绝 CIDR 列表和 GeoIP 国家代码检查入站请求。
// 支持动态更新访问控制列表和 GeoIP 配置。
type AccessControl struct {
	geoip          *GeoIPLookup
	allowList      []net.IPNet
	denyList       []net.IPNet
	trustedProxies []net.IPNet
	geoipConfig    config.GeoIPConfig
	defaultAction  Action
	mu             sync.RWMutex
}

// NewAccessControl 创建访问控制中间件。
//
// 根据配置创建访问控制中间件实例，解析 CIDR 列表并设置默认操作。
//
// 参数：
//   - cfg: 访问控制配置，包含允许/拒绝列表和默认操作
//
// 返回值：
//   - *AccessControl: 配置好的访问控制中间件
//   - error: CIDR 解析失败时返回错误
func NewAccessControl(cfg *config.AccessConfig) (*AccessControl, error) {
	if cfg == nil {
		return nil, errors.New("access config is nil")
	}

	ac := &AccessControl{}

	// 解析允许列表
	for _, cidr := range cfg.Allow {
		network, err := parseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid allow CIDR %s: %w", cidr, err)
		}
		ac.allowList = append(ac.allowList, *network)
	}

	// 解析拒绝列表
	for _, cidr := range cfg.Deny {
		network, err := parseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid deny CIDR %s: %w", cidr, err)
		}
		ac.denyList = append(ac.denyList, *network)
	}

	// 解析可信代理列表
	for _, cidr := range cfg.TrustedProxies {
		network, err := parseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted_proxy CIDR %s: %w", cidr, err)
		}
		ac.trustedProxies = append(ac.trustedProxies, *network)
	}

	// 设置默认操作
	switch strings.ToLower(cfg.Default) {
	case accessAllow, "":
		ac.defaultAction = ActionAllow
	case accessDeny:
		ac.defaultAction = ActionDeny
	default:
		return nil, fmt.Errorf("invalid default action: %s", cfg.Default)
	}

	// 初始化 GeoIP（如果启用）
	if cfg.GeoIP.Enabled && cfg.GeoIP.Database != "" {
		// 设置默认值
		cacheSize := cfg.GeoIP.CacheSize
		if cacheSize <= 0 {
			cacheSize = 10000 // 默认 10000 条
		}
		ttl := cfg.GeoIP.CacheTTL
		if ttl <= 0 {
			ttl = time.Hour // 默认 1 小时
		}

		geoip, err := NewGeoIPLookup(
			cfg.GeoIP.Database,
			cacheSize,
			ttl,
			cfg.GeoIP.PrivateIPBehavior,
		)
		if err != nil {
			return nil, fmt.Errorf("init geoip: %w", err)
		}
		ac.geoip = geoip
		ac.geoipConfig = cfg.GeoIP
	}

	return ac, nil
}

// Name 返回中间件名称。
//
// 返回值：
//   - string: 中间件标识名 "access_control"
func (ac *AccessControl) Name() string {
	return "access_control"
}

// Process 用访问控制逻辑包装下一个处理器。
//
// 被拒绝的 IP 请求将收到 403 Forbidden 响应。
//
// 参数：
//   - next: 下一个请求处理器
//
// 返回值：
//   - fasthttp.RequestHandler: 包装后的处理器
func (ac *AccessControl) Process(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		clientIP := ac.getClientIP(ctx)

		// 检查访问权限
		if !ac.Check(clientIP) {
			ctx.Error("Forbidden: Access denied", fasthttp.StatusForbidden)
			return
		}

		next(ctx)
	}
}

// Check 检查 IP 地址是否允许访问。
//
// 评估顺序：
// 1. 检查 CIDR 拒绝列表（显式拒绝优先）
// 2. 检查 GeoIP 国家拒绝（如果启用）
// 3. 检查 CIDR 允许列表
// 4. 检查 GeoIP 国家允许（如果启用）
// 5. 返回默认操作
//
// 参数：
//   - ip: 待检查的 IP 地址
//
// 返回值：
//   - bool: true 表示允许访问，false 表示拒绝
func (ac *AccessControl) Check(ip net.IP) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	// 1. 先检查 CIDR 拒绝列表（显式拒绝优先）
	for _, network := range ac.denyList {
		if network.Contains(ip) {
			return false
		}
	}

	// 2. 检查 GeoIP 国家拒绝（如果启用）
	if ac.geoip != nil && ac.geoipConfig.Enabled {
		country, err := ac.geoip.LookupCountry(ip)
		if err == nil {
			// 处理私有 IP 特殊标记
			if country == geoPrivateAllow {
				// 私有 IP 自动允许，跳过国家检查
				goto checkAllow
			}
			if country == geoPrivateDeny {
				return false
			}

			for _, c := range ac.geoipConfig.DenyCountries {
				if country == c {
					return false
				}
			}
		}
	}

checkAllow:
	// 3. 检查 CIDR 允许列表
	for _, network := range ac.allowList {
		if network.Contains(ip) {
			return true
		}
	}

	// 4. 检查 GeoIP 国家允许（如果启用）
	if ac.geoip != nil && ac.geoipConfig.Enabled {
		country, err := ac.geoip.LookupCountry(ip)
		if err == nil && country != geoPrivateDeny {
			for _, c := range ac.geoipConfig.AllowCountries {
				if country == c {
					return true
				}
			}
		}
	}

	// 5. 返回默认操作
	return ac.defaultAction == ActionAllow
}

// UpdateAllowList 动态更新允许列表。
//
// 替换当前的允许列表，使用写锁保护并发访问。
//
// 参数：
//   - cidrs: 新的 CIDR 字符串列表
//
// 返回值：
//   - error: CIDR 解析失败时返回错误
func (ac *AccessControl) UpdateAllowList(cidrs []string) error {
	newList, err := parseCIDRList(cidrs)
	if err != nil {
		return err
	}

	ac.mu.Lock()
	ac.allowList = newList
	ac.mu.Unlock()
	return nil
}

// UpdateDenyList 动态更新拒绝列表。
//
// 替换当前的拒绝列表，使用写锁保护并发访问。
//
// 参数：
//   - cidrs: 新的 CIDR 字符串列表
//
// 返回值：
//   - error: CIDR 解析失败时返回错误
func (ac *AccessControl) UpdateDenyList(cidrs []string) error {
	newList, err := parseCIDRList(cidrs)
	if err != nil {
		return err
	}

	ac.mu.Lock()
	ac.denyList = newList
	ac.mu.Unlock()
	return nil
}

// parseCIDRList 解析 CIDR 字符串列表为 IPNet 列表。
//
// 参数：
//   - cidrs: CIDR 字符串列表
//
// 返回值：
//   - []net.IPNet: 解析后的 IP 网络对象列表
//   - error: 任一 CIDR 解析失败时返回错误
func parseCIDRList(cidrs []string) ([]net.IPNet, error) {
	newList := make([]net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		network, err := parseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %s: %w", cidr, err)
		}
		newList = append(newList, *network)
	}
	return newList, nil
}

// SetDefault 设置默认操作。
//
// 参数：
//   - action: 操作类型，"allow" 或 "deny"
//
// 返回值：
//   - error: 无效的操作类型时返回错误
func (ac *AccessControl) SetDefault(action string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	switch strings.ToLower(action) {
	case accessAllow:
		ac.defaultAction = ActionAllow
	case accessDeny:
		ac.defaultAction = ActionDeny
	default:
		return fmt.Errorf("invalid action: %s", action)
	}

	return nil
}

// parseCIDR 解析 CIDR 字符串，支持 IPv4 和 IPv6。
//
// 支持完整的 CIDR 表示法（如 192.168.1.0/24）和单个 IP（如 192.168.1.1）。
// 单个 IP 会自动转换为 /32（IPv4）或 /128（IPv6）的 CIDR。
//
// 参数：
//   - cidr: CIDR 字符串或单个 IP 地址
//
// 返回值：
//   - *net.IPNet: 解析后的 IP 网络对象
//   - error: 解析失败时返回错误
func parseCIDR(cidr string) (*net.IPNet, error) {
	// 处理单个 IP（没有 /前缀）
	if !strings.Contains(cidr, "/") {
		ip := net.ParseIP(cidr)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address: %s", cidr)
		}

		// 转换为完整掩码的 CIDR
		if ip.To4() != nil {
			cidr = cidr + "/32"
		} else {
			cidr = cidr + "/128"
		}
	}

	// 解析 CIDR
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	// 确保 IP 为规范形式
	network.IP = ip

	return network, nil
}

// getClientIP 从请求上下文安全提取客户端 IP。
//
// 仅当请求来自可信代理时，才解析 X-Forwarded-For 头部。
// 使用右侧（最接近客户端）的非可信 IP 作为真实客户端 IP。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - net.IP: 客户端 IP 地址，无法获取时返回 nil
func (ac *AccessControl) getClientIP(ctx *fasthttp.RequestCtx) net.IP {
	remoteIP := netutil.GetRemoteAddrIP(ctx)

	// 仅当配置了可信代理且请求来自可信代理时，才解析 X-Forwarded-For
	if len(ac.trustedProxies) > 0 && remoteIP != nil {
		isTrusted := false
		for _, network := range ac.trustedProxies {
			if network.Contains(remoteIP) {
				isTrusted = true
				break
			}
		}

		if isTrusted {
			// 使用右侧（最接近客户端）的非可信 IP
			if xff := ctx.Request.Header.Peek("X-Forwarded-For"); len(xff) > 0 {
				ips := strings.Split(string(xff), ",")
				for i := len(ips) - 1; i >= 0; i-- {
					ipStr := strings.TrimSpace(ips[i])
					if ip := net.ParseIP(ipStr); ip != nil {
						// 检查该 IP 是否在可信代理列表中
						trusted := false
						for _, network := range ac.trustedProxies {
							if network.Contains(ip) {
								trusted = true
								break
							}
						}
						if !trusted {
							return ip
						}
					}
				}
			}
		}
	}

	// 检查 X-Real-IP 头部（仅来自可信代理时）
	if len(ac.trustedProxies) > 0 && remoteIP != nil {
		isTrusted := false
		for _, network := range ac.trustedProxies {
			if network.Contains(remoteIP) {
				isTrusted = true
				break
			}
		}

		if isTrusted {
			if xri := ctx.Request.Header.Peek("X-Real-IP"); len(xri) > 0 {
				if ip := net.ParseIP(string(xri)); ip != nil {
					return ip
				}
			}
		}
	}

	return remoteIP
}

// AccessStats 访问控制统计信息结构。
type AccessStats struct {
	Default    string
	AllowCount int
	DenyCount  int
}

// GetStats 返回当前访问控制的统计信息。
//
// 返回值：
//   - AccessStats: 包含规则数量和默认操作的统计对象
func (ac *AccessControl) GetStats() AccessStats {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	return AccessStats{
		AllowCount: len(ac.allowList),
		DenyCount:  len(ac.denyList),
		Default:    actionToString(ac.defaultAction),
	}
}

// actionToString 将 Action 转换为其字符串表示。
//
// 参数：
//   - action: 操作类型
//
// 返回值：
//   - string: 操作类型的字符串表示
func actionToString(action Action) string {
	switch action {
	case ActionAllow:
		return accessAllow
	case ActionDeny:
		return accessDeny
	default:
		return accessUnknown
	}
}

// Close 释放资源。
//
// 必须在服务器停止时调用，释放 GeoIP 数据库连接。
//
// 返回值：
//   - error: 关闭失败时返回错误
func (ac *AccessControl) Close() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.geoip != nil {
		return ac.geoip.Close()
	}
	return nil
}

// 验证接口实现
var _ middleware.Middleware = (*AccessControl)(nil)
