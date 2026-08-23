package ginboot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	schedulerTracer = "github.com/klass-lk/ginboot/scheduler"
	// workerSpanName is stable and shared by every worker, with the worker's own
	// name carried as an attribute. A span named after the worker would make
	// "show me every scheduled run" impossible to express as one query.
	workerSpanName = "ginboot.worker.run"
)

// Worker defines an interface for structured background workers.
type Worker interface {
	Name() string
	Interval() time.Duration
	Execute(ctx context.Context) error
}

// TaskFunc is a simple function signature for background tasks.
type TaskFunc func(ctx context.Context) error

// TaskOptions configures execution behavior for background tasks.
type TaskOptions struct {
	Name         string
	Interval     time.Duration
	CronExpr     string
	RunOnStartup bool
	SkipOverlaps bool
}

type ScheduledTask struct {
	options  TaskOptions
	taskFunc TaskFunc
	running  bool
	mu       sync.Mutex

	// schedule is the parsed form of options.CronExpr, nil for interval tasks.
	// Parsed once at registration so a malformed expression is refused where
	// someone is watching, rather than on the first tick that needed it.
	schedule cron.Schedule
}

func (t *ScheduledTask) Options() TaskOptions {
	return t.options
}

// ScheduledEvent represents an incoming scheduled trigger from any cloud provider (AWS EventBridge, GCP, Azure, HTTP).
type ScheduledEvent struct {
	Provider string // e.g. "aws_eventbridge", "gcp_cloud_scheduler", "azure_event_grid", "http_cron"
	TaskName string // Target task name to execute
	Raw      []byte
}

// ScheduledEventParser parses raw request payloads into a ScheduledEvent.
type ScheduledEventParser interface {
	Parse(raw []byte) (*ScheduledEvent, bool)
}

// AWSEventBridgeParser parses AWS EventBridge / CloudWatch Scheduled Event payloads.
type AWSEventBridgeParser struct{}

func (p *AWSEventBridgeParser) Parse(raw []byte) (*ScheduledEvent, bool) {
	str := string(raw)
	if strings.Contains(str, `"aws.events"`) && strings.Contains(str, `"Scheduled Event"`) {
		// Attempt to extract task name if provided in resources/detail
		var eventStruct struct {
			Resources []string `json:"resources"`
		}
		taskName := ""
		if err := json.Unmarshal(raw, &eventStruct); err == nil && len(eventStruct.Resources) > 0 {
			// e.g. arn:aws:events:...:rule/ginboot-schedule-telemetry-cleanup-worker
			res := eventStruct.Resources[0]
			if idx := strings.LastIndex(res, "ginboot-schedule-"); idx != -1 {
				taskName = res[idx+len("ginboot-schedule-"):]
			}
		}
		return &ScheduledEvent{
			Provider: "aws_eventbridge",
			TaskName: taskName,
			Raw:      raw,
		}, true
	}
	return nil, false
}

// HTTPCronParser parses incoming HTTP requests with X-Ginboot-Cron-Task header.
type HTTPCronParser struct{}

func (p *HTTPCronParser) Parse(raw []byte) (*ScheduledEvent, bool) {
	str := string(raw)
	if strings.Contains(str, `"x-ginboot-cron-task"`) || strings.Contains(str, `"X-Ginboot-Cron-Task"`) {
		return &ScheduledEvent{
			Provider: "http_cron",
			Raw:      raw,
		}, true
	}
	return nil, false
}

type Scheduler struct {
	tasks      map[string]*ScheduledTask
	parsers    []ScheduledEventParser
	logger     Logger
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex

	// tick is how often this runtime is woken to look for due work. It bounds
	// the resolution of every schedule, so registration validates against it.
	tick time.Duration
	// unschedulableWarn keeps the explanation for skipping interval workers to once
	// per process. Repeated every tick it would be noise; said once at the first
	// tick that skipped something, it is the answer to why nothing ran.
	unschedulableWarn sync.Once
}

func NewScheduler(logger Logger) *Scheduler {
	if logger == nil {
		logger = NewSlogLogger(slog.Default())
	}
	s := &Scheduler{
		tasks:  make(map[string]*ScheduledTask),
		logger: logger,
		tick:   tickFromEnv(),
	}
	// Register default cloud event parsers
	s.RegisterParser(&AWSEventBridgeParser{})
	s.RegisterParser(&HTTPCronParser{})
	return s
}

// RegisterParser adds a cloud event parser for custom cloud providers.
func (s *Scheduler) RegisterParser(parser ScheduledEventParser) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.parsers = append(s.parsers, parser)
}

// RegisterTask adds a scheduled background task to the scheduler.
func (s *Scheduler) RegisterTask(options TaskOptions, fn TaskFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if options.Name == "" {
		options.Name = fmt.Sprintf("task-%d", len(s.tasks)+1)
	}
	if options.SkipOverlaps == false {
		options.SkipOverlaps = true
	}

	task := &ScheduledTask{
		options:  options,
		taskFunc: fn,
	}

	// A cron expression is parsed and checked here rather than at the first
	// firing, because "never fired" is indistinguishable from "not due yet" from
	// the outside. Refusing at registration puts the complaint in the startup
	// log, where whoever wrote the expression is still looking.
	if expr := strings.TrimSpace(options.CronExpr); expr != "" {
		schedule, err := parseCron(expr)
		if err != nil {
			s.logger.Error(fmt.Sprintf("[Scheduler] not registering worker '%s': %v", options.Name, err))
			return
		}
		if err := validateAgainstTick(options.Name, expr, schedule, s.tick); err != nil {
			s.logger.Error(fmt.Sprintf("[Scheduler] not registering worker: %v", err))
			return
		}
		task.schedule = schedule
	}

	s.tasks[options.Name] = task
}

// GetTasks returns all registered scheduled tasks.
func (s *Scheduler) GetTasks() map[string]*ScheduledTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make(map[string]*ScheduledTask)
	for k, v := range s.tasks {
		copied[k] = v
	}
	return copied
}

// ParseScheduledEvent evaluates raw payloads against registered cloud event parsers.
func (s *Scheduler) ParseScheduledEvent(raw []byte) (*ScheduledEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, parser := range s.parsers {
		if event, ok := parser.Parse(raw); ok {
			return event, true
		}
	}
	return nil, false
}

// ExecuteWorkerByName runs a specific registered worker directly.
func (s *Scheduler) ExecuteWorkerByName(ctx context.Context, name string) error {
	s.mu.Lock()
	task, exists := s.tasks[name]
	s.mu.Unlock()

	if !exists {
		// If only one task exists, fallback to it
		s.mu.Lock()
		if len(s.tasks) == 1 {
			for _, t := range s.tasks {
				task = t
				exists = true
			}
		}
		s.mu.Unlock()
	}

	if !exists {
		return fmt.Errorf("worker '%s' not registered in scheduler", name)
	}

	return s.executeTaskSafe(ctx, task)
}

// ExecuteAllWorkers executes every registered task once, regardless of whether
// it is due.
//
// Deprecated: use ExecuteDueWorkers. Driving a tick with this gives every worker
// the tick's schedule instead of its own — an hourly job firing twelve times an
// hour. It remains for callers that genuinely mean "run everything now", such as
// a manual trigger.
func (s *Scheduler) ExecuteAllWorkers(ctx context.Context) map[string]error {
	s.mu.Lock()
	tasks := make([]*ScheduledTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	s.mu.Unlock()

	results := make(map[string]error)
	for _, t := range tasks {
		err := s.executeTaskSafe(ctx, t)
		results[t.options.Name] = err
	}
	return results
}

// Start begins executing all registered scheduled tasks in dedicated goroutines (Standalone Server Mode).
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	s.cancelFunc = cancel

	for _, task := range s.tasks {
		s.wg.Add(1)
		go s.runTaskLoop(ctx, task)
	}
}

func (s *Scheduler) runTaskLoop(ctx context.Context, t *ScheduledTask) {
	defer s.wg.Done()

	if t.options.RunOnStartup {
		_ = s.executeTaskSafe(ctx, t)
	}

	// A cron task has no interval to build a ticker from, and time.NewTicker
	// panics on a non-positive duration — so this is not merely the wrong
	// schedule for it, it is a crash at startup. Scheduling it by expression here
	// also means a worker behaves the same in development, where this loop runs,
	// as it does deployed, where the tick does the waking. A schedule that only
	// takes effect in production is a schedule nobody can test.
	if t.schedule != nil {
		s.runCronLoop(ctx, t)
		return
	}

	ticker := time.NewTicker(t.options.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.executeTaskSafe(ctx, t)
		}
	}
}

// runCronLoop waits for each firing in turn rather than polling.
//
// The deployed runtime evaluates a window because it is woken on someone else's
// schedule and has to ask what it missed. A process that is always running has
// no such constraint: it can be told exactly when the next firing is and sleep
// until then, which is both more accurate and cheaper than waking to discover
// there is nothing to do.
func (s *Scheduler) runCronLoop(ctx context.Context, t *ScheduledTask) {
	for {
		// UTC for the reason dueIn documents: a schedule must not mean a
		// different instant here than it will once deployed.
		now := time.Now().UTC()
		next := t.schedule.Next(now)
		if next.IsZero() {
			s.logger.Warn(fmt.Sprintf("[Scheduler] worker '%s' has no further firings; its loop is stopping", t.options.Name))
			return
		}

		timer := time.NewTimer(next.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			_ = s.executeTaskSafe(ctx, t)
		}
	}
}

func (s *Scheduler) executeTaskSafe(ctx context.Context, t *ScheduledTask) (err error) {
	if t.options.SkipOverlaps {
		t.mu.Lock()
		if t.running {
			s.logger.Warn(fmt.Sprintf("[Scheduler] Skipping overlapping execution for worker '%s'", t.options.Name))
			t.mu.Unlock()
			return nil
		}
		t.running = true
		t.mu.Unlock()

		defer func() {
			t.mu.Lock()
			t.running = false
			t.mu.Unlock()
		}()
	}

	// The run span, and the record of what it did.
	//
	// Started here rather than around the whole tick so that one span describes
	// one run of one worker: a tick's span would report a duration that belongs
	// to whichever workers happened to be due, which is the number nobody can act
	// on. Registered before the recovery below so that it closes after it — defers
	// run last-registered-first — and therefore observes the error a panic
	// produced rather than reporting the run as successful.
	//
	// otel.Tracer resolves to a no-op unless an application imported
	// instrumentation, so this costs an interface call in the applications that
	// did not.
	ctx, span := otel.Tracer(schedulerTracer).Start(ctx, workerSpanName,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("ginboot.worker", t.options.Name),
			attribute.String("ginboot.worker.schedule", t.scheduleDescription()),
		))
	runStart := time.Now()
	defer func() {
		span.SetAttributes(
			attribute.String("ginboot.worker.outcome", outcomeOf(err)),
			attribute.Int64("ginboot.worker.duration_ms", time.Since(runStart).Milliseconds()),
		)
		switch {
		case err == nil || errors.Is(err, ErrIncomplete):
			// Unfinished is not failed. Marking it an error would put a job that
			// is working through a backlog exactly as designed into the same
			// bucket as one that is broken.
			span.SetStatus(codes.Ok, "")
		default:
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	// Panic Recovery
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			err = fmt.Errorf("worker panicked: %v", r)
			s.logger.Error(fmt.Sprintf("[Scheduler Error] Worker '%s' panicked: %v\n%s", t.options.Name, r, stack))
		}
	}()

	start := time.Now()
	s.logger.Info(fmt.Sprintf("[Scheduler] Starting worker '%s'...", t.options.Name))

	err = t.taskFunc(ctx)
	duration := time.Since(start)

	if err != nil {
		s.logger.Error(fmt.Sprintf("[Scheduler Error] Worker '%s' failed in %v: %v", t.options.Name, duration, err))
	} else {
		s.logger.Info(fmt.Sprintf("[Scheduler] Worker '%s' completed successfully in %v", t.options.Name, duration))
	}
	return err
}

// Stop gracefully stops all running background workers, waiting up to timeout for completion.
func (s *Scheduler) Stop(timeout time.Duration) {
	s.mu.Lock()
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("[Scheduler] All background workers shut down gracefully.")
	case <-time.After(timeout):
		s.logger.Warn("[Scheduler] Timed out waiting for background workers to shut down.")
	}
}
