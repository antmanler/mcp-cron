// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jolks/mcp-cron/internal/config"
	"github.com/jolks/mcp-cron/internal/logging"
	"github.com/jolks/mcp-cron/internal/scheduler"
	"github.com/jolks/mcp-cron/internal/server"
	"github.com/jolks/mcp-cron/internal/storage"
)

var (
	// buildVersion is set at build time via -ldflags "-X main.buildVersion=<version>"
	buildVersion    = "dev"
	workDir         = flag.String("work-dir", "", "Working directory (default: ~/.mcp-cron)")
	address         = flag.String("address", "", "The address to bind the server to")
	port            = flag.Int("port", 0, "The port to bind the server to")
	transport       = flag.String("transport", "", "Transport mode: sse or stdio")
	logLevel        = flag.String("log-level", "", "Logging level: debug, info, warn, error, fatal")
	showVersion     = flag.Bool("version", false, "Show version information and exit")
	aiModel         = flag.String("ai-model", "", "AI model to use for AI tasks (default: gpt-4o)")
	aiMaxIterations = flag.Int("ai-max-iterations", 0, "Maximum iterations for tool-enabled AI tasks (default: 20)")
	mcpConfigPath   = flag.String("mcp-config-path", "", "Path to MCP configuration file (default: ~/.cursor/mcp.json)")
	storageBackend  = flag.String("storage-backend", "", "Storage backend to use (default: json)")
	// Deprecated flags removed: log-file, storage-json-path
	storageWatch = flag.Bool("storage-watch", false, "Watch storage for changes and hot-reload (default: true)")
)

func main() {
	flag.Parse()

	// Load configuration
	cfg := loadConfig()

	// Fill in build version from ldflags if available
	if buildVersion != "" {
		cfg.Server.Version = buildVersion
	}

	// Show version and exit if requested
	if *showVersion {
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
	applyCommandLineFlagsToConfig(cfg)

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	return cfg
}

// applyCommandLineFlagsToConfig applies command line flags to the configuration
func applyCommandLineFlagsToConfig(cfg *config.Config) {
	// Determine work directory (default to ~/.mcp-cron)
	wd := *workDir
	if wd == "" {
		home := os.Getenv("HOME")
		if home == "" {
			// Fallback to current directory if HOME is unset
			home, _ = os.Getwd()
		}
		wd = filepath.Join(home, ".mcp-cron")
	}
	// Ensure work dir exists
	_ = os.MkdirAll(wd, 0o755)

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
	// Always place logs in work-dir
	cfg.Logging.FilePath = filepath.Join(wd, "mcp-cron.log")
	if *aiModel != "" {
		cfg.AI.Model = *aiModel
	}
	if *aiMaxIterations > 0 {
		cfg.AI.MaxToolIterations = *aiMaxIterations
	}
	if *mcpConfigPath != "" {
		cfg.AI.MCPConfigFilePath = *mcpConfigPath
	}
	if *storageBackend != "" {
		cfg.Storage.Backend = *storageBackend
	}
	// Always place storage in work-dir
	cfg.Storage.JSONPath = filepath.Join(wd, "tasks.json")
	// only set if user passed the flag explicitly
	if flag.Lookup("storage-watch").Value.String() != "false" || *storageWatch {
		cfg.Storage.Watch = *storageWatch
	}

	// If transport is stdio, ensure file logging is enabled (already set above)
}

// Application represents the running application
type Application struct {
	scheduler scheduler.Scheduler
	server    *server.MCPServer
	logger    *logging.Logger
}

// createApp creates a new application instance
func createApp(cfg *config.Config) (*Application, error) {
	sched := scheduler.NewScheduler(&cfg.Scheduler)

	// Initialize storage backend
	switch cfg.Storage.Backend {
	case "json", "":
		// Create JSON storage
		store, err := storage.NewJSONStorage(cfg.Storage.JSONPath)
		if err != nil {
			return nil, err
		}
		// If watch is disabled, we still pass the storage; the scheduler starts watching regardless and storage may no-op
		_ = cfg.Storage.Watch
		sched.SetStorage(store)
	default:
		// Fallback: no storage
	}

	mcpServer, err := server.NewMCPServer(cfg, sched)
	if err != nil {
		return nil, err
	}

	logger := logging.GetDefaultLogger()

	app := &Application{
		scheduler: sched,
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
