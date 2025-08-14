// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ThinkInAIXYZ/go-mcp/protocol"

	"github.com/jolks/mcp-cron/internal/config"
	"github.com/jolks/mcp-cron/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockScheduler is a mock implementation of the scheduler for testing purposes
type MockScheduler struct {
	mock.Mock
}

func (m *MockScheduler) AddTask(task *model.Task) error {
	args := m.Called(task)
	return args.Error(0)
}

func (m *MockScheduler) ListTasks() []*model.Task {
	args := m.Called()
	return args.Get(0).([]*model.Task)
}

func (m *MockScheduler) GetTask(id string) (*model.Task, error) {
	args := m.Called(id)
	return args.Get(0).(*model.Task), args.Error(1)
}

func (m *MockScheduler) RemoveTask(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockScheduler) EnableTask(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockScheduler) DisableTask(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockScheduler) Start(ctx context.Context) {
	m.Called(ctx)
}

func (m *MockScheduler) Stop() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockScheduler) SetTaskExecutor(executor model.Executor) {
	m.Called(executor)
}

// TestRegisterToolsDeclarative tests if tools are registered correctly
func TestRegisterToolsDeclarative(t *testing.T) {
	// Create a mock scheduler
	mockScheduler := new(MockScheduler)
	mockScheduler.On("SetTaskExecutor", mock.Anything).Return()

	// Create a new MCPServer with the mock scheduler
	cfg := config.DefaultConfig()
	mcpServer, err := NewMCPServer(cfg, mockScheduler)
	assert.NoError(t, err)

	// Register tools
	mcpServer.registerToolsDeclarative()

	// Check if the create_task tool is registered

}

// TestHandleCreateTask tests the handler for creating a task
func TestHandleCreateTask(t *testing.T) {
	// Create a mock scheduler
	mockScheduler := new(MockScheduler)
	mockScheduler.On("SetTaskExecutor", mock.Anything).Return()

	// Create a new MCPServer with the mock scheduler
	cfg := config.DefaultConfig()
	mcpServer, err := NewMCPServer(cfg, mockScheduler)
	assert.NoError(t, err)

	// Define the parameters for the create_task tool
	params := CreateTaskParams{
		Cron:        "* * * * *",
		TaskName:    "test_task",
		Instruction: "do something",
	}
	rawParams, err := json.Marshal(params)
	assert.NoError(t, err)

	request := &protocol.CallToolRequest{
		RawArguments: json.RawMessage(rawParams),
	}

	// Mock the AddTask function of the scheduler
	mockScheduler.On("AddTask", mock.AnythingOfType("*model.Task")).Return(nil)

	// Call the handler
	result, err := mcpServer.handleCreateTask(context.Background(), request)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Check if the AddTask function was called
	mockScheduler.AssertCalled(t, "AddTask", mock.AnythingOfType("*model.Task"))

	// Check the response
	var responseTask model.Task
	textContent, ok := result.Content[0].(*protocol.TextContent)
	assert.True(t, ok)
	err = json.Unmarshal([]byte(textContent.Text), &responseTask)
	assert.NoError(t, err)
	assert.Equal(t, params.TaskName, responseTask.Name)
	assert.Equal(t, params.Cron, responseTask.Schedule)
	assert.Equal(t, params.Instruction, responseTask.Instruction)
}
