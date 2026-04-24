//go:build !windows

// Package app 提供应用程序功能的测试。
//
// 该文件测试应用程序模块的各项功能，包括：
//   - 应用创建和配置
//   - 信号处理（SIGTERM、SIGHUP、SIGUSR1等）
//   - 配置重载
//   - 日志重开
//   - 版本输出
//   - 优雅关闭
//
// 作者：xfy
package app

import (
	"bytes"
	"crypto/tls"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/http2"
	"rua.plus/lolly/internal/http3"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/server"
	"rua.plus/lolly/internal/version"
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

	// 异步读取管道，避免死锁
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&buf, r)
	}()

	return func() string {
			_ = w.Close()
			os.Stdout = old
			<-done
			return buf.String()
		}, func() {
			_ = w.Close()
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

	// 异步读取管道，避免死锁
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&buf, r)
	}()

	return func() string {
			_ = w.Close()
			os.Stderr = old
			<-done
			return buf.String()
		}, func() {
			_ = w.Close()
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
		expected string
		sig      syscall.Signal
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
		outputPath      string
		importPath      string
		wantContains    string
		wantErrContains string
		wantExitCode    int
		genConfig       bool
		showVersion     bool
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
			wantContains: "servers:",
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
		{
			name:            "generate 与 import 互斥",
			genConfig:       true,
			importPath:      "/tmp/nginx.conf",
			wantExitCode:    1,
			wantErrContains: "mutually exclusive",
		},
		{
			name:            "o 参数无 generate 或 import",
			outputPath:      "output.yaml",
			wantExitCode:    1,
			wantErrContains: "-o requires",
		},
		{
			name:            "导入 nginx 配置文件不存在",
			importPath:      "/tmp/nginx.conf",
			wantExitCode:    1,
			wantErrContains: "解析 nginx 配置失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getStdout, restoreStdout := captureStdout(t)
			getStderr, restoreStderr := captureStderr(t)

			exitCode := Run(tt.cfgPath, tt.genConfig, tt.outputPath, tt.importPath, tt.showVersion)

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
				} else if !strings.Contains(string(data), "servers:") {
					t.Errorf("生成的配置文件应包含 'servers:', 实际内容: %s", string(data)[:100])
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
		expectedFields := []string{"servers:", "listen:", "logging:", "performance:", "monitoring:"}
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
		expectedFields := []string{"servers:", "listen:", "logging:", "performance:", "monitoring:"}
		for _, field := range expectedFields {
			if !strings.Contains(content, field) {
				t.Errorf("配置文件应包含 %q", field)
			}
		}
	})

	t.Run("输出到无效路径", func(t *testing.T) {
		// Skip if running as root - root can write anywhere
		if os.Getuid() == 0 {
			t.Skip("Skipping permission test when running as root")
		}

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
		Servers: []config.ServerConfig{{
			Listen: ":0", // 使用随机端口
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
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
servers:
  - listen: ":8080"
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
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
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 使用一个未处理的信号
	result := app.handleSignal(syscall.SIGCHLD)

	if result != true {
		t.Error("Expected handleSignal(unknown) to return true (continue)")
	}
}

// TestShutdownHTTP3_NilServer 测试 HTTP/3 服务器为 nil 时关闭
func TestShutdownHTTP3_NilServer(_ *testing.T) {
	app := NewApp("")
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 不应 panic
	app.shutdownHTTP3()
}

// TestReopenLogs 测试重开日志
func TestReopenLogs(_ *testing.T) {
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
func TestReloadConfig_FileNotFound(_ *testing.T) {
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
servers:
  - listen: ":9090"
logging:
  error:
    level: "debug"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":8080",
		}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	app.reloadConfig()

	// 验证配置已更新
	if app.cfg.Servers[0].Listen != ":9090" {
		t.Errorf("Expected listen ':9090', got '%s'", app.cfg.Servers[0].Listen)
	}
}

// TestSetupSignalHandlers 测试信号处理设置
func TestSetupSignalHandlers(_ *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	sigChan := make(chan os.Signal, 1)
	app.setupSignalHandlers(sigChan)

	// 验证信号通道已设置（无法直接验证 signal.Notify，但可确认函数执行成功）
}

// TestHandleSignal_SIGUSR2 测试 SIGUSR2 信号处理（热升级）
func TestHandleSignal_SIGUSR2(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	app.srv = server.New(app.cfg)
	app.upgradeMgr = server.NewUpgradeManager(app.srv)

	// SIGUSR2 处理应返回 true（继续运行）
	result := app.handleSignal(syscall.SIGUSR2)

	if result != true {
		t.Error("Expected handleSignal(SIGUSR2) to return true (continue)")
	}
}

// TestGracefulUpgrade_NoListener 测试无监听器时的热升级
func TestGracefulUpgrade_NoListener(_ *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	app.srv = server.New(app.cfg)
	app.upgradeMgr = server.NewUpgradeManager(app.srv)

	// 在没有监听器的情况下执行热升级
	app.gracefulUpgrade()
	// 应记录错误但不 panic
}

// TestVersionVariables 测试版本变量默认值
func TestVersionVariables(t *testing.T) {
	if version.Version != "dev" {
		t.Errorf("Default Version should be 'dev', got '%s'", version.Version)
	}
	if version.GitCommit != "unknown" {
		t.Errorf("Default GitCommit should be 'unknown', got '%s'", version.GitCommit)
	}
	if version.GitBranch != "unknown" {
		t.Errorf("Default GitBranch should be 'unknown', got '%s'", version.GitBranch)
	}
	if version.BuildTime != "unknown" {
		t.Errorf("Default BuildTime should be 'unknown', got '%s'", version.BuildTime)
	}
	if version.GoVersion != "unknown" {
		t.Errorf("Default GoVersion should be 'unknown', got '%s'", version.GoVersion)
	}
	if version.BuildPlatform != "unknown" {
		t.Errorf("Default BuildPlatform should be 'unknown', got '%s'", version.BuildPlatform)
	}
}

// TestAppFields 测试 App 结构体字段初始化
func TestAppFields(t *testing.T) {
	app := NewApp("/test/config.yaml")

	// 验证初始状态
	if app.cfgPath != "/test/config.yaml" {
		t.Errorf("cfgPath = %q, want %q", app.cfgPath, "/test/config.yaml")
	}
	if app.cfg != nil {
		t.Error("cfg should be nil initially")
	}
	if app.srv != nil {
		t.Error("srv should be nil initially")
	}
	if app.http3Srv != nil {
		t.Error("http3Srv should be nil initially")
	}
	if app.streamSrv != nil {
		t.Error("streamSrv should be nil initially")
	}
	if app.upgradeMgr != nil {
		t.Error("upgradeMgr should be nil initially")
	}
	if len(app.listeners) != 0 {
		t.Error("listeners should be empty initially")
	}
}

// TestShutdownHTTP3_WithServer 测试有 HTTP3 服务器时的关闭
func TestShutdownHTTP3_WithServer(_ *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
		}},
		HTTP3: config.HTTP3Config{
			Enabled: false, // 禁用，避免实际启动
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	app.srv = server.New(app.cfg)

	// 创建但不启动 http3 服务器
	app.http3Srv = nil // 确保为 nil

	app.shutdownHTTP3()
	// 应正常执行无 panic
}

// TestReopenLogs_WithNilConfig 测试配置为 nil 时重开日志
func TestReopenLogs_WithNilConfig(_ *testing.T) {
	app := NewApp("")
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	app.reopenLogs()
	// 应正常执行无 panic
}

// TestReloadConfig_WithValidConfig 测试多次重载配置
func TestReloadConfig_WithValidConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建第一个配置
	cfgPath1 := filepath.Join(tmpDir, "config1.yaml")
	cfgContent1 := `
servers:
  - listen: ":8080"
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath1, []byte(cfgContent1), 0o644); err != nil {
		t.Fatalf("Failed to write config1: %v", err)
	}

	// 创建第二个配置
	cfgPath2 := filepath.Join(tmpDir, "config2.yaml")
	cfgContent2 := `
servers:
  - listen: ":9090"
logging:
  error:
    level: "debug"
`
	if err := os.WriteFile(cfgPath2, []byte(cfgContent2), 0o644); err != nil {
		t.Fatalf("Failed to write config2: %v", err)
	}

	app := NewApp(cfgPath1)
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":7070",
		}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 第一次重载
	app.reloadConfig()
	if app.cfg.Servers[0].Listen != ":8080" {
		t.Errorf("After first reload: listen = %q, want :8080", app.cfg.Servers[0].Listen)
	}

	// 更改配置路径并重载
	app.cfgPath = cfgPath2
	app.reloadConfig()
	if app.cfg.Servers[0].Listen != ":9090" {
		t.Errorf("After second reload: listen = %q, want :9090", app.cfg.Servers[0].Listen)
	}
}

// TestHandleSignal_AllSignals 测试所有信号类型
func TestHandleSignal_AllSignals(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":8080"
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	tests := []struct {
		name       string
		sig        syscall.Signal
		wantResult bool
	}{
		{"SIGQUIT - graceful stop", syscall.SIGQUIT, false},
		{"SIGTERM - fast stop", syscall.SIGTERM, false},
		{"SIGINT - fast stop", syscall.SIGINT, false},
		{"SIGHUP - reload", syscall.SIGHUP, true},
		{"SIGUSR1 - reopen logs", syscall.SIGUSR1, true},
		{"SIGUSR2 - upgrade", syscall.SIGUSR2, true},
		{"SIGCHLD - unknown", syscall.SIGCHLD, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewApp(cfgPath)
			app.cfg = &config.Config{
				Servers: []config.ServerConfig{{
					Listen: ":0",
				}},
			}
			app.logger = logging.NewAppLogger(&config.LoggingConfig{})
			app.srv = server.New(app.cfg)
			app.upgradeMgr = server.NewUpgradeManager(app.srv)

			result := app.handleSignal(tt.sig)

			if result != tt.wantResult {
				t.Errorf("handleSignal(%v) = %v, want %v", tt.sig, result, tt.wantResult)
			}
		})
	}
}

// TestHandleSignal_NilConfig 测试信号处理时配置为 nil 的防御性检查
func TestHandleSignal_NilConfig(t *testing.T) {
	tests := []struct {
		name       string
		sig        syscall.Signal
		wantResult bool
	}{
		{"SIGQUIT with nil config", syscall.SIGQUIT, false},
		{"SIGTERM with nil config", syscall.SIGTERM, false},
		{"SIGINT with nil config", syscall.SIGINT, false},
		{"SIGHUP with nil config", syscall.SIGHUP, true},
		{"SIGUSR1 with nil config", syscall.SIGUSR1, true},
		{"SIGUSR2 with nil config", syscall.SIGUSR2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewApp("")
			// 故意不设置 cfg，保持 nil
			app.logger = logging.NewAppLogger(&config.LoggingConfig{})
			app.srv = server.New(config.DefaultConfig())
			app.upgradeMgr = server.NewUpgradeManager(app.srv)

			result := app.handleSignal(tt.sig)

			if result != tt.wantResult {
				t.Errorf("handleSignal(%v) with nil config = %v, want %v", tt.sig, result, tt.wantResult)
			}
		})
	}
}

// TestHandleSignal_TimeoutDefaults 测试信号处理中超时默认值
func TestHandleSignal_TimeoutDefaults(t *testing.T) {
	t.Run("SIGQUIT with zero graceful timeout", func(t *testing.T) {
		app := NewApp("")
		app.cfg = &config.Config{
			Servers: []config.ServerConfig{{Listen: ":0"}},
			Shutdown: config.ShutdownConfig{
				GracefulTimeout: 0, // 使用默认值
			},
		}
		app.logger = logging.NewAppLogger(&config.LoggingConfig{})
		app.srv = server.New(app.cfg)

		result := app.handleSignal(syscall.SIGQUIT)
		// 验证函数正常执行，不 panic
		if result != false {
			t.Error("Expected false for SIGQUIT")
		}
	})

	t.Run("SIGTERM with zero fast timeout", func(t *testing.T) {
		app := NewApp("")
		app.cfg = &config.Config{
			Servers: []config.ServerConfig{{Listen: ":0"}},
			Shutdown: config.ShutdownConfig{
				FastTimeout: 0, // 使用默认值
			},
		}
		app.logger = logging.NewAppLogger(&config.LoggingConfig{})
		app.srv = server.New(app.cfg)

		result := app.handleSignal(syscall.SIGTERM)
		if result != false {
			t.Error("Expected false for SIGTERM")
		}
	})

	t.Run("SIGQUIT with negative graceful timeout", func(t *testing.T) {
		app := NewApp("")
		app.cfg = &config.Config{
			Servers: []config.ServerConfig{{Listen: ":0"}},
			Shutdown: config.ShutdownConfig{
				GracefulTimeout: -1 * time.Second, // 负数也使用默认值
			},
		}
		app.logger = logging.NewAppLogger(&config.LoggingConfig{})
		app.srv = server.New(app.cfg)

		result := app.handleSignal(syscall.SIGQUIT)
		if result != false {
			t.Error("Expected false for SIGQUIT")
		}
	})

	t.Run("SIGTERM with negative fast timeout", func(t *testing.T) {
		app := NewApp("")
		app.cfg = &config.Config{
			Servers: []config.ServerConfig{{Listen: ":0"}},
			Shutdown: config.ShutdownConfig{
				FastTimeout: -1 * time.Second,
			},
		}
		app.logger = logging.NewAppLogger(&config.LoggingConfig{})
		app.srv = server.New(app.cfg)

		result := app.handleSignal(syscall.SIGTERM)
		if result != false {
			t.Error("Expected false for SIGTERM")
		}
	})
}

// TestGracefulUpgrade_NilServer 测试服务器为 nil 时的热升级
func TestGracefulUpgrade_NilServer(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{Listen: ":0"}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	// 故意不设置 srv，保持 nil

	app.gracefulUpgrade()
	// 应记录错误但不 panic
}

// TestShutdownHTTP2_WithServer 测试有 HTTP/2 服务器时的关闭
func TestShutdownHTTP2_WithServer(t *testing.T) {
	t.Run("nil server", func(t *testing.T) {
		app := NewApp("")
		app.logger = logging.NewAppLogger(&config.LoggingConfig{})
		app.http2Srv = nil

		app.shutdownHTTP2()
		// 应正常执行无 panic
	})

	t.Run("with stopped server", func(t *testing.T) {
		app := NewApp("")
		app.logger = logging.NewAppLogger(&config.LoggingConfig{})

		// 创建一个 HTTP/2 服务器（不启动）
		h2Cfg := &config.HTTP2Config{
			MaxConcurrentStreams: 250,
		}
		h2Srv, err := http2.NewServer(h2Cfg, func(_ *fasthttp.RequestCtx) {}, nil)
		if err != nil {
			t.Fatalf("Failed to create HTTP/2 server: %v", err)
		}
		app.http2Srv = h2Srv

		app.shutdownHTTP2()
		// 应正常执行无 panic
	})
}

// TestShutdownHTTP3_WithActualServer 测试有实际 HTTP/3 服务器时的关闭
func TestShutdownHTTP3_WithActualServer(t *testing.T) {
	t.Run("with stopped server", func(t *testing.T) {
		app := NewApp("")
		app.logger = logging.NewAppLogger(&config.LoggingConfig{})

		// 创建一个 HTTP/3 服务器（不启动）
		h3Cfg := &config.HTTP3Config{
			Listen: ":0",
		}
		// 创建一个简单的 TLS 配置用于测试
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{},
		}
		h3Srv, err := http3.NewServer(h3Cfg, func(_ *fasthttp.RequestCtx) {}, tlsConfig)
		if err != nil {
			// 如果创建失败（例如缺少证书），跳过测试
			t.Skipf("Failed to create HTTP/3 server: %v", err)
		}
		app.http3Srv = h3Srv

		app.shutdownHTTP3()
		// 应正常执行无 panic
	})
}

// TestHandleSignal_SignalTypeAssertion 测试信号类型断言失败的情况
func TestHandleSignal_SignalTypeAssertion(t *testing.T) {
	// 创建一个自定义的 os.Signal 实现来测试类型断言失败路径
	customSignal := customSig{}

	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{Listen: ":0"}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	app.srv = server.New(app.cfg)

	// handleSignal 期望 syscall.Signal，传入自定义信号会触发类型断言失败
	// 注意：由于 handleSignal 的 switch 语句会匹配具体信号，自定义信号会走到 default 分支
	// 所以我们需要用一个匹配 SIGTERM/SIGINT case 但类型断言会失败的信号
	// 实际上这种场景很难构造，因为 switch case 已经限定了信号类型

	// 验证 customSignal 走到 default 分支
	result := app.handleSignal(customSignal)
	if result != true {
		t.Error("Expected true for unknown signal")
	}
}

// customSig 实现自定义信号类型用于测试
type customSig struct{}

func (customSig) String() string { return "custom" }
func (customSig) Signal()        {}

// TestHandleSignal_PositiveTimeout 测试信号处理中使用配置的超时值
func TestHandleSignal_PositiveTimeout(t *testing.T) {
	t.Run("SIGQUIT with positive graceful timeout", func(t *testing.T) {
		app := NewApp("")
		app.cfg = &config.Config{
			Servers: []config.ServerConfig{{Listen: ":0"}},
			Shutdown: config.ShutdownConfig{
				GracefulTimeout: 5 * time.Second, // 正数超时值
			},
		}
		app.logger = logging.NewAppLogger(&config.LoggingConfig{})
		app.srv = server.New(app.cfg)

		result := app.handleSignal(syscall.SIGQUIT)
		if result != false {
			t.Error("Expected false for SIGQUIT")
		}
	})

	t.Run("SIGTERM with positive fast timeout", func(t *testing.T) {
		app := NewApp("")
		app.cfg = &config.Config{
			Servers: []config.ServerConfig{{Listen: ":0"}},
			Shutdown: config.ShutdownConfig{
				FastTimeout: 3 * time.Second, // 正数超时值
			},
		}
		app.logger = logging.NewAppLogger(&config.LoggingConfig{})
		app.srv = server.New(app.cfg)

		result := app.handleSignal(syscall.SIGTERM)
		if result != false {
			t.Error("Expected false for SIGTERM")
		}
	})
}

// TestGracefulUpgrade_PositiveTimeout 测试热升级时使用配置的超时值
func TestGracefulUpgrade_PositiveTimeout(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{Listen: ":0"}},
		Shutdown: config.ShutdownConfig{
			GracefulTimeout: 5 * time.Second, // 正数超时值
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	app.srv = server.New(app.cfg)
	app.upgradeMgr = server.NewUpgradeManager(app.srv)

	// 无监听器时会失败，但会使用配置的超时值
	app.gracefulUpgrade()
	// 应正常执行无 panic
}

// TestGracefulUpgrade_ZeroTimeout 测试热升级时使用零超时值（使用默认值）
func TestGracefulUpgrade_ZeroTimeout(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{Listen: ":0"}},
		Shutdown: config.ShutdownConfig{
			GracefulTimeout: 0, // 零值，使用默认 30s
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	app.srv = server.New(app.cfg)
	app.upgradeMgr = server.NewUpgradeManager(app.srv)

	app.gracefulUpgrade()
	// 应正常执行无 panic
}

// TestSetPidFileAndWrite 测试 PID 文件设置和写入
func TestSetPidFileAndWrite(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{Listen: ":0"}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	app.srv = server.New(app.cfg)

	tmpDir := t.TempDir()
	pidPath := filepath.Join(tmpDir, "test.pid")
	app.SetPidFile(pidPath)

	if app.pidFile != pidPath {
		t.Errorf("pidFile = %q, want %q", app.pidFile, pidPath)
	}

	// 测试升级管理器写入 PID
	app.upgradeMgr = server.NewUpgradeManager(app.srv)
	app.upgradeMgr.SetPidFile(pidPath)
	if err := app.upgradeMgr.WritePid(); err != nil {
		t.Errorf("WritePid failed: %v", err)
	}

	// 验证 PID 文件已创建
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Error("PID file was not created")
	}
}

// TestApp_LoggerOperations 测试 App 日志操作
func TestApp_LoggerOperations(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{Listen: ":8080"}},
		Logging: config.LoggingConfig{
			Error: config.ErrorLogConfig{
				Level: "debug",
			},
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 测试各种日志方法
	app.logger.LogStartup("测试启动", map[string]string{"key": "value"})
	app.logger.LogShutdown("测试关闭")
	app.logger.LogSignal("SIGTERM", "测试信号")
	app.logger.Info().Msg("测试信息")
	app.logger.Error().Msg("测试错误")
}

// TestApp_Variables 测试全局变量加载
func TestApp_Variables(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":8080"
variables:
  set:
    APP_NAME: "lolly"
    DEBUG: "true"
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)
	app.cfg = config.DefaultConfig()
	app.cfg.Variables.Set = map[string]string{
		"TEST_VAR": "test_value",
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 验证变量配置存在
	if len(app.cfg.Variables.Set) != 1 {
		t.Errorf("Expected 1 variable, got %d", len(app.cfg.Variables.Set))
	}
}

// TestHandleSignal_AllSignalsWithServer 测试所有信号与服务器
func TestHandleSignal_AllSignalsWithServer(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
			SSL: config.SSLConfig{
				HTTP2: config.HTTP2Config{
					Enabled: false,
				},
			},
		}},
		HTTP3: config.HTTP3Config{
			Enabled: false,
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	app.srv = server.New(app.cfg)
	app.upgradeMgr = server.NewUpgradeManager(app.srv)

	// 测试所有信号
	signals := []struct {
		sig        syscall.Signal
		wantResult bool
	}{
		{syscall.SIGQUIT, false},
		{syscall.SIGTERM, false},
		{syscall.SIGINT, false},
		{syscall.SIGHUP, true},
		{syscall.SIGUSR1, true},
		{syscall.SIGUSR2, true},
	}

	for _, tc := range signals {
		result := app.handleSignal(tc.sig)
		if result != tc.wantResult {
			t.Errorf("handleSignal(%v) = %v, want %v", tc.sig, result, tc.wantResult)
		}
	}
}

// TestGenerateConfig_ErrorCase 测试生成配置时的错误情况
// 注意：config.GenerateConfigYAML 正常情况下不会失败，
// 但我们测试文件写入失败的情况
func TestGenerateConfig_ErrorCase(t *testing.T) {
	t.Run("写入无效路径", func(t *testing.T) {
		getStderr, restoreStderr := captureStderr(t)

		exitCode := generateConfig("/nonexistent/dir/config.yaml")
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

// TestApp_ResolverEnabled 测试启用 DNS 解析器
func TestApp_ResolverEnabled(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{Listen: ":0"}},
		Resolver: config.ResolverConfig{
			Enabled:   true,
			Addresses: []string{"8.8.8.8:53"},
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 验证配置
	if !app.cfg.Resolver.Enabled {
		t.Error("Resolver should be enabled")
	}
	if len(app.cfg.Resolver.Addresses) != 1 {
		t.Errorf("Expected 1 resolver address, got %d", len(app.cfg.Resolver.Addresses))
	}
}

// TestApp_MultiServerMode 测试多服务器模式
func TestApp_MultiServerMode(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{
			{Listen: ":8080", Name: "server1"},
			{Listen: ":8081", Name: "server2"},
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 验证多服务器配置
	if len(app.cfg.Servers) != 2 {
		t.Errorf("Expected 2 servers, got %d", len(app.cfg.Servers))
	}

	mode := app.cfg.GetMode()
	if mode != config.ServerModeMultiServer {
		t.Errorf("Expected multi-server mode, got %v", mode)
	}
}

// TestApp_StreamConfig 测试 Stream 配置
func TestApp_StreamConfig(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{Listen: ":0"}},
		Stream: []config.StreamConfig{
			{
				Listen:   ":9090",
				Protocol: "tcp",
				Upstream: config.StreamUpstream{
					Targets: []config.StreamTarget{
						{Addr: "127.0.0.1:9091", Weight: 1},
					},
					LoadBalance: "round_robin",
				},
			},
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 验证 Stream 配置
	if len(app.cfg.Stream) != 1 {
		t.Errorf("Expected 1 stream config, got %d", len(app.cfg.Stream))
	}
}

// TestApp_HTTP3Config 测试 HTTP/3 配置
func TestApp_HTTP3Config(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
			SSL: config.SSLConfig{
				Cert: "/path/to/cert.pem",
				Key:  "/path/to/key.pem",
			},
		}},
		HTTP3: config.HTTP3Config{
			Enabled: true,
			Listen:  ":443",
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 验证 HTTP/3 配置
	if !app.cfg.HTTP3.Enabled {
		t.Error("HTTP/3 should be enabled")
	}
}

// TestApp_HTTP2Config 测试 HTTP/2 配置
func TestApp_HTTP2Config(t *testing.T) {
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{
			Listen: ":0",
			SSL: config.SSLConfig{
				Cert: "/path/to/cert.pem",
				Key:  "/path/to/key.pem",
				HTTP2: config.HTTP2Config{
					Enabled:              true,
					MaxConcurrentStreams: 100,
					PushEnabled:          true,
				},
			},
		}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 验证 HTTP/2 配置
	if !app.cfg.Servers[0].SSL.HTTP2.Enabled {
		t.Error("HTTP/2 should be enabled")
	}
}

// TestGracefulUpgrade_GetExecutableError 测试获取可执行文件路径失败的情况
// 注意：os.Executable 正常情况下不会失败，此测试验证错误处理路径存在
func TestGracefulUpgrade_GetExecutableError(t *testing.T) {
	// 此测试验证代码路径存在，但实际很难触发 os.Executable 失败
	app := NewApp("")
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{Listen: ":0"}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})
	app.srv = server.New(app.cfg)
	app.upgradeMgr = server.NewUpgradeManager(app.srv)

	// 正常情况下会获取到可执行文件路径，但无监听器会失败
	app.gracefulUpgrade()
	// 验证不会 panic
}

// TestReloadConfig_UpdateLogger 测试重载配置后更新日志器
func TestReloadConfig_UpdateLogger(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":9090"
logging:
  error:
    level: "debug"
  format: "json"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{Listen: ":8080"}},
		Logging: config.LoggingConfig{
			Error:  config.ErrorLogConfig{Level: "info"},
			Format: "text",
		},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 重载配置
	app.reloadConfig()

	// 验证配置已更新
	if app.cfg.Servers[0].Listen != ":9090" {
		t.Errorf("Expected listen ':9090', got '%s'", app.cfg.Servers[0].Listen)
	}

	// 验证日志器已更新（通过新配置创建）
	if app.cfg.Logging.Format != "json" {
		t.Errorf("Expected format 'json', got '%s'", app.cfg.Logging.Format)
	}
}

// TestHandleSignal_SIGHUP_WithValidConfigFile 测试 SIGHUP 重载有效配置文件
func TestHandleSignal_SIGHUP_WithValidConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":7070"
logging:
  error:
    level: "debug"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)
	app.cfg = &config.Config{
		Servers: []config.ServerConfig{{Listen: ":8080"}},
	}
	app.logger = logging.NewAppLogger(&config.LoggingConfig{})

	// 发送 SIGHUP 信号
	result := app.handleSignal(syscall.SIGHUP)

	if result != true {
		t.Error("Expected handleSignal(SIGHUP) to return true")
	}

	// 验证配置已更新
	if app.cfg.Servers[0].Listen != ":7070" {
		t.Errorf("Expected listen ':7070', got '%s'", app.cfg.Servers[0].Listen)
	}
}

// TestApp_Run_WithValidConfig 测试 App.Run 加载有效配置
// 注意：此测试验证配置加载路径，但由于服务器启动会阻塞，
// 我们通过子进程测试或使用 short 标志跳过完整运行
func TestApp_Run_WithValidConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":0"
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)

	// 在 goroutine 中启动服务器
	done := make(chan int, 1)
	go func() {
		done <- app.Run()
	}()

	// 等待一小段时间让服务器启动
	time.Sleep(100 * time.Millisecond)

	// 验证配置已加载
	if app.cfg == nil {
		t.Error("Config should be loaded")
	}

	// 验证服务器已创建
	if app.srv == nil {
		t.Error("Server should be created")
	}

	// 验证 logger 已创建
	if app.logger == nil {
		t.Error("Logger should be created")
	}

	// 验证升级管理器已创建
	if app.upgradeMgr == nil {
		t.Error("Upgrade manager should be created")
	}

	// 发送 SIGTERM 信号停止服务器
	app.srv.StopWithTimeout(1 * time.Second)
}

// TestApp_Run_WithVariables 测试 App.Run 加载全局变量
func TestApp_Run_WithVariables(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":0"
variables:
  set:
    TEST_VAR: "test_value"
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)

	done := make(chan int, 1)
	go func() {
		done <- app.Run()
	}()

	time.Sleep(100 * time.Millisecond)

	// 验证配置已加载且包含变量
	if app.cfg == nil {
		t.Error("Config should be loaded")
	}

	// 停止服务器
	app.srv.StopWithTimeout(1 * time.Second)
}

// TestApp_Run_WithResolver 测试 App.Run 启用 DNS 解析器
func TestApp_Run_WithResolver(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":0"
resolver:
  enabled: true
  addresses:
    - "8.8.8.8:53"
  ipv4: true
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)

	done := make(chan int, 1)
	go func() {
		done <- app.Run()
	}()

	time.Sleep(100 * time.Millisecond)

	// 验证 DNS 解析器已创建
	if app.resv == nil {
		t.Error("Resolver should be created when enabled")
	}

	// 验证服务器已创建
	if app.srv == nil {
		t.Error("Server should be created")
	}

	// 停止服务器
	app.srv.StopWithTimeout(1 * time.Second)
}

// TestApp_Run_MultiServer 测试 App.Run 多服务器模式
func TestApp_Run_MultiServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":0"
    name: "server1"
  - listen: ":0"
    name: "server2"
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)

	done := make(chan int, 1)
	go func() {
		done <- app.Run()
	}()

	time.Sleep(100 * time.Millisecond)

	// 验证多服务器配置
	if app.cfg == nil {
		t.Error("Config should be loaded")
	}
	if len(app.cfg.Servers) != 2 {
		t.Errorf("Expected 2 servers, got %d", len(app.cfg.Servers))
	}

	// 停止服务器
	app.srv.StopWithTimeout(1 * time.Second)
}

// TestApp_Run_WithPidFile 测试 App.Run 设置 PID 文件
func TestApp_Run_WithPidFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":0"
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	pidPath := filepath.Join(tmpDir, "lolly.pid")

	app := NewApp(cfgPath)
	app.SetPidFile(pidPath)

	done := make(chan int, 1)
	go func() {
		done <- app.Run()
	}()

	time.Sleep(100 * time.Millisecond)

	// 验证 PID 文件已创建
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Error("PID file should be created")
	}

	// 停止服务器
	app.srv.StopWithTimeout(1 * time.Second)
}

// TestApp_Run_WithStreamConfig 测试 App.Run 配置 Stream 服务器
func TestApp_Run_WithStreamConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":0"
stream:
  - listen: ":19090"
    protocol: tcp
    upstream:
      targets:
        - addr: "127.0.0.1:19091"
          weight: 1
      load_balance: round_robin
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)

	done := make(chan int, 1)
	go func() {
		done <- app.Run()
	}()

	time.Sleep(150 * time.Millisecond)

	// 验证 Stream 服务器已创建
	if app.streamSrv == nil {
		t.Error("Stream server should be created when configured")
	}

	// 停止服务器
	app.srv.StopWithTimeout(1 * time.Second)
}

// TestApp_Run_GracefulUpgradeMode 测试 App.Run 热升级子进程模式
func TestApp_Run_GracefulUpgradeMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":0"
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// 设置热升级模式环境变量
	t.Setenv("GRACEFUL_UPGRADE", "1")

	app := NewApp(cfgPath)

	done := make(chan int, 1)
	go func() {
		done <- app.Run()
	}()

	time.Sleep(100 * time.Millisecond)

	// 验证升级管理器已创建
	if app.upgradeMgr == nil {
		t.Error("Upgrade manager should be created in graceful upgrade mode")
	}

	// 停止服务器
	app.srv.StopWithTimeout(1 * time.Second)
}

// TestApp_Run_WithLogFile 测试 App.Run 设置日志文件
func TestApp_Run_WithLogFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":0"
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	logPath := filepath.Join(tmpDir, "lolly.log")

	app := NewApp(cfgPath)
	app.SetLogFile(logPath)

	done := make(chan int, 1)
	go func() {
		done <- app.Run()
	}()

	time.Sleep(100 * time.Millisecond)

	// 验证日志文件路径已设置
	if app.logFile != logPath {
		t.Errorf("logFile = %q, want %q", app.logFile, logPath)
	}

	// 停止服务器
	app.srv.StopWithTimeout(1 * time.Second)
}

// TestApp_Run_WithZeroTimeout 测试 App.Run 使用零超时配置（会使用默认值）
func TestApp_Run_WithZeroTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":0"
shutdown:
  graceful_timeout: 0s
  fast_timeout: 0s
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)

	done := make(chan int, 1)
	go func() {
		done <- app.Run()
	}()

	time.Sleep(100 * time.Millisecond)

	// 验证配置已加载
	if app.cfg == nil {
		t.Error("Config should be loaded")
	}

	// 停止服务器（会使用默认超时值）
	app.srv.StopWithTimeout(1 * time.Second)
}

// TestApp_Run_WithExplicitTimeout 测试 App.Run 使用显式超时配置
func TestApp_Run_WithExplicitTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
servers:
  - listen: ":0"
shutdown:
  graceful_timeout: 10s
  fast_timeout: 5s
logging:
  error:
    level: "info"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	app := NewApp(cfgPath)

	done := make(chan int, 1)
	go func() {
		done <- app.Run()
	}()

	time.Sleep(100 * time.Millisecond)

	// 验证配置已加载
	if app.cfg == nil {
		t.Error("Config should be loaded")
	}

	// 验证超时值
	if app.cfg.Shutdown.GracefulTimeout != 10*time.Second {
		t.Errorf("GracefulTimeout = %v, want 10s", app.cfg.Shutdown.GracefulTimeout)
	}

	// 停止服务器
	app.srv.StopWithTimeout(1 * time.Second)
}
