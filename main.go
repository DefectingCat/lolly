// Package main 提供 Lolly 服务器的入口程序。
//
// 该文件包含命令行参数解析和应用程序启动逻辑：
//   - 配置文件路径指定（-c/--config）
//   - 默认配置生成（--generate-config）
//   - 版本信息显示（-v）
//
// 使用示例：
//
//	lolly -c /etc/lolly.yaml        # 使用指定配置启动
//	lolly --generate-config -o config.yaml  # 生成默认配置
//	lolly -v                        # 显示版本信息
//
// 作者：xfy
package main

import (
	"flag"
	"os"

	"rua.plus/lolly/internal/app"
)

func main() {
	cfgPath := flag.String("c", "lolly.yaml", "配置文件路径")
	cfgPathLong := flag.String("config", "", "配置文件路径（长参数）")
	genConfig := flag.Bool("generate-config", false, "生成默认配置")
	genConfigShort := flag.Bool("g", false, "生成默认配置（短参数）")
	outputPath := flag.String("o", "", "输出文件路径（配合 --generate-config）")
	showVersion := flag.Bool("v", false, "显示版本")

	flag.Parse()

	// 合并短参数和长参数
	configPath := *cfgPath
	if *cfgPathLong != "" {
		configPath = *cfgPathLong
	}
	generate := *genConfig || *genConfigShort

	os.Exit(app.Run(configPath, generate, *outputPath, *showVersion))
}
