// Package limitrate 提供响应速率限制中间件的测试。
//
// 该文件测试速率限制模块的各项功能，包括：
//   - 常量值
//
// 作者：xfy
package limitrate

import "testing"

// TestConstants 测试常量值。
func TestConstants(t *testing.T) {
	if LargeFileStrategySkip != "skip" {
		t.Errorf("LargeFileStrategySkip = %q, want %q", LargeFileStrategySkip, "skip")
	}
	if LargeFileStrategyCoarse != "coarse" {
		t.Errorf("LargeFileStrategyCoarse = %q, want %q", LargeFileStrategyCoarse, "coarse")
	}
}
