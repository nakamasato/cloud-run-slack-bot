package logger

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewLogger(t *testing.T) {
	logger, err := NewLogger("test-project")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	if logger == nil {
		t.Fatal("Expected non-nil logger")
	}
	if logger.projectID != "test-project" {
		t.Errorf("Expected projectID 'test-project', got '%s'", logger.projectID)
	}
	defer func() {
		_ = logger.Sync() // Ignore error in test cleanup
	}()
}

func TestNewDevelopmentLogger(t *testing.T) {
	logger, err := NewDevelopmentLogger("test-project")
	if err != nil {
		t.Fatalf("Failed to create development logger: %v", err)
	}
	if logger == nil {
		t.Fatal("Expected non-nil logger")
	}
	defer func() {
		_ = logger.Sync() // Ignore error in test cleanup
	}()
}

func TestWithContext_NoTrace(t *testing.T) {
	// Create logger with observer core for testing
	core, _ := observer.New(zapcore.InfoLevel)
	logger := &Logger{
		Logger:    zap.New(core),
		projectID: "test-project",
	}

	ctx := context.Background()
	loggerWithCtx := logger.WithContext(ctx)

	if loggerWithCtx == nil {
		t.Fatal("Expected non-nil logger with context")
	}
}

func TestWithContext_WithTrace(t *testing.T) {
	// Create logger with observer core for testing
	core, observed := observer.New(zapcore.InfoLevel)
	logger := &Logger{
		Logger:    zap.New(core),
		projectID: "test-project",
	}

	// Create a context with a trace span
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)

	loggerWithCtx := logger.WithContext(ctx)
	loggerWithCtx.Info("test message")

	logs := observed.All()
	if len(logs) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(logs))
	}

	logEntry := logs[0]
	contextMap := logEntry.ContextMap()

	// Check for Cloud Logging fields
	if trace, ok := contextMap["logging.googleapis.com/trace"].(string); !ok {
		t.Error("Expected logging.googleapis.com/trace field")
	} else if trace != "projects/test-project/traces/4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("Expected trace field to be 'projects/test-project/traces/4bf92f3577b34da6a3ce929d0e0e4736', got '%s'", trace)
	}

	if spanID, ok := contextMap["logging.googleapis.com/spanId"].(string); !ok {
		t.Error("Expected logging.googleapis.com/spanId field")
	} else if spanID != "00f067aa0ba902b7" {
		t.Errorf("Expected spanId field to be '00f067aa0ba902b7', got '%s'", spanID)
	}

	if sampled, ok := contextMap["logging.googleapis.com/trace_sampled"].(bool); !ok {
		t.Error("Expected logging.googleapis.com/trace_sampled field")
	} else if !sampled {
		t.Error("Expected trace_sampled to be true")
	}
}

func TestWithContext_NoProjectID(t *testing.T) {
	// Create logger with observer core for testing
	core, observed := observer.New(zapcore.InfoLevel)
	logger := &Logger{
		Logger:    zap.New(core),
		projectID: "", // No project ID
	}

	// Create a context with a trace span
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)

	loggerWithCtx := logger.WithContext(ctx)
	loggerWithCtx.Info("test message")

	logs := observed.All()
	if len(logs) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(logs))
	}

	logEntry := logs[0]
	contextMap := logEntry.ContextMap()

	// Should fall back to simple trace_id field
	if _, ok := contextMap["trace_id"].(string); !ok {
		t.Error("Expected trace_id field as fallback when no project ID")
	}
}
