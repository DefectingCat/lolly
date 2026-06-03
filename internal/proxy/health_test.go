// Package proxy 提供健康检查功能的测试。
//
// 该文件测试健康检查模块的各项功能，包括：
//   - 健康检查器创建
//   - 默认值应用
//   - 自定义配置
//   - 负值配置处理
//   - 零值配置处理
//   - 启动和停止控制
//   - 目标健康检查
//   - 超时处理
//   - 连接失败处理
//   - 标记不健康
//
// 作者：xfy
package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
)

// TestCheckTarget 测试 checkTarget 方法。
func TestCheckTarget(t *testing.T) {
	t.Run("健康响应", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != healthPath {
				t.Errorf("请求路径 = %q, want %q", r.URL.Path, healthPath)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		target := &loadbalance.Target{
			URL: server.URL,
		}
		target.Healthy.Store(false)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     healthPath,
		})

		checker.checkTarget(target)

		if !target.Healthy.Load() {
			t.Error("健康响应后 target 应标记为 healthy")
		}
	})

	t.Run("不健康响应", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		target := &loadbalance.Target{
			URL: server.URL,
		}
		target.Healthy.Store(true)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     healthPath,
		})

		checker.checkTarget(target)

		if target.Healthy.Load() {
			t.Error("5xx 响应后 target 应标记为 unhealthy")
		}
	})

	t.Run("超时", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
		}))
		defer server.Close()

		target := &loadbalance.Target{
			URL: server.URL,
		}
		target.Healthy.Store(true)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  10 * time.Millisecond,
			Path:     healthPath,
		})

		checker.checkTarget(target)

		if target.Healthy.Load() {
			t.Error("超时后 target 应标记为 unhealthy")
		}
	})

	t.Run("连接失败", func(t *testing.T) {
		target := &loadbalance.Target{
			URL: "http://invalid-host-that-does-not-exist:99999",
		}
		target.Healthy.Store(true)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  100 * time.Millisecond,
			Path:     healthPath,
		})

		checker.checkTarget(target)

		if target.Healthy.Load() {
			t.Error("连接失败后 target 应标记为 unhealthy")
		}
	})

	t.Run("3xx 重定向响应", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusMovedPermanently)
		}))
		defer server.Close()

		target := &loadbalance.Target{
			URL: server.URL,
		}
		target.Healthy.Store(true)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     healthPath,
		})

		checker.checkTarget(target)

		if target.Healthy.Load() {
			t.Error("3xx 响应后 target 应标记为 unhealthy")
		}
	})

	t.Run("4xx 客户端错误响应", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		target := &loadbalance.Target{
			URL: server.URL,
		}
		target.Healthy.Store(true)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     healthPath,
		})

		checker.checkTarget(target)

		if target.Healthy.Load() {
			t.Error("4xx 响应后 target 应标记为 unhealthy")
		}
	})

	t.Run("2xx 成功响应", func(t *testing.T) {
		tests := []struct {
			name       string
			statusCode int
		}{
			{"200 OK", http.StatusOK},
			{"201 Created", http.StatusCreated},
			{"204 No Content", http.StatusNoContent},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tt.statusCode)
				}))
				defer server.Close()

				target := &loadbalance.Target{
					URL: server.URL,
				}
				target.Healthy.Store(false)

				checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
					Interval: 1 * time.Hour,
					Timeout:  5 * time.Second,
					Path:     healthPath,
				})

				checker.checkTarget(target)

				if !target.Healthy.Load() {
					t.Errorf("%d 响应后 target 应标记为 healthy", tt.statusCode)
				}
			})
		}
	})
}

// TestMarkUnhealthy 测试 MarkUnhealthy 方法。
func TestMarkUnhealthy(t *testing.T) {
	t.Run("标记不健康", func(t *testing.T) {
		target := &loadbalance.Target{
			URL: "http://backend1:8080",
		}
		target.Healthy.Store(true)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     healthPath,
		})

		checker.MarkUnhealthy(target)

		if target.Healthy.Load() {
			t.Error("MarkUnhealthy 后 target 应标记为 unhealthy")
		}
	})

	t.Run("已不健康的 target 再次标记", func(t *testing.T) {
		target := &loadbalance.Target{
			URL: "http://backend1:8080",
		}
		target.Healthy.Store(false)

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     healthPath,
		})

		checker.MarkUnhealthy(target)

		if target.Healthy.Load() {
			t.Error("MarkUnhealthy 后 target 应保持 unhealthy 状态")
		}
	})

	t.Run("多 target 场景", func(t *testing.T) {
		target1 := &loadbalance.Target{
			URL: "http://backend1:8080",
		}
		target1.Healthy.Store(true)
		target2 := &loadbalance.Target{
			URL: "http://backend2:8080",
		}
		target2.Healthy.Store(true)

		checker := NewHealthChecker([]*loadbalance.Target{target1, target2}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     healthPath,
		})

		checker.MarkUnhealthy(target1)

		if target1.Healthy.Load() {
			t.Error("target1 应标记为 unhealthy")
		}
		if !target2.Healthy.Load() {
			t.Error("target2 应保持 healthy")
		}
	})
}

// TestMarkUnhealthy_WithSlowStartManager 测试 MarkUnhealthy 与 SlowStartManager 集成。
func TestMarkUnhealthy_WithSlowStartManager(t *testing.T) {
	target := &loadbalance.Target{
		URL:    "http://127.0.0.1:8080",
		Weight: 100,
	}
	target.Healthy.Store(true)
	target.SlowStart = 30 * time.Second

	checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
		Interval:  1 * time.Hour,
		Timeout:   5 * time.Second,
		Path:      "/health",
		SlowStart: 30 * time.Second,
	})

	// 先标记为健康以初始化慢启动
	checker.MarkHealthy(target)

	// 标记目标为不健康
	checker.MarkUnhealthy(target)

	if target.Healthy.Load() {
		t.Error("target 应标记为 unhealthy")
	}
}

// TestMarkHealthy_WithSlowStartManager 测试 MarkHealthy 与 SlowStartManager 集成。
func TestMarkHealthy_WithSlowStartManager(t *testing.T) {
	target := &loadbalance.Target{
		URL:    "http://127.0.0.1:8080",
		Weight: 100,
	}
	target.Healthy.Store(false)
	target.SlowStart = 30 * time.Second

	checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
		Interval:  1 * time.Hour,
		Timeout:   5 * time.Second,
		Path:      "/health",
		SlowStart: 30 * time.Second,
	})

	// 标记目标为健康
	checker.MarkHealthy(target)

	if !target.Healthy.Load() {
		t.Error("target 应标记为 healthy")
	}

	// 验证慢启动已开始（EffectiveWeight 应被设置为 1）
	ew := target.EffectiveWeight.Load()
	if ew <= 0 {
		t.Errorf("慢启动 EffectiveWeight 应大于 0，got: %d", ew)
	}
}
