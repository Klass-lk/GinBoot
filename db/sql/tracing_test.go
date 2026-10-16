package sql

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type tracedNote struct {
	ID   string `gorm:"primaryKey"`
	Body string
}

func TestTracingPluginRecordsStatementSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	db, err := gorm.Open(sqlite.Open("file:traced?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Use(TracingPlugin{tracer: tp.Tracer("test")}))
	require.NoError(t, db.AutoMigrate(&tracedNote{}))
	recorder = tracetest.NewSpanRecorder() // drop the migration's statements
	tp.RegisterSpanProcessor(recorder)

	ctx, parent := tp.Tracer("test").Start(context.Background(), "request")
	tx := db.WithContext(ctx)
	require.NoError(t, tx.Create(&tracedNote{ID: "n1", Body: "secret@example.com"}).Error)
	err = tx.Create(&tracedNote{ID: "n1", Body: "secret@example.com"}).Error
	require.Error(t, err)
	var found tracedNote
	require.ErrorIs(t, tx.First(&found, "id = ?", "missing").Error, gorm.ErrRecordNotFound)
	parent.End()

	spans := recorder.Ended()
	require.Len(t, spans, 4) // insert, failed insert, select, request
	assert.Equal(t, "insert traced_notes", spans[0].Name())
	assert.Equal(t, parent.SpanContext().SpanID(), spans[0].Parent().SpanID())
	assert.Contains(t, spans[0].Attributes(), attribute.String("db.system", "sqlite"))
	assert.Contains(t, spans[0].Attributes(), attribute.Int64("db.response.returned_rows", 1))
	assert.Equal(t, codes.Error, spans[1].Status().Code)
	assert.NotContains(t, spans[1].Status().Description, "secret", "driver messages can quote values")
	assert.Equal(t, "select traced_notes", spans[2].Name())
	assert.Equal(t, codes.Unset, spans[2].Status().Code, "not found is not a failure")
	for _, s := range spans {
		for _, a := range s.Attributes() {
			assert.NotContains(t, a.Value.Emit(), "secret", "no statement text or arguments by default")
		}
	}
}

func TestTracingPluginQueryTextIsParameterised(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	db, err := gorm.Open(sqlite.Open("file:traced2?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Use(TracingPlugin{QueryText: true, tracer: tp.Tracer("test")}))
	require.NoError(t, db.AutoMigrate(&tracedNote{}))
	var found []tracedNote
	require.NoError(t, db.WithContext(context.Background()).Where("body = ?", "secret@example.com").Find(&found).Error)
	last := recorder.Ended()[len(recorder.Ended())-1]
	var text string
	for _, a := range last.Attributes() {
		if a.Key == "db.query.text" {
			text = a.Value.AsString()
		}
	}
	assert.Contains(t, text, "body = ?")
	assert.NotContains(t, text, "secret")
}
