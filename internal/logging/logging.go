package logging

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
)

var log zerolog.Logger

// Init 初始化日志系统
func Init(level string, pretty bool) {
	l := parseLevel(level)
	if pretty {
		log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).Level(l).With().Timestamp().Logger()
	} else {
		log = zerolog.New(os.Stdout).Level(l).With().Timestamp().Logger()
	}
}

// LogAccess 记录访问日志
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

// Error 返回 Error 级别日志记录器
func Error() *zerolog.Event {
	return log.Error()
}

// parseLevel 解析日志级别
func parseLevel(level string) zerolog.Level {
	switch level {
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