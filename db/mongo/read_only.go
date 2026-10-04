package mongo

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
)

// ReadOnlyViolation is the panic value raised when a guarded client tries to
// run a command that would change data or schema.
type ReadOnlyViolation struct {
	Command    string
	Database   string
	Collection string
}

func (v ReadOnlyViolation) Error() string {
	return fmt.Sprintf("mongo: read-only guard refused %q on %s.%s", v.Command, v.Database, v.Collection)
}

// writeCommands are the server commands that change data or schema.
var writeCommands = map[string]bool{
	"insert": true, "update": true, "delete": true, "findandmodify": true, "bulkwrite": true,
	"create": true, "createindexes": true, "drop": true, "dropdatabase": true, "dropindexes": true,
	"renamecollection": true, "collmod": true, "convertToCapped": true, "converttocapped": true,
	"mapreduce": true, "clonecollectionascapped": true, "createuser": true, "updateuser": true,
	"dropuser": true, "dropallusersfromdatabase": true, "createrole": true, "droprole": true,
	"applyops": true, "compact": true, "shardcollection": true, "createsearchindexes": true,
	"updatesearchindex": true, "dropsearchindex": true,
}

// ReadOnlyMonitor returns a command monitor that refuses every command that
// would change data or schema: inserts, updates, deletes, findAndModify,
// index and collection management, and aggregations ending in $out or $merge.
//
// It exists for pointing a service at a database it must not change — for
// instance comparing a new implementation against production data. Attach it
// when building the client:
//
//	opts := options.Client().ApplyURI(uri).SetMonitor(mongo.ReadOnlyMonitor(nil))
//
// The driver calls the monitor on the calling goroutine immediately before it
// writes the command to the connection, so a refused command is never sent.
// The guard refuses by panicking with a ReadOnlyViolation, the only way a
// monitor can stop a command; Ginboot's panic recovery turns that into a 500
// for the request that tried. next, if not nil, still receives every event.
func ReadOnlyMonitor(next *event.CommandMonitor) *event.CommandMonitor {
	m := &event.CommandMonitor{
		Started: func(ctx context.Context, e *event.CommandStartedEvent) {
			if v, refused := checkReadOnly(e); refused {
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

func checkReadOnly(e *event.CommandStartedEvent) (ReadOnlyViolation, bool) {
	name := strings.ToLower(e.CommandName)
	v := ReadOnlyViolation{Command: e.CommandName, Database: e.DatabaseName}
	if coll, ok := e.Command.Lookup(e.CommandName).StringValueOK(); ok {
		v.Collection = coll
	}
	if writeCommands[name] {
		return v, true
	}
	if name == "aggregate" && pipelineWrites(e.Command) {
		return v, true
	}
	return v, false
}

// pipelineWrites reports whether an aggregate command ends in $out or $merge.
func pipelineWrites(cmd bson.Raw) bool {
	stages, ok := cmd.Lookup("pipeline").ArrayOK()
	if !ok {
		return false
	}
	values, err := stages.Values()
	if err != nil {
		return true // unreadable pipeline: refuse rather than guess
	}
	for _, s := range values {
		doc, ok := s.DocumentOK()
		if !ok {
			continue
		}
		if _, err := doc.LookupErr("$out"); err == nil {
			return true
		}
		if _, err := doc.LookupErr("$merge"); err == nil {
			return true
		}
	}
	return false
}
