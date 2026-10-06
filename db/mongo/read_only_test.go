package mongo

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

// TestReadOnlyMonitorAgainstServer proves the guard stops real writes. It
// needs a disposable server: GINBOOT_TEST_MONGO_URI=mongodb://127.0.0.1:27017
func TestReadOnlyMonitorAgainstServer(t *testing.T) {
	uri := os.Getenv("GINBOOT_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("GINBOOT_TEST_MONGO_URI not set")
	}
	ctx := context.Background()
	writer, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	defer writer.Disconnect(ctx)
	coll := writer.Database("ginboot_readonly_test").Collection("docs")
	require.NoError(t, coll.Drop(ctx))
	_, err = coll.InsertOne(ctx, bson.D{{Key: "_id", Value: "seed"}})
	require.NoError(t, err)
	defer coll.Database().Drop(ctx)

	guarded, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetMonitor(ReadOnlyMonitor(nil)))
	require.NoError(t, err)
	defer guarded.Disconnect(ctx)
	g := guarded.Database("ginboot_readonly_test").Collection("docs")

	n, err := g.CountDocuments(ctx, bson.D{})
	require.NoError(t, err, "reads still work")
	assert.Equal(t, int64(1), n)

	writes := map[string]func(){
		"insert":        func() { _, _ = g.InsertOne(ctx, bson.D{{Key: "_id", Value: "x"}}) },
		"update":        func() { _, _ = g.UpdateOne(ctx, bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "a", Value: 1}}}}) },
		"delete":        func() { _, _ = g.DeleteMany(ctx, bson.D{}) },
		"findAndModify": func() { g.FindOneAndUpdate(ctx, bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 1}}}}) },
		"createIndexes": func() { _, _ = g.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}}) },
		"drop":          func() { _ = g.Drop(ctx) },
		"$out":          func() { _, _ = g.Aggregate(ctx, mongo.Pipeline{{{Key: "$out", Value: "copy"}}}) },
	}
	for name, write := range writes {
		assert.Panics(t, write, name)
	}

	var doc bson.M
	require.NoError(t, coll.FindOne(ctx, bson.D{}).Decode(&doc))
	assert.Equal(t, bson.M{"_id": "seed"}, doc, "the collection is unchanged")
	n, _ = coll.CountDocuments(ctx, bson.D{})
	assert.Equal(t, int64(1), n)
	idx, _ := coll.Indexes().List(ctx)
	var indexes []bson.M
	_ = idx.All(ctx, &indexes)
	assert.Len(t, indexes, 1, "only the _id index exists")
	names, _ := writer.Database("ginboot_readonly_test").ListCollectionNames(ctx, bson.D{})
	assert.Equal(t, []string{"docs"}, names, "$out created nothing")

	n, err = g.CountDocuments(ctx, bson.D{})
	require.NoError(t, err, "the guarded client still works after refusals")
	assert.Equal(t, int64(1), n)
}
