// Package security 提供安全相关的 HTTP 中间件测试。
//
// 该文件包含 GeoIP 查询功能的单元测试。
//
// 作者：xfy
package security

import (
	"net"
	"os"
	"path/filepath"
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

// setupTestGeoIP 创建测试用 GeoIPLookup 实例。
// 它会复制测试数据库到临时目录，避免并发测试冲突。
func setupTestGeoIP(t *testing.T, behavior string) *GeoIPLookup {
	t.Helper()

	testDB := "/tmp/GeoIP2-Country-Test.mmdb"
	if _, err := os.Stat(testDB); os.IsNotExist(err) {
		t.Skipf("Skipping test: GeoIP test database not available: %v", err)
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "GeoIP2-Country-Test.mmdb")

	data, err := os.ReadFile(testDB)
	require.NoError(t, err, "failed to read test database")
	err = os.WriteFile(dbPath, data, 0o644)
	require.NoError(t, err, "failed to write test database to temp dir")

	geoip, err := NewGeoIPLookup(dbPath, 1000, time.Hour, behavior)
	require.NoError(t, err, "failed to create GeoIPLookup")
	t.Cleanup(func() { geoip.Close() })

	return geoip
}

// TestNewGeoIPLookup_ValidDB 测试使用有效数据库创建 GeoIPLookup。
func TestNewGeoIPLookup_ValidDB(t *testing.T) {
	geoip := setupTestGeoIP(t, "allow")
	assert.NotNil(t, geoip.db)
	assert.NotNil(t, geoip.cache)
	assert.Equal(t, time.Hour, geoip.ttl)
	assert.Equal(t, "allow", geoip.privateIPBehavior)
}

// TestNewGeoIPLookup_DefaultPrivateIPBehavior 测试默认私有 IP 行为（空字符串）。
func TestNewGeoIPLookup_DefaultPrivateIPBehavior(t *testing.T) {
	geoip := setupTestGeoIP(t, "")
	assert.Equal(t, "allow", geoip.privateIPBehavior)
}

// TestGeoIPLookup_LookupCountry 测试 IP 国家查询（已知国家代码）。
func TestGeoIPLookup_LookupCountry(t *testing.T) {
	geoip := setupTestGeoIP(t, "allow")

	// 2.125.160.216 在测试数据库中映射到 GB
	country, err := geoip.LookupCountry(net.ParseIP("2.125.160.216"))
	require.NoError(t, err)
	assert.Equal(t, "GB", country)

	// 67.43.156.1 在测试数据库中映射到 BT
	country2, err := geoip.LookupCountry(net.ParseIP("67.43.156.1"))
	require.NoError(t, err)
	assert.Equal(t, "BT", country2)
}

// TestGeoIPLookup_LookupCountry_Unknown 测试未找到国家代码时返回 UNKNOWN。
func TestGeoIPLookup_LookupCountry_Unknown(t *testing.T) {
	geoip := setupTestGeoIP(t, "allow")

	// 8.8.8.8 在测试数据库中没有记录
	country, err := geoip.LookupCountry(net.ParseIP("8.8.8.8"))
	require.NoError(t, err)
	assert.Equal(t, "UNKNOWN", country)
}

// TestGeoIPLookup_PrivateIPAllow 测试私有 IP allow 策略。
func TestGeoIPLookup_PrivateIPAllow(t *testing.T) {
	geoip := setupTestGeoIP(t, "allow")

	tests := []struct {
		name string
		ip   string
	}{
		{"10.0.0.1", "10.0.0.1"},
		{"192.168.1.1", "192.168.1.1"},
		{"127.0.0.1", "127.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			country, err := geoip.LookupCountry(net.ParseIP(tt.ip))
			require.NoError(t, err)
			assert.Equal(t, "PRIVATE_ALLOW", country)
		})
	}
}

// TestGeoIPLookup_PrivateIPDeny 测试私有 IP deny 策略。
func TestGeoIPLookup_PrivateIPDeny(t *testing.T) {
	geoip := setupTestGeoIP(t, "deny")

	tests := []struct {
		name string
		ip   string
	}{
		{"10.0.0.1", "10.0.0.1"},
		{"172.16.0.1", "172.16.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			country, err := geoip.LookupCountry(net.ParseIP(tt.ip))
			require.NoError(t, err)
			assert.Equal(t, "PRIVATE_DENY", country)
		})
	}
}

// TestGeoIPLookup_PrivateIPBypass 测试私有 IP bypass 策略。
func TestGeoIPLookup_PrivateIPBypass(t *testing.T) {
	geoip := setupTestGeoIP(t, "bypass")

	country, err := geoip.LookupCountry(net.ParseIP("172.16.0.1"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private IP bypassed")
	assert.Empty(t, country)
}

// TestGeoIPLookup_CacheBehavior 测试缓存行为。
func TestGeoIPLookup_CacheBehavior(t *testing.T) {
	geoip := setupTestGeoIP(t, "allow")

	// 第一次查询（数据库）
	country1, err := geoip.LookupCountry(net.ParseIP("2.125.160.216"))
	require.NoError(t, err)

	// 第二次查询（缓存）
	country2, err := geoip.LookupCountry(net.ParseIP("2.125.160.216"))
	require.NoError(t, err)
	assert.Equal(t, country1, country2)

	// 验证缓存大小 > 0
	stats := geoip.GetStats()
	assert.GreaterOrEqual(t, stats.CacheSize, 1)
}

// TestGeoIPLookup_MultipleIPsCaching 测试多个不同 IP 的缓存。
func TestGeoIPLookup_MultipleIPsCaching(t *testing.T) {
	geoip := setupTestGeoIP(t, "allow")

	// 查询多个不同的 IP
	ip1 := net.ParseIP("2.125.160.216")
	ip2 := net.ParseIP("67.43.156.0")

	_, err := geoip.LookupCountry(ip1)
	require.NoError(t, err)

	_, err = geoip.LookupCountry(ip2)
	require.NoError(t, err)

	// 缓存中应该有 2 个条目
	stats := geoip.GetStats()
	assert.GreaterOrEqual(t, stats.CacheSize, 2)
}

// TestGeoIPLookup_GetStats 测试统计信息获取。
func TestGeoIPLookup_GetStats(t *testing.T) {
	geoip := setupTestGeoIP(t, "allow")

	stats := geoip.GetStats()
	assert.GreaterOrEqual(t, stats.CacheSize, 0)
	assert.GreaterOrEqual(t, stats.CacheMaxSize, 0)

	// 查询后缓存大小应该增加
	_, err := geoip.LookupCountry(net.ParseIP("2.125.160.216"))
	require.NoError(t, err)

	stats = geoip.GetStats()
	assert.GreaterOrEqual(t, stats.CacheSize, 1)
}

// TestGeoIPLookup_Close 测试关闭数据库连接。
func TestGeoIPLookup_Close(t *testing.T) {
	testDB := "/tmp/GeoIP2-Country-Test.mmdb"
	if _, err := os.Stat(testDB); os.IsNotExist(err) {
		t.Skipf("Skipping test: GeoIP test database not available: %v", err)
	}

	geoip, err := NewGeoIPLookup(testDB, 100, time.Minute, "allow")
	require.NoError(t, err)

	err = geoip.Close()
	assert.NoError(t, err)

	// 关闭后再次查询应该报错
	_, err = geoip.LookupCountry(net.ParseIP("2.125.160.216"))
	assert.Error(t, err)
}

// TestGeoIPLookup_TTLExpiration 测试缓存 TTL 过期。
func TestGeoIPLookup_TTLExpiration(t *testing.T) {
	geoip, err := NewGeoIPLookup("/tmp/GeoIP2-Country-Test.mmdb", 1000, 1*time.Millisecond, "allow")
	require.NoError(t, err)
	defer geoip.Close()

	publicIP := net.ParseIP("2.125.160.216")

	// 第一次查询
	_, err = geoip.LookupCountry(publicIP)
	require.NoError(t, err)

	// 等待 TTL 过期
	time.Sleep(10 * time.Millisecond)

	// 再次查询（缓存应该已过期，重新查询数据库）
	country, err := geoip.LookupCountry(publicIP)
	assert.NoError(t, err)
	assert.Equal(t, "GB", country)
}

// TestGeoIPLookup_InvalidIP 测试无效 IP 地址查询。
func TestGeoIPLookup_InvalidIP(t *testing.T) {
	geoip := setupTestGeoIP(t, "allow")

	// 传递 nil IP
	_, err := geoip.LookupCountry(nil)
	assert.Error(t, err)
}

// TestGeoIPLookup_SmallCacheSize 测试小缓存容量 LRU 淘汰。
func TestGeoIPLookup_SmallCacheSize(t *testing.T) {
	// 使用很小的缓存容量（2），测试 LRU 淘汰
	testDB := "/tmp/GeoIP2-Country-Test.mmdb"
	if _, err := os.Stat(testDB); os.IsNotExist(err) {
		t.Skipf("Skipping test: GeoIP test database not available: %v", err)
	}

	geoip, err := NewGeoIPLookup(testDB, 2, time.Hour, "allow")
	require.NoError(t, err)
	defer geoip.Close()

	// 查询 3 个不同的 IP，超过缓存容量
	ips := []string{"2.125.160.216", "67.43.156.0", "67.43.156.1"}
	for _, ipStr := range ips {
		_, err := geoip.LookupCountry(net.ParseIP(ipStr))
		assert.NoError(t, err)
	}

	// 缓存大小不会超过设定的限制
	stats := geoip.GetStats()
	assert.LessOrEqual(t, stats.CacheSize, 2)
}

// TestGeoIPLookup_PrivateIPNotCached 测试私有 IP 不会被缓存。
func TestGeoIPLookup_PrivateIPNotCached(t *testing.T) {
	geoip := setupTestGeoIP(t, "allow")

	// 查询私有 IP（不会进入数据库查询缓存）
	_, err := geoip.LookupCountry(net.ParseIP("10.0.0.1"))
	require.NoError(t, err)

	// 查询一个公网 IP
	_, err = geoip.LookupCountry(net.ParseIP("2.125.160.216"))
	require.NoError(t, err)

	// 缓存中只有 1 个条目（公网 IP）
	stats := geoip.GetStats()
	assert.Equal(t, 1, stats.CacheSize)
}

// TestGeoIPLookup_ConcurrentAccess 测试并发访问安全性。
func TestGeoIPLookup_ConcurrentAccess(t *testing.T) {
	geoip := setupTestGeoIP(t, "allow")

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := geoip.LookupCountry(net.ParseIP("2.125.160.216"))
			assert.NoError(t, err)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
