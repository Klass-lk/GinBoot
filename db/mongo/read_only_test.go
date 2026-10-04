package mongo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
)

func started(t *testing.T, name string, cmd bson.D) *event.CommandStartedEvent {
	t.Helper()
	raw, err := bson.Marshal(cmd)
	require.NoError(t, err)
	return &event.CommandStartedEvent{CommandName: name, DatabaseName: "app", Command: raw}
}

func TestReadOnlyMonitorRefusesWrites(t *testing.T) {
	m := ReadOnlyMonitor(nil)
	for _, tc := range []struct {
		name string
		cmd  bson.D
	}{
		{"insert", bson.D{{Key: "insert", Value: "users"}}},
		{"update", bson.D{{Key: "update", Value: "users"}}},
		{"delete", bson.D{{Key: "delete", Value: "users"}}},
		{"findAndModify", bson.D{{Key: "findAndModify", Value: "users"}}},
		{"createIndexes", bson.D{{Key: "createIndexes", Value: "users"}}},
		{"drop", bson.D{{Key: "drop", Value: "users"}}},
		{"dropDatabase", bson.D{{Key: "dropDatabase", Value: 1}}},
		{"renameCollection", bson.D{{Key: "renameCollection", Value: "app.a"}}},
		{"aggregate", bson.D{{Key: "aggregate", Value: "users"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{}}}, bson.D{{Key: "$out", Value: "copy"}}}}}},
		{"aggregate", bson.D{{Key: "aggregate", Value: "users"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$merge", Value: bson.D{{Key: "into", Value: "x"}}}}}}}},
	} {
		e := started(t, tc.name, tc.cmd)
		assert.PanicsWithValue(t, func() ReadOnlyViolation {
			v, _ := checkReadOnly(e)
			return v
		}(), func() { m.Started(context.Background(), e) }, tc.name)
	}
}

func TestReadOnlyMonitorAllowsReads(t *testing.T) {
	var seen []string
	next := &event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) { seen = append(seen, e.CommandName) }}
	m := ReadOnlyMonitor(next)
	for _, tc := range []struct {
		name string
		cmd  bson.D
	}{
		{"find", bson.D{{Key: "find", Value: "users"}}},
		{"aggregate", bson.D{{Key: "aggregate", Value: "users"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{}}}}}}},
		{"count", bson.D{{Key: "count", Value: "users"}}},
		{"distinct", bson.D{{Key: "distinct", Value: "users"}}},
		{"getMore", bson.D{{Key: "getMore", Value: int64(1)}}},
		{"hello", bson.D{{Key: "hello", Value: 1}}},
		{"endSessions", bson.D{{Key: "endSessions", Value: bson.A{}}}},
	} {
		assert.NotPanics(t, func() { m.Started(context.Background(), started(t, tc.name, tc.cmd)) }, tc.name)
	}
	assert.Equal(t, []string{"find", "aggregate", "count", "distinct", "getMore", "hello", "endSessions"}, seen, "reads reach the wrapped monitor")
}

func TestReadOnlyViolationMessage(t *testing.T) {
	v, refused := checkReadOnly(started(t, "insert", bson.D{{Key: "insert", Value: "users"}}))
	assert.True(t, refused)
	assert.Equal(t, `mongo: read-only guard refused "insert" on app.users`, v.Error())
}
