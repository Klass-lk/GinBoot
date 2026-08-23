package ginboot

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// The behaviour under test is the one that makes scheduling work on a runtime
// that freezes: a task is due if its expression fired at any point in the window
// that just elapsed, not if it fires exactly now. Every test here is a statement
// about that window.

func mustSchedule(t *testing.T, expr string) *ScheduledTask {
	t.Helper()
	schedule, err := parseCron(expr)
	if err != nil {
		t.Fatalf("parsing %q: %v", expr, err)
	}
	return &ScheduledTask{options: TaskOptions{Name: "test"}, schedule: schedule}
}

func TestDueWithinWindowNotOnlyOnTheTick(t *testing.T) {
	// 02:03 daily, checked by a runtime that wakes on five-minute boundaries. The
	// expression never coincides with a wake, which is exactly the case that a
	// "does it match now?" test gets wrong and would never run.
	task := mustSchedule(t, "3 2 * * *")

	wake := time.Date(2026, 8, 23, 2, 5, 0, 0, time.UTC)
	if !task.dueIn(5*time.Minute, wake) {
		t.Fatal("a job at 02:03 must run on the 02:05 tick; it fired inside the window that just passed")
	}

	// And must not run again on the following tick, having already been claimed
	// by the window it belonged to.
	if task.dueIn(5*time.Minute, wake.Add(5*time.Minute)) {
		t.Fatal("the 02:10 tick re-ran a job whose only firing was at 02:03")
	}
}

func TestNotDueWhenNothingFiredInTheWindow(t *testing.T) {
	task := mustSchedule(t, "0 2 * * *")
	wake := time.Date(2026, 8, 23, 14, 0, 0, 0, time.UTC)

	if task.dueIn(5*time.Minute, wake) {
		t.Fatal("a daily 02:00 job ran at 14:00")
	}
}

func TestBoundaryFiringCountsOnce(t *testing.T) {
	// A firing landing exactly on the wake instant belongs to that wake — the
	// window is half-open at the start and closed at the end. Getting this
	// backwards either double-runs a job or drops it, depending on which side.
	task := mustSchedule(t, "0 * * * *")
	wake := time.Date(2026, 8, 23, 15, 0, 0, 0, time.UTC)

	if !task.dueIn(5*time.Minute, wake) {
		t.Fatal("a firing exactly at the wake instant must run on that wake")
	}
	if task.dueIn(5*time.Minute, wake.Add(5*time.Minute)) {
		t.Fatal("the same firing ran again on the next tick")
	}
}

func TestEveryFiringGetsExactlyOneTick(t *testing.T) {
	// Walk a day of five-minute ticks past an hourly job. Missing one is a job
	// that silently skipped; catching one twice is a job that double-ran. Both
	// are invisible in a single-tick test.
	task := mustSchedule(t, "17 * * * *")

	tick := 5 * time.Minute
	start := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	runs := 0
	for i := 1; i <= 24*12; i++ {
		if task.dueIn(tick, start.Add(time.Duration(i)*tick)) {
			runs++
		}
	}

	if runs != 24 {
		t.Fatalf("an hourly job over a day of ticks ran %d times, want 24", runs)
	}
}

func TestExpressionFinerThanTheTickIsRefused(t *testing.T) {
	// The failure this prevents: a job registered, listed, and apparently
	// scheduled, quietly running at a twelfth of its stated rate.
	schedule, err := parseCron("* * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	err = validateAgainstTick("hot-loop", "* * * * *", schedule, 5*time.Minute)
	if err == nil {
		t.Fatal("a once-a-minute expression was accepted by a five-minute runtime")
	}
	if !contains(err.Error(), TickIntervalEnv) {
		t.Errorf("the refusal should name %s so the reader can act on it; got: %v", TickIntervalEnv, err)
	}
}

func TestStepExpressionWrapIsCaught(t *testing.T) {
	// "*/7" reads as seven-minute spacing but wraps from :56 to :00 — four
	// minutes. Reading the step value alone accepts it against a five-minute
	// tick; sampling the schedule catches the wrap.
	schedule, err := parseCron("*/7 * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if err := validateAgainstTick("wrapper", "*/7 * * * *", schedule, 5*time.Minute); err == nil {
		t.Fatal("*/7 was accepted against a 5m tick, but its wrap gap is 4m")
	}
}

func TestCoarseExpressionsAreAccepted(t *testing.T) {
	for _, expr := range []string{"0 2 * * *", "*/15 * * * *", "@hourly", "0 0 * * 0"} {
		schedule, err := parseCron(expr)
		if err != nil {
			t.Fatalf("parsing %q: %v", expr, err)
		}
		if err := validateAgainstTick("fine", expr, schedule, 5*time.Minute); err != nil {
			t.Errorf("%q should be fine on a 5m tick: %v", expr, err)
		}
	}
}

func TestSecondsAreNotAccepted(t *testing.T) {
	// Six fields would describe a precision the tick cannot deliver. Refusing to
	// parse is better than accepting and rounding, which would silently move
	// when a job runs.
	if _, err := parseCron("0 */5 * * * *"); err == nil {
		t.Fatal("a six-field expression with seconds was accepted")
	}
}

// --- scheduling forms ----------------------------------------------------

// alwaysDue is an expression whose firing always falls inside a five-minute
// window, so a test can assert on what ran without pinning the clock.
const alwaysDue = "*/5 * * * *"

func TestIntervalWorkersDoNotRunOnATickRuntime(t *testing.T) {
	// The alternative — running them on every tick because nothing says
	// otherwise — turns "every six hours" into "every five minutes".
	t.Setenv("AWS_LAMBDA_RUNTIME_API", "127.0.0.1:9001")

	scheduler := NewScheduler(nil)
	ran := false
	scheduler.RegisterTask(TaskOptions{Name: "hourly-thing", Interval: time.Hour}, func(context.Context) error {
		ran = true
		return nil
	})

	scheduler.ExecuteDueWorkers(context.Background())

	if ran {
		t.Fatal("an interval worker ran on a runtime that cannot measure an interval")
	}
}

func TestIntervalWorkerIsReportedAsUnschedulable(t *testing.T) {
	// Not running is correct; not running silently is not. A registered worker
	// that never fires with nothing explaining why is the failure this feature
	// exists to remove, so it must not be reintroduced by the fix.
	t.Setenv("AWS_LAMBDA_RUNTIME_API", "127.0.0.1:9001")

	task := &ScheduledTask{options: TaskOptions{Name: "orphan", Interval: time.Hour}}
	reason := unschedulableHere(task)

	if reason == "" {
		t.Fatal("an interval worker on a tick runtime must explain itself")
	}
	if !contains(reason, "orphan") || !contains(reason, "Cron()") {
		t.Errorf("the explanation should name the worker and the fix; got: %s", reason)
	}
}

func TestIntervalWorkersStillRunOnAServer(t *testing.T) {
	// A long-running process has a ticker of its own, so nothing here restricts
	// it. Only the frozen runtime cannot honour an interval.
	task := &ScheduledTask{options: TaskOptions{Name: "fine-here", Interval: time.Hour}}

	if reason := unschedulableHere(task); reason != "" {
		t.Fatalf("interval workers are valid on a server; got: %s", reason)
	}
}

func TestCronWorkerRuns(t *testing.T) {
	scheduler := NewScheduler(nil)
	ran := 0
	scheduler.RegisterTask(TaskOptions{Name: "due-now", CronExpr: alwaysDue}, func(context.Context) error {
		ran++
		return nil
	})

	scheduler.ExecuteDueWorkers(context.Background())

	if ran != 1 {
		t.Fatalf("a due cron worker ran %d times, want 1", ran)
	}
}

func TestOutcomeDistinguishesIncompleteFromFailed(t *testing.T) {
	scheduler := NewScheduler(nil)
	scheduler.RegisterTask(TaskOptions{Name: "big-job", CronExpr: alwaysDue}, func(context.Context) error {
		return fmt.Errorf("processed 10000 rows: %w", ErrIncomplete)
	})

	results := scheduler.ExecuteDueWorkers(context.Background())

	if outcomeOf(results["big-job"]) != OutcomeIncomplete {
		t.Fatalf("a job that ran out of budget is unfinished, not failed; got %q", outcomeOf(results["big-job"]))
	}
}

func TestIncompleteSurvivesWrapping(t *testing.T) {
	// Workers will wrap this with their own context, and an outcome that only
	// survives a bare return is one that will be reported wrong in practice.
	wrapped := fmt.Errorf("page 3 of 40: %w", ErrIncomplete)
	if !errors.Is(wrapped, ErrIncomplete) {
		t.Fatal("ErrIncomplete must survive wrapping")
	}
	if outcomeOf(wrapped) != OutcomeIncomplete {
		t.Fatalf("outcome of a wrapped ErrIncomplete = %q", outcomeOf(wrapped))
	}
}

// --- budget ---------------------------------------------------------------

func TestWorkersRunInNameOrder(t *testing.T) {
	// Map order would make which workers a spent budget starves differ on every
	// invocation, so a tick could not be reasoned about from its own logs.
	scheduler := NewScheduler(nil)

	var order []string
	var mu sync.Mutex
	for _, name := range []string{"z-last", "a-first", "m-middle"} {
		scheduler.RegisterTask(TaskOptions{Name: name, CronExpr: alwaysDue}, func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, name)
			return nil
		})
	}

	scheduler.ExecuteDueWorkers(context.Background())

	want := []string{"a-first", "m-middle", "z-last"}
	for i := range want {
		if i >= len(order) || order[i] != want[i] {
			t.Fatalf("workers ran in %v, want %v", order, want)
		}
	}
}

func TestTickStopsBeforeTheBudgetRunsOut(t *testing.T) {
	scheduler := NewScheduler(nil)

	// Name order puts the hog first, so it consumes the budget deterministically.
	// The second must not be started: starting it would record a run that failed
	// on a timeout, rather than leaving it for the next tick.
	scheduler.RegisterTask(TaskOptions{Name: "a-hog", CronExpr: alwaysDue}, func(ctx context.Context) error {
		deadline, _ := ctx.Deadline()
		time.Sleep(time.Until(deadline) - budgetReserve + 50*time.Millisecond)
		return nil
	})
	scheduler.RegisterTask(TaskOptions{Name: "b-starved", CronExpr: alwaysDue}, func(context.Context) error {
		t.Error("a worker was started with no budget left for it")
		return nil
	})

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(budgetReserve+300*time.Millisecond))
	defer cancel()

	scheduler.ExecuteDueWorkers(ctx)
}

func TestASkippedCronFiringIsLostNotDeferred(t *testing.T) {
	// Documents the sharp edge rather than asserting it is fine. A firing starved
	// by a spent budget is gone, because the next tick asks about the next window
	// and the firing belonged to the previous one.
	task := mustSchedule(t, "3 2 * * *")
	tick := 5 * time.Minute
	missed := time.Date(2026, 8, 23, 2, 5, 0, 0, time.UTC)

	if !task.dueIn(tick, missed) {
		t.Fatal("precondition: the firing should be due on this tick")
	}
	if task.dueIn(tick, missed.Add(tick)) {
		t.Fatal("if this ever passes, skipped firings are being retried and " +
			"reportAbandoned should stop calling them lost")
	}
}

func TestNoDeadlineMeansNoBudgetLimit(t *testing.T) {
	// A long-running server and a test both have no deadline. Reading that as
	// "no time left" would stop the scheduler from ever running anything there.
	if !startable(context.Background()) {
		t.Fatal("a context without a deadline must not read as an exhausted budget")
	}
}

// --- registration ---------------------------------------------------------

func TestRefusedExpressionDoesNotRegister(t *testing.T) {
	scheduler := NewScheduler(nil)
	scheduler.RegisterTask(TaskOptions{Name: "nope", CronExpr: "not a cron"}, func(context.Context) error {
		return nil
	})

	if _, exists := scheduler.GetTasks()["nope"]; exists {
		t.Fatal("a worker with an unparseable expression was registered")
	}
}

func TestCronRegistrationParsesOnce(t *testing.T) {
	scheduler := NewScheduler(nil)
	scheduler.RegisterTask(TaskOptions{Name: "nightly", CronExpr: "0 2 * * *"}, func(context.Context) error {
		return nil
	})

	task, exists := scheduler.GetTasks()["nightly"]
	if !exists {
		t.Fatal("a valid expression was not registered")
	}
	if task.schedule == nil {
		t.Fatal("the expression should be parsed at registration, not on first use")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 ||
		indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestSchedulesAreEvaluatedInUTC(t *testing.T) {
	// robfig binds a default-location schedule to the timezone of the time it is
	// handed. Left alone, that makes an expression mean the process's local time:
	// 02:00 on a laptop in +0530 and 02:00 UTC once deployed, five and a half
	// hours apart, with nothing in the expression to say which was meant.
	task := mustSchedule(t, "0 2 * * *")
	colombo := time.FixedZone("+0530", 5*3600+1800)

	// 07:30 in +0530 is 02:00 UTC. The job belongs here.
	atUTCTwo := time.Date(2026, 8, 23, 7, 30, 0, 0, colombo)
	if !task.dueIn(5*time.Minute, atUTCTwo) {
		t.Error("a daily 02:00 job must fire at 02:00 UTC however the caller's clock is zoned")
	}

	// 02:00 in +0530 is 20:30 UTC the day before. The job does not belong here.
	atLocalTwo := time.Date(2026, 8, 23, 2, 0, 0, 0, colombo)
	if task.dueIn(5*time.Minute, atLocalTwo) {
		t.Error("the schedule followed the caller's timezone; it must be pinned to UTC")
	}
}

func TestAnExplicitTimezoneIsHonoured(t *testing.T) {
	// Pinning to UTC must not take the choice away from someone who stated a zone.
	task := mustSchedule(t, "TZ=Asia/Colombo 0 2 * * *")

	// 02:00 in Colombo is 20:30 UTC the previous day.
	if !task.dueIn(5*time.Minute, time.Date(2026, 8, 22, 20, 30, 0, 0, time.UTC)) {
		t.Error("an expression naming its own zone should fire at that zone's 02:00")
	}
	if task.dueIn(5*time.Minute, time.Date(2026, 8, 23, 2, 0, 0, 0, time.UTC)) {
		t.Error("a TZ= expression fired at 02:00 UTC instead of 02:00 in its stated zone")
	}
}
