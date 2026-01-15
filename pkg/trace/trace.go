// Package trace provides OpenTelemetry tracing with Google Cloud Trace integration.
package trace

import (
	"context"
	"fmt"
	"log"

	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "github.com/nakamasato/cloud-run-slack-bot"
)

// Config holds configuration for tracing.
type Config struct {
	ProjectID   string
	ServiceName string
	// SamplingRate is the probability of sampling a trace (0.0 to 1.0).
	// Use 1.0 for always sampling (default), or lower values for production.
	SamplingRate float64
}

// Provider wraps the OpenTelemetry TracerProvider.
type Provider struct {
	tp *sdktrace.TracerProvider
}

// NewProvider creates a new OpenTelemetry TracerProvider with Google Cloud Trace exporter.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("projectID is required")
	}

	if cfg.ServiceName == "" {
		cfg.ServiceName = "cloud-run-slack-bot"
	}

	if cfg.SamplingRate == 0 {
		cfg.SamplingRate = 1.0 // Default to always sampling
	}

	// Create Google Cloud Trace exporter
	exporter, err := texporter.New(texporter.WithProjectID(cfg.ProjectID))
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create sampler based on configuration
	var sampler sdktrace.Sampler
	if cfg.SamplingRate >= 1.0 {
		// AlwaysSample for development or when we want all traces
		sampler = sdktrace.AlwaysSample()
	} else {
		// ParentBased sampler with TraceIDRatioBased for production
		// This ensures distributed tracing works correctly
		sampler = sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(cfg.SamplingRate),
		)
	}

	// Create TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global TracerProvider
	otel.SetTracerProvider(tp)

	log.Printf("Trace provider initialized for project %s with sampling rate %.2f", cfg.ProjectID, cfg.SamplingRate)

	return &Provider{tp: tp}, nil
}

// Shutdown shuts down the trace provider, flushing any remaining spans.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.tp == nil {
		return nil
	}
	return p.tp.Shutdown(ctx)
}

// GetTracer returns a tracer with the default name.
func GetTracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// GetTracerWithName returns a tracer with a custom name.
func GetTracerWithName(name string) trace.Tracer {
	return otel.Tracer(name)
}
