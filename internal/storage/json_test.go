// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jolks/mcp-cron/internal/model"
)

func TestJSONStorage_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	store, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	defer store.Close()

	tasks := []*model.Task{
		{ID: "t1", Name: "task1", Schedule: "* * * * *", Enabled: false, Status: model.StatusPending},
		{ID: "t2", Name: "task2", Schedule: "*/5 * * * *", Enabled: false, Status: model.StatusDisabled},
	}
	if err := store.Save(context.Background(), tasks); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("got %d tasks, want 2", len(loaded))
	}
	if loaded[0].ID != "t1" || loaded[1].ID != "t2" {
		t.Fatalf("unexpected tasks: %+v", loaded)
	}
}

func TestJSONStorage_WatchEmitsOnChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	store, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := store.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Write initial content
	initial := []*model.Task{{ID: "t1", Name: "task1", Schedule: "* * * * *"}}
	b, _ := json.Marshal(initial)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Expect an event (with debounce); allow up to 2s
	select {
	case <-ch:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("expected watch event after file write")
	}
}
