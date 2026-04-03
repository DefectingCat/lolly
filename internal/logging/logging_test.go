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
		t.Run(tt.name, func(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
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
	_ = logger.Close()

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
