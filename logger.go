package ginboot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// Logger defines a generic logging interface that users can implement to provide their own loggers.
type Logger interface {
	Info(msg string, args ...any)
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)

	Infof(format string, args ...any)
	Debugf(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)

	// WithContext returns a new Logger instance bound to the given context.
	WithContext(ctx context.Context) Logger
}

// slogWrapper is the default implementation of Logger using Go's standard slog package.
type slogWrapper struct {
	logger *slog.Logger
	ctx    context.Context
}

// NewSlogLogger creates a ginboot.Logger backed by a *slog.Logger.
func NewSlogLogger(logger *slog.Logger) Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return &slogWrapper{
		logger: logger,
		ctx:    context.Background(),
	}
}

func extractTraceAttrs(ctx context.Context, args []any) []any {
	if ctx == nil {
		return args
	}
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		traceID := span.SpanContext().TraceID().String()
		spanID := span.SpanContext().SpanID().String()

		hasTrace := false
		for i := 0; i < len(args); i += 2 {
			if k, ok := args[i].(string); ok && (k == "trace_id" || k == "traceId") {
				hasTrace = true
				break
			}
		}
		if !hasTrace {
			newArgs := make([]any, 0, len(args)+4)
			newArgs = append(newArgs, "trace_id", traceID, "span_id", spanID)
			newArgs = append(newArgs, args...)
			return newArgs
		}
	}
	return args
}

func processMsgAndArgs(msg string, args []any) (string, []any) {
	if len(args) > 0 && strings.Contains(msg, "%") {
		msg = fmt.Sprintf(msg, args...)
		args = nil
	}
	return msg, args
}

func (w *slogWrapper) Info(msg string, args ...any) {
	msg, args = processMsgAndArgs(msg, args)
	w.logger.InfoContext(w.ctx, msg, extractTraceAttrs(w.ctx, args)...)
}

func (w *slogWrapper) Debug(msg string, args ...any) {
	msg, args = processMsgAndArgs(msg, args)
	w.logger.DebugContext(w.ctx, msg, extractTraceAttrs(w.ctx, args)...)
}

func (w *slogWrapper) Warn(msg string, args ...any) {
	msg, args = processMsgAndArgs(msg, args)
	w.logger.WarnContext(w.ctx, msg, extractTraceAttrs(w.ctx, args)...)
}

func (w *slogWrapper) Error(msg string, args ...any) {
	msg, args = processMsgAndArgs(msg, args)
	w.logger.ErrorContext(w.ctx, msg, extractTraceAttrs(w.ctx, args)...)
}

func (w *slogWrapper) Infof(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	w.logger.InfoContext(w.ctx, msg, extractTraceAttrs(w.ctx, nil)...)
}

func (w *slogWrapper) Debugf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	w.logger.DebugContext(w.ctx, msg, extractTraceAttrs(w.ctx, nil)...)
}

func (w *slogWrapper) Warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	w.logger.WarnContext(w.ctx, msg, extractTraceAttrs(w.ctx, nil)...)
}

func (w *slogWrapper) Errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	w.logger.ErrorContext(w.ctx, msg, extractTraceAttrs(w.ctx, nil)...)
}

func (w *slogWrapper) WithContext(ctx context.Context) Logger {
	return &slogWrapper{
		logger: w.logger,
		ctx:    ctx,
	}
}
