//go:build e2e

// Package testutil 提供 E2E 测试的工具函数。
//
// 包含重试和等待工具，提高测试稳定性。
//
// 作者：xfy
package testutil

import (
	"context"
	"fmt"
	"time"
)

// RetryConfig 重试配置。
type RetryConfig struct {
	// Interval 重试间隔
	Interval time.Duration
	// Timeout 总超时时间
	Timeout time.Duration
	// MaxRetries 最大重试次数（0 表示无限制）
	MaxRetries int
}

// DefaultRetryConfig 默认重试配置。
var DefaultRetryConfig = RetryConfig{
	Interval:   500 * time.Millisecond,
	Timeout:    30 * time.Second,
	MaxRetries: 0, // 无限制
}

// WaitForCondition 等待条件满足。
//
// 定期检查条件函数，直到返回 true 或超时。
// 使用默认配置，可通过 opts 覆盖。
//
// 使用示例：
//
//	err := testutil.WaitForCondition(ctx, testutil.RetryConfig{
//	    Interval: 1 * time.Second,
//	    Timeout:  30 * time.Second,
//	}, func() bool {
//	    resp, err := client.Get(url)
//	    if err != nil {
//	        return false
//	    }
//	    defer resp.Body.Close()
//	    return resp.StatusCode == 200
//	})
func WaitForCondition(ctx context.Context, cfg RetryConfig, condition func() bool) error {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultRetryConfig.Interval
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultRetryConfig.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	retries := 0
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("condition not met after %v: %w", cfg.Timeout, ctx.Err())
		case <-ticker.C:
			if condition() {
				return nil
			}
			retries++
			if cfg.MaxRetries > 0 && retries >= cfg.MaxRetries {
				return fmt.Errorf("condition not met after %d retries", retries)
			}
		}
	}
}

// WaitForNoError 等待操作无错误。
//
// 定期执行函数，直到返回 nil 或超时。
// 适用于需要等待某个操作成功的场景。
//
// 使用示例：
//
//	err := testutil.WaitForNoError(ctx, testutil.RetryConfig{
//	    Interval: 2 * time.Second,
//	    Timeout:  60 * time.Second,
//	}, func() error {
//	    resp, err := client.Get(url)
//	    if err != nil {
//	        return err
//	    }
//	    defer resp.Body.Close()
//	    if resp.StatusCode != 200 {
//	        return fmt.Errorf("unexpected status: %d", resp.StatusCode)
//	    }
//	    return nil
//	})
func WaitForNoError(ctx context.Context, cfg RetryConfig, fn func() error) error {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultRetryConfig.Interval
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultRetryConfig.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	retries := 0
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("operation failed after %v: %w (last error: %v)", cfg.Timeout, ctx.Err(), lastErr)
			}
			return fmt.Errorf("operation failed after %v: %w", cfg.Timeout, ctx.Err())
		case <-ticker.C:
			if err := fn(); err == nil {
				return nil
			} else {
				lastErr = err
			}
			retries++
			if cfg.MaxRetries > 0 && retries >= cfg.MaxRetries {
				if lastErr != nil {
					return fmt.Errorf("operation failed after %d retries: %w", retries, lastErr)
				}
				return fmt.Errorf("operation failed after %d retries", retries)
			}
		}
	}
}

// Retry 重试操作直到成功或超时。
//
// 与 WaitForNoError 类似，但返回最后一次错误。
// 适用于需要知道具体失败原因的场景。
func Retry(ctx context.Context, cfg RetryConfig, fn func() error) error {
	return WaitForNoError(ctx, cfg, fn)
}

// WaitForHealthy 等待服务健康。
//
// 便捷函数，等待 HTTP 服务返回 200 或预期状态码。
//
// 使用示例：
//
//	err := testutil.WaitForHealthy(ctx, lolly.HTTPBaseURL(), 30*time.Second, 200, 404)
func WaitForHealthy(ctx context.Context, url string, timeout time.Duration, expectedCodes ...int) error {
	cfg := RetryConfig{
		Interval: 500 * time.Millisecond,
		Timeout:  timeout,
	}

	if len(expectedCodes) == 0 {
		expectedCodes = []int{200}
	}

	return WaitForNoError(ctx, cfg, func() error {
		client := CreateDefaultHTTPClient()
		resp, err := client.Get(url)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		for _, code := range expectedCodes {
			if resp.StatusCode == code {
				return nil
			}
		}

		return fmt.Errorf("unexpected status code: %d (expected one of %v)", resp.StatusCode, expectedCodes)
	})
}

// WaitForBackendHealthy 等待后端服务健康。
//
// 用于等待后端池中的服务就绪。
func WaitForBackendHealthy(ctx context.Context, urls []string, timeout time.Duration) error {
	cfg := RetryConfig{
		Interval: 500 * time.Millisecond,
		Timeout:  timeout,
	}

	return WaitForNoError(ctx, cfg, func() error {
		client := CreateDefaultHTTPClient()
		for _, url := range urls {
			resp, err := client.Get(url)
			if err != nil {
				return fmt.Errorf("backend %s not reachable: %w", url, err)
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				return fmt.Errorf("backend %s returned status %d", url, resp.StatusCode)
			}
		}
		return nil
	})
}

// Poll 定期执行函数直到返回 true。
//
// 简化的轮询接口，适用于简单场景。
func Poll(ctx context.Context, interval, timeout time.Duration, fn func() (bool, error)) error {
	cfg := RetryConfig{
		Interval: interval,
		Timeout:  timeout,
	}

	return WaitForNoError(ctx, cfg, func() error {
		done, err := fn()
		if err != nil {
			return err
		}
		if !done {
			return fmt.Errorf("poll condition not met")
		}
		return nil
	})
}
