//go:build windows

// Package app 提供 Windows 平台的应用程序逻辑 stub。
//
// Windows 不支持 POSIX 信号（SIGUSR1、SIGUSR2、SIGHUP、SIGQUIT），
// 该文件提供兼容的实现，忽略这些 Unix 特有的信号。
//
// 作者：xfy
package app

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/http2"
	"rua.plus/lolly/internal/http3"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/resolver"
	"rua.plus/lolly/internal/server"
	"rua.plus/lolly/internal/stream"
	"rua.plus/lolly/internal/variable"
)

// 版本信息，通过 -ldflags 注入。
var (
	Version       = "dev"
	GitCommit     = "unknown"
	GitBranch     = "unknown"
	BuildTime     = "unknown"
	GoVersion     = "unknown"
	BuildPlatform = "unknown"
)

// App 应用程序结构（Windows 版本）。
type App struct {
	cfgPath    string
	cfg        *config.Config
	srv        *server.Server
	http3Srv   *http3.Server
	http2Srv   *http2.Server
	streamSrv  *stream.Server
	upgradeMgr *server.UpgradeManager
	pidFile    string
	logFile    string
	listeners  []net.Listener
	logger     *logging.AppLogger
	resv       resolver.Resolver
}

// NewApp 创建应用程序。
func NewApp(cfgPath string) *App {
	return &App{cfgPath: cfgPath}
}

// SetPidFile 设置 PID 文件路径。
func (a *App) SetPidFile(path string) {
	a.pidFile = path
}

// SetLogFile 设置日志文件路径。
func (a *App) SetLogFile(path string) {
	a.logFile = path
}

// Run 应用程序入口。
func Run(cfgPath string, genConfig bool, outputPath string, showVersion bool) int {
	if genConfig {
		return generateConfig(outputPath)
	}
	if showVersion {
		printVersion()
		return 0
	}
	app := NewApp(cfgPath)
	return app.Run()
}

// generateConfig 生成默认配置文件。
func generateConfig(outputPath string) int {
	cfg := config.DefaultConfig()
	yamlData, err := config.GenerateConfigYAML(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成配置失败: %v\n", err)
		return 1
	}
	if outputPath == "" {
		fmt.Print(string(yamlData))
	} else {
		if err := os.WriteFile(outputPath, yamlData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "写入文件失败: %v\n", err)
			return 1
		}
		fmt.Printf("配置已写入: %s\n", outputPath)
	}
	return 0
}

// printVersion 打印版本信息。
func printVersion() {
	fmt.Printf("lolly version %s\n", Version)
	fmt.Printf("  Git: %s (%s)\n", GitCommit, GitBranch)
	fmt.Printf("  Built: %s\n", BuildTime)
	fmt.Printf("  Go: %s\n", GoVersion)
	fmt.Printf("  Platform: %s\n", BuildPlatform)
}

// Run 启动应用程序。
func (a *App) Run() int {
	cfg, err := config.Load(a.cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		return 1
	}
	a.cfg = cfg
	a.logger = logging.NewAppLogger(&cfg.Logging)

	variable.SetGlobalVariables(cfg.Variables.Set)
	if len(cfg.Variables.Set) > 0 {
		a.logger.LogStartup("全局变量已加载", map[string]string{
			"count": fmt.Sprintf("%d", len(cfg.Variables.Set)),
		})
	}

	a.logger.LogStartup("配置加载成功", map[string]string{"config_path": a.cfgPath})
	a.logger.LogStartup("监听地址", map[string]string{"listen": a.cfg.Server.Listen})

	if a.cfg.Resolver.Enabled {
		a.resv = resolver.New(&a.cfg.Resolver)
		a.logger.LogStartup("DNS 解析器已启用", map[string]string{
			"addresses": fmt.Sprintf("%v", a.cfg.Resolver.Addresses),
			"ttl":       a.cfg.Resolver.TTL().String(),
		})
	}

	a.srv = server.New(a.cfg)
	if a.resv != nil {
		a.srv.SetResolver(a.resv)
	}

	// Stream 服务器
	if len(a.cfg.Stream) > 0 {
		a.streamSrv = stream.NewServer()
		for _, sc := range a.cfg.Stream {
			targets := make([]stream.TargetSpec, len(sc.Upstream.Targets))
			for i, t := range sc.Upstream.Targets {
				targets[i] = stream.TargetSpec{
					Addr:   t.Addr,
					Weight: t.Weight,
				}
			}
			if err := a.streamSrv.AddUpstream(sc.Listen, targets, sc.Upstream.LoadBalance, stream.HealthCheckSpec{}); err != nil {
				a.logger.Error().Err(err).Msg("添加 Stream 上游失败")
			}
			if sc.Protocol == "udp" {
				if err := a.streamSrv.ListenUDP(sc.Listen, sc.Listen, 60*time.Second); err != nil {
					a.logger.Error().Err(err).Str("listen", sc.Listen).Msg("监听 UDP 失败")
				}
			} else {
				if err := a.streamSrv.ListenTCP(sc.Listen); err != nil {
					a.logger.Error().Err(err).Str("listen", sc.Listen).Msg("监听 TCP 失败")
				}
			}
		}
		go func() {
			a.logger.LogStartup("Stream 服务器启动中", nil)
			if err := a.streamSrv.Start(); err != nil {
				a.logger.Error().Err(err).Msg("Stream 服务器启动失败")
			}
		}()
	}

	// HTTP/3 服务器
	if a.cfg.HTTP3.Enabled && a.cfg.Server.SSL.Cert != "" {
		tlsConfig, err := a.srv.GetTLSConfig()
		if err != nil {
			a.logger.Error().Err(err).Msg("获取 TLS 配置失败，跳过 HTTP/3")
		} else {
			a.http3Srv, err = http3.NewServer(&a.cfg.HTTP3, a.srv.GetHandler(), tlsConfig)
			if err != nil {
				a.logger.Error().Err(err).Msg("创建 HTTP/3 服务器失败")
			} else {
				go func() {
					a.logger.LogStartup("HTTP/3 服务器启动中", map[string]string{"listen": a.cfg.HTTP3.Listen})
					if err := a.http3Srv.Start(); err != nil {
						a.logger.Error().Err(err).Msg("HTTP/3 服务器启动失败")
					}
				}()
			}
		}
	}

	// HTTP/2 服务器
	if a.cfg.Server.SSL.HTTP2.Enabled && a.cfg.Server.SSL.Cert != "" {
		tlsConfig, err := a.srv.GetTLSConfig()
		if err != nil {
			a.logger.Error().Err(err).Msg("获取 TLS 配置失败，跳过 HTTP/2")
		} else {
			a.http2Srv, err = http2.NewServer(&a.cfg.Server.SSL.HTTP2, a.srv.GetHandler(), tlsConfig)
			if err != nil {
				a.logger.Error().Err(err).Msg("创建 HTTP/2 服务器失败")
			} else {
				go func() {
					a.logger.LogStartup("HTTP/2 服务器启动中", map[string]string{
						"listen":                 a.cfg.Server.Listen,
						"max_concurrent_streams": fmt.Sprintf("%d", a.cfg.Server.SSL.HTTP2.MaxConcurrentStreams),
						"push_enabled":           fmt.Sprintf("%t", a.cfg.Server.SSL.HTTP2.PushEnabled),
					})
					listeners := a.srv.GetListeners()
					if len(listeners) > 0 {
						if err := a.http2Srv.Serve(listeners[0]); err != nil {
							a.logger.Error().Err(err).Msg("HTTP/2 服务器启动失败")
						}
					} else {
						a.logger.Error().Msg("HTTP/2 服务器启动失败: 无可用监听器")
					}
				}()
			}
		}
	}

	// 创建升级管理器（Windows stub）
	a.upgradeMgr = server.NewUpgradeManager(a.srv)
	if a.pidFile != "" {
		a.upgradeMgr.SetPidFile(a.pidFile)
		_ = a.upgradeMgr.WritePid()
	}

	sigChan := make(chan os.Signal, 1)
	a.setupSignalHandlers(sigChan)

	errChan := make(chan error, 1)
	go func() {
		a.logger.LogStartup("HTTP 服务器启动中", nil)
		if err := a.srv.Start(); err != nil {
			errChan <- err
		}
	}()

	sigintCount := 0

	for {
		select {
		case err := <-errChan:
			a.logger.Error().Err(err).Msg("服务器启动失败")
			return 1
		case sig := <-sigChan:
			if sig == syscall.SIGINT {
				sigintCount++
				if sigintCount >= 3 {
					a.logger.LogShutdown("收到 3 次 SIGINT，强制退出")
					return 1
				}
			}
			if !a.handleSignal(sig) {
				a.logger.LogShutdown("服务器已停止")
				return 0
			}
		}
	}
}

// setupSignalHandlers 设置信号处理（Windows 版本）。
//
// Windows 仅支持 SIGINT 和 SIGTERM，忽略 Unix 特有的信号。
func (a *App) setupSignalHandlers(sigChan chan<- os.Signal) {
	signal.Notify(sigChan,
		syscall.SIGTERM,
		syscall.SIGINT,
	)
}

// handleSignal 处理信号（Windows 版本）。
//
// Windows 仅处理 SIGTERM 和 SIGINT，其他信号忽略并继续运行。
func (a *App) handleSignal(sig os.Signal) bool {
	switch sig {
	case syscall.SIGTERM, syscall.SIGINT:
		// 快速停止
		timeout := a.cfg.Shutdown.FastTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second // 默认值
		}
		a.logger.LogSignal(sigName(sig.(syscall.Signal)), "停止服务器")
		a.shutdownHTTP2()
		a.shutdownHTTP3()
		_ = a.srv.StopWithTimeout(timeout)
		return false
	default:
		a.logger.Info().Str("signal", sig.String()).Msg("收到信号（Windows 忽略）")
		return true
	}
}

// shutdownHTTP3 关闭 HTTP/3 服务器。
func (a *App) shutdownHTTP3() {
	if a.http3Srv != nil {
		if err := a.http3Srv.Stop(); err != nil {
			a.logger.Error().Err(err).Msg("HTTP/3 服务器关闭失败")
		}
	}
}

// shutdownHTTP2 关闭 HTTP/2 服务器。
func (a *App) shutdownHTTP2() {
	if a.http2Srv != nil {
		if err := a.http2Srv.Stop(); err != nil {
			a.logger.Error().Err(err).Msg("HTTP/2 服务器关闭失败")
		}
	}
}

// reloadConfig 重载配置（Windows stub）。
func (a *App) reloadConfig() {
	// Windows stub - 功能受限
}

// reopenLogs 重新打开日志文件（Windows stub）。
func (a *App) reopenLogs() {
	if a.cfg != nil {
		logging.Init(a.cfg.Logging.Error.Level, false)
		a.logger = logging.NewAppLogger(&a.cfg.Logging)
	}
	a.logger.LogStartup("日志已重新打开", nil)
}

// gracefulUpgrade 执行热升级（Windows stub）。
//
// Windows 不支持热升级，此方法为空实现。
func (a *App) gracefulUpgrade() {
	a.logger.Info().Msg("Windows 不支持热升级")
}

// sigName 返回信号名称（Windows 版本）。
func sigName(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGINT:
		return "SIGINT"
	default:
		return fmt.Sprintf("Signal(%d)", sig)
	}
}
