package ginboot

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

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
	if err == nil || !errors.Is(err, err) {
		if err == nil {
			t.Fatal("expected error from panicked worker")
		}
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
