//go:build windows

// Windows lacks POSIX signals (SIGUSR1, SIGUSR2, SIGHUP, SIGQUIT);
// this file provides stub implementations for those Unix-specific signals.
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

// App manages the server lifecycle (Windows version).
type App struct {
	resv       resolver.Resolver
	cfg        *config.Config
	srv        *server.Server
	http3Srv   *http3.Server
	http2Srv   *http2.Server
	streamSrv  *stream.Server
	logger     *logging.AppLogger
	cfgPath    string
	pidFile    string
	logFile    string
	listeners  []net.Listener
	upgradeMgr *server.UpgradeManager
}

func NewApp(cfgPath string) *App {
	return &App{
		cfgPath: cfgPath,
	}
}

func (a *App) SetPidFile(path string) {
	a.pidFile = path
}

func (a *App) SetLogFile(path string) {
	a.logFile = path
}

// Run starts the application: loads config, creates servers, and handles signals (Windows version).
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

	mode := a.cfg.GetMode()
	if mode == config.ServerModeMultiServer {
		for i, srv := range a.cfg.Servers {
			a.logger.LogStartup("监听地址", map[string]string{
				"index":  fmt.Sprintf("[%d]", i),
				"listen": srv.Listen,
				"name":   srv.Name,
			})
		}
	} else {
		a.logger.LogStartup("监听地址", map[string]string{"listen": a.cfg.Servers[0].Listen})
	}

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

	if a.cfg.HTTP3.Enabled && a.cfg.Servers[0].SSL.Cert != "" {
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

	if a.cfg.Servers[0].SSL.HTTP2.Enabled && a.cfg.Servers[0].SSL.Cert != "" {
		tlsConfig, err := a.srv.GetTLSConfig()
		if err != nil {
			a.logger.Error().Err(err).Msg("获取 TLS 配置失败，跳过 HTTP/2")
		} else {
			a.http2Srv, err = http2.NewServer(&a.cfg.Servers[0].SSL.HTTP2, a.srv.GetHandler(), tlsConfig)
			if err != nil {
				a.logger.Error().Err(err).Msg("创建 HTTP/2 服务器失败")
			} else {
				go func() {
					a.logger.LogStartup("HTTP/2 服务器启动中", map[string]string{
						"listen":                 a.cfg.Servers[0].Listen,
						"max_concurrent_streams": fmt.Sprintf("%d", a.cfg.Servers[0].SSL.HTTP2.MaxConcurrentStreams),
						"push_enabled":           fmt.Sprintf("%t", a.cfg.Servers[0].SSL.HTTP2.PushEnabled),
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

func (a *App) setupSignalHandlers(sigChan chan<- os.Signal) {
	signal.Notify(sigChan,
		syscall.SIGTERM,
		syscall.SIGINT,
	)
}

// handleSignal returns false to indicate the app should exit (Windows version).
func (a *App) handleSignal(sig os.Signal) bool {
	switch sig {
	case syscall.SIGTERM, syscall.SIGINT:
		timeout := a.cfg.Shutdown.FastTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
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

func (a *App) shutdownHTTP3() {
	if a.http3Srv != nil {
		if err := a.http3Srv.Stop(); err != nil {
			a.logger.Error().Err(err).Msg("HTTP/3 服务器关闭失败")
		}
	}
}

func (a *App) shutdownHTTP2() {
	if a.http2Srv != nil {
		if err := a.http2Srv.Stop(); err != nil {
			a.logger.Error().Err(err).Msg("HTTP/2 服务器关闭失败")
		}
	}
}

func (a *App) reloadConfig() {
	// Windows stub - functionality limited
}

func (a *App) reopenLogs() {
	if a.cfg != nil {
		logging.Init(a.cfg.Logging.Error.Level, a.cfg.Logging.Format)
		a.logger = logging.NewAppLogger(&a.cfg.Logging)
	}
	a.logger.LogStartup("日志已重新打开", nil)
}

func (a *App) gracefulUpgrade() {
	a.logger.Info().Msg("Windows 不支持热升级")
}

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
