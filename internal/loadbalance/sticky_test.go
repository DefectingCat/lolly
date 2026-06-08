package loadbalance

import (
	"sync"
	"testing"

	"github.com/valyala/fasthttp"
)

// TestStickySession_BasicRoute 测试基本的会话粘性路由。
// 第一次请求应设置 cookie，第二次携带相同 cookie 应路由到同一目标。
func TestStickySession_BasicRoute(t *testing.T) {
	t.Parallel()
	t.Run("首次请求设置cookie并路由", func(_ *testing.T) {
		config := DefaultStickyConfig()
		config.Enabled = true
		fallback := NewRoundRobin()
		sticky := NewStickySession(config, fallback)
		defer sticky.Stop()

		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		ctx := &fasthttp.RequestCtx{}
		got := sticky.Select(ctx, targets)
		if got == nil {
			t.Fatal("Select() = nil, want non-nil")
		}

		// 验证设置了 cookie
		cookieValue := ctx.Response.Header.PeekCookie(config.Name)
		if len(cookieValue) == 0 {
			t.Error("首次请求未设置 cookie")
		}
	})

	t.Run("相同cookie路由到同一目标", func(_ *testing.T) {
		config := DefaultStickyConfig()
		config.Enabled = true
		fallback := NewRoundRobin()
		sticky := NewStickySession(config, fallback)
		defer sticky.Stop()

		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		// 第一次请求
		ctx1 := &fasthttp.RequestCtx{}
		got1 := sticky.Select(ctx1, targets)
		if got1 == nil {
			t.Fatal("第一次 Select() = nil")
		}

		// 提取 cookie
		cookie := &fasthttp.Cookie{}
		cookie.SetKey(config.Name)
		if err := cookie.ParseBytes(ctx1.Response.Header.PeekCookie(config.Name)); err != nil {
			t.Fatalf("解析 cookie 失败: %v", err)
		}

		// 第二次请求携带相同 cookie
		ctx2 := &fasthttp.RequestCtx{}
		ctx2.Request.Header.SetCookie(config.Name, string(cookie.Value()))
		got2 := sticky.Select(ctx2, targets)
		if got2 == nil {
			t.Fatal("第二次 Select() = nil")
		}

		if got2.URL != got1.URL {
			t.Errorf("相同 cookie 路由到不同目标: 第一次=%q, 第二次=%q", got1.URL, got2.URL)
		}
	})

	t.Run("禁用时不设置cookie", func(_ *testing.T) {
		config := DefaultStickyConfig()
		config.Enabled = false
		fallback := NewRoundRobin()
		sticky := NewStickySession(config, fallback)
		defer sticky.Stop()

		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
		}

		ctx := &fasthttp.RequestCtx{}
		got := sticky.Select(ctx, targets)
		if got == nil {
			t.Fatal("Select() = nil")
		}

		cookieValue := ctx.Response.Header.PeekCookie(config.Name)
		if len(cookieValue) > 0 {
			t.Error("禁用时不应设置 cookie")
		}
	})
}

// TestStickySession_TargetUnavailable 测试目标不可用时回退到 fallback。
func TestStickySession_TargetUnavailable(t *testing.T) {
	t.Parallel()
	t.Run("目标不健康时回退", func(_ *testing.T) {
		config := DefaultStickyConfig()
		config.Enabled = true
		fallback := NewRoundRobin()
		sticky := NewStickySession(config, fallback)
		defer sticky.Stop()

		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		// 第一次请求，记录会话
		ctx1 := &fasthttp.RequestCtx{}
		got1 := sticky.Select(ctx1, targets)
		if got1 == nil {
			t.Fatal("第一次 Select() = nil")
		}

		// 提取 cookie
		cookie := &fasthttp.Cookie{}
		cookie.SetKey(config.Name)
		if err := cookie.ParseBytes(ctx1.Response.Header.PeekCookie(config.Name)); err != nil {
			t.Fatalf("解析 cookie 失败: %v", err)
		}

		// 使之前选中的目标不健康
		for _, target := range targets {
			if target.URL == got1.URL {
				target.Healthy.Store(false)
				break
			}
		}

		// 第二次请求，应回退到其他目标
		ctx2 := &fasthttp.RequestCtx{}
		ctx2.Request.Header.SetCookie(config.Name, string(cookie.Value()))
		got2 := sticky.Select(ctx2, targets)
		if got2 == nil {
			t.Fatal("第二次 Select() = nil")
		}

		if got2.URL == got1.URL {
			t.Errorf("不健康目标未回退: %q", got2.URL)
		}
	})
}

// TestStickySession_CookieEncodeDecode 测试 cookie 编解码。
func TestStickySession_CookieEncodeDecode(t *testing.T) {
	t.Parallel()
	t.Run("编码解码round-trip", func(_ *testing.T) {
		url := "http://backend1:8080"
		encoded := encodeStickyCookie(url)
		if encoded == "" {
			t.Fatal("encodeStickyCookie() 返回空字符串")
		}

		decoded, err := decodeStickyCookie(encoded)
		if err != nil {
			t.Fatalf("decodeStickyCookie() 错误: %v", err)
		}

		if decoded != url {
			t.Errorf("解码后 URL = %q, want %q", decoded, url)
		}
	})

	t.Run("空URL编码解码", func(_ *testing.T) {
		encoded := encodeStickyCookie("")
		decoded, err := decodeStickyCookie(encoded)
		if err != nil {
			t.Fatalf("decodeStickyCookie() 错误: %v", err)
		}
		if decoded != "" {
			t.Errorf("解码后 URL = %q, want 空字符串", decoded)
		}
	})

	t.Run("无效编码", func(_ *testing.T) {
		_, err := decodeStickyCookie("invalid-base64!!!")
		if err == nil {
			t.Error("decodeStickyCookie() 应返回错误")
		}
	})
}

// TestStickySession_Concurrent 测试并发安全。
// 100 个 goroutine 同时访问会话存储。
func TestStickySession_Concurrent(t *testing.T) {
	t.Parallel()
	config := DefaultStickyConfig()
	config.Enabled = true
	fallback := NewRoundRobin()
	sticky := NewStickySession(config, fallback)
	defer sticky.Stop()

	targets := []*Target{
		createHealthyTarget("http://backend1:8080", true),
		createHealthyTarget("http://backend2:8080", true),
		createHealthyTarget("http://backend3:8080", true),
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx := &fasthttp.RequestCtx{}
			// 交替使用有 cookie 和没有 cookie 的请求
			if idx%2 == 0 {
				ctx.Request.Header.SetCookie(config.Name, encodeStickyCookie("http://backend1:8080"))
			}
			got := sticky.Select(ctx, targets)
			if got == nil {
				t.Error("并发 Select() = nil")
			}
		}(i)
	}
	wg.Wait()
}

// TestStickySession_SelectExcluding 测试排除选择委托给 fallback。
func TestStickySession_SelectExcluding(t *testing.T) {
	t.Parallel()
	t.Run("SelectExcluding委托给fallback", func(_ *testing.T) {
		config := DefaultStickyConfig()
		config.Enabled = true
		fallback := NewRoundRobin()
		sticky := NewStickySession(config, fallback)
		defer sticky.Stop()

		targets := []*Target{
			createHealthyTarget("http://backend1:8080", true),
			createHealthyTarget("http://backend2:8080", true),
		}

		excluded := []*Target{targets[0]}
		got := sticky.SelectExcluding(targets, excluded)
		if got == nil {
			t.Fatal("SelectExcluding() = nil")
		}
		if got.URL != "http://backend2:8080" {
			t.Errorf("SelectExcluding() = %q, want %q", got.URL, "http://backend2:8080")
		}
	})
}
