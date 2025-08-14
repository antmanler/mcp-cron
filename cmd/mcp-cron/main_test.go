// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"os"
	"testing"

	"github.com/jolks/mcp-cron/internal/config"
	"github.com/jolks/mcp-cron/internal/scheduler"
	"github.com/jolks/mcp-cron/internal/server"
)

// TestMCPServerCreation tests server creation with custom configs
func TestMCPServerCreation(t *testing.T) {
	// Test creating MCP server with custom config

	// Import the config package from the same repo
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address:       "127.0.0.1",
			Port:          9999,
			TransportMode: "stdio", // Use stdio to avoid network binding
		},
		Scheduler: config.SchedulerConfig{
			DefaultTimeout: config.DefaultConfig().Scheduler.DefaultTimeout,
		},
	}

	// Create a scheduler
	cronScheduler := scheduler.NewScheduler(&cfg.Scheduler)

	// Create the server with custom config
	mcpServer, err := server.NewMCPServer(cfg, cronScheduler)

	if err != nil {
		t.Fatalf("Failed to create MCP server: %v", err)
	}

	if mcpServer == nil {
		t.Fatal("NewMCPServer returned nil server")
	}
}

// TestApplyCommandLineFlagsToConfig tests the application of command line flags to the configuration
func TestApplyCommandLineFlagsToConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	// Simulate setting command line flags
	testAddress := "192.168.1.1"
	testPort := 9090
	testTransport := "stdio"
	testLogLevel := "debug"
	testLogFile := "/var/log/mcp-cron.log"
	testAiModel := "gpt-3.5-turbo"
	testAiMaxIterations := 10
	testMcpConfigPath := "/etc/mcp/config.json"

	address = &testAddress
	port = &testPort
	transport = &testTransport
	logLevel = &testLogLevel
	logFile = &testLogFile
	aiModel = &testAiModel
	aiMaxIterations = &testAiMaxIterations
	mcpConfigPath = &testMcpConfigPath

	applyCommandLineFlagsToConfig(cfg)

	if cfg.Server.Address != testAddress {
		t.Errorf("expected address %s, got %s", testAddress, cfg.Server.Address)
	}
	if cfg.Server.Port != testPort {
		t.Errorf("expected port %d, got %d", testPort, cfg.Server.Port)
	}
	if cfg.Server.TransportMode != testTransport {
		t.Errorf("expected transport mode %s, got %s", testTransport, cfg.Server.TransportMode)
	}
	if cfg.Logging.Level != testLogLevel {
		t.Errorf("expected log level %s, got %s", testLogLevel, cfg.Logging.Level)
	}
	if cfg.Logging.FilePath != testLogFile {
		t.Errorf("expected log file %s, got %s", testLogFile, cfg.Logging.FilePath)
	}
	if cfg.AI.Model != testAiModel {
		t.Errorf("expected AI model %s, got %s", testAiModel, cfg.AI.Model)
	}
	if cfg.AI.MaxToolIterations != testAiMaxIterations {
		t.Errorf("expected AI max iterations %d, got %d", testAiMaxIterations, cfg.AI.MaxToolIterations)
	}
	if cfg.AI.MCPConfigFilePath != testMcpConfigPath {
		t.Errorf("expected MCP config path %s, got %s", testMcpConfigPath, cfg.AI.MCPConfigFilePath)
	}
}

// TestLoadConfig tests the loading of configuration from defaults, environment, and flags
func TestLoadConfig(t *testing.T) {
	// Set environment variables
	os.Setenv("MCP_CRON_SERVER_ADDRESS", "10.0.0.1")
	os.Setenv("MCP_CRON_SERVER_PORT", "8888")
	os.Setenv("MCP_CRON_LOGGING_LEVEL", "warn")

	// Simulate setting command line flags (which should override env vars)
	testAddress := "192.168.1.1"
	testPort := 9090
	testLogLevel := "debug"

	address = &testAddress
	port = &testPort
	logLevel = &testLogLevel
	// These are not set via env vars, so they should be applied from flags
	testTransport := "stdio"
	testLogFile := "/var/log/mcp-cron.log"
	transport = &testTransport
	logFile = &testLogFile

	cfg := loadConfig()

	if cfg.Server.Address != testAddress {
		t.Errorf("expected address %s, got %s", testAddress, cfg.Server.Address)
	}
	if cfg.Server.Port != testPort {
		t.Errorf("expected port %d, got %d", testPort, cfg.Server.Port)
	}
	if cfg.Logging.Level != testLogLevel {
		t.Errorf("expected log level %s, got %s", testLogLevel, cfg.Logging.Level)
	}
	if cfg.Server.TransportMode != testTransport {
		t.Errorf("expected transport mode %s, got %s", testTransport, cfg.Server.TransportMode)
	}
	if cfg.Logging.FilePath != testLogFile {
		t.Errorf("expected log file %s, got %s", testLogFile, cfg.Logging.FilePath)
	}

	// Clean up environment variables
	os.Unsetenv("MCP_CRON_SERVER_ADDRESS")
	os.Unsetenv("MCP_CRON_SERVER_PORT")
	os.Unsetenv("MCP_CRON_LOGGING_LEVEL")
}
