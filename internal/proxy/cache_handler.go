// Package proxy 提供反向代理的核心功能，支持请求转发、负载均衡、健康检查等特性。
//
// 包含缓存处理器相关的结构体，用于配置代理缓存行为。
//
// 作者：xfy
package proxy

import (
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/loadbalance"
)

// buildCacheKeyHash 使用 FNV-64a 计算缓存键的 uint64 哈希值。
// 使用零分配方式构建哈希，避免 []byte(origKey) 转换。
// 缓存键包含请求方法、Host、URI 以及配置的 Vary 请求头，防止缓存中毒。
func (p *Proxy) buildCacheKeyHash(ctx *fasthttp.RequestCtx, varyHeaders []string) (uint64, string) {
	return p.buildCacheKeyHashWithHost(ctx, ctx.Request.Header.Host(), varyHeaders)
}

// buildCacheKeyHashWithHost 与 buildCacheKeyHash 相同，但使用显式传入的 Host，
// 避免请求头在缓存键计算前被修改（例如 proxy 将 Host 改写为上游目标）。
func (p *Proxy) buildCacheKeyHashWithHost(ctx *fasthttp.RequestCtx, host []byte, varyHeaders []string) (uint64, string) {
	method := ctx.Request.Header.Method()
	uri := ctx.Request.URI().RequestURI()

	var h uint64 = 14695981039346656037
	for _, b := range [][]byte{method, host, uri} {
		h ^= uint64(':')
		h *= 1099511628211
		for i := 0; i < len(b); i++ {
			h ^= uint64(b[i])
			h *= 1099511628211
		}
	}

	for _, name := range varyHeaders {
		value := ctx.Request.Header.Peek(name)
		h ^= uint64(':')
		h *= 1099511628211
		for i := 0; i < len(value); i++ {
			h ^= uint64(value[i])
			h *= 1099511628211
		}
	}

	origKey := b2s(method) + ":" + b2s(host) + b2s(uri)
	for _, name := range varyHeaders {
		origKey += ":" + name + "=" + b2s(ctx.Request.Header.Peek(name))
	}
	return h, origKey
}

func (p *Proxy) buildCacheKeyHashValue(ctx *fasthttp.RequestCtx, varyHeaders []string) uint64 {
	h, _ := p.buildCacheKeyHash(ctx, varyHeaders)
	return h
}

// writeCachedResponse 将缓存的响应写入 FastHTTP 响应上下文。
//
// 设置响应体、状态码、响应头，并添加 X-Cache: HIT 头标记缓存命中。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - entry: 缓存条目，包含响应数据和元数据
func (p *Proxy) writeCachedResponse(ctx *fasthttp.RequestCtx, entry *cache.ProxyCacheEntry) {
	ctx.Response.SetBody(entry.Data)
	ctx.Response.SetStatusCode(entry.Status)
	for key, value := range entry.Headers {
		ctx.Response.Header.Set(key, value)
	}
	ctx.Response.Header.Set("X-Cache", "HIT")
}

// backgroundRefresh 在后台异步刷新缓存条目。
//
// 向对应的上游目标发送请求，获取最新响应并更新缓存。
// 该方法在独立 goroutine 中运行，不阻塞主请求流程。
//
// 参数：
//   - req: 预复制的请求副本（调用方负责 Acquire/Release）
//   - target: 要刷新的后端目标
//   - hashKey: 缓存哈希键
//   - origKey: 缓存原始键
func (p *Proxy) backgroundRefresh(req *fasthttp.Request, target *loadbalance.Target, hashKey uint64, origKey string) {
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	// 如果启用 Revalidate，添加条件请求头
	if p.config.Cache.Revalidate {
		if entry, ok, _ := p.cache.Get(hashKey, origKey); ok {
			if entry.LastModified != "" {
				req.Header.Set("If-Modified-Since", entry.LastModified)
			}
			if entry.ETag != "" {
				req.Header.Set("If-None-Match", entry.ETag)
			}
		}
	}

	// 获取客户端
	client := p.getClient(target.URL)
	if client == nil {
		return
	}

	// 执行请求
	err := client.Do(req, resp)
	if err != nil {
		p.cache.ReleaseLock(hashKey, err)
		return
	}

	// 处理 304 Not Modified 响应
	if resp.StatusCode() == 304 {
		newHeaders := make(map[string]string, 5) // 预分配，通常只有 Last-Modified 和 ETag
		if lm := resp.Header.Peek("Last-Modified"); len(lm) > 0 {
			newHeaders["Last-Modified"] = b2s(lm)
		}
		if et := resp.Header.Peek("ETag"); len(et) > 0 {
			newHeaders["ETag"] = b2s(et)
		}
		p.cache.RefreshTTL(hashKey, origKey, newHeaders)
		return
	}

	// 提取响应头
	headers := make(map[string]string, 20)
	for key, value := range resp.Header.All() {
		headers[string(key)] = string(value)
	}

	// 更新缓存
	p.cache.Set(hashKey, origKey, resp.Body(), headers, resp.StatusCode(), p.getCacheDuration(resp.StatusCode()))
}

// GetCache 返回代理的 ProxyCache 实例（用于 purge handler）。
// 如果缓存未启用，返回 nil。
func (p *Proxy) GetCache() *cache.ProxyCache {
	return p.cache
}

// GetCacheStats 返回代理缓存的统计信息。
// 如果缓存未启用，返回 nil。
func (p *Proxy) GetCacheStats() *cache.ProxyCacheStats {
	if p.cache == nil {
		return nil
	}
	stats := p.cache.Stats()
	return &stats
}

// getCacheDuration 根据状态码获取缓存时间。
// 优先级：CacheValid 配置 > MaxAge
//
// 映射规则：
//   - 200-299: CacheValid.OK（0 时继承 MaxAge）
//   - 301/302: CacheValid.Redirect
//   - 404: CacheValid.NotFound
//   - 400-499（除 404）: CacheValid.ClientError
//   - 500-599: CacheValid.ServerError
//   - 其他: 不缓存（返回 0）
func (p *Proxy) getCacheDuration(statusCode int) time.Duration {
	// 无 CacheValid 配置，使用 MaxAge
	if p.config.CacheValid == nil {
		return p.config.Cache.MaxAge
	}

	cv := p.config.CacheValid

	switch {
	case statusCode >= 200 && statusCode < 300:
		if cv.OK > 0 {
			return cv.OK
		}
		return p.config.Cache.MaxAge // 0 表示继承 MaxAge

	case statusCode == 301 || statusCode == 302:
		return cv.Redirect // 0 表示不缓存

	case statusCode == 404:
		return cv.NotFound

	case statusCode >= 400 && statusCode < 500:
		return cv.ClientError

	case statusCode >= 500:
		return cv.ServerError

	default:
		return 0 // 不缓存
	}
}
