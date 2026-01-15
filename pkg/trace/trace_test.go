package trace

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/codes"
)

func TestNewProvider_MissingProjectID(t *testing.T) {
	ctx := context.Background()
	_, err := NewProvider(ctx, Config{})
	if err == nil {
		t.Fatal("Expected error when projectID is missing")
	}
}

func TestNewProvider_WithConfig(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ProjectID:    "test-project",
		ServiceName:  "test-service",
		SamplingRate: 0.5,
	}

	provider, err := NewProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	if provider == nil {
		t.Fatal("Expected non-nil provider")
	}

	// Cleanup
	defer func() {
		if err := provider.Shutdown(ctx); err != nil {
			t.Errorf("Failed to shutdown provider: %v", err)
		}
	}()
}

func TestGetTracer(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ProjectID: "test-project",
	}

	provider, err := NewProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer func() {
		_ = provider.Shutdown(ctx) // Ignore error in test cleanup
	}()

	tracer := GetTracer()
	if tracer == nil {
		t.Fatal("Expected non-nil tracer")
	}

	// Test creating a span
	_, span := tracer.Start(ctx, "test-span")
	defer span.End()

	if !span.SpanContext().IsValid() {
		t.Error("Expected valid span context")
	}
}

func TestGetTracerWithName(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ProjectID: "test-project",
	}

	provider, err := NewProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer func() {
		_ = provider.Shutdown(ctx) // Ignore error in test cleanup
	}()

	tracer := GetTracerWithName("custom-tracer")
	if tracer == nil {
		t.Fatal("Expected non-nil tracer")
	}

	// Test creating a span with custom name
	_, span := tracer.Start(ctx, "custom-span")
	defer span.End()

	if !span.SpanContext().IsValid() {
		t.Error("Expected valid span context")
	}
}

func TestSpanErrorRecording(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ProjectID: "test-project",
	}

	provider, err := NewProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer func() {
		_ = provider.Shutdown(ctx) // Ignore error in test cleanup
	}()

	tracer := GetTracer()
	_, span := tracer.Start(ctx, "test-error-span")
	defer span.End()

	// Simulate an error
	testErr := &testError{msg: "test error"}
	span.RecordError(testErr)
	span.SetStatus(codes.Error, testErr.Error())

	// Verify span is still valid
	if !span.SpanContext().IsValid() {
		t.Error("Expected valid span context after error recording")
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
