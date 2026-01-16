package debug

import (
	"context"
	"fmt"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/adk"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/logging"
	"go.uber.org/zap"
)

const maxTraceLogsForAnalysis = 20 // Limit trace logs to prevent overwhelming LLM

// Debugger orchestrates the debug workflow.
type Debugger struct {
	lClients map[string]*logging.Client
	agent    *adk.DebugAgent
	config   Config
	logger   *zap.Logger
}

// NewDebugger creates a new debugger.
func NewDebugger(lClients map[string]*logging.Client, agent *adk.DebugAgent, cfg Config, logger *zap.Logger) *Debugger {
	return &Debugger{
		lClients: lClients,
		agent:    agent,
		config:   cfg,
		logger:   logger,
	}
}

// DebugResource performs debug analysis on a Cloud Run service or job.
func (d *Debugger) DebugResource(ctx context.Context, projectID, resourceType, resourceName string) (*DebugResult, error) {
	// Get logging client for the project
	lClient, ok := d.lClients[projectID]
	if !ok {
		return nil, fmt.Errorf("no logging client found for project %s", projectID)
	}

	d.logger.Info("Starting debug analysis",
		zap.String("resource_type", resourceType),
		zap.String("resource_name", resourceName),
		zap.String("project_id", projectID),
		zap.Duration("lookback", d.config.LookbackDuration))

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
		d.logger.Info("No errors found",
			zap.String("resource_type", resourceType),
			zap.String("resource_name", resourceName))
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
			TraceTimestamp: group.Representative.Timestamp,
		}

		// Get trace logs if available (limit to most recent relevant logs)
		var traceLogs []string
		if group.Representative.TraceID != "" {
			traceEntries, err := lClient.GetLogsByTraceID(ctx, group.Representative.TraceID)
			if err != nil {
				d.logger.Warn("Failed to get trace logs",
					zap.String("trace_id", group.Representative.TraceID),
					zap.Error(err))
			} else {
				for i, entry := range traceEntries {
					if i >= maxTraceLogsForAnalysis {
						break
					}
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
			d.logger.Warn("Failed to analyze error group",
				zap.String("pattern", group.Pattern),
				zap.Error(err))
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

	d.logger.Info("Debug analysis complete",
		zap.Int("total_errors", result.TotalErrors),
		zap.Int("group_count", len(result.ErrorGroups)))
	return result, nil
}
