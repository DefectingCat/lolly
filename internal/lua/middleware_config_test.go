// Package lua 提供 Lua 中间件配置测试
package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParsePhase 测试阶段解析
func TestParsePhase(t *testing.T) {
	tests := []struct {
		input    string
		expected Phase
		hasError bool
	}{
		{"rewrite", PhaseRewrite, false},
		{"access", PhaseAccess, false},
		{"content", PhaseContent, false},
		{"log", PhaseLog, false},
		{"header_filter", PhaseHeaderFilter, false},
		{"body_filter", PhaseBodyFilter, false},
		{"invalid", PhaseInit, true},
		{"", PhaseInit, true},
	}

	for _, tt := range tests {
		phase, err := ParsePhase(tt.input)
		if tt.hasError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tt.expected, phase)
		}
	}
}
