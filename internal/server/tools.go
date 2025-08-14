// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"context"

	"github.com/ThinkInAIXYZ/go-mcp/protocol"
	"github.com/ThinkInAIXYZ/go-mcp/server"
)

// ToolDefinition represents a tool that can be registered with the MCP server
type ToolDefinition struct {
	// Name is the name of the tool
	Name string

	// Description is a brief description of what the tool does
	Description string

	// Handler is the function that will be called when the tool is invoked
	Handler func(context.Context, *protocol.CallToolRequest) (*protocol.CallToolResult, error)

	// Parameters is the parameter schema for the tool (can be a struct)
	Parameters interface{}
}

// registerToolsDeclarative sets up all the MCP tools using a more declarative approach
func (s *MCPServer) registerToolsDeclarative() {
	tools := []ToolDefinition{
		{
			Name:        "create_task",
			Description: "Creates a scheduled task",
			Handler:     s.handleCreateTask,
			Parameters:  CreateTaskParams{},
		},
		{
			Name:        "list_tasks",
			Description: "Lists all scheduled tasks",
			Handler:     s.handleListTasks,
			Parameters:  struct{}{}, // No parameters for listing tasks
		},
		{
			Name:        "get_task",
			Description: "Gets a specific task by ID",
			Handler:     s.handleGetTask,
			Parameters:  TaskIDParams{},
		},
		{
			Name:        "remove_task",
			Description: "Removes a scheduled task",
			Handler:     s.handleRemoveTask,
			Parameters:  TaskIDParams{},
		},
	}

	for _, tool := range tools {
		registerToolWithError(s.server, tool)
	}
}

// registerToolWithError registers a tool with error handling
func registerToolWithError(srv *server.Server, def ToolDefinition) {
	tool, err := protocol.NewTool(def.Name, def.Description, def.Parameters)
	if err != nil {
		// In a real scenario, we might want to handle this differently,
		// but for now we'll panic since this is a critical error
		// that should never happen
		panic(err)
	}

	srv.RegisterTool(tool, def.Handler)
}
