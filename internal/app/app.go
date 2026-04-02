// Package app 提供应用程序的启动和运行逻辑。
package app

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/server"
)

// 版本信息，通过 -ldflags 注入。
var (
	Version       = "dev"
	GitCommit     = "unknown"
	GitBranch     = "unknown"
	BuildTime     = "unknown"
	GoVersion     = "unknown"
	BuildPlatform = "unknown"
)

// 应用状态。
var (
	shutdownTimeout = 30 * time.Second // 优雅停止超时时间
)

// Run 应用程序入口。
func Run(cfgPath string, genConfig bool, outputPath string, showVersion bool) int {
	if genConfig {
		return generateConfig(outputPath)
	}

	if showVersion {
		printVersion()
		return 0
	}

	return startServer(cfgPath)
}

// generateConfig 生成默认配置文件。
func generateConfig(outputPath string) int {
	cfg := config.DefaultConfig()
	yamlData, err := config.GenerateConfigYAML(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成配置失败: %v\n", err)
		return 1
	}

	if outputPath == "" {
		fmt.Print(string(yamlData))
	} else {
		if err := os.WriteFile(outputPath, yamlData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "写入文件失败: %v\n", err)
			return 1
		}
		fmt.Printf("配置已写入: %s\n", outputPath)
	}
	return 0
}

// printVersion 打印版本信息。
func printVersion() {
	fmt.Printf("lolly version %s\n", Version)
	fmt.Printf("  Git: %s (%s)\n", GitCommit, GitBranch)
	fmt.Printf("  Built: %s\n", BuildTime)
	fmt.Printf("  Go: %s\n", GoVersion)
	fmt.Printf("  Platform: %s\n", BuildPlatform)
}

// startServer 启动服务器。
func startServer(cfgPath string) int {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		return 1
	}

	fmt.Printf("配置加载成功: %s\n", cfgPath)
	fmt.Printf("监听地址: %s\n", cfg.Server.Listen)

	// 创建服务器
	srv := server.New(cfg)

	// 启动信号监听
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGTERM, // 快速停止（kill 或 systemd stop）
		syscall.SIGINT,  // 快速停止（Ctrl+C）
		syscall.SIGQUIT, // 优雅停止
	)

	// 启动服务器（在 goroutine 中）
	errChan := make(chan error, 1)
	go func() {
		fmt.Println("服务器启动中...")
		if err := srv.Start(); err != nil {
			errChan <- err
		}
	}()

	// 等待信号或启动错误
	select {
	case err := <-errChan:
		fmt.Fprintf(os.Stderr, "服务器启动失败: %v\n", err)
		return 1
	case sig := <-sigChan:
		// 根据信号类型决定停止方式
		switch sig {
		case syscall.SIGQUIT:
			// 优雅停止：等待请求完成
			fmt.Printf("\n收到 SIGQUIT，优雅停止（等待 %v）...\n", shutdownTimeout)
			srv.GracefulStop(shutdownTimeout)
		case syscall.SIGTERM, syscall.SIGINT:
			// 快速停止
			fmt.Printf("\n收到 %v，停止服务器...\n", sigName(sig.(syscall.Signal)))
			srv.Stop()
		}
	}

	fmt.Println("服务器已停止")
	return 0
}

// sigName 返回信号名称（用于日志输出）。
func sigName(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGQUIT:
		return "SIGQUIT"
	default:
		return fmt.Sprintf("Signal(%d)", sig)
	}
}