// SPDX-License-Identifier: AGPL-3.0-only
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Test Server defaults
	if cfg.Server.Address != "localhost" {
		t.Errorf("Expected default server address to be 'localhost', got '%s'", cfg.Server.Address)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected default server port to be 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.TransportMode != "sse" {
		t.Errorf("Expected default transport mode to be 'sse', got '%s'", cfg.Server.TransportMode)
	}
	if cfg.Server.Name != "mcp-cron" {
		t.Errorf("Expected default server name to be 'mcp-cron', got '%s'", cfg.Server.Name)
	}
	if cfg.Server.Version != "0.1.0" {
		t.Errorf("Expected default server version to be '0.1.0', got '%s'", cfg.Server.Version)
	}

	// Test Scheduler defaults
	if cfg.Scheduler.DefaultTimeout != 10*time.Minute {
		t.Errorf("Expected default timeout to be 10 minutes, got %s", cfg.Scheduler.DefaultTimeout)
	}

	// Test Logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected default logging level to be 'info', got '%s'", cfg.Logging.Level)
	}
	if cfg.Logging.FilePath != "" {
		t.Errorf("Expected default log file path to be empty, got '%s'", cfg.Logging.FilePath)
	}

	// Test AI defaults
	if cfg.AI.OpenAIAPIKey != "" {
		t.Errorf("Expected default OpenAI API key to be empty, got '%s'", cfg.AI.OpenAIAPIKey)
	}
	if cfg.AI.EnableOpenAITests != false {
		t.Errorf("Expected default EnableOpenAITests to be false, got %v", cfg.AI.EnableOpenAITests)
	}
	if cfg.AI.Model != "gpt-4o" {
		t.Errorf("Expected default AI model to be 'gpt-4o', got '%s'", cfg.AI.Model)
	}
	if cfg.AI.MaxToolIterations != 20 {
		t.Errorf("Expected default max tool iterations to be 20, got %d", cfg.AI.MaxToolIterations)
	}

	expectedPath := filepath.Join(os.Getenv("HOME"), ".cursor", "mcp.json")
	if cfg.AI.MCPConfigFilePath != expectedPath {
		t.Errorf("Expected default MCP config file path to be '%s', got '%s'", expectedPath, cfg.AI.MCPConfigFilePath)
	}
}

func TestValidate(t *testing.T) {
	// Test valid config
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Default config should be valid, got error: %v", err)
	}

	// Test invalid port (negative)
	invalidPort := DefaultConfig()
	invalidPort.Server.Port = -1
	if err := invalidPort.Validate(); err == nil {
		t.Error("Expected error for negative port, got nil")
	}

	// Test invalid port (too large)
	invalidLargePort := DefaultConfig()
	invalidLargePort.Server.Port = 70000
	if err := invalidLargePort.Validate(); err == nil {
		t.Error("Expected error for port > 65535, got nil")
	}

	// Test invalid transport mode
	invalidTransport := DefaultConfig()
	invalidTransport.Server.TransportMode = "invalid"
	if err := invalidTransport.Validate(); err == nil {
		t.Error("Expected error for invalid transport mode, got nil")
	}

	// Test invalid default timeout (too short)
	invalidTimeout := DefaultConfig()
	invalidTimeout.Scheduler.DefaultTimeout = time.Millisecond * 500
	if err := invalidTimeout.Validate(); err == nil {
		t.Error("Expected error for timeout < 1 second, got nil")
	}

	// Test invalid log level
	invalidLogLevel := DefaultConfig()
	invalidLogLevel.Logging.Level = "invalid"
	if err := invalidLogLevel.Validate(); err == nil {
		t.Error("Expected error for invalid log level, got nil")
	}

	// Test invalid max tool iterations (zero)
	invalidMaxIterations := DefaultConfig()
	invalidMaxIterations.AI.MaxToolIterations = 0
	if err := invalidMaxIterations.Validate(); err == nil {
		t.Error("Expected error for zero max tool iterations, got nil")
	}
}

func TestFromEnv(t *testing.T) {
	// Save current environment variables
	originalVars := map[string]string{
		"MCP_CRON_SERVER_ADDRESS":            os.Getenv("MCP_CRON_SERVER_ADDRESS"),
		"MCP_CRON_SERVER_PORT":               os.Getenv("MCP_CRON_SERVER_PORT"),
		"MCP_CRON_SERVER_TRANSPORT":          os.Getenv("MCP_CRON_SERVER_TRANSPORT"),
		"MCP_CRON_SERVER_NAME":               os.Getenv("MCP_CRON_SERVER_NAME"),
		"MCP_CRON_SERVER_VERSION":            os.Getenv("MCP_CRON_SERVER_VERSION"),
		"MCP_CRON_SCHEDULER_DEFAULT_TIMEOUT": os.Getenv("MCP_CRON_SCHEDULER_DEFAULT_TIMEOUT"),
		"MCP_CRON_LOGGING_LEVEL":             os.Getenv("MCP_CRON_LOGGING_LEVEL"),
		"MCP_CRON_LOGGING_FILE":              os.Getenv("MCP_CRON_LOGGING_FILE"),
		"OPENAI_API_KEY":                     os.Getenv("OPENAI_API_KEY"),
		"MCP_CRON_ENABLE_OPENAI_TESTS":       os.Getenv("MCP_CRON_ENABLE_OPENAI_TESTS"),
		"MCP_CRON_AI_MODEL":                  os.Getenv("MCP_CRON_AI_MODEL"),
		"MCP_CRON_AI_MAX_TOOL_ITERATIONS":    os.Getenv("MCP_CRON_AI_MAX_TOOL_ITERATIONS"),
		"MCP_CRON_MCP_CONFIG_FILE_PATH":      os.Getenv("MCP_CRON_MCP_CONFIG_FILE_PATH"),
	}

	// Restore environment variables after test
	defer func() {
		for key, value := range originalVars {
			if value != "" {
				os.Setenv(key, value)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	// Clear all relevant environment variables
	for key := range originalVars {
		os.Unsetenv(key)
	}

	// Set test values
	os.Setenv("MCP_CRON_SERVER_ADDRESS", "127.0.0.1")
	os.Setenv("MCP_CRON_SERVER_PORT", "9090")
	os.Setenv("MCP_CRON_SERVER_TRANSPORT", "stdio")
	os.Setenv("MCP_CRON_SERVER_NAME", "test-server")
	os.Setenv("MCP_CRON_SERVER_VERSION", "1.0.0")
	os.Setenv("MCP_CRON_SCHEDULER_DEFAULT_TIMEOUT", "5m")
	os.Setenv("MCP_CRON_LOGGING_LEVEL", "debug")
	os.Setenv("MCP_CRON_LOGGING_FILE", "/tmp/test.log")
	os.Setenv("OPENAI_API_KEY", "test-key")
	os.Setenv("MCP_CRON_ENABLE_OPENAI_TESTS", "true")
	os.Setenv("MCP_CRON_AI_MODEL", "gpt-4-turbo")
	os.Setenv("MCP_CRON_AI_MAX_TOOL_ITERATIONS", "30")
	os.Setenv("MCP_CRON_MCP_CONFIG_FILE_PATH", "/tmp/mcp.json")

	// Create a new config and apply environment variables
	cfg := DefaultConfig()
	FromEnv(cfg)

	// Verify values were loaded from environment
	if cfg.Server.Address != "127.0.0.1" {
		t.Errorf("Expected server address '127.0.0.1', got '%s'", cfg.Server.Address)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Expected server port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.TransportMode != "stdio" {
		t.Errorf("Expected transport mode 'stdio', got '%s'", cfg.Server.TransportMode)
	}
	if cfg.Server.Name != "test-server" {
		t.Errorf("Expected server name 'test-server', got '%s'", cfg.Server.Name)
	}
	if cfg.Server.Version != "1.0.0" {
		t.Errorf("Expected server version '1.0.0', got '%s'", cfg.Server.Version)
	}
	if cfg.Scheduler.DefaultTimeout != 5*time.Minute {
		t.Errorf("Expected default timeout 5m, got %s", cfg.Scheduler.DefaultTimeout)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected logging level 'debug', got '%s'", cfg.Logging.Level)
	}
	if cfg.Logging.FilePath != "/tmp/test.log" {
		t.Errorf("Expected log file path '/tmp/test.log', got '%s'", cfg.Logging.FilePath)
	}
	if cfg.AI.OpenAIAPIKey != "test-key" {
		t.Errorf("Expected OpenAI API key 'test-key', got '%s'", cfg.AI.OpenAIAPIKey)
	}
	if !cfg.AI.EnableOpenAITests {
		t.Errorf("Expected EnableOpenAITests true, got false")
	}
	if cfg.AI.Model != "gpt-4-turbo" {
		t.Errorf("Expected AI model 'gpt-4-turbo', got '%s'", cfg.AI.Model)
	}
	if cfg.AI.MaxToolIterations != 30 {
		t.Errorf("Expected max tool iterations 30, got %d", cfg.AI.MaxToolIterations)
	}
	if cfg.AI.MCPConfigFilePath != "/tmp/mcp.json" {
		t.Errorf("Expected MCP config file path '/tmp/mcp.json', got '%s'", cfg.AI.MCPConfigFilePath)
	}

	// Test invalid port format
	os.Setenv("MCP_CRON_SERVER_PORT", "invalid")
	cfg = DefaultConfig()
	FromEnv(cfg)
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected server port to remain 8080 for invalid input, got %d", cfg.Server.Port)
	}

	// Test invalid timeout format
	os.Setenv("MCP_CRON_SCHEDULER_DEFAULT_TIMEOUT", "invalid")
	cfg = DefaultConfig()
	FromEnv(cfg)
	if cfg.Scheduler.DefaultTimeout != 10*time.Minute {
		t.Errorf("Expected default timeout to remain 10m for invalid input, got %s", cfg.Scheduler.DefaultTimeout)
	}

	// Test invalid max tool iterations format
	os.Setenv("MCP_CRON_AI_MAX_TOOL_ITERATIONS", "invalid")
	cfg = DefaultConfig()
	FromEnv(cfg)
	if cfg.AI.MaxToolIterations != 20 {
		t.Errorf("Expected max tool iterations to remain 20 for invalid input, got %d", cfg.AI.MaxToolIterations)
	}
}
