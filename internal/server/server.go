// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ThinkInAIXYZ/go-mcp/protocol"
	"github.com/ThinkInAIXYZ/go-mcp/server"
	"github.com/ThinkInAIXYZ/go-mcp/transport"
	"github.com/jolks/mcp-cron/internal/config"
	"github.com/jolks/mcp-cron/internal/errors"
	"github.com/jolks/mcp-cron/internal/logging"
	"github.com/jolks/mcp-cron/internal/model"
	"github.com/jolks/mcp-cron/internal/scheduler"
)

// Make os.OpenFile mockable for testing
var osOpenFile = os.OpenFile

// TaskIDParams holds the ID parameter used by multiple handlers
type TaskIDParams struct {
	ID string `json:"id" description:"the ID of the task to get/remove/enable/disable"`
}

// FileContextParam represents file context input
type FileContextParam struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

// ListTasksParams defines parameters for listing tasks
type ListTasksParams struct {
	SessionID string `json:"session_id" description:"session id (chat id)"`
}

// CreateTaskParams defines parameters for creating a task
type CreateTaskParams struct {
	Cron                 string             `json:"cron" description:"cron expression"`
	TaskType             string             `json:"task_type,omitempty" description:"ONCE or PERIODICALLY, default is ONCE, for periodic tasks task_type must be set to PERIODICALLY"`
	TaskName             string             `json:"task_name" description:"task name"`
	SessionID            string             `json:"session_id" description:"session id (chat id)"`
	Instruction          string             `json:"instruction" description:"task instruction"`
	RelevantChatSnippets []string           `json:"relevant_chat_snippets,omitempty"`
	FileContext          []FileContextParam `json:"file_context,omitempty"`
}

// MCPServer represents the MCP scheduler server
type MCPServer struct {
	scheduler      scheduler.Scheduler
	server         *server.Server
	address        string
	port           int
	stopCh         chan struct{}
	wg             sync.WaitGroup
	config         *config.Config
	logger         *logging.Logger
	shutdownMutex  sync.Mutex
	isShuttingDown bool
}

// NewMCPServer creates a new MCP scheduler server
func NewMCPServer(cfg *config.Config, scheduler scheduler.Scheduler) (*MCPServer, error) {
	// Create default config if not provided
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	// Initialize logger
	var logger *logging.Logger

	if cfg.Logging.FilePath != "" {
		var err error
		logger, err = logging.FileLogger(cfg.Logging.FilePath, parseLogLevel(cfg.Logging.Level))
		if err != nil {
			return nil, fmt.Errorf("failed to create file logger: %w", err)
		}
	} else {
		logger = logging.New(logging.Options{
			Level: parseLogLevel(cfg.Logging.Level),
		})
	}

	// Set as the default logger
	logging.SetDefaultLogger(logger)

	// Configure logger based on transport mode
	if cfg.Server.TransportMode == "stdio" {
		// For stdio transport, we need to be careful with logging
		// as it could interfere with JSON-RPC messages
		// Redirect logs to a file instead of stdout

		// Prefer configured log file path (e.g., work-dir). Fallback to executable directory.
		logPath := cfg.Logging.FilePath
		if strings.TrimSpace(logPath) == "" {
			// Get the executable path
			execPath, err := os.Executable()
			if err != nil {
				logger.Errorf("Failed to get executable path: %v", err)
				execPath = cfg.Server.Name
			}
			// Get the directory containing the executable
			execDir := filepath.Dir(execPath)

			// Set log path in the same directory as the executable
			logFilename := fmt.Sprintf("%s.log", cfg.Server.Name)
			logPath = filepath.Join(execDir, logFilename)
		}

		logFile, err := osOpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			log.SetOutput(logFile)
			logger.Infof("Logging to %s", logPath)
		} else {
			logger.Errorf("Failed to open log file at %s: %v", logPath, err)
		}
	}

	// Create MCP Server
	mcpServer := &MCPServer{
		scheduler: scheduler,
		address:   cfg.Server.Address,
		port:      cfg.Server.Port,
		stopCh:    make(chan struct{}),
		config:    cfg,
		logger:    logger,
	}

	// Set up task routing
	scheduler.SetTaskExecutor(mcpServer)

	// Create transport based on mode
	var svrTransport transport.ServerTransport
	var err error

	switch cfg.Server.TransportMode {
	case "stdio":
		// Create stdio transport
		logger.Infof("Using stdio transport")
		svrTransport = transport.NewStdioServerTransport()
	case "sse":
		// Create HTTP SSE transport
		addr := fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port)
		logger.Infof("Using SSE transport on %s", addr)

		// Create SSE transport with the address
		svrTransport, err = transport.NewSSEServerTransport(addr)
		if err != nil {
			return nil, errors.Internal(fmt.Errorf("failed to create SSE transport: %w", err))
		}
	default:
		return nil, errors.InvalidInput(fmt.Sprintf("unsupported transport mode: %s", cfg.Server.TransportMode))
	}

	// Create MCP server with the transport
	mcpServer.server, err = server.NewServer(
		svrTransport,
		server.WithServerInfo(protocol.Implementation{
			Name:    cfg.Server.Name,
			Version: cfg.Server.Version,
		}),
	)
	if err != nil {
		return nil, errors.Internal(fmt.Errorf("failed to create MCP server: %w", err))
	}

	return mcpServer, nil
}

// Start starts the MCP server
func (s *MCPServer) Start(ctx context.Context) error {
	// Register all tools
	s.registerToolsDeclarative()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		// Start the server
		if err := s.server.Run(); err != nil {
			s.logger.Errorf("Error running MCP server: %v", err)
			return
		}
	}()

	// Listen for context cancellation
	go func() {
		<-ctx.Done()
		if err := s.Stop(); err != nil {
			s.logger.Errorf("Error stopping MCP server: %v", err)
		}
	}()

	return nil
}

// Stop stops the MCP server
func (s *MCPServer) Stop() error {
	s.shutdownMutex.Lock()
	defer s.shutdownMutex.Unlock()

	// Return early if server is already being shut down
	if s.isShuttingDown {
		s.logger.Debugf("Stop called but server is already shutting down, ignoring")
		return nil
	}

	s.isShuttingDown = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return errors.Internal(fmt.Errorf("error shutting down MCP server: %w", err))
	}

	// Only close stopCh if it hasn't been closed yet
	select {
	case <-s.stopCh:
		// Channel is already closed, do nothing
	default:
		close(s.stopCh)
	}

	s.wg.Wait()
	return nil
}

// handleListTasks lists all tasks
func (s *MCPServer) handleListTasks(ctx context.Context, request *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
	// Extract parameters
	var params ListTasksParams
	if err := extractParams(request, &params); err != nil {
		return createErrorResponse(err)
	}
	if strings.TrimSpace(params.SessionID) == "" {
		return createErrorResponse(errors.InvalidInput("session_id is required"))
	}

	s.logger.Debugf("Handling list_tasks request for session %s", params.SessionID)

	// Get all tasks then filter by session
	all := s.scheduler.ListTasks()
	filtered := make([]*model.Task, 0, len(all))
	for _, t := range all {
		if t != nil && t.SessionID == params.SessionID {
			filtered = append(filtered, t)
		}
	}

	return createTasksResponse(filtered)
}

// handleGetTask gets a specific task by ID
func (s *MCPServer) handleGetTask(ctx context.Context, request *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
	// Extract task ID
	taskID, err := extractTaskIDParam(request)
	if err != nil {
		return createErrorResponse(err)
	}

	s.logger.Debugf("Handling get_task request for task %s", taskID)

	// Get the task
	task, err := s.scheduler.GetTask(taskID)
	if err != nil {
		return createErrorResponse(err)
	}

	return createTaskResponse(task)
}

// handleCreateTask adds a new scheduled task
func (s *MCPServer) handleCreateTask(ctx context.Context, request *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
	// Extract parameters
	var params CreateTaskParams
	if err := extractParams(request, &params); err != nil {
		return createErrorResponse(err)
	}

	// Validate parameters
	if err := validateCreateTaskParams(params.Cron, params.TaskName, params.Instruction, params.SessionID); err != nil {
		return createErrorResponse(err)
	}

	s.logger.Debugf("Handling create_task request for task %s", params.TaskName)

	// Create task
	task := createBaseTask(params.TaskName, params.Cron, "", true)
	task.Instruction = params.Instruction
	task.TaskType = params.TaskType
	task.SessionID = params.SessionID
	task.RelevantChatSnippets = params.RelevantChatSnippets
	for _, fc := range params.FileContext {
		task.FileContext = append(task.FileContext, model.FileRef{Path: fc.Path, Description: fc.Description})
	}
	task.RawInput = string(request.RawArguments)

	// Add task to scheduler
	if err := s.scheduler.AddTask(task); err != nil {
		return createErrorResponse(err)
	}

	return createTaskResponse(task)
}

// createBaseTask creates a base task with common fields initialized
func createBaseTask(name, schedule, description string, enabled bool) *model.Task {
	now := time.Now()
	taskID := fmt.Sprintf("task_%d", now.UnixNano())

	return &model.Task{
		ID:          taskID,
		Name:        name,
		Schedule:    schedule,
		Description: description,
		Enabled:     enabled,
		Status:      model.StatusPending,
		LastRun:     now,
		NextRun:     now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// handleRemoveTask removes a task
func (s *MCPServer) handleRemoveTask(ctx context.Context, request *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
	// Extract task ID
	taskID, err := extractTaskIDParam(request)
	if err != nil {
		return createErrorResponse(err)
	}

	s.logger.Debugf("Handling remove_task request for task %s", taskID)

	// Remove task
	if err := s.scheduler.RemoveTask(taskID); err != nil {
		return createErrorResponse(err)
	}

	return createSuccessResponse(fmt.Sprintf("Task %s removed successfully", taskID))
}

// handleEnableTask enables a task
func (s *MCPServer) handleEnableTask(ctx context.Context, request *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
	// Extract task ID
	taskID, err := extractTaskIDParam(request)
	if err != nil {
		return createErrorResponse(err)
	}

	s.logger.Debugf("Handling enable_task request for task %s", taskID)

	// Enable task
	if err := s.scheduler.EnableTask(taskID); err != nil {
		return createErrorResponse(err)
	}

	// Get updated task
	task, err := s.scheduler.GetTask(taskID)
	if err != nil {
		return createErrorResponse(err)
	}

	return createTaskResponse(task)
}

// handleDisableTask disables a task
func (s *MCPServer) handleDisableTask(ctx context.Context, request *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
	// Extract task ID
	taskID, err := extractTaskIDParam(request)
	if err != nil {
		return createErrorResponse(err)
	}

	s.logger.Debugf("Handling disable_task request for task %s", taskID)

	// Disable task
	if err := s.scheduler.DisableTask(taskID); err != nil {
		return createErrorResponse(err)
	}

	// Get updated task
	task, err := s.scheduler.GetTask(taskID)
	if err != nil {
		return createErrorResponse(err)
	}

	return createTaskResponse(task)
}

// Execute implements the model.Executor interface by sending a message to the local server
func (s *MCPServer) Execute(ctx context.Context, task *model.Task, timeout time.Duration) error {
	content := fmt.Sprintf("@Duoduo 系统触发定时任务，按照下面指令执行:\n\n%s", task.RawInput)
	// Prefer SessionID if available; fall back to Name for backward compatibility
	chatID := task.SessionID
	if chatID == "" {
		chatID = task.Name
	}
	if err := sendMessageToLocalServer(ctx, chatID, content); err != nil {
		return err
	}

	// If the task type is ONCE (default), remove it after execution
	if strings.EqualFold(task.TaskType, "ONCE") || task.TaskType == "" {
		_ = s.scheduler.RemoveTask(task.ID)
	}
	return nil
}

// Helper function to parse log level
func parseLogLevel(level string) logging.LogLevel {
	switch level {
	case "debug":
		return logging.Debug
	case "info":
		return logging.Info
	case "warn":
		return logging.Warn
	case "error":
		return logging.Error
	case "fatal":
		return logging.Fatal
	default:
		return logging.Info
	}
}

// sendMessageToLocalServer sends a message to the local server
func sendMessageToLocalServer(ctx context.Context, chatID, content string) error {
	host := os.Getenv("LOCAL_SERVER_PUBLIC_HOST")
	if host == "" {
		host = "localhost"
	}
	if host == "0.0.0.0" {
		host = "localhost"
	}
	portStr := os.Getenv("LOCAL_SERVER_PORT")
	if portStr == "" {
		portStr = "8787"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid LOCAL_SERVER_PORT: %w", err)
	}

	baseURL := fmt.Sprintf("http://%s:%d/room/%s/append", host, port, chatID)
	params := urlpkg.Values{}
	params.Set("sender", "runtime:cron_task")
	params.Set("content", content)
	params.Set("tag", "reply")

	reqURL := baseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("local server returned status %s", resp.Status)
	}

	return nil
}
