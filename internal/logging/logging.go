// Package logging 提供日志管理功能，支持访问日志和错误日志分离。
//
// 该文件包含日志相关的核心逻辑，包括：
//   - 访问日志记录（请求方法、路径、状态码、耗时）
//   - 错误日志记录（Debug、Info、Warn、Error 级别）
//   - 日志格式配置（text 或 json）
//   - 应用生命周期日志（启动、停止、信号）
//
// 主要用途：
//
//	用于记录服务器运行时的各类日志信息，便于监控和排查问题。
//
// 注意事项：
//   - 支持 zerolog 高性能日志库
//   - 访问日志和错误日志可分离输出到不同文件
//
// 作者：xfy
package logging

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// Logger 日志管理器，分离访问日志和错误日志。
type Logger struct {
	accessLog    zerolog.Logger
	errorLog     zerolog.Logger
	accessFormat string    // 访问日志格式模板
	accessWriter io.Writer // 访问日志输出目标
	accessFile   *os.File
	errorFile    *os.File
}

// AppLogger 应用日志管理器，统一管理启动/停止日志。
type AppLogger struct {
	format   string // "text" 或 "json"
	errorLog zerolog.Logger
	writer   io.Writer
}

var log zerolog.Logger

// Init 初始化日志系统（兼容旧接口）。
func Init(level string, pretty bool) {
	l := parseLevel(level)
	if pretty {
		log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).Level(l).With().Timestamp().Logger()
	} else {
		log = zerolog.New(os.Stdout).Level(l).With().Timestamp().Logger()
	}
}

// New 创建日志管理器，支持访问/错误日志分离。
func New(cfg *config.LoggingConfig) *Logger {
	if cfg == nil {
		cfg = &config.LoggingConfig{}
	}

	accessWriter := getOutput(cfg.Access.Path)

	logger := &Logger{
		accessFormat: cfg.Access.Format,
		accessWriter: accessWriter,
		accessLog:    zerolog.New(accessWriter).With().Timestamp().Logger(),
		errorLog:     zerolog.New(getOutput(cfg.Error.Path)).Level(parseLevel(cfg.Error.Level)).With().Timestamp().Logger(),
	}

	return logger
}

// getOutput 获取输出目标（stdout/stderr/文件）。
func getOutput(path string) io.Writer {
	path = strings.TrimSpace(path)
	if path == "" || path == "stdout" {
		return os.Stdout
	}
	if path == "stderr" {
		return os.Stderr
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return os.Stdout
	}
	return f
}

// LogAccess 记录访问日志。
func LogAccess(ctx *fasthttp.RequestCtx, status int, size int64, duration time.Duration) {
	log.Info().
		Str("method", string(ctx.Method())).
		Str("path", string(ctx.Path())).
		Int("status", status).
		Int64("size", size).
		Dur("duration", duration).
		Str("remote_addr", ctx.RemoteAddr().String()).
		Msg("request")
}

// LogAccess 记录访问日志，支持模板格式或 JSON。
func (l *Logger) LogAccess(ctx *fasthttp.RequestCtx, status int, size int64, duration time.Duration) {
	// JSON 格式或空格式：输出结构化 JSON
	if l.accessFormat == "json" || l.accessFormat == "" {
		l.accessLog.Info().
			Str("remote_addr", ctx.RemoteAddr().String()).
			Str("request", string(ctx.Method())+" "+string(ctx.Path())).
			Int("status", status).
			Int64("body_bytes_sent", size).
			Dur("request_time", duration).
			Str("http_referrer", string(ctx.Request.Header.Peek("Referer"))).
			Str("http_user_agent", string(ctx.Request.Header.Peek("User-Agent"))).
			Msg("")
		return
	}

	// 模板格式：直接输出纯文本
	output := l.formatAccessLog(ctx, status, size, duration)
	_, _ = fmt.Fprintln(l.accessWriter, output)
}

// formatAccessLog 根据模板格式化访问日志。
func (l *Logger) formatAccessLog(ctx *fasthttp.RequestCtx, status int, size int64, duration time.Duration) string {
	// 获取认证用户名，无认证时为 "-"
	remoteUser := "-"
	if user := ctx.UserValue("remote_user"); user != nil {
		if username, ok := user.(string); ok && username != "" {
			remoteUser = username
		}
	}

	replacements := map[string]string{
		"$remote_addr":     ctx.RemoteAddr().String(),
		"$remote_user":     remoteUser,
		"$request":         string(ctx.Method()) + " " + string(ctx.Path()) + " " + string(ctx.Request.Header.Protocol()),
		"$status":          strconv.Itoa(status),
		"$body_bytes_sent": strconv.FormatInt(size, 10),
		"$request_time":    fmt.Sprintf("%.6f", duration.Seconds()),
		"$http_referer":    string(ctx.Request.Header.Peek("Referer")),
		"$http_user_agent": string(ctx.Request.Header.Peek("User-Agent")),
		"$time":            time.Now().Format(time.RFC3339),
	}

	result := l.accessFormat
	for varName, value := range replacements {
		result = strings.ReplaceAll(result, varName, value)
	}
	return result
}

// Debug 返回 Debug 级别日志记录器。
func (l *Logger) Debug() *zerolog.Event {
	return l.errorLog.Debug()
}

// Info 返回 Info 级别日志记录器。
func (l *Logger) Info() *zerolog.Event {
	return l.errorLog.Info()
}

// Warn 返回 Warn 级别日志记录器。
func (l *Logger) Warn() *zerolog.Event {
	return l.errorLog.Warn()
}

// Error 返回 Error 级别日志记录器。
func (l *Logger) Error() *zerolog.Event {
	return l.errorLog.Error()
}

// Close 关闭日志文件。
func (l *Logger) Close() error {
	if l.accessFile != nil {
		_ = l.accessFile.Close()
	}
	if l.errorFile != nil {
		_ = l.errorFile.Close()
	}
	return nil
}

// Error 返回 Error 级别日志记录器（全局实例）。
func Error() *zerolog.Event {
	return log.Error()
}

// Info 返回 Info 级别日志记录器（全局实例）。
func Info() *zerolog.Event {
	return log.Info()
}

// Warn 返回 Warn 级别日志记录器（全局实例）。
func Warn() *zerolog.Event {
	return log.Warn()
}

// Debug 返回 Debug 级别日志记录器（全局实例）。
func Debug() *zerolog.Event {
	return log.Debug()
}

// parseLevel 解析日志级别。
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// NewAppLogger 创建应用日志管理器。
func NewAppLogger(cfg *config.LoggingConfig) *AppLogger {
	if cfg == nil {
		cfg = &config.LoggingConfig{}
	}

	format := cfg.Format
	if format == "" {
		format = "text" // 默认纯文本
	}

	writer := getOutput(cfg.Error.Path)
	errorLog := zerolog.New(writer).Level(parseLevel(cfg.Error.Level)).With().Timestamp().Logger()

	return &AppLogger{
		format:   format,
		errorLog: errorLog,
		writer:   writer,
	}
}

// LogStartup 记录启动消息。
func (l *AppLogger) LogStartup(msg string, fields map[string]string) {
	if l.format == "json" {
		event := l.errorLog.Info()
		for k, v := range fields {
			event.Str(k, v)
		}
		event.Msg(msg)
		return
	}

	// 纯文本格式
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	if len(fields) == 0 {
		_, _ = fmt.Fprintf(l.writer, "[%s] INFO %s\n", timestamp, msg)
		return
	}

	// 带字段的文本格式
	extra := ""
	for k, v := range fields {
		extra += fmt.Sprintf(" %s=%s", k, v)
	}
	_, _ = fmt.Fprintf(l.writer, "[%s] INFO %s%s\n", timestamp, msg, extra)
}

// LogShutdown 记录停止消息。
func (l *AppLogger) LogShutdown(msg string) {
	if l.format == "json" {
		l.errorLog.Info().Msg(msg)
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, _ = fmt.Fprintf(l.writer, "[%s] INFO %s\n", timestamp, msg)
}

// LogSignal 记录信号处理消息。
func (l *AppLogger) LogSignal(sig string, action string) {
	if l.format == "json" {
		l.errorLog.Info().Str("signal", sig).Str("action", action).Msg("")
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, _ = fmt.Fprintf(l.writer, "[%s] INFO 收到 %s，%s\n", timestamp, sig, action)
}

// Info 返回 Info 级别日志记录器。
func (l *AppLogger) Info() *zerolog.Event {
	return l.errorLog.Info()
}

// Error 返回 Error 级别日志记录器。
func (l *AppLogger) Error() *zerolog.Event {
	return l.errorLog.Error()
}
