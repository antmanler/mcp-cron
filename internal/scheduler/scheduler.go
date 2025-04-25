// SPDX-License-Identifier: AGPL-3.0-only
package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jolks/mcp-cron/internal/errors"
	"github.com/jolks/mcp-cron/internal/model"
	"github.com/robfig/cron/v3"
)

// Scheduler manages cron tasks
type Scheduler struct {
	cron         *cron.Cron
	tasks        map[string]*model.Task
	entryIDs     map[string]cron.EntryID
	mu           sync.RWMutex
	taskExecutor model.Executor
}

// NewScheduler creates a new scheduler instance
func NewScheduler() *Scheduler {
	cronOpts := cron.New(
		cron.WithParser(cron.NewParser(
			cron.SecondOptional|cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow|cron.Descriptor)),
		cron.WithChain(
			cron.Recover(cron.DefaultLogger),
		),
	)

	scheduler := &Scheduler{
		cron:     cronOpts,
		tasks:    make(map[string]*model.Task),
		entryIDs: make(map[string]cron.EntryID),
	}

	return scheduler
}

// Start begins the scheduler
func (s *Scheduler) Start(ctx context.Context) {
	s.cron.Start()

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
func (s *Scheduler) Stop() error {
	s.cron.Stop()
	return nil
}

// AddTask adds a new task to the scheduler
func (s *Scheduler) AddTask(task *model.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[task.ID]; exists {
		return errors.AlreadyExists("task", task.ID)
	}

	// Store the task
	s.tasks[task.ID] = task

	if task.Enabled {
		err := s.scheduleTask(task)
		if err != nil {
			// If scheduling fails, set the task status to failed
			task.Status = model.StatusFailed
			return err
		}
	}

	return nil
}

// RemoveTask removes a task from the scheduler
func (s *Scheduler) RemoveTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.tasks[taskID]
	if !exists {
		return errors.NotFound("task", taskID)
	}

	// Remove the task from cron if it's scheduled
	if entryID, exists := s.entryIDs[taskID]; exists {
		s.cron.Remove(entryID)
		delete(s.entryIDs, taskID)
	}

	// Remove the task from our map
	delete(s.tasks, taskID)

	return nil
}

// EnableTask enables a disabled task
func (s *Scheduler) EnableTask(taskID string) error {
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

	return nil
}

// DisableTask disables a running task
func (s *Scheduler) DisableTask(taskID string) error {
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
	return nil
}

// GetTask retrieves a task by ID
func (s *Scheduler) GetTask(taskID string) (*model.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return nil, errors.NotFound("task", taskID)
	}

	return task, nil
}

// ListTasks returns all tasks
func (s *Scheduler) ListTasks() []*model.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*model.Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}

	return tasks
}

// UpdateTask updates an existing task
func (s *Scheduler) UpdateTask(task *model.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existingTask, exists := s.tasks[task.ID]
	if !exists {
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

	return nil
}

// SetTaskExecutor sets the executor to be used for task execution
func (s *Scheduler) SetTaskExecutor(executor model.Executor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskExecutor = executor
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
func (s *Scheduler) scheduleTask(task *model.Task) error {
	// Ensure we have a task executor
	if s.taskExecutor == nil {
		return fmt.Errorf("cannot schedule task: no task executor set")
	}

	// Create the job function that will execute when scheduled
	jobFunc := func() {
		task.LastRun = time.Now()
		task.Status = model.StatusRunning

		// Execute the task
		ctx := context.Background()
		timeout := 5 * time.Minute // Default timeout, could be made configurable

		if err := s.taskExecutor.Execute(ctx, task, timeout); err != nil {
			task.Status = model.StatusFailed
		} else {
			task.Status = model.StatusCompleted
		}

		task.UpdatedAt = time.Now()

		// Get next run time
		entries := s.cron.Entries()
		for _, entry := range entries {
			if entry.ID == s.entryIDs[task.ID] {
				task.NextRun = entry.Next
				break
			}
		}
	}

	// Add the job to cron
	entryID, err := s.cron.AddFunc(task.Schedule, jobFunc)
	if err != nil {
		return fmt.Errorf("failed to schedule task: %w", err)
	}

	// Store the cron entry ID
	s.entryIDs[task.ID] = entryID

	// Update the task's next run time
	entries := s.cron.Entries()
	for _, entry := range entries {
		if entry.ID == entryID {
			task.NextRun = entry.Next
			break
		}
	}

	return nil
}
