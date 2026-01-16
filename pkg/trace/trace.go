// Package trace provides OpenTelemetry tracing with Google Cloud Trace integration.
package trace

import (
	"context"
	"fmt"

	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
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
	// TestMode skips creating the real Cloud Trace exporter.
	// Used in tests to avoid requiring GCP credentials.
	TestMode bool
}

// Provider wraps the OpenTelemetry TracerProvider.
type Provider struct {
	tp *sdktrace.TracerProvider
}

// NewProvider creates a new OpenTelemetry TracerProvider with Google Cloud Trace exporter.
func NewProvider(ctx context.Context, cfg Config, logger *zap.Logger) (*Provider, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("projectID is required")
	}

	if cfg.ServiceName == "" {
		cfg.ServiceName = "cloud-run-slack-bot"
	}

	if cfg.SamplingRate == 0 {
		cfg.SamplingRate = 1.0 // Default to always sampling
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

	// Create TracerProvider options
	var tpOptions []sdktrace.TracerProviderOption
	tpOptions = append(tpOptions, sdktrace.WithResource(res))
	tpOptions = append(tpOptions, sdktrace.WithSampler(sampler))

	// Only create exporter in production mode
	if !cfg.TestMode {
		// Create Google Cloud Trace exporter
		exporter, err := texporter.New(texporter.WithProjectID(cfg.ProjectID))
		if err != nil {
			return nil, fmt.Errorf("failed to create trace exporter: %w", err)
		}
		tpOptions = append(tpOptions, sdktrace.WithBatcher(exporter))
	}

	// Create TracerProvider
	tp := sdktrace.NewTracerProvider(tpOptions...)

	// Set global TracerProvider
	otel.SetTracerProvider(tp)

	logger.Info("Trace provider initialized",
		zap.String("project_id", cfg.ProjectID),
		zap.Float64("sampling_rate", cfg.SamplingRate))

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
