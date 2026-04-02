package main

import (
	"fmt"
	"os"
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

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-v" || os.Args[1] == "--version" {
		printVersion()
		return
	}

	fmt.Println("Hello World!")
}

func printVersion() {
	fmt.Printf("lolly version %s\n", version)
	fmt.Printf("  Git: %s (%s)\n", gitCommit, gitBranch)
	fmt.Printf("  Built: %s\n", buildTime)
	fmt.Printf("  Go: %s\n", goVersion)
	fmt.Printf("  Platform: %s\n", buildPlatform)
}
