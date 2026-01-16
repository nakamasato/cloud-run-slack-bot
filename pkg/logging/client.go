// Package logging provides a client for reading Cloud Logging entries.
package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/logging/logadmin"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// LogEntry represents a simplified log entry for processing.
type LogEntry struct {
	Timestamp time.Time
	Severity  string
	Message   string
	TraceID   string
	SpanID    string
	Labels    map[string]string
	Resource  ResourceInfo
}

// ResourceInfo contains information about the logged resource.
type ResourceInfo struct {
	Type   string
	Labels map[string]string
}

// Client wraps Cloud Logging logadmin client.
type Client struct {
	project string
	client  *logadmin.Client
	logger  *zap.Logger
}

// NewLoggingClient creates a new logging client for a project.
func NewLoggingClient(ctx context.Context, project string, logger *zap.Logger) (*Client, error) {
	client, err := logadmin.NewClient(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("failed to create logadmin client: %w", err)
	}
	logger.Info("Logging client created", zap.String("project", project))
	return &Client{project: project, client: client, logger: logger}, nil
}

// GetErrorLogs retrieves error logs for a Cloud Run service or job.
func (c *Client) GetErrorLogs(ctx context.Context, resourceType, resourceName string, duration time.Duration) ([]LogEntry, error) {
	startTime := time.Now().Add(-duration)

	var filter string
	switch resourceType {
	case "service":
		filter = fmt.Sprintf(
			`resource.type = "cloud_run_revision" AND resource.labels.service_name = "%s" AND severity >= ERROR AND timestamp >= "%s"`,
			resourceName,
			startTime.Format(time.RFC3339),
		)
	case "job":
		filter = fmt.Sprintf(
			`resource.type = "cloud_run_job" AND resource.labels.job_name = "%s" AND severity >= ERROR AND timestamp >= "%s"`,
			resourceName,
			startTime.Format(time.RFC3339),
		)
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	c.logger.Info("Getting error logs",
		zap.String("project", c.project),
		zap.String("filter", filter))

	return c.queryLogs(ctx, filter)
}

// GetLogsByTraceID retrieves all logs for a specific trace.
func (c *Client) GetLogsByTraceID(ctx context.Context, traceID string) ([]LogEntry, error) {
	// Cloud Run trace format: projects/{project}/traces/{trace_id}
	filter := fmt.Sprintf(`trace = "projects/%s/traces/%s"`, c.project, traceID)
	c.logger.Info("Getting logs by trace ID",
		zap.String("project", c.project),
		zap.String("trace_id", traceID))
	return c.queryLogs(ctx, filter)
}

func (c *Client) queryLogs(ctx context.Context, filter string) ([]LogEntry, error) {
	var entries []LogEntry
	const maxEntries = 100 // Limit to prevent memory exhaustion and API quota issues

	it := c.client.Entries(ctx, logadmin.Filter(filter), logadmin.NewestFirst())
	for len(entries) < maxEntries {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate log entries: %w", err)
		}

		logEntry := LogEntry{
			Timestamp: entry.Timestamp,
			Severity:  entry.Severity.String(),
			Labels:    entry.Labels,
		}

		// Extract message from payload
		if entry.Payload != nil {
			switch p := entry.Payload.(type) {
			case string:
				logEntry.Message = p
			case map[string]interface{}:
				if msg, ok := p["message"].(string); ok {
					logEntry.Message = msg
				} else if textPayload, ok := p["textPayload"].(string); ok {
					logEntry.Message = textPayload
				} else {
					// Fallback to serializing the whole payload as JSON, which is better for LLM analysis than the Go map format
					jsonBytes, err := json.Marshal(p)
					if err == nil {
						logEntry.Message = string(jsonBytes)
					} else {
						logEntry.Message = fmt.Sprintf("%v", p)
					}
				}
			default:
				logEntry.Message = fmt.Sprintf("%v", p)
			}
		}

		// Extract trace ID from trace field (format: projects/{project}/traces/{trace_id})
		if entry.Trace != "" {
			parts := strings.Split(entry.Trace, "/")
			if len(parts) >= 4 {
				logEntry.TraceID = parts[len(parts)-1]
			}
		}
		logEntry.SpanID = entry.SpanID

		// Extract resource info
		if entry.Resource != nil {
			logEntry.Resource = ResourceInfo{
				Type:   entry.Resource.Type,
				Labels: entry.Resource.Labels,
			}
		}

		entries = append(entries, logEntry)
	}

	c.logger.Info("Retrieved log entries",
		zap.String("project", c.project),
		zap.Int("count", len(entries)))
	return entries, nil
}

// Close closes the underlying client.
func (c *Client) Close() error {
	return c.client.Close()
}
