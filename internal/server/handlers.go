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

// Standard response structures
type successResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// extractParams extracts and validates parameters from a request
// This function handles the basic JSON unmarshaling of request parameters
// but does not perform domain-specific validation (e.g., required fields)
// which should be done by the individual handlers
func extractParams(request *protocol.CallToolRequest, params interface{}) error {
	if err := utils.JsonUnmarshal(request.RawArguments, params); err != nil {
		return errors.InvalidInput(fmt.Sprintf("invalid parameters: %v", err))
	}
	return nil
}

// extractTaskIDParam is a helper to extract and validate a task ID parameter
func extractTaskIDParam(request *protocol.CallToolRequest) (string, error) {
	var params struct {
		ID string `json:"id"`
	}
	if err := extractParams(request, &params); err != nil {
		return "", err
	}
	if params.ID == "" {
		return "", errors.InvalidInput("missing required parameter: id")
	}
	return params.ID, nil
}

// createSuccessResponse creates a standardized success response
func createSuccessResponse(message string) (*protocol.CallToolResult, error) {
	response := successResponse{
		Success: true,
		Message: message,
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return nil, errors.Internal(fmt.Errorf("failed to marshal response: %w", err))
	}

	return &protocol.CallToolResult{
		Content: []protocol.Content{
			protocol.TextContent{
				Type: "text",
				Text: string(responseJSON),
			},
		},
	}, nil
}

// createErrorResponse creates a standardized error response
func createErrorResponse(err error) (*protocol.CallToolResult, error) {
	// Always return the original error as the second return value
	// This ensures MCP protocol error handling works correctly
	return nil, err
}

// createTaskResponse creates a response containing a task
func createTaskResponse(task *model.Task) (*protocol.CallToolResult, error) {
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return nil, errors.Internal(fmt.Errorf("failed to marshal task: %w", err))
	}

	return &protocol.CallToolResult{
		Content: []protocol.Content{
			protocol.TextContent{
				Type: "text",
				Text: string(taskJSON),
			},
		},
	}, nil
}

// createTasksResponse creates a response containing multiple tasks
func createTasksResponse(tasks []*model.Task) (*protocol.CallToolResult, error) {
	tasksJSON, err := json.Marshal(tasks)
	if err != nil {
		return nil, errors.Internal(fmt.Errorf("failed to marshal tasks: %w", err))
	}

	return &protocol.CallToolResult{
		Content: []protocol.Content{
			protocol.TextContent{
				Type: "text",
				Text: string(tasksJSON),
			},
		},
	}, nil
}
