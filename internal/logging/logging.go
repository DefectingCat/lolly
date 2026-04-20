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
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/variable"
)

// Logger 日志管理器，分离访问日志和错误日志。
type Logger struct {
	accessLog    zerolog.Logger
	errorLog     zerolog.Logger
	accessWriter io.Writer
	accessFile   *os.File
	errorFile    *os.File
	accessFormat string
}

// AppLogger 应用日志管理器，统一管理启动/停止日志。
type AppLogger struct {
	errorLog zerolog.Logger
	writer   io.Writer
	format   string
}

var log zerolog.Logger

const formatJSON = "json"

const formatText = "text"

// Init 初始化全局日志系统（兼容旧接口）。
//
// 该函数用于快速初始化全局 log 实例，支持 console、text 和 json 三种格式。
// 不指定输出路径时默认输出到标准输出。
//
// 参数：
//   - level: 日志级别，支持 debug、info、warn、error（不区分大小写）
//   - format: 日志格式，"console" 为带时间彩色、"text" 为纯文本、"json" 为结构化 JSON
func Init(level string, format string) {
	l := parseLevel(level)
	w := getOutput("") // stdout

	switch format {
	case "console":
		log = zerolog.New(zerolog.ConsoleWriter{Out: w, TimeFormat: time.RFC3339}).Level(l).With().Timestamp().Logger()
	case formatText:
		// text 格式：使用 ConsoleWriter 但不带颜色
		log = zerolog.New(zerolog.ConsoleWriter{Out: w, TimeFormat: time.RFC3339, NoColor: true}).Level(l).With().Timestamp().Logger()
	default:
		// json 或空格式：使用 JSON
		log = zerolog.New(w).Level(l).With().Timestamp().Logger()
	}
}

// New 创建日志管理器，支持访问/错误日志分离。
//
// 根据配置创建 Logger 实例，访问日志和错误日志可分别输出到不同路径。
// 配置为 nil 时使用默认设置（全部输出到标准输出）。
//
// 参数：
//   - cfg: 日志配置，包含访问日志和错误日志的输出路径、级别、格式等
//
// 返回值：
//   - *Logger: 初始化的日志管理器实例
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
//
// 根据路径返回对应的输出 writer：
//   - "stdout" 或空字符串：返回 os.Stdout
//   - "stderr"：返回 os.Stderr
//   - 其他：尝试打开文件，失败返回 os.Stdout
//
// 参数：
//   - path: 输出路径
//
// 返回值：
//   - io.Writer: 输出 writer
func getOutput(path string) io.Writer {
	path = strings.TrimSpace(path)
	if path == "" || path == "stdout" {
		return os.Stdout
	}
	if path == "stderr" {
		return os.Stderr
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return os.Stdout
	}
	return f
}

// LogAccess 记录访问日志（全局实例）。
//
// 使用全局 log 实例记录 HTTP 请求的基本信息，包括方法、路径、状态码、
// 响应大小、耗时和客户端地址。
//
// 参数：
//   - ctx: FastHTTP 请求上下文，用于提取请求信息
//   - status: HTTP 响应状态码
//   - size: 响应体大小（字节）
//   - duration: 请求处理耗时
func LogAccess(ctx *fasthttp.RequestCtx, status int, size int64, duration time.Duration) {
	log.Info().
		Bytes("method", ctx.Method()).
		Bytes("path", ctx.Path()).
		Int("status", status).
		Int64("size", size).
		Dur("duration", duration).
		Str("remote_addr", ctx.RemoteAddr().String()).
		Msg("request")
}

// LogAccess 记录访问日志，支持模板格式或 JSON。
func (l *Logger) LogAccess(ctx *fasthttp.RequestCtx, status int, size int64, duration time.Duration) {
	// JSON 格式或空格式：输出结构化 JSON
	if l.accessFormat == formatJSON || l.accessFormat == "" {
		l.accessLog.Info().
			Str("remote_addr", ctx.RemoteAddr().String()).
			Bytes("request", append(append(ctx.Method(), ' '), ctx.Path()...)).
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
//
// 使用变量系统展开模板字符串，支持以下变量：
//   - $remote_addr: 客户端地址
//   - $remote_user: 认证用户
//   - $request: 请求方法和路径
//   - $status: HTTP 状态码
//   - $body_bytes_sent: 响应体大小
//   - $request_time: 请求处理时间
//   - $http_referer: Referer 头
//   - $http_user_agent: User-Agent 头
//   - $time_local, $time_iso8601: 时间
//   - $host, $uri, $args: 请求信息
//
// 参数：
//   - ctx: FastHTTP 请求上下文
//   - status: HTTP 状态码
//   - size: 响应体大小
//   - duration: 请求处理时间
//
// 返回值：
//   - string: 格式化后的日志字符串
func (l *Logger) formatAccessLog(ctx *fasthttp.RequestCtx, status int, size int64, duration time.Duration) string {
	// 获取认证用户名，无认证时为 "-"
	remoteUser := "-"
	if user := ctx.UserValue("remote_user"); user != nil {
		if username, ok := user.(string); ok && username != "" {
			remoteUser = username
		}
	}

	// 创建变量上下文
	vc := variable.NewContext(ctx)
	defer variable.ReleaseContext(vc)

	// 设置响应信息（同时设置到 ctx 供 builtin getter 使用）
	vc.SetResponseInfo(status, size, duration.Nanoseconds())
	variable.SetResponseInfoInContext(ctx, status, size, duration.Nanoseconds())

	// 设置自定义变量（用于兼容旧的变量名）
	vc.Set("remote_user", remoteUser)
	vc.Set("request", string(ctx.Method())+" "+string(ctx.Path())+" "+string(ctx.Request.Header.Protocol()))
	vc.Set("http_referer", string(ctx.Request.Header.Peek("Referer")))
	vc.Set("http_user_agent", string(ctx.Request.Header.Peek("User-Agent")))
	// 添加 $time 别名（兼容旧格式）
	vc.Set("time", time.Now().Format(time.RFC3339))

	// 展开模板
	return vc.Expand(l.accessFormat)
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
	var err error
	if l.accessFile != nil {
		err = l.accessFile.Close()
	}
	if l.errorFile != nil {
		if closeErr := l.errorFile.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
}

// Error 返回 Error 级别日志记录器（全局实例）。
//
// 用于记录错误信息，调用链式方法添加字段后需调用 Msg() 输出。
//
// 返回值：
//   - *zerolog.Event: 错误级别日志事件，用于链式调用
func Error() *zerolog.Event {
	return log.Error()
}

// Info 返回 Info 级别日志记录器（全局实例）。
//
// 用于记录一般信息日志，调用链式方法添加字段后需调用 Msg() 输出。
//
// 返回值：
//   - *zerolog.Event: 信息级别日志事件，用于链式调用
func Info() *zerolog.Event {
	return log.Info()
}

// Warn 返回 Warn 级别日志记录器（全局实例）。
//
// 用于记录警告信息，调用链式方法添加字段后需调用 Msg() 输出。
//
// 返回值：
//   - *zerolog.Event: 警告级别日志事件，用于链式调用
func Warn() *zerolog.Event {
	return log.Warn()
}

// Debug 返回 Debug 级别日志记录器（全局实例）。
//
// 用于记录调试信息，调用链式方法添加字段后需调用 Msg() 输出。
// 仅在日志级别设置为 debug 时才会实际输出。
//
// 返回值：
//   - *zerolog.Event: 调试级别日志事件，用于链式调用
func Debug() *zerolog.Event {
	return log.Debug()
}

// parseLevel 解析日志级别。
//
// 将字符串级别转换为 zerolog.Level。
// 支持的级别：debug, info, warn, error（不区分大小写）。
// 未知级别默认返回 info。
//
// 参数：
//   - level: 日志级别字符串
//
// 返回值：
//   - zerolog.Level: 解析后的日志级别
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
//
// 根据配置创建 AppLogger 实例，用于统一管理应用生命周期日志
// （启动、停止、信号处理等）。默认使用 text 格式输出到错误日志路径。
//
// 参数：
//   - cfg: 日志配置，包含输出路径、级别、格式等设置
//
// 返回值：
//   - *AppLogger: 初始化的应用日志记录器实例
func NewAppLogger(cfg *config.LoggingConfig) *AppLogger {
	if cfg == nil {
		cfg = &config.LoggingConfig{}
	}

	format := cfg.Format
	if format == "" {
		format = "text" // 默认纯文本
	}

	writer := getOutput(cfg.Error.Path)

	var errorLog zerolog.Logger
	if format == "text" {
		// text 格式：使用 ConsoleWriter（无颜色）
		errorLog = zerolog.New(zerolog.ConsoleWriter{Out: writer, TimeFormat: time.RFC3339, NoColor: true}).Level(parseLevel(cfg.Error.Level)).With().Timestamp().Logger()
	} else {
		// json 格式
		errorLog = zerolog.New(writer).Level(parseLevel(cfg.Error.Level)).With().Timestamp().Logger()
	}

	return &AppLogger{
		format:   format,
		errorLog: errorLog,
		writer:   writer,
	}
}

// LogStartup 记录启动消息。
func (l *AppLogger) LogStartup(msg string, fields map[string]string) {
	event := l.errorLog.Info()
	for k, v := range fields {
		event.Str(k, v)
	}
	event.Msg(msg)
}

// LogShutdown 记录停止消息。
func (l *AppLogger) LogShutdown(msg string) {
	l.errorLog.Info().Msg(msg)
}

// LogSignal 记录信号处理消息。
func (l *AppLogger) LogSignal(sig string, action string) {
	l.errorLog.Info().Str("signal", sig).Str("action", action).Msg("收到信号")
}

// Info 返回 Info 级别日志记录器。
func (l *AppLogger) Info() *zerolog.Event {
	return l.errorLog.Info()
}

// Error 返回 Error 级别日志记录器。
func (l *AppLogger) Error() *zerolog.Event {
	return l.errorLog.Error()
}
