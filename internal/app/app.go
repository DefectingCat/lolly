// Package app 提供应用程序的启动和运行逻辑。
package app

import (
	"fmt"
	"os"

	"rua.plus/lolly/internal/config"
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

	// TODO: 启动服务器
	fmt.Println("服务器启动中...")
	return 0
}