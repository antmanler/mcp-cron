// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jolks/mcp-cron/internal/model"
)

// JSONStorage persists tasks to a single JSON file and watches it for changes.
type JSONStorage struct {
	path    string
	mu      sync.RWMutex
	watcher *fsnotify.Watcher
}

// NewJSONStorage creates a new JSON storage backed by the given file path.
func NewJSONStorage(path string) (*JSONStorage, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &JSONStorage{path: abs}, nil
}

// Load implements Storage.Load.
func (s *JSONStorage) Load(ctx context.Context) ([]*model.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []*model.Task{}, nil
		}
		return nil, fmt.Errorf("open tasks file: %w", err)
	}
	defer f.Close()

	var tasks []*model.Task
	dec := json.NewDecoder(bufio.NewReader(f))
	if err := dec.Decode(&tasks); err != nil {
		return nil, fmt.Errorf("decode tasks: %w", err)
	}
	return tasks, nil
}

// Save implements Storage.Save.
func (s *JSONStorage) Save(ctx context.Context, tasks []*model.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open temp file: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(tasks); err != nil {
		f.Close()
		return fmt.Errorf("encode tasks: %w", err)
	}
	if err := f.Sync(); err != nil { // ensure contents flushed for atomic rename
		f.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	return os.Rename(tmp, s.path)
}

// Watch implements Storage.Watch.
func (s *JSONStorage) Watch(ctx context.Context) (<-chan Event, error) {
	// Ensure directory exists to watch
	dir := filepath.Dir(s.path)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir storage dir: %w", err)
		}
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}
	s.watcher = w
	if err := w.Add(dir); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("watch dir: %w", err)
	}

	ch := make(chan Event)

	go func() {
		defer close(ch)
		defer w.Close()

		// Debounce events for the target file to avoid duplicate reloads during atomic save
		const debounce = 200 * time.Millisecond
		var timer *time.Timer
		var timerC <-chan time.Time
		var pending bool

		stopTimer := func() {
			if timer != nil {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer = nil
				timerC = nil
			}
		}

		startTimer := func() {
			if timer == nil {
				timer = time.NewTimer(debounce)
				timerC = timer.C
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(debounce)
			}
		}

		for {
			select {
			case <-ctx.Done():
				stopTimer()
				return
			case evt, ok := <-w.Events:
				if !ok {
					stopTimer()
					return
				}
				if filepath.Clean(evt.Name) != s.path {
					continue
				}
				if evt.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) != 0 {
					pending = true
					startTimer()
				}
			case <-timerC:
				if pending {
					// Non-blocking send: if receiver gone due to close, select will choose ctx.Done first on next loop
					select {
					case ch <- Event{}:
					case <-ctx.Done():
						stopTimer()
						return
					}
					pending = false
				}
				stopTimer()
			case _, ok := <-w.Errors:
				if !ok {
					stopTimer()
					return
				}
				// ignore error
			}
		}
	}()

	return ch, nil
}

// Close implements Storage.Close.
func (s *JSONStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watcher != nil {
		return s.watcher.Close()
	}
	return nil
}
