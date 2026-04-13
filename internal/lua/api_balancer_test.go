// Package lua 提供 ngx.balancer API 的测试
//
// 该文件测试负载均衡相关的 Lua API，包括：
//   - BalancerContext 创建和管理
//   - set_current_peer API
//   - set_more_tries API
//   - get_last_failure API
//   - get_targets API
//   - get_client_ip API
//   - IsSelected 边界测试
//
// 作者：xfy
package lua

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"rua.plus/lolly/internal/loadbalance"
)

// TestBalancerContext_IsSelected 测试 IsSelected 方法
func TestBalancerContext_IsSelected(t *testing.T) {
	tests := []struct {
		name       string
		selected   bool
		wantResult bool
	}{
		{
			name:       "已选择目标",
			selected:   true,
			wantResult: true,
		},
		{
			name:       "未选择目标",
			selected:   false,
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bctx := &BalancerContext{
				Targets: []*loadbalance.Target{
					{URL: "http://backend1:8080"},
					{URL: "http://backend2:8080"},
				},
				ClientIP: "127.0.0.1",
				Retries:  3,
				selected: tt.selected,
			}

			if tt.selected {
				bctx.Selected = bctx.Targets[0]
			}

			result := bctx.IsSelected()
			assert.Equal(t, tt.wantResult, result, "IsSelected() should return expected value")
		})
	}
}

// TestBalancerContext_IsSelected_ZeroValue 测试零值情况
func TestBalancerContext_IsSelected_ZeroValue(t *testing.T) {
	// 零值的 BalancerContext
	bctx := &BalancerContext{}

	// 默认应该返回 false
	result := bctx.IsSelected()
	assert.False(t, result, "Zero value BalancerContext should return false for IsSelected()")
}

// TestBalancerContext_IsSelected_AfterSelection 测试选择后的状态
func TestBalancerContext_IsSelected_AfterSelection(t *testing.T) {
	targets := []*loadbalance.Target{
		{URL: "http://backend1:8080"},
		{URL: "http://backend2:8080"},
	}

	bctx := &BalancerContext{
		Targets:  targets,
		ClientIP: "192.168.1.1",
		Retries:  3,
	}

	// 初始状态
	assert.False(t, bctx.IsSelected(), "Should not be selected initially")

	// 模拟选择目标
	bctx.Selected = targets[0]
	bctx.selected = true

	// 选择后状态
	assert.True(t, bctx.IsSelected(), "Should be selected after setting")

	// 清除选择
	bctx.selected = false
	assert.False(t, bctx.IsSelected(), "Should not be selected after clearing flag")
}

// TestClassifyError 测试错误分类函数
func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "timeout error",
			err:      errors.New("connection timeout"),
			expected: "timeout",
		},
		{
			name:     "connection error",
			err:      errors.New("connection refused"),
			expected: "failed",
		},
		{
			name:     "generic error",
			err:      errors.New("some error"),
			expected: "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBalancerContext_Structure 测试 BalancerContext 结构体字段
func TestBalancerContext_Structure(t *testing.T) {
	targets := []*loadbalance.Target{
		{URL: "http://backend1:8080", Weight: 1},
		{URL: "http://backend2:8080", Weight: 2},
	}

	bctx := &BalancerContext{
		Targets:   targets,
		ClientIP:  "10.0.0.1",
		Retries:   5,
		Selected:  nil,
		LastError: nil,
		selected:  false,
	}

	assert.Equal(t, 2, len(bctx.Targets))
	assert.Equal(t, "10.0.0.1", bctx.ClientIP)
	assert.Equal(t, 5, bctx.Retries)
	assert.Nil(t, bctx.Selected)
	assert.Nil(t, bctx.LastError)
	assert.False(t, bctx.selected)
}
