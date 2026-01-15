// Package debug provides debug workflow orchestration for Cloud Run error analysis.
package debug

import (
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/adk"
)

// Config for debugger.
type Config struct {
	LookbackDuration time.Duration // How far back to look for errors
}

// DebugResult contains the complete debug analysis.
type DebugResult struct {
	ResourceName string             // Name of the Cloud Run resource
	ResourceType string             // Type of the resource (service or job)
	ProjectID    string             // GCP project ID
	TotalErrors  int                // Total number of errors found
	ErrorGroups  []ErrorGroupResult // Analysis results per error group
	GeneratedAt  time.Time          // When the analysis was generated
	LookbackMin  int                // Lookback duration in minutes
}

// ErrorGroupResult contains analysis for one error group.
type ErrorGroupResult struct {
	Pattern        string            // Pattern describing this group
	ErrorCount     int               // Number of errors in this group
	Representative string            // Representative error message
	TraceID        string            // Representative trace ID for this group
	Analysis       adk.ErrorAnalysis // LLM analysis of this group
}
