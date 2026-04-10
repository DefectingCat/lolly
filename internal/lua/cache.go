// Package lua 提供 Lua 脚本嵌入能力
package lua

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	glua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// CacheKeyType 缓存键类型
type CacheKeyType int

// 缓存键类型常量：内联脚本和文件脚本
const (
	// CacheKeyInline 内联脚本缓存键
	CacheKeyInline CacheKeyType = iota // 内联脚本
	// CacheKeyFile 文件脚本缓存键
	CacheKeyFile // 文件脚本
)

// CachedProto 缓存的字节码
type CachedProto struct {
	Proto      *glua.FunctionProto // 编译后的字节码
	SourceType CacheKeyType        // 来源类型
	SourcePath string              // 文件路径（仅 file 类型）
	ModTime    time.Time           // 文件修改时间（仅 file 类型）
	CachedAt   time.Time           // 缓存时间
	AccessAt   atomic.Value        // 最后访问时间
}

// CodeCache 字节码缓存
type CodeCache struct {
	mu        sync.RWMutex
	protos    map[string]*CachedProto // 缓存键 -> 字节码
	order     []string                // LRU 顺序
	maxSize   int                     // 最大缓存数
	ttl       time.Duration           // 缓存 TTL
	fileWatch bool                    // 是否监控文件变更

	// 统计
	hits   uint64
	misses uint64
}

// NewCodeCache 创建字节码缓存
func NewCodeCache(maxSize int, ttl time.Duration, fileWatch bool) *CodeCache {
	return &CodeCache{
		protos:    make(map[string]*CachedProto),
		order:     make([]string, 0, maxSize),
		maxSize:   maxSize,
		ttl:       ttl,
		fileWatch: fileWatch,
	}
}

// generateInlineKey 生成内联脚本缓存键
func (c *CodeCache) generateInlineKey(src string) string {
	hash := sha256.Sum256([]byte(src))
	return "nhli_" + hex.EncodeToString(hash[:])
}

// generateFileKey 生成文件脚本缓存键
func (c *CodeCache) generateFileKey(path string) string {
	hash := sha256.Sum256([]byte(path))
	return "nhlf_" + hex.EncodeToString(hash[:])
}

// GetOrCompileInline 获取或编译内联脚本
func (c *CodeCache) GetOrCompileInline(src string) (*glua.FunctionProto, error) {
	key := c.generateInlineKey(src)

	c.mu.RLock()
	cached, ok := c.protos[key]
	c.mu.RUnlock()

	if ok && !c.isExpired(cached) {
		atomic.AddUint64(&c.hits, 1)
		cached.AccessAt.Store(time.Now())
		return cached.Proto, nil
	}

	atomic.AddUint64(&c.misses, 1)

	// 编译脚本
	chunk, err := parse.Parse(strings.NewReader(src), "<inline>")
	if err != nil {
		return nil, fmt.Errorf("parse inline script: %w", err)
	}
	proto, err := glua.Compile(chunk, "<inline>")
	if err != nil {
		return nil, fmt.Errorf("compile inline script: %w", err)
	}

	// 存入缓存
	cached = &CachedProto{
		Proto:      proto,
		SourceType: CacheKeyInline,
		CachedAt:   time.Now(),
	}
	cached.AccessAt.Store(time.Now())

	c.mu.Lock()
	c.storeLocked(key, cached)
	c.mu.Unlock()

	return proto, nil
}

// GetOrCompileFile 获取或编译文件脚本
func (c *CodeCache) GetOrCompileFile(path string) (*glua.FunctionProto, error) {
	key := c.generateFileKey(path)

	c.mu.RLock()
	cached, ok := c.protos[key]
	c.mu.RUnlock()

	// 检查是否需要重新加载
	if ok && !c.isExpired(cached) && !c.isFileChanged(cached) {
		atomic.AddUint64(&c.hits, 1)
		cached.AccessAt.Store(time.Now())
		return cached.Proto, nil
	}

	atomic.AddUint64(&c.misses, 1)

	// 读取文件
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}

	// 获取文件信息
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file %s: %w", path, err)
	}

	// 编译脚本
	reader := bufio.NewReader(strings.NewReader(string(content)))
	chunk, err := parse.Parse(reader, path)
	if err != nil {
		return nil, fmt.Errorf("parse file %s: %w", path, err)
	}
	proto, err := glua.Compile(chunk, path)
	if err != nil {
		return nil, fmt.Errorf("compile file %s: %w", path, err)
	}

	// 存入缓存
	cached = &CachedProto{
		Proto:      proto,
		SourceType: CacheKeyFile,
		SourcePath: path,
		ModTime:    info.ModTime(),
		CachedAt:   time.Now(),
	}
	cached.AccessAt.Store(time.Now())

	c.mu.Lock()
	c.storeLocked(key, cached)
	c.mu.Unlock()

	return proto, nil
}

// storeLocked 存入缓存（需持有锁）
func (c *CodeCache) storeLocked(key string, cached *CachedProto) {
	// 如果已存在，更新
	if _, ok := c.protos[key]; ok {
		c.protos[key] = cached
		return
	}

	// LRU 淘汰
	if len(c.protos) >= c.maxSize {
		c.evictLocked()
	}

	c.protos[key] = cached
	c.order = append(c.order, key)
}

// evictLocked 淘汰最久未使用的缓存（需持有锁）
func (c *CodeCache) evictLocked() {
	if len(c.order) == 0 {
		return
	}

	// 找到最久未访问的
	oldestKey := c.order[0]
	oldestTime := time.Now()

	for _, key := range c.order {
		cached := c.protos[key]
		if t, ok := cached.AccessAt.Load().(time.Time); ok && t.Before(oldestTime) {
			oldestTime = t
			oldestKey = key
		}
	}

	// 删除
	delete(c.protos, oldestKey)
	for i, k := range c.order {
		if k == oldestKey {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

// isExpired 检查缓存是否过期
func (c *CodeCache) isExpired(cached *CachedProto) bool {
	if c.ttl <= 0 {
		return false
	}
	return time.Since(cached.CachedAt) > c.ttl
}

// isFileChanged 检查文件是否变更
func (c *CodeCache) isFileChanged(cached *CachedProto) bool {
	if !c.fileWatch || cached.SourceType != CacheKeyFile {
		return false
	}

	info, err := os.Stat(cached.SourcePath)
	if err != nil {
		return true // 文件不存在，视为变更
	}

	return info.ModTime().After(cached.ModTime)
}

// Stats 返回缓存统计
func (c *CodeCache) Stats() (hits, misses uint64, size int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return atomic.LoadUint64(&c.hits), atomic.LoadUint64(&c.misses), len(c.protos)
}

// HitRate 返回缓存命中率
func (c *CodeCache) HitRate() float64 {
	hits := atomic.LoadUint64(&c.hits)
	misses := atomic.LoadUint64(&c.misses)
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// Clear 清空缓存
func (c *CodeCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.protos = make(map[string]*CachedProto)
	c.order = c.order[:0]
}
