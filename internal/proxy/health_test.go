// Package proxy 提供健康检查的测试。
package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/loadbalance"
)

// TestNewHealthChecker 测试 NewHealthChecker 函数。
func TestNewHealthChecker(t *testing.T) {
	t.Run("默认值应用", func(t *testing.T) {
		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080", Healthy: true},
		}
		cfg := &config.HealthCheckConfig{}

		checker := NewHealthChecker(targets, cfg)

		if checker.GetInterval() != 10*time.Second {
			t.Errorf("Interval = %v, want %v", checker.GetInterval(), 10*time.Second)
		}
		if checker.GetTimeout() != 5*time.Second {
			t.Errorf("Timeout = %v, want %v", checker.GetTimeout(), 5*time.Second)
		}
		if checker.GetPath() != "/health" {
			t.Errorf("Path = %q, want %q", checker.GetPath(), "/health")
		}
		if checker.IsRunning() {
			t.Error("新建的 checker 应未启动")
		}
	})

	t.Run("自定义配置", func(t *testing.T) {
		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080", Healthy: true},
			{URL: "http://backend2:8080", Healthy: true},
		}
		cfg := &config.HealthCheckConfig{
			Interval: 30 * time.Second,
			Timeout:  10 * time.Second,
			Path:     "/status",
		}

		checker := NewHealthChecker(targets, cfg)

		if checker.GetInterval() != 30*time.Second {
			t.Errorf("Interval = %v, want %v", checker.GetInterval(), 30*time.Second)
		}
		if checker.GetTimeout() != 10*time.Second {
			t.Errorf("Timeout = %v, want %v", checker.GetTimeout(), 10*time.Second)
		}
		if checker.GetPath() != "/status" {
			t.Errorf("Path = %q, want %q", checker.GetPath(), "/status")
		}
	})

	t.Run("负值配置使用默认值", func(t *testing.T) {
		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080", Healthy: true},
		}
		cfg := &config.HealthCheckConfig{
			Interval: -1 * time.Second,
			Timeout:  -1 * time.Second,
		}

		checker := NewHealthChecker(targets, cfg)

		if checker.GetInterval() != 10*time.Second {
			t.Errorf("负值 Interval 应使用默认值，got %v", checker.GetInterval())
		}
		if checker.GetTimeout() != 5*time.Second {
			t.Errorf("负值 Timeout 应使用默认值，got %v", checker.GetTimeout())
		}
	})

	t.Run("零值配置使用默认值", func(t *testing.T) {
		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080", Healthy: true},
		}
		cfg := &config.HealthCheckConfig{
			Interval: 0,
			Timeout:  0,
			Path:     "",
		}

		checker := NewHealthChecker(targets, cfg)

		if checker.GetInterval() != 10*time.Second {
			t.Errorf("零值 Interval 应使用默认值，got %v", checker.GetInterval())
		}
		if checker.GetTimeout() != 5*time.Second {
			t.Errorf("零值 Timeout 应使用默认值，got %v", checker.GetTimeout())
		}
		if checker.GetPath() != "/health" {
			t.Errorf("空 Path 应使用默认值，got %q", checker.GetPath())
		}
	})
}

// TestHealthCheckerStartStop 测试 Start 和 Stop 方法。
func TestHealthCheckerStartStop(t *testing.T) {
	t.Run("启动和停止", func(t *testing.T) {
		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080", Healthy: true},
		}
		cfg := &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     "/health",
		}

		checker := NewHealthChecker(targets, cfg)

		if checker.IsRunning() {
			t.Error("启动前 IsRunning 应返回 false")
		}

		checker.Start()

		if !checker.IsRunning() {
			t.Error("启动后 IsRunning 应返回 true")
		}

		checker.Stop()

		if checker.IsRunning() {
			t.Error("停止后 IsRunning 应返回 false")
		}
	})

	t.Run("重复启动无效果", func(t *testing.T) {
		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080", Healthy: true},
		}
		cfg := &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
		}

		checker := NewHealthChecker(targets, cfg)

		checker.Start()
		checker.Start()

		if !checker.IsRunning() {
			t.Error("重复启动后 checker 应仍在运行")
		}

		checker.Stop()
	})

	t.Run("重复停止无效果", func(t *testing.T) {
		targets := []*loadbalance.Target{
			{URL: "http://backend1:8080", Healthy: true},
		}
		cfg := &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
		}

		checker := NewHealthChecker(targets, cfg)

		checker.Stop()
		checker.Stop()

		if checker.IsRunning() {
			t.Error("未启动时停止，checker 应不在运行")
		}
	})
}

// TestCheckTarget 测试 checkTarget 方法。
func TestCheckTarget(t *testing.T) {
	t.Run("健康响应", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				t.Errorf("请求路径 = %q, want %q", r.URL.Path, "/health")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		target := &loadbalance.Target{
			URL:     server.URL,
			Healthy: false,
		}

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     "/health",
		})

		checker.checkTarget(target)

		if !target.Healthy {
			t.Error("健康响应后 target 应标记为 healthy")
		}
	})

	t.Run("不健康响应", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		target := &loadbalance.Target{
			URL:     server.URL,
			Healthy: true,
		}

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     "/health",
		})

		checker.checkTarget(target)

		if target.Healthy {
			t.Error("5xx 响应后 target 应标记为 unhealthy")
		}
	})

	t.Run("超时", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
		}))
		defer server.Close()

		target := &loadbalance.Target{
			URL:     server.URL,
			Healthy: true,
		}

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  10 * time.Millisecond,
			Path:     "/health",
		})

		checker.checkTarget(target)

		if target.Healthy {
			t.Error("超时后 target 应标记为 unhealthy")
		}
	})

	t.Run("连接失败", func(t *testing.T) {
		target := &loadbalance.Target{
			URL:     "http://invalid-host-that-does-not-exist:99999",
			Healthy: true,
		}

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  100 * time.Millisecond,
			Path:     "/health",
		})

		checker.checkTarget(target)

		if target.Healthy {
			t.Error("连接失败后 target 应标记为 unhealthy")
		}
	})

	t.Run("3xx 重定向响应", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMovedPermanently)
		}))
		defer server.Close()

		target := &loadbalance.Target{
			URL:     server.URL,
			Healthy: true,
		}

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     "/health",
		})

		checker.checkTarget(target)

		if target.Healthy {
			t.Error("3xx 响应后 target 应标记为 unhealthy")
		}
	})

	t.Run("4xx 客户端错误响应", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		target := &loadbalance.Target{
			URL:     server.URL,
			Healthy: true,
		}

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     "/health",
		})

		checker.checkTarget(target)

		if target.Healthy {
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
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tt.statusCode)
				}))
				defer server.Close()

				target := &loadbalance.Target{
					URL:     server.URL,
					Healthy: false,
				}

				checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
					Interval: 1 * time.Hour,
					Timeout:  5 * time.Second,
					Path:     "/health",
				})

				checker.checkTarget(target)

				if !target.Healthy {
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
			URL:     "http://backend1:8080",
			Healthy: true,
		}

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     "/health",
		})

		checker.MarkUnhealthy(target)

		if target.Healthy {
			t.Error("MarkUnhealthy 后 target 应标记为 unhealthy")
		}
	})

	t.Run("已不健康的 target 再次标记", func(t *testing.T) {
		target := &loadbalance.Target{
			URL:     "http://backend1:8080",
			Healthy: false,
		}

		checker := NewHealthChecker([]*loadbalance.Target{target}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     "/health",
		})

		checker.MarkUnhealthy(target)

		if target.Healthy {
			t.Error("MarkUnhealthy 后 target 应保持 unhealthy 状态")
		}
	})

	t.Run("多 target 场景", func(t *testing.T) {
		target1 := &loadbalance.Target{
			URL:     "http://backend1:8080",
			Healthy: true,
		}
		target2 := &loadbalance.Target{
			URL:     "http://backend2:8080",
			Healthy: true,
		}

		checker := NewHealthChecker([]*loadbalance.Target{target1, target2}, &config.HealthCheckConfig{
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
			Path:     "/health",
		})

		checker.MarkUnhealthy(target1)

		if target1.Healthy {
			t.Error("target1 应标记为 unhealthy")
		}
		if !target2.Healthy {
			t.Error("target2 应保持 healthy")
		}
	})
}
