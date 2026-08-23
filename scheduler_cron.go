package ginboot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduling on a runtime that freezes between invocations.
//
// A long-running server keeps a goroutine per task and a ticker decides when it
// fires. A function does not: it is woken from outside, does what is due, and is
// frozen again with no memory that it ran. Everything in this file exists to
// answer one question in that world — given that something woke us now, which
// tasks should run?

// DefaultTickInterval is how often a deployed application is woken to check for
// due work when nothing says otherwise.
//
// It is also the resolution of every schedule: a task can be up to one tick
// late, and an expression that wants to fire more often than this cannot be
// honoured. Five minutes trades precision nobody asked for against waking every
// application in the fleet twelve times an hour.
const DefaultTickInterval = 5 * time.Minute

// TickIntervalEnv names the environment variable a deployment uses to override
// the tick. It has to match what created the schedule that does the waking —
// the platform sets both from one value.
const TickIntervalEnv = "GINBOOT_SCHEDULER_TICK"

// CronWorker is a Worker scheduled by expression rather than by interval.
//
// Separate from Worker rather than added to it, so that implementing one does
// not break every existing worker. A type satisfying both is scheduled by its
// cron expression: it is the more specific statement of intent, and the only
// one that survives a freeze without external state.
type CronWorker interface {
	Name() string
	Cron() string
	Execute(ctx context.Context) error
}

// ErrIncomplete reports that a worker ran correctly but did not finish.
//
// The alternative to having this is a worker that must either finish inside one
// invocation or report failure, which pushes anything larger than a single
// budget out of the scheduler entirely. Returned instead, the run is recorded as
// unfinished rather than failed, and the worker is expected to have stored
// enough of its own progress to resume — the scheduler deliberately holds no
// opinion about what a cursor looks like.
var ErrIncomplete = errors.New("worker did not finish within the available budget")

// cronParser accepts standard five-field expressions plus the descriptors
// (@hourly, @daily). Seconds are deliberately not accepted: the tick is minutes
// wide, so an expression naming a second would describe a precision the runtime
// cannot deliver.
var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// parseCron validates an expression and reports the schedule it describes.
func parseCron(expr string) (cron.Schedule, error) {
	schedule, err := cronParser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return schedule, nil
}

// minimumGap reports the shortest interval between consecutive firings of a
// schedule, sampled forward from a fixed point.
//
// Sampled rather than derived because the fields interact: "*/7 * * * *" fires
// at :00, :07 … :56 and then again at :00, a four-minute gap that no single
// field states. Sampling over an hour observes the wrap; reading the step value
// alone does not, which is how an expression that looks safe turns out not to be.
func minimumGap(schedule cron.Schedule, from time.Time) time.Duration {
	smallest := time.Duration(1<<63 - 1)
	previous := schedule.Next(from.UTC())
	if previous.IsZero() {
		return smallest
	}

	// One hour of samples. Long enough to see a sub-hour step wrap, short enough
	// to stay cheap at registration, and schedules coarser than an hour cannot
	// be too fine for a tick measured in minutes.
	deadline := previous.Add(time.Hour)
	for {
		next := schedule.Next(previous)
		if next.IsZero() || next.After(deadline) {
			return smallest
		}
		if gap := next.Sub(previous); gap < smallest {
			smallest = gap
		}
		previous = next
	}
}

// validateAgainstTick rejects a schedule the runtime cannot honour.
//
// An expression that fires more often than the application is woken does not
// fire more often — it fires once per tick and silently drops the rest. That is
// the failure this exists to prevent: the job appears registered, appears
// scheduled, and quietly runs at a fraction of its stated rate.
func validateAgainstTick(name, expr string, schedule cron.Schedule, tick time.Duration) error {
	if gap := minimumGap(schedule, time.Now()); gap < tick {
		return fmt.Errorf(
			"worker %q asks to run every %v (%q) but this runtime checks for due work every %v; "+
				"it would silently run at most once per %v — widen the expression or lower %s",
			name, gap, expr, tick, tick, TickIntervalEnv,
		)
	}
	return nil
}

// dueIn reports whether a task should run, given that the previous check
// happened one window ago.
//
// The question is deliberately about a window rather than about now. A daemon
// ticking every minute can ask whether an expression matches the current minute;
// a function woken every five cannot, because an expression naming 02:03 never
// coincides with a wake at 02:00 or 02:05 and would never run at all. Asking
// whether the expression had any firing in the window that just elapsed makes a
// task at most one tick late and never skipped.
func (t *ScheduledTask) dueIn(window time.Duration, now time.Time) bool {
	if t.schedule == nil {
		// An interval says nothing about when to run without a record of the last
		// run, and this runtime keeps none. Never due here; unschedulableHere
		// explains why, once, where someone will read it.
		return false
	}
	// Evaluated in UTC, deliberately. robfig leaves a schedule with the default
	// location bound to the timezone of the time it is handed, so passing the
	// process clock would make an expression mean one thing on a laptop in +0530
	// and another on a function running in UTC — the same job, five and a half
	// hours apart, with nothing in the expression to say so. Handing it UTC
	// pins it. An expression that names its own zone (TZ=Europe/Berlin 0 2 * * *)
	// has a location set, which robfig honours over the input, so this does not
	// take that choice away from anyone who made it explicitly.
	utc := now.UTC()
	next := t.schedule.Next(utc.Add(-window))
	return !next.IsZero() && !next.After(utc)
}

// scheduleDescription states how a task is scheduled, in the form its author
// wrote it. Carried on every run span so a run can be read without also having
// to look up how the worker was registered.
func (t *ScheduledTask) scheduleDescription() string {
	if expr := strings.TrimSpace(t.options.CronExpr); expr != "" {
		return expr
	}
	if t.options.Interval > 0 {
		return "every " + t.options.Interval.String()
	}
	return "unscheduled"
}

// TickRuntime reports whether this process is woken from outside rather than
// running continuously. The distinction decides which schedules can be honoured:
// a process with a ticker of its own can measure an interval, one that is frozen
// between invocations cannot measure anything, because it does not exist between
// them.
func TickRuntime() bool {
	return os.Getenv("AWS_LAMBDA_RUNTIME_API") != ""
}

// unschedulableHere explains why a task cannot run on this runtime, or is empty.
//
// Recorded on the task rather than refused at registration, so the worker still
// appears in the manifest and the console can report that it exists and will
// never run. Dropped silently it would look identical to a worker nobody wrote.
func unschedulableHere(t *ScheduledTask) string {
	if t.schedule != nil || !TickRuntime() {
		return ""
	}
	return fmt.Sprintf(
		"worker %q is scheduled by interval (%v), which cannot be honoured on a runtime that freezes "+
			"between invocations: no process survives to measure the interval from. Give it a Cron() "+
			"expression, which depends only on the clock",
		t.options.Name, t.options.Interval)
}

// tickFromEnv reads the wake interval this runtime was configured with.
func tickFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(TickIntervalEnv))
	if raw == "" {
		return DefaultTickInterval
	}
	tick, err := time.ParseDuration(raw)
	if err != nil || tick <= 0 {
		return DefaultTickInterval
	}
	return tick
}

// budgetReserve is held back from the invocation's remaining time so that the
// runtime has room to answer and to drain telemetry after the last worker ends.
// Without it a worker can consume the whole budget and the run that recorded it
// never gets written — losing exactly the evidence of the run that went wrong.
const budgetReserve = 3 * time.Second

// startable reports whether there is enough of the invocation left to begin
// another worker. False on a context with no deadline is wrong in the other
// direction, so an absent deadline — a long-running server, a test — means yes.
func startable(ctx context.Context) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(deadline) > budgetReserve
}

// Outcomes recorded for a run. Distinct from an error string because the console
// groups by them and a counter is labelled with them.
const (
	OutcomeOK         = "ok"
	OutcomeFailed     = "failed"
	OutcomeIncomplete = "incomplete"
)

func outcomeOf(err error) string {
	switch {
	case err == nil:
		return OutcomeOK
	case errors.Is(err, ErrIncomplete):
		return OutcomeIncomplete
	default:
		return OutcomeFailed
	}
}

// ExecuteDueWorkers runs the workers that are due, and only those.
//
// This is what a tick calls. It replaces running everything on every wake, which
// turns each worker's schedule into the tick's schedule — an hourly job firing
// twelve times an hour, and a daily one two hundred and eighty-eight times.
//
// Workers run in series in name order. Deterministic rather than map order,
// because which workers a spent budget leaves unrun should be reproducible: a
// tick that behaves differently on each invocation cannot be reasoned about from
// its logs.
//
// The two forms differ in what an unrun worker costs. An interval worker was
// never claimed, so it is simply due again. A cron firing is lost: the next tick
// asks about the window that just elapsed, and a firing from the window before
// it is not in that window. It will not run until its expression comes round
// again. That is why running out of budget is logged as loudly as a failure,
// and why fanning each due worker out into its own invocation — where the budget
// is per worker and can never be shared — is the real answer rather than a
// refinement.
func (s *Scheduler) ExecuteDueWorkers(ctx context.Context) map[string]error {
	s.mu.Lock()
	tick := s.tick
	tasks := make([]*ScheduledTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	s.mu.Unlock()

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].options.Name < tasks[j].options.Name
	})

	s.reportUnschedulable(tasks)

	now := time.Now()
	results := make(map[string]error)

	for i, t := range tasks {
		if !startable(ctx) {
			s.reportAbandoned(tasks[i:], tick, now)
			break
		}
		if !t.dueIn(tick, now) {
			continue
		}
		results[t.options.Name] = s.executeTaskSafe(ctx, t)
	}

	return results
}

// reportUnschedulable names, once per process, any worker this runtime cannot
// run at all.
//
// Said once rather than every tick because it does not change, and said at all
// because the alternative is a worker that was registered, appears in the
// manifest, and never runs — with nothing anywhere explaining why.
func (s *Scheduler) reportUnschedulable(tasks []*ScheduledTask) {
	s.unschedulableWarn.Do(func() {
		for _, t := range tasks {
			if reason := unschedulableHere(t); reason != "" {
				s.logger.Error("[Scheduler] " + reason)
			}
		}
	})
}

// reportAbandoned says what a spent budget cost, naming the workers rather than
// counting them.
//
// A count tells an operator that something was dropped; a name tells them which
// report did not get sent. The two are separated because they are not equally
// recoverable — an interval worker is due again in minutes, while a cron firing
// is gone until the expression next comes round.
func (s *Scheduler) reportAbandoned(remaining []*ScheduledTask, tick time.Duration, now time.Time) {
	var lost, deferred []string
	for _, t := range remaining {
		if t.schedule == nil {
			deferred = append(deferred, t.options.Name)
			continue
		}
		if t.dueIn(tick, now) {
			lost = append(lost, t.options.Name)
		}
	}

	if len(lost) > 0 {
		s.logger.Error(fmt.Sprintf(
			"[Scheduler] the budget ran out before %v; these firings are lost and will not be retried "+
				"until their expressions come round again", lost))
	}
	if len(deferred) > 0 {
		s.logger.Warn(fmt.Sprintf(
			"[Scheduler] the budget ran out before %v; they remain due and will be reconsidered next tick", deferred))
	}
}
