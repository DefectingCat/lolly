// Package app 提供应用程序的启动和运行逻辑。
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/server"
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

// TestNewApp 测试 NewApp 构造器。
func TestNewApp(t *testing.T) {
	cfgPath := "/path/to/config.yaml"
	app := NewApp(cfgPath)

	if app.cfgPath != cfgPath {
		t.Errorf("cfgPath = %q, want %q", app.cfgPath, cfgPath)
	}

	if app.cfg != nil {
		t.Error("新创建的 App cfg 应为 nil")
	}

	if app.srv != nil {
		t.Error("新创建的 App srv 应为 nil")
	}

	if app.pidFile != "" {
		t.Errorf("pidFile = %q, want empty", app.pidFile)
	}

	if app.logFile != "" {
		t.Errorf("logFile = %q, want empty", app.logFile)
	}
}

// TestSetPidFile 测试 SetPidFile setter 方法。
func TestSetPidFile(t *testing.T) {
	app := NewApp("/test/config.yaml")
	pidPath := "/var/run/lolly.pid"

	app.SetPidFile(pidPath)

	if app.pidFile != pidPath {
		t.Errorf("pidFile = %q, want %q", app.pidFile, pidPath)
	}
}

// TestSetLogFile 测试 SetLogFile setter 方法。
func TestSetLogFile(t *testing.T) {
	app := NewApp("/test/config.yaml")
	logPath := "/var/log/lolly.log"

	app.SetLogFile(logPath)

	if app.logFile != logPath {
		t.Errorf("logFile = %q, want %q", app.logFile, logPath)
	}
}

// TestSigName 测试信号名称辅助函数。
func TestSigName(t *testing.T) {
	tests := []struct {
		name     string
		sig      syscall.Signal
		expected string
	}{
		{
			name:     "SIGTERM",
			sig:      syscall.SIGTERM,
			expected: "SIGTERM",
		},
		{
			name:     "SIGINT",
			sig:      syscall.SIGINT,
			expected: "SIGINT",
		},
		{
			name:     "SIGQUIT",
			sig:      syscall.SIGQUIT,
			expected: "SIGQUIT",
		},
		{
			name:     "SIGHUP",
			sig:      syscall.SIGHUP,
			expected: "SIGHUP",
		},
		{
			name:     "SIGUSR1",
			sig:      syscall.SIGUSR1,
			expected: "SIGUSR1",
		},
		{
			name:     "SIGUSR2",
			sig:      syscall.SIGUSR2,
			expected: "SIGUSR2",
		},
		{
			name:     "未知信号",
			sig:      syscall.Signal(999),
			expected: "Signal(999)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sigName(tt.sig)
			if result != tt.expected {
				t.Errorf("sigName(%d) = %q, want %q", tt.sig, result, tt.expected)
			}
		})
	}
}
func TestRun(t *testing.T) {
	tests := []struct {
		name            string
		cfgPath         string
		genConfig       bool
		outputPath      string
		showVersion     bool
		wantExitCode    int
		wantContains    string // stdout 应包含的内容
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
			name:            "配置文件不存在",
			cfgPath:         filepath.Join(t.TempDir(), "nonexistent.yaml"),
			genConfig:       false,
			showVersion:     false,
			wantExitCode:    1,
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

// TestHandleSignal_SIGQUIT 测试 SIGQUIT 信号处理（优雅停止）
func TestHandleSignal_SIGQUIT(t *testing.T) {
	// 创建一个简单的 App
	app := NewApp("")
	app.cfg = &config.Config{
		Server: config.ServerConfig{
			Listen: ":0", // 使用随机端口
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 创建 mock server
	app.srv = server.New(app.cfg)

	// 测试 SIGQUIT 处理
	result := app.handleSignal(syscall.SIGQUIT)

	if result != false {
		t.Error("Expected handleSignal(SIGQUIT) to return false (stop)")
	}
}

// TestHandleSignal_SIGTERM 测试 SIGTERM 信号处理（快速停止）
func TestHandleSignal_SIGTERM(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Server: config.ServerConfig{
			Listen: ":0",
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	app.srv = server.New(app.cfg)

	result := app.handleSignal(syscall.SIGTERM)

	if result != false {
		t.Error("Expected handleSignal(SIGTERM) to return false (stop)")
	}
}

// TestHandleSignal_SIGINT 测试 SIGINT 信号处理（快速停止）
func TestHandleSignal_SIGINT(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Server: config.ServerConfig{
			Listen: ":0",
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	app.srv = server.New(app.cfg)

	result := app.handleSignal(syscall.SIGINT)

	if result != false {
		t.Error("Expected handleSignal(SIGINT) to return false (stop)")
	}
}

// TestHandleSignal_SIGHUP 测试 SIGHUP 信号处理（重载配置）
func TestHandleSignal_SIGHUP(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
server:
  listen: ":8080"
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)
	app.cfg = &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	result := app.handleSignal(syscall.SIGHUP)

	if result != true {
		t.Error("Expected handleSignal(SIGHUP) to return true (continue)")
	}
}

// TestHandleSignal_SIGUSR1 测试 SIGUSR1 信号处理（重开日志）
func TestHandleSignal_SIGUSR1(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
		Logging: config.LoggingConfig{
			Error: config.ErrorLogConfig{
				Level: "info",
			},
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	result := app.handleSignal(syscall.SIGUSR1)

	if result != true {
		t.Error("Expected handleSignal(SIGUSR1) to return true (continue)")
	}
}

// TestHandleSignal_Unknown 测试未知信号处理
func TestHandleSignal_Unknown(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 使用一个未处理的信号
	result := app.handleSignal(syscall.SIGCHLD)

	if result != true {
		t.Error("Expected handleSignal(unknown) to return true (continue)")
	}
}

// TestShutdownHTTP3_NilServer 测试 HTTP/3 服务器为 nil 时关闭
func TestShutdownHTTP3_NilServer(t *testing.T) {
	app := NewApp("")
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 不应 panic
	app.shutdownHTTP3()
}

// TestReopenLogs 测试重开日志
func TestReopenLogs(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Logging: config.LoggingConfig{
			Error: config.ErrorLogConfig{
				Level: "info",
			},
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 不应 panic
	app.reopenLogs()
}

// TestReloadConfig_FileNotFound 测试重载不存在的配置
func TestReloadConfig_FileNotFound(t *testing.T) {
	app := NewApp("/nonexistent/config.yaml")
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 不应 panic，只是记录错误
	app.reloadConfig()
}

// TestReloadConfig_Success 测试成功重载配置
func TestReloadConfig_Success(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
server:
  listen: ":9090"
logging:
  error:
    level: "debug"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)
	app.cfg = &config.Config{
		Server: config.ServerConfig{
			Listen: ":8080",
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	app.reloadConfig()

	// 验证配置已更新
	if app.cfg.Server.Listen != ":9090" {
		t.Errorf("Expected listen ':9090', got '%s'", app.cfg.Server.Listen)
	}
}
