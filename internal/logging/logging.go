package logging

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
)

// Logger 日志管理器，分离访问日志和错误日志。
type Logger struct {
	accessLog  zerolog.Logger
	errorLog   zerolog.Logger
	accessFile *os.File
	errorFile  *os.File
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

	logger := &Logger{
		accessLog: zerolog.New(getOutput(cfg.Access.Path)).With().Timestamp().Logger(),
		errorLog:  zerolog.New(getOutput(cfg.Error.Path)).Level(parseLevel(cfg.Error.Level)).With().Timestamp().Logger(),
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

// LogAccessWithLogger 使用 Logger 实例记录访问日志（nginx 格式变量）。
func (l *Logger) LogAccess(ctx *fasthttp.RequestCtx, status int, size int64, duration time.Duration) {
	l.accessLog.Info().
		Str("remote_addr", ctx.RemoteAddr().String()).
		Str("request", string(ctx.Method())+" "+string(ctx.Path())).
		Int("status", status).
		Int64("body_bytes_sent", size).
		Dur("request_time", duration).
		Str("http_referrer", string(ctx.Request.Header.Peek("Referer"))).
		Str("http_user_agent", string(ctx.Request.Header.Peek("User-Agent"))).
		Msg("")
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
		l.accessFile.Close()
	}
	if l.errorFile != nil {
		l.errorFile.Close()
	}
	return nil
}

// Error 返回 Error 级别日志记录器（全局实例）。
func Error() *zerolog.Event {
	return log.Error()
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
