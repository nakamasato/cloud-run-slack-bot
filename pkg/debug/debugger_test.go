package debug

import (
	"testing"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/adk"
)

func TestDebugResult(t *testing.T) {
	result := DebugResult{
		ResourceName: "my-service",
		ResourceType: "service",
		ProjectID:    "my-project",
		TotalErrors:  10,
		GeneratedAt:  time.Now(),
		LookbackMin:  30,
	}

	if result.ResourceName != "my-service" {
		t.Errorf("Expected resource name 'my-service', got %s", result.ResourceName)
	}
	if result.ResourceType != "service" {
		t.Errorf("Expected resource type 'service', got %s", result.ResourceType)
	}
	if result.ProjectID != "my-project" {
		t.Errorf("Expected project ID 'my-project', got %s", result.ProjectID)
	}
	if result.TotalErrors != 10 {
		t.Errorf("Expected total errors 10, got %d", result.TotalErrors)
	}
	if result.LookbackMin != 30 {
		t.Errorf("Expected lookback 30, got %d", result.LookbackMin)
	}
}

func TestErrorGroupResult(t *testing.T) {
	groupResult := ErrorGroupResult{
		Pattern:        "Connection timeout",
		ErrorCount:     5,
		Representative: "connection timeout after 30s",
		TraceID:        "abc123",
		Analysis: adk.ErrorAnalysis{
			Summary:        "Database connection issues",
			PossibleCauses: []string{"Pool exhaustion"},
			Suggestions:    []string{"Increase pool size"},
		},
	}

	if groupResult.Pattern != "Connection timeout" {
		t.Errorf("Expected pattern 'Connection timeout', got %s", groupResult.Pattern)
	}
	if groupResult.ErrorCount != 5 {
		t.Errorf("Expected error count 5, got %d", groupResult.ErrorCount)
	}
	if groupResult.Analysis.Summary != "Database connection issues" {
		t.Errorf("Unexpected analysis summary: %s", groupResult.Analysis.Summary)
	}
}

func TestConfig(t *testing.T) {
	cfg := Config{
		LookbackDuration: 30 * time.Minute,
	}

	if cfg.LookbackDuration != 30*time.Minute {
		t.Errorf("Expected lookback 30m, got %v", cfg.LookbackDuration)
	}
}
