//go:build !windows

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
)

// Run starts the application: loads config, creates servers, and handles signals.
func (a *App) Run() int {
	if err := a.loadAndValidateConfig(); err != nil {
		return 1
	}

	a.initVariables()

	// Inherit parent listeners when running as a graceful upgrade child.
	a.inheritListeners()

	a.logServerAddresses()
	a.initResolver()
	a.initServer()
	a.initStreamServers()
	a.initHTTP3()
	a.initHTTP2()

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
		a.logger.LogStartup("Starting HTTP server", nil)
		if err := a.srv.Start(); err != nil {
			errChan <- err
		}
	}()

	sigintCount := 0

	for {
		select {
		case err := <-errChan:
			a.logger.Error().Err(err).Msg("Server failed to start")
			return 1
		case sig := <-sigChan:
			if sig == syscall.SIGINT {
				sigintCount++
				if sigintCount >= 3 {
					a.logger.LogShutdown("Received 3 SIGINT, forcing exit")
					return 1
				}
			}
			if !a.handleSignal(sig) {
				a.logger.LogShutdown("Server stopped")
				return 0
			}
		}
	}
}

// inheritListeners inherits parent listeners during graceful upgrade.
func (a *App) inheritListeners() {
	if os.Getenv("GRACEFUL_UPGRADE") == "1" {
		a.logger.LogStartup("Graceful upgrade mode detected, inheriting parent listeners", nil)
		a.upgradeMgr = server.NewUpgradeManager(nil)
		listeners, err := a.upgradeMgr.GetInheritedListeners()
		if err == nil && len(listeners) > 0 {
			a.listeners = listeners
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
		a.logger.Error().Msg("Signal handling failed: config is nil, using default timeout")
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
		a.logger.LogSignal("SIGQUIT", fmt.Sprintf("Graceful stop (waiting %v)", timeout))
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
			a.logger.LogSignal("unknown", "Stopping server")
		} else {
			a.logger.LogSignal(sigName(sigTyped), "Stopping server")
		}
		a.shutdownHTTP2()
		a.shutdownHTTP3()
		_ = a.srv.StopWithTimeout(timeout)
		return false

	case syscall.SIGHUP:
		a.logger.LogSignal("SIGHUP", "Reloading config")
		a.reloadConfig()
		return true

	case syscall.SIGUSR1:
		a.logger.LogSignal("SIGUSR1", "Reopening logs")
		a.reopenLogs()
		return true

	case syscall.SIGUSR2:
		a.logger.LogSignal("SIGUSR2", "Performing graceful upgrade")
		a.gracefulUpgrade()
		return true

	default:
		a.logger.Info().Str("signal", sig.String()).Msg("Received unknown signal")
		return true
	}
}

func (a *App) reloadConfig() {
	newCfg, err := config.Load(a.cfgPath)
	if err != nil {
		a.logger.Error().Err(err).Msg("Failed to reload config")
		return
	}

	a.cfg = newCfg
	a.logger = logging.NewAppLogger(&newCfg.Logging)
	a.logger.LogStartup("Config reloaded successfully", nil)
}

func (a *App) gracefulUpgrade() {
	execPath, err := os.Executable()
	if err != nil {
		a.logger.Error().Err(err).Msg("Failed to get executable path")
		return
	}

	if a.srv == nil {
		a.logger.Error().Msg("Graceful upgrade failed: server instance is nil")
		return
	}

	listeners := a.srv.GetListeners()
	if len(listeners) == 0 {
		a.logger.Error().Msg("Graceful upgrade failed: server has no saved listeners (graceful upgrade not fully implemented)")
		a.logger.Info().Msg("Hint: graceful upgrade requires the server to use manual listener management mode")
		return
	}

	a.upgradeMgr.SetListeners(listeners)

	if err := a.upgradeMgr.GracefulUpgrade(execPath); err != nil {
		a.logger.Error().Err(err).Msg("Graceful upgrade failed")
		return
	}

	a.logger.LogStartup("Graceful upgrade started, new process is taking over", nil)

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
