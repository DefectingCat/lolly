// Package lua 提供 Lua 脚本嵌入能力。
//
// 该文件实现 Lua 脚本字节码缓存（CodeCache），包括：
//   - 内联脚本缓存：基于 SHA256 哈希去重
//   - 文件脚本缓存：基于路径哈希 + 文件变更检测
//   - LRU 淘汰策略：容量满时淘汰最久未访问的缓存
//   - TTL 过期机制：超过生存期的缓存自动失效
//   - 文件监控：文件修改时间变化时自动重新编译
//
// 注意事项：
//   - 缓存读写使用 sync.RWMutex 保证并发安全
//   - 统计计数使用 atomic 操作
//
// 作者：xfy
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

// CacheKeyType 缓存键类型，区分内联脚本和文件脚本。
type CacheKeyType int

// 缓存键类型常量：内联脚本和文件脚本
const (
	// CacheKeyInline 内联脚本缓存键（通过 SHA256 哈希标识）
	CacheKeyInline CacheKeyType = iota

	// CacheKeyFile 文件脚本缓存键（通过路径 SHA256 哈希标识）
	CacheKeyFile
)

// CachedProto 缓存的编译后字节码。
type CachedProto struct {
	// ModTime 文件修改时间（仅文件脚本有效）
	ModTime time.Time

	// CachedAt 缓存存入时间（用于 TTL 过期检测）
	CachedAt time.Time

	// AccessAt 最后访问时间（用于 LRU 淘汰）
	AccessAt atomic.Value

	// Proto 编译后的 Lua 函数原型
	Proto *glua.FunctionProto

	// SourcePath 源文件路径（仅文件脚本有效）
	SourcePath string

	// SourceType 缓存键类型
	SourceType CacheKeyType
}

// CodeCache Lua 脚本字节码缓存。
//
// 支持两种缓存源：
//   - 内联脚本：基于内容 SHA256 哈希去重
//   - 文件脚本：基于路径哈希 + 文件变更检测
//
// 特性：
//   - LRU 淘汰：容量满时淘汰最久未访问的条目
//   - TTL 过期：超过生存期的缓存自动失效
//   - 文件监控：文件修改时间变化时自动重新编译
//   - 并发安全：使用 sync.RWMutex 保护读写
type CodeCache struct {
	// protos 缓存映射：键 -> 编译后的字节码
	protos map[string]*CachedProto

	// order 访问顺序列表（用于 LRU 淘汰）
	order []string

	// 最大缓存条目数
	maxSize int

	// 缓存生存时间
	ttl time.Duration

	// 缓存命中次数
	hits atomic.Uint64

	// 缓存未命中次数
	misses atomic.Uint64

	// 读写锁
	mu sync.RWMutex

	// 是否启用文件变更检测
	fileWatch bool
}

// NewCodeCache 创建字节码缓存实例。
//
// 参数：
//   - maxSize: 最大缓存条目数
//   - ttl: 缓存生存时间，零值表示永不过期
//   - fileWatch: 是否启用文件变更检测
//
// 返回值：
//   - *CodeCache: 初始化的缓存实例
func NewCodeCache(maxSize int, ttl time.Duration, fileWatch bool) *CodeCache {
	return &CodeCache{
		protos:    make(map[string]*CachedProto),
		order:     make([]string, 0, maxSize),
		maxSize:   maxSize,
		ttl:       ttl,
		fileWatch: fileWatch,
	}
}

// generateInlineKey 生成内联脚本的缓存键。
//
// 使用 SHA256 哈希算法对脚本内容进行摘要，前缀为 "nhli_"。
func (c *CodeCache) generateInlineKey(src string) string {
	hash := sha256.Sum256([]byte(src))
	return "nhli_" + hex.EncodeToString(hash[:])
}

// generateFileKey 生成文件脚本的缓存键。
//
// 使用 SHA256 哈希算法对文件路径进行摘要，前缀为 "nhlf_"。
// 注意：键基于路径而非内容，文件变更检测由 isFileChanged 负责。
func (c *CodeCache) generateFileKey(path string) string {
	hash := sha256.Sum256([]byte(path))
	return "nhlf_" + hex.EncodeToString(hash[:])
}

// GetOrCompileInline 获取或编译内联脚本。
//
// 查找流程：
//  1. 基于脚本内容生成缓存键
//  2. 检查缓存是否命中且未过期
//  3. 未命中则解析并编译脚本，存入缓存
//
// 参数：
//   - src: Lua 源代码字符串
//
// 返回值：
//   - *glua.FunctionProto: 编译后的函数原型
//   - error: 解析或编译失败时返回错误
func (c *CodeCache) GetOrCompileInline(src string) (*glua.FunctionProto, error) {
	key := c.generateInlineKey(src)

	c.mu.RLock()
	cached, ok := c.protos[key]
	c.mu.RUnlock()

	if ok && !c.isExpired(cached) {
		c.hits.Add(1)
		cached.AccessAt.Store(time.Now())
		return cached.Proto, nil
	}

	c.misses.Add(1)

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

// GetOrCompileFile 获取或编译文件脚本。
//
// 查找流程：
//  1. 基于文件路径生成缓存键
//  2. 检查缓存是否命中、未过期且文件未变更
//  3. 未命中则读取文件、解析并编译，存入缓存
//
// 参数：
//   - path: Lua 脚本文件路径
//
// 返回值：
//   - *glua.FunctionProto: 编译后的函数原型
//   - error: 读取、解析或编译失败时返回错误
func (c *CodeCache) GetOrCompileFile(path string) (*glua.FunctionProto, error) {
	key := c.generateFileKey(path)

	c.mu.RLock()
	cached, ok := c.protos[key]
	c.mu.RUnlock()

	// 检查是否需要重新加载
	if ok && !c.isExpired(cached) && !c.isFileChanged(cached) {
		c.hits.Add(1)
		cached.AccessAt.Store(time.Now())
		return cached.Proto, nil
	}

	c.misses.Add(1)

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

// storeLocked 将缓存条目存入映射（需已持有写锁）。
//
// 如果键已存在则更新；否则先检查容量并可能触发 LRU 淘汰。
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

// evictLocked 淘汰最久未访问的缓存条目（需已持有写锁）。
//
// 遍历 order 列表，找到 AccessAt 最早的条目并删除。
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

// isExpired 检查缓存条目是否超过 TTL。
//
// 如果 TTL 为零或负数，永不过期。
func (c *CodeCache) isExpired(cached *CachedProto) bool {
	if c.ttl <= 0 {
		return false
	}
	return time.Since(cached.CachedAt) > c.ttl
}

// isFileChanged 检查文件脚本是否已变更。
//
// 通过比较文件的修改时间与缓存中记录的 ModTime 判断。
// 如果文件不存在或无法 stat，视为已变更（触发重新编译）。
//
// 返回值：
//   - bool: true 表示文件已变更，false 表示未变更或文件监控未启用
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

// Stats 返回缓存统计信息。
//
// 返回值：
//   - hits: 缓存命中次数
//   - misses: 缓存未命中次数
//   - size: 当前缓存条目数
func (c *CodeCache) Stats() (hits, misses uint64, size int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits.Load(), c.misses.Load(), len(c.protos)
}

// HitRate 返回缓存命中率
func (c *CodeCache) HitRate() float64 {
	hits := c.hits.Load()
	misses := c.misses.Load()
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
