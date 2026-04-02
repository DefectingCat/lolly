package main

import (
	"flag"
	"fmt"
	"os"

	"rua.plus/lolly/internal/config"
)

// 通过 -ldflags 注入的版本信息
var (
	version       = "dev"
	gitCommit     = "unknown"
	gitBranch     = "unknown"
	buildTime     = "unknown"
	goVersion     = "unknown"
	buildPlatform = "unknown"
)

// CLI 参数
var (
	cfgPath     = flag.String("c", "lolly.yaml", "配置文件路径")
	cfgPathLong = flag.String("config", "", "配置文件路径（长参数）")
	genConfig   = flag.Bool("generate-config", false, "生成默认配置")
	outputPath  = flag.String("o", "", "输出文件路径（配合 --generate-config）")
	showVersion = flag.Bool("v", false, "显示版本")
)

func main() {
	flag.Parse()

	// --generate-config 优先处理
	if *genConfig {
		handleGenerateConfig(*outputPath)
		return
	}

	// 版本显示
	if *showVersion {
		printVersion()
		return
	}

	// 合并短参数和长参数
	configPath := *cfgPath
	if *cfgPathLong != "" {
		configPath = *cfgPathLong
	}

	// 加载配置
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("配置加载成功: %s\n", configPath)
	fmt.Printf("监听地址: %s\n", cfg.Server.Listen)

	// TODO: 启动服务器
	fmt.Println("服务器启动中...")
}

func handleGenerateConfig(outputPath string) {
	cfg := config.DefaultConfig()
	yamlData, err := config.GenerateConfigYAML(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成配置失败: %v\n", err)
		os.Exit(1)
	}

	if outputPath == "" {
		fmt.Print(string(yamlData))
	} else {
		if err := os.WriteFile(outputPath, yamlData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "写入文件失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("配置已写入: %s\n", outputPath)
	}
}

func printVersion() {
	fmt.Printf("lolly version %s\n", version)
	fmt.Printf("  Git: %s (%s)\n", gitCommit, gitBranch)
	fmt.Printf("  Built: %s\n", buildTime)
	fmt.Printf("  Go: %s\n", goVersion)
	fmt.Printf("  Platform: %s\n", buildPlatform)
}