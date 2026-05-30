package ginboot

import (
	"context"
	"log/slog"
)

// Logger defines a generic logging interface that users can implement to provide their own loggers.
type Logger interface {
	Info(msg string, args ...any)
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
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

func (w *slogWrapper) Info(msg string, args ...any) {
	w.logger.InfoContext(w.ctx, msg, args...)
}

func (w *slogWrapper) Debug(msg string, args ...any) {
	w.logger.DebugContext(w.ctx, msg, args...)
}

func (w *slogWrapper) Warn(msg string, args ...any) {
	w.logger.WarnContext(w.ctx, msg, args...)
}

func (w *slogWrapper) Error(msg string, args ...any) {
	w.logger.ErrorContext(w.ctx, msg, args...)
}

func (w *slogWrapper) WithContext(ctx context.Context) Logger {
	return &slogWrapper{
		logger: w.logger,
		ctx:    ctx,
	}
}
