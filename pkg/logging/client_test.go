package logging

import (
	"testing"
)

func TestLogEntry(t *testing.T) {
	entry := LogEntry{
		Severity: "ERROR",
		Message:  "test error message",
		TraceID:  "abc123",
	}

	if entry.Severity != "ERROR" {
		t.Errorf("Expected severity ERROR, got %s", entry.Severity)
	}
	if entry.Message != "test error message" {
		t.Errorf("Expected message 'test error message', got %s", entry.Message)
	}
	if entry.TraceID != "abc123" {
		t.Errorf("Expected traceID 'abc123', got %s", entry.TraceID)
	}
}

func TestResourceInfo(t *testing.T) {
	resource := ResourceInfo{
		Type: "cloud_run_revision",
		Labels: map[string]string{
			"service_name": "my-service",
		},
	}

	if resource.Type != "cloud_run_revision" {
		t.Errorf("Expected type 'cloud_run_revision', got %s", resource.Type)
	}
	if resource.Labels["service_name"] != "my-service" {
		t.Errorf("Expected service_name 'my-service', got %s", resource.Labels["service_name"])
	}
}
