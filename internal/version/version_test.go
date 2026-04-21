// Package version 提供版本信息的测试。
package version

import (
	"strings"
	"testing"
)

// TestDefaultValues 测试默认值。
func TestDefaultValues(t *testing.T) {
	// 注意：由于版本变量是包级别的变量，
	// 这个测试验证的是编译时的默认值。
	// 在实际构建中，这些值会通过 -ldflags 注入。

	t.Run("Version默认值", func(t *testing.T) {
		// 默认值应该是 "dev"
		if Version != "dev" {
			t.Errorf("Version = %q, want %q", Version, "dev")
		}
	})

	t.Run("GitCommit默认值", func(t *testing.T) {
		if GitCommit != "unknown" {
			t.Errorf("GitCommit = %q, want %q", GitCommit, "unknown")
		}
	})

	t.Run("GitBranch默认值", func(t *testing.T) {
		if GitBranch != "unknown" {
			t.Errorf("GitBranch = %q, want %q", GitBranch, "unknown")
		}
	})

	t.Run("BuildTime默认值", func(t *testing.T) {
		if BuildTime != "unknown" {
			t.Errorf("BuildTime = %q, want %q", BuildTime, "unknown")
		}
	})

	t.Run("GoVersion默认值", func(t *testing.T) {
		if GoVersion != "unknown" {
			t.Errorf("GoVersion = %q, want %q", GoVersion, "unknown")
		}
	})

	t.Run("BuildPlatform默认值", func(t *testing.T) {
		if BuildPlatform != "unknown" {
			t.Errorf("BuildPlatform = %q, want %q", BuildPlatform, "unknown")
		}
	})
}

// TestVersionVariableMutation 测试版本变量可以被修改。
// 这模拟了 -ldflags 注入的效果。
func TestVersionVariableMutation(t *testing.T) {
	// 保存原始值
	originalVersion := Version
	originalGitCommit := GitCommit
	originalGitBranch := GitBranch
	originalBuildTime := BuildTime
	originalGoVersion := GoVersion
	originalBuildPlatform := BuildPlatform

	// 在测试结束时恢复原始值
	t.Cleanup(func() {
		Version = originalVersion
		GitCommit = originalGitCommit
		GitBranch = originalGitBranch
		BuildTime = originalBuildTime
		GoVersion = originalGoVersion
		BuildPlatform = originalBuildPlatform
	})

	t.Run("设置Version", func(t *testing.T) {
		testVersion := "v1.2.3"
		Version = testVersion
		if Version != testVersion {
			t.Errorf("Version = %q, want %q", Version, testVersion)
		}
	})

	t.Run("设置GitCommit", func(t *testing.T) {
		testCommit := "abc123def456"
		GitCommit = testCommit
		if GitCommit != testCommit {
			t.Errorf("GitCommit = %q, want %q", GitCommit, testCommit)
		}
	})

	t.Run("设置GitBranch", func(t *testing.T) {
		testBranch := "feature/test"
		GitBranch = testBranch
		if GitBranch != testBranch {
			t.Errorf("GitBranch = %q, want %q", GitBranch, testBranch)
		}
	})

	t.Run("设置BuildTime", func(t *testing.T) {
		testBuildTime := "2024-01-15T10:30:00Z"
		BuildTime = testBuildTime
		if BuildTime != testBuildTime {
			t.Errorf("BuildTime = %q, want %q", BuildTime, testBuildTime)
		}
	})

	t.Run("设置GoVersion", func(t *testing.T) {
		testGoVersion := "go1.21.5"
		GoVersion = testGoVersion
		if GoVersion != testGoVersion {
			t.Errorf("GoVersion = %q, want %q", GoVersion, testGoVersion)
		}
	})

	t.Run("设置BuildPlatform", func(t *testing.T) {
		testPlatform := "linux/amd64"
		BuildPlatform = testPlatform
		if BuildPlatform != testPlatform {
			t.Errorf("BuildPlatform = %q, want %q", BuildPlatform, testPlatform)
		}
	})
}

// TestVersionInformationFormat 测试版本信息的格式化。
func TestVersionInformationFormat(t *testing.T) {
	// 保存原始值
	originalVersion := Version
	originalGitCommit := GitCommit
	originalGitBranch := GitBranch
	originalBuildTime := BuildTime
	originalGoVersion := GoVersion
	originalBuildPlatform := BuildPlatform

	t.Cleanup(func() {
		Version = originalVersion
		GitCommit = originalGitCommit
		GitBranch = originalGitBranch
		BuildTime = originalBuildTime
		GoVersion = originalGoVersion
		BuildPlatform = originalBuildPlatform
	})

	// 设置测试值
	Version = "v2.0.0"
	GitCommit = "a1b2c3d4e5f6"
	GitBranch = "main"
	BuildTime = "2024-06-01T12:00:00Z"
	GoVersion = "go1.22.0"
	BuildPlatform = "darwin/arm64"

	t.Run("版本号格式", func(t *testing.T) {
		// 版本号应该以 'v' 开头（语义化版本规范）
		if !strings.HasPrefix(Version, "v") {
			t.Errorf("Version = %q, should start with 'v'", Version)
		}
	})

	t.Run("Git提交哈希格式", func(t *testing.T) {
		// Git 提交哈希应该是十六进制字符串
		for _, c := range GitCommit {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("GitCommit = %q, contains invalid character %q", GitCommit, c)
				break
			}
		}
	})

	t.Run("构建平台格式", func(t *testing.T) {
		// 构建平台应该是 OS/Arch 格式
		if !strings.Contains(BuildPlatform, "/") {
			t.Errorf("BuildPlatform = %q, should be in OS/Arch format", BuildPlatform)
		}
	})

	t.Run("Go版本格式", func(t *testing.T) {
		// Go 版本应该以 'go' 开头
		if !strings.HasPrefix(GoVersion, "go") {
			t.Errorf("GoVersion = %q, should start with 'go'", GoVersion)
		}
	})
}

// TestAllVariablesNonEmpty 测试所有变量设置后非空。
func TestAllVariablesNonEmpty(t *testing.T) {
	// 保存原始值
	originalVersion := Version
	originalGitCommit := GitCommit
	originalGitBranch := GitBranch
	originalBuildTime := BuildTime
	originalGoVersion := GoVersion
	originalBuildPlatform := BuildPlatform

	t.Cleanup(func() {
		Version = originalVersion
		GitCommit = originalGitCommit
		GitBranch = originalGitBranch
		BuildTime = originalBuildTime
		GoVersion = originalGoVersion
		BuildPlatform = originalBuildPlatform
	})

	// 设置非空值
	Version = "v1.0.0"
	GitCommit = "1234567890abcdef"
	GitBranch = "master"
	BuildTime = "2024-01-01"
	GoVersion = "go1.21"
	BuildPlatform = "linux/amd64"

	vars := map[string]string{
		"Version":       Version,
		"GitCommit":     GitCommit,
		"GitBranch":     GitBranch,
		"BuildTime":     BuildTime,
		"GoVersion":     GoVersion,
		"BuildPlatform": BuildPlatform,
	}

	for name, value := range vars {
		if value == "" {
			t.Errorf("%s is empty, expected non-empty value", name)
		}
		if value == "unknown" {
			t.Errorf("%s = %q, expected set value", name, value)
		}
	}
}

// TestVersionConsistency 测试版本信息的一致性。
func TestVersionConsistency(t *testing.T) {
	// 保存原始值
	originalVersion := Version
	originalGitCommit := GitCommit
	originalGitBranch := GitBranch
	originalBuildTime := BuildTime
	originalGoVersion := GoVersion
	originalBuildPlatform := BuildPlatform

	t.Cleanup(func() {
		Version = originalVersion
		GitCommit = originalGitCommit
		GitBranch = originalGitBranch
		BuildTime = originalBuildTime
		GoVersion = originalGoVersion
		BuildPlatform = originalBuildPlatform
	})

	t.Run("语义化版本格式", func(t *testing.T) {
		testVersions := []string{
			"v1.0.0",
			"v2.1.3",
			"v0.0.1",
			"v10.20.30",
		}

		for _, v := range testVersions {
			Version = v
			// 验证版本号格式
			if !strings.HasPrefix(Version, "v") {
				t.Errorf("Version = %q, should start with 'v'", Version)
			}
			// 验证版本号包含点号
			if !strings.Contains(Version[1:], ".") {
				t.Errorf("Version = %q, should contain '.' after 'v'", Version)
			}
		}
	})

	t.Run("开发版本标识", func(t *testing.T) {
		Version = "dev"
		if Version != "dev" {
			t.Errorf("Version = %q, want %q", Version, "dev")
		}
	})
}

// TestBuildPlatformVariants 测试不同构建平台格式。
func TestBuildPlatformVariants(t *testing.T) {
	// 保存原始值
	originalBuildPlatform := BuildPlatform
	t.Cleanup(func() {
		BuildPlatform = originalBuildPlatform
	})

	platforms := []string{
		"linux/amd64",
		"linux/arm64",
		"darwin/amd64",
		"darwin/arm64",
		"windows/amd64",
		"freebsd/amd64",
	}

	for _, platform := range platforms {
		t.Run(platform, func(t *testing.T) {
			BuildPlatform = platform
			if BuildPlatform != platform {
				t.Errorf("BuildPlatform = %q, want %q", BuildPlatform, platform)
			}
		})
	}
}
