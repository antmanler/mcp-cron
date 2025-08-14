// SPDX-License-Identifier: AGPL-3.0-only
package model

import (
	"context"
	"time"
)

// TaskStatus represents the current status of a task
type TaskStatus string

// Task status constants
const (
	// StatusPending indicates a task that has not been run yet
	StatusPending TaskStatus = "pending"
	// StatusRunning indicates a task that is currently running
	StatusRunning TaskStatus = "running"
	// StatusCompleted indicates a task that has successfully completed
	StatusCompleted TaskStatus = "completed"
	// StatusFailed indicates a task that has failed
	StatusFailed TaskStatus = "failed"
	// StatusDisabled indicates a task that is disabled
	StatusDisabled TaskStatus = "disabled"
)

// String returns the string representation of the status, making it easier to use in string contexts
func (s TaskStatus) String() string {
	return string(s)
}

// FileRef represents a file reference for task context
type FileRef struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

// Task represents a scheduled task
type Task struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Description          string     `json:"description"`
	Schedule             string     `json:"schedule"`
	Instruction          string     `json:"instruction"`
	TaskType             string     `json:"task_type"`
	RelevantChatSnippets []string   `json:"relevant_chat_snippets,omitempty"`
	FileContext          []FileRef  `json:"file_context,omitempty"`
	RawInput             string     `json:"raw_input,omitempty"`
	Enabled              bool       `json:"enabled"`
	LastRun              time.Time  `json:"lastRun,omitempty"`
	NextRun              time.Time  `json:"nextRun,omitempty"`
	Status               TaskStatus `json:"status"`
	CreatedAt            time.Time  `json:"createdAt,omitempty"`
	UpdatedAt            time.Time  `json:"updatedAt,omitempty"`
}

// Result contains the results of a task execution
type Result struct {
	TaskID    string    `json:"task_id"`
	Output    string    `json:"output"`
	Error     string    `json:"error,omitempty"`
	ExitCode  int       `json:"exit_code"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  string    `json:"duration"`
}

// Executor defines the interface for executing tasks
type Executor interface {
	Execute(ctx context.Context, task *Task, timeout time.Duration) error
}

// ResultProvider defines an interface for getting task execution results
type ResultProvider interface {
	GetTaskResult(taskID string) (*Result, bool)
}
