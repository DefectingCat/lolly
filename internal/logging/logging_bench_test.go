// Package logging 提供日志功能的性能测试。
//
// 该文件包含日志相关的基准测试，用于评估关键操作的性能：
//   - JSON 格式访问日志记录
//   - 模板格式访问日志记录
//   - 变量展开开销
//   - 日志级别解析
//
// 主要用途：
//
//	用于监控日志系统的性能特征，优化内存分配和执行时间。
//
// 注意事项：
//   - 使用 -benchmem 标志查看内存分配统计
//   - 测试使用模拟数据避免外部依赖
//
// 作者：xfy
package logging

import (
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// BenchmarkLoggerLogAccessJSON 测试 JSON 格式访问日志记录性能。
// 这是最常见的访问日志格式，使用 zerolog 直接输出结构化数据。
// 预期结果：极低内存分配（< 1 allocs/op），高性能。
func BenchmarkLoggerLogAccessJSON(b *testing.B) {
	logger := New(&config.LoggingConfig{
		Access: config.AccessLogConfig{
			Path:   "stdout",
			Format: "json",
		},
		Error: config.ErrorLogConfig{
			Path:  "stdout",
			Level: "info",
		},
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/users?id=123")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.Set("User-Agent", "benchmark-agent/1.0")
	ctx.Request.Header.Set("Referer", "http://example.com/")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.LogAccess(ctx, 200, 1024, 150*time.Millisecond)
	}
}

// BenchmarkLoggerLogAccessTemplate 测试模板格式访问日志记录性能。
// 重点测试变量展开和字符串处理的开销，通常会有更多内存分配。
// 使用 Nginx 风格的日志格式模板。
func BenchmarkLoggerLogAccessTemplate(b *testing.B) {
	logger := New(&config.LoggingConfig{
		Access: config.AccessLogConfig{
			Path:   "stdout",
			Format: "$remote_addr - $remote_user [$time_local] \"$request\" $status $body_bytes_sent \"$http_referer\" \"$http_user_agent\" $request_time",
		},
		Error: config.ErrorLogConfig{
			Path:  "stdout",
			Level: "info",
		},
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/users?id=123")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.Set("User-Agent", "benchmark-agent/1.0")
	ctx.Request.Header.Set("Referer", "http://example.com/")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.LogAccess(ctx, 200, 1024, 150*time.Millisecond)
	}
}

// BenchmarkLoggerLogAccessSimpleTemplate 测试简单模板的访问日志记录性能。
// 相比复杂模板，变量数量更少，字符串拼接开销更低。
func BenchmarkLoggerLogAccessSimpleTemplate(b *testing.B) {
	logger := New(&config.LoggingConfig{
		Access: config.AccessLogConfig{
			Path:   "stdout",
			Format: "$remote_addr $request $status $body_bytes_sent",
		},
		Error: config.ErrorLogConfig{
			Path:  "stdout",
			Level: "info",
		},
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/test")
	ctx.Request.Header.SetMethod("POST")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.LogAccess(ctx, 201, 512, 50*time.Millisecond)
	}
}

// BenchmarkFormatAccessLog 直接测试 formatAccessLog 函数性能。
// 隔离变量展开的开销，不经过日志输出层。
func BenchmarkFormatAccessLog(b *testing.B) {
	logger := New(&config.LoggingConfig{
		Access: config.AccessLogConfig{
			Path:   "stdout",
			Format: "$remote_addr - $remote_user [$time_local] \"$request\" $status $body_bytes_sent \"$http_referer\" \"$http_user_agent\" $request_time",
		},
		Error: config.ErrorLogConfig{
			Path:  "stdout",
			Level: "info",
		},
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/v1/resources?name=test&limit=10")
	ctx.Request.Header.SetMethod("PUT")
	ctx.Request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Benchmark/1.0)")
	ctx.Request.Header.Set("Referer", "https://example.com/dashboard")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = logger.formatAccessLog(ctx, 200, 2048, 250*time.Microsecond)
	}
}

// BenchmarkFormatAccessLogMinimal 测试最小模板的 formatAccessLog 性能。
// 用于评估变量系统的基准开销。
func BenchmarkFormatAccessLogMinimal(b *testing.B) {
	logger := New(&config.LoggingConfig{
		Access: config.AccessLogConfig{
			Path:   "stdout",
			Format: "$status",
		},
		Error: config.ErrorLogConfig{
			Path:  "stdout",
			Level: "info",
		},
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/ping")
	ctx.Request.Header.SetMethod("GET")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = logger.formatAccessLog(ctx, 200, 4, 1*time.Microsecond)
	}
}

// BenchmarkParseLevel 测试日志级别解析性能。
// 在初始化时调用，预期极快且零分配。
func BenchmarkParseLevel(b *testing.B) {
	levels := []string{"debug", "info", "warn", "error", "DEBUG", "INFO", "WARN", "ERROR", "unknown", ""}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = parseLevel(levels[i%len(levels)])
	}
}

// BenchmarkParseLevelLowercase 测试小写日志级别解析性能。
// 最常见的输入场景，应该是最快路径。
func BenchmarkParseLevelLowercase(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = parseLevel("info")
	}
}

// BenchmarkParseLevelUppercase 测试大写日志级别解析性能。
// 测试 strings.ToLower 的开销。
func BenchmarkParseLevelUppercase(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = parseLevel("INFO")
	}
}

// BenchmarkLoggerLogAccessWithUser 测试带有用户认证的访问日志性能。
// 额外的 ctx.UserValue 查找会增加开销。
func BenchmarkLoggerLogAccessWithUser(b *testing.B) {
	logger := New(&config.LoggingConfig{
		Access: config.AccessLogConfig{
			Path:   "stdout",
			Format: "$remote_addr - $remote_user [$time_local] \"$request\" $status $body_bytes_sent",
		},
		Error: config.ErrorLogConfig{
			Path:  "stdout",
			Level: "info",
		},
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/admin/dashboard")
	ctx.Request.Header.SetMethod("GET")
	ctx.SetUserValue("remote_user", "admin_user_12345")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.LogAccess(ctx, 200, 1024, 100*time.Millisecond)
	}
}

// BenchmarkLoggerLogAccessEmptyFormat 测试空格式（默认 JSON）的访问日志性能。
// 验证空字符串回退到 JSON 的性能。
func BenchmarkLoggerLogAccessEmptyFormat(b *testing.B) {
	logger := New(&config.LoggingConfig{
		Access: config.AccessLogConfig{
			Path:   "stdout",
			Format: "",
		},
		Error: config.ErrorLogConfig{
			Path:  "stdout",
			Level: "info",
		},
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/health")
	ctx.Request.Header.SetMethod("GET")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.LogAccess(ctx, 200, 2, 5*time.Microsecond)
	}
}
