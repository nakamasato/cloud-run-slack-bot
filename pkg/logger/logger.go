// Package logger provides structured logging with Cloud Logging integration.
package logger

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.Logger with trace context support.
type Logger struct {
	*zap.Logger
	projectID string
}

// NewLogger creates a new logger with production configuration.
// If projectID is empty, it will be read from the GCP_PROJECT_ID environment variable.
func NewLogger(projectID string) (*Logger, error) {
	if projectID == "" {
		projectID = os.Getenv("GCP_PROJECT_ID")
	}

	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.MessageKey = "message"
	config.EncoderConfig.LevelKey = "severity"
	config.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	logger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return &Logger{
		Logger:    logger,
		projectID: projectID,
	}, nil
}

// NewDevelopmentLogger creates a new logger with development configuration.
func NewDevelopmentLogger(projectID string) (*Logger, error) {
	if projectID == "" {
		projectID = os.Getenv("GCP_PROJECT_ID")
	}

	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	logger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build development logger: %w", err)
	}

	return &Logger{
		Logger:    logger,
		projectID: projectID,
	}, nil
}

// WithContext returns a logger with trace context fields if available.
// This follows Google Cloud Logging format for proper log-trace correlation:
// - logging.googleapis.com/trace: Full trace identifier
// - logging.googleapis.com/spanId: Span ID for fine-grained correlation
// - logging.googleapis.com/trace_sampled: Whether the trace was sampled
func (l *Logger) WithContext(ctx context.Context) *zap.Logger {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return l.Logger
	}

	fields := []zap.Field{
		zap.String("logging.googleapis.com/spanId", spanCtx.SpanID().String()),
		zap.Bool("logging.googleapis.com/trace_sampled", spanCtx.IsSampled()),
	}

	// Only add trace field if projectID is available
	if l.projectID != "" {
		fields = append(fields, zap.String("logging.googleapis.com/trace",
			fmt.Sprintf("projects/%s/traces/%s", l.projectID, spanCtx.TraceID().String())))
	} else {
		// Fallback to simple trace_id if no project ID
		fields = append(fields, zap.String("trace_id", spanCtx.TraceID().String()))
	}

	return l.With(fields...)
}

// Sync flushes any buffered log entries.
func (l *Logger) Sync() error {
	return l.Logger.Sync()
}
