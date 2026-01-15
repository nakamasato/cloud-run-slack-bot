package adk

import (
	"testing"
	"time"
)

func TestErrorLog(t *testing.T) {
	now := time.Now()
	log := ErrorLog{
		Message:   "connection timeout",
		Timestamp: now,
		TraceID:   "abc123",
	}

	if log.Message != "connection timeout" {
		t.Errorf("Expected message 'connection timeout', got %s", log.Message)
	}
	if log.TraceID != "abc123" {
		t.Errorf("Expected traceID 'abc123', got %s", log.TraceID)
	}
}

func TestErrorGroup(t *testing.T) {
	group := ErrorGroup{
		Pattern: "Database timeout",
		Representative: ErrorLog{
			Message: "connection timeout after 30s",
		},
		Count: 5,
	}

	if group.Pattern != "Database timeout" {
		t.Errorf("Expected pattern 'Database timeout', got %s", group.Pattern)
	}
	if group.Count != 5 {
		t.Errorf("Expected count 5, got %d", group.Count)
	}
}

func TestErrorAnalysis(t *testing.T) {
	analysis := ErrorAnalysis{
		Summary:        "Multiple database connection failures",
		PossibleCauses: []string{"Connection pool exhaustion", "Network issues"},
		Suggestions:    []string{"Increase pool size", "Check network connectivity"},
	}

	if analysis.Summary != "Multiple database connection failures" {
		t.Errorf("Unexpected summary: %s", analysis.Summary)
	}
	if len(analysis.PossibleCauses) != 2 {
		t.Errorf("Expected 2 causes, got %d", len(analysis.PossibleCauses))
	}
	if len(analysis.Suggestions) != 2 {
		t.Errorf("Expected 2 suggestions, got %d", len(analysis.Suggestions))
	}
}

func TestConfig(t *testing.T) {
	cfg := Config{
		Project:   "my-project",
		Location:  "us-central1",
		ModelName: "gemini-2.5-flash",
	}

	if cfg.Project != "my-project" {
		t.Errorf("Expected project 'my-project', got %s", cfg.Project)
	}
	if cfg.Location != "us-central1" {
		t.Errorf("Expected location 'us-central1', got %s", cfg.Location)
	}
	if cfg.ModelName != "gemini-2.5-flash" {
		t.Errorf("Expected model 'gemini-2.5-flash', got %s", cfg.ModelName)
	}
}
