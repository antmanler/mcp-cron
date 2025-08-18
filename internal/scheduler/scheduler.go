// SPDX-License-Identifier: AGPL-3.0-only
package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jolks/mcp-cron/internal/config"
	"github.com/jolks/mcp-cron/internal/errors"
	"github.com/jolks/mcp-cron/internal/model"
	"github.com/jolks/mcp-cron/internal/storage"
	"github.com/robfig/cron/v3"
)

// Scheduler manages cron tasks
type cronScheduler struct {
	cron         *cron.Cron
	tasks        map[string]*model.Task
	entryIDs     map[string]cron.EntryID
	mu           sync.RWMutex
	taskExecutor model.Executor
	config       *config.SchedulerConfig
	store        storage.Storage
	cancelWatch  context.CancelFunc
}

// NewScheduler creates a new scheduler instance
func NewScheduler(cfg *config.SchedulerConfig) Scheduler {
	cronOpts := cron.New(
		cron.WithParser(cron.NewParser(
			cron.SecondOptional|cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow|cron.Descriptor)),
		cron.WithChain(
			cron.Recover(cron.DefaultLogger),
		),
	)

	scheduler := &cronScheduler{
		cron:     cronOpts,
		tasks:    make(map[string]*model.Task),
		entryIDs: make(map[string]cron.EntryID),
		config:   cfg,
	}

	return scheduler
}

// Start begins the scheduler
func (s *cronScheduler) Start(ctx context.Context) {
	s.cron.Start()

	// Initial load from storage if configured
	s.loadFromStorage(ctx)

	// Start watching storage for changes
	s.startWatch(ctx)

	// Listen for context cancellation to stop the scheduler
	go func() {
		<-ctx.Done()
		if err := s.Stop(); err != nil {
			// We cannot return the error here since we're in a goroutine,
			// so we'll just log it
			fmt.Printf("Error stopping scheduler: %v\n", err)
		}
	}()
}

// Stop halts the scheduler
func (s *cronScheduler) Stop() error {
	s.cron.Stop()
	// stop storage watch
	if s.cancelWatch != nil {
		s.cancelWatch()
	}
	if s.store != nil {
		_ = s.store.Close()
	}
	return nil
}

// AddTask adds a new task to the scheduler
func (s *cronScheduler) AddTask(task *model.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if task.ID == "" {
		// Defensive: scheduler should not accept empty IDs; server generates IDs
		return errors.InvalidInput("task ID must be set by server")
	}

	if existing, exists := s.tasks[task.ID]; exists {
		// If an existing task (even deleted) has this ID, reject to avoid collisions/resurrection
		if existing != nil {
			return errors.AlreadyExists("task", task.ID)
		}
		return errors.AlreadyExists("task", task.ID)
	}

	// Store the task
	task.Deleted = false
	task.DeletedAt = time.Time{}
	s.tasks[task.ID] = task

	if task.Enabled {
		err := s.scheduleTask(task)
		if err != nil {
			// If scheduling fails, set the task status to failed
			task.Status = model.StatusFailed
			return err
		}
	}

	// Persist change
	s.persistLocked()

	return nil
}

// RemoveTask removes a task from the scheduler
func (s *cronScheduler) RemoveTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.tasks[taskID]
	if !exists {
		return errors.NotFound("task", taskID)
	}

	// Remove the task from cron if it's scheduled
	if entryID, exists := s.entryIDs[taskID]; exists {
		s.cron.Remove(entryID)
		delete(s.entryIDs, taskID)
	}

	// Soft-delete: mark deleted but keep it for history
	now := time.Now()
	t.Enabled = false
	t.Status = model.StatusDeleted
	t.Deleted = true
	t.DeletedAt = now
	t.UpdatedAt = now

	// Persist change
	s.persistLocked()

	return nil
}

// EnableTask enables a disabled task
func (s *cronScheduler) EnableTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return errors.NotFound("task", taskID)
	}

	if task.Enabled {
		return nil // Already enabled
	}

	task.Enabled = true
	task.UpdatedAt = time.Now()

	err := s.scheduleTask(task)
	if err != nil {
		// If scheduling fails, set the task status to failed
		task.Status = model.StatusFailed
		return err
	}

	// Persist change
	s.persistLocked()

	return nil
}

// DisableTask disables a running task
func (s *cronScheduler) DisableTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return errors.NotFound("task", taskID)
	}

	if !task.Enabled {
		return nil // Already disabled
	}

	// Remove from cron
	if entryID, exists := s.entryIDs[taskID]; exists {
		s.cron.Remove(entryID)
		delete(s.entryIDs, taskID)
	}

	task.Enabled = false
	task.Status = model.StatusDisabled
	task.UpdatedAt = time.Now()
	// Persist change
	s.persistLocked()
	return nil
}

// GetTask retrieves a task by ID
func (s *cronScheduler) GetTask(taskID string) (*model.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return nil, errors.NotFound("task", taskID)
	}

	// Treat soft-deleted tasks as not found to consumers
	if task.Deleted {
		return nil, errors.NotFound("task", taskID)
	}

	return task, nil
}

// ListTasks returns all tasks
func (s *cronScheduler) ListTasks() []*model.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*model.Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		if task != nil && !task.Deleted {
			tasks = append(tasks, task)
		}
	}

	return tasks
}

// UpdateTask updates an existing task
func (s *cronScheduler) UpdateTask(task *model.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existingTask, exists := s.tasks[task.ID]
	if !exists {
		return errors.NotFound("task", task.ID)
	}

	if existingTask.Deleted {
		return errors.NotFound("task", task.ID)
	}

	// If the task was scheduled, remove it
	if existingTask.Enabled {
		if entryID, exists := s.entryIDs[task.ID]; exists {
			s.cron.Remove(entryID)
			delete(s.entryIDs, task.ID)
		}
	}

	// Update the task
	task.UpdatedAt = time.Now()
	s.tasks[task.ID] = task

	// If enabled, schedule it
	if task.Enabled {
		return s.scheduleTask(task)
	}

	// Persist change
	s.persistLocked()

	return nil
}

// SetTaskExecutor sets the executor to be used for task execution
func (s *cronScheduler) SetTaskExecutor(executor model.Executor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskExecutor = executor
}

// SetStorage sets the storage backend and performs an initial load if scheduler already started later.
func (s *cronScheduler) SetStorage(store storage.Storage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
}

// NewTask creates a new task with default values
func NewTask() *model.Task {
	now := time.Now()
	return &model.Task{
		Enabled:   false,
		Status:    model.StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// scheduleTask adds a task to the cron scheduler (internal method)
func (s *cronScheduler) scheduleTask(task *model.Task) error {
	// Ensure we have a task executor
	if s.taskExecutor == nil {
		return fmt.Errorf("cannot schedule task: no task executor set")
	}

	// Create the job function that will execute when scheduled
	const maxLogEntries = 50
	jobFunc := func() {
		start := time.Now()
		task.LastRun = start
		task.Status = model.StatusRunning

		// Execute the task
		ctx := context.Background()
		timeout := s.config.DefaultTimeout // Use the configured default timeout

		execErr := s.taskExecutor.Execute(ctx, task, timeout)
		if execErr != nil {
			task.Status = model.StatusFailed
		} else {
			task.Status = model.StatusCompleted
		}

		end := time.Now()
		task.UpdatedAt = end
		// Append execution result
		res := model.Result{
			TaskID:    task.ID,
			Output:    "",
			Error:     "",
			ExitCode:  0,
			StartTime: start,
			EndTime:   end,
			Duration:  end.Sub(start).String(),
		}
		if execErr != nil {
			res.Error = execErr.Error()
			res.ExitCode = 1
		}
		task.ExecutionLog = append(task.ExecutionLog, res)
		if len(task.ExecutionLog) > maxLogEntries {
			// keep last maxLogEntries
			task.ExecutionLog = task.ExecutionLog[len(task.ExecutionLog)-maxLogEntries:]
		}
		s.updateNextRunTime(task)
		// Persist status and execution log
		s.persistLocked()
	}

	// Add the job to cron
	entryID, err := s.cron.AddFunc(task.Schedule, jobFunc)
	if err != nil {
		return fmt.Errorf("failed to schedule task: %w", err)
	}

	// Store the cron entry ID
	s.entryIDs[task.ID] = entryID
	s.updateNextRunTime(task)

	return nil
}

// updateNextRunTime updates the task's next run time based on its cron entry
func (s *cronScheduler) updateNextRunTime(task *model.Task) {
	if entryID, exists := s.entryIDs[task.ID]; exists {
		entries := s.cron.Entries()
		for _, entry := range entries {
			if entry.ID == entryID {
				task.NextRun = entry.Next
				break
			}
		}
	}
}

// persistLocked saves tasks to storage. Caller must hold s.mu for reading the map safely.
func (s *cronScheduler) persistLocked() {
	if s.store == nil {
		return
	}
	tasks := make([]*model.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		// create shallow copy to avoid races
		tt := *t
		tasks = append(tasks, &tt)
	}
	go func(tasks []*model.Task) {
		// background save; ignore context for now
		_ = s.store.Save(context.Background(), tasks)
	}(tasks)
}

// loadFromStorage loads tasks from storage and reconciles scheduler state.
func (s *cronScheduler) loadFromStorage(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		return
	}
	tasks, err := s.store.Load(ctx)
	if err != nil {
		fmt.Printf("failed to load tasks from storage: %v\n", err)
		return
	}
	// Clear existing schedules and tasks, then rebuild from storage
	for id, eid := range s.entryIDs {
		s.cron.Remove(eid)
		delete(s.entryIDs, id)
	}
	s.tasks = make(map[string]*model.Task)
	for _, t := range tasks {
		if t == nil || t.ID == "" {
			continue
		}
		if prev, ok := s.tasks[t.ID]; ok {
			// keep the newer one by UpdatedAt (fallback to CreatedAt)
			prevTime := prev.UpdatedAt
			if prevTime.IsZero() {
				prevTime = prev.CreatedAt
			}
			curTime := t.UpdatedAt
			if curTime.IsZero() {
				curTime = t.CreatedAt
			}
			if curTime.Before(prevTime) {
				continue
			}
		}
		s.tasks[t.ID] = t
		if t.Enabled && !t.Deleted {
			if err := s.scheduleTask(t); err != nil {
				fmt.Printf("failed to schedule loaded task %s: %v\n", t.ID, err)
			}
		}
	}
}

// startWatch starts watching storage for changes if available and config indicates watch.
func (s *cronScheduler) startWatch(parent context.Context) {
	if s.store == nil {
		return
	}
	// We always start watching; storage implementation may no-op. Higher-level enable via config handled by JSONStorage creation decision.
	ctx, cancel := context.WithCancel(parent)
	s.cancelWatch = cancel
	ch, err := s.store.Watch(ctx)
	if err != nil {
		// if watcher cannot start, just ignore watching.
		fmt.Printf("failed to start storage watcher: %v\n", err)
		return
	}
	go func() {
		for range ch {
			s.loadFromStorage(context.Background())
		}
	}()
}
