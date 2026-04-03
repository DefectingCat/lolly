// Package app 提供应用程序的启动和运行逻辑。
package app

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rua.plus/lolly/internal/config"
	"rua.plus/lolly/internal/logging"
	"rua.plus/lolly/internal/server"
	"rua.plus/lolly/internal/stream"
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

// 应用状态。
var (
	shutdownTimeout = 30 * time.Second // 优雅停止超时时间
)

// App 应用程序结构。
type App struct {
	cfgPath      string
	cfg          *config.Config
	srv          *server.Server
	streamSrv    *stream.Server // Stream 服务器（可选）
	upgradeMgr   *server.UpgradeManager
	pidFile      string
	logFile      string // 日志文件路径（用于重新打开）
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
	// 检查是否是子进程（热升级）
	if os.Getenv("GRACEFUL_UPGRADE") == "1" {
		fmt.Println("检测到热升级模式，继承父进程监听器")
	}

	cfg, err := config.Load(a.cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		return 1
	}
	a.cfg = cfg

	fmt.Printf("配置加载成功: %s\n", a.cfgPath)
	fmt.Printf("监听地址: %s\n", cfg.Server.Listen)

	// 创建 HTTP 服务器
	a.srv = server.New(cfg)

	// 创建 Stream 服务器（如果配置了）
	if len(cfg.Stream) > 0 {
		a.streamSrv = stream.NewServer()
		for _, sc := range cfg.Stream {
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
				fmt.Fprintf(os.Stderr, "添加 Stream 上游失败: %v\n", err)
			}

			// 监听端口
			if sc.Protocol == "udp" {
				if err := a.streamSrv.ListenUDP(sc.Listen); err != nil {
					fmt.Fprintf(os.Stderr, "监听 UDP %s 失败: %v\n", sc.Listen, err)
				}
			} else {
				if err := a.streamSrv.ListenTCP(sc.Listen); err != nil {
					fmt.Fprintf(os.Stderr, "监听 TCP %s 失败: %v\n", sc.Listen, err)
				}
			}
		}

		// 启动 Stream 服务器
		go func() {
			fmt.Println("Stream 服务器启动中...")
			if err := a.streamSrv.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Stream 服务器启动失败: %v\n", err)
			}
		}()
	}

	// 创建升级管理器
	a.upgradeMgr = server.NewUpgradeManager(a.srv)
	if a.pidFile != "" {
		a.upgradeMgr.SetPidFile(a.pidFile)
		a.upgradeMgr.WritePid()
	}

	// 启动信号处理
	sigChan := make(chan os.Signal, 1)
	a.setupSignalHandlers(sigChan)

	// 启动 HTTP 服务器
	errChan := make(chan error, 1)
	go func() {
		fmt.Println("HTTP 服务器启动中...")
		if err := a.srv.Start(); err != nil {
			errChan <- err
		}
	}()

	// 等待信号或启动错误
	for {
		select {
		case err := <-errChan:
			fmt.Fprintf(os.Stderr, "服务器启动失败: %v\n", err)
			return 1
		case sig := <-sigChan:
			if !a.handleSignal(sig) {
				// 返回 false 表示退出
				fmt.Println("服务器已停止")
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
		fmt.Printf("\n收到 SIGQUIT，优雅停止（等待 %v）...\n", shutdownTimeout)
		a.srv.GracefulStop(shutdownTimeout)
		return false

	case syscall.SIGTERM, syscall.SIGINT:
		// 快速停止
		fmt.Printf("\n收到 %v，停止服务器...\n", sigName(sig.(syscall.Signal)))
		a.srv.Stop()
		return false

	case syscall.SIGHUP:
		// 重载配置
		fmt.Println("\n收到 SIGHUP，重载配置...")
		a.reloadConfig()
		return true

	case syscall.SIGUSR1:
		// 重新打开日志
		fmt.Println("\n收到 SIGUSR1，重新打开日志...")
		a.reopenLogs()
		return true

	case syscall.SIGUSR2:
		// 热升级
		fmt.Println("\n收到 SIGUSR2，执行热升级...")
		a.gracefulUpgrade()
		return true

	default:
		fmt.Printf("\n收到未知信号: %v\n", sig)
		return true
	}
}

// reloadConfig 重载配置。
func (a *App) reloadConfig() {
	newCfg, err := config.Load(a.cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "重载配置失败: %v\n", err)
		return
	}

	// 更新配置
	a.cfg = newCfg
	fmt.Println("配置重载成功")

	// 注意：当前实现不重启服务器，仅更新配置
	// 如需应用新配置，需要重启服务器或实现热更新
	fmt.Println("配置已重新加载")
}

// reopenLogs 重新打开日志文件。
func (a *App) reopenLogs() {
	// 重新初始化日志系统
	if a.cfg != nil {
		logging.Init(a.cfg.Logging.Error.Level, false)
	}
	fmt.Println("日志已重新打开")
}

// gracefulUpgrade 执行热升级。
func (a *App) gracefulUpgrade() {
	// 获取当前可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取可执行文件路径失败: %v\n", err)
		return
	}

	// 执行升级
	if err := a.upgradeMgr.GracefulUpgrade(execPath); err != nil {
		fmt.Fprintf(os.Stderr, "热升级失败: %v\n", err)
		return
	}

	fmt.Println("热升级已启动，新进程正在接管")

	// 当前进程优雅停止
	a.srv.GracefulStop(shutdownTimeout)
}

// sigName 返回信号名称（用于日志输出）。
func sigName(sig syscall.Signal) string {
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
