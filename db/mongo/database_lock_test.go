package mongo

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func startedOn(t *testing.T, db, name string, cmd bson.D) *event.CommandStartedEvent {
	t.Helper()
	e := started(t, name, cmd)
	e.DatabaseName = db
	return e
}

func TestDatabaseLockMonitorRefusesOtherDatabases(t *testing.T) {
	m := DatabaseLockMonitor([]string{"app_test"}, nil)
	for _, e := range []*event.CommandStartedEvent{
		startedOn(t, "prod", "find", bson.D{{Key: "find", Value: "users"}}),
		startedOn(t, "prod", "insert", bson.D{{Key: "insert", Value: "users"}}),
		startedOn(t, "admin", "listDatabases", bson.D{{Key: "listDatabases", Value: 1}}),
		startedOn(t, "admin", "renameCollection", bson.D{{Key: "renameCollection", Value: "app_test.a"}, {Key: "to", Value: "prod.a"}}),
		startedOn(t, "app_test", "aggregate", bson.D{{Key: "aggregate", Value: "users"}, {Key: "pipeline", Value: bson.A{
			bson.D{{Key: "$out", Value: bson.D{{Key: "db", Value: "prod"}, {Key: "coll", Value: "users"}}}}}}}),
		startedOn(t, "app_test", "aggregate", bson.D{{Key: "aggregate", Value: "users"}, {Key: "pipeline", Value: bson.A{
			bson.D{{Key: "$merge", Value: bson.D{{Key: "into", Value: bson.D{{Key: "db", Value: "prod"}, {Key: "coll", Value: "x"}}}}}}}}}),
	} {
		assert.Panics(t, func() { m.Started(context.Background(), e) }, "%s on %s", e.CommandName, e.DatabaseName)
	}
}

func TestDatabaseLockMonitorAllowsItsDatabase(t *testing.T) {
	var seen []string
	next := &event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) { seen = append(seen, e.CommandName) }}
	m := DatabaseLockMonitor([]string{"app_test"}, next)
	for _, e := range []*event.CommandStartedEvent{
		startedOn(t, "app_test", "find", bson.D{{Key: "find", Value: "users"}}),
		startedOn(t, "app_test", "insert", bson.D{{Key: "insert", Value: "users"}}),
		startedOn(t, "app_test", "aggregate", bson.D{{Key: "aggregate", Value: "users"}, {Key: "pipeline", Value: bson.A{
			bson.D{{Key: "$out", Value: "copy"}}}}}),
		startedOn(t, "admin", "hello", bson.D{{Key: "hello", Value: 1}}),
		startedOn(t, "admin", "endSessions", bson.D{{Key: "endSessions", Value: bson.A{}}}),
	} {
		assert.NotPanics(t, func() { m.Started(context.Background(), e) }, "%s on %s", e.CommandName, e.DatabaseName)
	}
	assert.Equal(t, []string{"find", "insert", "aggregate", "hello", "endSessions"}, seen)
}

func TestDatabaseLockViolationMessage(t *testing.T) {
	v, refused := checkDatabaseLock(startedOn(t, "prod", "insert", bson.D{{Key: "insert", Value: "users"}}),
		map[string]bool{"app_test": true}, []string{"app_test"})
	assert.True(t, refused)
	assert.Equal(t, `mongo: database lock refused "insert" on database "prod" (allowed: app_test)`, v.Error())
}

// TestDatabaseLockMonitorAgainstServer proves a locked client works on its
// database and cannot touch another one on the same server.
func TestDatabaseLockMonitorAgainstServer(t *testing.T) {
	if testing.Short() {
		t.Skip("needs Docker")
	}
	container, cfg, err := setupTestContainer(t)
	require.NoError(t, err)
	defer container.Terminate(context.Background())
	ctx := context.Background()
	uri := fmt.Sprintf("mongodb://%s:%d", cfg.Host, cfg.Port)

	writer, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	defer writer.Disconnect(ctx)
	other := writer.Database("other").Collection("docs")
	_, err = other.InsertOne(ctx, bson.D{{Key: "_id", Value: "seed"}})
	require.NoError(t, err)

	locked, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetMonitor(DatabaseLockMonitor([]string{"allowed"}, nil)))
	require.NoError(t, err)
	defer locked.Disconnect(ctx)

	mine := locked.Database("allowed").Collection("docs")
	_, err = mine.InsertOne(ctx, bson.D{{Key: "_id", Value: "a"}})
	require.NoError(t, err, "writes to the allowed database work")
	n, err := mine.CountDocuments(ctx, bson.D{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	theirs := locked.Database("other").Collection("docs")
	for name, op := range map[string]func(){
		"find":   func() { _, _ = theirs.CountDocuments(ctx, bson.D{}) },
		"insert": func() { _, _ = theirs.InsertOne(ctx, bson.D{{Key: "_id", Value: "x"}}) },
		"delete": func() { _, _ = theirs.DeleteMany(ctx, bson.D{}) },
		"drop":   func() { _ = theirs.Drop(ctx) },
		"$out to other": func() {
			_, _ = mine.Aggregate(ctx, mongo.Pipeline{{{Key: "$out", Value: bson.D{{Key: "db", Value: "other"}, {Key: "coll", Value: "docs"}}}}})
		},
	} {
		assert.Panics(t, op, name)
	}
	var docs []bson.M
	cur, err := other.Find(ctx, bson.D{})
	require.NoError(t, err)
	require.NoError(t, cur.All(ctx, &docs))
	assert.Equal(t, []bson.M{{"_id": "seed"}}, docs, "the other database is untouched")
}
