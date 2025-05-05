package logger

import (
	"context"
	"log"
	"os"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is a wrapper around zap.Logger to provide contextual logging
type Logger struct {
	*zap.Logger
}

// contextKey is used to store the logger in the context
type contextKey struct{}

var loggerKey = contextKey{}

// configure sets up the core configuration for the logger
func configure(config zap.Config) zap.Config {
	// Ensure UTC timestamps with nanosecond precision
	config.EncoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder

	// Include caller information
	config.DisableCaller = false

	// Configure for Cloud Logging - use JSON encoder
	config.Encoding = "json"

	// Use standard level names
	config.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// Add logging_type field for identifying logs in Cloud Logging
	config.InitialFields = map[string]interface{}{
		"logging_type": "app",
	}

	return config
}

// NewLogger creates a new logger with production configuration
func NewLogger() (*Logger, error) {
	config := configure(zap.NewProductionConfig())

	// Add service name to logs if available
	if serviceName := os.Getenv("SERVICE_NAME"); serviceName != "" {
		config.InitialFields["service"] = serviceName
	}

	zapLogger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{Logger: zapLogger}, nil
}

// NewDevelopmentLogger creates a new logger with development configuration
func NewDevelopmentLogger() (*Logger, error) {
	config := configure(zap.NewDevelopmentConfig())
	zapLogger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{Logger: zapLogger}, nil
}

// WithContext returns a copy of ctx with the Logger attached
func WithContext(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext returns the Logger stored in ctx, or creates a new one with trace data
func FromContext(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(loggerKey).(*Logger); ok {
		// Extract trace information and add to logger if not already present
		return enrichLoggerWithTrace(ctx, logger)
	}

	// If no logger is found in context, create a new one
	logger, err := NewLogger()
	if err != nil {
		log.Printf("Failed to create logger: %v", err)
		return &Logger{Logger: zap.NewExample()}
	}

	// Enrich with trace information
	return enrichLoggerWithTrace(ctx, logger)
}

// enrichLoggerWithTrace adds trace information from context to logger
func enrichLoggerWithTrace(ctx context.Context, logger *Logger) *Logger {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return logger
	}

	// Add trace and span IDs to the logger
	return logger.With(
		zap.String("trace_id", spanCtx.TraceID().String()),
		zap.String("span_id", spanCtx.SpanID().String()),
	)
}

// With creates a child logger with the given fields
func (l *Logger) With(fields ...zap.Field) *Logger {
	return &Logger{Logger: l.Logger.With(fields...)}
}

// WithTraceID adds a trace ID field to the logger
func (l *Logger) WithTraceID(traceID string) *Logger {
	if traceID == "" {
		return l
	}
	return l.With(zap.String("trace_id", traceID))
}
