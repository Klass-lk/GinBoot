package sql

import (
	"errors"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const (
	tracerName = "github.com/klass-lk/ginboot/db/sql"
	spanKey    = "ginboot:tracing:span"
)

// TracingPlugin is a gorm plugin that records each statement as an
// OpenTelemetry client span, named "<operation> <table>" (e.g. "select
// users") and parented by the statement's context: pass the request's with
// db.WithContext(ctx). A span carries the database system (postgres,
// mysql, sqlite...), the table, the operation and the rows affected. A
// failed statement marks the span as an error with its SQLSTATE or gorm
// error, never the driver's message, which can quote row values
// (gorm.ErrRecordNotFound is not a failure).
//
// Statement text is recorded only with QueryText, and then only the
// parameterised SQL, never its arguments; raw SQL that inlines values would
// carry them, which is why it is opt-in.
//
//	db.Use(sql.TracingPlugin{})
//
// ConnectGORM installs it. Spans that are not recorded (telemetry off,
// sampled out) cost a context lookup and nothing more.
type TracingPlugin struct {
	QueryText bool
	tracer    trace.Tracer // the global provider's when nil
}

func (TracingPlugin) Name() string { return "ginboot:tracing" }

func (p TracingPlugin) Initialize(db *gorm.DB) error {
	tracer := p.tracer
	if tracer == nil {
		tracer = otel.GetTracerProvider().Tracer(tracerName)
	}
	system := db.Dialector.Name()
	cb := db.Callback()
	type registrar interface {
		Register(name string, fn func(*gorm.DB)) error
	}
	for _, proc := range []struct {
		op            string
		before, after registrar
	}{
		{"insert", cb.Create().Before("gorm:create"), cb.Create().After("gorm:create")},
		{"select", cb.Query().Before("gorm:query"), cb.Query().After("gorm:query")},
		{"update", cb.Update().Before("gorm:update"), cb.Update().After("gorm:update")},
		{"delete", cb.Delete().Before("gorm:delete"), cb.Delete().After("gorm:delete")},
		{"row", cb.Row().Before("gorm:row"), cb.Row().After("gorm:row")},
		{"raw", cb.Raw().Before("gorm:raw"), cb.Raw().After("gorm:raw")},
	} {
		op := proc.op
		if err := proc.before.Register("ginboot:tracing:before_"+op, func(tx *gorm.DB) { p.start(tracer, system, op, tx) }); err != nil {
			return err
		}
		if err := proc.after.Register("ginboot:tracing:after_"+op, p.end); err != nil {
			return err
		}
	}
	return nil
}

func (p TracingPlugin) start(tracer trace.Tracer, system, op string, tx *gorm.DB) {
	if tx.Statement == nil || tx.Statement.Context == nil {
		return
	}
	table := tx.Statement.Table
	name := op
	if table != "" {
		name += " " + table
	}
	_, span := tracer.Start(tx.Statement.Context, name, trace.WithSpanKind(trace.SpanKindClient))
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("db.system", system),
		attribute.String("db.collection.name", table),
		attribute.String("db.operation.name", strings.ToUpper(op)),
	)
	tx.InstanceSet(spanKey, span)
}

func (p TracingPlugin) end(tx *gorm.DB) {
	v, ok := tx.InstanceGet(spanKey)
	if !ok {
		return
	}
	span, ok := v.(trace.Span)
	if !ok {
		return
	}
	if p.QueryText && tx.Statement != nil {
		span.SetAttributes(attribute.String("db.query.text", tx.Statement.SQL.String()))
	}
	span.SetAttributes(attribute.Int64("db.response.returned_rows", tx.RowsAffected))
	if tx.Error != nil && !errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		span.SetStatus(codes.Error, failure(tx.Error))
	}
	span.End()
}

// failure names an error without its message: the driver's SQLSTATE
// (pgx, mysql and others expose one) or gorm's own error.
func failure(err error) string {
	var state interface{ SQLState() string }
	if errors.As(err, &state) {
		return "SQLSTATE " + state.SQLState()
	}
	for _, known := range []error{gorm.ErrDuplicatedKey, gorm.ErrForeignKeyViolated, gorm.ErrCheckConstraintViolated,
		gorm.ErrInvalidTransaction, gorm.ErrInvalidData, gorm.ErrMissingWhereClause} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return "statement failed"
}
