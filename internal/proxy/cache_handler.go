package proxy

import (
	"hash/fnv"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/cache"
	"rua.plus/lolly/internal/loadbalance"
)

// buildCacheKey 构建缓存键字符串。
//
// 使用请求方法和完整请求 URI 作为缓存键。
// 该函数保留用于日志记录和调试场景。
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//
// 返回值：
//   - string: 缓存键（格式 "METHOD:URI"）
func (p *Proxy) buildCacheKey(ctx *fasthttp.RequestCtx) string {
	// 使用请求方法和路径作为缓存键
	return string(ctx.Request.Header.Method()) + ":" + string(ctx.Request.URI().RequestURI())
}

// buildCacheKeyHash 使用 FNV-64a 计算缓存键的 uint64 哈希值。
// 返回哈希值和原始字符串键。
// 注意：此函数会先构建字符串键再哈希，存在双重分配。
// 对于只需要哈希值的场景，使用 buildCacheKeyHashValue 代替。
func (p *Proxy) buildCacheKeyHash(ctx *fasthttp.RequestCtx) (uint64, string) {
	// 构建原始 key
	origKey := p.buildCacheKey(ctx)

	// 使用 FNV-64a 计算哈希
	h := fnv.New64a()
	h.Write([]byte(origKey))
	return h.Sum64(), origKey
}

// buildCacheKeyHashValue 直接计算缓存键的哈希值，零字符串分配。
// 用于只需要哈希值而不需要原始键的场景。
func (p *Proxy) buildCacheKeyHashValue(ctx *fasthttp.RequestCtx) uint64 {
	h := fnv.New64a()
	h.Write(ctx.Request.Header.Method())
	h.Write([]byte(":"))
	h.Write(ctx.Request.URI().RequestURI())
	return h.Sum64()
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
//   - ctx: 原始 FastHTTP 请求上下文（仅用于复制请求信息）
//   - target: 要刷新的后端目标
//   - hashKey: 缓存哈希键
//   - origKey: 缓存原始键
func (p *Proxy) backgroundRefresh(ctx *fasthttp.RequestCtx, target *loadbalance.Target, hashKey uint64, origKey string) {
	// 创建新的请求上下文副本
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	// 复制原始请求
	ctx.Request.CopyTo(req)

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
		newHeaders := make(map[string]string)
		if lm := resp.Header.Peek("Last-Modified"); len(lm) > 0 {
			newHeaders["Last-Modified"] = string(lm)
		}
		if et := resp.Header.Peek("ETag"); len(et) > 0 {
			newHeaders["ETag"] = string(et)
		}
		p.cache.RefreshTTL(hashKey, origKey, newHeaders)
		return
	}

	// 提取响应头（使用 pool 复用 map）
	headers, ok := headersPool.Get().(map[string]string)
	if !ok {
		headers = make(map[string]string, 20)
	}
	for k := range headers {
		delete(headers, k)
	}
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
