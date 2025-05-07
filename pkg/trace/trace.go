package trace

import (
	"context"
	"fmt"
	"os"
	"time"

	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"net/http"
)

// Initialize sets up OpenTelemetry and returns a shutdown function
func Initialize(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	// Configure resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Get environment
	env := os.Getenv("ENV")
	isProd := env == "prod"

	// Initialize trace exporter
	var traceExporter sdktrace.SpanExporter
	if isProd {
		// In production, use the Google Cloud Trace exporter
		// This will use Application Default Credentials from the environment
		projectID := os.Getenv("PROJECT")
		traceExporter, err = texporter.New(texporter.WithProjectID(projectID))
		if err != nil {
			return nil, fmt.Errorf("failed to create Google Cloud Trace exporter: %w", err)
		}
	} else {
		// In non-production, use a no-op exporter that doesn't send data
		// This avoids credentials and quota issues during development and testing
		traceExporter = newNoopExporter()
	}

	// Configure trace provider with the appropriate exporter
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
	)
	otel.SetTracerProvider(tracerProvider)

	// Set propagator for distributed tracing
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Return shutdown function
	return func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := tracerProvider.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown tracer provider: %w", err)
		}
		return nil
	}, nil
}

// newNoopExporter creates a no-op exporter that doesn't send any data
// This is useful for development and testing
type noopExporter struct{}

func newNoopExporter() sdktrace.SpanExporter {
	return &noopExporter{}
}

func (e *noopExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *noopExporter) Shutdown(ctx context.Context) error {
	return nil
}

// GetTracer returns the global tracer
func GetTracer() trace.Tracer {
	return otel.Tracer("github.com/nakamasato/cloud-run-slack-bot")
}

// ExtractTraceID returns the trace ID from the span context
func ExtractTraceID(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ""
	}
	return spanCtx.TraceID().String()
}

// WrapHandler wraps an http.Handler with OpenTelemetry instrumentation
func WrapHandler(handler http.Handler, operation string) http.Handler {
	return otelhttp.NewHandler(handler, operation)
}

// WrapHandlerFunc wraps an http.HandlerFunc with OpenTelemetry instrumentation
func WrapHandlerFunc(handlerFunc http.HandlerFunc, operation string) http.Handler {
	return otelhttp.NewHandler(handlerFunc, operation)
}
