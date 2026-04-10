// Package app 提供应用程序的启动和运行逻辑。
//
// 该文件包含应用程序相关的核心逻辑，包括：
//   - 应用程序生命周期管理
//   - 信号处理（优雅停止、重载配置、热升级）
//   - 配置加载和版本信息
//
// 主要用途：
//
//	用于启动和管理服务器进程，处理系统信号和运行时操作。
//
// 注意事项：
//   - 支持热升级（USR2 信号）
//   - 支持配置重载（HUP 信号）
//   - 支持日志重新打开（USR1 信号）
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
	// Version 版本号
	Version = "dev"
	// GitCommit Git 提交哈希
	GitCommit = "unknown"
	// GitBranch Git 分支名
	GitBranch = "unknown"
	// BuildTime 构建时间
	BuildTime = "unknown"
	// GoVersion Go 版本
	GoVersion = "unknown"
	// BuildPlatform 构建平台
	BuildPlatform = "unknown"
)

// 应用状态。
var (
	// shutdownTimeout 优雅停止超时时间
	shutdownTimeout = 30 * time.Second
)

// App 应用程序结构。
//
// 管理服务器的完整生命周期，包括 HTTP 服务器、HTTP/3 服务器、Stream 服务器
// 和热升级管理器。
type App struct {
	// cfgPath 配置文件路径
	cfgPath string

	// cfg 配置对象
	cfg *config.Config

	// srv HTTP 服务器实例
	srv *server.Server

	// http3Srv HTTP/3 服务器实例（可选）
	http3Srv *http3.Server

	// http2Srv HTTP/2 服务器实例（可选）
	http2Srv *http2.Server

	// streamSrv Stream 服务器实例（可选）
	streamSrv *stream.Server

	// upgradeMgr 热升级管理器
	upgradeMgr *server.UpgradeManager

	// pidFile PID 文件路径
	pidFile string

	// logFile 日志文件路径（用于重新打开）
	logFile string

	// listeners 继承的监听器（热升级时使用）
	listeners []net.Listener

	// logger 应用日志管理器
	logger *logging.AppLogger

	// resolver DNS 解析器（可选）
	resv resolver.Resolver
}

// NewApp 创建应用程序。
func NewApp(cfgPath string) *App {
	return &App{
		cfgPath: cfgPath,
	}
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
	// 加载配置
	cfg, err := config.Load(a.cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		return 1
	}
	a.cfg = cfg
	a.logger = logging.NewAppLogger(&cfg.Logging)

	// 设置全局变量
	variable.SetGlobalVariables(cfg.Variables.Set)
	if len(cfg.Variables.Set) > 0 {
		a.logger.LogStartup("全局变量已加载", map[string]string{
			"count": fmt.Sprintf("%d", len(cfg.Variables.Set)),
		})
	}

	// 检查是否是子进程（热升级）
	if os.Getenv("GRACEFUL_UPGRADE") == "1" {
		a.logger.LogStartup("检测到热升级模式，继承父进程监听器", nil)
		// 创建升级管理器以获取继承的监听器
		a.upgradeMgr = server.NewUpgradeManager(nil)
		listeners, err := a.upgradeMgr.GetInheritedListeners()
		if err == nil && len(listeners) > 0 {
			// 暂时保存监听器，等服务器创建后再设置
			a.listeners = listeners
		}
	}

	a.logger.LogStartup("配置加载成功", map[string]string{"config_path": a.cfgPath})
	a.logger.LogStartup("监听地址", map[string]string{"listen": a.cfg.Server.Listen})

	// 创建 DNS 解析器（如果启用）
	if a.cfg.Resolver.Enabled {
		a.resv = resolver.New(&a.cfg.Resolver)
		a.logger.LogStartup("DNS 解析器已启用", map[string]string{
			"addresses": fmt.Sprintf("%v", a.cfg.Resolver.Addresses),
			"ttl":       a.cfg.Resolver.TTL().String(),
		})
	}

	// 创建 HTTP 服务器
	a.srv = server.New(a.cfg)

	// 设置 DNS 解析器到服务器
	if a.resv != nil {
		a.srv.SetResolver(a.resv)
	}

	// 如果有继承的监听器，设置到服务器
	if len(a.listeners) > 0 {
		a.srv.SetListeners(a.listeners)
	}

	// 创建 Stream 服务器（如果配置了）
	if len(a.cfg.Stream) > 0 {
		a.streamSrv = stream.NewServer()
		for _, sc := range a.cfg.Stream {
			// 转换目标配置
			targets := make([]stream.TargetSpec, len(sc.Upstream.Targets))
			for i, t := range sc.Upstream.Targets {
				targets[i] = stream.TargetSpec{
					Addr:   t.Addr,
					Weight: t.Weight,
				}
			}

			// 添加上游配置
			if err := a.streamSrv.AddUpstream(sc.Listen, targets, sc.Upstream.LoadBalance, stream.HealthCheckSpec{}); err != nil {
				a.logger.Error().Err(err).Msg("添加 Stream 上游失败")
			}

			// 监听端口
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

		// 启动 Stream 服务器
		go func() {
			a.logger.LogStartup("Stream 服务器启动中", nil)
			if err := a.streamSrv.Start(); err != nil {
				a.logger.Error().Err(err).Msg("Stream 服务器启动失败")
			}
		}()
	}

	// 创建并启动 HTTP/3 服务器（如果启用）
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

	// 创建并启动 HTTP/2 服务器（如果启用且配置了 TLS）
	if a.cfg.Server.SSL.HTTP2.Enabled && a.cfg.Server.SSL.Cert != "" {
		tlsConfig, err := a.srv.GetTLSConfig()
		if err != nil {
			a.logger.Error().Err(err).Msg("获取 TLS 配置失败，跳过 HTTP/2")
		} else {
			// 创建 HTTP/2 服务器，共享同一个 handler
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
					// HTTP/2 服务器使用与主服务器相同的监听器
					// 通过 ALPN 协商自动处理协议选择
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

	// 创建升级管理器
	a.upgradeMgr = server.NewUpgradeManager(a.srv)
	if a.pidFile != "" {
		a.upgradeMgr.SetPidFile(a.pidFile)
		_ = a.upgradeMgr.WritePid()
	}

	// 启动信号处理
	sigChan := make(chan os.Signal, 1)
	a.setupSignalHandlers(sigChan)

	// 启动 HTTP 服务器
	errChan := make(chan error, 1)
	go func() {
		a.logger.LogStartup("HTTP 服务器启动中", nil)
		if err := a.srv.Start(); err != nil {
			errChan <- err
		}
	}()

	// SIGINT 计数器，用于强制退出
	sigintCount := 0

	// 等待信号或启动错误
	for {
		select {
		case err := <-errChan:
			a.logger.Error().Err(err).Msg("服务器启动失败")
			return 1
		case sig := <-sigChan:
			// 多次 SIGINT 强制退出
			if sig == syscall.SIGINT {
				sigintCount++
				if sigintCount >= 3 {
					a.logger.LogShutdown("收到 3 次 SIGINT，强制退出")
					return 1
				}
			}
			if !a.handleSignal(sig) {
				// 返回 false 表示退出
				a.logger.LogShutdown("服务器已停止")
				return 0
			}
			// 返回 true 表示继续运行（如重载配置）
		}
	}
}

// setupSignalHandlers 设置信号处理。
func (a *App) setupSignalHandlers(sigChan chan<- os.Signal) {
	signal.Notify(sigChan,
		syscall.SIGTERM, // 快速停止（kill 或 systemd stop）
		syscall.SIGINT,  // 快速停止（Ctrl+C）
		syscall.SIGQUIT, // 优雅停止
		syscall.SIGHUP,  // 重载配置
		syscall.SIGUSR1, // 重新打开日志
		syscall.SIGUSR2, // 热升级
	)
}

// handleSignal 处理信号，返回 false 表示退出。
func (a *App) handleSignal(sig os.Signal) bool {
	switch sig {
	case syscall.SIGQUIT:
		// 优雅停止：等待请求完成
		a.logger.LogSignal("SIGQUIT", fmt.Sprintf("优雅停止（等待 %v）", shutdownTimeout))
		a.shutdownHTTP2()
		a.shutdownHTTP3()
		_ = a.srv.GracefulStop(shutdownTimeout)
		return false

	case syscall.SIGTERM, syscall.SIGINT:
		// 快速停止
		a.logger.LogSignal(sigName(sig.(syscall.Signal)), "停止服务器")
		a.shutdownHTTP2()
		a.shutdownHTTP3()
		_ = a.srv.Stop()
		return false

	case syscall.SIGHUP:
		// 重载配置
		a.logger.LogSignal("SIGHUP", "重载配置")
		a.reloadConfig()
		return true

	case syscall.SIGUSR1:
		// 重新打开日志
		a.logger.LogSignal("SIGUSR1", "重新打开日志")
		a.reopenLogs()
		return true

	case syscall.SIGUSR2:
		// 热升级
		a.logger.LogSignal("SIGUSR2", "执行热升级")
		a.gracefulUpgrade()
		return true

	default:
		a.logger.Info().Str("signal", sig.String()).Msg("收到未知信号")
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

// reloadConfig 重载配置。
func (a *App) reloadConfig() {
	newCfg, err := config.Load(a.cfgPath)
	if err != nil {
		a.logger.Error().Err(err).Msg("重载配置失败")
		return
	}

	// 更新配置
	a.cfg = newCfg
	a.logger = logging.NewAppLogger(&newCfg.Logging)
	a.logger.LogStartup("配置重载成功", nil)
}

// reopenLogs 重新打开日志文件。
func (a *App) reopenLogs() {
	// 重新初始化日志系统
	if a.cfg != nil {
		logging.Init(a.cfg.Logging.Error.Level, false)
		a.logger = logging.NewAppLogger(&a.cfg.Logging)
	}
	a.logger.LogStartup("日志已重新打开", nil)
}

// gracefulUpgrade 执行热升级。
func (a *App) gracefulUpgrade() {
	// 获取当前可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		a.logger.Error().Err(err).Msg("获取可执行文件路径失败")
		return
	}

	// 尝试从服务器获取监听器
	listeners := a.srv.GetListeners()
	if len(listeners) == 0 {
		a.logger.Error().Msg("热升级失败: 服务器未保存监听器（热升级当前未完全实现）")
		a.logger.Info().Msg("提示: 热升级需要服务器使用手动监听器管理模式")
		return
	}

	// 设置监听器到升级管理器
	a.upgradeMgr.SetListeners(listeners)

	// 执行升级
	if err := a.upgradeMgr.GracefulUpgrade(execPath); err != nil {
		a.logger.Error().Err(err).Msg("热升级失败")
		return
	}

	a.logger.LogStartup("热升级已启动，新进程正在接管", nil)

	// 当前进程优雅停止
	a.shutdownHTTP2()
	a.shutdownHTTP3()
	_ = a.srv.GracefulStop(shutdownTimeout)
}

// sigName 返回信号名称（用于日志输出）。
func sigName(sig syscall.Signal) string {
	//nolint:exhaustive
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
