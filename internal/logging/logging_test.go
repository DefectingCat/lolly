// Package logging 提供日志功能的测试。
//
// 该文件测试日志模块的各项功能，包括：
//   - 日志级别解析
//   - 日志记录器创建
//   - 访问日志记录
//   - 错误日志记录
//   - 自定义格式
//   - 文件输出
//
// 作者：xfy
package logging

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected zerolog.Level
	}{
		{
			name:     "debug level",
			input:    "debug",
			expected: zerolog.DebugLevel,
		},
		{
			name:     "info level",
			input:    "info",
			expected: zerolog.InfoLevel,
		},
		{
			name:     "warn level",
			input:    "warn",
			expected: zerolog.WarnLevel,
		},
		{
			name:     "error level",
			input:    "error",
			expected: zerolog.ErrorLevel,
		},
		{
			name:     "unknown level defaults to info",
			input:    "unknown",
			expected: zerolog.InfoLevel,
		},
		{
			name:     "empty string defaults to info",
			input:    "",
			expected: zerolog.InfoLevel,
		},
		{
			name:     "uppercase DEBUG now works",
			input:    "DEBUG",
			expected: zerolog.DebugLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			result := parseLevel(tt.input)
			if result != tt.expected {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.LoggingConfig
	}{
		{"nil config", nil},
		{"empty paths", &config.LoggingConfig{}},
		{"with access format", &config.LoggingConfig{Access: config.AccessLogConfig{Format: "json"}}},
		{"with error level", &config.LoggingConfig{Error: config.ErrorLogConfig{Level: "debug"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			logger := New(tt.cfg)
			if logger == nil {
				t.Error("Expected non-nil Logger")
			}
		})
	}
}

func TestLoggerWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	accessPath := filepath.Join(tmpDir, "access.log")
	errorPath := filepath.Join(tmpDir, "error.log")

	cfg := &config.LoggingConfig{
		Access: config.AccessLogConfig{Path: accessPath, Format: "json"},
		Error:  config.ErrorLogConfig{Path: errorPath, Level: "info"},
	}

	logger := New(cfg)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")
	ctx.Request.Header.SetMethod("GET")

	logger.LogAccess(ctx, 200, 10, 100*time.Millisecond)
	logger.Error().Str("test", "value").Msg("test error")
	if err := logger.Close(); err != nil {
		t.Errorf("Unexpected error on Close: %v", err)
	}

	if _, err := os.Stat(accessPath); os.IsNotExist(err) {
		t.Error("Expected access log file to be created")
	}
}

func TestGetOutput(t *testing.T) {
	if getOutput("") != os.Stdout {
		t.Error("Expected stdout for empty path")
	}
	if getOutput("stderr") != os.Stderr {
		t.Error("Expected stderr for 'stderr' path")
	}

	tmpFile := filepath.Join(t.TempDir(), "test.log")
	out := getOutput(tmpFile)
	if out == nil {
		t.Error("Expected non-nil writer for file path")
	}
}

func TestLoggerNginxFormat(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger := New(&config.LoggingConfig{Access: config.AccessLogConfig{Format: "json"}})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/users?id=123")
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.Set("User-Agent", "test-agent")
	ctx.Request.Header.Set("Referer", "http://example.com/")

	logger.LogAccess(ctx, 201, 512, 250*time.Millisecond)

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	output := buf.String()
	expectedFields := []string{"request", "status", "body_bytes_sent", "request_time", "remote_addr"}

	for _, field := range expectedFields {
		if !strings.Contains(output, field) {
			t.Errorf("Expected output to contain '%s'", field)
		}
	}
}

func TestLoggerDebug(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger := New(&config.LoggingConfig{Error: config.ErrorLogConfig{Level: "debug"}})

	logger.Debug().Msg("debug message")
	logger.Info().Msg("info message")

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Error("Expected debug message to be logged")
	}
	if !strings.Contains(output, "info message") {
		t.Error("Expected info message to be logged")
	}
}

func TestLoggerWarn(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger := New(&config.LoggingConfig{Error: config.ErrorLogConfig{Level: "warn"}})

	logger.Warn().Msg("warn message")
	logger.Error().Msg("error message")

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	output := buf.String()
	if !strings.Contains(output, "warn message") {
		t.Error("Expected warn message to be logged")
	}
	if !strings.Contains(output, "error message") {
		t.Error("Expected error message to be logged")
	}
}

func TestInit(t *testing.T) {
	tests := []struct {
		name   string
		level  string
		format string
	}{
		{"debug console", "debug", "console"},
		{"info json", "info", "json"},
		{"warn text", "warn", "text"},
		{"error json", "error", "json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			Init(tt.level, tt.format)
			// 验证全局 logger 已初始化
			Debug().Msg("test debug")
			Info().Msg("test info")
			Warn().Msg("test warn")
			Error().Msg("test error")
		})
	}
}

func TestGlobalLogFunctions(_ *testing.T) {
	Init("debug", "json")

	// 测试全局日志函数
	Debug().Str("key", "value").Msg("global debug")
	Info().Str("key", "value").Msg("global info")
	Warn().Str("key", "value").Msg("global warn")
	Error().Str("key", "value").Msg("global error")
}

func TestLogAccessGlobal(_ *testing.T) {
	Init("info", "json")

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/global-test")
	ctx.Request.Header.SetMethod("GET")

	LogAccess(ctx, 200, 100, 50*time.Millisecond)
}

func TestFormatAccessLog(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "access.log")

	cfg := &config.LoggingConfig{
		Access: config.AccessLogConfig{
			Path:   logPath,
			Format: "$remote_addr - $remote_user [$time] $request $status $body_bytes_sent $request_time",
		},
	}

	logger := New(cfg)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/formatted")
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.Set("Referer", "http://referer.com/")
	ctx.Request.Header.Set("User-Agent", "test-agent")

	logger.LogAccess(ctx, 200, 512, 100*time.Millisecond)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Errorf("Failed to read log file: %v", err)
		return
	}

	output := string(data)
	if !strings.Contains(output, "POST /formatted") {
		t.Error("Expected request in log output")
	}
	if !strings.Contains(output, "200") {
		t.Error("Expected status in log output")
	}
	if !strings.Contains(output, "512") {
		t.Error("Expected size in log output")
	}
}

func TestFormatAccessLogWithUser(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "access.log")

	cfg := &config.LoggingConfig{
		Access: config.AccessLogConfig{
			Path:   logPath,
			Format: "$remote_addr - $remote_user",
		},
	}

	logger := New(cfg)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")
	ctx.Request.Header.SetMethod("GET")
	ctx.SetUserValue("remote_user", "testuser")

	logger.LogAccess(ctx, 200, 0, 0)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Errorf("Failed to read log file: %v", err)
		return
	}

	output := string(data)
	if !strings.Contains(output, "testuser") {
		t.Error("Expected remote_user in log output")
	}
}

func TestNewAppLogger(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.LoggingConfig
	}{
		{"nil config", nil},
		{"empty config", &config.LoggingConfig{}},
		{"text format", &config.LoggingConfig{Format: "text"}},
		{"json format", &config.LoggingConfig{Format: "json"}},
		{"with error level", &config.LoggingConfig{Error: config.ErrorLogConfig{Level: "debug"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			logger := NewAppLogger(tt.cfg)
			if logger == nil {
				t.Error("Expected non-nil AppLogger")
			}
		})
	}
}

func TestAppLoggerLogStartup(t *testing.T) {
	tests := []struct {
		name   string
		format string
		fields map[string]string
	}{
		{"text no fields", "text", nil},
		{"text with fields", "text", map[string]string{"version": "1.0", "port": "8080"}},
		{"json no fields", "json", nil},
		{"json with fields", "json", map[string]string{"version": "1.0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			logger := NewAppLogger(&config.LoggingConfig{Format: tt.format})
			logger.LogStartup("server started", tt.fields)
		})
	}
}

func TestAppLoggerLogShutdown(t *testing.T) {
	tests := []struct {
		name   string
		format string
	}{
		{"text format", "text"},
		{"json format", "json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			logger := NewAppLogger(&config.LoggingConfig{Format: tt.format})
			logger.LogShutdown("server stopped")
		})
	}
}

func TestAppLoggerLogSignal(t *testing.T) {
	tests := []struct {
		name   string
		format string
		sig    string
		action string
	}{
		{"text SIGTERM", "text", "SIGTERM", "正在关闭服务器"},
		{"json SIGINT", "json", "SIGINT", "正在重新加载配置"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			logger := NewAppLogger(&config.LoggingConfig{Format: tt.format})
			logger.LogSignal(tt.sig, tt.action)
		})
	}
}

func TestAppLoggerMethods(_ *testing.T) {
	logger := NewAppLogger(&config.LoggingConfig{Format: "json", Error: config.ErrorLogConfig{Level: "debug"}})

	logger.Info().Str("test", "value").Msg("app info")
	logger.Error().Str("test", "value").Msg("app error")
}

func TestLoggerClose(t *testing.T) {
	// 测试无文件情况
	logger := New(nil)
	if err := logger.Close(); err != nil {
		t.Errorf("Unexpected error on Close: %v", err)
	}

	// 测试有文件情况
	tmpDir := t.TempDir()
	accessPath := filepath.Join(tmpDir, "access.log")
	errorPath := filepath.Join(tmpDir, "error.log")

	cfg := &config.LoggingConfig{
		Access: config.AccessLogConfig{Path: accessPath},
		Error:  config.ErrorLogConfig{Path: errorPath},
	}

	logger2 := New(cfg)
	if err := logger2.Close(); err != nil {
		t.Errorf("Unexpected error on Close: %v", err)
	}
}
