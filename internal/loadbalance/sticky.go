package loadbalance

import (
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

const stickyShardCount = 256

type stickyEntry struct {
	targetURL string
	expires   time.Time
}

type stickyShard struct {
	mu      sync.RWMutex
	entries map[string]*stickyEntry
}

// StickySession 实现基于 cookie 的会话粘性负载均衡。
type StickySession struct {
	config   StickyConfig
	fallback Balancer
	shards   []*stickyShard
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewStickySession 创建一个新的会话粘性负载均衡器。
func NewStickySession(config StickyConfig, fallback Balancer) *StickySession {
	shards := make([]*stickyShard, stickyShardCount)
	for i := range shards {
		shards[i] = &stickyShard{
			entries: make(map[string]*stickyEntry),
		}
	}
	s := &StickySession{
		config:   config,
		fallback: fallback,
		shards:   shards,
		stopCh:   make(chan struct{}),
	}
	return s
}

// Start 启动后台清理 goroutine。
func (s *StickySession) Start() {
	s.wg.Add(1)
	go s.cleanupLoop()
}

// Stop 停止后台清理 goroutine。
func (s *StickySession) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// cleanupLoop 定期清理过期的会话条目。
func (s *StickySession) cleanupLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// cleanup 清理所有 shard 中的过期条目。
func (s *StickySession) cleanup() {
	now := time.Now()
	for _, shard := range s.shards {
		shard.mu.Lock()
		for key, entry := range shard.entries {
			if now.After(entry.expires) {
				delete(shard.entries, key)
			}
		}
		shard.mu.Unlock()
	}
}

// Select 根据会话 cookie 选择目标。
// 如果存在有效的会话 cookie 且目标健康，则路由到该目标。
// 否则使用 fallback 选择器，并设置新的会话 cookie。
func (s *StickySession) Select(ctx *fasthttp.RequestCtx, targets []*Target) *Target {
	if !s.config.Enabled {
		return s.fallback.Select(targets)
	}

	// 检查现有 cookie
	cookieValue := ctx.Request.Header.Cookie(s.config.Name)
	if len(cookieValue) > 0 {
		decodedURL, expires, ok := decodeStickyCookie(string(cookieValue))
		if ok && decodedURL != "" && expires.After(time.Now()) {
			// 查找对应的目标
			for _, target := range targets {
				if target.URL == decodedURL && target.IsAvailable() {
					return target
				}
			}
			// 目标不可用，删除会话记录
			s.deleteSession(string(cookieValue))
		}
	}

	// 使用 fallback 选择目标
	selected := s.fallback.Select(targets)
	if selected != nil {
		newCookieValue := s.setCookie(ctx, selected.URL)
		s.recordSession(newCookieValue, selected.URL)
	}
	return selected
}

// SelectExcluding 排除指定目标后选择，委托给 fallback 实现。
func (s *StickySession) SelectExcluding(targets []*Target, excluded []*Target) *Target {
	return s.fallback.SelectExcluding(targets, excluded)
}

// setCookie 设置会话 cookie 到响应头，返回编码后的 cookie 值。
func (s *StickySession) setCookie(ctx *fasthttp.RequestCtx, targetURL string) string {
	expires := time.Now().Add(s.config.Expires)
	cookie := &fasthttp.Cookie{}
	cookie.SetKey(s.config.Name)
	encoded := encodeStickyCookie(targetURL, expires)
	cookie.SetValue(encoded)

	if s.config.Expires > 0 {
		cookie.SetExpire(expires)
	}
	if s.config.Domain != "" {
		cookie.SetDomain(s.config.Domain)
	}
	if s.config.Path != "" {
		cookie.SetPath(s.config.Path)
	} else {
		cookie.SetPath("/")
	}
	if s.config.Secure {
		cookie.SetSecure(true)
	}
	if s.config.HttpOnly {
		cookie.SetHTTPOnly(true)
	}

	switch s.config.SameSite {
	case "Strict":
		cookie.SetSameSite(fasthttp.CookieSameSiteStrictMode)
	case "None":
		cookie.SetSameSite(fasthttp.CookieSameSiteNoneMode)
	default:
		cookie.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	}

	ctx.Response.Header.SetCookie(cookie)
	return encoded
}

// recordSession 记录会话到 shard 中。
func (s *StickySession) recordSession(cookieValue, targetURL string) {
	shard := s.getShard(cookieValue)
	shard.mu.Lock()
	shard.entries[cookieValue] = &stickyEntry{
		targetURL: targetURL,
		expires:   time.Now().Add(s.config.Expires),
	}
	shard.mu.Unlock()
}

// deleteSession 从 shard 中删除会话记录。
func (s *StickySession) deleteSession(cookieValue string) {
	shard := s.getShard(cookieValue)
	shard.mu.Lock()
	delete(shard.entries, cookieValue)
	shard.mu.Unlock()
}

// getShard 根据 cookieValue 选择对应的 shard。
func (s *StickySession) getShard(cookieValue string) *stickyShard {
	hash := fnvHash64a(cookieValue)
	return s.shards[hash%stickyShardCount]
}

// encodeStickyCookie 将目标 URL 和过期时间编码为 cookie 值（base64）。
func encodeStickyCookie(targetURL string, expires time.Time) string {
	raw := targetURL + "|" + strconv.FormatInt(expires.Unix(), 10)
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// decodeStickyCookie 解码 cookie 值为目标 URL 和过期时间。
func decodeStickyCookie(value string) (targetURL string, expires time.Time, ok bool) {
	raw, err := base64.URLEncoding.DecodeString(value)
	if err != nil {
		return
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return
	}
	ts, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return
	}
	return parts[0], time.Unix(ts, 0), true
}

// Ensure StickySession implements the SelectExcluding part of Balancer interface.
// Note: Select signature differs (includes *fasthttp.RequestCtx), so it does
// not fully implement Balancer.
