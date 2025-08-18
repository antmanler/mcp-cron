// SPDX-License-Identifier: AGPL-3.0-only
package scheduler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jolks/mcp-cron/internal/config"
	"github.com/jolks/mcp-cron/internal/model"
	"github.com/jolks/mcp-cron/internal/storage"
)

// dummy executor to satisfy scheduler requirements
type nopExecutor struct{}

func (n nopExecutor) Execute(ctx context.Context, task *model.Task, timeout time.Duration) error {
	return nil
}

func TestScheduler_PersistsOnMutations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	// initialize storage and scheduler
	store, err := storage.NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	sched := NewScheduler(&config.SchedulerConfig{DefaultTimeout: time.Minute})
	sched.SetTaskExecutor(nopExecutor{})
	sched.SetStorage(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)
	defer func() {
		if err := sched.Stop(); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	}()

	// Add a task
	task := &model.Task{ID: "t1", Name: "task1", Schedule: "* * * * *", Enabled: false}
	if err := sched.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Allow async save
	time.Sleep(200 * time.Millisecond)

	// Load from storage file
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var tasks []*model.Task
	if err := json.Unmarshal(b, &tasks); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "t1" {
		t.Fatalf("unexpected persisted tasks: %+v", tasks)
	}

	// Remove (soft delete) and ensure persisted update
	if err := sched.RemoveTask("t1"); err != nil {
		t.Fatalf("RemoveTask: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	tasks = nil
	if err := json.Unmarshal(b, &tasks); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(tasks) != 1 || !tasks[0].Deleted || tasks[0].Status != model.StatusDeleted {
		t.Fatalf("expected soft-deleted task persisted with deleted=true, got: %+v", tasks)
	}
}

func TestScheduler_ReloadsOnExternalChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	store, err := storage.NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	sched := NewScheduler(&config.SchedulerConfig{DefaultTimeout: time.Minute})
	sched.SetTaskExecutor(nopExecutor{})
	sched.SetStorage(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)
	defer func() {
		if err := sched.Stop(); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	}()

	// Simulate external writer updating file
	external := []*model.Task{
		{ID: "ext1", Name: "external", Schedule: "* * * * *", Enabled: false},
	}
	b, _ := json.Marshal(external)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Wait for watcher to pick up change
	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for reload")
		}
		tasks := sched.ListTasks()
		if len(tasks) == 1 && tasks[0].ID == "ext1" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
}
