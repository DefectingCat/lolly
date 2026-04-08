// Package cache 提供缓存模块的基准测试。
//
// 该文件测试缓存模块的性能，包括：
//   - LRU Get/Set 操作性能
//   - 并发读写性能
//   - 不同缓存大小下的性能表现
//
// 作者：xfy
package cache

import (
	"fmt"
	"hash/fnv"
	"testing"
	"time"
)

// hashKeyBench 计算字符串的 FNV-64a 哈希值，用于 benchmark。
func hashKeyBench(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// BenchmarkFileCacheGet 测试热点读取场景下的 Get 性能。
// 模拟缓存命中率高的场景，测试 LRU 链表的访问效率。
func BenchmarkFileCacheGet(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			fc := NewFileCache(int64(size), 0, 1*time.Hour)

			// 预填充缓存
			for i := 0; i < size; i++ {
				path := fmt.Sprintf("/file%d.txt", i)
				data := []byte("cached data content")
				_ = fc.Set(path, data, int64(len(data)), time.Now())
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					// 热点读取：集中在前面 10% 的数据
					path := fmt.Sprintf("/file%d.txt", i%(size/10+1))
					fc.Get(path)
					i++
				}
			})
		})
	}
}

// BenchmarkFileCacheSet 测试 Set 操作性能，包括 LRU 淘汰开销。
// 测试在缓存达到容量限制后的写入性能。
func BenchmarkFileCacheSet(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			fc := NewFileCache(int64(size), 0, 1*time.Hour)

			// 预填充到容量上限
			for i := 0; i < size; i++ {
				path := fmt.Sprintf("/file%d.txt", i)
				data := []byte("cached data content")
				_ = fc.Set(path, data, int64(len(data)), time.Now())
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				path := fmt.Sprintf("/newfile%d.txt", i)
				data := []byte("new cached data content")
				_ = fc.Set(path, data, int64(len(data)), time.Now())
			}
		})
	}
}

// BenchmarkFileCacheSetNoEviction 测试无淘汰场景下的 Set 性能。
// 此时缓存未满，没有 LRU 淘汰开销。
func BenchmarkFileCacheSetNoEviction(b *testing.B) {
	fc := NewFileCache(int64(b.N+1000), 0, 1*time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := fmt.Sprintf("/file%d.txt", i)
		data := []byte("cached data content")
		_ = fc.Set(path, data, int64(len(data)), time.Now())
	}
}

// BenchmarkFileCacheConcurrent 测试并发读写混合负载性能。
// 使用 90% Get / 10% Set 的混合负载模拟真实场景。
func BenchmarkFileCacheConcurrent(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			fc := NewFileCache(int64(size), 0, 1*time.Hour)

			// 预填充缓存
			for i := 0; i < size; i++ {
				path := fmt.Sprintf("/file%d.txt", i)
				data := []byte("cached data content")
				_ = fc.Set(path, data, int64(len(data)), time.Now())
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					// 90% Get, 10% Set
					if i%10 == 0 {
						path := fmt.Sprintf("/newfile%d.txt", i)
						data := []byte("updated data content")
						_ = fc.Set(path, data, int64(len(data)), time.Now())
					} else {
						path := fmt.Sprintf("/file%d.txt", i%size)
						fc.Get(path)
					}
					i++
				}
			})
		})
	}
}

// BenchmarkFileCacheGetOnly 测试纯读场景下的性能。
// 模拟静态文件服务的缓存读取。
func BenchmarkFileCacheGetOnly(b *testing.B) {
	fc := NewFileCache(1000, 0, 1*time.Hour)

	// 预填充缓存
	for i := 0; i < 1000; i++ {
		path := fmt.Sprintf("/static/file%d.css", i)
		data := make([]byte, 1024) // 1KB 数据
		_ = fc.Set(path, data, int64(len(data)), time.Now())
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			path := fmt.Sprintf("/static/file%d.css", i%1000)
			fc.Get(path)
			i++
		}
	})
}

// BenchmarkFileCacheSizeEviction 测试基于内存大小的淘汰性能。
// 测试当缓存超过内存限制时的淘汰开销。
func BenchmarkFileCacheSizeEviction(b *testing.B) {
	// 限制最大 1MB 内存
	maxSize := int64(1024 * 1024)
	fc := NewFileCache(0, maxSize, 1*time.Hour)

	// 预填充到接近容量上限
	data := make([]byte, 1024) // 1KB 每条
	for i := 0; i < 1000; i++ {
		path := fmt.Sprintf("/file%d.txt", i)
		_ = fc.Set(path, data, int64(len(data)), time.Now())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := fmt.Sprintf("/newfile%d.txt", i)
		newData := make([]byte, 1024)
		_ = fc.Set(path, newData, int64(len(newData)), time.Now())
	}
}

// BenchmarkFileCacheLRUTouch 测试 LRU 链表更新开销。
// 频繁访问同一批条目，观察 LRU 移动性能。
func BenchmarkFileCacheLRUTouch(b *testing.B) {
	fc := NewFileCache(100, 0, 1*time.Hour)

	// 预填充缓存
	for i := 0; i < 100; i++ {
		path := fmt.Sprintf("/file%d.txt", i)
		data := []byte("cached data")
		_ = fc.Set(path, data, int64(len(data)), time.Now())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 按顺序访问，触发 LRU 链表更新
		path := fmt.Sprintf("/file%d.txt", i%100)
		fc.Get(path)
	}
}

// BenchmarkProxyCacheGet 测试代理缓存 Get 性能。
func BenchmarkProxyCacheGet(b *testing.B) {
	pc := NewProxyCache(nil, false, 0)

	// 预填充缓存
	for i := 0; i < 1000; i++ {
		origKey := fmt.Sprintf("key%d", i)
		hashKey := hashKeyBench(origKey)
		data := []byte("response body")
		headers := map[string]string{"Content-Type": "application/json"}
		pc.Set(hashKey, origKey, data, headers, 200, 10*time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			origKey := fmt.Sprintf("key%d", i%1000)
			hashKey := hashKeyBench(origKey)
			pc.Get(hashKey, origKey)
			i++
		}
	})
}

// BenchmarkProxyCacheSet 测试代理缓存 Set 性能。
func BenchmarkProxyCacheSet(b *testing.B) {
	pc := NewProxyCache(nil, false, 0)
	data := []byte("response body")
	headers := map[string]string{"Content-Type": "application/json"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		origKey := fmt.Sprintf("key%d", i)
		hashKey := hashKeyBench(origKey)
		pc.Set(hashKey, origKey, data, headers, 200, 10*time.Minute)
	}
}

// BenchmarkProxyCacheConcurrent 测试代理缓存并发混合负载。
// 使用 90% Get / 10% Set 的混合负载。
func BenchmarkProxyCacheConcurrent(b *testing.B) {
	pc := NewProxyCache(nil, false, 0)

	// 预填充缓存
	for i := 0; i < 1000; i++ {
		origKey := fmt.Sprintf("key%d", i)
		hashKey := hashKeyBench(origKey)
		data := []byte("response body")
		headers := map[string]string{"Content-Type": "application/json"}
		pc.Set(hashKey, origKey, data, headers, 200, 10*time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				origKey := fmt.Sprintf("newkey%d", i)
				hashKey := hashKeyBench(origKey)
				data := []byte("new response body")
				headers := map[string]string{"Content-Type": "application/json"}
				pc.Set(hashKey, origKey, data, headers, 200, 10*time.Minute)
			} else {
				origKey := fmt.Sprintf("key%d", i%1000)
				hashKey := hashKeyBench(origKey)
				pc.Get(hashKey, origKey)
			}
			i++
		}
	})
}
