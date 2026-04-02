// Package app 提供应用程序的启动和运行逻辑。
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout 捕获 stdout 输出，返回捕获的内容和恢复函数。
func captureStdout(t *testing.T) (func() string, func()) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建 pipe 失败: %v", err)
	}
	os.Stdout = w

	return func() string {
			w.Close()
			os.Stdout = old
			var buf bytes.Buffer
			buf.ReadFrom(r)
			return buf.String()
		}, func() {
			w.Close()
			os.Stdout = old
		}
}

// captureStderr 捕获 stderr 输出，返回捕获的内容和恢复函数。
func captureStderr(t *testing.T) (func() string, func()) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建 pipe 失败: %v", err)
	}
	os.Stderr = w

	return func() string {
			w.Close()
			os.Stderr = old
			var buf bytes.Buffer
			buf.ReadFrom(r)
			return buf.String()
		}, func() {
			w.Close()
			os.Stderr = old
		}
}

// TestRun 测试 Run 函数的各种场景。
func TestRun(t *testing.T) {
	tests := []struct {
		name         string
		cfgPath      string
		genConfig    bool
		outputPath   string
		showVersion  bool
		wantExitCode int
		wantContains string // stdout 应包含的内容
		wantErrContains string // stderr 应包含的内容（可选）
	}{
		{
			name:         "显示版本",
			showVersion:  true,
			wantExitCode: 0,
			wantContains: "lolly version",
		},
		{
			name:         "生成配置输出到 stdout",
			genConfig:    true,
			outputPath:   "",
			wantExitCode: 0,
			wantContains: "server:",
		},
		{
			name:         "生成配置输出到文件",
			genConfig:    true,
			outputPath:   filepath.Join(t.TempDir(), "config.yaml"),
			wantExitCode: 0,
			wantContains: "配置已写入:",
		},
		{
			name:         "配置文件不存在",
			cfgPath:      filepath.Join(t.TempDir(), "nonexistent.yaml"),
			genConfig:    false,
			showVersion:  false,
			wantExitCode: 1,
			wantErrContains: "加载配置失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getStdout, restoreStdout := captureStdout(t)
			getStderr, restoreStderr := captureStderr(t)

			exitCode := Run(tt.cfgPath, tt.genConfig, tt.outputPath, tt.showVersion)

			restoreStderr()
			restoreStdout()

			stdout := getStdout()
			stderr := getStderr()

			if exitCode != tt.wantExitCode {
				t.Errorf("exit code = %d, want %d", exitCode, tt.wantExitCode)
			}

			if tt.wantContains != "" && !strings.Contains(stdout, tt.wantContains) {
				t.Errorf("stdout 应包含 %q, 实际输出: %q", tt.wantContains, stdout)
			}

			if tt.wantErrContains != "" && !strings.Contains(stderr, tt.wantErrContains) {
				t.Errorf("stderr 应包含 %q, 实际输出: %q", tt.wantErrContains, stderr)
			}

			// 验证生成配置文件的内容
			if tt.outputPath != "" && tt.genConfig && exitCode == 0 {
				data, err := os.ReadFile(tt.outputPath)
				if err != nil {
					t.Errorf("读取生成的配置文件失败: %v", err)
				} else if !strings.Contains(string(data), "server:") {
					t.Errorf("生成的配置文件应包含 'server:', 实际内容: %s", string(data)[:100])
				}
			}
		})
	}
}

// TestGenerateConfig 测试 generateConfig 函数。
func TestGenerateConfig(t *testing.T) {
	t.Run("输出到 stdout", func(t *testing.T) {
		getStdout, restoreStdout := captureStdout(t)

		exitCode := generateConfig("")
		restoreStdout()

		stdout := getStdout()

		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}

		// 验证输出包含基本配置结构
		expectedFields := []string{"server:", "listen:", "logging:", "performance:", "monitoring:"}
		for _, field := range expectedFields {
			if !strings.Contains(stdout, field) {
				t.Errorf("输出应包含 %q", field)
			}
		}
	})

	t.Run("输出到文件", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputPath := filepath.Join(tmpDir, "test-config.yaml")

		getStdout, restoreStdout := captureStdout(t)

		exitCode := generateConfig(outputPath)
		restoreStdout()

		stdout := getStdout()

		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}

		if !strings.Contains(stdout, outputPath) {
			t.Errorf("stdout 应包含文件路径 %q, 实际输出: %q", outputPath, stdout)
		}

		// 验证文件存在且内容正确
		data, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("读取生成的配置文件失败: %v", err)
		}

		content := string(data)
		expectedFields := []string{"server:", "listen:", "logging:", "performance:", "monitoring:"}
		for _, field := range expectedFields {
			if !strings.Contains(content, field) {
				t.Errorf("配置文件应包含 %q", field)
			}
		}
	})

	t.Run("输出到无效路径", func(t *testing.T) {
		// 使用一个无法写入的路径（如根目录下的文件）
		invalidPath := "/root/cannot-write-here.yaml"

		getStderr, restoreStderr := captureStderr(t)

		exitCode := generateConfig(invalidPath)
		restoreStderr()

		stderr := getStderr()

		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}

		if !strings.Contains(stderr, "写入文件失败") {
			t.Errorf("stderr 应包含 '写入文件失败', 实际输出: %q", stderr)
		}
	})
}

// TestPrintVersion 测试 printVersion 函数。
func TestPrintVersion(t *testing.T) {
	getStdout, restoreStdout := captureStdout(t)

	printVersion()
	restoreStdout()

	stdout := getStdout()

	// 验证版本输出格式
	expectedLines := []string{
		"lolly version",
		"Git:",
		"Built:",
		"Go:",
		"Platform:",
	}

	for _, line := range expectedLines {
		if !strings.Contains(stdout, line) {
			t.Errorf("版本输出应包含 %q, 实际输出: %q", line, stdout)
		}
	}
}