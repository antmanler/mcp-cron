// SPDX-License-Identifier: AGPL-3.0-only
package scheduler

import (
	"context"

	"github.com/jolks/mcp-cron/internal/model"
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
}
