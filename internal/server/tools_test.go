// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ThinkInAIXYZ/go-mcp/protocol"

	"github.com/jolks/mcp-cron/internal/config"
	"github.com/jolks/mcp-cron/internal/model"
	"github.com/jolks/mcp-cron/internal/storage"

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

func (m *MockScheduler) SetStorage(store storage.Storage) {
	m.Called(store)
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
		SessionID:   "chat-test",
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

// TestHandleListTasksRequiresSessionID ensures session_id is required
func TestHandleListTasksRequiresSessionID(t *testing.T) {
	mockScheduler := new(MockScheduler)
	mockScheduler.On("SetTaskExecutor", mock.Anything).Return()

	cfg := config.DefaultConfig()
	mcpServer, err := NewMCPServer(cfg, mockScheduler)
	assert.NoError(t, err)

	// No session_id provided
	request := &protocol.CallToolRequest{RawArguments: json.RawMessage(`{}`)}
	res, err := mcpServer.handleListTasks(context.Background(), request)
	assert.Nil(t, res)
	assert.Error(t, err)
}

// TestHandleListTasksFiltersBySession ensures filtering works by session_id
func TestHandleListTasksFiltersBySession(t *testing.T) {
	mockScheduler := new(MockScheduler)
	mockScheduler.On("SetTaskExecutor", mock.Anything).Return()

	// Prepare tasks
	tasks := []*model.Task{
		{ID: "1", Name: "a", SessionID: "s1"},
		{ID: "2", Name: "b", SessionID: "s2"},
		{ID: "3", Name: "c", SessionID: "s1"},
	}
	mockScheduler.On("ListTasks").Return(tasks)

	cfg := config.DefaultConfig()
	mcpServer, err := NewMCPServer(cfg, mockScheduler)
	assert.NoError(t, err)

	// Provide session_id s1
	raw, _ := json.Marshal(ListTasksParams{SessionID: "s1"})
	request := &protocol.CallToolRequest{RawArguments: json.RawMessage(raw)}

	res, err := mcpServer.handleListTasks(context.Background(), request)
	assert.NoError(t, err)
	assert.NotNil(t, res)

	// Parse response
	textContent, ok := res.Content[0].(*protocol.TextContent)
	if !ok {
		t.Fatalf("expected TextContent")
	}
	var got []*model.Task
	err = json.Unmarshal([]byte(textContent.Text), &got)
	assert.NoError(t, err)
	assert.Len(t, got, 2)
	// Ensure all have SessionID s1
	for _, g := range got {
		assert.Equal(t, "s1", g.SessionID)
	}
}
