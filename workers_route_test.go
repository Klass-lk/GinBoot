package ginboot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The manifest is what a deployment reads to decide whether an application gets
// any scheduling infrastructure at all. Its contract therefore matters as much
// as the scheduler's: an application that answers wrongly gets rules it cannot
// use, or none when it needed them.

func manifestRequest(t *testing.T, srv *Server, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, defaultWorkersPath, nil)
	if token != "" {
		req.Header.Set(WorkersTokenHeader, token)
	}
	rec := httptest.NewRecorder()
	srv.Engine().ServeHTTP(rec, req)
	return rec
}

func TestManifestListsRegisteredWorkers(t *testing.T) {
	srv := New()
	srv.scheduler.RegisterTask(TaskOptions{Name: "nightly", CronExpr: "0 2 * * *"}, noopTask)
	srv.scheduler.RegisterTask(TaskOptions{Name: "quarter-hourly", CronExpr: "*/15 * * * *"}, noopTask)
	srv.registerWorkersEndpoint()

	rec := manifestRequest(t, srv, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("manifest answered %d", rec.Code)
	}

	var manifest WorkersManifest
	if err := json.Unmarshal(rec.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decoding manifest: %v", err)
	}

	if len(manifest.Workers) != 2 {
		t.Fatalf("manifest listed %d workers, want 2: %+v", len(manifest.Workers), manifest.Workers)
	}
	// Sorted, so a deployment comparing two reads sees a change only when
	// something changed.
	if manifest.Workers[0].Name != "nightly" || manifest.Workers[1].Name != "quarter-hourly" {
		t.Errorf("workers should be listed in name order; got %+v", manifest.Workers)
	}
	if manifest.Workers[0].Schedule != "0 2 * * *" {
		t.Errorf("the schedule should be reported as written; got %q", manifest.Workers[0].Schedule)
	}
	if !manifest.Workers[0].Schedulable {
		t.Errorf("a cron worker on a server is schedulable; got %+v", manifest.Workers[0])
	}
}

func TestManifestReportsTheTick(t *testing.T) {
	// The platform sets the rule's rate and this value from one number. Reporting
	// it is what lets a disagreement be spotted at all: from either side alone a
	// mismatch looks like jobs that occasionally double-run or vanish.
	t.Setenv(TickIntervalEnv, "10m")

	srv := New()
	srv.registerWorkersEndpoint()

	var manifest WorkersManifest
	if err := json.Unmarshal(manifestRequest(t, srv, "").Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if manifest.Tick != (10 * time.Minute).String() {
		t.Errorf("manifest tick = %q, want %q", manifest.Tick, (10 * time.Minute).String())
	}
}

func TestManifestFlagsUnschedulableWorkers(t *testing.T) {
	// An empty list and a list of jobs that can never run mean opposite things to
	// the deployment reading it, so the difference has to survive the wire.
	t.Setenv("AWS_LAMBDA_RUNTIME_API", "127.0.0.1:9001")

	srv := New()
	srv.scheduler.RegisterTask(TaskOptions{Name: "legacy", Interval: time.Hour}, noopTask)
	srv.registerWorkersEndpoint()

	var manifest WorkersManifest
	if err := json.Unmarshal(manifestRequest(t, srv, "").Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if len(manifest.Workers) != 1 {
		t.Fatalf("an unschedulable worker must still be listed; got %+v", manifest.Workers)
	}
	if manifest.Workers[0].Schedulable {
		t.Error("an interval worker on a tick runtime is not schedulable")
	}
	if manifest.Workers[0].Problem == "" {
		t.Error("an unschedulable worker must carry the reason")
	}
	if manifest.Runtime != "tick" {
		t.Errorf("runtime = %q, want \"tick\"", manifest.Runtime)
	}
}

func TestManifestTokenIsRequiredWhenConfigured(t *testing.T) {
	t.Setenv(workersAccessEnv, WorkersToken)
	t.Setenv(workersTokenEnv, "s3cret")

	srv := New()
	srv.scheduler.RegisterTask(TaskOptions{Name: "nightly", CronExpr: "0 2 * * *"}, noopTask)
	srv.registerWorkersEndpoint()

	if rec := manifestRequest(t, srv, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token should be rejected; got %d", rec.Code)
	}
	if rec := manifestRequest(t, srv, "wrong"); rec.Code != http.StatusUnauthorized {
		t.Errorf("a wrong token should be rejected; got %d", rec.Code)
	}
	if rec := manifestRequest(t, srv, "s3cret"); rec.Code != http.StatusOK {
		t.Errorf("the right token should be accepted; got %d", rec.Code)
	}
}

func TestTokenModeWithoutATokenServesNothing(t *testing.T) {
	// Fails closed. The alternative reading — publish it, since no secret was
	// set — turns an unfinished configuration into an open endpoint.
	t.Setenv(workersAccessEnv, WorkersToken)
	t.Setenv(workersTokenEnv, "")

	srv := New()
	srv.registerWorkersEndpoint()

	if rec := manifestRequest(t, srv, ""); rec.Code != http.StatusNotFound {
		t.Errorf("token mode with no token must not serve the manifest; got %d", rec.Code)
	}
}

func TestDisabledMeansTheRouteDoesNotExist(t *testing.T) {
	// 404 rather than 403, so nothing advertises that a manifest exists at all.
	t.Setenv(workersAccessEnv, WorkersDisabled)

	srv := New()
	srv.registerWorkersEndpoint()

	if rec := manifestRequest(t, srv, ""); rec.Code != http.StatusNotFound {
		t.Errorf("disabled should leave the path unrouted; got %d", rec.Code)
	}
}

func noopTask(_ context.Context) error { return nil }
