//go:build !windows

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

// App manages the server lifecycle, including HTTP, HTTP/3, Stream servers and graceful upgrades.
type App struct {
	resv       resolver.Resolver
	cfg        *config.Config
	srv        *server.Server
	http3Srv   *http3.Server
	http2Srv   *http2.Server
	streamSrv  *stream.Server
	upgradeMgr *server.UpgradeManager
	logger     *logging.AppLogger
	cfgPath    string
	pidFile    string
	logFile    string
	listeners  []net.Listener
}

// NewApp creates a new App instance with the given config path.
func NewApp(cfgPath string) *App {
	return &App{
		cfgPath: cfgPath,
	}
}

// SetPidFile sets the path to the PID file for the app.
func (a *App) SetPidFile(path string) {
	a.pidFile = path
}

// SetLogFile sets the path to the log file for the app.
func (a *App) SetLogFile(path string) {
	a.logFile = path
}

// Run starts the application: loads config, creates servers, and handles signals.
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

	// Inherit parent listeners when running as a graceful upgrade child.
	if os.Getenv("GRACEFUL_UPGRADE") == "1" {
		a.logger.LogStartup("检测到热升级模式，继承父进程监听器", nil)
		a.upgradeMgr = server.NewUpgradeManager(nil)
		listeners, err := a.upgradeMgr.GetInheritedListeners()
		if err == nil && len(listeners) > 0 {
			a.listeners = listeners
		}
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

	if len(a.listeners) > 0 {
		a.srv.SetListeners(a.listeners)
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
					// HTTP/2 shares the main server's listener; ALPN negotiates protocol selection.
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
	a.srv.SetUpgradeManager(a.upgradeMgr)
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
		syscall.SIGQUIT,
		syscall.SIGHUP,
		syscall.SIGUSR1,
		syscall.SIGUSR2,
	)
}

// handleSignal returns false to indicate the app should exit.
func (a *App) handleSignal(sig os.Signal) bool {
	if a.cfg == nil {
		a.logger.Error().Msg("信号处理失败: 配置为 nil，使用默认超时")
		a.cfg = &config.Config{
			Shutdown: config.ShutdownConfig{
				GracefulTimeout: 30 * time.Second,
				FastTimeout:     5 * time.Second,
			},
		}
	}

	switch sig {
	case syscall.SIGQUIT:
		timeout := a.cfg.Shutdown.GracefulTimeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		a.logger.LogSignal("SIGQUIT", fmt.Sprintf("优雅停止（等待 %v）", timeout))
		a.shutdownHTTP2()
		a.shutdownHTTP3()
		_ = a.srv.GracefulStop(timeout)
		return false

	case syscall.SIGTERM, syscall.SIGINT:
		timeout := a.cfg.Shutdown.FastTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		sigTyped, ok := sig.(syscall.Signal)
		if !ok {
			a.logger.LogSignal("unknown", "停止服务器")
		} else {
			a.logger.LogSignal(sigName(sigTyped), "停止服务器")
		}
		a.shutdownHTTP2()
		a.shutdownHTTP3()
		_ = a.srv.StopWithTimeout(timeout)
		return false

	case syscall.SIGHUP:
		a.logger.LogSignal("SIGHUP", "重载配置")
		a.reloadConfig()
		return true

	case syscall.SIGUSR1:
		a.logger.LogSignal("SIGUSR1", "重新打开日志")
		a.reopenLogs()
		return true

	case syscall.SIGUSR2:
		a.logger.LogSignal("SIGUSR2", "执行热升级")
		a.gracefulUpgrade()
		return true

	default:
		a.logger.Info().Str("signal", sig.String()).Msg("收到未知信号")
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
	newCfg, err := config.Load(a.cfgPath)
	if err != nil {
		a.logger.Error().Err(err).Msg("重载配置失败")
		return
	}

	a.cfg = newCfg
	a.logger = logging.NewAppLogger(&newCfg.Logging)
	a.logger.LogStartup("配置重载成功", nil)
}

func (a *App) reopenLogs() {
	if a.cfg != nil {
		logging.Init(a.cfg.Logging.Error.Level, a.cfg.Logging.Format)
		a.logger = logging.NewAppLogger(&a.cfg.Logging)
	}
	a.logger.LogStartup("日志已重新打开", nil)
}

func (a *App) gracefulUpgrade() {
	execPath, err := os.Executable()
	if err != nil {
		a.logger.Error().Err(err).Msg("获取可执行文件路径失败")
		return
	}

	if a.srv == nil {
		a.logger.Error().Msg("热升级失败: 服务器实例为 nil")
		return
	}

	listeners := a.srv.GetListeners()
	if len(listeners) == 0 {
		a.logger.Error().Msg("热升级失败: 服务器未保存监听器（热升级当前未完全实现）")
		a.logger.Info().Msg("提示: 热升级需要服务器使用手动监听器管理模式")
		return
	}

	a.upgradeMgr.SetListeners(listeners)

	if err := a.upgradeMgr.GracefulUpgrade(execPath); err != nil {
		a.logger.Error().Err(err).Msg("热升级失败")
		return
	}

	a.logger.LogStartup("热升级已启动，新进程正在接管", nil)

	timeout := a.cfg.Shutdown.GracefulTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	a.shutdownHTTP2()
	a.shutdownHTTP3()
	_ = a.srv.GracefulStop(timeout)
}

func sigName(sig syscall.Signal) string {
	//nolint:exhaustive // Only handling app-relevant signals.
	switch sig {
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGQUIT:
		return "SIGQUIT"
	case syscall.SIGHUP:
		return "SIGHUP"
	case syscall.SIGUSR1:
		return "SIGUSR1"
	case syscall.SIGUSR2:
		return "SIGUSR2"
	default:
		return fmt.Sprintf("Signal(%d)", sig)
	}
}
