// SPDX-License-Identifier: AGPL-3.0-only
package scheduler

import (
	"context"

	"github.com/jolks/mcp-cron/internal/model"
	"github.com/jolks/mcp-cron/internal/storage"
)

// Scheduler is the interface for the task scheduler
type Scheduler interface {
	AddTask(task *model.Task) error
	ListTasks() []*model.Task
	GetTask(id string) (*model.Task, error)
	RemoveTask(id string) error
	EnableTask(id string) error
	DisableTask(id string) error
	Start(ctx context.Context)
	Stop() error
	SetTaskExecutor(executor model.Executor)
	// SetStorage sets the storage backend for persisting and reloading tasks at runtime
	SetStorage(store storage.Storage)
}
