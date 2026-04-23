//go:build e2e

// Package testutil 提供 E2E 测试的工具函数。
//
// 包含测试常量定义。
//
// 作者：xfy
package testutil

import (
	"crypto/tls"
	"time"
)

// 测试超时常量。
const (
	// ContainerStartupTimeout 容器启动超时。
	ContainerStartupTimeout = 30 * time.Second

	// HealthCheckWaitTimeout 健康检查等待超时。
	HealthCheckWaitTimeout = 30 * time.Second

	// HealthCheckDetectionTime 健康检查检测时间。
	HealthCheckDetectionTime = 10 * time.Second

	// CacheExpireBuffer 缓存过期缓冲时间。
	CacheExpireBuffer = 1 * time.Second

	// DefaultTestTimeout 测试上下文超时。
	DefaultTestTimeout = 180 * time.Second

	// DefaultClientTimeout HTTP 客户端超时。
	DefaultClientTimeout = 10 * time.Second

	// ConcurrentRequestTimeout 并发请求超时。
	ConcurrentRequestTimeout = 30 * time.Second

	// ShortTestTimeout 短测试超时（用于快速测试）。
	ShortTestTimeout = 60 * time.Second

	// MediumTestTimeout 中等测试超时。
	MediumTestTimeout = 120 * time.Second
)

// 测试配置常量。
const (
	// DefaultBackendCount 默认后端数量。
	DefaultBackendCount = 2

	// DefaultConcurrentRequests 并发请求数量。
	DefaultConcurrentRequests = 10

	// HighConcurrentRequests 高并发请求数量。
	HighConcurrentRequests = 20

	// CacheTestMaxAge 缓存测试过期时间。
	CacheTestMaxAge = 5 * time.Minute

	// CacheTestShortMaxAge 短缓存过期时间（用于过期测试）。
	CacheTestShortMaxAge = 2 * time.Second
)

// TLS 版本常量（用于配置客户端）。
const (
	// TLSVersion12 TLS 1.2。
	TLSVersion12 = tls.VersionTLS12

	// TLSVersion13 TLS 1.3。
	TLSVersion13 = tls.VersionTLS13
)
