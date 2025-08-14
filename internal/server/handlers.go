// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"encoding/json"
	"fmt"

	"github.com/ThinkInAIXYZ/go-mcp/protocol"
	"github.com/jolks/mcp-cron/internal/errors"
	"github.com/jolks/mcp-cron/internal/model"
	"github.com/jolks/mcp-cron/internal/utils"
)

// extractParams extracts parameters from a tool request
func extractParams(request *protocol.CallToolRequest, params interface{}) error {
	if err := utils.JsonUnmarshal(request.RawArguments, params); err != nil {
		return errors.InvalidInput(fmt.Sprintf("invalid parameters: %v", err))
	}
	return nil
}

// extractTaskIDParam extracts the task ID parameter from a request
func extractTaskIDParam(request *protocol.CallToolRequest) (string, error) {
	var params TaskIDParams
	if err := extractParams(request, &params); err != nil {
		return "", err
	}

	if params.ID == "" {
		return "", errors.InvalidInput("task ID is required")
	}

	return params.ID, nil
}

// createSuccessResponse creates a success response
func createSuccessResponse(message string) (*protocol.CallToolResult, error) {
	response := map[string]interface{}{
		"success": true,
		"message": message,
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return nil, errors.Internal(fmt.Errorf("failed to marshal response: %w", err))
	}

	return &protocol.CallToolResult{
		Content: []protocol.Content{
			&protocol.TextContent{
				Type: "text",
				Text: string(responseJSON),
			},
		},
	}, nil
}

// createErrorResponse creates an error response
func createErrorResponse(err error) (*protocol.CallToolResult, error) {
	// Always return the original error as the second return value
	// This ensures MCP protocol error handling works correctly
	return nil, err
}

// createTaskResponse creates a response with a single task
func createTaskResponse(task *model.Task) (*protocol.CallToolResult, error) {
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return nil, errors.Internal(fmt.Errorf("failed to marshal task: %w", err))
	}

	return &protocol.CallToolResult{
		Content: []protocol.Content{
			&protocol.TextContent{
				Type: "text",
				Text: string(taskJSON),
			},
		},
	}, nil
}

// createTasksResponse creates a response with multiple tasks
func createTasksResponse(tasks []*model.Task) (*protocol.CallToolResult, error) {
	tasksJSON, err := json.Marshal(tasks)
	if err != nil {
		return nil, errors.Internal(fmt.Errorf("failed to marshal tasks: %w", err))
	}

	return &protocol.CallToolResult{
		Content: []protocol.Content{
			&protocol.TextContent{
				Type: "text",
				Text: string(tasksJSON),
			},
		},
	}, nil
}

// validateCreateTaskParams validates parameters for creating a task
func validateCreateTaskParams(cron, name, instruction, sessionID string) error {
	if cron == "" || name == "" || instruction == "" || sessionID == "" {
		return errors.InvalidInput("missing required fields: cron, task_name, instruction and session_id are required")
	}
	return nil
}
