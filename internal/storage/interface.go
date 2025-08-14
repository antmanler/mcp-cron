// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"

	"github.com/jolks/mcp-cron/internal/model"
)

// Event represents a storage change event. For now, it's a simple signal to reload.
type Event struct{}

// Storage abstracts task persistence backends.
type Storage interface {
	// Load returns all tasks from storage.
	Load(ctx context.Context) ([]*model.Task, error)
	// Save persists all tasks atomically to storage.
	Save(ctx context.Context, tasks []*model.Task) error
	// Watch starts watching for underlying storage changes and emits events.
	// The returned channel is closed when the context is done or on fatal watcher error.
	Watch(ctx context.Context) (<-chan Event, error)
	// Close releases any resources used by the storage.
	Close() error
}
