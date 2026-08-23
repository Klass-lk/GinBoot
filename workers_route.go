package ginboot

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// A running application is the only authority on what jobs it has.
//
// The schedules could be declared in configuration and read from the repository
// at build time, which would let a deployment know about a job before the code
// implementing it is running. That was rejected for the same reason
// registerOpenAPIEndpoint gives for specifications: it makes the declaration a
// second source of truth that can disagree with the code, and someone has to
// remember to keep both in step. Asking the process cannot disagree with itself.
//
// The cost is that a job is discoverable only after it deploys. The platform
// absorbs that by reading this endpoint once the stack has settled and adding
// the schedule then, inside the same deployment.
const (
	// WorkersPublic serves the manifest to anyone who asks. The default: it
	// names jobs and their schedules, which is what an operator running locally
	// wants and nothing an attacker could not infer from behaviour.
	WorkersPublic = "public"
	// WorkersToken serves it only to a caller presenting the shared secret.
	WorkersToken = "token"
	// WorkersDisabled does not register the route, so the path is a 404 like any
	// other and nothing advertises that a manifest exists.
	WorkersDisabled = "disabled"

	defaultWorkersPath = "/_ginboot/workers"

	// WorkersTokenHeader carries the shared secret. A dedicated header rather
	// than Authorization, so presenting it cannot be confused with — or logged
	// beside — an end user's own credentials.
	WorkersTokenHeader = "X-Ginboot-Workers-Token"

	workersAccessEnv = "GINBOOT_WORKERS_ACCESS"
	workersTokenEnv  = "GINBOOT_WORKERS_TOKEN"
	workersPathEnv   = "GINBOOT_WORKERS_PATH"
)

// WorkerDescription is one registered job, as the process sees it.
type WorkerDescription struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	// Schedulable is false when this runtime cannot honour the schedule at all.
	// Reported rather than omitted: a job that exists and can never run is a
	// different problem from a job that was never written, and only one of them
	// is fixed by writing code.
	Schedulable bool `json:"schedulable"`
	// Problem explains an unschedulable job, and is empty otherwise.
	Problem string `json:"problem,omitempty"`
}

// WorkersManifest is what the endpoint answers with.
type WorkersManifest struct {
	// Tick is how often this runtime looks for due work, which bounds the
	// resolution of every schedule below. Reported so that a reader can tell
	// whether the deployment agrees with the schedule that wakes it — a
	// disagreement there produces missed or doubled runs and is invisible from
	// either side alone.
	Tick    string              `json:"tick"`
	Runtime string              `json:"runtime"`
	Workers []WorkerDescription `json:"workers"`
}

// Manifest describes every registered job.
func (s *Scheduler) Manifest() WorkersManifest {
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

	runtime := "server"
	if TickRuntime() {
		runtime = "tick"
	}

	workers := make([]WorkerDescription, 0, len(tasks))
	for _, t := range tasks {
		problem := unschedulableHere(t)
		workers = append(workers, WorkerDescription{
			Name:        t.options.Name,
			Schedule:    t.scheduleDescription(),
			Schedulable: problem == "",
			Problem:     problem,
		})
	}

	return WorkersManifest{Tick: tick.String(), Runtime: runtime, Workers: workers}
}

// registerWorkersEndpoint mounts the manifest endpoint. Called from Start,
// beside the OpenAPI and health routes.
func (s *Server) registerWorkersEndpoint() {
	access := strings.ToLower(strings.TrimSpace(os.Getenv(workersAccessEnv)))
	if access == "" {
		access = WorkersPublic
	}

	if access == WorkersDisabled {
		return
	}

	token := os.Getenv(workersTokenEnv)

	// Fail closed. "token" with no token is a configuration someone started and
	// did not finish, and the safe reading is that they meant to restrict the
	// manifest — not that they meant to publish it.
	if access == WorkersToken && token == "" {
		fmt.Printf("[ginboot] %s is \"token\" but no token is set; not serving the worker manifest. Set %s.\n",
			workersAccessEnv, workersTokenEnv)
		return
	}

	if access != WorkersPublic && access != WorkersToken {
		fmt.Printf("[ginboot] %s %q is not one of %q, %q or %q; not serving the worker manifest\n",
			workersAccessEnv, access, WorkersPublic, WorkersToken, WorkersDisabled)
		return
	}

	path := strings.TrimSpace(os.Getenv(workersPathEnv))
	if path == "" {
		path = defaultWorkersPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	scheduler := s.scheduler
	s.engine.GET(path, func(c *gin.Context) {
		if access == WorkersToken {
			// Constant time, because a comparison that returns early leaks the
			// secret one byte at a time to anyone willing to measure.
			presented := c.GetHeader(WorkersTokenHeader)
			if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "the worker manifest requires a token",
				})
				return
			}
		}
		if scheduler == nil {
			c.JSON(http.StatusOK, WorkersManifest{Workers: []WorkerDescription{}})
			return
		}
		c.JSON(http.StatusOK, scheduler.Manifest())
	})
}
