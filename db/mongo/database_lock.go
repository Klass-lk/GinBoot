package mongo

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
)

// DatabaseLockViolation is the panic value raised when a locked client tries
// to run a command against a database it is not allowed to use.
type DatabaseLockViolation struct {
	Command  string
	Database string
	Allowed  []string
}

func (v DatabaseLockViolation) Error() string {
	return fmt.Sprintf("mongo: database lock refused %q on database %q (allowed: %s)",
		v.Command, v.Database, strings.Join(v.Allowed, ", "))
}

// adminCommands are the commands a driver runs against the admin database on
// its own: handshakes, liveness checks, cursor and session housekeeping and
// transaction outcomes. They touch no user data, so a locked client may still
// send them there.
var adminCommands = map[string]bool{
	"hello": true, "ismaster": true, "ping": true, "buildinfo": true, "getparameter": true,
	"endsessions": true, "killcursors": true, "committransaction": true, "aborttransaction": true,
	"saslstart": true, "saslcontinue": true, "authenticate": true, "getnonce": true, "logout": true,
}

// DatabaseLockMonitor returns a command monitor that refuses every command
// addressed to a database outside allowed — reads included — and every
// aggregation whose $out or $merge stage targets another database.
//
// It exists for credentials that can reach more than the database a service
// may use, such as a test database on a production cluster:
//
//	opts := options.Client().ApplyURI(uri).SetMonitor(mongo.DatabaseLockMonitor([]string{"app_test"}, nil))
//
// As with ReadOnlyMonitor, the driver calls the monitor immediately before it
// writes the command to the connection, so a refused command is never sent;
// the refusal is a panic with a DatabaseLockViolation. Monitors chain:
// DatabaseLockMonitor(dbs, ReadOnlyMonitor(nil)) applies both. next, if not
// nil, still receives every event.
func DatabaseLockMonitor(allowed []string, next *event.CommandMonitor) *event.CommandMonitor {
	permitted := map[string]bool{}
	for _, db := range allowed {
		permitted[db] = true
	}
	m := &event.CommandMonitor{
		Started: func(ctx context.Context, e *event.CommandStartedEvent) {
			if v, refused := checkDatabaseLock(e, permitted, allowed); refused {
				panic(v)
			}
			if next != nil && next.Started != nil {
				next.Started(ctx, e)
			}
		},
	}
	if next != nil {
		m.Succeeded, m.Failed = next.Succeeded, next.Failed
	}
	return m
}

func checkDatabaseLock(e *event.CommandStartedEvent, permitted map[string]bool, allowed []string) (DatabaseLockViolation, bool) {
	v := DatabaseLockViolation{Command: e.CommandName, Database: e.DatabaseName, Allowed: allowed}
	if !permitted[e.DatabaseName] {
		if e.DatabaseName == "admin" && adminCommands[strings.ToLower(e.CommandName)] {
			return v, false
		}
		return v, true
	}
	if strings.EqualFold(e.CommandName, "aggregate") {
		if db, ok := pipelineTarget(e.Command); ok && !permitted[db] {
			v.Database = db
			return v, true
		}
	}
	return v, false
}

// pipelineTarget returns the database a pipeline's $out or $merge writes to,
// when it names one other than the command's own.
func pipelineTarget(cmd bson.Raw) (string, bool) {
	stages, ok := cmd.Lookup("pipeline").ArrayOK()
	if !ok {
		return "", false
	}
	values, err := stages.Values()
	if err != nil {
		return "", false
	}
	for _, s := range values {
		doc, ok := s.DocumentOK()
		if !ok {
			continue
		}
		for _, stage := range []string{"$out", "$merge"} {
			target, err := doc.LookupErr(stage)
			if err != nil {
				continue
			}
			spec, ok := target.DocumentOK()
			if !ok {
				continue // a bare collection name stays in the command's database
			}
			into := spec
			if stage == "$merge" {
				if nested, ok := spec.Lookup("into").DocumentOK(); ok {
					into = nested
				} else {
					continue
				}
			}
			if db, ok := into.Lookup("db").StringValueOK(); ok {
				return db, true
			}
		}
	}
	return "", false
}
