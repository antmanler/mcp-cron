package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	urlpkg "net/url"
	"testing"
	"time"

	"github.com/jolks/mcp-cron/internal/config"
	"github.com/jolks/mcp-cron/internal/model"
	"github.com/jolks/mcp-cron/internal/scheduler"
)

// TestSendMessageToLocalServer verifies that the helper sends a request to the
// configured local server endpoint with the expected parameters.
func TestSendMessageToLocalServer(t *testing.T) {
	received := make(chan *http.Request, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	u, err := urlpkg.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	t.Setenv("LOCAL_SERVER_PUBLIC_HOST", host)
	t.Setenv("LOCAL_SERVER_PORT", port)

	if err := sendMessageToLocalServer(context.Background(), "room1", "hello"); err != nil {
		t.Fatalf("sendMessageToLocalServer returned error: %v", err)
	}

	select {
	case req := <-received:
		if req.URL.Path != "/room/room1/append" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		q := req.URL.Query()
		if got := q.Get("sender"); got != "runtime_reminder" {
			t.Fatalf("sender = %q, want runtime_reminder", got)
		}
		if got := q.Get("content"); got != "hello" {
			t.Fatalf("content = %q, want hello", got)
		}
		if got := q.Get("tag"); got != "reply" {
			t.Fatalf("tag = %q, want reply", got)
		}
	case <-time.After(time.Second):
		t.Fatal("no request received")
	}
}

// helper to set up test server and environment
func setupTestLocalServer(t *testing.T) (*httptest.Server, chan *http.Request) {
	received := make(chan *http.Request, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r
		w.WriteHeader(http.StatusOK)
	}))

	u, err := urlpkg.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	t.Setenv("LOCAL_SERVER_PUBLIC_HOST", host)
	t.Setenv("LOCAL_SERVER_PORT", port)

	return ts, received
}

// TestExecuteRemovesOneTimeTask ensures ONCE tasks are removed after execution.
func TestExecuteRemovesOneTimeTask(t *testing.T) {
	ts, received := setupTestLocalServer(t)
	defer ts.Close()

	sched := scheduler.NewScheduler(&config.SchedulerConfig{DefaultTimeout: time.Minute})
	srv := &MCPServer{scheduler: sched}

	task := &model.Task{
		ID:        "task1",
		Name:      "chat1",
		SessionID: "chat1",
		Schedule:  "* * * * *",
		RawInput:  "{\"foo\":\"bar\"}",
		TaskType:  "ONCE",
		Enabled:   false,
	}
	if err := sched.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	if err := srv.Execute(context.Background(), task, time.Minute); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	select {
	case <-received:
		// request received
	case <-time.After(time.Second):
		t.Fatal("expected request to local server")
	}

	if _, err := sched.GetTask(task.ID); err == nil {
		t.Fatalf("expected task to be soft-deleted (not retrievable) after execution")
	}
}

// TestExecuteKeepsPeriodicTask ensures PERIODICALLY tasks remain scheduled after execution.
func TestExecuteKeepsPeriodicTask(t *testing.T) {
	ts, received := setupTestLocalServer(t)
	defer ts.Close()

	sched := scheduler.NewScheduler(&config.SchedulerConfig{DefaultTimeout: time.Minute})
	srv := &MCPServer{scheduler: sched}

	task := &model.Task{
		ID:        "task2",
		Name:      "chat2",
		SessionID: "chat2",
		Schedule:  "* * * * *",
		RawInput:  "{\"foo\":\"baz\"}",
		TaskType:  "PERIODICALLY",
		Enabled:   false,
	}
	if err := sched.AddTask(task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	if err := srv.Execute(context.Background(), task, time.Minute); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("expected request to local server")
	}

	if _, err := sched.GetTask(task.ID); err != nil {
		t.Fatalf("expected task to remain after execution, got error: %v", err)
	}
}

// TestSendMessageToLocalServerCustomSender verifies that the sender can be
// configured via the MCP_CRON_SENDER environment variable.
func TestSendMessageToLocalServerCustomSender(t *testing.T) {
	received := make(chan *http.Request, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	u, err := urlpkg.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	t.Setenv("LOCAL_SERVER_PUBLIC_HOST", host)
	t.Setenv("LOCAL_SERVER_PORT", port)
	t.Setenv("MCP_CRON_SENDER", "custom_sender")

	if err := sendMessageToLocalServer(context.Background(), "room1", "hello"); err != nil {
		t.Fatalf("sendMessageToLocalServer returned error: %v", err)
	}

	select {
	case req := <-received:
		q := req.URL.Query()
		if got := q.Get("sender"); got != "custom_sender" {
			t.Fatalf("sender = %q, want custom_sender", got)
		}
	case <-time.After(time.Second):
		t.Fatal("no request received")
	}
}
