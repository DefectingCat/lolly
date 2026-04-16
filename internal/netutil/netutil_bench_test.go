// Package netutil 提供网络工具功能的基准测试。
//
// 该文件测试网络工具模块的性能，包括：
//   - 客户端 IP 提取性能
//   - IP 解析为 net.IP 性能
//   - 端口移除性能
//   - 端口检查性能
//
// 作者：xfy
package netutil

import (
	"net"
	"testing"

	"github.com/valyala/fasthttp"
)

// createBenchCtx 创建用于基准测试的 fasthttp 上下文。
func createBenchCtx() *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	ctx.Init(&fasthttp.Request{}, nil, nil)
	return ctx
}

// BenchmarkExtractClientIP 测试 ExtractClientIP 函数的性能。
// 覆盖不同场景：X-Forwarded-For 单 IP、多 IP、X-Real-IP、RemoteAddr 回退。
func BenchmarkExtractClientIP(b *testing.B) {
	b.Run("X-Forwarded-For single IP", func(b *testing.B) {
		ctx := createBenchCtx()
		ctx.Request.Header.Set("X-Forwarded-For", "192.168.1.100")
		b.ResetTimer()
		for b.Loop() {
			ExtractClientIP(ctx)
		}
	})

	b.Run("X-Forwarded-For multiple IPs", func(b *testing.B) {
		ctx := createBenchCtx()
		ctx.Request.Header.Set("X-Forwarded-For", "192.168.1.100, 10.0.0.1, 172.16.0.1")
		b.ResetTimer()
		for b.Loop() {
			ExtractClientIP(ctx)
		}
	})

	b.Run("X-Real-IP only", func(b *testing.B) {
		ctx := createBenchCtx()
		ctx.Request.Header.Set("X-Real-IP", "192.168.1.200")
		b.ResetTimer()
		for b.Loop() {
			ExtractClientIP(ctx)
		}
	})

	b.Run("RemoteAddr fallback", func(b *testing.B) {
		ctx := createBenchCtx()
		b.ResetTimer()
		for b.Loop() {
			ExtractClientIP(ctx)
		}
	})
}

// BenchmarkExtractClientIPNet 测试 ExtractClientIPNet 函数的性能。
// 覆盖不同场景：X-Forwarded-For、X-Real-IP、RemoteAddr 回退。
func BenchmarkExtractClientIPNet(b *testing.B) {
	b.Run("X-Forwarded-For single IP", func(b *testing.B) {
		ctx := createBenchCtx()
		ctx.Request.Header.Set("X-Forwarded-For", "192.168.1.100")
		b.ResetTimer()
		for b.Loop() {
			ExtractClientIPNet(ctx)
		}
	})

	b.Run("X-Real-IP only", func(b *testing.B) {
		ctx := createBenchCtx()
		ctx.Request.Header.Set("X-Real-IP", "192.168.1.200")
		b.ResetTimer()
		for b.Loop() {
			ExtractClientIPNet(ctx)
		}
	})

	b.Run("RemoteAddr fallback", func(b *testing.B) {
		ctx := createBenchCtx()
		b.ResetTimer()
		for b.Loop() {
			_ = ExtractClientIPNet(ctx)
		}
	})
}

// BenchmarkStripPort 测试 StripPort 函数的性能。
// 覆盖不同场景：IPv4 带端口、IPv6 带端口、无端口、空字符串。
func BenchmarkStripPort(b *testing.B) {
	b.Run("IPv4 with port", func(b *testing.B) {
		host := "example.com:8080"
		b.ResetTimer()
		for b.Loop() {
			StripPort(host)
		}
	})

	b.Run("IPv6 with port", func(b *testing.B) {
		host := "[2001:db8::1]:8443"
		b.ResetTimer()
		for b.Loop() {
			StripPort(host)
		}
	})

	b.Run("no port", func(b *testing.B) {
		host := "example.com"
		b.ResetTimer()
		for b.Loop() {
			StripPort(host)
		}
	})

	b.Run("empty string", func(b *testing.B) {
		host := ""
		b.ResetTimer()
		for b.Loop() {
			StripPort(host)
		}
	})
}

// BenchmarkHasPort 测试 HasPort 函数的性能。
// 覆盖不同场景：IPv4 带端口、IPv6 带端口、无端口、空字符串。
func BenchmarkHasPort(b *testing.B) {
	b.Run("IPv4 with port", func(b *testing.B) {
		host := "example.com:8080"
		b.ResetTimer()
		for b.Loop() {
			HasPort(host)
		}
	})

	b.Run("IPv6 with port", func(b *testing.B) {
		host := "[2001:db8::1]:443"
		b.ResetTimer()
		for b.Loop() {
			HasPort(host)
		}
	})

	b.Run("no port", func(b *testing.B) {
		host := "example.com"
		b.ResetTimer()
		for b.Loop() {
			HasPort(host)
		}
	})

	b.Run("empty string", func(b *testing.B) {
		host := ""
		b.ResetTimer()
		for b.Loop() {
			HasPort(host)
		}
	})
}

// 确保 net 包被使用（避免未使用导入警告）
var _ = net.IPv4zero
