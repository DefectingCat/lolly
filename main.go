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
	outputPath := flag.String("o", "", "输出文件路径（配合 --generate-config）")
	showVersion := flag.Bool("v", false, "显示版本")

	flag.Parse()

	// 合并短参数和长参数
	configPath := *cfgPath
	if *cfgPathLong != "" {
		configPath = *cfgPathLong
	}

	os.Exit(app.Run(configPath, *genConfig, *outputPath, *showVersion))
}