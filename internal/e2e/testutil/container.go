//go:build e2e

// Package testutil 提供 E2E 测试的工具函数。
//
// 包含容器管理、测试配置、等待工具等。
//
// 作者：xfy
package testutil

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// defaultLollyConfig 是 lolly 容器的默认配置。
const defaultLollyConfig = `
servers:
  - listen: ":8080"
    static:
      - path: "/"
        root: "/var/www/html"
        index:
          - "index.html"
`

// LollyContainer 封装 lolly 服务器容器。
type LollyContainer struct {
	Container testcontainers.Container
	Host      string
	HTTPPort  int
	HTTPSPort int
}

// StartLollyContainer 启动 lolly 服务器容器。
//
// 使用预构建的 lolly 镜像。如果 configPath 为空，使用默认配置。
func StartLollyContainer(ctx context.Context, configPath string) (*LollyContainer, error) {
	req := testcontainers.ContainerRequest{
		Image:        "lolly:latest",
		ExposedPorts: []string{"8080/tcp", "8443/tcp"},
		WaitingFor:   wait.ForLog("HTTP 服务器启动中").WithStartupTimeout(30 * time.Second),
	}

	// 配置文件挂载
	if configPath != "" {
		req.Mounts = []testcontainers.ContainerMount{
			{
				Source: testcontainers.GenericBindMountSource{
					HostPath: configPath,
				},
				Target: "/etc/lolly/lolly.yaml",
			},
		}
	} else {
		// 使用内嵌默认配置
		req.Files = []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(defaultLollyConfig),
				ContainerFilePath: "/etc/lolly/lolly.yaml",
				FileMode:          0o644,
			},
		}
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get host: %w", err)
	}

	httpPort, err := container.MappedPort(ctx, "8080/tcp")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get HTTP port: %w", err)
	}

	httpsPort, err := container.MappedPort(ctx, "8443/tcp")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get HTTPS port: %w", err)
	}

	// 解析端口数字
	httpPortInt := parsePort(httpPort.Port())
	httpsPortInt := parsePort(httpsPort.Port())

	return &LollyContainer{
		Container: container,
		Host:      host,
		HTTPPort:  httpPortInt,
		HTTPSPort: httpsPortInt,
	}, nil
}

// parsePort 解析端口字符串为整数。
func parsePort(portStr string) int {
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}

// HTTPBaseURL 返回 HTTP 基础 URL。
func (c *LollyContainer) HTTPBaseURL() string {
	return fmt.Sprintf("http://%s:%d", c.Host, c.HTTPPort)
}

// HTTPSBaseURL 返回 HTTPS 基础 URL。
func (c *LollyContainer) HTTPSBaseURL() string {
	return fmt.Sprintf("https://%s:%d", c.Host, c.HTTPSPort)
}

// Terminate 终止容器。
func (c *LollyContainer) Terminate(ctx context.Context) error {
	if c.Container != nil {
		return c.Container.Terminate(ctx)
	}
	return nil
}

// WaitForHealthy 等待服务健康。
func (c *LollyContainer) WaitForHealthy(ctx context.Context, timeout time.Duration) error {
	client := &http.Client{Timeout: 5 * time.Second}
	url := c.HTTPBaseURL()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 404 {
				// 200 或 404 都表示服务器正在运行
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("service not healthy after %v", timeout)
}

// MockBackendContainer 启动一个模拟后端服务器容器。
func MockBackendContainer(ctx context.Context, port int) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "nginx:alpine",
		ExposedPorts: []string{fmt.Sprintf("%d/tcp", port)},
		WaitingFor:   wait.ForHTTP("/").WithPort(fmt.Sprintf("%d/tcp", port)).WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to start mock backend: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get host: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, fmt.Sprintf("%d/tcp", port))
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get port: %w", err)
	}

	addr := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	return container, addr, nil
}

// DockerAvailable 检查 Docker 是否可用。
func DockerAvailable(ctx context.Context) bool {
	req := testcontainers.ContainerRequest{
		Image:      "alpine:latest",
		Cmd:        []string{"echo", "test"},
		AutoRemove: true,
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return false
	}
	container.Terminate(ctx)
	return true
}

// LollyImageAvailable 检查 lolly:latest 镜像是否可用。
func LollyImageAvailable(ctx context.Context) bool {
	req := testcontainers.ContainerRequest{
		Image:      "lolly:latest",
		Cmd:        []string{"echo", "test"},
		AutoRemove: true,
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return false
	}
	container.Terminate(ctx)
	return true
}

// StartNginxContainer 启动 nginx 容器，返回容器和访问地址。
func StartNginxContainer(ctx context.Context) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "nginx:alpine",
		ExposedPorts: []string{"80/tcp"},
		WaitingFor:   wait.ForHTTP("/").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to start nginx container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get host: %w", err)
	}

	port, err := container.MappedPort(ctx, "80/tcp")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get port: %w", err)
	}

	addr := fmt.Sprintf("http://%s:%s", host, port.Port())
	return container, addr, nil
}
