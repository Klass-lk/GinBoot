package ginboot

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type mockStructWorker struct {
	executed atomic.Int32
}

func (m *mockStructWorker) Name() string {
	return "mock-struct-worker"
}

func (m *mockStructWorker) Interval() time.Duration {
	return 100 * time.Millisecond
}

func (m *mockStructWorker) Execute(ctx context.Context) error {
	m.executed.Add(1)
	return nil
}

func TestScheduler_BasicExecution(t *testing.T) {
	scheduler := NewScheduler(nil)

	var count int32
	scheduler.RegisterTask(TaskOptions{
		Name:         "test-task",
		Interval:     50 * time.Millisecond,
		RunOnStartup: true,
	}, func(ctx context.Context) error {
		atomic.AddInt32(&count, 1)
		return nil
	})

	ctx := context.Background()
	scheduler.Start(ctx)
	time.Sleep(120 * time.Millisecond)
	scheduler.Stop(100 * time.Millisecond)

	if atomic.LoadInt32(&count) < 2 {
		t.Fatalf("expected at least 2 executions, got %d", count)
	}
}

func TestScheduler_PanicRecovery(t *testing.T) {
	scheduler := NewScheduler(nil)

	scheduler.RegisterTask(TaskOptions{
		Name: "panic-task",
	}, func(ctx context.Context) error {
		panic("boom")
	})

	err := scheduler.ExecuteWorkerByName(context.Background(), "panic-task")
	if err == nil {
		t.Fatal("expected error from panicked worker")
	}
}

func TestScheduler_EventBridgeParser(t *testing.T) {
	scheduler := NewScheduler(nil)

	payload := []byte(`{
		"source": "aws.events",
		"detail-type": "Scheduled Event",
		"resources": ["arn:aws:events:ap-southeast-1:12345:rule/ginboot-schedule-telemetry-worker"]
	}`)

	event, ok := scheduler.ParseScheduledEvent(payload)
	if !ok {
		t.Fatal("expected EventBridge parser to recognize payload")
	}
	if event.Provider != "aws_eventbridge" {
		t.Fatalf("expected provider aws_eventbridge, got %s", event.Provider)
	}
	if event.TaskName != "telemetry-worker" {
		t.Fatalf("expected taskName telemetry-worker, got %s", event.TaskName)
	}
}

func TestScheduler_HTTPCronParser(t *testing.T) {
	scheduler := NewScheduler(nil)

	payload := []byte(`{
		"headers": {
			"X-Ginboot-Cron-Task": "session-cleanup"
		}
	}`)

	event, ok := scheduler.ParseScheduledEvent(payload)
	if !ok {
		t.Fatal("expected HTTPCronParser to recognize payload")
	}
	if event.Provider != "http_cron" {
		t.Fatalf("expected provider http_cron, got %s", event.Provider)
	}
}

func TestScheduler_ExecuteWorkerNotFound(t *testing.T) {
	scheduler := NewScheduler(nil)
	err := scheduler.ExecuteWorkerByName(context.Background(), "non-existent-worker")
	if err == nil {
		t.Fatal("expected error when executing non-existent worker")
	}
}

func TestScheduler_ExecuteAllWorkers(t *testing.T) {
	scheduler := NewScheduler(nil)

	var run1, run2 int32

	scheduler.RegisterTask(TaskOptions{Name: "worker-1"}, func(ctx context.Context) error {
		atomic.StoreInt32(&run1, 1)
		return nil
	})
	scheduler.RegisterTask(TaskOptions{Name: "worker-2"}, func(ctx context.Context) error {
		atomic.StoreInt32(&run2, 1)
		return nil
	})

	results := scheduler.ExecuteAllWorkers(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 worker execution results, got %d", len(results))
	}
	if atomic.LoadInt32(&run1) != 1 || atomic.LoadInt32(&run2) != 1 {
		t.Fatal("expected both workers to be executed by ExecuteAllWorkers")
	}
}

func TestServer_WorkerRegistration(t *testing.T) {
	server := New()

	mockWorker := &mockStructWorker{}
	server.RegisterWorkerStruct(mockWorker)

	tasks := server.Scheduler().GetTasks()
	if _, exists := tasks["mock-struct-worker"]; !exists {
		t.Fatal("expected mock-struct-worker to be registered on Server")
	}

	err := server.Scheduler().ExecuteWorkerByName(context.Background(), "mock-struct-worker")
	if err != nil {
		t.Fatalf("unexpected error executing registered struct worker: %v", err)
	}

	if mockWorker.executed.Load() != 1 {
		t.Fatalf("expected mockWorker to execute once, got %d", mockWorker.executed.Load())
	}
}
