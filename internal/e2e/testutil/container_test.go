//go:build e2e

// Package testutil 提供 E2E 测试的工具函数。
//
// 包含容器管理、测试配置、等待工具等。
//
// 作者：xfy
package testutil

import (
	"context"
	"testing"
	"time"
)

// TestContainerSetup 测试容器基础设施。
func TestContainerSetup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 检查 Docker 是否可用
	if !DockerAvailable(ctx) {
		t.Skip("Docker not available, skipping E2E tests")
	}

	t.Log("Docker is available for E2E tests")
}

// TestDockerAvailable 测试 Docker 可用性检查。
func TestDockerAvailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	available := DockerAvailable(ctx)
	t.Logf("Docker available: %v", available)

	// 不强制要求 Docker 可用，只是报告状态
}

// TestMockBackendContainer 测试模拟后端容器启动。
func TestMockBackendContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if !DockerAvailable(ctx) {
		t.Skip("Docker not available")
	}

	container, addr, err := MockBackendContainer(ctx, 80)
	if err != nil {
		t.Fatalf("Failed to start mock backend: %v", err)
	}
	defer container.Terminate(ctx)

	t.Logf("Mock backend started at: %s", addr)

	if addr == "" {
		t.Error("Expected non-empty address")
	}
}
