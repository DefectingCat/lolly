//go:build integration

// cache_integration_test.go - 缓存集成测试（L2 层，进程内）
//
// 测试代理缓存的核心功能。
//
// 作者：xfy
package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rua.plus/lolly/internal/cache"
)

// TestProxyCacheCreation 测试代理缓存创建
func TestProxyCacheCreation(t *testing.T) {
	rules := []cache.ProxyCacheRule{
		{
			Path:     "/api/",
			Methods:  []string{"GET", "HEAD"},
			Statuses: []int{200, 301, 302},
			MaxAge:   60 * time.Second,
		},
	}

	pc := cache.NewProxyCache(rules, true, 30*time.Second)
	require.NotNil(t, pc)
}

// TestProxyCacheDisabled 测试禁用缓存（空规则）
func TestProxyCacheDisabled(t *testing.T) {
	rules := []cache.ProxyCacheRule{}

	pc := cache.NewProxyCache(rules, false, 0)
	require.NotNil(t, pc)
}

// TestProxyCacheSetAndGet 测试缓存存取
func TestProxyCacheSetAndGet(t *testing.T) {
	rules := []cache.ProxyCacheRule{
		{
			Path:     "/",
			Methods:  []string{"GET"},
			Statuses: []int{200},
			MaxAge:   60 * time.Second,
		},
	}

	pc := cache.NewProxyCache(rules, true, 30*time.Second)
	require.NotNil(t, pc)

	origKey := "GET:/test"
	hashKey := cache.HashPathWithMethod("/test", "GET")
	body := []byte("test response body")
	headers := map[string]string{
		"Content-Type": "application/json",
		"X-Custom":     "value",
	}

	// 存入缓存
	pc.Set(hashKey, origKey, body, headers, 200, 60*time.Second)

	// 从缓存获取
	entry, found, stale := pc.Get(hashKey, origKey)
	assert.True(t, found)
	assert.False(t, stale)
	require.NotNil(t, entry)
	assert.Equal(t, 200, entry.Status)
	assert.Equal(t, body, entry.Data)
	assert.Equal(t, "application/json", entry.Headers["Content-Type"])
}

// TestProxyCacheMiss 测试缓存未命中
func TestProxyCacheMiss(t *testing.T) {
	rules := []cache.ProxyCacheRule{
		{
			Path:     "/",
			Methods:  []string{"GET"},
			Statuses: []int{200},
			MaxAge:   60 * time.Second,
		},
	}

	pc := cache.NewProxyCache(rules, true, 30*time.Second)
	require.NotNil(t, pc)

	origKey := "GET:/nonexistent"
	hashKey := cache.HashPathWithMethod("/nonexistent", "GET")

	// 不存在的键应返回未命中
	entry, found, stale := pc.Get(hashKey, origKey)
	assert.False(t, found)
	assert.False(t, stale)
	assert.Nil(t, entry)
}

// TestProxyCacheExpiration 测试缓存过期
func TestProxyCacheExpiration(t *testing.T) {
	rules := []cache.ProxyCacheRule{
		{
			Path:     "/",
			Methods:  []string{"GET"},
			Statuses: []int{200},
			MaxAge:   100 * time.Millisecond, // 短过期时间
		},
	}

	pc := cache.NewProxyCache(rules, true, 50*time.Millisecond)
	require.NotNil(t, pc)

	origKey := "GET:/expiring"
	hashKey := cache.HashPathWithMethod("/expiring", "GET")
	body := []byte("will expire")

	// 存入缓存
	pc.Set(hashKey, origKey, body, nil, 200, 100*time.Millisecond)

	// 立即获取应该命中
	entry, found, _ := pc.Get(hashKey, origKey)
	assert.True(t, found)
	assert.NotNil(t, entry)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 过期后应该未命中
	entry, found, stale := pc.Get(hashKey, origKey)
	assert.False(t, found)
	assert.False(t, stale)
	assert.Nil(t, entry)
}

// TestProxyCacheStale 测试过期缓存复用
func TestProxyCacheStale(t *testing.T) {
	rules := []cache.ProxyCacheRule{
		{
			Path:     "/",
			Methods:  []string{"GET"},
			Statuses: []int{200},
			MaxAge:   100 * time.Millisecond,
		},
	}

	// staleTime = 200ms，允许过期后 200ms 内复用
	pc := cache.NewProxyCache(rules, true, 200*time.Millisecond)
	require.NotNil(t, pc)

	origKey := "GET:/stale"
	hashKey := cache.HashPathWithMethod("/stale", "GET")
	body := []byte("stale data")

	// 存入缓存
	pc.Set(hashKey, origKey, body, nil, 200, 100*time.Millisecond)

	// 等待过期但仍在 stale 时间内
	time.Sleep(150 * time.Millisecond)

	// 应该返回 stale 数据
	entry, found, stale := pc.Get(hashKey, origKey)
	assert.True(t, found)
	assert.True(t, stale)
	assert.NotNil(t, entry)
	assert.Equal(t, body, entry.Data)
}

// TestProxyCacheHashKeyCollision 测试哈希碰撞检测
func TestProxyCacheHashKeyCollision(t *testing.T) {
	rules := []cache.ProxyCacheRule{
		{
			Path:     "/",
			Methods:  []string{"GET"},
			Statuses: []int{200},
			MaxAge:   60 * time.Second,
		},
	}

	pc := cache.NewProxyCache(rules, true, 30*time.Second)
	require.NotNil(t, pc)

	// 两个不同的 key
	key1 := "GET:/resource/1"
	key2 := "GET:/resource/2"

	hashKey1 := cache.HashPathWithMethod("/resource/1", "GET")
	hashKey2 := cache.HashPathWithMethod("/resource/2", "GET")

	// 存入不同的数据
	pc.Set(hashKey1, key1, []byte("data1"), nil, 200, 60*time.Second)
	pc.Set(hashKey2, key2, []byte("data2"), nil, 200, 60*time.Second)

	// 验证各自返回正确的数据
	entry1, found1, _ := pc.Get(hashKey1, key1)
	assert.True(t, found1)
	assert.Equal(t, []byte("data1"), entry1.Data)

	entry2, found2, _ := pc.Get(hashKey2, key2)
	assert.True(t, found2)
	assert.Equal(t, []byte("data2"), entry2.Data)
}

// TestProxyCacheConcurrent 测试并发访问
func TestProxyCacheConcurrent(t *testing.T) {
	rules := []cache.ProxyCacheRule{
		{
			Path:     "/",
			Methods:  []string{"GET"},
			Statuses: []int{200},
			MaxAge:   60 * time.Second,
		},
	}

	pc := cache.NewProxyCache(rules, true, 30*time.Second)
	require.NotNil(t, pc)

	// 并发写入
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			key := "GET:/concurrent"
			hashKey := cache.HashPathWithMethod("/concurrent", "GET")
			pc.Set(hashKey, key, []byte("concurrent data"), nil, 200, 60*time.Second)
			done <- true
		}(i)
	}

	// 等待所有写入完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证数据一致性
	key := "GET:/concurrent"
	hashKey := cache.HashPathWithMethod("/concurrent", "GET")
	entry, found, _ := pc.Get(hashKey, key)
	assert.True(t, found)
	assert.NotNil(t, entry)
}

// TestProxyCacheUsesCount 测试访问计数
func TestProxyCacheUsesCount(t *testing.T) {
	rules := []cache.ProxyCacheRule{
		{
			Path:     "/",
			Methods:  []string{"GET"},
			Statuses: []int{200},
			MaxAge:   60 * time.Second,
		},
	}

	pc := cache.NewProxyCache(rules, true, 30*time.Second)
	require.NotNil(t, pc)

	origKey := "GET:/counted"
	hashKey := cache.HashPathWithMethod("/counted", "GET")

	// 存入缓存
	pc.Set(hashKey, origKey, []byte("data"), nil, 200, 60*time.Second)

	// 多次访问
	for i := 0; i < 5; i++ {
		entry, found, _ := pc.Get(hashKey, origKey)
		assert.True(t, found)
		require.NotNil(t, entry)
	}

	// 验证访问计数
	entry, _, _ := pc.Get(hashKey, origKey)
	require.NotNil(t, entry)
	// Uses 计数应该是 6（5次循环 + 1次验证）
	assert.GreaterOrEqual(t, entry.Uses.Load(), int32(6))
}
