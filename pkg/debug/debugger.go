package debug

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/adk"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/logging"
)

// Debugger orchestrates the debug workflow.
type Debugger struct {
	lClients map[string]*logging.Client
	agent    *adk.DebugAgent
	config   Config
}

// NewDebugger creates a new debugger.
func NewDebugger(lClients map[string]*logging.Client, agent *adk.DebugAgent, cfg Config) *Debugger {
	return &Debugger{
		lClients: lClients,
		agent:    agent,
		config:   cfg,
	}
}

// DebugResource performs debug analysis on a Cloud Run service or job.
func (d *Debugger) DebugResource(ctx context.Context, projectID, resourceType, resourceName string) (*DebugResult, error) {
	// Get logging client for the project
	lClient, ok := d.lClients[projectID]
	if !ok {
		return nil, fmt.Errorf("no logging client found for project %s", projectID)
	}

	log.Printf("Starting debug analysis for %s %s in project %s (lookback: %v)\n",
		resourceType, resourceName, projectID, d.config.LookbackDuration)

	// Step 1: Get error logs
	errorLogs, err := lClient.GetErrorLogs(ctx, resourceType, resourceName, d.config.LookbackDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to get error logs: %w", err)
	}

	result := &DebugResult{
		ResourceName: resourceName,
		ResourceType: resourceType,
		ProjectID:    projectID,
		TotalErrors:  len(errorLogs),
		GeneratedAt:  time.Now(),
		LookbackMin:  int(d.config.LookbackDuration.Minutes()),
	}

	if len(errorLogs) == 0 {
		log.Printf("No errors found for %s %s\n", resourceType, resourceName)
		return result, nil
	}

	// Convert logging.LogEntry to adk.ErrorLog
	adkErrors := make([]adk.ErrorLog, len(errorLogs))
	for i, entry := range errorLogs {
		adkErrors[i] = adk.ErrorLog{
			Message:   entry.Message,
			Timestamp: entry.Timestamp,
			TraceID:   entry.TraceID,
		}
	}

	// Step 2: Group errors using LLM
	groups, err := d.agent.GroupErrors(ctx, adkErrors)
	if err != nil {
		return nil, fmt.Errorf("failed to group errors: %w", err)
	}

	// Step 3: Analyze each group
	for _, group := range groups {
		groupResult := ErrorGroupResult{
			Pattern:        group.Pattern,
			ErrorCount:     group.Count,
			Representative: group.Representative.Message,
			TraceID:        group.Representative.TraceID,
		}

		// Get trace logs if available
		var traceLogs []string
		if group.Representative.TraceID != "" {
			traceEntries, err := lClient.GetLogsByTraceID(ctx, group.Representative.TraceID)
			if err != nil {
				log.Printf("Warning: failed to get trace logs for %s: %v\n", group.Representative.TraceID, err)
			} else {
				for _, entry := range traceEntries {
					traceLogs = append(traceLogs, fmt.Sprintf("[%s] %s: %s",
						entry.Timestamp.Format(time.RFC3339),
						entry.Severity,
						entry.Message))
				}
			}
		}

		// Analyze the error group
		analysis, err := d.agent.AnalyzeErrors(ctx, group, traceLogs)
		if err != nil {
			log.Printf("Warning: failed to analyze error group %s: %v\n", group.Pattern, err)
			groupResult.Analysis = adk.ErrorAnalysis{
				Summary:        fmt.Sprintf("Analysis unavailable for: %s", group.Pattern),
				PossibleCauses: []string{"Analysis failed"},
				Suggestions:    []string{"Review logs manually"},
			}
		} else {
			groupResult.Analysis = *analysis
		}

		result.ErrorGroups = append(result.ErrorGroups, groupResult)
	}

	log.Printf("Debug analysis complete: %d errors in %d groups\n", result.TotalErrors, len(result.ErrorGroups))
	return result, nil
}
