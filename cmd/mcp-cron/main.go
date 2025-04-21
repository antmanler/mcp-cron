// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jolks/mcp-cron/internal/config"
	"github.com/jolks/mcp-cron/internal/executor"
	"github.com/jolks/mcp-cron/internal/logging"
	"github.com/jolks/mcp-cron/internal/scheduler"
	"github.com/jolks/mcp-cron/internal/server"
)

var (
	address   = flag.String("address", "", "The address to bind the server to")
	port      = flag.Int("port", 0, "The port to bind the server to")
	transport = flag.String("transport", "", "Transport mode: sse or stdio")
	logLevel  = flag.String("log-level", "", "Logging level: debug, info, warn, error, fatal")
	logFile   = flag.String("log-file", "", "Log file path (default: stdout)")
	version   = flag.Bool("version", false, "Show version information and exit")
)

func main() {
	flag.Parse()

	// Load configuration
	cfg := loadConfig()

	// Show version and exit if requested
	if *version {
		log.Printf("%s version %s", cfg.Server.Name, cfg.Server.Version)
		os.Exit(0)
	}

	// Create a context that will be cancelled on interrupt signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize the application
	app, err := createApp(cfg)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// Start the application
	if err := app.Start(ctx); err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}

	// Wait for termination signal
	waitForSignal(cancel, app)
}

// loadConfig loads configuration from environment and command line flags
func loadConfig() *config.Config {
	// Start with defaults
	cfg := config.DefaultConfig()

	// Override with environment variables
	config.FromEnv(cfg)

	// Override with command-line flags
	if *address != "" {
		cfg.Server.Address = *address
	}
	if *port != 0 {
		cfg.Server.Port = *port
	}
	if *transport != "" {
		cfg.Server.TransportMode = *transport
	}
	if *logLevel != "" {
		cfg.Logging.Level = *logLevel
	}
	if *logFile != "" {
		cfg.Logging.FilePath = *logFile
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	return cfg
}

// Application represents the running application
type Application struct {
	scheduler *scheduler.Scheduler
	executor  *executor.CommandExecutor
	server    *server.MCPServer
	logger    *logging.Logger
}

// createApp creates a new application instance
func createApp(cfg *config.Config) (*Application, error) {
	// Create components
	exec := executor.NewCommandExecutor()
	sched := scheduler.NewScheduler()

	// Create the MCP server
	mcpServer, err := server.NewMCPServer(cfg, sched, exec)
	if err != nil {
		return nil, err
	}

	// Get the default logger that was configured by the server
	logger := logging.GetDefaultLogger()

	// Create the application
	app := &Application{
		scheduler: sched,
		executor:  exec,
		server:    mcpServer,
		logger:    logger,
	}

	return app, nil
}

// Start starts the application
func (a *Application) Start(ctx context.Context) error {
	// Start the scheduler
	a.scheduler.Start(ctx)
	a.logger.Infof("Task scheduler started")

	// Start the MCP server
	if err := a.server.Start(ctx); err != nil {
		return err
	}
	a.logger.Infof("MCP server started")

	return nil
}

// Stop stops the application
func (a *Application) Stop() error {
	// Stop the scheduler
	err := a.scheduler.Stop()
	if err != nil {
		return err
	}
	a.logger.Infof("Task scheduler stopped")

	// Stop the server
	if err := a.server.Stop(); err != nil {
		a.logger.Errorf("Error stopping MCP server: %v", err)
		return err
	}
	a.logger.Infof("MCP server stopped")

	return nil
}

// waitForSignal waits for termination signals and performs cleanup
func waitForSignal(cancel context.CancelFunc, app *Application) {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	<-signalCh
	app.logger.Infof("Received termination signal, shutting down...")

	// Cancel the context to initiate shutdown
	cancel()

	// Stop the application with a timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	shutdownDone := make(chan struct{})
	go func() {
		if err := app.Stop(); err != nil {
			app.logger.Errorf("Error during shutdown: %v", err)
		}
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
		app.logger.Infof("Graceful shutdown completed")
	case <-shutdownCtx.Done():
		app.logger.Warnf("Shutdown timed out")
	}
}
