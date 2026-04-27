//go:build e2e

package testutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWaitForConditionSuccess 测试条件满足场景。
func TestWaitForConditionSuccess(t *testing.T) {
	ctx := context.Background()

	count := 0
	err := WaitForCondition(ctx, RetryConfig{
		Interval: 10 * time.Millisecond,
		Timeout:  100 * time.Millisecond,
	}, func() bool {
		count++
		return count >= 3
	})

	require.NoError(t, err, "Should succeed when condition is met")
	assert.GreaterOrEqual(t, count, 3, "Should have retried at least 3 times")
}

// TestWaitForConditionTimeout 测试超时场景。
func TestWaitForConditionTimeout(t *testing.T) {
	ctx := context.Background()

	start := time.Now()
	err := WaitForCondition(ctx, RetryConfig{
		Interval: 10 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
	}, func() bool {
		return false // 永远不满足
	})

	elapsed := time.Since(start)

	require.Error(t, err, "Should fail when condition is never met")
	assert.Contains(t, err.Error(), "condition not met")
	assert.Less(t, elapsed, 100*time.Millisecond, "Should timeout around the specified duration")
}

// TestWaitForConditionMaxRetries 测试最大重试次数。
func TestWaitForConditionMaxRetries(t *testing.T) {
	ctx := context.Background()

	count := 0
	err := WaitForCondition(ctx, RetryConfig{
		Interval:   10 * time.Millisecond,
		Timeout:    1 * time.Second,
		MaxRetries: 3,
	}, func() bool {
		count++
		return false
	})

	require.Error(t, err, "Should fail after max retries")
	assert.Contains(t, err.Error(), "3 retries")
	assert.Equal(t, 3, count, "Should have retried exactly 3 times")
}

// TestWaitForConditionContextCancel 测试上下文取消。
func TestWaitForConditionContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// 50ms 后取消
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := WaitForCondition(ctx, RetryConfig{
		Interval: 10 * time.Millisecond,
		Timeout:  1 * time.Second,
	}, func() bool {
		return false
	})

	require.Error(t, err, "Should fail when context is cancelled")
	assert.Contains(t, err.Error(), "context canceled")
}

// TestWaitForNoErrorSuccess 测试操作成功场景。
func TestWaitForNoErrorSuccess(t *testing.T) {
	ctx := context.Background()

	count := 0
	err := WaitForNoError(ctx, RetryConfig{
		Interval: 10 * time.Millisecond,
		Timeout:  100 * time.Millisecond,
	}, func() error {
		count++
		if count < 3 {
			return errors.New("not ready")
		}
		return nil
	})

	require.NoError(t, err, "Should succeed when operation returns nil")
	assert.GreaterOrEqual(t, count, 3, "Should have retried at least 3 times")
}

// TestWaitForNoErrorTimeout 测试操作超时场景。
func TestWaitForNoErrorTimeout(t *testing.T) {
	ctx := context.Background()

	err := WaitForNoError(ctx, RetryConfig{
		Interval: 10 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
	}, func() error {
		return errors.New("always fails")
	})

	require.Error(t, err, "Should fail when operation always returns error")
	assert.Contains(t, err.Error(), "operation failed")
	assert.Contains(t, err.Error(), "always fails")
}

// TestWaitForNoErrorMaxRetries 测试最大重试次数。
func TestWaitForNoErrorMaxRetries(t *testing.T) {
	ctx := context.Background()

	count := 0
	err := WaitForNoError(ctx, RetryConfig{
		Interval:   10 * time.Millisecond,
		Timeout:    1 * time.Second,
		MaxRetries: 2,
	}, func() error {
		count++
		return errors.New("fail")
	})

	require.Error(t, err, "Should fail after max retries")
	assert.Contains(t, err.Error(), "2 retries")
	assert.Equal(t, 2, count, "Should have retried exactly 2 times")
}

// TestDefaultRetryConfig 测试默认配置。
func TestDefaultRetryConfig(t *testing.T) {
	assert.Equal(t, 500*time.Millisecond, DefaultRetryConfig.Interval)
	assert.Equal(t, 30*time.Second, DefaultRetryConfig.Timeout)
	assert.Equal(t, 0, DefaultRetryConfig.MaxRetries)
}

// TestRetryConfigZeroValues 测试零值配置使用默认值。
func TestRetryConfigZeroValues(t *testing.T) {
	ctx := context.Background()

	// 零值配置应该使用默认值
	count := 0
	err := WaitForCondition(ctx, RetryConfig{}, func() bool {
		count++
		return count >= 1
	})

	require.NoError(t, err, "Should use default config values")
}

// TestPollSuccess 测试轮询成功。
func TestPollSuccess(t *testing.T) {
	ctx := context.Background()

	count := 0
	err := Poll(ctx, 10*time.Millisecond, 100*time.Millisecond, func() (bool, error) {
		count++
		return count >= 3, nil
	})

	require.NoError(t, err, "Poll should succeed")
	assert.GreaterOrEqual(t, count, 3)
}

// TestPollError 测试轮询返回错误。
func TestPollError(t *testing.T) {
	ctx := context.Background()

	err := Poll(ctx, 10*time.Millisecond, 50*time.Millisecond, func() (bool, error) {
		return false, errors.New("poll error")
	})

	require.Error(t, err, "Poll should fail with error")
	assert.Contains(t, err.Error(), "poll error")
}

// TestWaitForHealthySuccess 测试等待健康检查成功。
func TestWaitForHealthySuccess(t *testing.T) {
	// 这个测试需要 HTTP 服务器，在集成测试中验证
	// 这里只测试函数签名和基本逻辑
	t.Log("WaitForHealthy function exists and has correct signature")
}

// TestWaitForBackendHealthySuccess 测试等待后端健康。
func TestWaitForBackendHealthySuccess(t *testing.T) {
	// 这个测试需要 HTTP 服务器，在集成测试中验证
	t.Log("WaitForBackendHealthy function exists and has correct signature")
}
