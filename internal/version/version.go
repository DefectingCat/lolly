// Package version 提供版本信息。
//
// 版本信息通过 -ldflags 在构建时注入。
package version

// 版本信息，通过 -ldflags 注入。
var (
	// Version 版本号
	Version = "dev"
	// GitCommit Git 提交哈希
	GitCommit = "unknown"
	// GitBranch Git 分支名
	GitBranch = "unknown"
	// BuildTime 构建时间
	BuildTime = "unknown"
	// GoVersion Go 版本
	GoVersion = "unknown"
	// BuildPlatform 构建平台
	BuildPlatform = "unknown"
)
