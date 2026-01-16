# Tracing and Logging

This document explains how tracing and logging are implemented in the cloud-run-slack-bot application.

## Overview

The application uses:
- **OpenTelemetry** for distributed tracing
- **Google Cloud Trace** for trace storage and visualization
- **Zap** for structured logging
- **Google Cloud Logging** for log storage and correlation with traces

## Architecture

### Tracing

The application uses OpenTelemetry to instrument key operations and export traces to Google Cloud Trace.

**Key Components:**
- `pkg/trace`: OpenTelemetry initialization and configuration
- Instrumented operations:
  - Slack event handling
  - Cloud Run API calls
  - Monitoring API calls
  - Chart generation

**Sampling Strategy:**
- Default: AlwaysSample (100% of traces)
- Production: Consider using ParentBased sampling with TraceIDRatioBased for lower sampling rates
- Configurable via `trace.Config.SamplingRate`

### Logging

The application uses Zap for structured logging with special fields for Cloud Logging integration.

**Key Components:**
- `pkg/logger`: Zap logger with Cloud Logging field support
- Log-trace correlation fields follow [Google Cloud Logging format](https://cloud.google.com/logging/docs/structured-logging):
  - `logging.googleapis.com/trace`: Full trace identifier in format `projects/[PROJECT_ID]/traces/[TRACE_ID]`
  - `logging.googleapis.com/spanId`: Span ID for fine-grained correlation
  - `logging.googleapis.com/trace_sampled`: Whether the trace was sampled

## Configuration

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GCP_PROJECT_ID` | Optional | GCP project ID used for all GCP services (Cloud Trace, Cloud Logging, Vertex AI). If not set, falls back to the first project ID in `PROJECTS_CONFIG`. |
| `TRACING_ENABLED` | Optional | Set to `true` to enable distributed tracing with Cloud Trace. When enabled, requires `GCP_PROJECT_ID` to be set. Default: `false` (tracing disabled). |
| `LOG_LEVEL` | Optional | Controls the minimum log level for structured logging. Valid values: `debug`, `info`, `warn`, `error`. Default: `info`. Use `debug` for detailed diagnostic information during development or troubleshooting. |

### Initialization

Tracing and logging are initialized in `main.go`:

```go
// Get GCP project ID
projectID := os.Getenv("GCP_PROJECT_ID")
if projectID == "" && len(cfg.Projects) > 0 {
    // Fallback to first project ID if GCP_PROJECT_ID not set
    projectID = cfg.Projects[0].ID
}

// Initialize tracing if TRACING_ENABLED is set
var traceProvider *trace.Provider
tracingEnabled := os.Getenv("TRACING_ENABLED") == "true"
if tracingEnabled && projectID != "" {
    traceProvider, err := trace.NewProvider(ctx, trace.Config{
        ProjectID:    projectID,
        ServiceName:  "cloud-run-slack-bot",
        SamplingRate: 1.0, // Adjust for production
    })
    if err != nil {
        log.Printf("Warning: Failed to initialize tracing: %v", err)
    } else {
        defer func() {
            shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()
            if err := traceProvider.Shutdown(shutdownCtx); err != nil {
                log.Printf("Failed to shutdown trace provider: %v", err)
            }
        }()
    }
}

// Initialize structured logger
zapLogger, err := logger.NewLogger(projectID)
if err != nil {
    log.Fatalf("Failed to initialize logger: %v", err)
}
defer func() {
    if err := zapLogger.Sync(); err != nil {
        log.Printf("Failed to sync logger: %v", err)
    }
}()
```

## Usage

### Creating Spans

To instrument a function with tracing:

```go
import (
    "github.com/nakamasato/cloud-run-slack-bot/pkg/trace"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
)

func myFunction(ctx context.Context) error {
    ctx, span := trace.GetTracer().Start(ctx, "myFunction")
    defer span.End()

    // Add attributes
    span.SetAttributes(
        attribute.String("key", "value"),
        attribute.Int("count", 42),
    )

    // Do work...
    if err != nil {
        // Record errors
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return err
    }

    return nil
}
```

### Logging with Trace Context

To log with trace correlation:

```go
import (
    "github.com/nakamasato/cloud-run-slack-bot/pkg/logger"
    "go.uber.org/zap"
)

// Get logger with trace context from request context
loggerWithCtx := zapLogger.WithContext(ctx)
loggerWithCtx.Info("Processing request",
    zap.String("user_id", userID),
    zap.String("action", action),
)
```

## Viewing Traces and Logs

### Google Cloud Trace

1. Navigate to [Cloud Trace](https://console.cloud.google.com/traces) in GCP Console
2. Select your project
3. View trace timeline and span details
4. Filter by service, latency, time range

### Google Cloud Logging

1. Navigate to [Cloud Logging](https://console.cloud.google.com/logs) in GCP Console
2. Select your project
3. Use the query builder to filter logs
4. Click on a log entry to see correlated trace (if available)

### Log-Trace Correlation

When viewing logs in Cloud Logging, entries with trace correlation will show a "View in Trace" link that takes you directly to the associated trace in Cloud Trace.

To query logs for a specific trace:

```
trace="projects/[PROJECT_ID]/traces/[TRACE_ID]"
```

## Best Practices

### Tracing

1. **Add spans to key business operations**: Instrument functions that represent significant work or external API calls
2. **Use semantic attributes**: Follow [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/) for standard attributes
3. **Record errors in spans**: Always call `span.RecordError(err)` and `span.SetStatus(codes.Error, ...)` when errors occur
4. **Propagate context**: Always pass `context.Context` to functions that perform I/O operations
5. **Keep span names concise**: Use format `package.function` (e.g., `cloudrun.ListServices`)

### Logging

1. **Use structured logging**: Always use zap fields instead of string formatting
2. **Add trace context**: Use `logger.WithContext(ctx)` to ensure log-trace correlation
3. **Log at appropriate levels**:
   - `Debug`: Detailed diagnostic information
   - `Info`: General informational messages
   - `Warn`: Warning messages for potentially harmful situations
   - `Error`: Error messages for error events
4. **Include relevant context**: Add fields that help with debugging (user_id, resource_name, etc.)

### Performance

1. **Sampling in production**: Use lower sampling rates (e.g., 0.1 for 10%) in high-traffic production environments
2. **Attribute limits**: Be mindful of the number and size of span attributes
3. **Batch processing**: OpenTelemetry SDK batches spans automatically for efficient export

## Troubleshooting

### Traces not appearing in Cloud Trace

1. Verify `TRACING_ENABLED=true` is set
2. Verify `GCP_PROJECT_ID` environment variable is set correctly
3. Check service account has `roles/cloudtrace.agent` permission
4. Review application logs for trace exporter errors
5. Ensure sampling rate is not too low (check `SamplingRate` in configuration)

### Logs not correlated with traces

1. Verify `GCP_PROJECT_ID` is set and logger is initialized with correct project ID
2. Ensure `logger.WithContext(ctx)` is called with a context containing trace information
3. Check that OpenTelemetry tracer is properly initialized before creating spans
4. Verify log entries contain `logging.googleapis.com/trace` field

### High latency or performance issues

1. Consider reducing sampling rate in production
2. Check if too many attributes are being added to spans
3. Review span creation frequency and optimize instrumentation
4. Monitor Cloud Trace API quota usage

## References

- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/languages/go/)
- [Google Cloud Trace Setup](https://cloud.google.com/trace/docs/setup/go-ot)
- [OpenTelemetry Operations Go Exporter](https://github.com/GoogleCloudPlatform/opentelemetry-operations-go)
- [Google Cloud Logging Structured Logging](https://cloud.google.com/logging/docs/structured-logging)
- [Zap Documentation](https://pkg.go.dev/go.uber.org/zap)
