// Package security 提供安全相关的 HTTP 中间件测试。
//
// 该文件包含 GeoIP 查询功能的单元测试。
//
// 作者：xfy
package security

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsPrivateIP 测试私有 IP 检测功能。
func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"IPv4 私有地址 10.x.x.x", "10.0.0.1", true},
		{"IPv4 私有地址 172.16.x.x", "172.16.0.1", true},
		{"IPv4 私有地址 172.31.x.x", "172.31.255.1", true},
		{"IPv4 私有地址 192.168.x.x", "192.168.1.1", true},
		{"IPv4 回环地址", "127.0.0.1", true},
		{"IPv4 公网地址", "8.8.8.8", false},
		{"IPv4 公网地址", "1.1.1.1", false},
		{"IPv6 回环地址", "::1", true},
		{"IPv6 本地链路地址", "fe80::1", true},
		{"IPv6 公网地址", "2001:4860:4860::8888", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "failed to parse IP: %s", tt.ip)
			result := isPrivateIP(ip)
			assert.Equal(t, tt.expected, result, "isPrivateIP(%s)", tt.ip)
		})
	}
}

// TestNewGeoIPLookup_InvalidPath 测试无效数据库路径。
func TestNewGeoIPLookup_InvalidPath(t *testing.T) {
	_, err := NewGeoIPLookup("", 1000, time.Hour, "allow")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database path is required")
}

// TestNewGeoIPLookup_NonExistentDB 测试不存在的数据库文件。
func TestNewGeoIPLookup_NonExistentDB(t *testing.T) {
	_, err := NewGeoIPLookup("/nonexistent/path/to/geoip.mmdb", 1000, time.Hour, "allow")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "open geoip database")
}

// TestGeoIPLookup_PrivateIPBehavior 测试私有 IP 处理策略。
func TestGeoIPLookup_PrivateIPBehavior(t *testing.T) {
	// 注意：这个测试需要有效的 GeoIP2 数据库文件
	// 如果没有数据库文件，测试会被跳过
	dbPath := "/var/lib/geoip/GeoIP2-Country.mmdb"

	// 尝试创建 GeoIPLookup
	geoip, err := NewGeoIPLookup(dbPath, 1000, time.Hour, "allow")
	if err != nil {
		t.Skipf("Skipping test: GeoIP database not available: %v", err)
	}
	defer geoip.Close()

	privateIP := net.ParseIP("192.168.1.1")

	// 测试 allow 策略
	country, err := geoip.LookupCountry(privateIP)
	require.NoError(t, err)
	assert.Equal(t, "PRIVATE_ALLOW", country)
}

// TestGeoIPLookup_PrivateIPBehavior_Deny 测试私有 IP deny 策略。
func TestGeoIPLookup_PrivateIPBehavior_Deny(t *testing.T) {
	dbPath := "/var/lib/geoip/GeoIP2-Country.mmdb"

	geoip, err := NewGeoIPLookup(dbPath, 1000, time.Hour, "deny")
	if err != nil {
		t.Skipf("Skipping test: GeoIP database not available: %v", err)
	}
	defer geoip.Close()

	privateIP := net.ParseIP("10.0.0.1")

	country, err := geoip.LookupCountry(privateIP)
	require.NoError(t, err)
	assert.Equal(t, "PRIVATE_DENY", country)
}

// TestGeoIPLookup_PrivateIPBehavior_Bypass 测试私有 IP bypass 策略。
func TestGeoIPLookup_PrivateIPBehavior_Bypass(t *testing.T) {
	dbPath := "/var/lib/geoip/GeoIP2-Country.mmdb"

	geoip, err := NewGeoIPLookup(dbPath, 1000, time.Hour, "bypass")
	if err != nil {
		t.Skipf("Skipping test: GeoIP database not available: %v", err)
	}
	defer geoip.Close()

	privateIP := net.ParseIP("172.16.0.1")

	_, err = geoip.LookupCountry(privateIP)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private IP bypassed")
}

// TestGeoIPLookup_DefaultPrivateIPBehavior 测试默认私有 IP 行为。
func TestGeoIPLookup_DefaultPrivateIPBehavior(t *testing.T) {
	dbPath := "/var/lib/geoip/GeoIP2-Country.mmdb"

	// 空字符串应该使用默认的 "allow"
	geoip, err := NewGeoIPLookup(dbPath, 1000, time.Hour, "")
	if err != nil {
		t.Skipf("Skipping test: GeoIP database not available: %v", err)
	}
	defer geoip.Close()

	privateIP := net.ParseIP("127.0.0.1")

	country, err := geoip.LookupCountry(privateIP)
	require.NoError(t, err)
	assert.Equal(t, "PRIVATE_ALLOW", country)
}

// TestGeoIPLookup_GetStats 测试统计信息获取。
func TestGeoIPLookup_GetStats(t *testing.T) {
	dbPath := "/var/lib/geoip/GeoIP2-Country.mmdb"

	geoip, err := NewGeoIPLookup(dbPath, 1000, time.Hour, "allow")
	if err != nil {
		t.Skipf("Skipping test: GeoIP database not available: %v", err)
	}
	defer geoip.Close()

	stats := geoip.GetStats()
	assert.GreaterOrEqual(t, stats.CacheSize, 0)
	assert.GreaterOrEqual(t, stats.CacheMaxSize, 0)
}

// TestGeoIPLookup_CacheBehavior 测试缓存行为。
func TestGeoIPLookup_CacheBehavior(t *testing.T) {
	dbPath := "/var/lib/geoip/GeoIP2-Country.mmdb"

	geoip, err := NewGeoIPLookup(dbPath, 1000, time.Hour, "allow")
	if err != nil {
		t.Skipf("Skipping test: GeoIP database not available: %v", err)
	}
	defer geoip.Close()

	// 使用公网 IP 进行测试（假设 8.8.8.8 是美国）
	publicIP := net.ParseIP("8.8.8.8")

	// 第一次查询
	country1, err := geoip.LookupCountry(publicIP)
	if err != nil {
		// 数据库中可能没有该 IP 的信息
		t.Skipf("Skipping test: IP not found in database: %v", err)
	}

	// 第二次查询（应该从缓存返回）
	country2, err := geoip.LookupCountry(publicIP)
	require.NoError(t, err)
	assert.Equal(t, country1, country2)

	// 验证缓存大小
	stats := geoip.GetStats()
	assert.GreaterOrEqual(t, stats.CacheSize, 1)
}

// TestGeoIPLookup_TTLExpiration 测试缓存 TTL 过期。
func TestGeoIPLookup_TTLExpiration(t *testing.T) {
	dbPath := "/var/lib/geoip/GeoIP2-Country.mmdb"

	// 使用很短的 TTL
	geoip, err := NewGeoIPLookup(dbPath, 1000, 1*time.Millisecond, "allow")
	if err != nil {
		t.Skipf("Skipping test: GeoIP database not available: %v", err)
	}
	defer geoip.Close()

	publicIP := net.ParseIP("8.8.8.8")

	// 第一次查询
	_, err = geoip.LookupCountry(publicIP)
	if err != nil {
		t.Skipf("Skipping test: IP not found in database: %v", err)
	}

	// 等待 TTL 过期
	time.Sleep(10 * time.Millisecond)

	// 再次查询（缓存应该已过期）
	_, err = geoip.LookupCountry(publicIP)
	// 不应该报错，只是重新查询数据库
	assert.NoError(t, err)
}
